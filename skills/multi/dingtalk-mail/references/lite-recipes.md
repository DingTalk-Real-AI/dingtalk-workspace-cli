# mail Lite Recipe

本文件从单 Skill `lite-recipes.md` 拆分而来，仅保留与本产品相关的轻量流程。

## #9 邮件

### mail-list-mailbox

查询当前用户自己的可用邮箱地址列表。**仅返回自己的邮箱**，不能查他人邮箱（查他人邮箱请走 [mail.md](./mail.md) 中「查找他人邮箱地址」三路并发查询流程）。

`mail mailbox list`

### mail-search

搜索邮件。**必须使用 KQL 语法通过 `--query` 传递查询条件**，禁止臆造 `--subject`、`--from` 等不存在的 flag。

1. 获取邮箱地址：`mail mailbox list` → 取用户邮箱地址。若用户已提供邮箱可跳过。
2. 构造 KQL 查询：根据用户意图将搜索条件转为 KQL 表达式（详见 [mail.md](./mail.md) 中 KQL 查询字段说明）。
   - 按主题：`subject:周报`、`subject:"项目 进展"`（含空格须加双引号）
   - 按发件人：`from:alice@company.com` 或 `from:"张三"`
   - 按日期：`date>2025-06-01T00:00:00Z`（ISO8601 格式，必须含时间部分）
   - 按文件夹：`folderId:2`（2=收件箱, 1=已发送, 5=草稿, 6=已删除）
   - 按是否有附件：`hasAttachments:true`
   - 组合：`from:alice AND subject:周报 AND date>2025-06-01T00:00:00Z`
3. 执行搜索：`mail message search --email <邮箱> --query "<KQL表达式>" --limit 20`
4. 查看详情（按需）：`mail message get --email <邮箱> --id <messageId>`

### mail-send

发送邮件。

1. 获取邮箱地址：`mail mailbox list` → 取用户邮箱作为 `--from`。
2. 确定收件人：用户直接提供邮箱地址 → 直接使用；用户提供姓名或工号 → 走「查找他人邮箱地址」三路并发流程（见 [mail.md](./mail.md)）。
3. 发送：`mail message send --from <发件邮箱> --to <收件邮箱> --subject "<主题>" --content "<正文>"`（可选 `--cc`、`--attachment`、`--inline-attachment`）。

### mail-reply-forward

回复或转发邮件。

1. 获取邮箱地址：`mail mailbox list` → 取用户邮箱。
2. 定位原始邮件：若用户未提供 messageId → 先用 `mail-search` 搜索定位。
3. 执行：
   - 回复：`mail message reply --from <邮箱> --id <messageId>`（可选 `--to`、`--subject`、`--content`）
   - 回复全部：`mail message reply-all --from <邮箱> --id <messageId>`（可选 `--to`、`--subject`、`--content`）
   - 转发：`mail message forward --from <邮箱> --to <收件邮箱> --id <messageId>`（可选 `--subject`、`--content`）
