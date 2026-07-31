---
name: dingtalk-chat
description: 钉钉群聊与消息。Use when 用户提到 发消息/单聊/群聊/建群/拉人进群/改群名/搜索群/群成员管理/@消息/撤回消息/机器人群发/Webhook通知/发图片或文件到群/标记未读/清除红点/置顶消息/全部群列表。不做紧急 DING/短信/电话（走 dingtalk-ding）、邮件（走 dingtalk-mail）、班级群（走 dingtalk-misc）。命令前缀：dws chat。
metadata:
  cli_version: ">=0.2.14"
  category: product
  requires:
    bins:
      - dws
---

# 钉钉群聊 / 消息 Skill

## Preconditions

> **CRITICAL — Before any `dws` operation, MUST fully read [`dws-shared`](../dws-shared/SKILL.md).** It defines the global execution contract, safety floor, and on-demand shared-reference routing. Do not preload all of its references.

> Command reference: [chat.md](references/chat.md); emoji list: [chat-emoji-list.md](references/chat-emoji-list.md); workflows: [01-messaging.md](references/01-messaging.md).

<!-- VISIBLE_SHORTCUTS_START -->
## Shortcuts（无专用脚本/recipe 时优先）

Complete public catalog index; every command uses the `dws chat` prefix. Each row keeps a Chinese intent cue. Once selected, execute directly; read leaf Schema only when parameters, constraints, or safety are uncertain, and leaf Help only when flags are uncertain. See “Shortcut 执行契约” below.

