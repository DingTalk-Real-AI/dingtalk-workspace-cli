package clitrack

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

// New 在 PID 为空时应返回 no-op 实例:inner 为 nil,Run 仍能跑完 execute。
func TestNewNoPIDIsNoop(t *testing.T) {
	tr := New(Config{App: "x"})
	if tr.inner != nil {
		t.Fatal("PID 为空时 inner 应为 nil")
	}

	called := false
	tr.Run(func() error { called = true; return nil }, nil)
	if !called {
		t.Fatal("Run 应执行 execute,即使不上报")
	}
}

// New 应正确填充各项默认值。
func TestNewDefaults(t *testing.T) {
	tr := New(Config{PID: "p"})
	if tr.inner == nil {
		t.Fatal("PID 非空时应创建 inner")
	}
	if tr.eventID != defaultEventID {
		t.Errorf("eventID 默认值 = %q, 期望 %q", tr.eventID, defaultEventID)
	}
	if tr.outputMaxLen != defaultOutputLen {
		t.Errorf("outputMaxLen 默认值 = %d, 期望 %d", tr.outputMaxLen, defaultOutputLen)
	}
	if tr.flushTimeout != defaultFlushTimeout {
		t.Errorf("flushTimeout 默认值 = %v, 期望 %v", tr.flushTimeout, defaultFlushTimeout)
	}
	if tr.captureOutput {
		t.Error("captureOutput 默认应为 false")
	}
}

func TestDefaultExitCode(t *testing.T) {
	if got := defaultExitCode(nil); got != 0 {
		t.Errorf("defaultExitCode(nil) = %d, 期望 0", got)
	}
	if got := defaultExitCode(os.ErrNotExist); got != 1 {
		t.Errorf("defaultExitCode(err) = %d, 期望 1", got)
	}
}

func TestTruncate(t *testing.T) {
	cases := []struct {
		in     string
		maxLen int
		want   string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello world", 8, "hello..."},
		{"中文测试内容很长", 5, "中文..."},
		{"ab", 2, "ab"},
		{"abc", 2, "ab"}, // maxLen <= 3 时直接硬截断
	}
	for _, c := range cases {
		if got := truncate(c.in, c.maxLen); got != c.want {
			t.Errorf("truncate(%q, %d) = %q, 期望 %q", c.in, c.maxLen, got, c.want)
		}
	}
}

func TestCommandAndCommandLine(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()

	os.Args = []string{"/usr/local/bin/mycli", "login", "--env", "prod"}
	if got := command(); got != "mycli" {
		t.Errorf("command() = %q, 期望 %q", got, "mycli")
	}
	if got := commandLine(); got != "login --env prod" {
		t.Errorf("commandLine() = %q, 期望 %q", got, "login --env prod")
	}
}

func TestShellType(t *testing.T) {
	orig, had := os.LookupEnv("SHELL")
	defer func() {
		if had {
			os.Setenv("SHELL", orig)
		} else {
			os.Unsetenv("SHELL")
		}
	}()

	os.Setenv("SHELL", "/bin/zsh")
	if got := shellType(); got != "zsh" {
		t.Errorf("shellType() = %q, 期望 %q", got, "zsh")
	}
	os.Unsetenv("SHELL")
	if got := shellType(); got != "" {
		t.Errorf("无 SHELL 时 shellType() = %q, 期望空", got)
	}
}

// buildFields 应遵守字段约定:固定 p1/p4,采集 c1~c8,归因写入 p2/p3。
func TestBuildFieldsMapping(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"mycli", "do", "stuff"}

	tr := New(Config{PID: "p", EventID: "cli.exec"})
	attr := attributionResult{
		executor: executorQoderwork,
		ancestry: "/bin/zsh|/Applications/QoderWork.app/Contents/MacOS/QoderWork",
	}
	f := tr.buildFields(2, 1500*time.Millisecond, "boom", "some output", attr)

	want := map[string]string{
		"p1": "cli.exec",
		"p4": "SYS",
		"p2": executorQoderwork,
		"p3": "/bin/zsh|/Applications/QoderWork.app/Contents/MacOS/QoderWork",
		"c1": "mycli",
		"c2": "do stuff",
		"c3": "2",
		"c4": "1500",
		"c5": "boom",
		"c8": "some output",
	}
	for k, v := range want {
		if f[k] != v {
			t.Errorf("字段 %s = %q, 期望 %q", k, f[k], v)
		}
	}
	if f["c7"] == "" {
		t.Error("c7(cwd) 默认应被采集")
	}
}

// 无错误、无 output 时不应出现 c5/c8;p2 恒有值,血缘缺失时无 p3。
func TestBuildFieldsOmitsEmpty(t *testing.T) {
	tr := New(Config{PID: "p"})
	f := tr.buildFields(0, time.Second, "", "", attributionResult{executor: executorNone})
	if _, ok := f["c5"]; ok {
		t.Error("无错误时不应有 c5")
	}
	if _, ok := f["c8"]; ok {
		t.Error("无 output 时不应有 c8")
	}
	if f["p2"] != executorNone {
		t.Errorf("p2 应恒有值,无命中时为 none,得到 %q", f["p2"])
	}
	if _, ok := f["p3"]; ok {
		t.Error("血缘采集失败时不应有 p3")
	}
}

