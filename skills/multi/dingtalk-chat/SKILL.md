---
name: dingtalk-chat
description: 钉钉群聊与消息。Use when 用户提到 发消息/单聊/群聊/建群/拉人进群/改群名/搜索群/群成员管理/@消息/收藏消息/撤回消息/机器人群发/Webhook通知/发图片或文件到群。Distinct from dingtalk-ding(紧急DING消息/短信/电话)、dingtalk-mail(邮件)、dingtalk-edu-group(班级群)。命令前缀：dws chat。
cli_version: ">=0.2.14"
metadata:
  category: product
  stability: experimental
  requires:
    bins:
      - dws
---

# 钉钉群聊 / 消息 Skill

> 🧪 **EXPERIMENTAL · 试验版 / Preview** — multi 模式当前未达 stable 标准。22 个 dingtalk-* skill 全部通过 dispatch verifier，但接口、命名、跨 skill 引用后续可能调整；生产 / 共享环境请优先使用 mono 模式（`dws skill setup --mode mono`）。问题请提 issue 反馈。

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> ⚠️ **命令可用性以当前 dws 二进制为准**。服务发现已下线，本文档随内置 skill 发布；如果 `dws <cmd> --help` 不存在，说明当前版本未暴露该命令。若命令存在但调用失败，请按错误中的 endpoint 或 tool 提示确认静态端点目录和后端工具注册。实际调用前可用 `dws <cmd> --help` 或 `--dry-run` 验证。


> 命令参考：[chat.md](references/chat.md)；表情：[chat-emoji-list.md](references/chat-emoji-list.md)；剧本：[01-messaging.md](references/01-messaging.md)。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "发消息给张三" | `dws chat message send --open-dingtalk-id <id> --title "<标题>" --text "<内容>"` |
| "发到XX群" | `dws chat search --query "<群名>"` → `dws chat message send --group <openConversationId> --title "<标题>" --text "<内容>"` |
| "建群" / "拉人进群" | `dws chat group create` / `dws chat group members add` |
| "改群名" / "踢人" | `dws chat group rename` / `dws chat group members remove --yes`（踢人不可逆，确认目标后加 --yes；踢群主会被 CLI 拦截，需先 `transfer-owner`）|
| "@我消息" | `dws chat message list-mentions` |
| "查群聊记录" | `dws chat message list` |
| "收藏/取消收藏这条消息" | `dws chat message add-favorite` / `dws chat message remove-favorite`（均需 `openMessageId` 和 `openConversationId`）|
| "查看我收藏的消息" | `dws chat message list-favorites`（默认 `--cursor 0 --size 20`）|
| "用机器人发消息" | `dws chat message send-by-bot --robot-code <code> --group <id> --title "<标题>" --text "<内容>"` |
| "Webhook 推一条" | `dws chat message send-by-webhook --token <token> --title "<标题>" --text "<内容>"` |
| "撤回我发的消息" | `dws chat message recall`（撤回当前用户发送的消息）|
| "撤回机器人消息" | `dws chat message recall-by-bot --robot-code <code> --group <openConversationId> --keys <processQueryKey>`（撤回机器人发的）|

> **注**：`chat message send` 的 `--title` 可选（不传时用正文首行作标题）；`send-by-bot` / `send-by-webhook` 的 `--title` 必填。

## 跨产品协作

- 收件人是人名 → 先用 `dingtalk-contact` 或 `dingtalk-aisearch` 拿 `openDingTalkId` / `userId`
- 要发图片/文件 → 先 `dt_media_upload` 上传 → `python scripts/extract_media_id.py "<URL>"` 提取 mediaId → 再用 `--media-id`
- 紧急升级（应用内/短信/电话）→ 切到 `dingtalk-ding`
- 发邮件 → 切到 `dingtalk-mail`
