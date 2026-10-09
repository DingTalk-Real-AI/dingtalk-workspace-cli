# 将 Pi Coding Agent 接入钉钉机器人

`dws dev connect --channel pi` 原生使用 Pi 的 JSON 事件流，支持模型覆盖、
流式文本、按钉钉会话隔离记忆、连接重启后恢复和 `/new` 重置。
数字员工的本地 Adapter 共用同一实现，可通过 `dingtalk-tag connect --channel pi` 选择。

## 安装与配置

官方包已从 `@mariozechner/pi-coding-agent` 迁移至
`@earendil-works/pi-coding-agent`。安装当前官方包：

```bash
npm install -g @earendil-works/pi-coding-agent
pi --version
pi
```

在 Pi 中完成 `/login` 和 `/model` 配置，先确认 Pi 本身可以回答问题。
DWS 复用 Pi 的提供商配置；不会转换其他 Agent 的登录凭证。
当前适配已对旧包 `0.73.1` 和新包 `1.1.0` 执行官方 CLI 协议验收。

## 连接

先预览现有机器人应用的连接计划：

```bash
dws dev connect --channel pi --unified-app-id <unifiedAppId> --dry-run --format json
```

启动连接，并按需要指定 `provider/model`：

```bash
dws dev connect --channel pi --unified-app-id <unifiedAppId> --agent-model <provider/model> --agent-workdir <工作目录>
```

在钉钉中 @ 机器人后，DWS 读取 Pi 的助手文本增量，经已有 AI 卡片或
sessionWebhook 通道回复。启用并正确配置卡片模板时可显示流式文本；普通消息在完成后发送。
Pi 完成重试和工具调用并退出后，本轮才判定完成；不会把第一条 `agent_end` 当成最终成功。

`--agent-memory=false` 关闭会话落盘；默认按机器人及会话隔离文件。
会话存储在 DWS 配置目录的 `connect/<scope>/pi/`，不使用 Pi 的全局 `--continue`。
未提供 scope 时使用 `--no-session`，不创建配置目录中的会话文件。
`/new` 和 `/clear` 都会解除当前会话映射；旧会话文件保留，`/clear` 不声明物理删除。
Pi 调用失败或取消时解除本轮会话映射，下一轮从新会话开始；旧文件保留。
`--agent-timeout`、停止连接及守护进程生命周期继续由 DWS 管理；
默认无超时限制时，停止连接也会取消正在运行的 Pi 进程。
不同钉钉会话可并行调用 Pi，同会话仍按顺序执行和重置。
某会话挂起不会阻塞其他会话的回复或 `/new`、`/clear`；
同会话的控制指令仍按现有消息队列顺序处理，不抢占当前轮次。
关闭连接会取消全部运行中及等待中的 Pi 操作。

## 能力边界

- 默认使用 Pi 自身的内置工具设置；`--ask` 或 `--yolo=false` 会禁用工具，
  因为当前没有逐工具批准的钉钉交互桥接。
- 通道禁用 Pi 扩展发现，避免无宿主的交互 UI；不会加载扩展提供商。
  内置提供商与 `models.json` 自定义兼容端点仍可使用。
- 图片与文本文件通过 Pi 官方 `@file` 输入传入；直接音频、视频输入明确拒绝。
- `--agent-cmd` / `DWS_AGENT_CMD` 覆盖沿用一次性 custom 命令语义，
  会禁用 Pi 协议、模型和会话注入。选择原生 Pi 时不要同时设置覆盖命令。
- 不回显提供商错误正文或 stderr；失败时检查独立运行的 Pi 和模型配置。

## 本地验收

普通回归：

```bash
go test ./internal/helpers -run 'TestCrossPlatformCoveragePi|TestPi|TestEveryStreamBridgeAgentHasAttachmentDeliveryPath' -count=1
```

显式运行已安装的官方 Pi，对接本机 loopback 模型 fixture：

```bash
DWS_PI_TEST_BIN=<pi绝对路径> go test ./internal/helpers -run '^TestPiOfficialCLI$' -v -count=1
```

该测试覆盖真实 Pi 进程、流式输出、模型覆盖、会话隔离、跨重启续聊、
`/new`、禁用记忆、附件、内置 `read` 工具调用、提供商失败和超时恢复；模型回答由 fixture 生成。

使用现有 Pi 配置执行无工具的真实模型流式与记忆验收：

```bash
DWS_PI_LIVE=1 DWS_PI_TEST_BIN=<pi绝对路径> go test ./internal/helpers -run '^TestPiLiveModel$' -v -count=1
```

可用 `DWS_PI_LIVE_MODEL=<provider/model>` 覆盖真实验收模型。
上述测试不发送钉钉消息；钉钉送达需要单独核验实际机器人消息回复。

协议依据：[官方 JSON 事件流](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/json.md)、
[官方命令行](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/cli.md)。