| Shortcut | 适用场景 |
|---|---|
| `+at-me` | 查最近 @我；auto window, project sender/time/content/chat. |
| `+bot-find` | 搜索全部机器人；include others/official, return DM-ready openDingTalkId. |
| `+bot-search` | 搜索当前用户自己创建的机器人 |
| `+broadcast` | 多人单聊群发；resolve userId and send individually. |
| `+category-add-conversation` | 将会话移动到指定的自定义分组中 |
| `+category-create` | 创建用户自定义会话分组 |
| `+category-delete` | 删除用户自定义会话分组 |
| `+category-list` | 获取用户自定义会话分组 |
| `+category-list-conversations` | 拉取指定自定义会话分组下的会话 |
| `+category-remove-conversation` | 将会话从指定的自定义分组中移出 |
| `+category-rename` | 更新用户自定义会话分组的名称 |
| `+chat-add-bot` | 将机器人添加到群中 |
| `+chat-audit-join` | 审批入群验证；approve/reject/delete/ignore/block. |
| `+chat-bots` | 查看群内所有机器人 |
| `+chat-create` | 以当前用户身份创建钉钉群聊 |
| `+chat-dismiss` | 解散群聊（不可逆，需群主权限） |
| `+chat-get-by-id` | 根据群号获取群聊信息 |
| `+chat-invite-url` | 获取群邀请链接 |
| `+chat-list-all` | 分页拉取我加入的所有群列表 |
| `+chat-list-join-requests` | 分页拉取入群验证记录 |
| `+chat-list-mine` | 拉取我创建/管理的群 |
| `+chat-members-get` | 根据成员 openDingTalkId 批量查询群成员详情 |
| `+chat-members-list` | 列出群成员；group users/bots, resolve chat name. |
| `+chat-messages` | 拉取会话消息；group/DM, project sender/text/time. |
| `+chat-mute` | 全员禁言 / 取消全员禁言 |
| `+chat-mute-member` | 指定群成员禁言 / 取消禁言 |
| `+chat-quit` | 退出群聊 |
| `+chat-remove-bot` | 从群内移除机器人 |
| `+chat-role-add` | 添加群身份 |
| `+chat-role-list` | 拉取会话的群身份列表 |
| `+chat-role-query-user` | 查询群成员的群身份 |
| `+chat-role-remove` | 删除群身份 |
| `+chat-role-remove-user` | 移除用户的指定群身份 |
| `+chat-role-set-user` | 设置用户的群身份（覆盖该用户的全部群身份） |
| `+chat-role-update` | 更新群身份名称 |
| `+chat-search` | 按关键词搜索群聊 |
| `+chat-set-admin` | 设置 / 取消群管理员 |
| `+chat-set-history` | 设置新成员入群可查看历史消息范围 |
| `+chat-transfer-owner` | 转让群主 |
| `+chat-update` | 更新群名称（仅名称，不支持 description） |
| `+chat-update-alias` | 设置群备注（仅自己可见） |
| `+chat-update-icon` | 更新群头像 |
| `+chat-update-nick` | 设置当前用户在群内的群昵称 |
| `+chat-update-settings` | 更新群设置（settingKey + status） |
| `+conversation-clear-all-red-point` | 清除所有会话红点（全部已读） |
| `+conversation-clear-messages` | 清空本人会话记录；current-user view only, irreversible. |
| `+conversation-clear-red-point` | 清除会话红点 |
| `+conversation-hide` | 会话列表中隐藏会话（收到新消息会重新出现） |
| `+conversation-info` | 获取会话信息（群聊传 --group，单聊传 --open-dingtalk-id） |
| `+conversation-list` | 分页获取当前用户的全部会话列表（单聊+群聊） |
| `+conversation-list-top` | 拉取置顶会话列表，可只看群聊或单聊 |
| `+conversation-mark-read` | 标记消息已读；includes all earlier messages. |
| `+conversation-mark-unread` | 标记会话为未读 |
| `+conversation-mute` | 会话消息免打扰（支持单聊/群聊） |
| `+conversation-set-top` | 批量置顶/取消；max 10 chats. |
| `+dm` | 按姓名直接给某人发单聊消息（自动解析 userId） |
| `+feed-group-query-item` | 在会话分组结果中按会话 ID 精确查询多项 |
| `+flag-cancel` | 取消收藏一条或多条消息（最多 10 条） |
| `+flag-create` | 收藏一条或多条消息（最多 10 条） |
| `+flag-list` | 分页查询当前用户收藏的消息 |
| `+group-members` | 按群名列出群成员（自动搜群解析 openConversationId） |
| `+messages-add-emoji` | 对消息添加 emoji 表情回应 |
| `+messages-add-text-emotion` | 对消息添加文字表情回应 |
| `+messages-batch-recall-by-bot` | 机器人撤回单聊消息 |
| `+messages-batch-send-by-bot` | 机器人批量向用户发送单聊 Markdown 消息 |
| `+messages-combine-forward` | 合并转发多条消息 |
| `+messages-create-text-emotion` | 创建文字表情（获取 emotionId） |
| `+messages-forward` | 转发单条消息 |
| `+messages-forward-topic` | 转发话题消息到目标会话 |
| `+messages-list` | 拉取群聊会话消息 |
| `+messages-list-direct` | 拉取单聊会话消息 |
| `+messages-list-pin` | 拉取会话中钉住的消息列表 |
| `+messages-list-unread-conversations` | 获取有未读消息的会话列表 |
| `+messages-mget` | 根据消息 ID 批量查询消息（最多 50 条） |
| `+messages-query-send-status` | 查询消息发送状态 |
| `+messages-read-status` | 查询消息的已读/未读状态 |
| `+messages-recall` | 撤回当前用户发送的消息 |
| `+messages-recall-by-bot` | 机器人撤回群消息 |
| `+messages-remove-emoji` | 移除消息的 emoji 表情回应 |
| `+messages-remove-text-emotion` | 移除消息的文字表情回应 |
| `+messages-reply` | 以当前用户身份引用回复消息（自动补全原发送者） |
| `+messages-resource-download` | 安全下载消息资源；image/video/audio/file. |
| `+messages-resource-url` | 获取消息资源（图片/视频/语音）下载链接 |
| `+messages-send` | 统一发送文本、Markdown、当前用户文件或已有 mediaId 图片 |
| `+messages-send-by-bot` | 机器人向群聊发送 Markdown 消息 |
| `+messages-send-by-webhook` | 自定义机器人 Webhook 发送群消息 |
| `+messages-send-card` | 创建流式卡片，可在同一次调用中写入内容并结束 |
| `+messages-set-pin` | 钉住消息（Pin） |
| `+messages-set-top` | 置顶消息 |
| `+messages-unset-pin` | 取消钉住消息（Unpin） |
| `+messages-unset-top` | 取消置顶消息 |
| `+messages-update-card` | 流式更新卡片；final `--flow-status` must be 3. |
| `+my-groups` | 列出我加入的群，可按类型过滤并投影关键字段 |
| `+search-msg` | 多维搜索消息，可全量翻页并批量富化详情 |
| `+send-to-group` | 按群名发消息；resolve openConversationId. |
| `+thread-replies` | 拉取话题全部回复；project sender/text/time. |
| `+unread-chats` | 列出未读会话；project name/count/chat ID. |

