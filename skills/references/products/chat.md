# 会话与群聊 (chat) 命令参考

> 命令别名: `dws im` 等价于 `dws chat`

## 命令总览

### group (群组管理)

| 子命令 | 用途 |
|-------|------|
| `group create` | 创建内部群 |
| `group members` | 查看群成员列表 |
| `group members add` | 添加群成员 |
| `group members remove` | 移除群成员（⚠️ 危险操作） |
| `group members add-bot` | 添加机器人到群 |
| `group rename` | 修改群名称 |
| `search` | 搜索群会话 |
| `search-common` | 搜索共同群 |
| `conversation-info` | 获取会话基础信息（单聊/群聊） |

### message (会话消息管理)

| 子命令 | 用途 |
|-------|------|
| `message send` | 以当前用户身份发群消息或单聊消息 |
| `message list` | 拉取群聊或单聊会话消息 |
| `message list-all` | 按时间范围拉取当前用户所有会话消息 |
| `message list-topic-replies` | 拉取群话题回复消息列表 |
| `message list-by-sender` | 搜索指定发送者的消息 |
| `message list-mentions` | 拉取 @我 的消息 |
| `message list-focused` | 拉取特别关注人的消息 |
| `message list-unread-conversations` | 获取未读会话列表 |
| `message search` | 按关键词搜索消息 |
| `message send-by-bot` | 机器人发消息（群聊或批量单聊） |
| `message recall-by-bot` | 机器人撤回消息 |
| `message send-by-webhook` | 自定义机器人 Webhook 发消息 |
| `list-top-conversations` | 拉取置顶会话列表 |

### bot (机器人管理)

| 子命令 | 用途 |
|-------|------|
| `bot search` | 搜索我的机器人 |

---

## group create — 创建内部群

当前登录用户自动成为群主。

```
Usage:
  dws chat group create [flags]
Example:
  dws chat group create --name "Q1 项目冲刺群" --users userId1,userId2,userId3
Flags:
      --users string    成员 userId 列表，用户本身会自动加入，无需包含，逗号分隔，不超过20个 (必填)
      --name string     群名称 (必填)
```

---

## group members — 查看群成员列表

分页查询指定群聊的成员。

```
Usage:
  dws chat group members [flags]
Example:
  dws chat group members --id <openconversation_id>
Flags:
      --cursor string   分页游标，首次从 0 开始
      --id string       群 ID / openconversation_id (必填)
```

---

## group members add — 添加群成员

向指定群聊添加成员，需传入群 ID 与用户 ID 列表。

```
Usage:
  dws chat group members add [flags]
Example:
  dws chat group members add --id <openconversation_id> --users userId1,userId2
Flags:
      --id string      群 ID / openconversation_id (必填)
      --users string   要添加的用户 userId 列表，逗号分隔 (必填)
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
      --users string   要移除的用户 userId 列表，逗号分隔 (必填)
```

---

## group members add-bot — 添加机器人到群

将自定义机器人添加到当前用户有管理权限的群聊中，如果没有权限则会报错。

```
Usage:
  dws chat group members add-bot [flags]
Example:
  dws chat group members add-bot --robot-code <robot-code> --id <openconversation_id>
Flags:
      --id string           群聊 openConversationId (必填)
      --robot-code string   机器人 Code (必填)
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
      --name string   修改后的群名称 (必填)
```

---

## search — 搜索群会话

根据名称搜索会话列表。

```
Usage:
  dws chat search [flags]
Example:
  dws chat search --query "项目冲刺"
Flags:
      --cursor string   分页游标 (首页留空)
      --query string    搜索关键词 (必填)
```

---

## search-common — 搜索共同群

根据昵称列表搜索共同群聊。--nicks 指定要搜索的人员昵称（逗号分隔，必填）。--match-mode 控制匹配模式：AND 表示所有人都在群里，OR 表示任一人在群里（默认 AND）。

