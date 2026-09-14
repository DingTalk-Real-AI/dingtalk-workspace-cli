package clitrack

import (
	"errors"
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// attributionVars 是 detectExecutor 会探测的全部环境变量。
// 每个用例前先清空,排除测试环境自身注入的干扰。
var attributionVars = []string{
	"QODER_WORK_INTEGRATION_MODE", "QODERWORK_IS_CN", "CLAUDECODE", "QODER_IDE",
	"QODERWAKE_HOME", "QODERWAKE_CLI_PATH", "QODERWAKE_DAEMON_URL",
	"QODER_PRODUCT_ID", "QODER_SESSION_TYPE", "OPENCODE",
	"WUKONG_PRODUCT_CODE", "WUKONG_BIN", "WUKONG_CLIENT_VERSION",
	"CODEX_CI", "CODEX_SHELL", "CODEX_PERMISSION_PROFILE",
	"CI", "GITHUB_ACTIONS", "GITLAB_CI", "JENKINS_URL",
}

func clearAttributionVars(t *testing.T) {
	t.Helper()
	for _, k := range attributionVars {
		k := k
		if v, ok := os.LookupEnv(k); ok {
			t.Cleanup(func() { os.Setenv(k, v) })
		}
		os.Unsetenv(k)
	}
	// 前缀族无法用清单穷举,按前缀清扫整个环境,防止测试环境残留干扰
	for _, kv := range os.Environ() {
		i := strings.IndexByte(kv, '=')
		if i <= 0 {
			continue
		}
		name := kv[:i]
		for _, p := range []string{
			"QODER_", "QODERWORK_", "QW_", "QWENWORK_", "WUKONG_", "CODEX_",
		} {
			if strings.HasPrefix(name, p) {
				name := name
				if v, ok := os.LookupEnv(name); ok {
					t.Cleanup(func() { os.Setenv(name, v) })
				}
				os.Unsetenv(name)
				break
			}
		}
	}
}

// detectExecutor 按优先级先命中先得;顺序错乱会导致共享变量误判。
func TestDetectExecutorPriority(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"无任何标记", nil, executorNone},
		{"QoderWork 主判定", map[string]string{"QODER_WORK_INTEGRATION_MODE": "1"}, executorQoderwork},
		// 陷阱:QODER* 家族与 CI 同时存在时,产品标记优先
		{"QoderWork 优先于 CI 与共享变量", map[string]string{
			"QODER_WORK_INTEGRATION_MODE": "1", "CI": "true", "GITHUB_ACTIONS": "true",
		}, executorQoderwork},
		{"值为 0 不算命中", map[string]string{"QODER_WORK_INTEGRATION_MODE": "0"}, executorNone},
		// 第三批实测:国内版与国际版共享 INTEGRATION_MODE,靠 IS_CN 分流
		{"QwenWork 国内版分流", map[string]string{
			"QODER_WORK_INTEGRATION_MODE": "1", "QODERWORK_IS_CN": "1",
		}, executorQwenwork},
		{"Claude Code", map[string]string{"CLAUDECODE": "1"}, executorClaudeCode},
		{"CLAUDECODE 优先于 OPENCODE", map[string]string{"CLAUDECODE": "1", "OPENCODE": "x"}, executorClaudeCode},
		{"Qoder IDE 存在即命中", map[string]string{"QODER_IDE": ""}, executorQoderIDE},
		{"QODER_IDE 优先于 OPENCODE", map[string]string{"QODER_IDE": "1", "OPENCODE": "1"}, executorQoderIDE},
		{"QoderWake daemon", map[string]string{"QODERWAKE_HOME": "/x"}, executorQoderwake},
		// 前缀族:清单之外的族变量同样命中(对照表会随宿主升级扩充)
		{"QoderWake 前缀族新变量", map[string]string{"QODERWAKE_ENV_MODE": "prod"}, executorQoderwake},
		// 防混淆:SESSION_TYPE 在 QoderWake 也出现,必须先命中 QoderWake
		{"QoderWake 优先于 SESSION_TYPE 误判", map[string]string{
			"QODER_SESSION_TYPE": "chat", "QODERWAKE_CLI_PATH": "/x",
		}, executorQoderwake},
		{"Qoder 改用 PRODUCT_ID", map[string]string{"QODER_PRODUCT_ID": "p"}, executorQoder},
		// 防混淆:SESSION_TYPE 单独出现不再判 Qoder(与 QoderWake 共享)
		{"SESSION_TYPE 单独出现不判定", map[string]string{"QODER_SESSION_TYPE": "chat"}, executorNone},
		{"OpenCode", map[string]string{"OPENCODE": "1"}, executorOpencode},
		{"钉钉悟空", map[string]string{"WUKONG_PRODUCT_CODE": "x"}, executorWukong},
		{"Wukong 前缀族新变量", map[string]string{"WUKONG_TRACE_ID": "t"}, executorWukong},
		{"Codex 内嵌桌面版", map[string]string{"CODEX_SHELL": "bash"}, executorCodex},
		{"Codex 前缀族新变量", map[string]string{"CODEX_INTERNAL_ORIGINATOR_OVERRIDE": "x"}, executorCodex},
		{"CODEX 标记优先于 CI", map[string]string{"CODEX_SHELL": "bash", "CI": "true"}, executorCodex},
		{"CI", map[string]string{"CI": "true"}, executorCI},
		{"GitHub Actions", map[string]string{"GITHUB_ACTIONS": "true"}, executorCI},
		{"GitLab CI", map[string]string{"GITLAB_CI": "true"}, executorCI},
		{"Jenkins", map[string]string{"JENKINS_URL": "http://jenkins.local/"}, executorCI},
		{"产品标记优先于 CI", map[string]string{"OPENCODE": "1", "CI": "true"}, executorOpencode},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			clearAttributionVars(t)
			for k, v := range c.env {
				t.Setenv(k, v)
			}
			if got := detectExecutor(); got != c.want {
				t.Errorf("detectExecutor() = %q, 期望 %q", got, c.want)
			}
		})
	}
}