// 隐私开关应能关闭 c2/c7。
func TestBuildFieldsPrivacyToggles(t *testing.T) {
	tr := New(Config{PID: "p", NoCommandLine: true, NoCwd: true})
	f := tr.buildFields(0, time.Second, "", "", attributionResult{executor: executorNone})
	if _, ok := f["c2"]; ok {
		t.Error("NoCommandLine 时不应有 c2")
	}
	if _, ok := f["c7"]; ok {
		t.Error("NoCwd 时不应有 c7")
	}
}

// ExtraFields 钩子应合并自定义字段,且忽略空值。
func TestBuildFieldsExtraHook(t *testing.T) {
	tr := New(Config{PID: "p", ExtraFields: func() map[string]string {
		return map[string]string{"c9": "custom", "c10": ""}
	}})
	f := tr.buildFields(0, time.Second, "", "", attributionResult{executor: executorNone})
	if f["c9"] != "custom" {
		t.Errorf("c9 = %q, 期望 custom", f["c9"])
	}
	if _, ok := f["c10"]; ok {
		t.Error("ExtraFields 的空值字段应被忽略")
	}
}

// ExtraFields 不得覆盖协议保留键(p1~p4、c1~c8),自定义键照常写入。
func TestBuildFieldsReservedKeysProtected(t *testing.T) {
	tr := New(Config{PID: "p", EventID: "cli.exec", ExtraFields: func() map[string]string {
		return map[string]string{"p1": "hijack", "p2": "hijack", "c1": "hijack", "c9": "ok"}
	}})
	f := tr.buildFields(0, time.Second, "", "", attributionResult{executor: executorNone})
	if f["p1"] != "cli.exec" {
		t.Errorf("p1 被 ExtraFields 覆盖为 %q", f["p1"])
	}
	if f["p2"] != executorNone {
		t.Errorf("p2 被 ExtraFields 覆盖为 %q", f["p2"])
	}
	if f["c1"] == "hijack" {
		t.Error("c1 不应被 ExtraFields 覆盖")
	}
	if f["c9"] != "ok" {
		t.Errorf("非保留键 c9 应正常写入,得到 %q", f["c9"])
	}
}

// 血缘探测解析的是系统外部输入:即使发生 panic 也必须被兜住,
// 不能崩掉 CLI 进程,也不能影响事件其余字段。
func TestAncestryPanicIsContained(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("仅 darwin 可注入 ps panic")
	}
	clearAttributionVars(t)
	t.Setenv("OPENCODE", "1")

	orig := psTable
	defer func() { psTable = orig }()
	psTable = func() (string, error) { panic("malformed system output") }

	tr := New(Config{PID: "p"})
	got := tr.takeAttribution() // 若无 recover,此处直接崩溃
	if got.ancestry != "" {
		t.Errorf("panic 时 ancestry 应为空,得到 %q", got.ancestry)
	}
	if got.executor != executorOpencode {
		t.Errorf("panic 不应影响 env 探测结果,得到 %q", got.executor)
	}
}

// 血缘回补:链上命中 qodercli 时,executor 回补为 Qoder CLI,
// 覆盖 env 结论(对照表钦定其主判定为进程路径)。
func TestTakeAttributionQoderCLIBackfill(t *testing.T) {
	ch := make(chan ancestrySignal, 1)
	tr := &Tracker{executor: executorNone, ancestryCh: ch}
	ch <- ancestrySignal{
		chain:    "/bin/zsh|/Users/u/.local/bin/qodercli|-zsh",
		qodercli: true,
	}
	got := tr.takeAttribution()
	if got.executor != executorQoderCLI {
		t.Errorf("executor = %q, 期望回补为 Qoder CLI", got.executor)
	}
	if got.ancestry == "" {
		t.Error("ancestry 应同时写入")
	}

	// 即使 env 已有结论(如 CI),路径主判定仍覆盖
	ch2 := make(chan ancestrySignal, 1)
	tr2 := &Tracker{executor: executorCI, ancestryCh: ch2}
	ch2 <- ancestrySignal{chain: "x", qodercli: true}
	if got := tr2.takeAttribution(); got.executor != executorQoderCLI {
		t.Errorf("env=CI 时也应被回补覆盖,得到 %q", got.executor)
	}

	// 未命中时不干扰 env 结论
	ch3 := make(chan ancestrySignal, 1)
	tr3 := &Tracker{executor: executorCI, ancestryCh: ch3}
	ch3 <- ancestrySignal{chain: "x", qodercli: false}
	if got := tr3.takeAttribution(); got.executor != executorCI {
		t.Errorf("未命中时不应改变 env 结论,得到 %q", got.executor)
	}
}

