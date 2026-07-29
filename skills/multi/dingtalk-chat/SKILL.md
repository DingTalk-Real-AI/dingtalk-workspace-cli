---
name: dingtalk-chat
description: 钉钉群聊与消息。Use when 用户提到 发消息/单聊/群聊/建群/拉人进群/改群名/搜索群/群成员管理/@消息/撤回消息/机器人群发/Webhook通知/发图片或文件到群/标记未读/清除红点/置顶消息/全部群列表。不做紧急 DING/短信/电话（走 dingtalk-misc）、邮件（走 dingtalk-mail）、班级群（走 dingtalk-misc）。命令前缀：dws chat。
metadata:
  cli_version: ">=0.2.14"
  category: product
  requires:
    bins:
      - dws
---

# 钉钉群聊 / 消息 Skill

## 前置条件 — 执行操作前必读

> **CRITICAL — 执行任何 `dws` 操作前，MUST 先用 Read 工具完整读取 [`dws-shared`](../dws-shared/SKILL.md)。**该轻量文件包含全局执行契约、安全底线及 shared references 的按需加载导航；不要预加载其全部 references。

> 命令参考：[chat.md](references/chat.md)；表情：[chat-emoji-list.md](references/chat-emoji-list.md)；剧本：[01-messaging.md](references/01-messaging.md)。

<!-- VISIBLE_SHORTCUTS_START -->
## Shortcuts（无专用脚本/recipe 时优先）

以下 shortcut 同时进入公开 catalog 与 Runtime Schema。先按本 skill 的意图表、脚本和 recipe 路由：存在精确覆盖该场景的专用脚本/recipe 时按其执行；否则用户意图命中时，shortcut 优先于手写原子命令。用 leaf Schema（例如 `dws schema --cli-path "chat +<shortcut>" --format json`）读取 Agent 选择、参数、约束、风险和确认语义；用 `dws shortcut list --service chat --format json` 批量发现；最后以 `dws chat <shortcut> --help` 核对当前 Cobra flags。

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
| "发消息给张三" | `dws chat message send --open-dingtalk-id <id> --text "<内容>"` |
| "发到XX群" | `dws chat search --query "<群名>"` → `dws chat message send --group <openConversationId> --text "<内容>"` |
| "建群" / "拉人进群" | `dws chat group create` / `dws chat group members add` |
| "改群名" / "踢人" | `dws chat group rename` / `dws chat group members remove`（先确认群和成员；踢群主会被 CLI 拦截，需先 `transfer-owner`）|
| "@我消息" | `dws chat message list-mentions` |
| "查群聊记录" | `dws chat search --query "<群名>"` → `dws chat message list --group <openConversationId> --time "<yyyy-MM-dd HH:mm:ss>" --direction older` |
| "收藏/取消收藏这条消息" | `dws chat message add-favorite` / `dws chat message remove-favorite`（均需 `openMessageId` 和 `openConversationId`）|
| "查看我收藏的消息" | `dws chat message list-favorites`（默认 `--cursor 0 --size 20`）|
| "用机器人发消息" | `dws chat message send-by-bot --robot-code <code> --group <id> --title "<标题>" --text "<内容>"` |
| "Webhook 推一条" | `dws chat message send-by-webhook --token <token> --title "<标题>" --text "<内容>"` |
| "编辑 / 修改已发送消息" | `dws chat message edit --conversation-id <openConversationId> --msg-id <openMessageId> --text "<新内容>"` |
| "撤回我发的消息" | `dws chat message recall --conversation-id <openConversationId> --msg-id <openMessageId>` |
| "撤回机器人消息" | `dws chat message recall-by-bot --robot-code <code> --group <openConversationId> --keys <processQueryKey>`（撤回机器人发的）|
| "把普通群升级为外部群" | `dws chat group upgrade-to-external --group <openConversationId> --dry-run`，确认后加 `--yes` |
| "清除我的群昵称" | `dws chat group update-nick --group <openConversationId>`（省略 `--nick`） |
| "这个会话属于哪些分组" | `dws chat category list-by-conv --group <openConversationId>` |
| "批量查询分组信息" | `dws chat category batch-info --category-ids <id1>,<id2>` |
| "查群聊记录（含翻页/导出）" | `python scripts/chat_export_messages.py --query "<群名>" --time "<时间>"` |
| "查和某人的聊天记录" | `python scripts/chat_history_with_user.py --name "<姓名>" --time "<时间>"` |
| "机器人多群群发" | `python scripts/bot_broadcast.py --robot-code <code> --chats <id1>,<id2> --title "<标题>" --text "<内容>"` |
| "查某人发的消息" | `dws chat message list-by-sender --sender-user-id <userId> --start <ISO> --end <ISO>` |
| "特别关注人的消息" | `dws chat message list-focused --limit 50` |
| "查共同群" | `dws chat search-common --nicks "<昵称1>,<昵称2>" --match-mode AND` |
| "退群" / "解散群" | `dws chat group quit --group <openConversationId>` / `dws chat group dismiss --group <openConversationId> --yes` |
| "转群主" | `dws chat group transfer-owner --group <openConversationId> --new-owner <openDingTalkId>` |
| "群公告" | `dws chat group notice create` / `edit` / `get` / `list` |
| "查看或设置群身份" | `dws chat group-role list` / `add` / `update` / `remove` / `set-user` / `remove-user` / `query-user` |
| "消息已读未读" | `dws chat message read-status --conversation-id <id> --message-id <id>` |
| "搜索消息内容" | 简单关键词用 `dws chat message search`；发送者、@、多会话组合条件用 `search-advanced` |
| "引用回复/转发/合并转发" | `dws chat message reply` / `forward` / `combine-forward` / `forward-topic` |
| "置顶/钉住消息" | `dws chat message set-top-msg` / `set-pin-msg`；对应取消命令为 `unset-*` |
| "会话免打扰/隐藏/标记已读" | `dws chat mute` / `hide` / `mark-read` / `mark-unread` |
| "标记未读 / 清除红点 / 全部已读" | `dws chat mark-unread` / `dws chat clear-red-point` / `dws chat clear-all-red-point` |
| "我加入的所有群 / 全部群列表" | `dws chat group list-all` |

