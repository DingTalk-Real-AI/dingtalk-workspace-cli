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
## Shortcut 发现（按需）

`chat` 当前有 97 条公开 shortcut。完整清单保留在 Runtime Shortcut Catalog；已完成 Schema curation 的子集可通过 leaf Schema 查询。高频产品根 Skill 不重复展开完整清单。已知意图直接使用下方的优先路由、意图表或任务 reference；命令已选中时直接执行，只在参数/安全语义不确定时读取 leaf Schema，在当前 Cobra flags 不确定时读取 leaf Help。

仅当现有路由和 reference 都无法定位低频能力时，才执行 `dws shortcut list --service chat --compact --format json` 做最后回退；不要为已知高频意图加载完整 Shortcut Catalog 或产品级 Schema。
<!-- VISIBLE_SHORTCUTS_END -->

## Shortcut 执行契约

This is the only Shortcut entry point for multi/chat; `references/` documents
atomic commands only. Routing priority:
exact recipe/runnable script > matching public Shortcut > atomic command.
Skip a script if its interpreter (for example, `python3`) is unavailable.

- Select a real `cli_path` from the high-frequency routes below; never guess names.
  Use `dws shortcut list --service chat --compact --format json` only when no
  reviewed route matches.
- Once selected, execute directly. Read leaf Schema only when parameters,
  constraints, or safety are uncertain; read leaf `--help` only when flags are
  uncertain. If absent from Schema, use the same path in the Runtime Shortcut
  Catalog.
- For `confirmation=user_required`, confirm before adding `--yes`. On source
  conflict, use the safer interpretation and report it.

### 高频直接执行骨架

命中后照抄参数名，不先调用 `--help`。

| 用户意图 | 精确 Shortcut 骨架 |
|---|---|
| 姓名发单聊 / 群名发群消息 | `+dm --to <姓名> --text <内容>` / `+send-to-group --group <群名> --text <内容>` |
| 三种身份发消息 | `+messages-send --as user|bot|webhook`；按下方身份模板补参数 |
| 改群名 | `+chat-update --group <openConversationId> --name <新群名>` |
| 列成员 / 批查成员 | `+chat-members-list --group <群名>` 或 `--conversation-id <cid>` / `+chat-members-get --id <cid> --users <odid,...>` |
| 消息详情 / 撤回 / 发送状态 | `+messages-mget --msg-ids <mid,...>` / `+messages-recall --conversation-id <cid> --msg-id <mid>` / `+messages-query-send-status --open-task-id <tid>` |
| 查 @ 我的消息 | 全局用 `+at-me --days <N>`；限定群用 `+search-msg --at-me --group <cid>`，不要给 `+at-me` 猜群参数 |
| 群邀请链接 / 群机器人 | 先 `+chat-search --query <群名>` 取 cid，再 `+chat-invite-url --group <cid>` / `+chat-bots --group <cid>` |
| 会话置顶 / 收藏列表 | `+conversation-set-top --conversation-id <cid> [--off]` / `+flag-list --size <1-100>` |

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

## 核心意图与执行边界

Use this table to disambiguate identity, chat type, and operation. Prefer public
Shortcuts; otherwise use the atomic fallback. Apply the shared `--format json`
rule and take every downstream ID from actual output. Every Chat workflow
must work without Python.

| 用户说 | 首选路由 / 原子回退 | 必须保留的执行边界 |
|---|---|---|
| “发给某人” | `+dm --to <姓名> --text <内容>` / `message send --open-dingtalk-id` | Resolve one real person; never pass a name as an ID |
| “发到某群” | `+send-to-group --group <群名> --text <内容>` / `chat search` → `message send --group` | Resolve one real cid; mentions/`@all` use `+messages-send` with that cid |
| “用应用机器人发” | `+messages-send --as bot` / `message send-by-bot` | Never impersonate the current user; keep robot and target IDs from real output |
| “Webhook 推送” | `+messages-send --as webhook` / `message send-by-webhook` | Webhook identity is separate; mention text and `--at-*` flags must agree |
| “建群 / 拉人进群” | `+chat-create --name <群名> --users <uid,...>` / `group members add` | Resolve every member to a real `userId`; extract the new cid before follow-up actions |
| “拉某个会话的消息” | `+chat-messages` / `message list` | Choose one group or DM target and use the user's time range or an explicitly narrowed boundary |
| “搜消息关键词 / 组合搜索” | `+search-msg --query <关键词>`；群内 @我用 `--at-me --group <cid>` | Add only real filters; use `--page-all` only when complete pagination is needed |
| “撤回用户消息 / 机器人消息” | `+messages-recall --conversation-id <cid> --msg-id <mid>` / `recall-by-bot` | Only on explicit recall; IDs must come from the same identity and conversation |
| “群消息翻页导出” | `+chat-messages`; save merged JSON if requested | Follow pagination to completion and report partial results instead of claiming a complete export |
| “查和某人的聊天记录” | Resolve the person, then use `+chat-messages` with one user ID | Stop for ambiguous people; do not merge different users with the same name |
| “机器人多群广播” | Call `+messages-send-by-bot` once per resolved group | Confirm the recipient set once, preserve one message body, and return a per-group success/failure ledger |

Detailed native CLI loops are in [01-messaging.md](references/01-messaging.md).

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