// env 探测同步恒可靠;血缘未就绪时按 attrTimeout 降级,本次缺 p3。
func TestTakeAttributionTimeoutDegrades(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("仅 darwin 可注入 ps 延迟")
	}
	clearAttributionVars(t)
	t.Setenv("OPENCODE", "1")

	orig := psTable
	defer func() { psTable = orig }()
	psTable = func() (string, error) { time.Sleep(300 * time.Millisecond); return "", nil }

	tr := New(Config{PID: "p"})
	start := time.Now()
	got := tr.takeAttribution()
	elapsed := time.Since(start)
	if elapsed < 40*time.Millisecond || elapsed > 200*time.Millisecond {
		t.Fatalf("takeAttribution 耗时 %v,应在 attrTimeout(%v)附近降级", elapsed, attrTimeout)
	}
	if got.executor != executorOpencode {
		t.Errorf("executor = %q, env 同步探测不应受 ps 超时影响", got.executor)
	}
	if got.ancestry != "" {
		t.Errorf("超时降级 ancestry 应为空,得到 %q", got.ancestry)
	}
}

// 核心保证:即使上报端极慢,close() 也必须在 flushTimeout 量级内返回,绝不阻塞退出。
func TestCloseBestEffortFlushDoesNotBlock(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(700 * time.Millisecond) // 远长于 flushTimeout
		w.WriteHeader(http.StatusOK)
	}))
	defer slow.Close()
	defer slow.CloseClientConnections() // LIFO:先切断 in-flight 请求,再 Close,避免拖慢测试

	tr := New(Config{
		PID:          "p",
		Endpoint:     slow.URL, // http:// 前缀,sender 按原样使用
		FlushTimeout: 50 * time.Millisecond,
	})
	tr.trackExec(0, time.Second, "", "") // 入队一条,worker 会卡在慢服务器上

	start := time.Now()
	tr.close()
	elapsed := time.Since(start)

	if elapsed > 300*time.Millisecond {
		t.Fatalf("close() 耗时 %v,应在 flushTimeout(50ms)量级内返回,不能被慢上报阻塞", elapsed)
	}
}

// no-op 实例的 close() 应立即返回。
func TestCloseNoopReturns(t *testing.T) {
	tr := New(Config{})
	done := make(chan struct{})
	go func() { tr.close(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("no-op close() 未及时返回")
	}
}

func TestEndpointSanity(t *testing.T) {
	// 防御:Endpoint 传入本地地址时 New 不应 panic。
	tr := New(Config{PID: "p", Endpoint: "127.0.0.1:9999"})
	if tr.inner == nil {
		t.Fatal("应创建 inner")
	}
}

// 端到端:注入含 qodercli 的伪进程表,走完整 采集→上报→HTTP 链路,
// 抓取真实报文验证血缘回补后 p2=Qoder CLI、p3 含原始证据。
// 伪表以真实自身 pid 构造,保证爬虫真能爬到。
func TestRunReportsQoderCLIEndToEnd(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("仅 darwin 可注入 ps 表")
	}
	clearAttributionVars(t)

	orig := psTable
	defer func() { psTable = orig }()
	psTable = func() (string, error) {
		return fmt.Sprintf(""+
			"%[1]d 200 /bin/zsh\n"+
			"200 300 /Users/u/.local/bin/qodercli\n"+
			"300 1 /Applications/Warp.app/Contents/MacOS/stable\n", os.Getpid()), nil
	}

	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = []string{"mycli", "run"}

	New(Config{PID: "p", Endpoint: srv.URL}).Run(func() error { return nil }, nil)
	if body == "" {
		t.Fatal("未收到上报请求")
	}

	// 报文结构:JSON{gokey: encodeURIComponent(gokey)},gokey 尾部是
	// &msg=encodeURIComponent(事件字段串);两层解码后断言。
	var payload struct {
		Gokey string `json:"gokey"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("报文不是预期 JSON: %v", err)
	}
	gokey, err := url.QueryUnescape(payload.Gokey)
	if err != nil {
		t.Fatalf("gokey 解码失败: %v", err)
	}
	i := strings.Index(gokey, "msg=")
	if i < 0 {
		t.Fatalf("gokey 无 msg 段: %s", gokey)
	}
	msg, err := url.QueryUnescape(gokey[i+4:])
	if err != nil {
		t.Fatalf("msg 解码失败: %v", err)
	}

	fields := make(map[string]string)
	for _, kv := range strings.Split(msg, "&") {
		if j := strings.IndexByte(kv, '='); j > 0 {
			v, _ := url.QueryUnescape(kv[j+1:]) // 字段值还有一层单独编码
			fields[kv[:j]] = v
		}
	}
	if fields["p2"] != executorQoderCLI {
		t.Errorf("p2 = %q, 期望 %q(血缘回补)", fields["p2"], executorQoderCLI)
	}
	if !strings.Contains(fields["p3"], "/Users/u/.local/bin/qodercli") {
		t.Errorf("p3 应含 qodercli 原始证据,得到 %q", fields["p3"])
	}
	if fields["c1"] != "mycli" {
		t.Errorf("归因不应影响既有字段, c1 = %q", fields["c1"])
	}
}