```
Usage:
  dws chat search-common [flags]
Example:
  dws chat search-common --nicks "风雷,山乔" --limit 20 --cursor 0
  dws chat search-common --nicks "天鸡,乐函" --match-mode OR --limit 20 --cursor 0
  dws chat search-common --nicks "风雷,山乔,天鸡" --limit 10 --cursor <nextCursor>
Flags:
      --nicks string        要搜索的昵称列表，逗号分隔 (必填)
      --match-mode string   匹配模式：AND=所有人都在群里，OR=任一人在群里（默认 AND）
      --limit int           每页返回数量（默认 20）
      --cursor string       分页游标（默认 "0"，翻页传 nextCursor）

注意:
  - --nicks 传人员昵称（花名），逗号分隔，如 "风雷,山乔"
  - --match-mode AND 表示群里必须包含所有指定的人；OR 表示包含任意一人即可
  - 翻页：hasMore=true 时，用返回的 nextCursor 作为下次 --cursor
```

---

## message send — 以当前用户身份发消息

--group 指定群聊 ID 发群消息；--user 指定用户 userId 发单聊；--open-dingtalk-id 指定用户 openDingTalkId 发单聊。三者只能选其一，不能同时指定。消息内容为位置参数（恰好 1 个），支持 Markdown。可选 --title 作为消息标题。
--群聊时可选 --at-all @所有人，或 --at-users 指定成员（仅群聊时生效）。
--发送图片消息：指定 --media-id（通过 dt_media_upload 工具上传获得），自动设置 msgType=image，此时不需要传文本内容。

```
Usage:
  dws chat message send [flags] [<text>]
Example:
  dws chat message send --group <openconversation_id> --text "hello"
  dws chat message send --user <userId> --text "请查收"
  dws chat message send --open-dingtalk-id <openDingTalkId> --text "请查收"
  dws chat message send --group <openconversation_id> "hello"
  dws chat message send --group <openconversation_id> --title "周报提醒" --text "请大家本周五前提交周报"
  dws chat message send --group <openconversation_id> --at-all "<@all> 请大家注意"
  dws chat message send --group <openconversation_id> --at-users userId1,userId2 "<@userId1> <@userId2> 请查收"
  dws chat message send --group <openconversation_id> --media-id <mediaId>
  dws chat message send --open-dingtalk-id <openDingTalkId> --media-id <mediaId>
Flags:
      --text string              消息内容（推荐使用，也可用位置参数）
      --group string             群聊 openconversation_id（群聊时必填）
      --user string              接收人 userId（单聊时与 --open-dingtalk-id 二选一）
      --open-dingtalk-id string  接收人 openDingTalkId（单聊时与 --user 二选一，适用于三方应用等无法获取 userId 的场景）
      --title string             消息标题（可选，默认「消息」）
      --at-all                   @所有人（仅群聊时生效，可选，默认 false）
      --at-users string          @指定成员的 userId 列表，逗号分隔（仅群聊时生效，可选）
      --media-id string          图片 mediaId（通过 dt_media_upload 工具上传获得，需从返回链接中去除 _宽_高.格式 后缀并加上 @ 前缀），指定后发送图片消息，不需要传文本内容
      --msg-type string          消息类型（可选，如 text/markdown/image/file；通常由 --text/--media-id/--dentry-id 自动推断）
      --dentry-id string         钉盘文件 dentryId（发送钉盘文件消息时使用，需配合 --space-id）
      --space-id string          钉盘空间 spaceId（与 --dentry-id 配合使用）
      --file-name string         文件消息的文件名
      --file-size string         文件消息的文件大小（字节）
      --file-type string         文件消息的文件类型（如 pdf / docx / xlsx 等）

注意:
  - --text 和位置参数二选一，--text 优先
  - --group、--user、--open-dingtalk-id 三者互斥，只需指定其一：群聊用 --group，单聊用 --user 或 --open-dingtalk-id
  - --group 的别名: --id, --chat, --conversation-id (均可替代 --group)
  - --at-all 和 --at-users 仅在 --group 群聊时生效；当设置--at-all时，消息内容中一定要包含对应的占位符<@all>；当设置--at-users userId1,userId2时，消息内容中一定要包含对应格式的占位符<@userId1> <@userId2>
  - --media-id 指定图片 mediaId 时自动发送图片消息（msgType=image），不需要传 --text；图片单聊仅支持 --open-dingtalk-id，不支持 --user
  - 发送钉盘文件消息：传 --dentry-id + --space-id（必要时配合 --file-name / --file-size / --file-type），msg-type 自动推断为 file
```

