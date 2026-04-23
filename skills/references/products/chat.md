# 会话与群聊 (chat) 命令参考

> 本文档与真实 CLI 严格对齐（`dws chat --help` 实测）。所有命令路径、flag 名、默认值均以 CLI 实际为准；文档作者补充的用法约定会标注「约定」。

## 命令总览

### chat 顶级

| 子命令 | 用途 |
|-------|------|
| `search` | 按名称搜索会话列表 |
| `list-conversation-message` | ⚠️ legacy 接口，请改用 `message list`（保留仅向后兼容） |

### group (群组管理)

| 子命令 | 用途 |
|-------|------|
| `group create` | 创建内部群 |
| `group create-org` | 创建企业全员群 |
| `group rename` | 修改群名称 |
| `group search-common` | 搜索共同群 |
| `group members list` | 查看群成员列表 |
| `group members add` | 添加群成员 |
| `group members remove` | 移除群成员（⚠️ 危险操作） |
| `group members add-bot` | 添加机器人到群 |

### message (会话消息管理)

| 子命令 | 用途 |
|-------|------|
| `message info` | 获取会话详情（群聊 `--id` 或单聊 `--open-id`） |
| `message list` | 拉取**群聊**消息（按时间点+方向翻页） |
| `message list-direct` | 拉取**单聊**消息 |
| `message list-topic-replies` | 拉取群话题回复列表 |
| `message list-focus` | 拉取特别关注人的消息 |
| `message list-top` | 拉取置顶会话列表 |
| `message unread` | 获取未读会话列表 |
| `message search` | 按关键词搜索消息（跨所有会话或限定群） |
| `message search-at-me` | 拉取 @我 的消息 |
| `message search-by-sender` | 拉取指定发送者发给我的消息（跨单聊/群聊） |
| `message search-by-time` | 按时间范围拉取当前用户所有会话消息 |
| `message send` | 以当前用户身份发**群聊**消息 |
| `message send-direct` | 以当前用户身份发**单聊**消息 |
| `message send-personal` | 发送个人消息（⚠️ 敏感操作） |
| `message send-by-bot` | 机器人发消息（群聊 `--group` / 批量单聊 `--users`） |
| `message send-by-webhook` | 自定义机器人 Webhook 发群消息 |
| `message recall-by-bot` | 机器人撤回消息 |

### bot (机器人管理)

> ⚠️ 注意路径：真实 CLI 下存在**双 bot** 前缀（历史遗留），机器人管理命令挂在 `chat bot bot` 下。

| 子命令 | 用途 |
|-------|------|
| `bot bot search` | 搜索我的机器人 |
| `bot bot create` | 创建企业机器人 |
| `bot bot search-groups` | 搜索机器人所在群 |

---

## search — 按名称搜索会话

```
Usage:
  dws chat search [flags]
Example:
  dws chat search --query "项目冲刺"
  dws chat search --query "项目冲刺" --cursor <nextCursor>
Flags:
      --query string    搜索关键词 (必填)
      --cursor string   分页游标（首页留空）
```

---

## list-conversation-message — ⚠️ legacy

> legacy 接口，flag 用下划线命名（`--openconversation_id`），推荐改用 `chat message list`。此命令仅向后兼容保留。

```
Usage:
  dws chat list-conversation-message [flags]
Flags:
      --openconversation_id string   群会话 ID（必填）
      --start-time string            起始时间 startTime
      --end-time string              结束时间 endTime
```

---

## group create — 创建内部群

当前登录用户自动成为群主。

```
Usage:
  dws chat group create [flags]
Example:
  dws chat group create --name "Q1 项目冲刺群" --users userId1,userId2,userId3
Flags:
      --name string    群名称 (必填)
      --users string   群成员 userId 列表，逗号分隔 (必填)
```

---

## group create-org — 创建企业全员群