## 命令树（Tree Structure）

先按一级分支选路，再进入叶子命令。`+shortcut` 分支已在上方 Shortcuts 表中单列；下面的树覆盖原生命令与本 skill 自带脚本。

```text
dingtalk-chat
├── message
│   ├── send / send-by-bot / send-by-webhook
│   ├── send-card → update-card
│   ├── edit / recall / recall-by-bot
│   ├── reply / forward / forward-topic / combine-forward
│   ├── list / list-direct / list-all
│   ├── list-by-sender / list-mentions / list-focused
│   ├── list-unread-conversations / list-topic-replies / list-by-ids
│   ├── search / search-advanced
│   ├── query-send-status / read-status
│   ├── add-favorite / remove-favorite / list-favorites
│   ├── add-emoji / remove-emoji
│   ├── add-text-emotion / remove-text-emotion / create-text-emotion / update-text-emotion
│   ├── list-emotion-replies
│   ├── set-top-msg / unset-top-msg
│   ├── set-pin-msg / unset-pin-msg / list-pin-msg
│   └── download-media
├── group
│   ├── create / rename / dismiss / quit / transfer-owner
│   ├── get-by-group-id / invite-url / share-invite
│   ├── update-icon / update-settings / update-alias / update-nick
│   ├── get-mute-config / user-settings query / user-settings set
│   ├── upgrade-to-external / set-history / set-admin
│   ├── bots / list-my-groups / list-all
│   ├── list-join-validations / audit-join-validation
│   ├── members
│   │   ├── [list] / list-by-ids
│   │   ├── add / remove
│   │   └── add-bot / remove-bot
│   └── notice
│       └── create / edit / get / list
├── group-role
│   └── list / add / update / remove / set-user / remove-user / query-user
├── bot
│   └── search / find
├── category
│   ├── list / list-by-conv / batch-info / list-conversations
│   ├── create / create-smart / rename / delete
│   └── add-conv / remove-conv
├── search / search-common
├── conversation-info / list-top-conversations / list-all-conversations
├── mute / set-top / hide / mark-unread / mark-read
├── clear-red-point / clear-all-red-point / clear-messages
├── mute-at-all / mute-red-envelope
├── group-mute / group-mute-member
├── text
│   └── translate
├── data-auth
│   └── cross-org
├── chmod
├── +shortcut
│   └── 见上方 Shortcuts 表；有精确脚本/recipe 时优先脚本/recipe
└── scripts
    ├── chat_export_messages.py
    ├── chat_history_with_user.py
    └── bot_broadcast.py
```