---

## message list — 拉取会话消息内容

拉取指定群聊或单聊的会话消息内容。

--group 指定群聊，--user 指定单聊用户（通过 userId），--open-dingtalk-id 指定单聊用户（通过 openDingTalkId），三者互斥。默认拉取给定时间之后的消息，--forward=false 拉之前的。hasMore=true 时用结果中的边界 createTime 作为下次 --time 翻页。

```
Usage:
  dws chat message list [flags]
Example:
  dws chat message list --group <openconversation_id> --time "2025-03-01 00:00:00"
  dws chat message list --user <userId> --time "2025-03-01 00:00:00" --limit 50
  dws chat message list --open-dingtalk-id <openDingTalkId> --time "2025-03-01 00:00:00" --limit 50
  dws chat message list --group <openconversation_id> --time "2025-03-01 00:00:00" --forward=false
Flags:
      --forward                  true=拉给定时间之后的消息，false=拉给定时间之前的消息 (default true)
      --group string             群聊 openconversation_id（群聊时必填）
      --limit int                返回数量，不传则不限制
      --time string              开始时间，格式: yyyy-MM-dd HH:mm:ss (必填)
      --user string              单聊用户 userId（单聊时与 --open-dingtalk-id 二选一）
      --open-dingtalk-id string  单聊用户 openDingTalkId（单聊时与 --user 二选一，适用于三方应用等无法获取 userId 的场景）

注意:
  - --group、--user、--open-dingtalk-id 三者互斥，只需指定其一：群聊用 --group，单聊用 --user 或 --open-dingtalk-id
  - --group 的别名: --id, --chat, --conversation-id (均可替代 --group)
  - 翻页：hasMore=true 时，用结果中的边界 createTime 作为下次 --time
  - 如果返回的会话消息中包含 openConvThreadId 字段，说明是话题类消息，需要调用 dws chat message list-topic-replies 拉取话题的回复内容列表，openConvThreadId 作为 --topic-id 参数
```

---

## message list-all — 拉取指定时间范围内当前用户的所有会话消息

分页拉取当前登录用户在指定时间范围内的所有会话消息。

--start 和 --end 限定时间范围，--limit 指定每页数量，--cursor 传分页游标（首页传 "0"，后续从响应中的 nextCursor 获取）。

```
Usage:
  dws chat message list-all [flags]
Example:
  dws chat message list-all --start "2025-03-01 00:00:00" --end "2025-03-31 23:59:59" --limit 50
  dws chat message list-all --start "2025-03-01 00:00:00" --end "2025-03-31 23:59:59" --limit 50 --cursor "abc123token"
Flags:
      --start string    起始时间，格式: yyyy-MM-dd HH:mm:ss (必填)
      --end string      结束时间，格式: yyyy-MM-dd HH:mm:ss (必填)
      --limit int       每页返回数量（默认 50）
      --cursor string   分页游标（首页传 "0"，后续从响应中的 nextCursor 获取）

注意:
  - 四个参数每次请求都会传递给服务端，cursor 首页传 "0"
  - 与 chat message list 的区别：list 拉取指定单个会话（群聊或单聊）的消息，list-all 拉取当前用户所有会话的消息
  - 翻页：hasMore=true 时，用响应中的 nextCursor 值作为下次 --cursor 参数继续翻页
  - 时间格式统一为 yyyy-MM-dd HH:mm:ss
```

---

## message list-topic-replies — 拉取群话题回复消息列表

查询指定群聊中某条话题消息的全部回复。--group 指定群会话 ID，--topic-id 指定话题 ID（由 dws chat message list 返回）。

```
Usage:
  dws chat message list-topic-replies [flags]
Example:
  dws chat message list-topic-replies --group <openconversation_id> --topic-id <topicId>
  dws chat message list-topic-replies --group <openconversation_id> --topic-id <topicId> --time "2025-03-01 00:00:00" --limit 20
Flags:
      --group string      群会话 openconversationId (必填)
      --topic-id string   话题 ID，由 dws chat message list 返回 (必填)
      --time string       开始时间，格式: yyyy-MM-dd HH:mm:ss（可选）
      --limit int         返回数量（默认 50）
      --forward           true=从老往新，false=从新往老（默认 false）
```

