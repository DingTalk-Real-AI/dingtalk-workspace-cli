---
name: dingtalk-event
description: 钉钉个人 IM、OA 审批、VoIP 通话邀请、待办、日历、互动卡片、好友与听记事件长连接监听。Use when 用户说监听消息/@我/某人/某群/全部消息、已读/撤回/reaction、群成员加入/群成员退出，监听审批任务创建/完成/转交、审批实例发起/抄送/终止/完成、VoIP 通话邀请、待办创建/更新/删除、日程创建/新增参与/修改/取消、好友申请/好友添加成功、听记创建/纪要生成，或互动卡片回调，以及应用事件发现、和事件Schema 查询、实时消费。命令前缀：dws event。
metadata:
  cli_version: ">=0.2.14"
  category: product
  requires:
    bins:
      - dws
---

# 钉钉个人与应用事件

> **前置：执行 `dws` 前必须完整读取 [`dingtalk-shared`](../dingtalk-shared/SKILL.md)。**Shared references 仅按需加载。

个人事件以外，应用事件经远端目录发现（见 [应用事件参考](references/event-app.md)）；消息、审批、待办、日程操作分别走 `dingtalk-chat`、`dingtalk-misc`、`dingtalk-todo`、`dingtalk-calendar`。

<!-- dws-intent: event.listen.im -->消息、reaction、已读和撤回的默认监听入口是 `dws event +listen-im`；
只有群生命周期、Filter DSL、原始 envelope 或底层订阅控制才使用 `event consume`。

<!-- dws-intent: event.listen.oa -->OA 审批任务与审批实例的实时变化使用 `dws event consume`；查询或操作已有审批走 `dws oa`，不要用轮询模拟事件。

<!-- dws-intent: event.listen.todo -->待办创建、更新或删除的实时变化使用 `dws event consume`；查询或操作已有待办走 `dws todo`，不要用轮询模拟事件。

## Golden Route

| 用户意图 | 唯一推荐入口 |
|---|---|
| 监听 @我的消息 | `dws event +listen-im --kind at-me` |
| 监听某人发来的消息 | `dws event +listen-im --kind sender --user-query <姓名>` |
| 监听指定群消息 | `dws event +listen-im --kind group --chat-query <群名>` |
| 同一人/群的消息、表情、已读或撤回 | `dws event +listen-im --kind <sender|group> --events message,reaction,read,recall ...` |
| 监听全部单聊或全部群消息 | `dws event +listen-im --kind <all-direct|all-group>`（`user_im_message_receive_o2o_all`/`user_im_message_receive_group_all`）；只有用户明确要求“全部”时使用 |
| 群改名、成员进退、群解散 | EventKey 为 `user_im_group_updated`、`user_im_group_member_added`、`user_im_group_member_exited`、`user_im_group_disbanded`，读取 [EventKey 索引](references/event-im-keys.md) |
| OA 审批任务或实例事件 | [OA 参考](references/event-oa.md)：`event consume` |
| VoIP 通话邀请 | [VoIP 参考](references/event-voip.md)：`event consume` |
| 待办创建/更新/删除 | [Todo 参考](references/event-todo.md)：`event consume` 与 `--role-types` |
| 日程创建/新增参与/修改/取消 | [日历参考](references/event-calendar.md)：`event consume user_calendar_event_changed` |
| 好友申请/添加成功 | [好友参考](references/event-contact.md)：`event consume` |
| 互动卡片回调 | [卡片参考](references/event-card.md) |
| 听记创建/纪要生成 | <!-- dws-intent: event.listen.minutes -->`dws event consume`，见 [听记参考](references/event-minutes.md) |
| 应用事件发现 → 事件 Schema → 消费 | [event-app.md](references/event-app.md)：`event list --category appEvent`、`event schema`、`event consume` |
| 查看事件目录 | `dws event list --category <分类>` |
| 已知 EventKey 或需要底层订阅控制 | `dws event consume`；参数与约束以 leaf Schema 为准 |
| 查看状态 / 停止 | `dws event status` / `dws event stop <subscribe_id> --dry-run`，确认后再 `--yes` |

默认 `--events message`，可选 `message,reaction,read,recall`。`at-me/all-direct/all-group` 只支持 message；`sender` 只能选一个 `--user`/`--open-dingtalk-id`/`--user-query`；`group` 只能选一个 `--chat-id`/`--chat-query`；混入非 message 时不能用 `--query`。

OA/好友/听记各 EventKey 使用 `ruleType=all`、空 `filterRule`，不接受目标或消息过滤；Todo 仅接受 `--role-types creator,executor,participant`，省略取并集。每项独立订阅并共享 bus。

<!-- dws-intent: event.listen.card -->互动卡片使用 `dws event consume user_card_action_triggered --flatten -f ndjson`、`ruleType=all`、`filterRule={}`；不接受目标、角色或消息过滤。结构化上下文在 `payload.body.actionData.context`：`questions[].id` 关联 `answers[question_id]` 读取 `selected`，详见 [卡片参考](references/event-card.md)。

<!-- dws-intent: event.listen.contact -->好友申请/好友添加成功事件使用 `dws event consume`；查询或操作好友关系走 `dws contact`，不要轮询模拟事件。