// 前缀族匹配:命中前缀即真;但泄漏变量即使前缀命中也必须被排除。
func TestEnvHasAnyPrefixAndLeakBlacklist(t *testing.T) {
	clearAttributionVars(t)

	t.Setenv("QODER_SOME_FUTURE_VAR", "x")
	if !envHasAnyPrefix("QODER_") {
		t.Error("前缀命中应返回 true")
	}
	if envHasAnyPrefix("NONEXIST_") {
		t.Error("无前缀命中应返回 false")
	}

	// 泄漏变量:即使前缀命中也不得参与判定(黑名单优先)
	t.Setenv("QODER_WORKER_RUNTIME_PATH", "/leaked")
	t.Setenv("QW_QODER_WORKER_RUNTIME_PATH", "/leaked")
	os.Unsetenv("QODER_SOME_FUTURE_VAR")
	if envHasAnyPrefix("QODER_", "QW_") {
		t.Error("泄漏变量被黑名单排除后,不应再有前缀命中")
	}
}

// 词表锚定:上报值必须是对照表正式名原文(含大小写与空格)。
// 词表是跨 CLI 聚合的 key,一经发布即冻结;改动这里等于改协议。
func TestExecutorVocabularyGolden(t *testing.T) {
	golden := map[string]string{
		executorQoderwork:  "QoderWork",
		executorQwenwork:   "QwenWork",
		executorClaudeCode: "Claude Code",
		executorQoderIDE:   "Qoder IDE",
		executorQoder:      "Qoder",
		executorQoderwake:  "QoderWake",
		executorQoderCLI:   "Qoder CLI",
		executorOpencode:   "OpenCode",
		executorWukong:     "Wukong",
		executorCodex:      "Codex",
		executorCI:         "CI",
		executorNone:       "none",
	}
	for got, want := range golden {
		if got != want {
			t.Errorf("枚举值漂移:得到 %q,对照表正式名为 %q", got, want)
		}
	}
	if len(golden) != 12 {
		t.Errorf("词表应为 12 个值(10 宿主 + CI + none),实际 %d", len(golden))
	}
}