---

## message list-by-sender — 拉取指定发送者的消息

搜索特定人发送给我的消息，返回结果包含单聊和群聊标识。--sender-user-id 指定发送者 userId，--sender-open-dingtalk-id 指定发送者 openDingTalkId，二者互斥。

```
Usage:
  dws chat message list-by-sender [flags]
Example:
  dws chat message list-by-sender --sender-user-id <userId> --start "2026-03-10T00:00:00+08:00" --end "2026-03-11T00:00:00+08:00" --limit 50 --cursor 0
  dws chat message list-by-sender --sender-open-dingtalk-id <openDingTalkId> --start "2026-03-10T00:00:00+08:00" --end "2026-03-11T00:00:00+08:00" --limit 50 --cursor 0
Flags:
      --sender-user-id string              发送者 userId（与 --sender-open-dingtalk-id 二选一）
      --sender-open-dingtalk-id string     发送者 openDingTalkId（与 --sender-user-id 二选一）
      --start string                       开始时间，ISO-8601 格式 (必填)
      --end string                         结束时间，ISO-8601 格式 (必填)
      --limit int                          每页返回数量（默认 50）
      --cursor string                      分页游标（默认 "0"，翻页传 nextCursor）

注意:
  - --sender-user-id 和 --sender-open-dingtalk-id 二者互斥，必须且只能指定其一
  - 不需要指定单聊/群聊，MCP 返回结果自带会话类型标识
  - 时间支持多种 ISO-8601 格式，如 "2026-03-10T00:00:00+08:00"、"2026-03-10 14:00:00" 等
  - 翻页：hasMore=true 时，用返回的 nextCursor 作为下次 --cursor
```

---

## message list-mentions — 拉取 @我 的消息

搜索时间范围内 @我 的消息，可选指定群聊。

```
Usage:
  dws chat message list-mentions [flags]
Example:
  dws chat message list-mentions --start "2026-03-10T00:00:00+08:00" --end "2026-03-11T00:00:00+08:00" --limit 50 --cursor 0
  dws chat message list-mentions --group <openconversation_id> --start "2026-03-10T00:00:00+08:00" --end "2026-03-11T00:00:00+08:00" --limit 50 --cursor 0
Flags:
      --group string    群聊 openconversation_id（可选，不传则查全部）
      --start string    开始时间，ISO-8601 格式 (必填)
      --end string      结束时间，ISO-8601 格式 (必填)
      --limit int       每页返回数量（默认 50）
      --cursor string   分页游标（默认 "0"，翻页传 nextCursor）

注意:
  - --group 可选，不传则查询所有会话中 @我 的消息；传入则只查指定群聊
  - --group 的别名: --id, --chat, --conversation-id (均可替代 --group)
  - 翻页：hasMore=true 时，用返回的 nextCursor 作为下次 --cursor
```

---

## message list-focused — 拉取特别关注人的消息

拉取当前用户特别关注人的消息。

```
Usage:
  dws chat message list-focused [flags]
Example:
  dws chat message list-focused --limit 50
  dws chat message list-focused --limit 20 --cursor <nextCursor>
Flags:
      --limit int       每页返回数量（默认 50）
      --cursor int64    分页游标（首次不传或传 0，翻页传 nextCursor）

注意:
  - 首次调用不传 --cursor 或传 0，后续翻页传 nextCursor
```

---

## message list-unread-conversations — 获取未读会话列表

获取当前用户有未读消息的会话信息。可选通过 `--count` 限制返回条数。

```
Usage:
  dws chat message list-unread-conversations [flags]
Example:
  dws chat message list-unread-conversations
  dws chat message list-unread-conversations --count 20
Flags:
      --count int    返回未读会话条数（可选）
```

---

## message search — 按关键词搜索消息

在当前用户的会话中按关键词搜索消息。--keyword 必填，可选 --group 限定搜索某个会话。

