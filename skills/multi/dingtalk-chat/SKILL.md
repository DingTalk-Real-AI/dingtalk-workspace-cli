---
name: dingtalk-chat
description: 钉钉群聊与消息。Use when 用户提到 发消息/单聊/群聊/建群/搜群或消息/聊天记录/群成员管理/回复转发撤回/@消息/表情回应/图片文件与资源下载/机器人群发/Webhook通知/互动卡片/收藏与Pin/消息或会话置顶/未读与红点/已读状态/会话分类。不做紧急 DING/短信/电话（走 dingtalk-ding）、邮件（走 dingtalk-mail）、班级群（走 dingtalk-misc）；找人本身走 dingtalk-contact 或 dingtalk-aisearch。命令前缀：dws chat。
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

> Command reference: [chat.md](references/chat.md); emoji list: [chat-emoji-list.md](references/chat-emoji-list.md).

<!-- VISIBLE_SHORTCUTS_START -->
## Shortcut 发现（按需）

`chat` 当前有 97 条公开 shortcut。完整清单保留在 Runtime Shortcut Catalog；已完成 Schema curation 的子集可通过 leaf Schema 查询。高频产品根 Skill 不重复展开完整清单。已知意图直接使用下方的优先路由、意图表或任务 reference；命令已选中时直接执行，只在参数/安全语义不确定时读取 leaf Schema，在当前 Cobra flags 不确定时读取 leaf Help。

仅当现有路由和 reference 都无法定位低频能力时，才执行 `dws shortcut list --service chat --compact --format json` 做最后回退；不要为已知高频意图加载完整 Shortcut Catalog 或产品级 Schema。
<!-- VISIBLE_SHORTCUTS_END -->

## 加载与路由顺序

This is the only Shortcut entry point for multi/chat; `references/` documents atomic commands only. Use `exact recipe/runnable script > matching public Shortcut > atomic command`; skip an optional script if its interpreter is unavailable.

1. 已知高频意图：直接使用“核心意图与执行骨架”，命中后照抄参数名，不先调用 `--help`。
2. 已有匹配 Shortcut：直接执行；参数、约束或安全不确定时才读 leaf Schema。
3. 仅当前 Cobra flags 不确定时读 leaf `--help`。
4. 现有路由无法定位低频能力时才查 Runtime Shortcut Catalog；select a real `cli_path` and never guess names.
5. 没有 Shortcut 时，按需读取对应 branch reference，进入原子命令。

For `confirmation=user_required`, confirm before adding `--yes`. On source conflict, use the safer interpretation and report it.

## 核心对象与 ID

| 对象 | 核心标识与边界 |
|---|---|
| 人员 | 姓名必须解析成唯一真实的 `userId` / `openDingTalkId`，不能把名称当 ID |
| 会话 | 使用真实 `openConversationId` / cid；群名只用于 Shortcut 目标解析 |
| 消息 | 使用真实 `openMessageId` / msgId，并保持与身份和会话一致 |
| 发送任务 | `openTaskId` 只用于查询发送状态，不能替代消息 ID |
| Thread | thread/topic ID 必须绑定真实会话，不跨会话复用 |
| 身份 | current-user、app-bot、Webhook 是不同操作者，不能自动互换 |
| 状态 | 收藏、消息置顶、消息 Pin、会话置顶作用于不同对象 |

## 核心意图与执行骨架

Prefer the exact Shortcut below; otherwise use the atomic fallback. Apply shared `--format json` and take downstream IDs from actual output. Every Chat workflow must work without Python.

| 用户意图 | 精确 Shortcut 骨架 / 原子回退 | 必须保留的执行边界 |
|---|---|---|
| 姓名发单聊 / 群名发群消息 | `+dm --to <姓名> --text <内容>` / `+send-to-group --group <群名> --text <内容>` | Resolve one real person or cid; mentions/`@all` use `+messages-send` |
| user / bot / webhook 发消息 | `+messages-send --as user|bot|webhook` / 对应 `message send*` | Identity determines the operator; use the matching target and content flags |
| 建群 / 改群名 / 拉人 | `+chat-create --name <群名> --users <uid,...>` / `+chat-update --group <cid> --name <新群名>` / `group members add` | Resolve every member; extract the new cid before follow-up actions |
| 列成员 / 批查成员 | `+chat-members-list --group <群名>` 或 `--conversation-id <cid>` / `+chat-members-get --id <cid> --users <odid,...>` | Stop on ambiguous group names; keep user and bot bucket failures |
| 拉会话消息 / 查某人记录 | `+chat-messages` / `message list` | Select one group or DM target; use the requested or explicitly narrowed time boundary |
| 搜消息 / 查 @ 我 | `+search-msg --query <关键词>`；全局 `+at-me --days <N>`；群内 `+search-msg --at-me --group <cid>` | Add only real filters; never invent group flags for `+at-me` |
| 消息详情 / 撤回 / 发送状态 | `+messages-mget --msg-ids <mid,...>` / `+messages-recall --conversation-id <cid> --msg-id <mid>` / `+messages-query-send-status --open-task-id <tid>` | Recall only when explicit; use IDs from the same identity and conversation |
| 群邀请链接 / 群机器人 | `+chat-search --query <群名>` → `+chat-invite-url --group <cid>` / `+chat-bots --group <cid>` | Reuse the resolved cid; do not search again by a guessed name |
| 会话置顶 / 收藏列表 | `+conversation-set-top --conversation-id <cid> [--off]` / `+flag-list --size <1-100>` | Do not confuse conversation top with message top, Pin, or favorite |
| 群消息完整导出 | `+chat-messages`; save merged JSON if requested | Follow pagination to completion; report partial results as incomplete |
| 机器人多群广播 | Call `+messages-send-by-bot` once per resolved group | Confirm recipients once; preserve one body and return a per-group ledger |

