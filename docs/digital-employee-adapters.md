# 数字员工本地 Agent Adapter

异常 lease 退出会留下私有 `lease-guard.json` 隔离标记。文件锁释放不代表宿主已释放；只有原 `runtimeInstanceId` 的停止确认才能清除标记。未知宿主不可按 PID、超时或空实例响应接管。宿主崩溃后无法取得原实例确认时保持 blocked，需要先核验旧进程全部退出后进行人工恢复；本期不提供强制接管。

## 目标与边界

本地接入复用受管登录，与外部 `auth exchange` 和主管 `manage login` 共享身份校验能力，不改变数字员工创建/管理/执行的 MCP 协议或机器人 Stream 收发协议。

- 创建/管理数字员工与本地接入独立；connect 不创建、修改或发布数字员工。
- 支持 Qoder、QoderWork、WorkBuddy、Claude Code、CodeBuddy、Codex、Gemini、OpenCode、custom 和 DSH。
- 普通 Agent 复用 dev connect 的调用、模型、目录、会话及权限实现；传输改为员工 Event Consumer 上行、员工 Profile 引用回复下行。第一阶段仅文本。
- DWS 统一管理本机 binding 和运行期望；DSH 实际管理自己的 Event Consumer、Agent 和回合，提供员工级释放确认。兼容读取旧 DSH binding（revision 0）。
- OpenClaw、Hermes、卡片、多媒体、知识库及远程审计扩展不属于本次数字员工新链路。

## 命令

```bash
dws dingtalk-tag manage login --agent-uuid <agentUuid>
dws dingtalk-tag connect --agent-uuid <agentUuid> --channel dsh
dws dingtalk-tag connect --agent-uuid <agentUuid> --channel codex --agent-workdir <directory> --daemon --alwayson
dws dingtalk-tag connect status --agent-uuid <agentUuid> --format json
dws dingtalk-tag connect list --format json
dws dingtalk-tag connect stop --agent-uuid <agentUuid> --format json
dws dingtalk-tag connect restart --agent-uuid <agentUuid> --format json
dws dingtalk-tag connect unbind --agent-uuid <agentUuid> --dry-run
# 确认解绑成功后再连接目标 Agent
dws dingtalk-tag connect --agent-uuid <agentUuid> --channel qoder --daemon --alwayson --dry-run
```

真实 connect 需汇总确认；可先用 `--dry-run`。默认前台，daemon 仅在 IPC 订阅完成、上游连接已观测为 connected/idle 且 Agent 初始化完成后报告运行成功。初始化成功不代表模型授权或真实消息已经验收。

`event consume` 的 `[event] ready` 保留原有 IPC 握手语义；新增 stderr `[event] transport <JSON>` 单独报告上游状态。运行器据此更新 `transportReady`，断线重连期间为 false，`executorReady` 单独反映 Agent 初始化结果。`event status` 与 HelloAck 使用真实 Source 状态快照，包含重连次数和最近事件/重连时间。业务消息是否送达仍需核对消息计数与实际回复。

普通本地 Agent 启动 `event consume` 时不显式传入 `--stream-source-id`，沿用 Event 的来源规则：优先使用 `DWS_STREAM_SOURCE_ID`，未设置或为空时使用 edition 默认值（开源版为 `open`）。宿主环境仍设置 `DWS_STREAM_SOURCE_ID=digital_employee` 时，仍使用该来源。升级后需重启员工 worker 才能生效；来源变化会使用不同的 Event Bus/订阅。这里只取消调用处的显式参数，服务端请求仍携带解析后的 `sourceId`。

旧 Bus 没有 `source_observed`/`observed` 字段，不能视为已验证连接。原生运行器返回 `event_bus_upgrade_required`，需升级并正常停止该员工的旧 Bus 后重试，不会自动停止共享 Bus。DSH 状态同时核对宿主报告与同配置目录、同员工身份的单聊 Bus；缺失、歧义或旧 Bus 返回 `transportReady=false`，并提示 `transport_not_verified`。保留宿主的 `runtimeState`，不把进程运行状态当作业务消息送达证明。

