# 核心工作流

## 核心工作流

```bash
# 1. 查看可用邮箱 — 提取邮箱地址
dws mail mailbox list --format json

# 1b. 创建顶层邮件文件夹
dws mail folder create --email user@company.com --name "项目资料" --format json

# 1c. 创建子文件夹 — 先通过 folder list 获取父文件夹 id，再传给 --folder
dws mail folder list --email user@company.com --format json
dws mail folder create --email user@company.com --name "子文件夹" --folder <folderId> --format json

# 1d. 更新或删除邮件文件夹 — 先通过 folder list 获取目标文件夹 id，再传给 --id
dws mail folder list --email user@company.com --format json
dws mail folder update --email user@company.com --id <folderId> --name "新文件夹名" --format json
dws mail folder delete --email user@company.com --id <folderId> --format json

# 1e. 创建顶层邮件标签
dws mail tag create --email user@company.com --name "项目资料" --format json

# 1f. 创建子标签 — 先通过 tag list 获取父标签 id，再传给 --parent-id
dws mail tag list --email user@company.com --format json
dws mail tag create --email user@company.com --name "子标签" --parent-id <tagId> --format json

# 1g. 更新或删除邮件标签 — 先通过 tag list 获取目标标签 id，再传给 --id
dws mail tag list --email user@company.com --format json
dws mail tag update --email user@company.com --id <tagId> --name "新标签名" --format json
dws mail tag delete --email user@company.com --id <tagId> --format json

# 1h. 列出文件夹中的邮件会话 — 先通过 folder list 获取文件夹 id，再传给 --folder
dws mail folder list --email user@company.com --format json
dws mail thread list --email user@company.com --folder <folderId> --limit 10 --format json

# 1i. 修改或删除邮件会话 — 先通过 thread list 获取 conversationId
dws mail thread update --email user@company.com --id <conversationId> --action markRead --format json
dws mail thread batch-update --email user@company.com --ids <conversationId1>,<conversationId2> --action markUnread --format json
dws mail thread trash --email user@company.com --id <conversationId> --format json
dws mail thread batch-trash --email user@company.com --ids <conversationId1>,<conversationId2> --format json

# 2. 搜索邮件 — 提取 messageId
dws mail message search --email user@company.com \
  --query "subject:\"周报\" AND date>2025-06-01T00:00:00Z" --limit 10 --format json

# 3. 查看邮件详情
dws mail message get --email user@company.com --id <messageId> --format json

# 4. 发送邮件（纯文本）
dws mail message send --from user@company.com --to colleague@company.com \
  --subject "周报" --content "本周完成…" --format json

# 4b. 发送带附件的邮件（自动编排：创建草稿→上传附件→发送草稿）
dws mail message send --from user@company.com --to colleague@company.com \
  --subject "周报" --content "见附件" --attachment ./report.pdf --format json

# 4c. 发送带内联图片的邮件（正文自动转 HTML，<img> 标签自动注入）
dws mail message send --from user@company.com --to colleague@company.com \
  --subject "图表周报" --content "本周图表如下：[inline:chart.png]" \
  --inline-attachment ./chart.png --format json

# 5. 下载邮件附件到本地（每次只能下载一个附件，不支持批量下载）
# 步骤 5.1：搜索匹配的邮件，获取 messageId 列表
# 示例：下载4月所有发票邮件的附件
dws mail message search --email user@company.com \
  --query "subject:发票 AND date>2025-04-01T00:00:00Z AND date<2025-05-01T00:00:00Z AND hasAttachments:true" --limit 50 --format json

# 步骤 5.2：对每封邮件，列出附件获取 attachmentId 和 name
# （对搜索结果中的每封邮件都要执行一次）
dws mail attachment list --email user@company.com --id <messageId> --format json

# 步骤 5.3：对每个附件逐个下载（没有批量下载命令，必须循环调用）
dws mail attachment download --email user@company.com \
  --message-id <messageId> --attachment-id <attachmentId> --name report.pdf --output ~/invoices/

# 6. 获取邮件所属会话详情（thread）
# 步骤 6.1：先通过 message search 或 message get 获取邮件中的 conversationId
dws mail message search --email user@company.com \
  --query "subject:\"周报\"" --limit 5 --format json
# 从返回的邮件列表中提取 conversationId 字段

# 步骤 6.2：用 conversationId 获取会话详情
dws mail thread get --email user@company.com --id <conversationId> --format json
```
