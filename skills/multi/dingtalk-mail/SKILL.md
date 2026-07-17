---
name: dingtalk-mail
description: 钉钉邮箱读写、搜索、回复与转发。Use when 用户说发邮件/查邮件/回邮件/转发邮件/未读邮件/邮件搜索/邮箱附件。一句话解析联系人并确认发送邮件走配方 dingtalk-one-click-email；不做钉钉消息（走 dingtalk-chat）、紧急通知（走 dingtalk-ding）。命令前缀：dws mail。
cli_version: ">=0.2.14"
metadata:
  category: product
  stability: experimental
  requires:
    bins:
      - dws
---

# 钉钉邮箱 Skill

> 🧪 **EXPERIMENTAL · 试验版 / Preview** — multi 模式当前未达 stable 标准。全部 dingtalk-* skill 已通过 dispatch verifier，但接口、命名、跨 skill 引用后续可能调整；生产 / 共享环境请优先使用 mono 模式（`dws skill setup --mode mono`）。问题请提 issue 反馈。

## 前置条件 — 执行操作前必读

> **`use_skill(dws-shared)`** — 认证、全局参数（`--format json` / `--yes`）、错误码、URL 模板、跨产品消歧、安全规则与 capability 边界。**执行任何 `dws` 命令前先读；** 单产品的清晰命令可直接用本 skill。

<!-- SAFETY_PREAMBLE_INJECT -->

> 渐进式参考：[mail-index.md](references/mail-index.md)。复杂搜索、附件、批量处理、草稿等多步邮件场景参考：[09-mail.md](references/09-mail.md)。先按索引选择专题。

> 旧路径兼容入口：[mail.md](references/mail.md)。

<!-- VISIBLE_SHORTCUTS_START -->
## Shortcuts（无专用脚本/recipe 时优先）

以下 shortcut 来自独立于 Runtime Schema 的公开 catalog。先按本 skill 的意图表、脚本和 recipe 路由：存在精确覆盖该场景的专用脚本/recipe 时按其执行；否则用户意图命中时，shortcut 优先于手写原子命令。用 `dws shortcut list --service mail --format json` 读取参数、约束、风险和示例，并以 `dws mail <shortcut> --help` 核对当前 Cobra flags；不要对 `+` 路径调用 `dws schema`。

| Shortcut | 风险 | 适用场景 |
|---|---|---|
| `dws mail +contact-list` | read | 列出指定邮箱的所有邮件联系人 |
| `dws mail +find-mail-user` | read | 按关键词搜索邮箱联系人并投影列表（姓名/昵称/邮箱/工号等） |
| `dws mail +folder-list` | read | 列出顶层文件夹或指定父文件夹下的子文件夹 |
| `dws mail +recent-mail` | read | 列出收件箱近期邮件会话并投影列表（主题/发件人/时间/threadId） |
| `dws mail +search-mail` | read | 按 KQL 关键词搜索邮件并投影列表（主题/发件人/时间/messageId） |
| `dws mail +tag-list` | read | 列出指定邮箱下的所有邮件标签 |
| `dws mail +template-list` | read | 列出指定邮箱的所有邮件模板 |
| `dws mail +thread-list` | read | 列出指定邮箱文件夹下的邮件会话（thread） |
| `dws mail +unread-mail` | read | 列出未读邮件并投影列表（主题/发件人/时间/messageId） |
| `dws mail +user-search` | read | 按关键词或工号搜索邮箱用户（仅企业邮箱） |
<!-- VISIBLE_SHORTCUTS_END -->

## 意图表

| 用户说 | 命令 |
|--------|------|
| "发邮件给 a@b.com" | `dws mail mailbox list --format json` → `dws mail message send --from <邮箱> --to a@b.com --subject "<标题>" --content "<正文>" --format json` |
| "回复 / 全部回复 / 转发" | `dws mail message reply` / `reply-all` / `forward` |
| "今天未读邮件" | `python scripts/mail_unread_summary.py` |
| "带抄送发送" | `python scripts/mail_send_with_cc.py --to a@b.com --cc c@d.com --subject "<标题>" --body "<正文>"` |

## 标准 SOP（必遵流程）

> 命中以下意图**必须**按对应 SOP 顺序执行；**禁止**跳步、替换命令、编造 email/messageId。每条命令必须带 `--format json`。收件邮箱**必须**真实解析，**禁止**猜测。

