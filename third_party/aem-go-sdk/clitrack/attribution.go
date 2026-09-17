package clitrack

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// 执行环境归因(纯只读探测,默认开启,无关闭开关):
//   - p2:env 匹配到的执行环境,低基数枚举,见 detectExecutor
//   - p3:进程血缘完整链(每跳只含可执行文件路径,禁 argv、不含 pid),见 processAncestry
//
// 两路信号独立上报,不做融合判定;归因结论由分析侧交叉得出。唯一例外:
// Qoder CLI 的主判定是对照表钦定的进程路径(env 标记因共享不可靠),
// 故链上命中 qodercli 时 p2 回补为 Qoder CLI(见 matchQoderCLI)。
// 设计依据见 docs/clitrack-attribution-design.md。

// executor 枚举值。词表一经发布即冻结(跨 CLI 聚合的 key),新增宿主走追加。
// 值一律采用《环境变量特征 × 宿主软件对照表》的正式名原文,不做大小写/
// 连字符改写;非宿主类别只有 CI 与 none。
const (
	executorQoderwork  = "QoderWork"
	executorQwenwork   = "QwenWork" // bundle 为 QwenWorkCN.app,QoderWork 国内版
	executorClaudeCode = "Claude Code"
	executorQoderIDE   = "Qoder IDE"
	executorQoder      = "Qoder"     // Qoder.app 桌面端
	executorQoderwake  = "QoderWake" // daemon 形态,主进程直挂 pid 1
	executorQoderCLI   = "Qoder CLI" // env 判不出,由血缘回补(主判定是进程路径)
	executorOpencode   = "OpenCode"
	executorWukong     = "Wukong" // 钉钉悟空(Wukong.app)
	executorCodex      = "Codex"  // 内嵌于 ChatGPT.app 的 agent 二进制
	executorCI         = "CI"
	executorNone       = "none"
)

const (
	maxAncestryHops = 32  // 向上遍历跳数上限,防异常环路
	maxAncestryLen  = 500 // p3 总长上限(字符)
)

// detectExecutor 按环境变量匹配执行环境,按优先级先命中先得。
//
// 顺序不可乱,且判据均来自实测(见交接文档四批实测),几个关键约束:
//   - QODER_WORK_INTEGRATION_MODE 在 QoderWork 与 QwenWorkCN(国内版)中都注入,
//     命中后须用 QODERWORK_IS_CN 分流;
//   - QODER_SESSION_TYPE 在 Qoder(Qoder.app) 与 QoderWake 中都出现,已弃用;
//     QoderWake 靠 QODERWAKE_* 判定且必须先于 Qoder 检查;
//   - QODER_WORKER_RUNTIME_PATH / QW_QODER_WORKER_RUNTIME_PATH 是
//     `launchctl setenv` 写入的系统级泄漏变量(连 Codex 环境都携带),永不采用。
//
// 只判存在性与常量比对,绝不读取、返回任何变量的原始值(部分值含敏感信息)。
func detectExecutor() string {
	if os.Getenv("QODER_WORK_INTEGRATION_MODE") == "1" {
		if _, ok := os.LookupEnv("QODERWORK_IS_CN"); ok {
			return executorQwenwork
		}
		return executorQoderwork
	}
	if os.Getenv("CLAUDECODE") == "1" {
		return executorClaudeCode
	}
	if _, ok := os.LookupEnv("QODER_IDE"); ok {
		return executorQoderIDE
	}
	// 三个"前缀族"按前缀扫描而非枚举清单:变量族随宿主升级扩充,
	// 清单永远滞后(对照表 §8.5)。
	// 必须先于 QODER_PRODUCT_ID 检查(QODER_SESSION_TYPE 与 Qoder 共享)。
	if envHasAnyPrefix("QODERWAKE_") {
		return executorQoderwake
	}
	if _, ok := os.LookupEnv("QODER_PRODUCT_ID"); ok {
		return executorQoder
	}
	if _, ok := os.LookupEnv("OPENCODE"); ok {
		return executorOpencode
	}
	if envHasAnyPrefix("WUKONG_") {
		return executorWukong
	}
	if envHasAnyPrefix("CODEX_") {
		return executorCodex
	}
	if envAnyExists("CI", "GITHUB_ACTIONS", "GITLAB_CI", "JENKINS_URL") {
		return executorCI
	}
	return executorNone
}

// envAnyExists 任一变量存在(无论值)即返回 true。
func envAnyExists(keys ...string) bool {
	for _, k := range keys {
		if _, ok := os.LookupEnv(k); ok {
			return true
		}
	}
	return false
}

// envLeakedVars 是已证实的系统级泄漏变量,永不参与判定:
// 经 `launchctl setenv` 写入 macOS 全局会话环境,连非 Qoder 系的
// Codex 环境都携带(实测用 `launchctl getenv` 验证)。
// 排查手段:候选变量出现在无关进程中时,先 `launchctl getenv <名>` 验证。
var envLeakedVars = map[string]bool{
	"QODER_WORKER_RUNTIME_PATH":    true,
	"QW_QODER_WORKER_RUNTIME_PATH": true,
}

// envHasAnyPrefix 扫描全部环境变量,任一变量名命中给定前缀(排除泄漏
// 变量)即返回 true。用前缀而非清单:变量族会随宿主升级扩充。
func envHasAnyPrefix(prefixes ...string) bool {
	for _, kv := range os.Environ() {
		name := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			name = kv[:i]
		}
		if envLeakedVars[name] {
			continue
		}
		for _, p := range prefixes {
			if strings.HasPrefix(name, p) {
				return true
			}
		}
	}
	return false
}