```
Usage:
  dws chat message search [flags]
Example:
  dws chat message search --keyword "changefree" --start "2026-04-01T00:00:00+08:00" --end "2026-04-15T00:00:00+08:00" --limit 50 --cursor 0
  dws chat message search --keyword "codereview" --group <openconversation_id> --start "2026-04-01T00:00:00+08:00" --end "2026-04-15T00:00:00+08:00" --limit 100 --cursor 0
Flags:
      --keyword string   搜索关键词 (必填)
      --group string     群聊 openconversation_id（可选，不传则搜索所有会话）
      --start string     开始时间，ISO-8601 格式 (必填)
      --end string       结束时间，ISO-8601 格式 (必填)
      --limit int        每页返回数量（默认 100）
      --cursor string    分页游标（默认 "0"，翻页传 nextCursor）

注意:
  - --group 可选，不传则搜索所有会话中的消息；传入则只搜索指定会话
  - --group 的别名: --id, --chat, --conversation-id (均可替代 --group)
  - 翻页：hasMore=true 时，用返回的 nextCursor 作为下次 --cursor
```

---

## conversation-info — 获取会话基础信息

按会话 ID 获取单聊或群聊的基础元数据（名称、类型、成员数等）。在对会话执行操作前用来确认上下文。

```
Usage:
  dws chat conversation-info [flags]
Example:
  dws chat conversation-info --group <openConversationId>
  dws chat conversation-info --open-dingtalk-id <openDingTalkId>
Flags:
      --group string             群聊会话 ID openConversationId（与 --open-dingtalk-id 二选一）
      --open-dingtalk-id string  用户 openDingTalkId（单聊时与 --group 二选一）

注意:
  - --group（群聊 openConversationId）和 --open-dingtalk-id（单聊用户 openDingTalkId）二选一
```

---

## list-top-conversations — 拉取置顶会话列表

拉取当前用户的置顶会话列表。

```
Usage:
  dws chat list-top-conversations [flags]
Example:
  dws chat list-top-conversations --limit 1000
  dws chat list-top-conversations --limit 1000 --cursor <nextCursor>
Flags:
      --limit int        每页返回数量（默认 1000）
      --cursor int       分页游标（首次不传或传 0，翻页传 nextCursor）

注意:
  - 用户询问"置顶会话"时，直接调用此命令返回置顶会话列表即可
  - 用户询问"置顶消息"时，需两步：先调用此命令拉取置顶会话列表获取各会话的 openConversationId，再用 `chat message list --group <openConversationId>` 分别拉取每个会话内的消息
  - 翻页：hasMore=true 时，用返回的 nextCursor 作为下次 --cursor
```

---

## bot search — 搜索我的机器人

```
Usage:
  dws chat bot search [flags]
Example:
  dws chat bot search --page 1
  dws chat bot search --page 1 --size 10 --name "日报"
Flags:
      --name string   按名称搜索
      --page int      页码，从1开始 (默认 1)
      --size int      每页条数 (默认 50)，别名: --limit
```

---

## message send-by-bot — 机器人发消息

群聊：传 --group 指定群；单聊：传 --users 指定用户列表，二者只能选其一，不能同时指定。--text 支持 Markdown。

```
Usage:
  dws chat message send-by-bot [flags]
Example:
  dws chat message send-by-bot --robot-code <robot-code> --group <openconversation_id> --title "日报" --text "## 今日完成..."
  dws chat message send-by-bot --robot-code <robot-code> --users userId1,userId2 --title "提醒" --text "请提交周报"
Flags:
      --group string        群聊 openConversationId（群聊时必填）
      --robot-code string   机器人 Code (必填)
      --text string         消息内容 Markdown (必填)
      --title string        消息标题 (必填)
      --users string        用户 userId 列表，逗号分隔，最多20个（单聊时必填）

注意:
  - --group 与 --users 互斥，必须且只能指定其一
  - --group 的别名: --id, --chat, --conversation-id (均可替代 --group)
```

---

## message recall-by-bot — 机器人撤回消息

群聊：传 --group 与 --keys；单聊：仅传 --keys。--keys 为发送时返回的 processQueryKey 列表，逗号分隔。