<!-- dws-intent: event.listen.calendar -->主日历创建/新增参与/修改/取消使用 `dws event consume user_calendar_event_changed --flatten -f ndjson`；过滤与去重见 [日历参考](references/event-calendar.md)。

姓名/群名零命中或多候选时停止；解析、监听、状态、停止使用同一 `--profile`，不得跨组织搬运 ID。

### EventKey 索引

16 个 EventKey 及目标约束见 [索引](references/event-im-keys.md)；群输出可含 `operator_open_dingtalk_id`、`members[].open_dingtalk_id`。

## 运行与结果契约

- 正常消费使用当前用户 OAuth、`--flatten` 和 NDJSON；stdout 仅事件，stderr 为生命周期状态。
- 单事件 ready：`[event] ready event_key=<key> bus_pid=<pid> subscribe_id=<id>`。
- 多事件先逐条输出 subscription，全部就绪后输出 `[event] ready event_count=<n> bus_pid=<pid>`；等 ready，不用 `sleep` 猜测。
- 有界任务用 `--max-events N`/`--duration 10m`，干净退出取消本次新订阅；无界任务用 SIGTERM 或 bounded exit，不用 `kill -9`。
- 自发消息会被 self-loop 过滤；回复用真实 `conversation_id`/`sender_open_dingtalk_id`，见[回复交接](references/event-im-output.md)。
- 扁平消息/动作字段按事件类型读取：已读为 `reader_open_dingtalk_id`，撤回为 `recaller_open_dingtalk_id`，回应为 `reaction_name`、`operation_type`。媒体优先通过聊天读取命令加 `--download-resources`，已知消息 ID 用 `dws chat message download-media`。
- OA/Todo 扁平事件提供审批实例/任务与 `task_id` 等字段；稳定 ID 交接见对应 reference。
- 互动卡片输出 `type/event_id/timestamp/subscribe_id/payload`；操作者 `operatorDTO.uid`，`triggerTimestamp` 等三个时间戳照原样保留；回退见 [互动卡片参考](references/event-card.md)。

## 安全与失败处理

开源 normal 获取缺失 AppKey，无需 AppSecret；custom／定制版除外。[排障](references/event-im-operations.md)。

- `event stop` 取消订阅并影响本地 consumer：先 `--dry-run`，确认后 `--yes`。
- 多事件属于一次原始操作；任一订阅启动失败时 Runtime 回滚本次已创建项，不拆成新命令绕过重试预算。
- 全部 33 个公开个人 EventKey（16 IM + 7 OA + 1 VoIP + 3 Todo + 1 卡片 + 2 好友 + 2 听记 + 1 日历）遵循 Agent/host `0/2/1`：`retryable=false`→`max_additional_attempts=0`；`retryable=true`→`max_additional_attempts=2`；`retryable=unknown`→`max_additional_attempts=1`。它不是 CLI 持久化硬总次数上限；进程内不会自动重试，也不持久化或计算跨调用的 Agent/host 尝试次数。
- 遵守 `retry_after_seconds`/`next_retry_at`；`in_flight`、`cooldown`、`terminal_hold` 时不并发或换 `subscribe_id`/`trace_id` 绕过。
- 认证、profile、订阅保护状态和 bus 排障按失败类型读取 [订阅运维](references/event-im-operations.md)。

### 本地订阅保护

默认记录为 `~/.dws/events/open/personal_stream/<identity_hash>/personal_subscription_attempts.json`，`DWS_CONFIG_DIR` 改变根目录。identity 目录权限 `0700`，JSON 与 `personal_subscription_attempts.lock` 为 `0600`；连续 24h 无失败后重置，`terminal_hold` 为 1h。

紧急恢复前确认没有创建进程，只删除 `personal_subscription_attempts.json`，不要删除 lock 文件；这会清空该 identity 的全部保护记录，随后仅尝试一次原始操作。详见 [订阅运维](references/event-im-operations.md)。

## 何时查询 Schema

- 已知 Golden Route 直接执行；解析业务字段用 `dws event schema <event_key> --flatten`。
- 参数或安全不确定时用 `dws schema --cli-path "event +listen-im" --compact` 或对应 compact leaf。
- `event schema` 描述事件 payload；顶层 `dws schema` 描述 CLI 命令，两者不要混用。

## Reference

| 何时读取 | Reference |
|---|---|
| 应用事件发现、事件 Schema、版本与安全 | [event-app.md](references/event-app.md) |
| 任务索引 | [event-im.md](references/event-im.md) |
| EventKey、目标规则与底层 consume | [event-im-keys.md](references/event-im-keys.md) |
| ready、bounded consume 与退出清理 | [event-im-lifecycle.md](references/event-im-lifecycle.md) |
| 扁平字段与事件到 Chat 交接 | [event-im-output.md](references/event-im-output.md) |
| Filter、status/stop、重试与排障 | [event-im-operations.md](references/event-im-operations.md) |
| OA 审批事件 | [event-oa.md](references/event-oa.md) |
| VoIP 通话邀请 | [event-voip.md](references/event-voip.md) |
| Todo 待办事件 | [event-todo.md](references/event-todo.md) |
| 好友事件 | [event-contact.md](references/event-contact.md) |
| 互动卡片回调事件 | [event-card.md](references/event-card.md) |
| 日历创建/修改/取消、过滤与去重 | [event-calendar.md](references/event-calendar.md) |
