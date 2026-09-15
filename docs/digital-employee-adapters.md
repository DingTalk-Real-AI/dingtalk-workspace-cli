# 数字员工本地 Agent Adapter

异常 lease 退出会留下私有 `lease-guard.json` 隔离标记。文件锁释放不代表宿主已释放；只有原 `runtimeInstanceId` 的停止确认才能清除标记。未知宿主不可按 PID、超时或空实例响应接管。宿主崩溃后无法取得原实例确认时保持 blocked，需要先核验旧进程全部退出后进行人工恢复；本期不提供强制接管。

## 目标与边界

本改造起于 haoxiao 测试分支 build 15.1（`a36bc7885f20c58426d80cc9dcb70feecdb4828f`），并整合 build 18.1（`b78ec43912fec74df38d25d5af0b9e0842434f0f`）的外部 `auth exchange` 和主管 `manage login`。本地接入复用受管登录，不改变 DEAP 创建/管理/执行的 MCP 协议或机器人 Stream 收发协议。

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
dws dingtalk-tag connect unbind --agent-uuid <agentUuid> --dry-run
# 确认解绑成功后再连接目标 Agent
dws dingtalk-tag connect --agent-uuid <agentUuid> --channel qoder --daemon --alwayson --dry-run
```

真实 connect 需汇总确认；可先用 `--dry-run`。默认前台，daemon 仅在 Event ready 且 Agent 初始化完成后报告运行成功。初始化成功不代表模型授权或真实消息已经验收。

未发布的 `connection` 组已收拢到 `connect`。`connect` 是同时拥有业务执行和子命令的显式 hybrid；CLI、Help 与 Schema 均保留裸 connect。Schema 精确路径优先返回父工具自身，子工具通过产品导航或各自精确路径查询。机器人 `dev connect` 路径不变。

## 统一生命周期（本次需求）

- `stop` 保存 stopped 期望，停止该员工，保留 binding 与 Profile；`restart` 复用保存的 Profile，不向主管重新换票。
- `unbind` 先停止并确认释放，随后写入 unbound 墓碑；保留 Profile、Token、去重记录、会话及审计。
- 更换 Agent 或设备时先完成 `connect unbind`，再执行 `connect`；新连接保存新的服务端 ID 和 bindingRevision 后才启动目标 Adapter。解绑失败不得启动新实例；新绑定提交后启动失败用 restart 恢复。
- 状态区分 `bindingState`（bound/unbound/unbinding；旧版 rebinding 记录仍需核对恢复）、`desiredState`（running/stopped）、`runtimeState`，并返回绑定版本、实例、transportReady、executorReady、observedAt 和阻塞原因。旧 `status` 保留兼容。
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

正文仅通过受限 stdin 进入 DWS 下行命令；Agent 自身协议按现有实现运行，custom 的问题仍作为末参传递。DWS 凭据不传给 Agent，但这不是操作系统级沙箱；Agent 自身访问权限由权限参数和宿主控制。

## 验收记录口径

必须分别记录数字员工链路与 dev connect 回归。真实消息往返、群白名单、双员工隔离、Agent 授权、后台恢复及 DSH 的实机结果不能用 mock 替代。构建、focused/race、Schema、完整 Go 测试分别记录；因主机执行策略、缺测试身份或发布审批未完成的项目标为 NOT VERIFIED。

回滚本地连接时先使用 connect stop，仅停止本员工；回滚代码使用 PR revert。回滚前停止新版宿主；旧二进制不应读取新绑定版本后继续启动。无需删除员工或 Profile，不自动合并或发布 tag。