```
Usage:
  dws chat message recall-by-bot [flags]
Example:
  dws chat message recall-by-bot --robot-code <robot-code> --group <openconversation_id> --keys <process-query-key>
  dws chat message recall-by-bot --robot-code <robot-code> --keys key1,key2
Flags:
      --group string        群聊 openConversationId（群聊撤回时必填）
      --keys string         消息 processQueryKey 列表，逗号分隔 (必填)
      --robot-code string   机器人 Code (必填)
```

---

## message send-by-webhook — 自定义机器人 Webhook 发消息

@ 人时需在 --text 中包含 @userId 或 @手机号，否则 @ 不生效。

```
Usage:
  dws chat message send-by-webhook [flags]
Example:
  dws chat message send-by-webhook --token <webhook-token> --title "告警" --text "CPU 超 90%" --at-all
  dws chat message send-by-webhook --token <webhook-token> --title "test" --text "hi @118785" --at-users 118785
Flags:
      --at-all              @ 所有人
      --at-mobiles string   @ 指定手机号，逗号分隔
      --at-users string     @ 指定用户，逗号分隔（需在 text 中包含 @userId）
      --text string         消息内容 (必填)
      --title string        消息标题 (必填)
      --token string        Webhook Token (必填)
```

---

## 意图判断

用户说"建群/创建群聊" → `chat group create`
用户说"搜索群/找群" → `chat search`
用户说"群成员/看群里有谁" → `chat group members`
用户说"拉人进群/加群成员" → `chat group members add`
用户说"踢人/移除群成员" → `chat group members remove`
用户说"加机器人到群" → `chat group members add-bot`
用户说"改群名" → `chat group rename`
用户说"聊天记录/会话消息/拉取会话" → `chat message list`
用户说"某人发给我的消息/指定发送者/某人的消息" → `chat message list-by-sender`（用户未明确说"单聊"时优先使用，跨单聊/群聊）
用户说"拉取和某人的单聊记录/单聊消息" → `chat message list --user`（用户明确说"单聊"时使用）
用户说"@我的消息/at我的/提及我的" → `chat message list-mentions`
用户说"未读消息会话/未读会话列表/我的未读会话" → `chat message list-unread-conversations`
用户说"发群消息(以个人身份)" → `chat message send --group`
用户说"发单聊消息(以个人身份)" → `chat message send --user`（有 userId 时）或 `chat message send --open-dingtalk-id`（有 openDingTalkId 时）
用户说"机器人发消息/机器人群发" → `chat message send-by-bot`
用户说"机器人撤回消息" → `chat message recall-by-bot`
用户说"Webhook 发消息/告警消息" → `chat message send-by-webhook`
用户说"话题回复/群话题消息回复/拉取话题回复" → `chat message list-topic-replies`
用户说"所有消息/全部会话消息/拉取全部消息/时间范围内消息/我的消息/我今天的消息/查我的钉钉消息/最近的消息" → `chat message list-all`
用户说"特别关注人的消息/关注的人的消息/星标联系人的消息" → `chat message list-focused`
用户说"查看我的机器人" → `chat bot search`
用户说"搜索消息/查找关键词/搜一下消息里的XX" → `chat message search`
用户说"我和XX的共同群/我们都在哪些群/查共同群" → `chat search-common`
用户说"置顶会话/置顶消息/我的置顶/查看置顶" → `chat list-top-conversations`
用户说"获取会话信息/会话详情/会话元数据" → `chat conversation-info`

