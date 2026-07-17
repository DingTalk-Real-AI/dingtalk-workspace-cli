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

> 🧪 **EXPERIMENTAL · 试验版 / Preview** — multi 模式当前未达 stable 标准。全部 dingtalk-* skill 已通过 dispatch verifier，但接口、命名、跨 skill 引用后续可能调整；生产 / 共享环境请优先使用 mono 模式（`dws skill setup --mode mono`）。问题请提 issue 反馈。

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> ⚠️ **命令可用性以当前 dws 二进制为准**。服务发现已下线，本文档随内置 skill 发布；如果 `dws <cmd> --help` 不存在，说明当前版本未暴露该命令。若命令存在但调用失败，请按错误中的 endpoint 或 tool 提示确认静态端点目录和后端工具注册。实际调用前可用 `dws <cmd> --help` 或 `--dry-run` 验证。


> 命令参考：[chat.md](references/chat.md)；表情：[chat-emoji-list.md](references/chat-emoji-list.md)；剧本：[01-messaging.md](references/01-messaging.md)。

<!-- VISIBLE_SHORTCUTS_START -->
## Shortcuts（无专用脚本/recipe 时优先）

以下 shortcut 来自独立于 Runtime Schema 的公开 catalog。先按本 skill 的意图表、脚本和 recipe 路由：存在精确覆盖该场景的专用脚本/recipe 时按其执行；否则用户意图命中时，shortcut 优先于手写原子命令。用 `dws shortcut list --service chat --format json` 读取参数、约束、风险和示例，并以 `dws chat <shortcut> --help` 核对当前 Cobra flags；不要对 `+` 路径调用 `dws schema`。

| Shortcut | 风险 | 适用场景 |
|---|---|---|
| `dws chat +at-me` | read | 查最近 @我 的消息（自动算时间窗，投影发送人/时间/内容/会话） |
| `dws chat +bot-find` | read | 搜索全部可用机器人（含他人/官方，返回 openDingTalkId 可发单聊） |
| `dws chat +bot-search` | read | 搜索当前用户自己创建的机器人 |
| `dws chat +broadcast` | write | 按姓名逐一给多个人群发同一条单聊消息（自动解析 userId、逐个发送） |
| `dws chat +category-create` | write | 创建用户自定义会话分组 |
| `dws chat +category-delete` | high-risk-write | 删除用户自定义会话分组 |
| `dws chat +category-list` | read | 获取用户自定义会话分组 |
| `dws chat +category-rename` | write | 更新用户自定义会话分组的名称 |
| `dws chat +chat-bots` | read | 查看群内所有机器人 |
| `dws chat +chat-dismiss` | high-risk-write | 解散群聊（不可逆，需群主权限） |
| `dws chat +chat-invite-url` | read | 获取群邀请链接 |
| `dws chat +chat-list-all` | read | 分页拉取我加入的所有群列表 |
| `dws chat +chat-list-join-requests` | read | 分页拉取入群验证记录 |
| `dws chat +chat-list-mine` | read | 拉取我创建/管理的群 |
| `dws chat +chat-mute` | write | 全员禁言 / 取消全员禁言 |
| `dws chat +chat-role-add` | write | 添加群身份 |
| `dws chat +chat-role-list` | read | 拉取会话的群身份列表 |
| `dws chat +chat-role-query-user` | read | 查询群成员的群身份 |
| `dws chat +chat-role-set-user` | write | 设置用户的群身份（覆盖该用户的全部群身份） |
| `dws chat +chat-role-update` | write | 更新群身份名称 |
| `dws chat +chat-search` | read | 按关键词搜索群聊 |
| `dws chat +chat-set-admin` | write | 设置 / 取消群管理员 |
| `dws chat +chat-set-history` | write | 设置新成员入群可查看历史消息范围 |
| `dws chat +chat-update-alias` | write | 设置群备注（仅自己可见） |
| `dws chat +chat-update-nick` | write | 设置当前用户在群内的群昵称 |
| `dws chat +conversation-clear-all-red-point` | write | 清除所有会话红点（全部已读） |
| `dws chat +conversation-info` | read | 获取会话信息（群聊传 --group，单聊传 --open-dingtalk-id） |
| `dws chat +conversation-list` | read | 分页获取当前用户的全部会话列表（单聊+群聊） |
| `dws chat +conversation-list-top` | read | 拉取置顶会话列表 |
| `dws chat +dm` | write | 按姓名直接给某人发单聊消息（自动解析 userId） |
| `dws chat +group-members` | read | 按群名列出群成员（自动搜群解析 openConversationId） |
| `dws chat +messages-list-direct` | read | 拉取单聊会话消息 |
| `dws chat +messages-list-pin` | read | 拉取会话中钉住的消息列表 |
| `dws chat +messages-list-unread-conversations` | read | 获取有未读消息的会话列表 |
| `dws chat +messages-mget` | read | 根据消息 ID 批量查询消息（最多 50 条） |
| `dws chat +messages-query-send-status` | read | 查询消息发送状态 |
| `dws chat +messages-read-status` | read | 查询消息的已读/未读状态 |
| `dws chat +messages-send-by-webhook` | write | 自定义机器人 Webhook 发送群消息 |
| `dws chat +messages-update-card` | write | 流式更新卡片内容（最后一次 --flow-status 应为 3） |
| `dws chat +my-groups` | read | 列出我加入的群，可按类型过滤并投影关键字段 |
| `dws chat +send-to-group` | write | 按群名直接给群发消息（自动搜群解析 openConversationId） |
| `dws chat +unread-chats` | read | 列出我有未读消息的会话（投影会话名/未读数/会话ID） |
<!-- VISIBLE_SHORTCUTS_END -->

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
