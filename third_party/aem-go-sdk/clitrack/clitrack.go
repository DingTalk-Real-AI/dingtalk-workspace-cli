package clitrack

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"gitlab.alibaba-inc.com/aes/aem-go-sdk/aem"
)

// 默认值。
const (
	defaultEventID   = "cli.exec" // p1:AEM 自定义事件 ID
	eventTypeSys     = "SYS"      // p4:AEM 事件类型,系统事件
	defaultOutputLen = 500        // c8 截断长度
	maxErrorLen      = 200        // c5 截断长度
)

// defaultFlushTimeout 是 CLI 退出前等待异步队列 flush 的最长时间。
// 到点未发完就放弃上报直接退出——宁可丢这条埋点,也不让用户等。
const defaultFlushTimeout = 300 * time.Millisecond

// attrTimeout 是上报时等待血缘探测结果的最长时间。实测一次 `ps` 全表
// 查询约 50ms;等待上限取 100ms 留余量,到点未就绪则本次事件缺 p3。
// (env 探测是同步的,不受此超时影响——p2 恒可靠。)
const attrTimeout = 100 * time.Millisecond

// Config 是 clitrack 接入配置。只有 PID 必填,其余都有合理默认值。
type Config struct {
	// —— 必填 ——
	PID string // AEM 项目 ID

	// —— 应用维度 ——
	App     string // CLI 名称,默认 "unknown"
	Env     string // 环境:prod/pre/daily,默认 prod
	Version string // CLI 版本号(建议用 ldflags 注入)

	// —— 用户维度(可选,接入方自己填,本包不读取任何凭据文件)——
	UID      string // 用户 ID,如工号
	Username string // 用户名
	UserType string // 账号类型,如 "14"

	// —— 行为 ——
	EventID  string // p1 事件 ID,默认 "cli.exec"
	Endpoint string // 上报域名,海外站点填 sg.mmstat.com;默认走 SDK 默认

	// CaptureOutput 控制是否捕获 stdout 到 c8。默认 false。
	// 捕获会用 os.Pipe 劫持 os.Stdout,可能干扰进度条/TTY 检测/颜色输出,
	// 仅在确认 CLI 输出适合采集时开启。
	CaptureOutput bool
	OutputMaxLen  int // c8 截断长度,默认 500;仅在 CaptureOutput 时生效

	// FlushTimeout 是退出前等待上报完成的最长时间,默认 300ms。
	FlushTimeout time.Duration

	// —— 字段级隐私开关(给接入开发者的编译期选项,默认采集)——
	NoCommandLine bool // 不采 c2 完整命令行(命令行常带敏感参数时设 true)
	NoCwd         bool // 不采 c7 工作目录
	// NoAutomaticDimensions is a DWS extension that suppresses device, OS,
	// session, locale, and shell dimensions. Attribution is controlled separately.
	NoAutomaticDimensions bool
	// NoAttribution is a DWS extension: do not probe or publish p2/p3.
	NoAttribution bool

	// —— 扩展钩子 ——
	// ExtraFields 返回的字段会合并进事件,用于补充 c9/c10/ext 等自定义维度。
	// 协议保留键(p1~p4、c1~c8)即使返回也不会被写入——语义固定,覆盖会破坏
	// 跨 CLI 聚合。空值字段会被忽略。
	ExtraFields func() map[string]string
}

// attributionResult 是执行环境归因的两路独立探测结果(不做融合)。
type attributionResult struct {
	executor string // p2:env 匹配到的执行环境,恒有值(无命中为 "none")
	ancestry string // p3:进程血缘完整链,空表示采集失败/超时/平台不支持
}

// Tracker 是埋点实例,通过 New 创建,通过 Run 执行 CLI 并自动上报。
type Tracker struct {
	inner *aem.Tracker

	eventID               string
	captureOutput         bool
	outputMaxLen          int
	flushTimeout          time.Duration
	noCommandLine         bool
	noCwd                 bool
	extraFields           func() map[string]string
	noAutomaticDimensions bool
	noAttribution         bool

	executor   string              // env 探测结果(微秒级,在 New 同步完成)
	ancestryCh chan ancestrySignal // 血缘探测结果(一次 ps,异步)
}

