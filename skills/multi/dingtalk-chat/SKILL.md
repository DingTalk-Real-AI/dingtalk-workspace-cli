---
name: dingtalk-chat
description: 钉钉群聊与消息。Use when 用户提到 发消息/单聊/群聊/建群/拉人进群/改群名/搜索群/群成员管理/@消息/撤回消息/机器人群发/Webhook通知/发图片或文件到群/搜机器人/查我的机器人/创建机器人/给agent接入钉钉机器人/接入OpenClaw。Distinct from dingtalk-ding(紧急DING消息/短信/电话)、dingtalk-mail(邮件)、dingtalk-edu-group(班级群)。命令前缀：dws chat。
cli_version: ">=0.2.14"
metadata:
  category: product
  stability: experimental
  requires:
    bins:
      - dws
---

# 钉钉群聊 / 消息 Skill

> 🧪 **EXPERIMENTAL · 试验版 / Preview** — multi 模式当前未达 stable 标准。20 个 dingtalk-* skill 全部通过 dispatch verifier，但接口、命名、跨 skill 引用后续可能调整；生产 / 共享环境请优先使用 mono 模式（`dws skill setup --mode mono`）。问题请提 issue 反馈。

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> ⚠️ **命令可用性可能因企业服务发现配置而异**。本文档列出的命令基于 dws envelope schema 与本仓库 v1.0.30 实测，但部分命令的 cobra 子命令暴露与否还取决于你的企业 MCP gateway 是否注册了对应 tool。如果跑某条命令报 `unknown command` 或 fall back 到父级 help，说明当前账号企业未开通该能力。实际调用前可用 `dws <cmd> --help` 或 `--dry-run` 验证。


> 命令参考：[chat.md](references/chat.md)；表情：[chat-emoji-list.md](references/chat-emoji-list.md)；剧本：[01-messaging.md](references/01-messaging.md)。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "发消息给张三" | `dws chat message send --open-dingtalk-id <id> --title "<标题>" --text "<内容>"` |
| "发到XX群" | `dws chat search --query "<群名>"` → `dws chat message send --group <openConversationId> --title "<标题>" --text "<内容>"` |
| "建群" / "拉人进群" | `dws chat group create` / `dws chat group members add` |
| "改群名" / "踢人" | `dws chat group rename` / `dws chat group members remove --yes`（踢人不可逆，确认目标后加 --yes）|
| "@我消息" / "查群聊记录" | `dws chat message list` |
| "用机器人发消息" | `dws chat message send-by-bot --robot-code <code> --group <id> --title "<标题>" --text "<内容>"` |
| "Webhook 推一条" | `dws chat message send-by-webhook --token <token> --title "<标题>" --text "<内容>"` |
| "撤回机器人消息" | `dws chat message recall-by-bot --robot-code <code> --group <openConversationId> --keys <processQueryKey>`（只能撤回机器人发的；撤回普通用户消息开源 dws v1.0.30 暂不支持）|
| "搜机器人" / "查我创建的机器人" | `dws chat bot find --query "<关键词>"`（全部可用，带 openDingTalkId）/ `dws chat bot search`（仅我创建的）|
| "给我建个机器人" / "给 agent 接入钉钉" / "接入 OpenClaw" | dws 无原生创建命令，走 OpenClaw 连接器：`npx -y @dingtalk-real-ai/dingtalk-connector install` → 扫码一键建号 → `dws chat bot search` 交叉验证。固定路线见 [chat.md](references/chat.md) 「创建【新】机器人」节 |

> **注**：v1.0.30 起 `chat message send / send-by-bot / send-by-webhook` 全部强制 `--title` 必填（单聊群聊都要）。

## 跨产品协作

- 收件人是人名 → 先用 `dingtalk-contact` 或 `dingtalk-aisearch` 拿 `openDingTalkId` / `userId`
- 要发图片/文件 → 先 `dt_media_upload` 上传 → `python scripts/extract_media_id.py "<URL>"` 提取 mediaId → 再用 `--media-id`
- 紧急升级（应用内/短信/电话）→ 切到 `dingtalk-ding`
- 发邮件 → 切到 `dingtalk-mail`
- 要**新建**机器人 / 给 agent 接入钉钉 → dws 只能搜不能建，交给官方连接器 `@dingtalk-real-ai/dingtalk-connector`（见 [chat.md](references/chat.md) 「创建【新】机器人」节），建完用 `dws chat bot search` 交叉验证