```
Usage:
  dws chat group create-org [flags]
Example:
  dws chat group create-org --name "全员通知群" --users userId1,userId2
Flags:
      --name string    群名称 (groupName) (必填)
      --users string   群成员 userId 列表 (groupMembers) (必填)
```

---

## group rename — 修改群名称

```
Usage:
  dws chat group rename [flags]
Example:
  dws chat group rename --id <openconversation_id> --name "新群名"
Flags:
      --id string     群 ID / openconversation_id (必填)
      --name string   新群名称 (必填)
```

---

## group search-common — 搜索共同群

根据昵称列表搜索共同群聊。

```
Usage:
  dws chat group search-common [flags]
Example:
  dws chat group search-common --nicks "风雷,山乔" --limit 20 --cursor 0
  dws chat group search-common --nicks "天鸡,乐函" --mode OR --limit 20 --cursor 0
Flags:
      --nicks string    要搜索的昵称列表，逗号分隔 (必填)
      --mode string     匹配模式：AND=所有人都在群里 / OR=任一人在群里（默认 AND）
      --limit string    每页返回数量
      --cursor string   分页游标（首页传 "0"，翻页传 nextCursor）

注意:
  - 真实 flag 名是 --mode，不是 --match-mode
  - --limit 类型是 string（CLI 自动生成 stub），传数字字符串如 "20"
```

---

## group members list — 查看群成员列表

```
Usage:
  dws chat group members list [flags]
Example:
  dws chat group members list --id <openconversation_id>
  dws chat group members list --id <openconversation_id> --cursor <nextCursor>
Flags:
      --id string       群 ID / openconversation_id (必填)
      --cursor string   分页游标
```

---

## group members add — 添加群成员

```
Usage:
  dws chat group members add [flags]
Example:
  dws chat group members add --id <openconversation_id> --users userId1,userId2
Flags:
      --id string      群 ID / openconversation_id (必填)
      --users string   要添加的 userId 列表，逗号分隔 (必填)
```

---

## group members remove — 移除群成员

> ⚠️ 危险操作：执行前必须向用户确认，同意后才加 `--yes`。

```
Usage:
  dws chat group members remove [flags]
Example:
  dws chat group members remove --id <openconversation_id> --users userId1,userId2
Flags:
      --id string      群 ID / openconversation_id (必填)
      --users string   要移除的 userId 列表，逗号分隔 (必填)
```

---

## group members add-bot — 添加机器人到群

将自定义机器人添加到当前用户有管理权限的群。

```
Usage:
  dws chat group members add-bot [flags]
Example:
  dws chat group members add-bot --id <openconversation_id> --robot-code <robot-code>
Flags:
      --id string           群 openConversationId (必填)
      --robot-code string   机器人 Code (必填)
```

---

## message info — 获取会话详情

```
Usage:
  dws chat message info [flags]
Example:
  dws chat message info --id <openConversationId>
  dws chat message info --open-id <openDingTalkId>
Flags:
      --id string        群聊 openConversationId（与 --open-id 二选一）
      --open-id string   用户 openDingTalkId（单聊时与 --id 二选一）
```

---

## message list — 拉取群聊消息

> 仅群聊。单聊用 `message list-direct`。

```
Usage:
  dws chat message list [flags]
Example:
  dws chat message list --id <openconversation_id> --time "2025-03-01 00:00:00"
  dws chat message list --id <openconversation_id> --time "2025-03-01 00:00:00" --limit 50
  dws chat message list --id <openconversation_id> --time "2025-03-01 00:00:00" --forward false
Flags:
      --id string        群聊 openconversation_id (必填)
      --time string      起始时间，格式: yyyy-MM-dd HH:mm:ss (必填)
      --forward string   "true"=拉给定时间之后（默认） / "false"=拉给定时间之前
      --limit string     返回数量

注意:
  - 真实 flag 是 --id（不是 --group）
  - --forward 类型是 string，需传字符串 "true"/"false"
  - 翻页：hasMore=true 时，用结果中边界 createTime 作为下次 --time
  - 如果返回消息含 openConvThreadId，表示是话题消息，需用 message list-topic-replies 拉话题回复
```

