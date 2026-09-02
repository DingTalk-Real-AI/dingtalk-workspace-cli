# 邮件

> 邮件的 4 条 **lite** recipe：`mail-list-mailbox`、`mail-search`、`mail-send`、`mail-reply-forward`，见 [lite-recipes.md](./lite-recipes.md)。下列专用规则与消歧请在命中邮件场景且**超出**上述 lite 时阅读本文。
> 产品命令见 [mail.md](./mail.md)。通用批量/并行见 [recipes/conventions.md](recipes/conventions.md)。

## 专用规则（#9 非 lite 步骤必守）

- **KQL 语法强制**：邮件搜索的查询条件**只能**通过 `--query` 参数以 KQL 语法传入（如 `subject:周报`），多条件必须写显式 `AND`（如 `hasAttachments:true AND folderId:2`）；**禁止臆造** `--subject`、`--sender`、`--from-address` 等不存在的 flag。详见 [mail.md](./mail.md) 中 KQL 查询字段说明。
- **邮箱地址前置**：大部分邮件命令需要 `--email` 或 `--from` 参数，执行前**必须**先通过 `mail mailbox list` 获取当前用户邮箱，禁止猜测邮箱地址。
- **查找他人邮箱**：需要获取某人邮箱地址时，**不要用 `mailbox list`**（只返回自己的），必须走三路并发查询流程（见 [mail.md](./mail.md) 中「查找他人邮箱地址」章节）。
- **附件下载三步走**：先 `message search` 搜索邮件获取 messageId，再 `attachment list` 取附件 ID 和文件名，最后逐个 `attachment download`。用户只要首个/任一单附件时，初始最多检查 3 个相关候选；均未命中只能说明已检查范围并询问是否继续，不能断言不存在。用户要求全部/批量附件时，必须沿搜索游标遍历全部匹配页和邮件，再逐个下载每个附件。**不存在 `download_batch` / `download_all` 等批量下载命令，禁止编造**。
- **危险操作确认**：`batch-delete` 执行前必须向用户确认，同意后加 `--yes`。

## 与其他场景消歧

- **"给某人发邮件"**（只知姓名不知邮箱）→ 先走「查找他人邮箱地址」三路并发，再 `mail-send`。
- **"找某人邮箱"**（终点是获取邮箱地址）→ 三路并发查询，不走 `mail-search`。
- **"搜某人发的邮件"**（终点是邮件内容）→ `mail-search`，KQL 用 `from:xxx`。
- **"催+邮件"** → `mail-send` 发催促邮件，不是 #1 消息。
- **"邮件+待办"** → 先 `mail-search` 找邮件内容，再走 #2 创建待办。

## Recipe 速查（本表步骤，非 SKILL lite）

| Recipe | 步骤 |
|--------|------|
| `mail-get` | `mail message get --email <邮箱> --id <messageId>` → 查看邮件完整内容（含正文） |
| `mail-folder-list` | `mail folder list --email <邮箱>` → 列举文件夹；`--folder <id>` 查子文件夹 |
| `mail-tag-list` | `mail tag list --email <邮箱>` → 列举邮件标签 |
| `mail-thread-get` | `mail thread get --email <邮箱> --id <conversationId>` → 获取会话（邮件线程）详情 |
| `mail-attachment-list` | `mail attachment list --email <邮箱> --id <messageId>` → 列举指定邮件的附件 |
| `mail-attachment-download` | 1. `mail attachment list --email <邮箱> --id <messageId>` → 取附件 `id` 和 `name`<br>2. `mail attachment download --email <邮箱> --message-id <messageId> --attachment-id <attachmentId> --name <文件名>` |
| `mail-batch-move` | `mail message batch-move --email <邮箱> --ids <id1,id2,...> --folder <folderId>`（常用 folderId: 2=收件箱, 6=已删除） |
| `mail-batch-delete` | 确认前仅展示 `mail message batch-delete --email <邮箱> --ids <id1,id2,...>` 和精确目标并停止；明确确认后由执行流程对相同参数追加 `--yes` |
| `mail-draft-create` | `mail draft create --from <邮箱> --subject "<标题>"` → 取 `messageId`（可选 `--to`、`--content`、`--cc`） |
| `mail-draft-update` | `mail draft update --from <邮箱> --id <draftId> --subject "<新标题>"`（可选 `--content`、`--to`、`--cc`） |
| `mail-draft-send` | `mail draft send --from <邮箱> --id <draftId>` |

## Full / 多步组合

| Recipe | 行动指南（固定路线） |
|--------|---------------------|
| search-and-download-attachment | 1. `mail mailbox list` → 取邮箱<br>2. `mail message search --email <邮箱> --query "<KQL>" --limit 10`；多条件用显式 `AND`<br>3. 首个/任一单附件：按相关性最多检查 3 个候选，命中后进入第 4 步；未命中则说明范围并询问是否继续翻页<br>4. 全部/批量附件：沿 `nextCursor` 遍历全部匹配页，对每封执行 `attachment list`<br>5. 对命中附件逐个执行 `mail attachment download --email <邮箱> --message-id <messageId> --attachment-id <attachmentId> --name <文件名>`；单附件做一次本地检查后停止，全部/批量请求完成全部逐项下载（**不存在批量下载命令**） |
| search-reply-forward | 1. `mail mailbox list` → 取邮箱<br>2. `mail message search --email <邮箱> --query "<KQL>" --limit 10` → 取 `messageId`<br>3. 展示搜索结果供用户选择<br>4. 按用户指示执行 reply / reply-all / forward（参见 lite `mail-reply-forward`） |
| batch-mail-cleanup | 1. `mail mailbox list` → 取邮箱<br>2. `mail message search --email <邮箱> --query "<KQL>" --limit 100` → 取多个 `messageId`<br>3. 展示列表供用户确认<br>4. `mail message batch-move --email <邮箱> --ids <id1,id2,...> --folder 6 ` 移到已删除；或 `batch-delete` 永久删除 |
| send-to-person-by-name | 1. `mail mailbox list` → 取发件邮箱<br>2. 走「查找他人邮箱地址」三路并发查询获取收件人邮箱（见 [mail.md](./mail.md)）<br>3. `mail message send --from <发件邮箱> --to <收件邮箱> --subject "<标题>" --content "<内容>"` |
