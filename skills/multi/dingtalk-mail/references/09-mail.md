# 邮件

> **SKILL.md** 中 #9 内联 4 条 **lite**：`mail-list-mailbox`、`mail-search`、`mail-send`、`mail-reply-forward`，见 `dws-shared/references/best_practices/_common/lite-recipes.md`。下列 recipe、专用规则与消歧请在命中 #9 且**超出**上述 lite 时阅读本文。
> 产品命令见 [mail.md](./mail.md)。通用批量/并行见 `dws-shared/references/best_practices/_common/conventions.md`。

> **能力范围**：当前 `dws mail` 只实现 `mailbox list` 与 `message search / get / send` 四条命令；其余 `reply` / `reply-all` / `forward` / `batch-*` / `draft *` / `folder *` / `attachment *` / `tag *` / `thread *` / `user search` 等命令**均未实现**，禁止编造。上面的 `mail-reply-forward` lite 触发场景（"回复邮件 / 转发邮件"）需要回退到引导用户在钉钉客户端手动操作。

## 专用规则（#9 非 lite 步骤必守）

- **KQL 语法强制**：邮件搜索的查询条件**只能**通过 `--query` 参数以 KQL 语法传入（如 `subject:周报`），**禁止臆造** `--subject`、`--sender`、`--from-address` 等不存在的 flag。详见 [mail.md](./mail.md) 中 KQL 查询字段说明。
- **邮箱地址前置**：所有邮件命令需要 `--email` 或 `--from` 参数，执行前**必须**先通过 `mail mailbox list` 获取当前用户邮箱（返回字段 `emailAccounts`），禁止猜测邮箱地址。
- **查找他人邮箱**：需要获取某人邮箱地址时，**不要用 `mailbox list`**（只返回自己的），必须走两路并发查询流程（见 [mail.md](./mail.md) 中「查找他人邮箱地址」章节）。
- **未实现命令禁止编造**：回复 / 转发 / 批量移动 / 批量删除 / 草稿 / 文件夹 / 附件 / 标签 / 会话线程 / mail user search 等命令当前都没实现，遇到相关诉求要么走 search/get/send 能覆盖的子集，要么向用户说明并引导到钉钉客户端。

## 与其他场景消歧

- **"给某人发邮件"**（只知姓名不知邮箱）→ 先走「查找他人邮箱地址」两路并发，再 `mail-send`。
- **"找某人邮箱"**（终点是获取邮箱地址）→ 两路并发查询，不走 `mail-search`。
- **"搜某人发的邮件"**（终点是邮件内容）→ `mail-search`，KQL 用 `from:xxx`。
- **"催+邮件"** → `mail-send` 发催促邮件，不是 #1 消息。
- **"邮件+待办"** → 先 `mail-search` 找邮件内容，再走 #2 创建待办。
- **"回复 / 转发 / 删除 / 移动 / 草稿 / 附件 / 标签 / 会话线程"** → 当前 CLI 不支持，引导用户在钉钉客户端处理，不要编造命令。

## Recipe 速查（本表步骤，非 SKILL lite）

| Recipe | 步骤 |
|--------|------|
| `mail-get` | `mail message get --email <邮箱> --id <messageId>` → 查看邮件完整内容（含 `message.markdownBody` 正文） |

## Full / 多步组合

| Recipe | 行动指南（固定路线） |
|--------|---------------------|
| send-to-person-by-name | 1. `mail mailbox list` → 从 `emailAccounts` 取发件邮箱<br>2. 走「查找他人邮箱地址」两路并发查询获取收件人邮箱（见 [mail.md](./mail.md)）<br>3. `mail message send --from <发件邮箱> --to <收件邮箱> --subject "<标题>" --content "<内容>"` |
| search-then-read | 1. `mail mailbox list` → 取邮箱<br>2. `mail message search --email <邮箱> --query "<KQL>" --limit 20` → 取 `messageId`<br>3. 对感兴趣的一封执行 `mail message get --email <邮箱> --id <messageId>` 读取正文 |