未发布的 `connection` 组已收拢到 `connect`。`connect` 是同时拥有业务执行和子命令的显式 hybrid；CLI、Help 与 Schema 均保留裸 connect。Schema 精确路径优先返回父工具自身，子工具通过产品导航或各自精确路径查询。机器人 `dev connect` 路径不变。

未知的 connect 子命令仅提示命令不存在及 `connect --help`，不根据名称相似度推荐其他操作。发布详情请求失败保留原始错误原因和恢复建议；查询成功且数据为空时才提示尚未发布。

## 统一生命周期（本次需求）

- `stop` 保存 stopped 期望，停止该员工，保留 binding 与 Profile；`restart` 复用保存的 Profile，不向主管重新换票。
- `unbind` 先停止并确认释放，随后写入 unbound 墓碑；保留 Profile、Token、去重记录、会话及审计。
- 更换 Agent 或设备时先完成 `connect unbind`，再执行 `connect`；新连接保存新的服务端 ID 和 bindingRevision 后才启动目标 Adapter。解绑失败不得启动新实例；新绑定提交后启动失败用 restart 恢复。
- 状态区分 `bindingState`（bound/unbound/unbinding）、`desiredState`（running/stopped）、`runtimeState`，并返回绑定版本、实例、transportReady、executorReady、observedAt 和阻塞原因。旧 `status` 保留兼容。
- DSH 通过私有本机 IPC 处理 prepare/start/status/stop/release；宿主失联或停止超时为 unknown/未释放，不强制换绑。
- DSH 持有与 DWS 原生 worker 相同的 Profile 运行锁，最后才释放。两个宿主或跨 Adapter 并发不能同时取得锁。此保证仅限同机、同配置目录、受管理的运行入口，不是跨机器在线状态服务。
- 回复带 bindingRevision；旧版本不能在换绑提交后继续调用 Channel 下行。
- `manage login` 只登录数字员工并保存 Profile，不创建 binding、不操作 DSH。DSH 首次注册后若宿主尚未运行，返回 restartRequired；宿主已运行则员工级启动，不重启整个宿主。
- 升级旧版 DSH 时先正常停止旧宿主，再启用新版。旧宿主没有释放协议，不能凭心跳或 PID 猜测已停；不提供强制覆盖入口。

## 身份、状态与恢复

connect 使用主管取得一次性授权码，严格使用返回的 Client ID 换票，按发布详情校验员工身份后保存独立 Profile，不切换主管 Current。普通 Agent 的进程、会话及处理记录按员工 Profile 摘要和 Adapter 隔离，不用共享 OAuth Client ID 隔离员工。

默认仅主管能触发；其他用户通过精确 userId 白名单开放，并在员工上下文解析开放 ID。群消息同时检查群和发送人。不同 Adapter 的已有绑定返回冲突，不隐式替换 DSH。

重启使用保存的员工 Profile，不重新向主管换票。显式 connect 完成新的授权后才开始新的重试预算。订阅错误遵守已知不可重试 0 次、可重试 2 次、未知 1 次的预算，并遵守服务端冷却时间；terminal hold 不自动解除。

每会话排队；已接收消息按会话和消息 ID 去重，稳定幂等键用于下行。发送结果 unknown 或处理途中崩溃均标为 needs_review，不盲目重发、不重复调用 Agent。本地无正文审计可靠落盘失败后停止接收新任务。没有服务端业务 ACK/replay/cursor 时不承诺 exactly-once 或断线不丢消息。

## 原消息状态反馈（普通本地 Agent）

普通本地 Agent 使用员工自己的 Profile，在触发消息上添加文字表情，帮助用户判断消息是否进入处理流程。DSH 的回合由宿主管理，不使用这里的反馈实现；机器人 `dev connect` 也保留原链路。