// psTable 执行一次 `ps -axo pid=,ppid=,comm=` 拿全系统进程表。
// 变量形式便于单测注入伪输出。
//
// 只用 comm=(可执行文件路径,实测为完整路径);严禁换用 command=
// 或读命令行参数——那是 argv,可能带 --token 等敏感信息。
var psTable = func() (string, error) {
	out, err := exec.Command("ps", "-axo", "pid=,ppid=,comm=").Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// processAncestry 采集当前进程的完整血缘链:从自身沿 ppid 爬到 pid=1,
// 每跳只取可执行文件路径,"|" 连接,总长截断到 maxAncestryLen。
// 任何失败(平台不支持、进程退出、查询出错)都截断或返回空串,不报错。
func processAncestry() string {
	return joinAncestry(collectAncestry(psTable))
}

// collectAncestry 与 processAncestry 相同,但返回未拼接的路径切片,
// 供宿主路径匹配(如 matchQoderCLI)复用。ps 执行器显式传入,
// New() 在起 goroutine 前快照函数值,避免并发读全局变量。
func collectAncestry(ps func() (string, error)) []string {
	switch runtime.GOOS {
	case "darwin":
		out, err := ps()
		if err != nil {
			return nil
		}
		return ancestryPathsFromPsTable(out, os.Getpid())
	case "linux":
		return ancestryPathsLinux(os.Getpid())
	default:
		return nil
	}
}

// matchQoderCLI 判断血缘链上是否有 qodercli。Qoder CLI 的主判定是进程
// 路径(对照表钦定,env 标记因共享不可靠),这是 p2 能输出 Qoder CLI 的
// 唯一依据(血缘回补)。
//
// 两个防误判约束:跳过自身跳(防止恰好叫 qodercli 的 CLI 自判);
// basename 精确相等(与 QoderWake 的子进程 qodercli-wake 区分)。
func matchQoderCLI(paths []string) bool {
	for i := 1; i < len(paths); i++ {
		if filepath.Base(paths[i]) == "qodercli" {
			return true
		}
	}
	return false
}

// ancestryFromPsTable 在进程表中从 self 沿 ppid 爬链,返回拼接后的链。
func ancestryFromPsTable(table string, self int) string {
	return joinAncestry(ancestryPathsFromPsTable(table, self))
}

// ancestryPathsFromPsTable 在进程表中从 self 沿 ppid 爬链。
// 路径可能含空格(如 "Qoder IDE.app"),解析按"前两列数字 + 其余全部为路径"切分。
func ancestryPathsFromPsTable(table string, self int) []string {
	type entry struct {
		ppid int
		path string
	}
	procs := make(map[int]entry, 256)
	for _, line := range strings.Split(table, "\n") {
		pid, ppid, path, ok := parsePsLine(line)
		if ok {
			procs[pid] = entry{ppid, path}
		}
	}

	var paths []string
	pid := self
	for i := 0; i < maxAncestryHops; i++ {
		e, ok := procs[pid]
		if !ok {
			break // 链到此为止(父进程已退出或不在表中)
		}
		paths = append(paths, e.path)
		if e.ppid <= 1 {
			break // 到 pid=1 之下即止,不采集 launchd 本身
		}
		pid = e.ppid
	}
	return paths
}

// parsePsLine 解析 "  <pid>  <ppid>  <path...>" 一行。
// 不合法行返回 ok=false 跳过,不中断整体解析。
func parsePsLine(line string) (pid, ppid int, path string, ok bool) {
	s := strings.TrimLeft(line, " \t")
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 {
		return 0, 0, "", false
	}
	pid, _ = strconv.Atoi(s[:i])
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	j := i
	for j < len(s) && s[j] >= '0' && s[j] <= '9' {
		j++
	}
	if j == i {
		return 0, 0, "", false
	}
	ppid, _ = strconv.Atoi(s[i:j])
	for j < len(s) && (s[j] == ' ' || s[j] == '\t') {
		j++
	}
	path = strings.TrimSpace(s[j:])
	if path == "" {
		return 0, 0, "", false
	}
	return pid, ppid, path, true
}

// ancestryPathsLinux 从 /proc 爬链:stat 取 ppid,readlink exe 取可执行路径。
// 不读 /proc/<pid>/cmdline(那是参数)。
func ancestryPathsLinux(self int) []string {
	var paths []string
	pid := self
	for i := 0; i < maxAncestryHops; i++ {
		exe, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
		if err != nil {
			break // 截断:保留已采集部分
		}
		paths = append(paths, exe)
		ppid, err := linuxParent(pid)
		if err != nil || ppid <= 1 {
			break
		}
		pid = ppid
	}
	return paths
}

// linuxParent 从 /proc/<pid>/stat 解析 ppid。
// comm 字段格式为 "(可含空格与括号)",必须取最后一个 ')' 之后再分列。
func linuxParent(pid int) (int, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0, err
	}
	s := string(data)
	i := strings.LastIndexByte(s, ')')
	if i < 0 || i+2 >= len(s) {
		return 0, fmt.Errorf("clitrack: malformed stat for pid %d", pid)
	}
	fields := strings.Fields(s[i+1:])
	if len(fields) < 2 {
		return 0, fmt.Errorf("clitrack: malformed stat for pid %d", pid)
	}
	return strconv.Atoi(fields[1])
}

// joinAncestry 拼接并在 "|" 边界截断到上限(不留半截路径)。
func joinAncestry(paths []string) string {
	s := strings.Join(paths, "|")
	if len(s) <= maxAncestryLen {
		return s
	}
	cut := s[:maxAncestryLen]
	if i := strings.LastIndexByte(cut, '|'); i > 0 {
		cut = cut[:i]
	}
	return cut
}