---

## message list-direct — 拉取单聊消息

```
Usage:
  dws chat message list-direct [flags]
Example:
  dws chat message list-direct --user <userId> --time "2025-03-01 00:00:00" --limit 50
  dws chat message list-direct --open-id <openDingTalkId> --time "2025-03-01 00:00:00"
Flags:
      --user string       单聊对方 userId（与 --open-id 二选一）
      --open-id string    单聊对方 openDingTalkId（与 --user 二选一，三方应用等无 userId 场景）
      --time string       起始时间，格式: yyyy-MM-dd HH:mm:ss (必填)
      --forward string    "true"=拉给定时间之后（默认） / "false"=拉给定时间之前
      --limit string      返回数量
```

---

## message list-topic-replies — 拉取群话题回复

```
Usage:
  dws chat message list-topic-replies [flags]
Example:
  dws chat message list-topic-replies --id <openconversation_id> --topic <topicId>
  dws chat message list-topic-replies --id <openconversation_id> --topic <topicId> --start "2025-03-01 00:00:00" --size 20
Flags:
      --id string        群会话 openconversationId (必填)
      --topic string     话题 ID，由 message list 返回的 openConvThreadId (必填)
      --start string     起始时间 startTime（可选）
      --forward string   "true"=从老往新 / "false"=从新往老
      --size string      每页返回数量 pageSize

注意:
  - 真实 flag 是 --topic（不是 --topic-id）、--size（不是 --limit）、--start（不是 --time）
```

---

## message list-focus — 拉取特别关注人的消息

```
Usage:
  dws chat message list-focus [flags]
Example:
  dws chat message list-focus --limit 50
  dws chat message list-focus --limit 20 --cursor <nextCursor>
Flags:
      --limit string    每页返回数量
      --cursor string   分页游标（首次传 "0"，翻页传 nextCursor）
```

---

## message list-top — 拉取置顶会话列表

> 用户说「置顶会话」直接调用此命令即可；用户说「置顶消息」时，先用此命令拿到各 `openConversationId`，再用 `chat message list --id` 逐会话拉消息。

```
Usage:
  dws chat message list-top [flags]
Example:
  dws chat message list-top --limit 1000
  dws chat message list-top --limit 1000 --cursor <nextCursor>
Flags:
      --limit string    每页返回数量
      --cursor string   分页游标（首次传 "0"，翻页传 nextCursor）
```

---

## message unread — 获取未读会话列表

```
Usage:
  dws chat message unread [flags]
Example:
  dws chat message unread
  dws chat message unread --count 20
Flags:
      --count string   返回未读会话条数（可选）
```

---

## message search — 按关键词搜索消息

```
Usage:
  dws chat message search [flags]
Example:
  dws chat message search --keyword "changefree" --start "2026-04-01T00:00:00+08:00" --end "2026-04-15T00:00:00+08:00" --limit 50 --cursor 0
  dws chat message search --keyword "codereview" --id <openConversationId> --start "2026-04-01T00:00:00+08:00" --end "2026-04-15T00:00:00+08:00" --limit 100 --cursor 0
Flags:
      --keyword string   搜索关键词 (必填)
      --id string        群聊 openConversationId（可选，不传则搜所有会话）
      --start string     起始时间，ISO-8601 格式 (必填)
      --end string       结束时间，ISO-8601 格式 (必填)
      --limit string     每页返回数量
      --cursor string    分页游标（首页传 "0"，翻页传 nextCursor）

注意:
  - 真实 flag 是 --id（不是 --group）
```

---

## message search-at-me — 拉取 @我 的消息

