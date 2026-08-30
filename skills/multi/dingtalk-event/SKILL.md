---
name: dingtalk-event
description: 钉钉个人 IM、OA 审批与待办事件长连接监听。Use when 用户说监听消息/@我/某人/某群/全部消息、已读/撤回/reaction、群成员加入/群成员退出/群状态变化，监听审批任务创建/完成/转交、审批实例发起/抄送/终止/完成，或监听待办创建/更新/删除。命令前缀：dws event。
metadata:
  cli_version: ">=0.2.14"
  category: product
  requires:
    bins:
      - dws
---

# 钉钉个人 IM、OA 审批与待办事件

> **前置：执行 `dws` 前必须完整读取 [`dingtalk-shared`](../dingtalk-shared/SKILL.md)。**Shared references 仅按需加载。

本 Skill 只负责未来个人 IM/OA/Todo 实时事件；发送和历史消息走 `dingtalk-chat`，审批查询与处理走 `dingtalk-misc` 的 OA，待办查询与操作走 `dingtalk-todo`，开放平台应用事件配置走其 DevApp。子 reference 按需加载。

实时监听必须使用事件长连接，不写轮询脚本，不用历史消息、审批列表或待办列表查询模拟事件。高频 IM 意图优先交给 `dws event +listen-im`；它在 CLI 内解析自然目标、选择 EventKey，并复用现有订阅与 bus 生命周期。OA 审批与 Todo 事件使用显式 `dws event consume`。

<!-- dws-intent: event.listen.im -->消息、reaction、已读和撤回的默认监听入口是 `dws event +listen-im`；
只有群生命周期、Filter DSL、原始 envelope 或底层订阅控制才使用
`event consume` fallback。

<!-- dws-intent: event.listen.oa -->OA 审批任务与审批实例的实时变化使用 `dws event consume`；查询或操作已有审批走 `dws oa`，不要用轮询模拟事件。

<!-- dws-intent: event.listen.todo -->待办创建、更新或删除的实时变化使用 `dws event consume`；查询或操作已有待办走 `dws todo`，不要用轮询模拟事件。

## Golden Route

| 用户意图 | 唯一推荐入口 |
|---|---|
| 监听 @我的消息 | `dws event +listen-im --kind at-me` |
| 监听某人发来的消息 | `dws event +listen-im --kind sender --user-query <姓名>` |
| 监听指定群消息 | `dws event +listen-im --kind group --chat-query <群名>` |
| 同一人/群的消息、表情、已读或撤回 | `dws event +listen-im --kind <sender|group> --events message,reaction,read,recall ...` |
| 监听全部单聊或全部群消息 | `dws event +listen-im --kind <all-direct|all-group>`；只有用户明确要求“全部”时使用 |
| 群改名、成员进退、群解散 | 读取 [EventKey 索引](references/event-im-keys.md)，使用精确 `event consume` EventKey |
| OA 审批任务或实例事件 | 读取 [OA 事件参考](references/event-oa.md)，使用精确 `event consume` EventKey |
| 查看 OA 事件目录 | `dws event list --category oa` |
| 待办创建、更新或删除事件 | 读取 [Todo 事件参考](references/event-todo.md)，使用精确 `event consume` EventKey 与 `--role-types` |
| 查看 Todo 事件目录 | `dws event list --category todo` |
| 已知 EventKey 或需要底层订阅控制 | `dws event consume`；参数与约束以 leaf Schema 为准 |
| 查看状态 / 停止 | `dws event status` / `dws event stop <subscribe_id> --dry-run`，确认后再 `--yes` |

默认 `--events message`。可选事件为 `message`、`reaction`、`read`、`recall`：

- `at-me`、`all-direct`、`all-group` 只支持 `message`，且不接受目标。
- `sender` 必须且只能传 `--user`、`--open-dingtalk-id` 或 `--user-query` 之一。
- `group` 必须且只能传 `--chat-id` 或 `--chat-query` 之一。
- `--query` 只用于纯 `message` 监听；混入 reaction/read/recall 时不得使用。

OA 事件不进入 `+listen-im`。七个公开 OA EventKey 都订阅当前 OAuth 用户相关的全部审批事件，使用 `ruleType=all`、`filterRule={}`；不接受 `--user`、`--open-dingtalk-id`、`--group`、`--query` 或 `--filter-json`。七项可放入同一个 consume，每项建立独立订阅并共享 bus。

Todo 事件也不进入 `+listen-im`。三个公开 EventKey 使用 `--role-types creator,executor,participant` 控制当前用户作为创建者、执行者或参与者的范围；省略时默认三种角色。Todo 不接受用户、群或消息 Filter 参数，三项可共享同一角色范围和 bus。

自然姓名和群名由 CLI 内部唯一解析：零命中或多候选返回结构化失败，在创建任何订阅前停止。`--dry-run` 走同一解析链。解析、监听、状态和停止必须使用同一个 `--profile`，不得跨组织搬运 ID。

### EventKey 索引

16 个 EventKey 的目标与组合约束见 [EventKey 索引](references/event-im-keys.md)。兼容键包括 `user_im_message_receive_o2o_all`、`user_im_message_receive_group_all`、`user_im_group_updated`、`user_im_group_member_added`、`user_im_group_member_exited`、`user_im_group_disbanded`；群输出可含 `operator_open_dingtalk_id`、`members[].open_dingtalk_id`。OA 见 [OA 事件参考](references/event-oa.md)，Todo 见 [Todo 事件参考](references/event-todo.md)。

