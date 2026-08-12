# IM 事件输出与 Chat 交接

Agent 默认使用 `--flatten -f ndjson`，顶层直接读取字段，不再 `fromjson`。消息接收常见字段：

| 字段 | 语义 |
|---|---|
| `type` / `event_id` / `timestamp` | 事件类型、去重 ID、时间 |
| `subscribe_id` | 个人订阅与本地输出隔离键 |
| `message_id` / `conversation_id` | 稳定消息与会话 ID |
| `sender` / `sender_open_dingtalk_id` | 展示名与稳定发送人开放 ID |
| `content` / `create_time` / `event_time` | 正文及时间 |
| `quoted_message` | 可选引用原消息 |
| `forward_messages` | 可选合并转发子消息数组 |

引用和转发子消息字段为 `message_id`、`conversation_id`、`sender`、
`sender_open_dingtalk_id`、`content`、`create_time`。按结构识别转发，不匹配本地化摘要。

动作事件通用字段之外：已读提供 `reader_open_dingtalk_id/read_time`，撤回提供
`recaller_open_dingtalk_id/recall_time`，reaction 提供
`operator_open_dingtalk_id/reaction_name/reaction_text/operation_type/operation_time`。

成员加入/退出提供 `conversation_id`、`operator_open_dingtalk_id`、`members[]` 和
`event_time`；成员稳定 ID 是 `members[].open_dingtalk_id`。群标题变化和群解散仅保守承诺
基础路由字段及 `payload`，不得猜未由真实样本确认的键。

## 事件驱动回复的精确 ID 映射

<!-- DWS_EVENT_CHAT_HANDOFF_START -->
| event field | exact chat target |
|---|---|
| `conversation_id` | `dws chat +messages-send --as user --group <conversation_id>` |
| `sender_open_dingtalk_id` | `dws chat +messages-send --as user --open-dingtalk-id <sender_open_dingtalk_id>` |
<!-- DWS_EVENT_CHAT_HANDOFF_END -->

在上述命令后追加真实 `--text`/`--markdown` 和必要 confirmation。禁止使用 `sender` 再做
`--user-query`，也禁止把单聊发送人 ID 当成群 ID。交接 marker 由 policy 逐字验证。

媒体事件正文可能只是描述。优先按真实消息 ID/会话 ID 使用 Chat 消息读取命令加
`--download-resources`；只有精确 lower fallback 才用 `chat message download-media`。