```
Usage:
  dws chat message search-at-me [flags]
Example:
  dws chat message search-at-me --start "2026-03-10T00:00:00+08:00" --end "2026-03-11T00:00:00+08:00" --limit 50 --cursor 0
  dws chat message search-at-me --id <openconversation_id> --start "2026-03-10T00:00:00+08:00" --end "2026-03-11T00:00:00+08:00"
Flags:
      --id string       群聊 openConversationId（可选，不传则查全部会话）
      --start string    起始时间，ISO-8601 格式 (必填)
      --end string      结束时间，ISO-8601 格式 (必填)
      --limit string    每页返回数量
      --cursor string   分页游标（首页传 "0"，翻页传 nextCursor）
```

---

## message search-by-sender — 拉取指定发送者发给我的消息

跨单聊+群聊，返回结果自带会话类型标识。

```
Usage:
  dws chat message search-by-sender [flags]
Example:
  dws chat message search-by-sender --user <userId> --start "2026-03-10T00:00:00+08:00" --end "2026-03-11T00:00:00+08:00" --limit 50 --cursor 0
  dws chat message search-by-sender --open-id <openDingTalkId> --start "2026-03-10T00:00:00+08:00" --end "2026-03-11T00:00:00+08:00"
Flags:
      --user string      发送者 userId（与 --open-id 二选一）
      --open-id string   发送者 openDingTalkId（与 --user 二选一）
      --start string     起始时间，ISO-8601 格式 (必填)
      --end string       结束时间，ISO-8601 格式 (必填)
      --limit string     每页返回数量
      --cursor string    分页游标（首页传 "0"，翻页传 nextCursor）

注意:
  - 真实 flag 是 --user / --open-id（不是 --sender-user-id / --sender-open-dingtalk-id）
```

---

## message search-by-time — 按时间范围拉取当前用户所有会话消息

拉取当前登录用户在时间范围内的所有会话消息（跨群聊+单聊）。

```
Usage:
  dws chat message search-by-time [flags]
Example:
  dws chat message search-by-time --start "2025-03-01 00:00:00" --end "2025-03-31 23:59:59" --limit 50
  dws chat message search-by-time --start "2025-03-01 00:00:00" --end "2025-03-31 23:59:59" --limit 50 --cursor "abc123token"
Flags:
      --start string    起始时间 (必填)
      --end string      结束时间 (必填)
      --limit string    每页返回数量
      --cursor string   分页游标（首页传 "0"，翻页传 nextCursor）

注意:
  - 与 message list 的区别：list 拉单个会话的消息；search-by-time 拉当前用户所有会话的消息
```

---

## message send — 以当前用户身份发**群聊**消息

> 仅群聊。单聊用 `message send-direct`。

```
Usage:
  dws chat message send [flags]
Example:
  dws chat message send --id <openconversation_id> --text "hello"
  dws chat message send --id <openconversation_id> --title "周报提醒" --text "请大家本周五前提交周报"
  dws chat message send --id <openconversation_id> --at-all true --text "<@all> 请大家注意"
  dws chat message send --id <openconversation_id> --at-users userId1,userId2 --text "<@userId1> <@userId2> 请查收"
Flags:
      --id string         群聊 openConversation_id (必填)
      --text string       消息内容（支持 Markdown）
      --title string      消息标题
      --at-all string     "true"=@所有人（仅群聊生效）
      --at-users string   @指定成员的 userId 列表，逗号分隔

注意:
  - 真实 flag 是 --id（不是 --group）
  - --at-all 类型是 string，传 "true" / "false"
  - --at-all 时消息内容中必须包含占位符 <@all>；--at-users userId1,userId2 时消息内容中必须包含对应 <@userId1> <@userId2> 占位符
```

---

## message send-direct — 以当前用户身份发**单聊**消息

```
Usage:
  dws chat message send-direct [flags]
Example:
  dws chat message send-direct --user <userId> --text "你好"
  dws chat message send-direct --open-id <openDingTalkId> --title "通知" --text "请查收"
Flags:
      --user string      接收人 userId（与 --open-id 二选一）
      --open-id string   接收人 openDingTalkId（与 --user 二选一，三方应用等无 userId 场景）
      --text string      消息内容（支持 Markdown）
      --title string     消息标题
```