// 安全兜底:无论变量值多敏感,返回值只能是枚举词表内的值。
func TestDetectExecutorNeverLeaksValue(t *testing.T) {
	allowed := map[string]bool{
		executorQoderwork: true, executorQwenwork: true, executorClaudeCode: true,
		executorQoderIDE: true, executorQoder: true, executorQoderwake: true,
		executorQoderCLI: true, executorOpencode: true, executorWukong: true,
		executorCodex: true, executorCI: true, executorNone: true,
	}
	clearAttributionVars(t)
	t.Setenv("OPENCODE", "--token=SECRET&password=xxx")
	got := detectExecutor()
	if !allowed[got] {
		t.Fatalf("detectExecutor() = %q 不在枚举词表内,可能泄漏变量值", got)
	}
}

// 实测样本:QoderWork 内直连宿主(两跳,父进程即 pid 1 之下)。
func TestAncestryFromPsTableQoderWork(t *testing.T) {
	table := "" +
		"   100     200 /bin/zsh\n" +
		"   200       1 /Applications/QoderWork.app/Contents/MacOS/QoderWork\n"
	want := "/bin/zsh|/Applications/QoderWork.app/Contents/MacOS/QoderWork"
	if got := ancestryFromPsTable(table, 100); got != want {
		t.Errorf("ancestry = %q, 期望 %q", got, want)
	}
}

// 实测样本:Qoder IDE 链含 Electron Helper 中间跳,且路径带空格。
func TestAncestryFromPsTableHelperHopAndSpaces(t *testing.T) {
	table := "" +
		"  2653   19693 /bin/zsh\n" +
		" 19693   19647 /Applications/Qoder IDE.app/Contents/Frameworks/Qoder Helper.app/Contents/MacOS/Qoder Helper\n" +
		" 19647       1 /Applications/Qoder IDE.app/Contents/MacOS/Qoder\n"
	want := "/bin/zsh" +
		"|/Applications/Qoder IDE.app/Contents/Frameworks/Qoder Helper.app/Contents/MacOS/Qoder Helper" +
		"|/Applications/Qoder IDE.app/Contents/MacOS/Qoder"
	if got := ancestryFromPsTable(table, 2653); got != want {
		t.Errorf("ancestry = %q, 期望 %q", got, want)
	}
}

// 实测样本:agent CLI(opencode)从 Warp 启动,链上有终端容器;全链保留,执行者判定是分析侧的事。
func TestAncestryFromPsTableAgentInsideTerminal(t *testing.T) {
	table := "" +
		"   100     200 /bin/zsh\n" +
		"   200     300 opencode\n" +
		"   300     400 -zsh\n" +
		"   400       1 /Applications/Warp.app/Contents/MacOS/stable\n"
	want := "/bin/zsh|opencode|-zsh|/Applications/Warp.app/Contents/MacOS/stable"
	if got := ancestryFromPsTable(table, 100); got != want {
		t.Errorf("ancestry = %q, 期望 %q", got, want)
	}
}

// nohup 剥离:自身直挂 pid 1,链只有一跳。
func TestAncestryFromPsTableDetached(t *testing.T) {
	table := "   100       1 /usr/local/bin/mycli\n"
	if got := ancestryFromPsTable(table, 100); got != "/usr/local/bin/mycli" {
		t.Errorf("ancestry = %q, 期望单跳", got)
	}
}

// 父进程不在表中(恰好退出):截断,保留已采集部分,不报错。
func TestAncestryFromPsTableMissingParentTruncates(t *testing.T) {
	table := "   100     999 /bin/zsh\n"
	if got := ancestryFromPsTable(table, 100); got != "/bin/zsh" {
		t.Errorf("ancestry = %q, 期望截断为单跳", got)
	}
}

// self 不在表中:返回空串。
func TestAncestryFromPsTableSelfMissing(t *testing.T) {
	if got := ancestryFromPsTable("   100       1 /bin/zsh\n", 999); got != "" {
		t.Errorf("self 不在表中应返回空,得到 %q", got)
	}
}

// ppid 环路:靠跳数上限兜底,不得死循环。
func TestAncestryFromPsTableLoopGuard(t *testing.T) {
	table := "" +
		"   100     200 /a\n" +
		"   200     100 /b\n"
	got := ancestryFromPsTable(table, 100)
	if hops := strings.Count(got, "|") + 1; hops != maxAncestryHops {
		t.Errorf("环路时应在 %d 跳截断,实际 %d 跳", maxAncestryHops, hops)
	}
}

