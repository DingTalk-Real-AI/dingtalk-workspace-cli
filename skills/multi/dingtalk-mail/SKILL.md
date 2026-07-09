---
name: dingtalk-mail
description: 钉钉邮箱。Use when 用户说 发邮件/查邮件/回邮件/转发邮件/未读邮件/邮件搜索。Distinct from dingtalk-chat(钉钉消息)、dingtalk-ding(紧急通知)。命令前缀：dws mail。
cli_version: ">=0.2.14"
metadata:
  category: product
  stability: experimental
  requires:
    bins:
      - dws
---

# 钉钉邮箱 Skill

> 🧪 **EXPERIMENTAL · 试验版 / Preview** — multi 模式当前未达 stable 标准。22 个 dingtalk-* skill 全部通过 dispatch verifier，但接口、命名、跨 skill 引用后续可能调整；生产 / 共享环境请优先使用 mono 模式（`dws skill setup --mode mono`）。问题请提 issue 反馈。

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> ⚠️ **命令可用性以当前 dws 二进制为准**。服务发现已下线，本文档随内置 skill 发布；如果 `dws <cmd> --help` 不存在，说明当前版本未暴露该命令。若命令存在但调用失败，请按错误中的 endpoint 或 tool 提示确认静态端点目录和后端工具注册。实际调用前可用 `dws <cmd> --help` 或 `--dry-run` 验证。


> 命令参考：[mail.md](references/mail.md)。复杂搜索、附件、批量处理、草稿等多步邮件场景参考：[09-mail.md](references/09-mail.md)。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "发邮件给 a@b.com" | `dws mail message send --from <自己邮箱> --to a@b.com --subject "<标题>" --content "<正文>"`（正文规范 flag 是 `--content`，`--body` 为隐藏别名） |
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