关键区分:
- `chat message list` — 拉取指定会话的消息（需指定 --group 或 --user），按时间点 + 方向翻页
- `chat message list --user` — list 的单聊模式，拉取与指定用户的单聊记录（用户明确说"单聊""私聊"时使用）
- `chat message list-by-sender` — 搜索指定发送者发给我的消息，跨所有会话（单聊+群聊均包含，用户只说"某人发的消息"时优先使用）
- `chat message list-mentions` — 拉取 @我 的消息（跨单聊/群聊，可选指定群）
- `chat message list-unread-conversations` — 拉取当前用户存在未读消息的会话列表（可选 `--count`）
- `chat message list-all` — 拉取当前用户所有会话的消息，按时间范围 + cursor 分页。只要用户没有指定某个具体的会话（如某个群名、某个人名），即使提到"单聊消息""群聊消息"等笼统范围，也应路由到此命令
- `chat message list-topic-replies` — 拉取群话题的回复消息列表
- `chat message list-focused` — 拉取特别关注人的消息，cursor 分页
- `chat list-top-conversations` — 拉取置顶会话列表（用户询问"置顶会话"或"置顶消息"时路由到此），cursor 分页
- `chat message send` — 以**当前用户**身份发消息（群聊或单聊），text 为位置参数；支持 --media-id 发送图片消息
- `chat message search` — 按关键词搜索消息内容（跨所有会话，可选指定群）
- `chat search-common` — 搜索共同群，查询指定人共同所在的群聊（AND=所有人都在，OR=任一人在）
- `chat message send-by-bot` — 以**机器人**身份发消息（群聊或单聊），text 为 --text flag
- `chat message send-by-webhook` — 通过**自定义机器人 Webhook** 发群消息
- `chat message recall-by-bot` — 通过机器人撤回已发送的消息
- `chat conversation-info` — 按会话 ID 获取单聊/群聊的基础元数据（名称、类型、成员数等）
- `chat bot search` — 搜索当前用户名下的机器人，拿到 robotCode 用于 send-by-bot / recall-by-bot / group members add-bot

## 核心工作流

```bash
# 1. 搜索群 — 提取 openconversation_id
dws chat search --query "项目冲刺" --format json

# 2. 拉取群消息
dws chat message list --group <openconversation_id> --time "2025-03-01 00:00:00" --format json

# 2b. 拉取未读会话列表
dws chat message list-unread-conversations --count 20 --format json

# 3. 以个人身份发送群消息
dws chat message send --group <openconversation_id> --title "周报提醒" "请大家本周五前提交周报" --format json

# 4. 以个人身份单聊（通过 userId）
dws chat message send --user <userId> "你好" --format json

# 4b. 以个人身份单聊（通过 openDingTalkId，三方应用等无法获取 userId 时使用）
dws chat message send --open-dingtalk-id <openDingTalkId> "你好" --format json

# 5. 机器人发群消息（Markdown）
dws chat message send-by-bot --robot-code <robot-code> \
  --group <openconversation_id> --title "日报" --text "## 今日完成..." --format json

# 6. 机器人单聊发消息
dws chat message send-by-bot --robot-code <robot-code> \
  --users userId1,userId2 --title "提醒" --text "请提交周报" --format json

# 7. Webhook 发告警
dws chat message send-by-webhook --token <webhook-token> \
  --title "告警" --text "CPU 超 90%" --at-all --format json
```

## 复合工作流

### 机器人发消息后撤回（完整流程）

撤回只能用于 `send-by-bot` 发出的消息。个人身份 (`chat message send`) 发出的消息**无法通过 API 撤回**。

```bash
# Step 1: 查我的机器人 — 提取 robot-code
dws chat bot search --format json

# Step 2: 用机器人发消息 — 提取返回中的 processQueryKey
dws chat message send-by-bot --robot-code <robot-code> --group <openconversation_id> \
  --title "通知" --text "内容" --format json

# Step 3: 用同一个 robot-code + processQueryKey 撤回
dws chat message recall-by-bot --robot-code <robot-code> --group <openconversation_id> \
  --keys <processQueryKey> --format json
```

### 将已有机器人加入群并发消息（完整流程）

机器人需先在钉钉开放平台创建好，这里只做"找机器人 → 加入群 → 发消息"。

```bash
# Step 1: 搜索我的机器人 — 提取 robotCode
dws chat bot search --name "项目提醒" --format json

# Step 2: 搜索群 — 提取 openConversationId
dws chat search --query "项目群" --format json

# Step 3: 将机器人添加到群（需当前用户对该群有管理权限）
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

```bash
# Step 1: 上传文件到钉盘 — 获取 uploadId 和凭证
dws drive upload-info --file-name "截图.png" --file-size <字节数> --format json

# Step 2: HTTP PUT 上传文件到 OSS
curl -X PUT -T "截图.png" "<upload-info 返回的上传 URL>"

# Step 3: 提交上传 — 获取 dentryUuid
dws drive commit --file-name "截图.png" --file-size <字节数> --upload-id <uploadId> --format json

