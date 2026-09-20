// Package clitrack 在 aem 核心 SDK 之上,为任意 Go CLI 提供零侵入的使用埋点。
//
// 设计理念:CLI 入口只需一行 clitrack.New(cfg).Run(...),剩下全部自动完成——
// 从 os.Args 采集原始入参、自动计时、自动推导命令路径和退出码、异步上报。
//
// 埋点尽力而为。默认最多等待 FlushTimeout；显式启用 NoFlushWait 的接入方
// 接受进程退出时最后一条事件可能丢失。接入方负责
// 向最终用户披露采集范围并提供符合其产品要求的退出机制。
//
// 最小接入示例(以 cobra CLI 为例):
//
//	func Execute() {
//	    clitrack.New(clitrack.Config{
//	        PID: "your-pid", App: "my-cli", Version: version,
//	    }).Run(rootCmd.Execute, nil)
//	}
//
// 框架无关:Run 只要求一个 func() error 入口,cobra / urfave-cli / 标准库 flag
// 都能套。exitCode 传 nil 时用默认映射(nil→0,其余→1)。
//
// AEM 字段映射约定(所有接入的 CLI 统一遵守,这是跨 CLI 聚合分析的基础):
//
//	Config 维度(初始化时设一次):
//	  pid       → Config.PID        AEM 项目 ID(必填)
//	  app_name  → Config.App        CLI 名称
//	  env       → Config.Env        环境 (prod/pre/daily),默认 prod
//	  version   → Config.Version    CLI 版本号
//	  uid       → Config.UID        用户 ID(可选,接入方自己填)
//	  username  → Config.Username   用户名(可选)
//	  user_type → Config.UserType   账号类型(可选)
//	  endpoint  → Config.Endpoint   上报域名(可选,海外站点填 sg.mmstat.com)
//	  sid       → $TERM_SESSION_ID  终端会话 ID(自动采集的数据维度)
//	  ext.language → $LANG          终端 locale(自动采集的数据维度)
//
//	Event 维度(每次命令执行打一条,type = "event"):
//	  p1  → "cli.exec"     AEM 自定义事件 ID(默认值,可用 Config.EventID 覆盖)
//	  p2  → executor       执行环境归因:匹配到的执行环境,枚举值(采用对照表
//	                       正式名原文):QoderWork / QwenWork / Claude Code /
//	                       Qoder IDE / Qoder / QoderWake / Qoder CLI /
//	                       OpenCode / Wukong / Codex / CI / none
//	                       (自动采集,默认开启,无开关)。其中 Qoder CLI 是
//	                       血缘回补:其 env 标记不可靠,主判定是进程路径,
//	                       链上命中 qodercli 时输出
//	  p3  → ancestry       执行环境归因:进程血缘完整链(每跳仅可执行文件
//	                       路径,"|" 连接,禁 argv;采集失败时不传)
//	  p4  → "SYS"          AEM 事件类型:系统事件(固定值)
//	  c1  → command        filepath.Base(os.Args[0])(CLI 二进制名,如 "aem")
//	  c2  → command_line   os.Args[1:] 拼接(完整参数);Config.NoCommandLine 可关
//	  c3  → exit_code      退出码
//	  c4  → duration_ms    执行耗时(毫秒)
//	  c5  → error_message  错误摘要,截断 200 字符
//	  c6  → shell_type     Shell 类型(zsh/bash 等)
//	  c7  → cwd            当前工作目录;Config.NoCwd 可关
//	  c8  → output         CLI stdout 输出摘要;默认不采,Config.CaptureOutput 显式开启
//	  c9/c10/ext → 自定义   由 Config.ExtraFields 钩子返回
//
//	协议保留键:p1~p4 与 c1~c8 语义固定,即使 ExtraFields 返回同名键也不会
//	写入。p2/p3 是执行环境归因的两路独立信号(不做融合),归因结论由分析侧
//	交叉得出,设计依据见 docs/clitrack-attribution-design.md。
//
// 关于环境变量:本包不引入任何「配置开关」类环境变量(如 DISABLED/PID 覆盖)。
// 埋点是内部工具的无感采集,接入成本只有一行代码;最终用户无需、也不应感知。
// 上面的 $TERM_SESSION_ID / $LANG 是「数据采集来源」而非配置开关,类比浏览器
// 读取 navigator.language,属于自动采集的维度。
package clitrack
