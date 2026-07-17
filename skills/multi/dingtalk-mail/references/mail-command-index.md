# 命令速查目录

## 按需加载详细参数

| 命令族 | 详见 |
|---|---|
| 邮箱选择、邮件列表/搜索/读取/发送 | [`mail-message-commands.md`](./mail-message-commands.md) |
| 文件夹、附件、标签 | [`mail-folder-attachment-tag-commands.md`](./mail-folder-attachment-tag-commands.md) |
| 会话、回复/转发、批量动作、撤回 | [`mail-thread-actions.md`](./mail-thread-actions.md) |
| 草稿、用户搜索、模板、联系人 | [`mail-draft-directory-commands.md`](./mail-draft-directory-commands.md) |
| 自动回复、黑白名单、收信规则 | [`mail-policy-commands.md`](./mail-policy-commands.md) |

## 命令速查目录

| 命令 | 功能简述 |
|------|----------|
| `dws mail mailbox list` | 查询**当前用户自己**的可用邮箱列表 |
| `dws mail mailbox profile` | 获取用户邮箱详细信息（容量、别名等） |
| `dws mail message list` | 列出指定文件夹中的邮件（默认收件箱） |
| `dws mail message search` | 搜索邮件（KQL 语法，按主题/发件人/日期等） |
| `dws mail message get` | 查看邮件完整内容（含正文） |
| `dws mail message send` | 发送邮件（支持附件/内联图片） |
| `dws mail message reply` | 回复邮件（支持附件/内联图片） |
| `dws mail message reply-all` | 回复所有人（支持附件/内联图片） |
| `dws mail message forward` | 转发邮件（支持附件/内联图片） |
| `dws mail message batch-move` | 批量移动邮件到指定文件夹 |
| `dws mail message batch-delete` | 批量删除邮件 |
| `dws mail message batch-update` | 批量修改邮件状态（标记已读/未读/添加标签/移除标签） |
| `dws mail message batch-get` | 批量获取邮件详情（最多 20 封） |
| `dws mail message verify` | 根据 internetMessageId 查询邮件发送状态 |
| `dws mail sent-message recall` | 撤回已发送的邮件（仅支持同组织内未读邮件） |
| `dws mail sent-message recall-detail` | 查询邮件撤回进度 |
| `dws mail draft create` | 创建草稿（保留在草稿箱，不发送） |
| `dws mail draft update` | 更新草稿内容（保留在草稿箱，不发送） |
| `dws mail draft send` | 发送草稿箱中已有的草稿 |
| `dws mail folder list` | 列举邮件文件夹 |
| `dws mail folder create` | 创建邮件文件夹 |
| `dws mail folder delete` | 删除邮件文件夹 |
| `dws mail folder update` | 更新邮件文件夹名称 |
| `dws mail attachment list` | 列举指定邮件的所有附件 |
| `dws mail attachment download` | 下载邮件附件到本地（**仅支持逐个下载，不支持批量下载**） |
| `dws mail tag list` | 列举邮件标签 |
| `dws mail tag create` | 创建邮件标签 |
| `dws mail tag delete` | 删除邮件标签 |
| `dws mail tag update` | 更新邮件标签名称 |
| `dws mail thread list` | 列出指定邮箱、指定文件夹下的邮件会话 |
| `dws mail thread get` | 获取会话详情 |
| `dws mail thread update` | 修改单个邮件会话的状态或标签（标记已读/未读/添加标签/移除标签） |
| `dws mail thread batch-update` | 批量修改邮件会话的状态或标签（单次最多 100 个） |
| `dws mail thread trash` | 将单个邮件会话移动到已删除文件夹（不会永久删除） |
| `dws mail thread batch-trash` | 将多个邮件会话批量移动到已删除文件夹（单次最多 100 个，不会永久删除） |
| `dws mail user search` | 搜索通讯录用户（**按姓名或工号查他人邮箱**，不是搜邮件） |
| `dws mail template create` | 创建邮件模板 |
| `dws mail template list` | 列举邮件模板 |
| `dws mail template get` | 获取邮件模板详情 |
| `dws mail template update` | 更新邮件模板 |
| `dws mail template delete` | 删除邮件模板 |
| `dws mail contact create` | 创建个人邮件联系人（添加到自己的联系人列表） |
| `dws mail contact list` | 列举个人邮件联系人（查看自己保存的联系人，**不是搜索通讯录用户**） |
| `dws mail contact update` | 更新个人邮件联系人信息 |
| `dws mail contact batch-delete` | 批量删除个人邮件联系人 |
| `dws mail auto-reply get` | 获取用户的自动回复配置 |
| `dws mail auto-reply update` | 更新/设置用户的自动回复配置 |
| `dws mail allow-list list` | 列出个人收信白名单 |
| `dws mail allow-list add` | 添加个人收信白名单 |
| `dws mail allow-list remove` | 移除个人收信白名单 |
| `dws mail block-list list` | 列出个人收信黑名单 |
| `dws mail block-list add` | 添加个人收信黑名单 |
| `dws mail block-list remove` | 移除个人收信黑名单 |
| `dws mail rule list` | 列出个人收信规则 |
| `dws mail rule create` | 创建个人收信规则 |
| `dws mail rule update` | 更新个人收信规则 |
| `dws mail rule delete` | 删除个人收信规则 |
| `dws mail rule adjust` | 调整收信规则排序 |

> **查找他人邮箱**（如「获取严龙的邮箱」）→ **不要用 `mailbox list`**，应走三路并发查询，详见「查找他人邮箱地址」章节。

---
