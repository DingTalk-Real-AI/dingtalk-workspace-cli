# 上下文传递表

## 上下文传递表

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `chat search` | `openConversationId` | message send/list、group members 等的 --group |
| `chat group create` | `openConversationId` | 同上 |
| `chat message list-all` | `nextCursor` | 下次 list-all 的 --cursor |
| `aisearch person` | `userId` | message send 的 --user、send-by-bot 的 --users、send-by-bot 的 --at-user-ids、list-by-sender 的 --sender-user-id |
| `contact user search` | `openDingTalkId` | message send 的 --at-open-dingtalk-ids、--open-dingtalk-id、send-by-bot 的 --open-dingtalk-ids、send-by-bot 的 --at-open-dingtalk-ids、list-by-sender 的 --sender-open-dingtalk-id、message list 的 --open-dingtalk-id |
| `chat bot search` | `robotCode` | send-by-bot / recall-by-bot 的 --robot-code（仅我创建的机器人，无 openDingTalkId） |
| `chat bot find` | `openDingTalkId` | 给机器人发单聊消息（全部可用机器人，额外返回 openDingTalkId） |
| `chat message send-by-bot` | `processQueryKey` | recall-by-bot 的 --keys |
| `chat message send` | `openTaskId` | query-send-status 的 --open-task-id |
| `chat message list` | `openMessageId` | recall 的 --msg-id |
| `chat message search` | `nextCursor` | 下次 message search 的 --cursor |
| `chat message search-advanced` | `nextCursor` | 下次 message search-advanced 的 --cursor |
| `chat search-common` | `openConversationId` | message send/list 等的 --group |
| `chat message list` | `openMsgId` | message read-status 的 --message-id |
| `chat group-role list` | `openRoleId` | group-role update/remove/set-user/remove-user 的 --role-id |
| `chat message create-text-emotion` | `emotionId` | add-text-emotion 的 --emotion-id |
| `chat category list` | `categoryId` | category list-conversations 的 --category-id |
| `chat group get-by-group-id` | `openConversationId` | 同 chat search，将群号转为 openConversationId |
| `chat message send-card` | `bizId` | update-card 的 --biz-id |
| `chat message list` | `openMessageId` | message reply 的 --ref-msg-id、message forward 的 --msg-id |
| `chat search` | `openConversationId` | set-top 的 --conversation-id、group-mute / group-mute-member 的 --group |