---

## message send-personal — 发送个人消息

> ⚠️ 敏感操作：执行前必须向用户确认，同意后才加 `--yes`。

```
Usage:
  dws chat message send-personal [flags]
Example:
  dws chat message send-personal --id <openConversationId> --content "你好" --type text
  dws chat message send-personal --open-id <openDingTalkId> --content "消息内容" --type text
  dws chat message send-personal --id <openConversationId> --content "内容" --at-all true
Flags:
      --content string    消息内容 (必填)
      --type string       消息类型，如 text / markdown (必填)
      --id string         群聊 openConversationId（与 --open-id 二选一）
      --open-id string    接收人 openDingTalkId（与 --id 二选一）
      --at-all string     "true"=@所有人（可选）
      --at-users string   @指定人的 openDingTalkId 列表，逗号分隔（可选）
```

---

## message send-by-bot — 机器人发消息

群聊：传 `--group`；单聊：传 `--users`，二者互斥。`--text` 支持 Markdown。

```
Usage:
  dws chat message send-by-bot [flags]
Example:
  dws chat message send-by-bot --robot-code <robot-code> --group <openconversation_id> --title "日报" --text "## 今日完成..."
  dws chat message send-by-bot --robot-code <robot-code> --users userId1,userId2 --title "提醒" --text "请提交周报"
Flags:
      --robot-code string   机器人 Code (必填)
      --group string        群会话 openConversationId (群聊必填)
      --users string        接收者 userId 列表，逗号分隔，最多 20 个 (单聊必填)
      --text string         消息内容 (Markdown)
      --title string        消息标题

注意:
  - --group 与 --users 互斥，必须且只能指定其一
  - 此命令优于 `chat bot message send-by-bot`（后者是 stub 版，仅单聊）
```

---

## message send-by-webhook — 自定义机器人 Webhook 发群消息

@ 人时需在 `--text` 中包含 `@userId` 或 `@手机号`，否则 @ 不生效。

```
Usage:
  dws chat message send-by-webhook [flags]
Example:
  dws chat message send-by-webhook --token <webhook-token> --title "告警" --text "CPU 超 90%" --at-all
  dws chat message send-by-webhook --token <webhook-token> --title "test" --text "hi @118785" --at-users 118785
Flags:
      --token string        Webhook token (必填)
      --title string        消息标题 (必填)
      --text string         消息内容 (必填)
      --at-all              @所有人（bool flag，不带参数值）
      --at-mobiles string   按手机号 @，逗号分隔
      --at-users string     按 userId @，逗号分隔
```

---

## message recall-by-bot — 机器人撤回消息

群聊：传 `--group` 与 `--keys`；单聊：仅传 `--keys`。`--keys` 为 `send-by-bot` 返回的 `processQueryKey` 列表。

```
Usage:
  dws chat message recall-by-bot [flags]
Example:
  dws chat message recall-by-bot --robot-code <robot-code> --group <openconversation_id> --keys <process-query-key>
  dws chat message recall-by-bot --robot-code <robot-code> --keys key1,key2
Flags:
      --robot-code string   机器人 Code (必填)
      --group string        群会话 openConversationId（群聊撤回必填）
      --keys string         逗号分隔的消息 processQueryKey 列表 (必填)
```

---

## bot bot search — 搜索我的机器人

> 注意路径：真实 CLI 是 `chat bot bot search`（双 bot 前缀）。

```
Usage:
  dws chat bot bot search [flags]
Example:
  dws chat bot bot search --page 1
  dws chat bot bot search --page 1 --size 10 --name "日报"
Flags:
      --name string   按名称搜索（可选）
      --page string   页码，从 1 开始
      --size string   每页条数
```

---

## bot bot create — 创建企业机器人

```
Usage:
  dws chat bot bot create [flags]
Example:
  dws chat bot bot create --name "日报提醒机器人" --desc "负责每日日报提醒"
Flags:
      --name string   机器人名称 robot_name (必填)
      --desc string   机器人描述（可选）
```

