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

## 前置条件 — 执行操作前必读

> **CRITICAL — 执行任何 `dws` 操作前，MUST 先用 Read 工具完整读取 [`dws-shared`](../dws-shared/SKILL.md)。**该轻量文件包含全局执行契约、安全底线及 shared references 的按需加载导航；不要预加载其全部 references。

> 命令参考：[chat.md](references/chat.md)；表情：[chat-emoji-list.md](references/chat-emoji-list.md)；剧本：[01-messaging.md](references/01-messaging.md)。

<!-- VISIBLE_SHORTCUTS_START -->
## Shortcut 发现与选择

本产品提供公开内建 `+shortcut`，但本 Skill 不静态枚举，避免与当前 CLI catalog 漂移。

- 路由优先级：精确脚本 / recipe > 匹配的公开 shortcut > reference 中的原子命令。
- 无精确脚本 / recipe 覆盖，或准备手工组合多个原子命令前，必须先运行 `dws shortcut list --service chat --compact --format json`；当前 CLI 不接受 `--compact` 时去掉该 flag 重试。
- 必须依据返回的 `intent`、`description` 和 `cli_path` 选择，不得猜测 `+` 命令名。
- 选中后用 `dws schema --cli-path "<cli_path>" --format json` 读取完整契约；若该 leaf 暂未进入 Schema，则重新运行不带 `--compact` 的 catalog 命令并选取同一 `cli_path`。
- 执行前始终用 `dws <cli_path> --help` 核对当前 Cobra flags；无匹配项时回退到意图表、reference 和原子命令。
- `confirmation=user_required` 时先征得用户确认，再添加 `--yes`；Schema、catalog 与 Help 冲突时采用更安全的解释并报告契约漂移。
<!-- VISIBLE_SHORTCUTS_END -->

## 渐进加载与一级路由