// 超过 500 字符时按 "|" 边界截断,不留半截路径。
func TestAncestryTruncation(t *testing.T) {
	var b strings.Builder
	const hops = 20
	for i := 0; i < hops; i++ {
		ppid := i + 1
		if i == hops-1 {
			ppid = 1
		}
		b.WriteString(padLine(i, ppid))
	}
	got := ancestryFromPsTable(b.String(), 0)
	if len(got) > maxAncestryLen {
		t.Fatalf("截断后长度 %d 超过上限 %d", len(got), maxAncestryLen)
	}
	if strings.HasSuffix(got, "|") || strings.Contains(got, "||") {
		t.Errorf("截断应落在完整路径边界,得到 %q", got)
	}
	for _, seg := range strings.Split(got, "|") {
		if !strings.HasPrefix(seg, "/usr/local/bin/tool-") {
			t.Errorf("出现半截路径段 %q", seg)
		}
	}
}

// padLine 生成一行 60 字符路径的进程表记录:pid=hop, ppid=hop+1。
func padLine(hop, ppid int) string {
	path := "/usr/local/bin/tool-" + strings.Repeat("x", 39)
	line := strings.Repeat(" ", 3) + strconv.Itoa(hop) + strings.Repeat(" ", 5) + strconv.Itoa(ppid) + " " + path + "\n"
	return line
}

// parsePsLine 边界:表头、空行、前导空白。
func TestParsePsLine(t *testing.T) {
	if _, _, _, ok := parsePsLine("  PID PPID COMMAND"); ok {
		t.Error("表头行不应解析成功")
	}
	if _, _, _, ok := parsePsLine(""); ok {
		t.Error("空行不应解析成功")
	}
	pid, ppid, path, ok := parsePsLine("   42    7 /Applications/Qoder IDE.app/Contents/MacOS/Qoder")
	if !ok || pid != 42 || ppid != 7 || path != "/Applications/Qoder IDE.app/Contents/MacOS/Qoder" {
		t.Errorf("解析结果 = (%d,%d,%q,%v)", pid, ppid, path, ok)
	}
}

// ps 不可用时降级为空串(仅 darwin 路径)。
func TestProcessAncestryPsFailure(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("仅 darwin 走 ps 路径")
	}
	orig := psTable
	defer func() { psTable = orig }()
	psTable = func() (string, error) { return "", errors.New("ps unavailable") }
	if got := processAncestry(); got != "" {
		t.Errorf("ps 失败时应返回空串,得到 %q", got)
	}
}

func TestJoinAncestry(t *testing.T) {
	if got := joinAncestry(nil); got != "" {
		t.Errorf("空链应为空串,得到 %q", got)
	}
	if got := joinAncestry([]string{"/a", "/b"}); got != "/a|/b" {
		t.Errorf("joinAncestry = %q", got)
	}
}

// matchQoderCLI:只看祖先跳;自身恰好叫 qodercli 不自判;
// basename 精确匹配,与 QoderWake 的 qodercli-wake 区分。
func TestMatchQoderCLI(t *testing.T) {
	// 实测链形:CLI 由 qodercli 从 Warp 内发起
	chain := []string{
		"/bin/zsh",
		"/Users/u/.local/bin/qodercli",
		"-zsh",
		"/Applications/Warp.app/Contents/MacOS/stable",
	}
	if !matchQoderCLI(chain) {
		t.Error("祖先链含 qodercli 应命中")
	}
	// 自身就是 qodercli(假设它接入了 clitrack):不得自判
	if matchQoderCLI([]string{"/Users/u/.local/bin/qodercli"}) {
		t.Error("自身跳不应参与匹配")
	}
	// QoderWake 链含 qodercli-wake,不得误判
	wake := []string{
		"/bin/zsh",
		"/Users/u/.qoderwake/qodercli/qodercli-wake",
		"/Users/u/.qoderwake/qoderwake",
	}
	if matchQoderCLI(wake) {
		t.Error("qodercli-wake 不应被误判为 qodercli")
	}
	if matchQoderCLI(nil) {
		t.Error("空链不应命中")
	}
}