## 运行与结果契约

- 正常消费固定使用当前用户 OAuth 身份、`--flatten` 和 NDJSON；stdout 只输出事件，stderr 输出订阅、ready、退出和错误状态。
- 单事件 ready：`[event] ready event_key=<key> bus_pid=<pid> subscribe_id=<id>`。
- 多事件先逐条输出 subscription，全部就绪后输出 `[event] ready event_count=<n> bus_pid=<pid>`。必须等待 ready，不用 `sleep` 猜测。
- 有界任务使用 `--max-events N` 或 `--duration 10m`；无界任务需要宿主管理进程并持续读取 stdout。
- 干净退出会取消本次新建的订阅；使用 SIGTERM、关闭符合条件的管道 stdin，或 Runtime 的 bounded exit。不要 `kill -9`。
- 当前用户自己发送的消息会被 self-loop 过滤；自测事件应由另一用户或机器人发送。
- 事件只负责监听；需要回复时按 [输出与 Chat 交接](references/event-im-output.md) 把真实 `conversation_id` 或 `sender_open_dingtalk_id` 交给 `dws chat +messages-send`，不要从显示名猜 ID。
- 扁平消息/动作字段按事件类型读取：已读为 `reader_open_dingtalk_id`，撤回为 `recaller_open_dingtalk_id`，回应为 `reaction_name`、`operation_type`。媒体优先通过聊天读取命令加 `--download-resources`；已知消息 ID 的底层降级入口是 `dws chat message download-media`。
- OA 扁平事件提供审批实例、任务和状态字段；字段差异、原始回退条件及与 OA 命令的稳定 ID 交接以 [OA 事件参考](references/event-oa.md) 为准。
- Todo 扁平事件提供 `task_id`、标题、角色、状态阶段和时间字段；用真实 `task_id` 交给 `dws todo`，字段差异见 [Todo 事件参考](references/event-todo.md)。

## 安全与失败处理

- `event stop` 会取消订阅并影响本地 consumer：先 `--dry-run`，用户确认后再加 `--yes`。
- 多事件属于一次原始操作；任一订阅启动失败时 Runtime 回滚本次已创建项，不拆成新命令绕过重试预算。
- 这套 `0/2/1` 是 **Agent/host** 编排预算，适用于全部 26 个公开个人 EventKey（16 个 IM + 7 个 OA + 3 个 Todo）：`retryable=false` 对应 `max_additional_attempts=0`；`retryable=true` 对应 `max_additional_attempts=2`；`retryable=unknown` 对应 `max_additional_attempts=1`。它不是 CLI 持久化硬总次数上限；每次调用最多创建一次，进程内不会自动重试，CLI 也不持久化或计算跨调用的 Agent/host 尝试次数。
- 重试必须遵守 `retry_after_seconds` / `next_retry_at`。遇到 `in_flight`、`cooldown`、`terminal_hold` 不并发或递归重启同一逻辑订阅，也不换 `subscribe_id` / `trace_id` 绕过保护。
- 认证、profile、订阅保护状态和 bus 排障按失败类型读取 [订阅运维](references/event-im-operations.md)，不要在正常路径预加载完整运维手册。

### 本地订阅保护

状态在 `~/.dws/events/open/personal_stream/<identity_hash>/personal_subscription_attempts.json`（`DWS_CONFIG_DIR` 改根）；目录 `0700`，`personal_subscription_attempts.json` 与 `personal_subscription_attempts.lock` 为 `0600`。连续 `24h` 无失败后重置，`terminal_hold` 为 `1h`。紧急恢复只删除 `personal_subscription_attempts.json`，不要删除 lock 文件；这会清空该 identity 的全部保护记录。

## 何时查询 Schema

- 已知 Golden Route 时直接执行，不先跑 `event list`。
- 只有解析业务字段时才用 `dws event schema <event_key> --flatten`。
- 只有参数或安全不确定时才用 `dws schema --cli-path "event +listen-im" --compact` 或对应 compact leaf。
- `event schema` 描述事件 payload；顶层 `dws schema` 描述 CLI 命令，两者不要混用。

## Reference

| Topic | Reference | 何时读取 |
|---|---|---|
| 任务索引 | [event-im.md](references/event-im.md) | 还不能判断应该加载哪一个子 reference |
| EventKey、目标规则与底层 consume | [event-im-keys.md](references/event-im-keys.md) | 群生命周期、显式 EventKey 或多事件组合 |
| ready、bounded consume 与退出清理 | [event-im-lifecycle.md](references/event-im-lifecycle.md) | 启动/托管/关闭 consumer |
| 扁平字段与事件到 Chat 交接 | [event-im-output.md](references/event-im-output.md) | 解析事件或自动回复 |
| Filter、status/stop、重试与排障 | [event-im-operations.md](references/event-im-operations.md) | 订阅控制或失败恢复 |
| OA 审批事件 | [event-oa.md](references/event-oa.md) | 选择七个 OA EventKey、组合消费或解析审批字段 |
| Todo 待办事件 | [event-todo.md](references/event-todo.md) | 选择三个 Todo EventKey、设置角色范围或解析待办字段 |