决定回退到原子命令后，先按一级分支定位能力；兄弟命令容易混淆时读取
[intent-guide.md](references/intent-guide.md)，再从
[chat.md 的命令索引表](references/chat.md#命令索引表) 找到叶子。执行前用
`dws schema --cli-path "chat <leaf>" --format json` 读取选择、参数、约束和确认语义，
并以当前叶子的 `--help` 核对 Cobra flags。

```text
dws chat
├── message              # 发送、查询、搜索、回复、转发、撤回、卡片、表情
├── group* / group-role  # 群、成员、设置、禁言、公告、群身份
├── bot                  # 机器人搜索
├── category             # 会话分组
├── search               # 群搜索与共同群
├── conversation-info / list-*  # 会话信息与会话列表
├── mute* / hide / set-top
├── mark-* / clear-*     # 已读、未读、红点与消息清理
├── text / chmod / data-auth
├── +shortcut
└── scripts
```

分支细节按需读取：[消息](references/chat/chat-message.md)、
[群与成员](references/chat/chat-group.md)、[机器人与 Webhook](references/chat/chat-bot.md)、
[会话状态与分组](references/chat/chat-conversation.md)。

## 核心意图表

本表只负责身份、会话类型、操作类型和三个专用脚本的消歧。除精确命中的脚本外，
表中命令均为未匹配公开 shortcut 时的原子回退；不得跳过上方 Shortcut 发现流程。

| 用户说 | 分支 / 原子回退 |
|---|---|
| “发给某人” | 当前用户单聊：先解析 `openDingTalkId` / `userId`，再用 `message send` |
| “发到某群” | 当前用户群聊：先 `chat search`，再用 `message send --group` |
| “用应用机器人发” | `message send-by-bot`，不要用当前用户身份代发 |
| “Webhook 推送” | `message send-by-webhook`；需要 @ 时正文与 @ 参数必须对应 |
| “拉某个会话的消息” | `message list`，先定位会话并确定时间边界 |
| “搜消息关键词 / 组合搜索” | 简单关键词可用 `message search`；发送者、@、多会话条件用 `search-advanced` |
| “撤回用户消息 / 机器人消息” | `message recall` / `recall-by-bot`，两者所需消息标识不同 |
| “群消息翻页导出” | `python3 scripts/chat_export_messages.py` |
| “查和某人的聊天记录” | `python3 scripts/chat_history_with_user.py` |
| “机器人多群广播” | `python3 scripts/bot_broadcast.py` |

脚本路径相对于本 Skill 根目录；命中脚本时先确认 `python3` 可用，不可用则按
[01-messaging.md](references/01-messaging.md) 中对应场景的原子命令备选流程执行。

## 关键 SOP

以下 SOP 约束身份、ID、时间边界和成功判断，不覆盖 Shortcut 路由优先级。匹配
shortcut 时按其 leaf 契约执行；未匹配时再使用下面的原子命令。所有结构化调用带
`--format json`；下游 ID 必须取自真实返回。

### 发消息

1. 人名先用 `dws aisearch person --keyword "<姓名>" --dimension name` 取
   `openDingTalkId`（优先）或 `userId`；群名先用 `dws chat search --query "<群名>"`
   取 `openConversationId`。
2. 未匹配 shortcut 时，单聊使用 `message send --open-dingtalk-id`（必要时用 `--user`）；群聊使用
   `message send --group`。本地文件、音频、视频使用 `--msg-type file|audio|video --file-path`。
3. 按结构化返回判断成功；`openTaskId` 只能查询发送状态，不是撤回所需消息 ID。

### 建群或拉人

1. 对每个成员先用 `aisearch person` 取得真实 `userId`，不要把姓名直接作为成员参数。
2. 未匹配 shortcut 时，建群使用 `group create --name "<群名>" --users <userId...>`；已有群加成员使用
   `group members add --id <openConversationId> --users <userId...>`。
3. 建群后从返回提取 `openConversationId`；群名或成员有歧义时先让用户确认。

### 拉取或撤回消息

1. 群名先用 `chat search` 定位；单聊对象先解析 `userId` 或 `openDingTalkId`。
2. 未匹配 shortcut 时，`message list` 必须使用用户时间范围或明确收敛后的时间边界。
3. 仅在用户明确要求撤回时，从消息列表取得真实 `openMessageId`，再调用
   `message recall --conversation-id <openConversationId> --msg-id <openMessageId>`。

## 低频操作原子回退入口

这里只负责未匹配公开 shortcut 后的原子命令定位；具体 flags、风险和确认以 leaf
Schema、`--help` 与对应 reference 为准。

| 用户意图 | 原子回退 | 关键边界 |
|---|---|---|
| 收藏 / 取消收藏 / 收藏列表 | `message add-favorite` / `remove-favorite` / `list-favorites` | 只改变个人收藏，不撤回原消息 |
| 编辑已发送消息 | `message edit` | 修改现有消息，不是重新发送 |
| 普通群升级为外部群 | `group upgrade-to-external` | 不可逆，执行前读取 leaf Schema 的确认语义 |
| 设置或清除群昵称 | `group update-nick` | 省略 `--nick` 表示清除当前用户群昵称 |
| 查会话所属分组 / 批量查分组 | `category list-by-conv` / `batch-info` | 与列出全部分组不同 |
| 特别关注人的消息 | `message list-focused` | 查消息；“我关注了谁”走 `dingtalk-contact` |
| 我和某人的共同群 | `search-common` | 包含“我”时先用 `contact user get-self` 取本人昵称；按 `nextCursor` 翻页 |
| 群公告 / 群身份 | `group notice *` / `group-role *` | 公告与企业公告不同；群身份不是管理员角色 |
| 消息置顶 / Pin / 会话置顶 | `message set-top-msg` / `set-pin-msg` / `chat set-top` | 三者作用对象不同 |
| 查未读会话 / 谁看了消息 | `message list-unread-conversations` / `message read-status` | 前者查会话，后者查指定消息的阅读人员 |
| 标未读 / 标已读 / 清红点 / 全部已读 | `mark-unread` / `mark-read` / `clear-red-point` / `clear-all-red-point` | 修改会话或消息状态，不是只读查询 |
| 行为授权 / 跨组织聊天数据授权 | `chmod` / `data-auth cross-org` | 前者只授权具体操作，后者只授权跨组织数据读取；都不执行目标操作 |
| 退群 / 解散群 | `group quit` / `group dismiss` | 前者只退出当前用户，后者解散整个群 |

## 跨命令关键边界

- 当前用户、应用机器人、Webhook 三种发送身份不能混用。
- `message list` 拉取指定会话；关键词与组合条件使用 `search` / `search-advanced`。
- `openTaskId` 不是 `openMessageId`；审批、日历、待办等产品对象 ID 也不能代替消息 ID。
- 消息置顶、消息 Pin、会话置顶是三种不同操作。
- 参数、风险与确认以 leaf Schema 和当前 `--help` 为准，不从其他安全字段推断。

## Workflow 与错误导航

- 复杂消息流程读取 [01-messaging.md](references/01-messaging.md)。
- 新人接待读取 [01-onboarding.md](references/workflows/01-onboarding.md)。
- 命令返回错误时读取 [chat-error-recovery.md](references/chat-error-recovery.md)，只修正一次；
  权限不足、目标仍有歧义或重新搜索无结果时停止并报告。

## 跨产品协作

- 收件人是人名 → 先用 `dingtalk-contact` 或 `dingtalk-aisearch` 拿 `openDingTalkId` / `userId`
- 要发本地图片/文件 → 直接用 `dws chat message send --msg-type file --file-path <本地路径>`；图片会作为可下载的文件附件发送，不会内联渲染。只有上游已提供有效 mediaId 时才用 `--msg-type image --media-id`，DWS CLI 不能把本地文件转换成 mediaId
- 紧急升级（应用内/短信/电话）→ 切到 `dingtalk-ding`
- 发邮件 → 切到 `dingtalk-mail`
