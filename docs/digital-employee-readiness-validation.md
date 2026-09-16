# 数字员工事件连接修复验证

本次只处理事件连接误报就绪、个人 Stream 控制帧处理与订阅查询兼容性；优先验证 Qoder、Codex、custom、DSH。未对其他 Agent 作完整实机验收。

## 缺陷与修复

1. Bus 的 HelloAck 和状态查询原先固定返回 `connected/inferred`，没有读取 Source 状态机。现在读取实际快照，并推送连接变化、保留最近事件/重连时间及重连次数。无观测能力的旧 Bus 或替身源保持未知。
2. 员工运行器原先收到 `[event] ready` 即认为传输就绪。该标记只代表 IPC 订阅完成；现在还要等观测到上游 connected/idle。Agent 初始化结果独立保存，重连时及时撤销传输就绪，停止时清除两项就绪状态。
3. DSH 宿主报告与员工身份对应的实时 Bus 状态交叉核验；不接受其他员工、歧义来源、无单聊 consumer 或旧 Bus 的连接推断。
4. 个人 Stream 原先把 SYSTEM ping/disconnect 当作业务事件并返回通用 ACK。现在按协议回显 ping 内容，确认 disconnect 后重连，系统帧不进入业务队列。
5. 订阅列表支持 `items` 与 `list`，时间戳支持字符串与整数；服务端声明有数据而缺少列表时返回错误，不再伪装为空结果。

这些缺陷均有本地回归证据；不能仅凭这些修复就断言已找到线上某条单聊未投递的唯一原因。

## 本地验证结果（2026-09-16，macOS arm64）

| Adapter | 实际可执行程序验证 | 隔离链路验证 | 真实钉钉往返 |
| --- | --- | --- | --- |
| Qoder 1.1.53 | 初始化及一次模型调用返回约定测试口令 | stream-json、错误处理、会话与员工就绪/重连回归通过 | 未验收 |
| Codex CLI 0.154.0 | app-server 初始化及一次模型调用返回约定测试口令 | app-server、错误处理、会话与员工就绪/重连回归通过 | 未验收 |
| custom | 实际进程 argv → stdout 收发通过 | IPC 等待、事件去重、回复确认、退出与断线恢复通过 | 未验收 |
| DSH | 专用本地宿主恢复后，使用修复版 DWS 完成实际回合 | 宿主本地运行、控制、路由、渲染、审计、会话、配置相关测试 35/35 通过；DWS 身份隔离/就绪判断通过 | 正常收发及 Bus 重启后收发均通过，各约 5 秒 |

DSH 使用当前本地插件的隔离测试，没有替换运行中的插件构建产物、修改员工绑定或发布插件。DSH 测试首次因临时目录下 Unix socket 路径过长失败，改用指向受控目录的短路径后通过。

事件模块全量 `go test -race ./internal/event/...` 通过；数字员工与上述 Adapter 的 focused/race 检查通过。另一次广域 helpers/auth/app 测试未全部通过，出现子进程被终止、macOS Keychain 超时和 app 全集超时；因此不宣称全仓测试通过。本次未接管原 PR 的 CI 修复或合并验收。

## 重现回归测试

在允许构建和执行的受控目录设置 TMPDIR/GOTMPDIR 后运行：

```bash
go test -race ./internal/event/... -count=1
go test -race ./internal/helpers \
  -run 'TestCrossPlatformCoverage(Subscription|PersonalSystem|Employee|DSHTransport)|TestEmployee|TestQoder|TestCodex|TestCustomChannel|TestDigitalEmployee' \
  -count=1
```

重点覆盖未连接、connecting、connected、reconnecting、idle、旧版未观测状态、IPC-only、系统 ping/disconnect、列表分页及时间戳类型。真实消息验收须使用升级后的员工 Bus，核对会话历史、Source/consumer 计数、Agent 回合与实际回复。


## 真实消息验收补充（2026-09-16）

验证代码为 `888d6cf42c2b61cd6e421d310198893d880ea9f4`。使用现有专用 DSH 测试员工与修复版 DWS，通过 DWS 用户消息发送接口发送唯一口令；没有改员工绑定、接收白名单或日常宿主配置。

| 场景 | 消息发送时间（UTC+8） | 宿主接收 / 启动 | 回复投递 / 完成 | 会话历史核验 |
| --- | --- | --- | --- | --- |
| 连接空闲约 9 分钟后首次发送 | 23:34:32 | 23:34:32.463 / 23:34:32.586 | 23:34:37.464 / 23:34:37.476 | 返回约定口令，约 5 秒，单次回复 |
| 仅测试 Bus 受控退出并自动恢复后发送 | 23:36:41 | 23:36:41.582 / 23:36:41.598 | 23:36:46.016 / 23:36:46.021 | 返回约定口令，约 5 秒，单次回复 |

两次发送状态均为 `SUCCESS`，实际回复由会话历史和宿主审计交叉确认；消费者没有丢弃记录。Bus 退出后，宿主自动重启 consumer 并建立新 Bus，原订阅继续使用。最终传输和执行器均就绪，Source 为已观测的 connected/idle。真实员工、会话、消息标识及原始日志只保留在本地。

原始测试的后续历史回读显示，消息最终约 237 秒后得到回复，故应描述为长延迟，不能断言永久丢失。上述两次修复版验证没有复现长延迟，但不足以确定原延迟的唯一原因。本次实机恢复测试覆盖 Bus 进程重启，不等同于网络半开连接或全部上游断线场景；SYSTEM ping/disconnect 和 Source 重连状态由隔离回归覆盖。Qoder、Codex、custom 的验证仍限于前述实际可执行程序及隔离链路，不能描述为四种 Agent 均已完成真实钉钉端到端验收。