---

## bot bot search-groups — 搜索机器人所在群

```
Usage:
  dws chat bot bot search-groups [flags]
Example:
  dws chat bot bot search-groups --keyword "项目"
  dws chat bot bot search-groups --keyword "冲刺" --cursor <nextCursor>
Flags:
      --keyword string   搜索关键词 (必填)
      --cursor string    分页游标（首页留空，翻页传返回的 cursor）
```

---

## 意图判断

用户说"建群/创建群聊" → `chat group create`
用户说"创建企业全员群/组织群" → `chat group create-org`
用户说"搜索群/找群" → `chat search`
用户说"群成员/看群里有谁" → `chat group members list`
用户说"拉人进群/加群成员" → `chat group members add`
用户说"踢人/移除群成员" → `chat group members remove`（⚠️ 敏感，需确认）
用户说"加机器人到群" → `chat group members add-bot`
用户说"改群名" → `chat group rename`
用户说"聊天记录/群消息/拉取群会话" → `chat message list --id`
用户说"和某人的单聊记录/单聊消息" → `chat message list-direct --user`（或 `--open-id`）
用户说"某人发给我的消息/指定发送者/某人的消息" → `chat message search-by-sender`（跨单聊/群聊，用户未明确说"单聊"时优先）
用户说"@我的消息/at我的/提及我的" → `chat message search-at-me`
用户说"未读消息会话/未读会话列表" → `chat message unread`
用户说"发群消息(以个人身份)" → `chat message send --id`
用户说"发单聊消息(以个人身份)" → `chat message send-direct --user`（或 `--open-id`）
用户说"发个人消息/个人通知" → `chat message send-personal`（⚠️ 敏感操作，需确认）
用户说"机器人发消息/机器人群发" → `chat message send-by-bot`
用户说"机器人撤回消息" → `chat message recall-by-bot`
用户说"Webhook 发消息/告警消息" → `chat message send-by-webhook`
用户说"话题回复/群话题消息回复" → `chat message list-topic-replies`
用户说"所有消息/全部会话消息/时间范围内消息/我的消息/最近的消息" → `chat message search-by-time`
用户说"特别关注人的消息/关注的人的消息/星标联系人" → `chat message list-focus`
用户说"查看我的机器人" → `chat bot bot search`
用户说"创建机器人" → `chat bot bot create`
用户说"搜索消息/查找关键词" → `chat message search`
用户说"我和XX的共同群/查共同群" → `chat group search-common`
用户说"置顶会话/置顶消息/我的置顶" → `chat message list-top`
用户说"获取会话信息/会话详情" → `chat message info`
用户说"机器人在哪些群" → `chat bot bot search-groups`

### 关键区分

- `chat message list` — 拉**群聊**消息（`--id` 群 ID + `--time` 时间点 + `--forward` 方向），按时间翻页
- `chat message list-direct` — 拉**单聊**消息（`--user` 或 `--open-id`）
- `chat message search-by-sender` — 按发送者搜消息，跨单聊+群聊
- `chat message search-at-me` — @我 的消息（跨单聊+群聊，可选 `--id` 限定群）
- `chat message unread` — 有未读的会话列表
- `chat message search-by-time` — 当前用户所有会话按时间范围分页拉取
- `chat message list-topic-replies` — 群话题回复
- `chat message list-focus` — 特别关注人的消息
- `chat message list-top` — 置顶会话列表
- `chat message send` — 当前用户身份发**群聊**
- `chat message send-direct` — 当前用户身份发**单聊**
- `chat message send-personal` — 发个人消息（`--id` 群 / `--open-id` 人）（⚠️ 敏感）
- `chat message search` — 按关键词搜消息（可选 `--id` 限定群）
- `chat group search-common` — 搜索共同群（AND=所有人都在 / OR=任一人在）
- `chat message send-by-bot` — 机器人发消息（`--group` 群 / `--users` 单聊批量）
- `chat message send-by-webhook` — 自定义 Webhook 发群消息
- `chat message recall-by-bot` — 机器人撤回已发消息
- `chat message info` — 获取会话详情