// ancestrySignal 是血缘探测的一次结果。
type ancestrySignal struct {
	chain    string // p3 内容;空表示采集失败
	qodercli bool   // 链上命中 qodercli(其主判定是进程路径,见 matchQoderCLI)
}

// New 根据配置创建 Tracker。
//
// 如果 PID 为空,返回空实例:Run 仍可正常执行 CLI,只是不上报。这样接入方
// 在缺少 PID(如本地开发)时无需加任何判断,埋点自动降级为 no-op。
func New(cfg Config) *Tracker {
	if cfg.PID == "" {
		return &Tracker{}
	}

	env := cfg.Env
	if env == "" {
		env = "prod"
	}
	app := cfg.App
	if app == "" {
		app = "unknown"
	}
	eventID := cfg.EventID
	if eventID == "" {
		eventID = defaultEventID
	}
	outputMaxLen := cfg.OutputMaxLen
	if outputMaxLen <= 0 {
		outputMaxLen = defaultOutputLen
	}
	flushTimeout := cfg.FlushTimeout
	if flushTimeout <= 0 {
		flushTimeout = defaultFlushTimeout
	}

	aemCfg := aem.Config{
		"pid":      cfg.PID,
		"app_name": app,
		"env":      env,
		"version":  cfg.Version,
		"platform": "cli",
	}
	if cfg.NoAutomaticDimensions {
		aemCfg["disable_auto_dimensions"] = true
	}
	if cfg.Endpoint != "" {
		aemCfg["endpoint"] = cfg.Endpoint
	}
	if cfg.UID != "" {
		aemCfg["uid"] = cfg.UID
	}
	if cfg.Username != "" {
		aemCfg["username"] = cfg.Username
	}
	if cfg.UserType != "" {
		aemCfg["user_type"] = cfg.UserType
	}

	if !cfg.NoAutomaticDimensions {
		// sid:终端会话 ID,自动从环境变量采集(数据维度,非配置开关)。
		sid := os.Getenv("TERM_SESSION_ID")
		if sid == "" {
			sid = os.Getenv("TMUX_PANE")
		}
		if sid != "" {
			aemCfg["sid"] = sid
		}

		// ext.language:终端 locale,自动采集。
		lang := os.Getenv("LANG")
		if lang == "" {
			lang = os.Getenv("LC_ALL")
		}
		if lang != "" {
			aemCfg["ext"] = fmt.Sprintf(`{"language":%q}`, lang)
		}
	}

	var ancestryCh chan ancestrySignal
	var executor string
	if !cfg.NoAttribution {
		executor = detectExecutor()
		ancestryCh = make(chan ancestrySignal, 1)
		ps := psTable // 快照,避免 goroutine 并发读全局变量
		go func() {
			// 探测解析的是系统外部输入,必须兜底:未捕获的 panic 会崩掉整个
			// CLI 进程,击穿"埋点绝不影响 CLI"红线。panic 按采集失败处理。
			defer func() {
				if recover() != nil {
					ancestryCh <- ancestrySignal{}
				}
			}()
			paths := collectAncestry(ps)
			ancestryCh <- ancestrySignal{
				chain:    joinAncestry(paths),
				qodercli: matchQoderCLI(paths),
			}
		}()

	}
	return &Tracker{
		inner:                 aem.NewTracker(aemCfg),
		eventID:               eventID,
		captureOutput:         cfg.CaptureOutput,
		outputMaxLen:          outputMaxLen,
		flushTimeout:          flushTimeout,
		noCommandLine:         cfg.NoCommandLine,
		noCwd:                 cfg.NoCwd,
		extraFields:           cfg.ExtraFields,
		executor:              executor,
		ancestryCh:            ancestryCh,
		noAutomaticDimensions: cfg.NoAutomaticDimensions,
		noAttribution:         cfg.NoAttribution,
	}
}

