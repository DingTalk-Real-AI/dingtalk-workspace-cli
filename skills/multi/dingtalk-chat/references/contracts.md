# IM 稳定契约与能力边界

本页只记录需要跨命令复用的 Runtime 契约。下面四个 marker 区块由
`check-multi-im-skill-chain.sh` 对照 Go typed descriptor 逐字校验；修改能力时先改 Runtime、
测试与 descriptor，再同步本页。命令参数仍以精确 leaf Schema 为准。

## 消息结果 `im.message-list.v1`

`+chat-messages`、`+search-msg`、`+messages-mget` 及相关消息读取使用同一兼容版本。
字段可能为空，但不能换名猜测；全量任务必须结合完整性 ledger 判断。

<!-- DWS_MESSAGE_RESULT_CONTRACT_START -->
- `version`: `im.message-list.v1`
- `message_fields`: `messageId`, `conversationId`, `threadId`, `sender`, `senderId`, `senderType`, `messageType`, `text`, `createTime`, `updateTime`, `reactions`, `quotedMessage`, `forwarded`, `resourceRefs`
- `envelope_fields`: `contractVersion`, `messages`, `count`, `resolvedFilters`, `queryRange`, `pagesFetched`, `paginationKnown`, `complete`, `hasMore`, `nextPage`, `stopReason`, `truncatedByPageLimit`, `truncatedByResultLimit`, `failedCount`, `failures`, `partial`, `resourceDownloads`
<!-- DWS_MESSAGE_RESULT_CONTRACT_END -->

当 `complete=false` 时不能称为全量成功。`nextPage` 只能来自真实 lower boundary；
`failedCount/failures`、`partial` 和两个 truncated 字段必须原样保留。
当 Runtime 解析并应用自然发送者条件时，`resolvedFilters.senders[]` 保留原查询及选中的
`userId/openDingTalkId`。消息展示名可以与通讯录姓名不同；只能用稳定 `senderId` 与解析结果关联，
不得重新做姓名字符串比较。

当查询声明时间范围或顺序时，`queryRange` 保留规范化的 `startTime`、`endTime`、`order` 与
`semantics=[start,end)`。排序只覆盖本次实际取得的 `messages`；`complete=false` 时不得把它描述为完整范围的全局排序。

## `+messages-send` 身份矩阵

<!-- DWS_IDENTITY_CAPABILITY_CONTRACT_START -->
| identity | targets | content types | natural targets | mention targets | idempotency keys | batch ledger |
|---|---|---|---|---|---:|---:|
| `user` | `group`<br>`direct-user`<br>`direct-open-dingtalk-id` | `text`<br>`markdown`<br>`image-media-id`<br>`file`<br>`audio-as-file`<br>`video-as-file` | `chat-query`<br>`user-query` | `open-dingtalk-id`<br>`all` | `true` | `false` |
| `bot` | `group`<br>`groups`<br>`direct-users`<br>`direct-open-dingtalk-ids` | `text`<br>`markdown` | — | `user-id`<br>`open-dingtalk-id`<br>`all` | `false` | `true` |
| `webhook` | `token-owned-group` | `text`<br>`markdown` | — | `user-id`<br>`mobile`<br>`all` | `false` | `false` |
<!-- DWS_IDENTITY_CAPABILITY_CONTRACT_END -->

Bot 多群用 `--groups` 或 `--groups-file`，Runtime 去重后输出
`im.batch-write.v1` 逐目标 ledger。Bot/Webhook 不支持的内容类型会在写前失败，不能降级为
另一身份或偷偷改成纯文本。

## 流式卡片

<!-- DWS_CARD_WORKFLOW_CONTRACT_START -->
- `version`: `im.streaming-card.v1`
- `targets`: `group`, `direct-user`, `direct-open-dingtalk-id`
- `content_types`: `streaming-text`
- `flow_statuses`: `1=processing`, `2=typing`, `3=completed`, `4=executing`, `5=error`
- `callback_supported`: `false`
<!-- DWS_CARD_WORKFLOW_CONTRACT_END -->

发送目标与状态范围由 Runtime 校验。当前不是 Lark Card JSON 编译器，也不消费按钮 callback；
具体创建和更新流程见 `card/` 下的精确 reference。

## 正向能力与负向边界

<!-- DWS_CAPABILITY_BOUNDARY_CONTRACT_START -->
| capability | supported | current route / boundary |
|---|---:|---|
| `thread-write` | `false` | quote reply with +messages-reply; thread reading with +thread-replies |
| `bot-rich-media` | `false` | bot text/markdown, or current-user file/image send |
| `card-action-callback` | `false` | streaming text card create/update only |
| `resource-resume` | `false` | atomic whole-file download with explicit retry |
| `group-member-full-pagination` | `true` | +chat-members-list or +group-members |
| `group-owner-selection` | `true` | +chat-create owner flags |
<!-- DWS_CAPABILITY_BOUNDARY_CONTRACT_END -->

`supported=false` 是执行门禁，不是待猜测字段。只有 lower interface、Runtime、测试、Schema 和
此页同时升级后，才能改变对外承诺。