## 统一发送

身份决定真实操作者、可见范围和可用能力；同一目标使用 user、bot 或 webhook 时结果和权限可能不同，禁止自动切换身份重试。Before sending, verify identity, target, body, title, mentions, message type, and attachment path; ask if ambiguous. Reuse the same `--idempotency-key` on retry.

```bash
dws chat +messages-send --as user --chat-id <openConversationId> --text "内容" --idempotency-key <key> --format json
dws chat +messages-send --as user --open-dingtalk-id <openDingTalkId> --msg-type file --file ./report.pdf --idempotency-key <key> --format json
dws chat +messages-send --as bot --robot-code <robotCode> --chat-id <openConversationId> --markdown "## 标题

正文" --format json
dws chat +messages-send --as webhook --webhook-token <token> --title "告警" --text "内容" --at-all --format json
```

- `user` supports text, Markdown, existing mediaId images, and local files. Audio/video use `--file` and are sent as files.
- `bot` supports group or batch-DM text/Markdown. A webhook token selects its chat. Neither identity supports files.
- Pass only required `--at-*` / `--at-all`; never construct `@10` manually. Use real `U+000A` newlines and a blank line between Markdown paragraphs.

## 查询、资源与卡片

- `+search-msg` 只搜索消息内容；禁止用于按群名找群或解析群 CID。
- `+search-msg --page-all` paginates and enriches in batches; use it only when complete pagination is required.
- Preserve primary results on partial failure and return a per-item ledger; never claim an incomplete result is complete.
- If a sender name is absent, keep the real sender ID; do not guess a name or expand into an unrequested directory lookup.
- A missing optional enrichment is not a primary-query failure; record enrichment failures in the ledger.
- `+at-me`、`+chat-messages`、`+messages-mget`、`+search-msg`、`+thread-replies` support `--download-resources --output-dir ./downloads [--overwrite]`; resource download is opt-in.
- For nested resources, prefer the child `messageId` in `resourceRefs`; inherit the parent only if the chat ID is missing.
- These queries and `+messages-resource-download` are `read/not_required`; never pass `--yes`. Keep output relative, inside the working directory, without `..`; add `--overwrite` only when requested.
- Accept reviewed DingTalk/public OSS HTTPS URLs only. Validate every redirect and never forward headers across hosts.
- `+messages-send-card` 的 `--group`、`--receiver`、`--receiver-open-dingtalk-id` are mutually exclusive. With `--content`, update immediately; otherwise return `bizId`. The final `+messages-update-card` call uses `--flow-status 3`.

## 低频原子路由

Use this section only when no public Shortcut matches. If sibling commands are ambiguous, read [intent-guide.md](references/intent-guide.md), locate the leaf in [chat.md](references/chat.md#命令索引表), then load only the matching branch reference.

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
└── scripts
```

Branch references: [消息](references/chat/chat-message.md), [群与成员](references/chat/chat-group.md), [机器人与 Webhook](references/chat/chat-bot.md), [会话状态与分组](references/chat/chat-conversation.md).

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

## 错误恢复与按需 Reference

- If a leaf is missing or parameters are invalid, follow the established Catalog → Schema → `--help` order and correct once.
- Re-extract every downstream ID from actual output; never repair an ID by guessing.
- For complex messaging, read [01-messaging.md](references/01-messaging.md); for onboarding, read [01-onboarding.md](references/workflows/01-onboarding.md).
- On command error, read [chat-error-recovery.md](references/chat-error-recovery.md) and correct once.
- Stop and report insufficient permission, unresolved ambiguity, no result after re-search, or contract conflict.

## 跨产品协作

- 收件人是人名 → use `dingtalk-contact` or `dingtalk-aisearch` to obtain one real `openDingTalkId` / `userId`.
- 要发本地图片/文件 → use `dws chat message send --msg-type file --file-path <本地路径>`. Images are downloadable attachments, not inline. Use `--msg-type image --media-id` only with an existing valid mediaId; DWS cannot convert local files to mediaId.
- 紧急升级（应用内/短信/电话）→ route to `dingtalk-ding`.
- 发邮件 → route to `dingtalk-mail`.
