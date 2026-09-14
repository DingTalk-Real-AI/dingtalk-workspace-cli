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
	// NoAutomaticDimensions 只保留接入方显式配置的公共维度，并关闭
	// device_id、os、os_version、timezone_offset、pv_id、sdk_version、sid、
	// ext.language 与 c6 Shell 自动采集。
	NoAutomaticDimensions bool

	// —— 扩展钩子 ——
	// ExtraFields 返回的字段会合并进事件,用于补充 c9/c10/ext 等自定义维度。
	// 不要覆盖 c1~c8 的约定语义,否则破坏跨 CLI 聚合。空值字段会被忽略。
	ExtraFields func() map[string]string
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
	noAutomaticDimensions bool
	extraFields           func() map[string]string
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

	return &Tracker{
		inner:                 aem.NewTracker(aemCfg),
		eventID:               eventID,
		captureOutput:         cfg.CaptureOutput,
		outputMaxLen:          outputMaxLen,
		flushTimeout:          flushTimeout,
		noCommandLine:         cfg.NoCommandLine,
		noCwd:                 cfg.NoCwd,
		noAutomaticDimensions: cfg.NoAutomaticDimensions,
		extraFields:           cfg.ExtraFields,
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
	_ = t.inner.Track(aem.Event{
		Type:   "event",
		Fields: t.buildFields(exitCode, duration, errMsg, output),
	})
}

// buildFields 按字段约定组装一次命令执行的事件字段(纯函数,便于测试)。
func (t *Tracker) buildFields(exitCode int, duration time.Duration, errMsg, output string) map[string]string {
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
			if v != "" {
				fields[k] = v
			}
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