### SOP-1 拿邮箱（get-mailbox）

**触发**：我的邮箱/发件需要邮箱/查邮件需要邮箱。

1. **执行（必须）**：`dws mail mailbox list --format json`，取自己的 `email`（默认选企业邮箱 `type:ORG`）；查**他人**邮箱用 `dws mail user search --keyword "<姓名>" --format json`，**禁止**用 `mailbox list` 查他人。

**禁止**：把 `mailbox list` 当作他人邮箱查询、猜测邮箱地址。

### SOP-2 查 / 搜邮件（search-mail）

**触发**：查邮件/搜邮件/某主题邮件/某人发的邮件。

1. **拿邮箱（必须）**：先按 SOP-1 取 `email`；用户已明确提供可跳过。
2. **执行（必须）**：浏览文件夹 `dws mail message list --email <邮箱> --limit <n> --format json`；KQL 搜索 `dws mail message search --email <邮箱> --query "<KQL>" --limit 20 --format json`（KQL 如 `subject:周报`、`from:alice@x.com`、`folderId:2`、`hasAttachments:true`，**只通过 `--query` 传**）。
3. **取正文（必须）**：`dws mail message get --email <邮箱> --id <messageId> --format json`；`messageId` 从列表/搜索结果取，**禁止**编造。

**禁止**：把 KQL 拆成多个 flag、跳过 `message list/search` 直接猜 messageId。

### SOP-3 发邮件（send-mail）

**触发**：发邮件/写邮件/群发。

1. **发件邮箱（必须）**：`dws mail mailbox list` 取自己邮箱。
2. **收件邮箱（必须）**：地址直接用；姓名按 [mail-message-commands.md](references/mail-message-commands.md) “查找他人邮箱地址”流程（`mail user search` 等）获取，**禁止**猜测。
3. **执行（必须）**：`dws mail message send --from <发件邮箱> --to <收件邮箱> --subject "<主题>" --content "<正文>" --format json`；按需 `--cc`/`--attachment`/`--inline-attachment`。
4. **验证（必须）**：从返回取 `internetMessageId`，可 `dws mail message verify --email <发件邮箱> --internet-message-id <internetMessageId> --format json` 查发送状态。

**完成条件**：收件邮箱来自真实解析结果，且发送返回或 `message verify` 显示成功。

### SOP-4 回复 / 转发（reply-forward）

**触发**：回复邮件/回复全部/转发。

1. **拿邮箱 + 原邮件（必须）**：SOP-1 取邮箱；用户未给 `messageId` 时**必须**先走 SOP-2 定位原邮件 `messageId`。
2. **执行（必须）**：回复 `dws mail message reply --from <邮箱> --id <messageId> --content "<正文>" --format json`；回复全部用 `reply-all`；转发 `dws mail message forward --from <邮箱> --to <收件邮箱> --id <messageId> --content "<附言>" --format json`。

**禁止**：未定位原邮件就回复/转发、编造 messageId。

## 高频硬约束

- 用户要“完整内容 / 看看这封邮件 / 正文”时，`message search` 只负责定位 messageId，随后用 `dws mail message get --email <邮箱> --id <messageId> --format json` 获取正文。
- 搜到多封邮件时，若用户给了明确主题、附件名、发件人或时间线索，先选最匹配的一封执行 `message get`；只有同等候选无法判断时才询问用户。
- 附件链路固定三步：`message search` → `attachment list --email <邮箱> --id <messageId>` → `attachment download --email <邮箱> --message-id <messageId> --attachment-id <attachmentId> --name <文件名>`；不存在批量下载命令。
- 写入类操作（发送、回复、转发、删除、批量移动）按安全策略确认；只读查看、搜索、附件列表、下载不需要确认。
- 所有 `dws mail` 命令加 `--format json`，并复用同一封邮件的 `messageId`，不要重新搜索导致目标漂移。

## 跨产品协作

- 收件人是人名 → 先用 `dingtalk-contact` 取 `orgAuthEmail`
- 钉钉内消息 → 切到 `dingtalk-chat`
## 局部意图与 Recipe

- [局部意图消歧](references/intent-guide.md)；[Lite Recipe](references/lite-recipes.md)。