// Run 执行 CLI 主函数并自动上报埋点。
//
// 自动完成:计时、从 os.Args 采集入参、推导退出码、(可选)捕获 stdout、
// 错误输出到 stderr、best-effort flush(带超时,不阻塞退出)。
//
// execute  CLI 入口,返回 error;cobra 直接传 rootCmd.Execute。
// exitCode 把 error 映射为退出码;传 nil 用默认映射(nil→0,其余→1)。
//
// 与现状一致:退出码非 0 时调用 os.Exit;为 0 时正常 return,不调 os.Exit。
func (t *Tracker) Run(execute func() error, exitCode func(error) int) {
	if exitCode == nil {
		exitCode = defaultExitCode
	}

	start := time.Now()

	var output string
	var err error
	if t.captureOutput {
		output, err = captureStdout(execute)
	} else {
		err = execute()
	}

	code := 0
	var errStr string
	if err != nil {
		code = exitCode(err)
		errStr = err.Error()
		if errStr != "" {
			fmt.Fprintln(os.Stderr, errStr)
		}
	}

	t.trackExec(code, time.Since(start), errStr, output)
	t.close()

	if code != 0 {
		os.Exit(code)
	}
}

// trackExec 上报一次命令执行事件。
func (t *Tracker) trackExec(exitCode int, duration time.Duration, errMsg, output string) {
	if t.inner == nil {
		return
	}
	attr := t.takeAttribution()
	_ = t.inner.Track(aem.Event{
		Type:   "event",
		Fields: t.buildFields(exitCode, duration, errMsg, output, attr),
	})
}

// takeAttribution 组装归因结果:executor 是同步结果恒可靠;
// 血缘超过 attrTimeout 未就绪则本次缺 p3(并失去 Qoder CLI 回补)。
// 血缘命中 qodercli 时 executor 回补为 Qoder CLI——对照表钦定
// Qoder CLI 的主判定是进程路径,这是"不做融合"原则的唯一例外。
// no-op 实例(ancestryCh 为 nil)只有 executor。
func (t *Tracker) takeAttribution() attributionResult {
	if t.noAttribution {
		return attributionResult{}
	}
	r := attributionResult{executor: t.executor}
	if r.executor == "" {
		r.executor = executorNone
	}
	if t.ancestryCh == nil {
		return r
	}
	var sig ancestrySignal
	select {
	case sig = <-t.ancestryCh:
	case <-time.After(attrTimeout):
		return r
	}
	r.ancestry = sig.chain
	if sig.qodercli {
		r.executor = executorQoderCLI
	}
	return r
}

// reservedFieldKeys 是协议保留键,ExtraFields 不得覆盖。
// 覆盖会破坏跨 CLI 聚合的字段语义(与 c1~c8 红线同理,扩展到 p1~p4)。
var reservedFieldKeys = map[string]bool{
	"p1": true, "p2": true, "p3": true, "p4": true,
	"c1": true, "c2": true, "c3": true, "c4": true,
	"c5": true, "c6": true, "c7": true, "c8": true,
}

// buildFields 按字段约定组装一次命令执行的事件字段(纯函数,便于测试)。
func (t *Tracker) buildFields(exitCode int, duration time.Duration, errMsg, output string, attr attributionResult) map[string]string {
	fields := map[string]string{
		"p1": t.eventID,
		"p4": eventTypeSys,
		"c1": command(),
		"c3": strconv.Itoa(exitCode),
		"c4": strconv.FormatInt(duration.Milliseconds(), 10),
	}
	if !t.noAutomaticDimensions {
		fields["c6"] = shellType()
	}
	if !t.noAttribution && attr.executor != "" {
		fields["p2"] = attr.executor
	}
	if !t.noAttribution && attr.ancestry != "" {
		fields["p3"] = attr.ancestry
	}
	if !t.noCommandLine {
		fields["c2"] = commandLine()
	}
	if !t.noCwd {
		fields["c7"] = cwd()
	}
	if errMsg != "" {
		fields["c5"] = truncate(errMsg, maxErrorLen)
	}
	if output != "" {
		fields["c8"] = truncate(output, t.outputMaxLen)
	}
	if t.extraFields != nil {
		for k, v := range t.extraFields() {
			if v == "" || reservedFieldKeys[k] {
				continue
			}
			fields[k] = v
		}
	}
	return fields
}

// close 关闭内部 Tracker,best-effort flush:最多等 flushTimeout,超时即放弃。
func (t *Tracker) close() {
	if t.inner == nil {
		return
	}
	done := make(chan struct{})
	go func() {
		_ = t.inner.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(t.flushTimeout):
	}
}

// defaultExitCode 是 exitCode 参数为 nil 时的默认映射。
func defaultExitCode(err error) int {
	if err == nil {
		return 0
	}
	return 1
}