---

## 核心工作流

```bash
# 1. 搜索群 — 提取 openconversation_id
dws chat search --query "项目冲刺" --format json

# 2. 拉群聊消息
dws chat message list --id <openconversation_id> --time "2025-03-01 00:00:00" --format json

# 2b. 拉未读会话
dws chat message unread --count 20 --format json

# 3. 个人身份发群消息
dws chat message send --id <openconversation_id> --title "周报提醒" --text "请大家本周五前提交周报" --format json

# 4. 个人身份单聊（userId）
dws chat message send-direct --user <userId> --text "你好" --format json

# 4b. 个人身份单聊（openDingTalkId，三方应用场景）
dws chat message send-direct --open-id <openDingTalkId> --text "你好" --format json

# 5. 机器人发群消息（Markdown）
dws chat message send-by-bot --robot-code <robot-code> \
  --group <openconversation_id> --title "日报" --text "## 今日完成..." --format json

# 6. 机器人批量单聊
dws chat message send-by-bot --robot-code <robot-code> \
  --users userId1,userId2 --title "提醒" --text "请提交周报" --format json

# 7. Webhook 发告警
dws chat message send-by-webhook --token <webhook-token> \
  --title "告警" --text "CPU 超 90%" --at-all --format json
```

## 复合工作流

### 机器人发消息后撤回

撤回只能用于 `send-by-bot` 发出的消息。个人身份 (`chat message send` / `send-direct`) 发出的消息**无法通过 API 撤回**。

```bash
# Step 1: 查我的机器人 — 提取 robotCode
dws chat bot bot search --format json

# Step 2: 机器人发消息 — 提取返回的 processQueryKey
dws chat message send-by-bot --robot-code <robot-code> --group <openconversation_id> \
  --title "通知" --text "内容" --format json

# Step 3: 撤回
dws chat message recall-by-bot --robot-code <robot-code> --group <openconversation_id> \
  --keys <processQueryKey> --format json
```

### 创建并使用机器人

```bash
# Step 1: 创建机器人
dws chat bot bot create --name "项目提醒机器人" --desc "项目状态提醒" --format json

# Step 2: 搜索群 — 提取 openConversationId
dws chat search --query "项目群" --format json

# Step 3: 把机器人加入群
dws chat group members add-bot --id <openConversationId> --robot-code <robotCode> --format json

# Step 4: 机器人发消息
dws chat message send-by-bot --robot-code <robotCode> --group <openConversationId> \
  --title "提醒" --text "请及时更新项目状态" --format json
```

### 机器人 @指定人发群消息

`--text` 中**必须**包含 `<@userId>` 占位符，否则 @ 不生效。

```bash
# Step 1: 搜人获取 userId
dws aisearch person --keyword "张三" --dimension name --format json

# Step 2: 取 userId 发送（注意 text 中的占位符）
dws chat message send-by-bot --robot-code <robot-code> --group <openconversation_id> \
  --title "提醒" --text "<@userId1> <@userId2> 请查收本周报告" --format json
```

### 发送图片/文件消息（跨产品: drive → chat）

> 当前 `chat message send` 没有 `--media-id` 原生图片消息 flag，文件/图片通过 Markdown 链接形式发送。

```bash
# Step 1: 获取上传凭证 — 拿 uploadId 和 upload URL
dws drive get-upload-info --file-name "截图.png" --file-size <字节数> --format json

# Step 2: HTTP PUT 上传到 OSS
curl -X PUT -T "截图.png" "<get-upload-info 返回的上传 URL>"

# Step 3: 提交上传 — 获取 dentryUuid
dws drive commit-upload --file-name "截图.png" --file-size <字节数> --upload-id <uploadId> --format json

# Step 4: 获取下载链接
dws drive download-file --file-id <dentryUuid> --format json

# Step 5: 用 Markdown 图片语法发送
dws chat message send --id <openconversation_id> \
  --text "![截图](下载链接)" --format json
```