> 路由规则：普通文本、图片或文件发送走 `message send`；明确要求应用机器人走 `send-by-bot`；明确要求 Webhook 机器人走 `send-by-webhook`。关键词搜索优先 `message search`，只有发送者、@、多会话等组合条件才走 `search-advanced`。

## 评测高频硬约束

- `chat message edit` 必须同时提供会话 ID 与消息 ID；`--text` 和 `--content` 二选一，`--title` 只与 `--text` 搭配。
- `chat group upgrade-to-external` 只适用于 `NORMAL_GROUP`，不可逆且仅群主可执行。先 `--dry-run`，获得用户明确确认后再加 `--yes`。
- `chat group update-nick` 不传 `--nick` 表示清除当前用户的群昵称，不是参数缺失。
- 会话分组 ID 是数值 ID；按会话反查用 `category list-by-conv`，按多个分组 ID 查详情用 `category batch-info`。
- 群成员命令使用 `--id <openConversationId>`；不要臆造 `members list` 子命令，也不要把消息发送用的 `--group` 套到成员管理。
- `message list` 是按时间拉消息，不是关键词搜索；关键词审计用 `message search`。
- 转发审批、日历、待办等原生卡片时，必须先从源会话消息中取得真实 `openMessageId`；产品对象 ID 不能代替消息 ID。
- `send-card` 创建的是通用流式卡片，不是原生审批、日历或待办卡片；`update-card --biz-id` 只能使用 `send-card` 返回的 `bizId`。
- 会话分组标题最多 15 个字符，必须保持用户原文；不得静默截断、缩写或改写。

## 典型 Workflow

- [新人入职群聊接待](references/workflows/01-onboarding.md)：查人 → 拉群 → 欢迎消息 → 待办 → 会议。
- [消息处理剧本](references/01-messaging.md)：查询、转发、共同群、特别关注与收藏。

## 错误处理与自纠

按 [chat-error-recovery.md](references/chat-error-recovery.md) 处理 chat 局部错误：

- 参数或路径错误：先查当前叶子 `--help`，只修正一次。
- 群 ID 无效：用 `chat search` 重新取得真实 `openConversationId`。
- 权限不足、机器人无法加群、搜索仍无结果或目标有歧义：停止并报告用户，不猜测 ID、不反复试错。

## 标准 SOP（必遵流程）

> 命中以下意图**必须**按对应 SOP 顺序执行；**禁止**跳步、替换命令、编造 ID。每条命令必须带 `--format json`，执行后必须按"解析"步取真实字段（`openDingTalkId` / `openConversationId` / `openMessageId`）。

### SOP-1 发消息（send-message）

**触发**：发消息/单聊/通知某人/发到群里。

1. **解析收件人（必须）**：人名 → 先 `dws aisearch person --keyword "<姓名>" --dimension name --format json` 取 `openDingTalkId`（优先）或 `userId`；群名 → 先 `dws chat search --query "<群名>" --format json` 取 `openConversationId`。
2. **执行（必须）**：单聊 `dws chat message send --open-dingtalk-id <openDingTalkId> --text "<内容>" --format json`（只有拿不到 `openDingTalkId` 时才用 `--user <userId>`）；群聊 `dws chat message send --group <openConversationId> --text "<内容>" --format json`。
3. **验证（必须）**：发送接口成功时可能只返回 `result.openTaskId`，它可用于确认提交成功，但**不是撤回所需消息 ID**。需要撤回时必须按 SOP-7 用带 `--time` 的 `message list` 回查，取得真实 `openMessageId`；返回非 `success` 必须如实报错，不要谎报已发。

**禁止**：把人名/群名直接当 ID 传入、跳过 `aisearch person`/`chat search` 解析、跳过 `--format json`、未发送成功就答复"已发送"。

### SOP-2 建群（create-group）

**触发**：建群/拉人进群/新建讨论组。

1. **解析成员（必须）**：对每个成员 `dws aisearch person --keyword "<姓名>" --dimension name --format json` 取 `userId`，多人英文逗号拼接。
2. **执行（必须）**：`dws chat group create --name "<群名>" --users <userId1,userId2,...> --format json`；外部群加 `--type EXTERNAL`，话题圈加 `--thread`。
3. **验证（必须）**：从返回取 `openConversationId`，可用 `dws chat search --query "<群名>" --format json` 复核。

