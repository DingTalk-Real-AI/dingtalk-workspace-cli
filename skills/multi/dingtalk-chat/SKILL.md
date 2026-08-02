---
name: dingtalk-chat
description: 钉钉群聊与消息。Use when 用户提到 发消息/编辑或撤回消息/单聊/群聊/建群/普通群升级外部群/群昵称/会话分组/群成员管理/@消息/搜索聊天记录/话题回复/收藏消息/机器人群发/Webhook通知/发送或下载消息图片与文件。不做紧急 DING/短信/电话（走 dingtalk-misc）、邮件（走 dingtalk-mail）、班级群（走 dingtalk-misc）。命令前缀：dws chat。
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
## Shortcut 发现（按需）

`chat` 当前有 97 条公开 shortcut，完整清单保留在 Runtime Catalog 与 Schema，不在高频产品根 Skill 中重复展开。已知意图直接使用下方的优先路由、意图表或任务 reference；命令已选中时直接执行，只在参数/安全语义不确定时读取 leaf Schema，在当前 Cobra flags 不确定时读取 leaf Help。

仅当现有路由和 reference 都无法定位低频能力时，才执行 `dws shortcut list --service chat --format json` 做最后回退；不要为已知高频意图加载完整 Shortcut Catalog 或产品级 Schema。
<!-- VISIBLE_SHORTCUTS_END -->

## IM Shortcut 优先路由

- 统一发送优先用 `chat +messages-send`：通过 `--as user|bot|webhook` 选择身份，并只传该身份真实支持的目标、内容和凭据。仅在需要原子命令的特定返回结构或兼容参数时，才降级到 `chat message send` / `send-by-bot` / `send-by-webhook`。
- 消息查询按意图选择：指定会话用 `+chat-messages`，跨维度过滤/全量翻页用 `+search-msg`，@我用 `+at-me`，已知消息 ID 批量富化用 `+messages-mget`，已知 thread/topic ID 读取回复用 `+thread-replies`。
- 查询结果中的 `resourceRefs` 是可继续执行的资源上下文。引用、回复或合并转发中的子消息必须使用该子消息返回的 `messageId`；子消息缺会话 ID 时才继承父消息的 `openConversationId`。
- 上述五个查询 Shortcut 与 `+messages-resource-download` 都沿用安全本地下载的 `read/not_required` 契约，不应添加 `--yes` 或触发交互确认。下载只允许工作目录内相对路径、默认不覆盖并原子落盘；需要覆盖时必须由用户显式传 `--overwrite`。
- `+messages-send` 会按身份规范化 @ 占位符：user 使用 `<@id>` / `<@all>`，bot/webhook 使用 `@id` / `@手机号` / `@all`；只需声明 `--at-*` / `--at-all`，缺失占位符会自动补齐。
- `+messages-send --as user --user <userId>` 会通过通讯录关键词搜索并按 userId 精确匹配 openDingTalkId，所有内容类型与 `--dry-run` 都使用同一只读解析链路；已有 openDingTalkId 时直接传 `--open-dingtalk-id`。
- 流式卡片优先用 `+messages-send-card`：群聊传 `--group`，单聊 userId 传 `--receiver` 并由 CLI 通过通讯录关键词搜索做 userId 精确匹配，已有 openDingTalkId 则必须显式传 `--receiver-open-dingtalk-id`；三者严格三选一，禁止根据首字母猜身份类型。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "发消息给张三" | `dws chat +messages-send --as user --open-dingtalk-id <id> --text "<内容>"` |
| "发到XX群" | `dws chat +chat-search --query "<群名>"` → `dws chat +messages-send --as user --chat-id <openConversationId> --text "<内容>"` |
| "建群" / "拉人进群" | `dws chat group create` / `dws chat group members add` |
| "改群名" / "踢人" | `dws chat group rename` / `dws chat group members remove --yes`（踢人不可逆，先确认目标） |
| "@我消息" | `dws chat +at-me` |
| "查群聊记录" | `dws chat +chat-messages --group <openConversationId>` |
| "收藏/取消收藏这条消息" | `dws chat +flag-create` / `dws chat +flag-cancel` |
| "查看我收藏的消息" | `dws chat +flag-list` |
| "用机器人发消息" | `dws chat +messages-send --as bot --robot-code <code> --chat-id <id> --text "<内容>"` |
| "Webhook 推一条" | `dws chat +messages-send --as webhook --webhook-token <token> --text "<内容>"` |
| "发送流式卡片" | `dws chat +messages-send-card --group <openConversationId> --content "<内容>"`；单聊 userId 改用 `--receiver`，openDingTalkId 改用 `--receiver-open-dingtalk-id` |
| "撤回消息" | `dws chat message recall --conversation-id <openConversationId> --msg-id <openMessageId>` |
| "标记未读 / 清除红点 / 全部已读" | `dws chat mark-unread` / `dws chat clear-red-point` / `dws chat clear-all-red-point` |
| "置顶某条消息 / 取消消息置顶" | `dws chat message set-top-msg` / `dws chat message unset-top-msg` |
| "我加入的所有群 / 全部群列表" | `dws chat group list-all` |

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
- 要发图片/文件 → 先 `dt_media_upload` 上传 → `python scripts/extract_media_id.py "<URL>"` 提取 mediaId → 再用 `--media-id`
- 紧急升级（应用内/短信/电话）→ 切到 `dingtalk-misc`（`references/ding.md`）
- 发邮件 → 切到 `dingtalk-mail`
## 局部意图与短流程

- [局部意图消歧](references/intent-guide.md)；[短流程](references/lite-recipes.md)。