`risk=high-risk-write`（高风险）: `+category-delete`, `+chat-dismiss`, `+chat-remove-bot`, `+chat-role-remove`, `+conversation-clear-messages`. Confirmation follows leaf Schema `confirmation` and the runtime gate; never infer it from risk.
<!-- VISIBLE_SHORTCUTS_END -->

## Shortcut 执行契约

This is the only Shortcut entry point for multi/chat; `references/` documents
atomic commands only. Routing priority:
exact script/recipe > matching public Shortcut > atomic command.

- Select a real `cli_path` from the table; never guess names. Use
  `dws shortcut list --service chat --format json` only if no row matches.
- Once selected, execute directly. Read leaf Schema only when parameters,
  constraints, or safety are uncertain; read leaf `--help` only when flags are
  uncertain. If absent from Schema, use the same path in the full Catalog.
- For `confirmation=user_required`, confirm before adding `--yes`. On source
  conflict, use the safer interpretation and report it.

### 高频选择

| 用户意图 | 首选 Shortcut | 关键边界 |
|---|---|---|
| 当前用户 / 应用机器人 / Webhook 发消息 | `+messages-send --as user|bot|webhook` | Never mix identities |
| 按姓名发单聊 / 按群名发群消息 | `+dm` / `+send-to-group` | Resolves real person/chat IDs |
| 拉单个群聊或单聊消息 | `+chat-messages` | Choose one of `--group`, `--user`, `--open-dingtalk-id` |
| 组合搜索消息 | `+search-msg` | Keyword, sender, @, chat, type, time, `--page-all` |
| 查询 @ 我的消息 | `+at-me` | Defaults to 7 days; window and pagination are adjustable |
| 按消息 ID 批量取详情与 reaction | `+messages-mget` | Max 50; supports `--no-reactions` |
| 读取已知 thread/topic 的全部回复 | `+thread-replies` | `--group` required; choose thread or topic ID |
| 下载单个 mediaId/fileId | `+messages-resource-download` | mediaId needs message/chat context; fileId does not |
| 创建并按需立即更新流式卡片 | `+messages-send-card` | Choose one target; `--content` controls immediate update |
| 清空会话消息 / 删除分组 / 解散群 | `+conversation-clear-messages` / `+category-delete` / `+chat-dismiss` | 高风险; add `--yes` only after Schema confirmation |

### 统一发送

Before sending, verify identity, target, body, title, mentions, message type,
and attachment path; ask if ambiguous. Reuse the same `--idempotency-key` on retry.

```bash
dws chat +messages-send --as user --chat-id <openConversationId> --text "内容" --idempotency-key <key> --format json
dws chat +messages-send --as user --open-dingtalk-id <openDingTalkId> --msg-type file --file ./report.pdf --idempotency-key <key> --format json
dws chat +messages-send --as bot --robot-code <robotCode> --chat-id <openConversationId> --markdown "## 标题

正文" --format json
dws chat +messages-send --as webhook --webhook-token <token> --title "告警" --text "内容" --at-all --format json
```

- `user` supports text, Markdown, existing mediaId images, and local files.
  Audio/video use `--file` and are sent as files.
- `bot` supports group or batch-DM text/Markdown. A webhook token selects its
  chat. Neither identity supports files.
- The Shortcut normalizes @ placeholders. Pass only required `--at-*` /
  `--at-all`; never construct `@10` manually.
- Newlines in `--text` / `--markdown` must be real `U+000A`; separate Markdown
  paragraphs with a blank line.

### 查询、资源与卡片

- `+search-msg --page-all` paginates and enriches in batches. On partial
  failure, preserve results and return a per-item ledger.