# Step 4: 获取下载链接
dws drive download --file-id <dentryUuid> --format json

# Step 5: 用 Markdown 图片语法发送
dws chat message send --group <openconversation_id> \
  --text "![截图](下载链接)" --format json
```

## 上下文传递表

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `chat search` | `openConversationId` | message send/list、group members 等的 --group |
| `chat group create` | `openConversationId` | 同上 |
| `chat message list-all` | `nextCursor` | 下次 list-all 的 --cursor |
| `aisearch person` | `userId` | message send 的 --user、--at-users、send-by-bot 的 --users、list-by-sender 的 --sender-user-id |
| `aisearch person` → `contact user get` | `openDingTalkId` | list-by-sender 的 --sender-open-dingtalk-id、message send/list 的 --open-dingtalk-id |
| `chat bot search` | `robotCode` | send-by-bot / recall-by-bot 的 --robot-code、group members add-bot 的 --robot-code |
| `chat message send-by-bot` | `processQueryKey` | recall-by-bot 的 --keys |
| `chat conversation-info` | `openConversationId` / 成员信息 | 作为 message send/list 等后续操作的会话上下文确认 |
| `chat message search` | `nextCursor` | 下次 message search 的 --cursor |
| `chat search-common` | `openConversationId` | message send/list 等的 --group |
| `drive download` | 下载链接 | message send 的 Markdown 图片/链接语法 |

## 注意事项

- `--group` 为群聊会话 ID (openconversation_id)，可从群搜索或群聊信息中获取
- `chat message send` 的 text 是位置参数（恰好 1 个），非 flag；群聊用 `--group`，单聊用 `--user`（userId）或 `--open-dingtalk-id`（openDingTalkId），三者互斥；`--at-all`、`--at-users` 仅在 `--group` 群聊时生效；发送图片消息用 `--media-id`
- `chat message list-all` 的四个参数（--start、--end、--limit、--cursor）每次请求都必须传递；翻页时用响应中的 nextCursor 值作为下次 --cursor
- `chat message list` 的 `--group`、`--user`、`--open-dingtalk-id` 三者互斥，必须且只能指定其一
- `chat message list-by-sender` 不需要指定单聊/群聊，返回结果自带会话类型标识
- `chat message list-mentions` 可选 `--group` 指定群聊，不传则查全部
- `chat message list-unread-conversations` 获取当前用户未读会话列表，可选 `--count` 指定返回条数
- `chat message search` 按关键词搜索消息内容，`--keyword` 必填，可选 `--group` 限定搜索某个会话
- `chat search-common` 搜索共同群，`--nicks` 传人员昵称（逗号分隔），`--match-mode` AND/OR 控制匹配逻辑
- `chat list-top-conversations` 拉取置顶会话列表，分页用 `--limit`（默认 1000）/`--cursor`
- `send-by-bot` 群聊传 `--group`，单聊传 `--users`，二者互斥且必选其一
- `recall-by-bot` 群聊传 `--group` + `--keys`，单聊仅传 `--keys`（不传 `--group` 即为单聊撤回）
- `send-by-webhook` 支持 `--at-all`、`--at-mobiles`、`--at-users` 进行 @ 操作，但需在 `--text` 中包含 `@userId` 或 `@手机号` 才能生效

## 自动化脚本

| 脚本 | 场景 | 用法 |
|------|------|------|
| [chat_export_messages.py](../../scripts/chat_export_messages.py) | 导出群聊消息到 JSON 文件 | `python chat_export_messages.py --query "项目冲刺" --time "2026-03-10 00:00:00"` |
| [chat_history_with_user.py](../../scripts/chat_history_with_user.py) | 查询与某人的单聊聊天记录 | `python chat_history_with_user.py --name "张三" --time "2026-03-10 00:00:00"` |

## 相关产品

- [contact](./contact.md) — 搜索同事/好友，获取 userId 用于 --user、--at-users、send-by-bot --users、list-by-sender --sender-user-id；获取 openDingTalkId 用于 list-by-sender --sender-open-dingtalk-id、--open-dingtalk-id
- [drive](./drive.md) — 上传文件获取下载链接，用于 Markdown 图片/文件消息
