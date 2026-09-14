// Package clitrack 在 aem 核心 SDK 之上,为任意 Go CLI 提供零侵入的使用埋点。
//
// 设计理念:CLI 入口只需一行 clitrack.New(cfg).Run(...),剩下全部自动完成——
// 从 os.Args 采集原始入参、自动计时、自动推导命令路径和退出码、异步上报。
//
// 埋点尽力而为,数据可丢,但绝不阻塞 CLI 退出(见 FlushTimeout)。接入方负责
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
//	  sid       → $TERM_SESSION_ID  终端会话 ID(自动采集,NoAutomaticDimensions 可关)
//	  ext.language → $LANG          终端 locale(自动采集,NoAutomaticDimensions 可关)
//
//	Event 维度(每次命令执行打一条,type = "event"):
//	  p1  → "cli.exec"     AEM 自定义事件 ID(默认值,可用 Config.EventID 覆盖)
//	  p4  → "SYS"          AEM 事件类型:系统事件(固定值)
//	  c1  → command        filepath.Base(os.Args[0])(CLI 二进制名,如 "aem")
//	  c2  → command_line   os.Args[1:] 拼接(完整参数);Config.NoCommandLine 可关
//	  c3  → exit_code      退出码
//	  c4  → duration_ms    执行耗时(毫秒)
//	  c5  → error_message  错误摘要,截断 200 字符
//	  c6  → shell_type     Shell 类型(zsh/bash 等);NoAutomaticDimensions 可关
//	  c7  → cwd            当前工作目录;Config.NoCwd 可关
//	  c8  → output         CLI stdout 输出摘要;默认不采,Config.CaptureOutput 显式开启
//	  c9/c10/ext → 自定义   由 Config.ExtraFields 钩子返回
//
// Config.NoAutomaticDimensions 会关闭设备、操作系统、时区、随机会话、终端会话、
// locale 和 Shell 等自动维度,只保留接入方显式配置的公共维度和事件字段。本包不读取
// 产品级退出环境变量;接入应用应在创建 Tracker 前执行自己的退出策略。PID 为空时
// Tracker 自动降级为 no-op,命令仍正常执行。
package clitrack