- `+at-me`、`+chat-messages`、`+messages-mget`、`+search-msg`、`+thread-replies`
  support `--download-resources --output-dir ./downloads [--overwrite]`.
  For nested resources, prefer the child `messageId` in `resourceRefs`;
  inherit the parent only if the chat ID is missing.
- These queries and `+messages-resource-download` are `read/not_required`;
  never pass `--yes`. Output must be a relative path inside the working
  directory without `..`; pass `--overwrite` only when explicitly requested.
- Accept reviewed DingTalk/public OSS HTTPS URLs only. Validate every redirect
  and never forward headers across hosts.
- `+messages-send-card` 的 `--group`、`--receiver`、`--receiver-open-dingtalk-id`
  are mutually exclusive. With `--content`, update immediately; otherwise
  return `bizId`. The final `+messages-update-card` call must use `--flow-status 3`.

### Shortcut 错误处理

If a leaf is missing or parameters are invalid, check the full Catalog,
Schema, and `--help`, then correct once. Re-extract IDs from actual output.
Stop and report insufficient permission, unresolved ambiguity, no result, or
contract conflict.

## 渐进加载与一级路由

After choosing atomic fallback, select a top-level branch. If sibling commands
are ambiguous, read [intent-guide.md](references/intent-guide.md), then locate
the leaf in [chat.md](references/chat.md#命令索引表). If parameters, constraints,
or safety are uncertain, read
`dws schema --cli-path "chat <leaf>" --format json`; use leaf `--help` only
when Cobra flags are uncertain.

```text
dws chat
├── message              # send, list, search, reply, forward, recall, cards, reactions
├── group* / group-role  # chats, members, settings, mute, notice, roles
├── bot                  # bot search
├── category             # conversation categories
├── search               # chat and common-chat search
├── conversation-info / list-*  # conversation info and lists
├── mute* / hide / set-top
├── mark-* / clear-*     # read/unread, red dots, message clearing
├── text / chmod / data-auth
├── +shortcut
└── scripts
```

Load branch details on demand: [消息](references/chat/chat-message.md),
[群与成员](references/chat/chat-group.md),
[机器人与 Webhook](references/chat/chat-bot.md), and
[会话状态与分组](references/chat/chat-conversation.md).

## 核心意图表

Use this table only to disambiguate identity, chat type, operation, and three
dedicated scripts. Except for exact script matches, commands are atomic
fallbacks used only when no public Shortcut matches.

| 用户说 | 分支 / 原子回退 |
|---|---|
| “发给某人” | Current-user DM: resolve `openDingTalkId` / `userId`, then `message send` |
| “发到某群” | Current-user group message: `chat search`, then `message send --group` |
| “用应用机器人发” | Use `message send-by-bot`; never impersonate the current user |
| “Webhook 推送” | Use `message send-by-webhook`; mention text and @ flags must agree |
| “拉某个会话的消息” | Use `message list`; locate the chat and set time bounds first |
| “搜消息关键词 / 组合搜索” | Use `message search` for keywords; `search-advanced` for sender/@/multi-chat filters |
| “撤回用户消息 / 机器人消息” | Use `message recall` / `recall-by-bot`; their message IDs differ |
| “群消息翻页导出” | `python3 scripts/chat_export_messages.py` |
| “查和某人的聊天记录” | `python3 scripts/chat_history_with_user.py` |
| “机器人多群广播” | `python3 scripts/bot_broadcast.py` |

Script paths are relative to this Skill. For a script match, verify `python3`;
if unavailable, use the matching atomic fallback in
[01-messaging.md](references/01-messaging.md).

## 关键 SOP

These SOPs constrain identity, IDs, time bounds, and success checks without
overriding Shortcut priority. Follow a matching leaf; otherwise use the atomic
commands below. Add `--format json` to structured calls and take downstream
IDs from actual output.

### 发消息

1. Resolve a person with
   `dws aisearch person --keyword "<姓名>" --dimension name`; prefer
   `openDingTalkId`, otherwise `userId`. Resolve a chat with
   `dws chat search --query "<群名>"` to get `openConversationId`.
2. Without a matching Shortcut, use `message send --open-dingtalk-id`
   (`--user` if needed) for DMs and `message send --group` for groups. Local
   files/audio/video use `--msg-type file|audio|video --file-path`.
3. Judge success from structured output. `openTaskId` queries send status; it
   is not the message ID required for recall.

### 建群或拉人

1. Resolve every member to a real `userId` with `aisearch person`; never pass
   names as member arguments.
2. Without a matching Shortcut, create with
   `group create --name "<群名>" --users <userId...>` or add members with
   `group members add --id <openConversationId> --users <userId...>`.
3. Extract `openConversationId` from creation output. Confirm ambiguous chat
   names or members.

### 拉取或撤回消息

1. Resolve a group with `chat search`; resolve a DM target to `userId` or
   `openDingTalkId`.
2. Without a matching Shortcut, run `message list` only with the user's time
   range or an explicitly narrowed boundary.
3. Only on an explicit recall request, take the real `openMessageId` from the
   message list, then call
   `message recall --conversation-id <openConversationId> --msg-id <openMessageId>`.

## 低频操作原子回退入口

Use this table only to locate atomic fallbacks when no public Shortcut matches.
Follow leaf Schema, `--help`, and the linked reference for flags, risk, and
confirmation.

| 用户意图 | 原子回退 | 关键边界 |
|---|---|---|
| 收藏 / 取消收藏 / 收藏列表 | `message add-favorite` / `remove-favorite` / `list-favorites` | Changes personal favorites; does not recall |
| 编辑已发送消息 | `message edit` | Edits the existing message; does not resend |
| 普通群升级为外部群 | `group upgrade-to-external` | 不可逆; read leaf confirmation before execution |
| 设置或清除群昵称 | `group update-nick` | Omit `--nick` to clear the current user's chat nickname |
| 查会话所属分组 / 批量查分组 | `category list-by-conv` / `batch-info` | Different from listing every category |
| 特别关注人的消息 | `message list-focused` | Lists messages; “我关注了谁” routes to `dingtalk-contact` |
| 我和某人的共同群 | `search-common` | For “我”, get self nickname via `contact user get-self`; paginate by `nextCursor` |
| 群公告 / 群身份 | `group notice *` / `group-role *` | Chat notices differ from company notices; identity is not admin role |
| 消息置顶 / Pin / 会话置顶 | `message set-top-msg` / `set-pin-msg` / `chat set-top` | Three different target types |
| 查未读会话 / 谁看了消息 | `message list-unread-conversations` / `message read-status` | First lists chats; second lists readers of one message |
| 标未读 / 标已读 / 清红点 / 全部已读 | `mark-unread` / `mark-read` / `clear-red-point` / `clear-all-red-point` | Mutates chat/message state; not read-only |
| 行为授权 / 跨组织聊天数据授权 | `chmod` / `data-auth cross-org` | Grants action or cross-org read access; executes neither target action |
| 退群 / 解散群 | `group quit` / `group dismiss` | First exits current user; second dismisses the entire chat |

## 跨命令关键边界

- Never mix current-user, app-bot, and Webhook identities.
- `message list` reads one chat; use `search` / `search-advanced` for keywords
  and combined filters.
- `openTaskId` is not `openMessageId`; approval, calendar, todo, and other
  product IDs cannot replace a message ID.
- Message top, message Pin, and conversation top are distinct operations.
- Follow leaf Schema and current `--help` for parameters, risk, and
  confirmation; never infer across safety fields.

## Workflow 与错误导航

- For complex messaging, read [01-messaging.md](references/01-messaging.md).
- For onboarding, read
  [01-onboarding.md](references/workflows/01-onboarding.md).
- On command error, read
  [chat-error-recovery.md](references/chat-error-recovery.md) and correct
  once. Stop on insufficient permission, unresolved ambiguity, or no result
  after re-search.

## 跨产品协作

- 收件人是人名 → use `dingtalk-contact` or `dingtalk-aisearch` to get
  `openDingTalkId` / `userId`.
- 要发本地图片/文件 → use
  `dws chat message send --msg-type file --file-path <本地路径>`. Images are
  downloadable attachments, not inline. Use `--msg-type image --media-id`
  only with an existing valid mediaId; DWS cannot convert local files to mediaId.
- 紧急升级（应用内/短信/电话）→ route to `dingtalk-ding`.
- 发邮件 → route to `dingtalk-mail`.
