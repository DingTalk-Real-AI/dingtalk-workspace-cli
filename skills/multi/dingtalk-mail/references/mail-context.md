# 上下文传递表

## 上下文传递表

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `mailbox list` | 邮箱地址 | message search/get/send/thread get 的 --email/--from |
| `message search` | `messageId` | message get 的 --id |
| `message search` | `conversationId` | thread get 的 --id |
| `message search` | `messageId` | attachment list 的 --id |
| `attachment list` | `attachments[].id` / `attachments[].name` | attachment download 的 --attachment-id / --name |
| `message get` | `conversationId` | thread get 的 --id |
| `folder list` | `folders[].id` | folder create 的 --folder；folder delete/update 的 --id |
| `folder create` | `result.folder.id` | 后续创建子文件夹或移动邮件时作为 --folder；更新/删除该文件夹时作为 --id |
| `folder list` | `folders[].id` | thread list 的 --folder |
| `thread list` | `conversations[].id` | thread get/update/trash/batch-update/batch-trash 的 --id/--ids |
| `tag list` | `tags[].id` | thread update/batch-update 的 --tag-ids |
| `tag list` | `tags[].id` | tag create 的 --parent-id；tag delete/update 的 --id |
| `tag create` | `result.tag.id` | 后续创建子标签时作为 --parent-id；更新/删除该标签时作为 --id |
| `aisearch person` → `contact user get` / `contact user search` / `mail user search` | 用户邮箱 (orgAuthEmail / email) | message send 的 --to/--cc（三路并发，取先到结果） |
| `user search` | 用户邮箱 (email) | message send 的 --to/--cc |
| `message send` / `draft send` / `message reply` / `message reply-all` / `message forward` | `internetMessageId` | `message verify` 的 --internet-message-id |