| 事实 | 期望标签 |
|---|---|
| 消息通过白名单、去重和容量检查，已保存接收记录，且同会话仍有未结束任务 | 排队中 |
| 会话空闲，新消息已接收并即将执行 | 思考中 |
| 开始调用本地 Agent | 思考中 |
| 回复接口返回明确成功结果及消息 ID，且本地记录已保存 | 已完成 |
| Agent 正常结束，未生成回复文字 | 收到，仅更新原消息表情 |
| Agent 调用失败 | 处理失败，并尝试发送不含底层错误的文字提示 |
| 当前执行收到取消信号 | 已取消 |
| 正文发送失败或回执结果未知 | 发送待确认 |
| 进程中断、账本收尾失败，或恢复时无法确定执行结果 | 已中断，待核查 |

“已完成”沿用 qoderwake 的展示文案，表示本轮处理结束且必要的文字回复已获发送确认，不表示对方已读或业务目标已通过验收。任务记录分别保存 `execution`（Agent 调用结果）和 `delivery`（发送确认），不从模型回复推断业务验收。原有 `status=delivered` 保持兼容；新增 `delivery=accepted` 明确其显示语义。失败提示发送成功仍显示“处理失败”。

表情更新在独立协程中进行，每条消息串行提交，合并尚未发送的中间状态。因此快速回复可能直接显示“已完成”，不会为了显示所有阶段延迟回复。撤旧、贴新不是原子操作；只撤除本员工在该消息上记录的表情。不同员工使用独立 Profile 和账本，同员工并发 worker 继续由运行锁拒绝。

状态定义通过现有 `create-text-emotion` 获取真实 ID，按员工持久化复用。私有 `feedback/` 记录期望标签、已确认的应用标签、尝试添加的表情 ID 以及更新结果；接口成功不等于客户端已经显示，当前没有额外的客户端读回证明。每次更新最多使用 5 秒请求预算，退出时另给一次最多 5 秒的清理机会。权限不足、接口拒绝或网络异常只记录 `message_feedback_failed`，不输出原始错误，不阻塞 Agent，不重跑业务或重发正文。进程强制退出时仍可能残留表情；下次启动仅对已有反馈记录尝试收敛，失败留待后续启动核查，不无限重试。

本次实现覆盖文本问答、正常结束但无文字时的“收到”回应、错误提示和中断恢复。无文字成功单独记录为 `completed_without_reply`、`execution=success`、`delivery=not_required`，表情期望与实际应用仍分别记录；失败或未知结果不能因正文为空而转换为“收到”。Codex 必须收到明确的 `completed` 通知且没有协议错误，断流、失败和取消保留异常事实；机器人原有空正文契约保持不变。业务验收证据、模型自由选择其他表情、审批等待标签、跨机器的多 Waker 聚合及流式状态卡片仍需独立设计。当前自动化测试验证本地时序与协议参数，真实渠道已验证普通问答最终显示“已完成”，正常无正文最终只贴“收到”，以及此前的失败提示。连接 readiness 只验证协议初始化，不代表所选模型已获账号授权；遇到模型不支持时，应使用该账号可用的模型并通过 `--agent-model` 显式指定，不能把反复重试当作修复。

正文仅通过受限 stdin 进入 DWS 下行命令；Agent 自身协议按现有实现运行，custom 的问题仍作为末参传递。DWS 凭据不传给 Agent，但这不是操作系统级沙箱；Agent 自身访问权限由权限参数和宿主控制。

## 验收记录口径

必须分别记录数字员工链路与 dev connect 回归。真实消息往返、群白名单、双员工隔离、Agent 授权、后台恢复及 DSH 的实机结果不能用 mock 替代。构建、focused/race、Schema、完整 Go 测试分别记录；因主机执行策略、缺测试身份或发布审批未完成的项目标为 NOT VERIFIED。

回滚本地连接时先使用 connect stop，仅停止本员工；回滚代码使用 PR revert。回滚前停止新版宿主；旧二进制不应读取新绑定版本后继续启动。无需删除员工或 Profile，不自动合并或发布 tag。
