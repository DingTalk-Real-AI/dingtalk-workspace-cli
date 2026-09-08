# 数字员工本地 Agent Adapter

## 目标与边界

本改造基于 haoxiao 测试分支 build 15.1（`a36bc7885f20c58426d80cc9dcb70feecdb4828f`），不改动公开 auth exchange、DEAP manage/run/capability 或机器人 Stream 收发协议。

- 创建/管理数字员工与本地接入独立；connect 不创建、修改或发布数字员工。
- 支持 Qoder、QoderWork、WorkBuddy、Claude Code、CodeBuddy、Codex、Gemini、OpenCode、custom 和 DSH。
- 普通 Agent 复用 dev connect 的调用、模型、目录、会话及权限实现；传输改为员工 Event Consumer 上行、员工 Profile 引用回复下行。第一阶段仅文本。
- DSH 只交接注册配置，由其宿主管理 Event Consumer 和 Agent；仍兼容旧 DSH binding。
- OpenClaw、Hermes、卡片、多媒体、知识库及远程审计扩展不属于本次数字员工新链路。

## 命令

```bash
dws dingtalk-tag connect --agent-uuid <agentUuid> --profile-only
dws dingtalk-tag connect --agent-uuid <agentUuid> --channel dsh
dws dingtalk-tag connect --agent-uuid <agentUuid> --channel codex --agent-workdir <directory> --daemon --alwayson
dws dingtalk-tag connection status --agent-uuid <agentUuid> --format json
dws dingtalk-tag connection list --format json
dws dingtalk-tag connection stop --agent-uuid <agentUuid> --format json
dws dingtalk-tag connection restart --agent-uuid <agentUuid> --format json
```

真实 connect 需汇总确认；可先用 `--dry-run`。默认前台，daemon 仅在 Event ready 且 Agent 初始化完成后报告运行成功。初始化成功不代表模型授权或真实消息已经验收。

运行管理单独放在 `connection` 组，保留 connect 作为 Schema 可发现的叶子。该基线的 Schema 身份收集不支持 hybrid 父命令；本次不修改全局框架或机器人命令的目录结构。

## 身份、状态与恢复

connect 使用主管取得一次性授权码，严格使用返回的 Client ID 换票，按发布详情校验员工身份后保存独立 Profile，不切换主管 Current。普通 Agent 的进程、会话及处理记录按员工 Profile 摘要和 Adapter 隔离，不用共享 OAuth Client ID 隔离员工。

默认仅主管能触发；其他用户通过精确 userId 白名单开放，并在员工上下文解析开放 ID。群消息同时检查群和发送人。不同 Adapter 的已有绑定返回冲突，不隐式替换 DSH。

重启使用保存的员工 Profile，不重新向主管换票。显式 connect 完成新的授权后才开始新的重试预算。订阅错误遵守已知不可重试 0 次、可重试 2 次、未知 1 次的预算，并遵守服务端冷却时间；terminal hold 不自动解除。

每会话排队；已接收消息按会话和消息 ID 去重，稳定幂等键用于下行。发送结果 unknown 或处理途中崩溃均标为 needs_review，不盲目重发、不重复调用 Agent。本地无正文审计可靠落盘失败后停止接收新任务。没有服务端业务 ACK/replay/cursor 时不承诺 exactly-once 或断线不丢消息。

正文仅通过受限 stdin 进入 DWS 下行命令；Agent 自身协议按现有实现运行，custom 的问题仍作为末参传递。DWS 凭据不传给 Agent，但这不是操作系统级沙箱；Agent 自身访问权限由权限参数和宿主控制。

## 验收记录口径

必须分别记录数字员工链路与 dev connect 回归。真实消息往返、群白名单、双员工隔离、Agent 授权、后台恢复及 DSH 的实机结果不能用 mock 替代。构建、focused/race、Schema、完整 Go 测试分别记录；因主机执行策略、缺测试身份或发布审批未完成的项目标为 NOT VERIFIED。

回滚本地连接时先使用 connection stop，仅停止本员工；回滚代码使用 PR revert。不删除已创建员工、Profile 或历史 DSH 配置，不自动合并或发布 tag。
