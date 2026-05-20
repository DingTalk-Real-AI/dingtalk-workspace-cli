---
name: dingtalk-mail
description: 钉钉邮箱。Use when 用户说 发邮件/查邮件/回邮件/转发邮件/未读邮件/邮件搜索。Distinct from dingtalk-chat(钉钉消息)、dingtalk-ding(紧急通知)。命令前缀：dws mail。
cli_version: ">=0.2.14"
metadata:
  category: product
  requires:
    bins:
      - dws
---

# 钉钉邮箱 Skill

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> 命令参考：[mail.md](references/mail.md)。复杂搜索、附件、批量处理、草稿等多步邮件场景参考：[09-mail.md](references/09-mail.md)。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "发邮件给 a@b.com" | `dws mail message send --from <自己邮箱> --to a@b.com --subject "<标题>" --body "<正文>"` |
| "今天未读邮件" | `python scripts/mail_unread_summary.py` |
| "带抄送发送" | `python scripts/mail_send_with_cc.py --to a@b.com --cc c@d.com --subject "<标题>" --body "<正文>"` |

## 评测高频硬约束

- 用户要"完整内容/看看这封邮件/正文"时，`message search` 命中后必须继续调用 `dws mail message get --email <邮箱> --id <messageId> --format json`；不要只列候选后停下。
- 搜到多封邮件时，若用户给了明确主题、附件名、发件人或时间线索，先选最匹配的一封执行 `message get`；只有同等候选无法判断时才询问用户。
- 写入类操作（发送）按安全策略确认；只读查看、搜索不需要确认。
- 所有 `dws mail` 命令加 `--format json`，并复用同一封邮件的 `messageId`，不要重新搜索导致目标漂移。

## 跨产品协作

- 收件人是人名 → 先用 `dingtalk-contact` 取 `orgAuthEmail`
- 钉钉内消息 → 切到 `dingtalk-chat`
