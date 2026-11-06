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

本 Skill 负责个人事件与应用事件消费；应用事件的发现、事件 Schema、业务版本和安全边界见 [应用事件参考](references/event-app.md)。发送/历史消息、审批、待办、日程操作分别走 `dingtalk-chat`、`dingtalk-misc`、`dingtalk-todo`、`dingtalk-calendar`。

<!-- dws-intent: event.listen.im -->IM 消息、reaction、已读和撤回优先用 `dws event +listen-im`；群生命周期、Filter DSL、原始 envelope 或底层控制才用 `event consume`。
<!-- dws-intent: event.listen.oa -->OA 审批任务/实例变化用 `event consume`；已有审批查询/操作走 `dws oa`，不要轮询模拟事件。
<!-- dws-intent: event.listen.todo -->待办创建、更新、删除用 `event consume`；已有待办走 `dws todo`。
<!-- dws-intent: event.listen.minutes -->听记创建/纪要生成用 `event consume`，见 `event-minutes.md`。
<!-- dws-intent: event.listen.card -->互动卡片回调用 `event consume user_card_action_triggered`，见 `event-card.md`。
<!-- dws-intent: event.listen.contact -->好友申请/添加成功用 `event consume`，关系查询走 `dws contact`。
<!-- dws-intent: event.listen.calendar -->日历主事件用 `event consume user_calendar_event_changed`，见 `event-calendar.md`。

## Golden Route

| 用户意图 | 入口 |
|---|---|
| @我、某人、某群、全部单聊/群聊 | `dws event +listen-im --kind <at-me\|sender\|group\|all-direct\|all-group>`；姓名/群名用唯一 query |
| 同一人/群的消息、reaction、已读、撤回 | `+listen-im --kind <sender\|group> --events message,reaction,read,recall` |
| 群生命周期 | `event consume` + [EventKey 索引](references/event-im-keys.md) |
| OA、VoIP、待办、日历、好友、听记、卡片 | `event consume` + 对应 reference；待办角色用 `--role-types` |
| 应用事件 | [event-app.md](references/event-app.md)：`event list --category appEvent` → `event schema` → `event consume` |
| 已知 EventKey、状态或停止 | `event consume`；`event status`；`event stop <subscribe_id> --dry-run` 后确认 `--yes` |

IM 默认 `--events message`，可选 `message,reaction,read,recall`。`at-me/all-direct/all-group` 只支持 message；`sender` 只能选一个 `--user`、`--open-dingtalk-id` 或 `--user-query`；`group` 只能选一个 `--chat-id` 或 `--chat-query`；混入非 message 时不能用 `--query`。姓名/群名零命中或多候选停止。解析、监听、状态、停止使用同一 `--profile`，不得跨组织搬 ID。

OA 七个 EventKey、好友和听记使用 `ruleType=all`、空 `filterRule`，不接受目标过滤；Todo 只接受 `--role-types creator,executor,participant`；卡片不接受目标/角色/消息过滤；日历过滤与去重见 reference。EventKey 目标约束见 [event-im-keys.md](references/event-im-keys.md)。

## 应用事件说明

应用事件不是内置个人 EventKey：先从登录后的远端目录取得真实 `eventKey`、应用 ID 和 scope，再用 `dws event schema <event_key> --flatten` 查询**事件 payload Schema**，最后用 `dws event consume <event_key> --flatten -f ndjson` 实时消费。业务字段保留在 `.payload`，业务版本为 `payload_schema_version`；顶层 `dws schema` 只描述 CLI 参数，不是业务事件 Schema。应用事件不接受用户、群、角色或 query 过滤；目录分页、版本查询、DRAFT/ONLINE/debug、体验投递、授权和安全边界统一见 [event-app.md](references/event-app.md)。

## 运行与结果契约

- 消费使用当前 OAuth、`--flatten`、NDJSON；stdout 只输出事件，stderr 输出生命周期。
- 单事件 ready：`[event] ready event_key=<key> bus_pid=<pid> subscribe_id=<id>`；多事件等全部 subscription 后的 `event_count` ready。ready 只代表订阅建立，不代表收到业务事件。
- 有界任务用 `--max-events N`/`--duration 10m`；正常退出清理本次新订阅。无界任务用 SIGTERM 或 bounded exit，不用 `kill -9`。
- 自发消息 self-loop 过滤。已读为 `reader_open_dingtalk_id`，撤回为 `recaller_open_dingtalk_id`，reaction 为 `reaction_name`/`operation_type`；媒体沿 Chat 下载。OA/Todo/卡片字段和交接见对应 reference。

## 安全与失败处理

开源 normal 获取缺失 AppKey，无需 AppSecret；custom/定制版除外。[订阅运维](references/event-im-operations.md)。

- `event stop` 先 dry-run，确认后才 `--yes`；多事件任一启动失败时整体回滚，不拆分绕过保护。
- 遵守 `retryable`、`retry_after_seconds`、`next_retry_at`；`in_flight`、`cooldown`、`terminal_hold` 时不并发、不换 ID 绕过。个人事件 Agent/host 额外尝试预算为 false=0、true=2、unknown=1。
- 本地保护记录在 `~/.dws/events/open/personal_stream/<identity_hash>/personal_subscription_attempts.json`，`DWS_CONFIG_DIR` 可改根目录；目录 0700，JSON/lock 0600。连续 24h 无失败重置，`terminal_hold` 1h。紧急恢复只删 JSON，不删 lock，并只尝试一次原始操作。

## 查询 Schema 与 references

已知 Golden Route 直接执行，不先探目录；解析 payload 用 `dws event schema <event_key> --flatten`；参数不确定时查对应 CLI leaf 的 compact Schema。`event schema` 是事件 payload，`dws schema` 是命令契约，不能混用。

| 主题 | Reference |
|---|---|
| 应用事件发现、Schema、版本、安全与开放态 | [event-app.md](references/event-app.md) |
| EventKey 与目标规则 | [event-im-keys.md](references/event-im-keys.md) |
| 生命周期与 ready | [event-im-lifecycle.md](references/event-im-lifecycle.md) |
| 扁平字段与 Chat 交接 | [event-im-output.md](references/event-im-output.md) |
| 状态、停止、重试与排障 | [event-im-operations.md](references/event-im-operations.md) |
| OA / VoIP / Todo / 好友 / 卡片 / 日历 / 听记 | `event-oa.md` / `event-voip.md` / `event-todo.md` / `event-contact.md` / `event-card.md` / `event-calendar.md` / `event-minutes.md` |