## 上下文传递表

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `chat search` | `openConversationId` | `message send --id`、`message list --id`、`group members *` 的 `--id` 等 |
| `chat group create` / `create-org` | `openConversationId` | 同上 |
| `chat group search-common` | `openConversationId` | 同上 |
| `chat message list` | 边界 `createTime` | 下次 `message list` 的 `--time` 翻页 |
| `chat message list` | `openConvThreadId` | `message list-topic-replies` 的 `--topic` |
| `chat message search-by-time` / `search` / `search-at-me` / `search-by-sender` | `nextCursor` | 下次同命令的 `--cursor` |
| `aisearch person` | `userId` | `message send --at-users`、`send-direct --user`、`send-by-bot --users`、`search-by-sender --user` |
| `aisearch person` → `contact user get` | `openDingTalkId` | `send-direct --open-id`、`list-direct --open-id`、`search-by-sender --open-id`、`message info --open-id` |
| `chat bot bot search` | `robotCode` | `send-by-bot` / `recall-by-bot` 的 `--robot-code`、`group members add-bot` 的 `--robot-code` |
| `chat bot bot create` | `robotCode` | 同上 |
| `chat message send-by-bot` | `processQueryKey` | `recall-by-bot` 的 `--keys` |
| `drive download-file` | 下载链接 | `message send --text` 的 Markdown 图片/链接语法 |

## 注意事项

### 路径 & 命名
- **双 bot 路径**：机器人管理命令在 `chat bot bot`（不是 `chat bot`）下，如 `chat bot bot search`、`chat bot bot create`、`chat bot bot search-groups`
- **单聊 / 群聊分开命令**：`message list` / `message send` 仅群聊；单聊对应 `message list-direct` / `message send-direct`
- **flag 名对齐**：真实 CLI 普遍用 `--id`（群 ID）、`--open-id`（openDingTalkId）、`--topic`、`--mode`、`--size`；文档里历史出现过的 `--group`、`--topic-id`、`--match-mode`、`--limit` 在某些命令上是错的，请以各命令章节标注为准

### 参数类型
- 真实 CLI 对很多数值/布尔 flag 使用 `string` 类型（如 `--limit string`、`--forward string`、`--at-all string`），需要传字符串形式（`"20"`、`"true"`、`"false"`）；部分命令例外（如 `send-by-webhook --at-all` 是 bool，无需传值）
- 时间字段：`message list` / `list-direct` / `list-topic-replies` 用 `yyyy-MM-dd HH:mm:ss`；`message search*` / `search-at-me` / `search-by-sender` / `search-by-time` 用 ISO-8601（如 `2026-03-10T00:00:00+08:00`）

### 敏感/危险操作（需用户确认后才加 `--yes`）
- `chat group members remove` — 移除群成员
- `chat message send-personal` — 发送个人消息

### 重复/别名（选 polished 版）
- `chat list-conversation-message` → ⚠️ legacy，请改用 `chat message list --id`
- `chat bot message send-by-bot` / `recall-by-bot` / `send-by-webhook` → stub 版（仅单聊/参数少），请改用 `chat message send-by-bot` / `recall-by-bot` / `send-by-webhook`
- `chat bot search` ≈ `chat bot bot search`（短路别名），推荐显式写 `bot bot search`

## 相关产品

- [contact](./contact.md) — 搜同事获取 userId（用于 `message send --at-users`、`send-direct --user`、`send-by-bot --users`、`search-by-sender --user`）；获取 openDingTalkId（用于 `*-direct --open-id`、`search-by-sender --open-id`、`message info --open-id`）
- [drive](./drive.md) — 上传文件得下载链接，用 Markdown 图片/文件语法发送