**禁止**：跳过成员 userId 解析直接传姓名、编造 `openConversationId`。

### SOP-3 Webhook 推送（send-by-webhook）

**触发**：用机器人群 webhook 推一条消息。

1. **执行（必须）**：`dws chat message send-by-webhook --token <webhookToken> --title "<标题>" --text "<内容>" --format json`。
2. **@ 人（必须）**：需要 @ 时，`--text` 中**必须**先包含对应 `@userId` / `@手机号` / `@10`，再配合 `--at-users` / `--at-mobiles` / `--at-all`；否则 @ 不生效。

**禁止**：只传 `--at-users` 而 `--text` 里不含 `@<标识>`。

### SOP-4 共同群查询（search-common-group）

**触发**："我和 XX 的共同群"。

1. **取昵称（必须）**：先 `dws contact user get-self --format json` 取自己昵称；对方昵称从历史/上下文取，拿不到必须先问用户。
2. **执行（必须）**：`dws chat search-common --nicks "<昵称1>,<昵称2>" --limit 20 --cursor 0 --format json`；`hasMore=true` 时**必须**用 `nextCursor` 翻页，不要停在第一页。

**禁止**：跳过昵称解析、忽略 `hasMore` 不翻页。

### SOP-5 红点 / 未读管理（manage-red-point）

**触发**：标记未读/清除红点/全部已读。

1. **执行（必须）**：标记某会话未读 `dws chat mark-unread --conversation-id <openConversationId> --format json`；清除某会话红点 `dws chat clear-red-point --conversation-id <openConversationId> --format json`；全部已读 `dws chat clear-all-red-point --format json`。
2. **取会话 ID（必须）**：`openConversationId` 拿不准时先 `dws chat group list-all --format json` 或 `dws chat search --query "<群名>" --format json`，**禁止**编造。

**禁止**：未确认会话就批量"全部已读"（破坏性，必须先与用户确认）。

### SOP-6 特别关注消息（focus-messages）

**触发**："特别关注的人最近发了什么/聊了什么"。

1. **执行（必须）**：`dws chat message list-focused --limit 50 --format json`，直接基于返回答复。
2. **边界（必须）**：只有用户终点是"我关注了谁"这种**人员列表**时，才切 `dingtalk-contact` 关系查询。

**禁止**：用普通 `message list` 冒充 focused、把人员列表需求硬塞进 chat。

### SOP-7 拉取 / 撤回消息（list-or-recall-message）

**触发**：查某个群或单聊的聊天记录、撤回某条消息。

1. **定位会话（必须）**：群名先 `dws chat search --query "<群名>" --format json` 取 `openConversationId`；单聊对象先解析真实 `userId` 或 `openDingTalkId`。
2. **拉取消息（必须）**：`dws chat message list --group <openConversationId> --time "<yyyy-MM-dd HH:mm:ss>" --direction older --format json`；单聊将 `--group` 换成 `--user` 或 `--open-dingtalk-id`。`--time` 是必填参数，必须来自用户时间范围或明确收敛后的边界。
3. **撤回（必须）**：仅在用户明确要求撤回时，从消息列表 `result.messages[].openMessageId` 取真实 ID，执行 `dws chat message recall --conversation-id <openConversationId> --msg-id <openMessageId> --format json`。

**禁止**：省略 `--time`、用发送返回的 `clientMsgId` 代替 `openMessageId`、只传 `--client-msg-id`、编造会话或消息 ID。

## 跨产品协作

- 收件人是人名 → 先用 `dingtalk-contact` 或 `dingtalk-aisearch` 拿 `openDingTalkId` / `userId`
- 要发本地图片/文件 → 直接用 `dws chat message send --msg-type file --file-path <本地路径>`；图片会作为可下载的文件附件发送，不会内联渲染。只有上游已提供有效 mediaId 时才用 `--msg-type image --media-id`，DWS CLI 不能把本地文件转换成 mediaId
- 紧急升级（应用内/短信/电话）→ 切到 `dingtalk-ding`
- 发邮件 → 切到 `dingtalk-mail`
## 局部意图与短流程

- [局部意图消歧](references/intent-guide.md)；[短流程](references/lite-recipes.md)。
