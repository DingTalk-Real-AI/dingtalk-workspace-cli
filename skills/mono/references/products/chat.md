# 会话与群聊 (chat) 命令参考

## 命令总览

### group (群组管理)

| 子命令 | 用途 |
|-------|------|
| `group create` | 创建内部群 |
| `group members list` | 查看群成员列表 |
| `group members add` | 添加群成员 |
| `group members remove` | 移除群成员（⚠️ 危险操作） |
| `group members add-bot` | 添加机器人到群 |
| `group rename` | 修改群名称 |
| `group get-by-group-id` | 按数字群号获取会话信息（群号转 openConversationId） |
| `group transfer-owner` | 转让群主 |
| `group invite-url` | 获取群邀请链接 |
| `group quit` | 退出群聊 |
| `group update-icon` | 更新群头像 |
| `group update-settings` | 更新群设置 |
| `group set-admin` | 设置/取消群管理员 |
| `group member-role` | 群自定义角色（add / list / update / remove / set-user / remove-user / query-user） |

### message (会话消息管理)

| 子命令 | 用途 |
|-------|------|
| `message send` | 以当前用户身份发消息（群聊或单聊） |
| `message recall-by-bot` | 撤回机器人发出的消息（开源 dws v1.0.30 不提供撤回普通用户消息的命令）|
| `message send-by-bot` | 机器人发消息（群聊或批量单聊） |
| `message recall-by-bot` | 机器人撤回消息 |
| `message send-by-webhook` | 自定义机器人 Webhook 发消息 |
| `message send-card` / `update-card` | 创建并推送流式卡片，配合更新内容 |
| `message reply` | 引用回复消息（单聊/群聊均可） |
| `message forward` | 转发单条消息（源/目标会话均支持单聊/群聊） |
| `message list` | 拉取群聊或单聊会话消息 |
| `message list-all` | 按时间范围拉取当前用户所有会话消息 |
| `message list-topic-replies` | 拉取群话题回复消息列表 |
| `message list-by-sender` | 搜索指定发送者的消息 |
| `message list-mentions` | 拉取 @我 的消息 |
| `message list-focused` | 拉取特别关注人的消息 |
| `message list-unread-conversations` | 获取未读会话列表 |
| `message list-by-ids` | 按消息 ID 批量获取消息 |
| `message search` | 按关键词搜索消息 |
| `message search-advanced` | 多维度搜索消息（推荐首选，严格超集） |
| `message query-send-status` | 查询消息发送状态（按 openTaskId） |
| `message query-read-status` | 查询消息已读/未读状态 |
| `message add-emoji` / `remove-emoji` | 给消息添加/移除 emoji 表情回应 |
| `message add-text-emotion` / `remove-text-emotion` | 添加/移除文字表情回应 |
| `message create-text-emotion` | 创建文字表情模板（返回 emotionId 供 add-text-emotion 使用） |

### chat 顶级命令

| 子命令 | 用途 |
|-------|------|
| `search` | 根据关键词搜索群聊 |
| `search-common` | 搜索共同群 |
| `conversation-info` | 获取会话基础信息（含 newCSpaceIdIM） |
| `list-top-conversations` | 拉取置顶会话列表 |
| `set-top` | 设置/取消会话置顶 |
| `mute` | 会话消息免打扰开关 |
| `group-mute` | 群全员禁言开关 |
| `group-mute-member` | 群成员禁言开关 |
| `list-categories` | 列出用户自定义会话分类 |
| `list-conversations` | 按分类拉取会话列表 |

### bot (机器人管理)

| 子命令 | 用途 |
|-------|------|
| `bot search` | 搜索我创建的机器人 |

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

## group members list — 查看群成员列表

分页查询指定群聊的成员。

```
Usage:
  dws chat group members list [flags]
Example:
  dws chat group members list --id <openconversation_id>
Flags:
      --cursor string   分页游标，首次从 0 开始
      --id string       群 ID / openconversation_id (必填)
```

> ⚠️ 注意：v1.0.17 起 list 是显式子命令；旧用法 `dws chat group members --id ...` 已不再支持，请改用 `dws chat group members list --id ...`。

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

根据关键词搜索群聊会话，分页返回匹配的群列表。

```
Usage:
  dws chat search [flags]
Example:
  dws chat search --query "项目冲刺"
  dws chat search --query "项目冲刺" --limit 20 --cursor 0
Flags:
      --keyword string   搜索关键词 (必填，旧版别名: --query)
      --limit int        每页返回数量（默认 20）
      --cursor string    分页游标（默认 "0"，翻页传 nextCursor）

注意:
  - 旧版命令使用 `--query`，新版统一改为 `--keyword`；`--query` 仍可作为别名使用
  - 翻页：hasMore=true 时，用返回的 nextCursor 作为下次 --cursor
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

--group 指定群聊 ID 发群消息；--user 指定用户 userId 发单聊；--open-dingtalk-id 指定用户 openDingTalkId 发单聊。三者只能选其一，不能同时指定。消息内容为位置参数（恰好 1 个），支持 Markdown。`--title` 是消息标题，**群聊与单聊都必填**（API 强制要求；缺失时服务端返回误导性的 "发群服务窗会话消息失败"，CLI 现在前置校验直接报错）。
--群聊时可选 --at-all @所有人，或 --at-users 指定成员（仅群聊时生效）。
--发送图片消息：指定 --media-id（通过 dt_media_upload 工具上传获得），自动设置 msgType=image，此时不需要传文本内容。

```
Usage:
  dws chat message send [flags] [<text>]
Example:
  dws chat message send --group <openconversation_id> --title "周报" --text "请提交本周日报"
  dws chat message send --user <userId> --title "提醒" --text "请查收"
  dws chat message send --open-dingtalk-id <openDingTalkId> --title "提醒" --text "请查收"
  dws chat message send --group <openconversation_id> --title "通知" "hello"
  dws chat message send --group <openconversation_id> --title "周报提醒" --text "请大家本周五前提交周报"
  dws chat message send --group <openconversation_id> --title "通知" --at-all "<@all> 请大家注意"
  dws chat message send --group <openconversation_id> --title "通知" --at-users userId1,userId2 "<@userId1> <@userId2> 请查收"
  dws chat message send --group <openconversation_id> --title "图片" --media-id <mediaId>
  dws chat message send --open-dingtalk-id <openDingTalkId> --title "图片" --media-id <mediaId>
Flags:
      --text string              消息内容（推荐使用，也可用位置参数）
      --group string             群聊 openconversation_id（群聊时必填）
      --user string              接收人 userId（单聊时与 --open-dingtalk-id 二选一）
      --open-dingtalk-id string  接收人 openDingTalkId（单聊时与 --user 二选一，适用于三方应用等无法获取 userId 的场景）
      --title string             消息标题（必填）
      --at-all                   @所有人（仅群聊时生效，可选，默认 false）
      --at-users string          @指定成员的 userId 列表，逗号分隔（仅群聊时生效，可选）
      --at-mobiles string        @指定成员的手机号列表，逗号分隔（仅群聊时生效，可选）
      --media-id string          图片 mediaId（通过 dt_media_upload 工具上传获得，需从返回链接中去除 _宽_高.格式 后缀并加上 @ 前缀），指定后发送图片消息，不需要传文本内容
      --msg-type string          消息类型（可选，如 text/markdown/image/file；通常由 --text/--media-id/--dentry-id 自动推断）
      --dentry-id string         钉盘文件 dentryId（发送钉盘文件消息时使用，需配合 --space-id）
      --space-id string          钉盘空间 spaceId（与 --dentry-id 配合使用）
      --file-name string         文件消息的文件名
      --file-size string         文件消息的文件大小（字节）
      --file-type string         文件消息的文件类型（如 pdf / docx / xlsx 等）

注意:
  - --text 和位置参数二选一，--text 优先
  - --title 必填（群聊与单聊都必填，API 强制要求）
  - --group、--user、--open-dingtalk-id 三者互斥，只需指定其一：群聊用 --group，单聊用 --user 或 --open-dingtalk-id
  - --group 的别名: --id, --chat, --conversation-id (均可替代 --group)
  - --at-all / --at-users / --at-mobiles 仅在 --group 群聊时生效；当设置--at-all时，消息内容中一定要包含对应的占位符<@all>；当设置--at-users userId1,userId2时，消息内容中一定要包含对应格式的占位符<@userId1> <@userId2>
  - --media-id 指定图片 mediaId 时自动发送图片消息（msgType=image），不需要传 --text；图片单聊仅支持 --open-dingtalk-id，不支持 --user
  - 发送钉盘文件消息：传 --dentry-id + --space-id（必要时配合 --file-name / --file-size / --file-type），msg-type 自动推断为 file
```

---

## message list — 拉取会话消息内容

拉取指定群聊或单聊的会话消息内容。

--group 指定群聊，--user 指定单聊用户（通过 userId），--open-dingtalk-id 指定单聊用户（通过 openDingTalkId），三者互斥。默认拉取给定时间之后的消息，--forward=false 拉之前的。

```
Usage:
  dws chat message list [flags]
Example:
  # 拉取群聊中某个时间点之后的消息
  dws chat message list --group <openconversation_id> --time "2025-03-01 00:00:00"
  # 拉取单聊消息（通过 userId）
  dws chat message list-direct --user <userId> --time "2025-03-01 00:00:00" --limit 50
  # 拉取单聊消息（通过 openDingTalkId）
  dws chat message list-direct --open-dingtalk-id <openDingTalkId> --time "2025-03-01 00:00:00" --limit 50
  # 拉取某个时间点之前的消息（向过去翻页）
  dws chat message list --group <openconversation_id> --time "2025-03-01 00:00:00" --forward=false
Flags:
      --forward                  true=拉给定时间之后的消息，false=拉给定时间之前的消息 (default true)
      --group string             群聊 openconversation_id（群聊时必填）
      --limit int                返回数量，不传则不限制
      --time string              开始时间，格式: yyyy-MM-dd HH:mm:ss（不传则默认拉取最新消息）
      --user string              单聊用户 userId（单聊时与 --open-dingtalk-id 二选一）
      --open-dingtalk-id string  单聊用户 openDingTalkId（单聊时与 --user 二选一，适用于三方应用等无法获取 userId 的场景）

注意:
  - --group、--user、--open-dingtalk-id 三者互斥，只需指定其一：群聊用 --group，单聊用 --user 或 --open-dingtalk-id
  - --group 的别名: --id, --chat, --conversation-id (均可替代 --group)
  - 如果返回的会话消息中包含 openConvThreadId 字段，说明是话题类消息，需要调用 dws chat message list-topic-replies 拉取话题的回复内容列表，openConvThreadId 作为 --topic-id 参数
```

### 分页翻页说明（重要）

`message list` 的翻页方式与 `message list-all` **完全不同**，请勿混淆：

| 命令 | 翻页参数 | 翻页值来源 | 值格式 |
|------|---------|-----------|--------|
| `message list` | `--time` | 上一页结果中**最后一条消息的 `createTime` 字段** | `yyyy-MM-dd HH:mm:ss`（如 `"2025-03-01 14:30:00"`） |
| `message list-all` | `--cursor` | 上一页响应中的 `nextCursor` 字段 | 字符串（如 `"abc123token"`） |

**翻页步骤（message list）：**

1. **首次请求**：指定起始时间
   ```bash
   dws chat message list --group <id> --time "2025-03-01 00:00:00" --limit 50 --format json
   ```
2. **检查响应**：查看 `hasMore` 字段
   - `hasMore: false` → 没有更多消息，翻页结束
   - `hasMore: true` → 还有更多消息，继续下一步
3. **获取翻页时间**：取返回结果中**最后一条消息**的 `createTime` 字段值（如 `"2025-03-01 14:30:00"`）
4. **下一页请求**：将该 `createTime` 作为 `--time` 传入
   ```bash
   dws chat message list --group <id> --time "2025-03-01 14:30:00" --limit 50 --format json
   ```
5. 重复步骤 2-4 直到 `hasMore: false`

> ⚠️ **常见错误**：
> - **不要把 `nextCursor` 传给 `--time`**：响应中的 `nextCursor` 字段（纯数字时间戳如 `1776684611219`）**不是给 `--time` 用的**。`--time` 只接受 `yyyy-MM-dd HH:mm:ss` 格式。将 `nextCursor` 传给 `--time` 会导致返回相同页面，陷入死循环。`nextCursor` 仅用于 `message list-all` 的 `--cursor` 参数。
> - **不要把 `nextCursor` 传给 `--forward`**：`--forward` 只接受 `true`（拉给定时间之后的消息）或 `false`（拉给定时间之前的消息），不是时间戳或游标参数。

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

---

## group get-by-group-id — 按数字群号获取会话信息

当用户只提供数字群号而非 openConversationId 时，用此命令把群号转换为 openConversationId，再调用其他需要 openConversationId 的命令。

```
Usage:
  dws chat group get-by-group-id [flags]
Example:
  dws chat group get-by-group-id --group-id 12345678
Flags:
      --group-id string   群号 (必填，数字字符串)
```

---

## group transfer-owner — 转让群主

将群主身份转让给群内其他成员。--new-owner 传新群主的 openDingTalkId。

```
Usage:
  dws chat group transfer-owner [flags]
Example:
  dws chat group transfer-owner --group <openConversationId> --new-owner <openDingTalkId>
  # 查询群 ID: dws chat search --query "群名"
  # 查询 openDingTalkId: dws contact user search --query "姓名"
Flags:
      --group string       群聊 openConversationId (必填)
      --new-owner string   新群主 openDingTalkId (必填)
```

---

## group invite-url — 获取群邀请链接

获取指定群聊的邀请加入链接。可选 --expires-seconds 指定链接有效期（秒），0 表示永久有效，不传则使用服务端默认值。

```
Usage:
  dws chat group invite-url [flags]
Example:
  dws chat group invite-url --group <openConversationId>
  dws chat group invite-url --group <openConversationId> --expires-seconds 86400
  dws chat group invite-url --group <openConversationId> --expires-seconds 0
Flags:
      --group string            群聊 openConversationId (必填)
      --expires-seconds int     链接有效期（秒），0=永久有效，不传使用服务端默认值
```

---

## group quit — 退出群聊

当前用户退出指定群聊。

```
Usage:
  dws chat group quit [flags]
Example:
  dws chat group quit --group <openConversationId>
Flags:
      --group string   群聊 openConversationId (必填)
```

---

## group update-icon — 更新群头像

更新指定群聊的群头像，--icon-media-id 传通过 media upload 获取的 mediaId。

```
Usage:
  dws chat group update-icon [flags]
Example:
  dws chat group update-icon --group <openConversationId> --icon-media-id <mediaId>
Flags:
      --group string           群聊 openConversationId (必填)
      --icon-media-id string   群头像 mediaId (必填，先用 media upload 上传获取)
```

---

## group update-settings — 更新群设置

--setting-key 指定设置项，--status 指定值（通常 "0"=关闭，"1"=开启；含义随 settingKey 变化）。

支持的 settingKey: `authority`、`joinValidation`、`onlyAdminCanAtAll`、`searchable`、`addFriendForbidden`、`toolbarStatus`、`pluginCustomizeVerify`、`onlyAdminCanDING`、`allMembersCanCreateMcsConf`、`onlyAdminCanSetMsgTop`、`onlyAdminCanPinMsg`、`onlyAdminCanSendFile`、`allMembersCanCreateCalendar`、`groupEmailDisabled`、`groupRedEnvelopeSwitch`、`groupLiveAuthority`、`groupBillAuthority`

```
Usage:
  dws chat group update-settings [flags]
Example:
  dws chat group update-settings --group <openConversationId> --setting-key searchable --status 1
  dws chat group update-settings --group <openConversationId> --setting-key onlyAdminCanAtAll --status 0
Flags:
      --group string         群聊 openConversationId (必填)
      --setting-key string   群设置项 key (必填)
      --status string        设置值: "0"=关闭, "1"=开启 (必填，字符串类型)
```

---

## group set-admin — 设置/取消群管理员

默认设为管理员，传 --off 取消。--users 传目标成员的 openDingTalkId 列表（逗号分隔）。

```
Usage:
  dws chat group set-admin [flags]
Example:
  dws chat group set-admin --group <openConversationId> --users openDingTalkId1,openDingTalkId2
  dws chat group set-admin --group <openConversationId> --users openDingTalkId1 --off
Flags:
      --group string   群聊 openConversationId (必填)
      --users string   目标成员 openDingTalkId 列表 (必填，逗号分隔)
      --off            传 --off 取消管理员（默认设为管理员）
```

---

## group member-role — 群自定义角色

`group member-role` 系列命令用于管理群的自定义身份/角色标签。包含 7 个子命令：

- `list` — 列出群自定义角色
- `add` — 新增群自定义角色
- `update` — 更新群自定义角色名称
- `remove` — 删除群自定义角色
- `set-user` — 给成员设置自定义角色（覆盖语义，传空则清除该成员所有角色）
- `remove-user` — 移除成员的指定自定义角色（不影响其他角色）
- `query-user` — 查询成员当前持有的所有自定义角色

```
Usage:
  dws chat group member-role list --group <openConversationId>
  dws chat group member-role add --group <openConversationId> --name "管理员"
  dws chat group member-role update --group <openConversationId> --role-id <openRoleId> --name "新名称"
  dws chat group member-role remove --group <openConversationId> --role-id <openRoleId>
  dws chat group member-role set-user --group <openConversationId> --user <openDingTalkId> --role-ids roleId1,roleId2
  dws chat group member-role remove-user --group <openConversationId> --user <openDingTalkId> --role-ids roleId1
  dws chat group member-role query-user --group <openConversationId> --user <openDingTalkId>

注意:
  - --user 传 openDingTalkId（不是 userId），可通过 `dws contact user search --query "姓名"` 获取
  - --role-id 来源于 `member-role list` 返回的 openRoleId
  - `set-user` 是覆盖语义：传空 --role-ids 则清除该用户所有角色
  - `remove-user` 是增量语义：仅移除指定角色，不影响其他角色
```

---

## message recall-by-bot — 撤回机器人发出的消息

> **能力范围说明**：开源 dws v1.0.30 仅提供 **机器人消息撤回**（`recall-by-bot`），不提供"撤回当前用户自己发出的消息"的命令。如果你想撤回的是普通用户消息，请在钉钉客户端长按消息撤回。

撤回机器人发出的消息，需要机器人 robotCode 和消息 processQueryKey。群聊撤回还需要 `--group <openConversationId>`。

```
Usage:
  dws chat message recall-by-bot [flags]
Example:
  # 群聊撤回机器人消息
  dws chat message recall-by-bot --robot-code <robot-code> --group <openConversationId> --keys <processQueryKey>
  # 单聊撤回机器人消息（不传 --group）
  dws chat message recall-by-bot --robot-code <robot-code> --keys key1,key2
Flags:
      --robot-code string   机器人 Code (必填)
      --group string        群会话 openConversationId (群聊撤回必填；单聊撤回不传)
      --keys string         逗号分隔的消息 processQueryKey 列表 (必填)
```

---

## message reply — 引用回复消息

以当前用户身份引用某条消息并回复，**单聊与群聊均可**。需指定会话 ID、被引用消息 ID、被引用消息发送者 openDingTalkId、回复正文。

```
Usage:
  dws chat message reply [flags]
Example:
  dws chat message reply --conversation-id <openConversationId> --ref-msg-id <openMessageId> --ref-sender <openDingTalkId> --text "收到，马上处理"
Flags:
      --conversation-id string   会话 openConversationId (必填，支持单聊/群聊)
      --ref-msg-id string        被引用的消息 openMessageId (必填，由 message list 返回)
      --ref-sender string        被引用消息发送者 openDingTalkId (必填)
      --text string              回复正文 (必填)
      --uuid string              幂等 UUID (可选)

注意:
  - --conversation-id 单聊和群聊使用同一字段；目前回复类型仅支持 text
```

---

## message forward — 转发消息

转发单条消息，**源/目标会话均支持单聊/群聊**，常见组合：群→群、群→单、单→群、单→单。

```
Usage:
  dws chat message forward [flags]
Example:
  dws chat message forward --src-conversation-id <srcOpenConversationId> --msg-id <openMessageId> --dest-conversation-id <destOpenConversationId>
Flags:
      --src-conversation-id string    源会话 openConversationId (必填)
      --msg-id string                 源消息 openMessageId (必填，由 message list 返回)
      --dest-conversation-id string   目标会话 openConversationId (必填)
```

---

## message list-by-ids — 按消息 ID 批量获取消息

根据消息 ID 列表批量获取消息内容（最多 50 条）。

```
Usage:
  dws chat message list-by-ids [flags]
Example:
  dws chat message list-by-ids --msg-ids msgId1,msgId2,msgId3
Flags:
      --msg-ids string   消息 openMessageId 列表 (必填，逗号分隔，最多 50 条)
```

---

## message query-send-status — 查询消息发送状态

查询通过 `message send` 等命令发送的消息的发送状态。需要传入发送时返回的 openTaskId。

```
Usage:
  dws chat message query-send-status [flags]
Example:
  dws chat message query-send-status --open-task-id <openTaskId>
Flags:
      --open-task-id string   消息任务 ID openTaskId (必填，来自 send/send-by-bot 返回值)
```

---

## message query-read-status — 查询消息已读/未读状态

查询指定会话中消息的已读/未读状态（**仅消息发送者**可查询自己发出的消息）。

```
Usage:
  dws chat message query-read-status [flags]
Example:
  dws chat message query-read-status --group <openConversationId> --msg-id <openMessageId>
Flags:
      --group string    会话 openConversationId (必填，群聊或单聊均可)
      --msg-id string   消息 openMessageId (必填，必须是当前用户发送的消息)

注意:
  - 仅消息发送者可查询自己发出的消息的已读/未读状态；查询他人发的消息会报错
  - --msg-id 从 `dws chat message list` 返回的消息列表中获取（字段名 openMessageId）
```

---

## message search-advanced — 多维度搜索消息（推荐首选）

> 推荐：这是消息搜索的首选接口。它可以完全替代 `chat message search`（keyword 可选 vs 必填，支持多个会话 vs 单个），大部分替代 `chat message list-by-sender`（通过 --sender-ids 按 openDingTalkId 搜索）和 `chat message list-mentions`（通过 --at-me 搜索 @我 的消息）。仅在以下场景需要退回其他命令：按 userId（非 openDingTalkId）搜发送者时用 list-by-sender，拉取「特别关注人」消息时用 list-focused。

支持按关键词、发送者、@我、@指定人、指定会话、时间范围等多维度组合搜索消息。所有参数均为可选，至少指定一个搜索条件。

```
Usage:
  dws chat message search-advanced [flags]
Example:
  dws chat message search-advanced --keyword "周报" --start "2026-04-01T00:00:00+08:00" --end "2026-04-15T00:00:00+08:00"
  dws chat message search-advanced --sender-ids <openDingTalkId1>,<openDingTalkId2> --start "2026-04-01T00:00:00+08:00" --end "2026-04-15T00:00:00+08:00"
  dws chat message search-advanced --at-me --start "2026-04-01T00:00:00+08:00" --end "2026-04-15T00:00:00+08:00"
  dws chat message search-advanced --at-ids <openDingTalkId1>,<openDingTalkId2> --conversation-ids <openConversationId1>,<openConversationId2> --limit 50 --cursor 0
  dws chat message search-advanced --conversation-ids <单聊openConversationId> --keyword "合同" --start "2026-04-01T00:00:00+08:00" --end "2026-04-15T00:00:00+08:00"
Flags:
      --keyword string            搜索关键词（可选）
      --sender-ids string         发送者 openDingTalkId 列表，逗号分隔（可选）
      --at-me                     只搜索 @我 的消息（可选，默认 false）
      --at-ids string             @指定人的 openDingTalkId 列表，逗号分隔（可选）
      --conversation-ids string   会话 openConversationId 列表，逗号分隔（可选，群聊或单聊均可，不传则搜索所有会话）
      --start string              开始时间，ISO-8601 格式（可选）
      --end string                结束时间，ISO-8601 格式（可选）
      --cursor string             分页游标（默认 "0"）
      --limit int                 每页返回数量（默认 100）

注意:
  - 所有参数均为可选，但至少需要指定一个搜索条件
  - --sender-ids 和 --at-ids 传 openDingTalkId（非 userId），可通过 `dws contact user search --query "姓名"` 获取
  - 群聊 openConversationId 通过 `dws chat search --query "群名"` 获取；单聊 openConversationId 通过 `dws chat conversation-info --open-dingtalk-id <openDingTalkId>` 获取
  - 时间支持多种 ISO-8601 格式，如 "2026-03-10T00:00:00+08:00"、"2026-03-10 14:00:00"、"2026-03-10" 等
  - 翻页：hasMore=true 时，用返回的 nextCursor 作为下次 --cursor
  - 替代关系：完全替代 search（严格超集）；大部分替代 list-by-sender（按 openDingTalkId 搜索）和 list-mentions（--at-me 覆盖核心功能）；不能替代 list-focused（「特别关注」是独立维度）
```

---

## 表情回应（emoji / 文字表情）

钉钉消息支持两种"表情回应"：内置 emoji 表情 + 自定义文字表情。优先用 emoji（参见 [chat-emoji-list.md](chat-emoji-list.md) 中的 199 个默认名称，如「赞」「鼓掌」「感谢」），命中即用 `add-emoji`；未命中先用 `create-text-emotion` 创建文字表情拿到 emotionId，再 `add-text-emotion` 贴上。

### message add-emoji — 给消息添加 emoji 表情回应

```
Usage:
  dws chat message add-emoji [flags]
Example:
  dws chat message add-emoji --group <openConversationId> --msg-id <openMessageId> --emoji "赞"
  dws chat message add-emoji --group <openConversationId> --msg-id <openMessageId> --emoji "鼓掌"
Flags:
      --group string    会话 openConversationId (必填，支持单聊/群聊)
      --msg-id string   消息 openMessageId (必填)
      --emoji string    emoji 表情名称 (必填，参见 chat-emoji-list.md 的 name 字段)
```

### message remove-emoji — 移除 emoji 表情回应

```
Usage:
  dws chat message remove-emoji [flags]
Example:
  dws chat message remove-emoji --group <openConversationId> --msg-id <openMessageId> --emoji "赞"
Flags:
      --group string    会话 openConversationId (必填，支持单聊/群聊)
      --msg-id string   消息 openMessageId (必填)
      --emoji string    emoji 表情名称 (必填)
```

### message create-text-emotion — 创建文字表情（获取 emotionId）

当 chat-emoji-list.md 中没有所需表情时，先创建再贴。

```
Usage:
  dws chat message create-text-emotion [flags]
Example:
  dws chat message create-text-emotion --emotion-name "赞" --text "nice"
  dws chat message create-text-emotion --emotion-name "感谢" --text "感谢" --background-id im_bg_5
Flags:
      --emotion-name string    表情名称 (必填)
      --text string            文字内容 (必填)
      --background-id string   背景 ID（可选，不传则由服务端默认分配）
```

### message add-text-emotion — 添加文字表情回应

需要先通过 `create-text-emotion` 创建拿到 emotionId 才能贴。六个参数全部必填。

```
Usage:
  dws chat message add-text-emotion [flags]
Example:
  dws chat message add-text-emotion --group <openConversationId> --msg-id <openMessageId> --emotion-id <emotionId> --emotion-name "赞" --text "nice" --background-id im_bg_5
Flags:
      --group string           会话 openConversationId (必填，支持单聊/群聊)
      --msg-id string          消息 openMessageId (必填)
      --emotion-id string      表情 ID (必填，通过 create-text-emotion 获取)
      --emotion-name string    表情名称 (必填)
      --text string            文字内容 (必填)
      --background-id string   背景 ID (必填)
```

### message remove-text-emotion — 移除文字表情回应

```
Usage:
  dws chat message remove-text-emotion [flags]
Example:
  dws chat message remove-text-emotion --group <openConversationId> --msg-id <openMessageId> --emotion-id <emotionId> --emotion-name "赞" --text "nice" --background-id <backgroundId>
Flags:
      --group string           会话 openConversationId (必填)
      --msg-id string          消息 openMessageId (必填)
      --emotion-id string      表情 ID (必填)
      --emotion-name string    表情名称 (必填)
      --text string            文字内容 (必填)
      --background-id string   背景 ID (必填)
```

---

## message send-card — 创建并推送流式卡片

向群聊或单聊发送流式卡片消息。群聊传 --group，单聊传 --user（openDingTalkId），二者互斥。

**⚠️ 必须配套使用 `update-card`：** 创建卡片后通过 `update-card` 推送内容更新，**最后一次更新必须将 `--flow-status` 设为 3（finish）**，否则卡片会一直停在"生成中"加载态。

flow-status 取值：1=处理中(PROCESSING)，2=输入中(INPUTTING)，3=完成(FINISH)，4=执行中(EXECUTING)，5=错误(ERROR)。

```
Usage:
  dws chat message send-card [flags]
Example:
  dws chat message send-card --group <openConversationId>
  dws chat message send-card --user <openDingTalkId>
  dws chat message send-card --group <openConversationId> --card-template-id <templateId> --card-data '{"key":"value"}'
Flags:
      --group string              群聊 openConversationId（群聊时必填，与 --user 互斥）
      --user string               单聊接收者 openDingTalkId（单聊时必填，与 --group 互斥）
      --card-template-id string   卡片模板 ID（可选）
      --card-data string          卡片数据 JSON（可选）
```

## message update-card — 流式更新卡片内容

--biz-id 为 send-card 返回的业务 ID，--flow-status 控制流式状态。**最后一次更新必须将 --flow-status 设为 3（finish）**。

```
Usage:
  dws chat message update-card [flags]
Example:
  dws chat message update-card --biz-id <bizId> --content "更新的卡片内容" --flow-status 2
  dws chat message update-card --biz-id <bizId> --content "最终内容" --flow-status 3
Flags:
      --biz-id string    卡片业务 ID (必填，来自 send-card 返回的 bizId)
      --content string   卡片消息内容 (必填)
      --flow-status int  流式状态 (必填，1/2/3/4/5)
```

---

## set-top — 设置/取消会话置顶

设置/取消会话置顶（**单聊/群聊均可**）。默认置顶，传 --off 取消置顶。

```
Usage:
  dws chat set-top [flags]
Example:
  dws chat set-top --group <openConversationId>
  dws chat set-top --group <openConversationId> --off
Flags:
      --group string   会话 openConversationId (必填，支持单聊/群聊)
      --off            传 --off 取消置顶（默认置顶）
```

---

## mute — 会话消息免打扰

开启或关闭会话消息免打扰（**单聊/群聊均可**）。默认开启，传 --off 关闭免打扰。

```
Usage:
  dws chat mute [flags]
Example:
  dws chat mute --group <openConversationId>
  dws chat mute --group <openConversationId> --off
Flags:
      --group string   会话 openConversationId (必填，支持单聊/群聊)
      --off            传 --off 关闭免打扰（默认开启）
```

---

## group-mute — 群全员禁言

全员禁言/取消全员禁言。默认禁言，传 --off 取消。

```
Usage:
  dws chat group-mute [flags]
Example:
  dws chat group-mute --group <openConversationId>
  dws chat group-mute --group <openConversationId> --off
Flags:
      --group string   群聊 openConversationId (必填)
      --off            传 --off 取消全员禁言（默认开启）
```

---

## group-mute-member — 群成员禁言

指定群成员禁言/解除禁言。默认禁言，传 --off 解除。

--mute-time 为禁言时长（毫秒），仅禁言时必填，常用值：300000（5 分钟）、3600000（1 小时）、86400000（1 天）、604800000（1 周）、2592000000（30 天）。

```
Usage:
  dws chat group-mute-member [flags]
Example:
  dws chat group-mute-member --group <openConversationId> --users openDingTalkId1,openDingTalkId2 --mute-time 3600000
  dws chat group-mute-member --group <openConversationId> --users openDingTalkId1 --off
Flags:
      --group string     群聊 openConversationId (必填)
      --users string     被禁言成员 openDingTalkId 列表 (必填，逗号分隔)
      --mute-time int    禁言时长（毫秒，仅禁言时必填）
      --off              传 --off 解除禁言（默认禁言）
```

---

## list-categories — 列出用户自定义会话分类

获取当前用户的所有自定义会话分类。

```
Usage:
  dws chat list-categories
Example:
  dws chat list-categories
```

## list-conversations — 按分类拉取会话列表

根据分类 ID 拉取该分类下的会话列表。分类 ID 通过 `list-categories` 获取。

```
Usage:
  dws chat list-conversations [flags]
Example:
  dws chat list-conversations --category-id <categoryId>
Flags:
      --category-id string   分类 ID (必填，由 list-categories 返回)
```


## 意图判断

用户说"建群/创建群聊" → `chat group create`
用户说"搜索群/找群" → `chat search`
用户说"群成员/看群里有谁" → `chat group members list`
用户说"拉人进群/加群成员" → `chat group members add`
用户说"踢人/移除群成员" → `chat group members remove`
用户说"加机器人到群" → `chat group members add-bot`
用户说"改群名" → `chat group rename`
用户说"聊天记录/会话消息/拉取会话" → `chat message list`
用户说"某人发给我的消息/指定发送者/某人的消息" → `chat message list-by-sender`（用户未明确说"单聊"时优先使用，跨单聊/群聊）
用户说"拉取和某人的单聊记录/单聊消息" → `chat message list-direct --user`（用户明确说"单聊"时使用）
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
用户说"撤回我发的消息/撤回消息" → `chat message recall-by-bot`（仅能撤回机器人消息；撤回用户自己发的消息开源 dws v1.0.30 暂不支持，需在钉钉客户端长按撤回）
用户说"撤回机器人发的消息/机器人撤回消息" → `chat message recall-by-bot`（机器人接口撤回机器人发出的消息，需要 robot-code + processQueryKey）
用户说"引用回复/回复消息/引用消息回复" → `chat message reply`
用户说"转发消息/转发一条消息/把消息转发到另一个群" → `chat message forward`
用户说"消息已读未读/谁看了消息/查读状态/消息读取状态" → `chat message query-read-status`
用户说"查询消息发送状态/消息发没发成功/消息状态" → `chat message query-send-status`
用户说"多维度搜索消息/按发送者搜消息/按人搜消息/指定多个群搜索/@我的消息搜索" → `chat message search-advanced`（推荐首选，支持多维度组合搜索）
用户说"批量查消息/按ID查消息/根据消息ID查" → `chat message list-by-ids`
用户说"emoji回应/表情回应/给消息加表情" → `chat message add-emoji`
用户说"取消emoji回应/移除表情回应" → `chat message remove-emoji`
用户说"文字表情回应/添加文字表情" → `chat message add-text-emotion`
用户说"取消文字表情回应/移除文字表情" → `chat message remove-text-emotion`
用户说"创建文字表情/新建文字表情" → `chat message create-text-emotion`
用户说"流式卡片/AI 卡片/发送卡片" → `chat message send-card` → `chat message update-card`（必须配合 update-card，最后一次 --flow-status 必须为 3 finish）
用户说"置顶会话/取消置顶/会话置顶（操作）" → `chat set-top`（设置/取消置顶），`chat list-top-conversations`（查看置顶列表）
用户说"免打扰/消息免打扰/静音/开启免打扰/关闭免打扰" → `chat mute`
用户说"全员禁言/群禁言/解除禁言" → `chat group-mute`
用户说"禁言某人/指定成员禁言/解除某人禁言" → `chat group-mute-member`
用户说"设管理员/取消管理员/设置群管理员" → `chat group set-admin`
用户说"转让群主/换群主/群主转让" → `chat group transfer-owner`
用户说"群邀请链接/入群链接/加群链接" → `chat group invite-url`
用户说"退出群聊/我要退群" → `chat group quit`
用户说"改群头像/更新群头像" → `chat group update-icon`
用户说"群设置/修改群设置" → `chat group update-settings`
用户说"根据群号查群信息/群号查群/群号转 openConversationId" → `chat group get-by-group-id`（用户提供数字群号时把它转为 openConversationId）
用户说"查看群身份/群的自定义身份列表/群成员角色" → `chat group member-role list`
用户说"创建/添加群身份/添加群角色" → `chat group member-role add`
用户说"修改群身份名称/改群角色名" → `chat group member-role update`
用户说"删除群身份/删群角色" → `chat group member-role remove`
用户说"给某人设置群身份/设定用户角色" → `chat group member-role set-user`
用户说"移除某人的群身份/撤销群角色" → `chat group member-role remove-user`
用户说"查询某人的群身份/某人在群里有什么身份" → `chat group member-role query-user`
用户说"查看会话分类/自定义分类" → `chat list-categories`
用户说"某个分类下的会话/分类会话列表" → `chat list-conversations`

关键区分:
- `chat message list` — 拉取指定会话的消息（需指定 --group 或 --user），按时间点 + 方向翻页
- `chat message list-direct --user` — list 的单聊模式，拉取与指定用户的单聊记录（用户明确说"单聊""私聊"时使用）
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
- `chat message recall-by-bot` — 撤回机器人发出的消息（需 --robot-code + --keys；群聊另加 --group）。撤回用户自己的消息开源 dws v1.0.30 不支持
- `chat message reply` — 引用回复消息（单聊/群聊均可），以当前用户身份，需 --conversation-id + --ref-msg-id + --ref-sender + --text
- `chat message forward` — 转发单条消息（源/目标会话均支持单聊/群聊），需 --src-conversation-id + --msg-id + --dest-conversation-id
- `chat message list-by-ids` — 按消息 ID 列表批量查询（最多 50 条）
- `chat message query-send-status` — 查询消息发送状态，需 --open-task-id（send 返回值）
- `chat message query-read-status` — 查询消息的已读/未读状态（仅消息发送者可查询自己发的消息），需 --group + --msg-id
- `chat message search-advanced` — 多维度搜索消息（推荐首选，是 search 的严格超集，可叠加 keyword/sender-ids/at-me/at-ids/conversation-ids/start/end）
- `chat message add-emoji` / `remove-emoji` — 对消息添加/移除 emoji 表情回应（参考 chat-emoji-list.md 的 199 个默认表情）
- `chat message add-text-emotion` / `remove-text-emotion` — 添加/移除文字表情回应
- `chat message create-text-emotion` — 创建文字表情模板，返回 emotionId 供 add-text-emotion 使用
- `chat message send-card` / `update-card` — 流式卡片，必须配套使用；最后一次 update-card 的 --flow-status 必须为 3 (finish)
- `chat set-top` — 设置/取消会话置顶（单聊/群聊均可），默认置顶，--off 取消
- `chat mute` — 开启/关闭会话消息免打扰（单聊/群聊均可），默认开启，--off 关闭
- `chat group-mute` — 群全员禁言开关，默认开启，--off 关闭
- `chat group-mute-member` — 指定群成员禁言，需 --users + --mute-time（毫秒，仅禁言时必填），--off 解除
- `chat group set-admin` — 设置/取消群管理员，默认设为管理员，--off 取消
- `chat group transfer-owner` — 转让群主
- `chat group invite-url` — 获取群邀请链接，可选 --expires-seconds（0=永久）
- `chat group quit` — 当前用户退出群聊
- `chat group update-icon` / `update-settings` — 更新群头像 / 群设置
- `chat group get-by-group-id` — 把数字群号转为 openConversationId
- `chat group member-role` — 群自定义角色管理（list/add/update/remove/set-user/remove-user/query-user）
- `chat list-categories` / `list-conversations` — 自定义会话分类列表 / 按分类拉会话

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
dws chat message send --user <userId> --title "问候" "你好" --format json

# 4b. 以个人身份单聊（通过 openDingTalkId，三方应用等无法获取 userId 时使用）
dws chat message send --open-dingtalk-id <openDingTalkId> --title "问候" "你好" --format json

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
  --title "截图" --text "![截图](下载链接)" --format json
```

## 上下文传递表

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `chat search` | `openConversationId` | message send/list、group members 等的 --group |
| `chat group create` | `openConversationId` | 同上 |
| `chat group get-by-group-id` | `openConversationId` | 把数字群号转换后给 message send/list 等命令使用 |
| `chat message list-all` | `nextCursor` | 下次 list-all 的 --cursor |
| `chat message search` / `search-advanced` | `nextCursor` | 下次 message search/search-advanced 的 --cursor |
| `aisearch person` | `userId` | message send 的 --user、--at-users、send-by-bot 的 --users、list-by-sender 的 --sender-user-id |
| `aisearch person` → `contact user get` | `openDingTalkId` | list-by-sender 的 --sender-open-dingtalk-id、message send/list 的 --open-dingtalk-id、search-advanced 的 --sender-ids/--at-ids、reply 的 --ref-sender、group transfer-owner 的 --new-owner、group set-admin / group-mute-member 的 --users、group member-role 系列的 --user |
| `chat bot search` | `robotCode` | send-by-bot / recall-by-bot 的 --robot-code、group members add-bot 的 --robot-code |
| `chat message send-by-bot` | `processQueryKey` | recall-by-bot 的 --keys |
| `chat message send` | `openTaskId` | query-send-status 的 --open-task-id |
| `chat message list` | `openMessageId` | recall 的 --msg-id、reply 的 --ref-msg-id、forward 的 --msg-id、query-read-status 的 --msg-id、list-by-ids 的 --msg-ids、add-emoji/add-text-emotion 的 --msg-id |
| `chat conversation-info` | `openConversationId` / `newCSpaceIdIM` | 作为 message send/list 等后续操作的会话上下文；newCSpaceIdIM 用于 drive upload-info 的 --space-id |
| `chat search-common` | `openConversationId` | message send/list 等的 --group |
| `chat list-categories` | `categoryId` | list-conversations 的 --category-id |
| `chat group member-role list` | `openRoleId` | group member-role update/remove/set-user/remove-user 的 --role-id / --role-ids |
| `chat message create-text-emotion` | `emotionId` | add-text-emotion 的 --emotion-id |
| `chat message send-card` | `bizId` | update-card 的 --biz-id |
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

- **发消息前参数审查（必须执行）**：发消息（`message send`、`send-by-bot`、`send-by-webhook`、`send-card`、`reply`、`forward`）是严肃操作，发错人/发错群会导致严重问题。执行前 agent 必须把全部参数（收件人/群、消息内容、@对象、消息类型等）与用户的原始需求逐一对比，确认每个参数都能从原始需求中找到明确依据；存在任何不明确/有歧义/原始需求中未提及的参数，必须先向用户确认，严禁自行假设
- **消息换行符**：`message send` / `send-by-bot` / `send-by-webhook` 的 `--text` 按 Markdown 渲染，换行有两层要求，缺一不可：(1) 必须使用**真实换行符** `U+000A`，而非字面量字符串 `\n`；(2) Markdown 单换行不产生换行，需用空行（连续两个真实换行符）做段落分隔，或行尾两空格 + 换行 / `<br>` 做硬换行
- `chat message recall` 撤回当前用户自己发的消息，--group + --msg-id 必填；与 `recall-by-bot`（撤回机器人消息，--robot-code + --keys）区分
- `chat message reply` 引用回复（单聊/群聊均可），需 --conversation-id + --ref-msg-id + --ref-sender + --text；目前回复类型仅支持 text
- `chat message forward` 转发消息（源/目标会话单聊群聊均可），需 --src-conversation-id + --msg-id + --dest-conversation-id
- `chat message query-send-status` 查询消息发送状态，需 --open-task-id（send 返回值）
- `chat message query-read-status` 查询消息已读/未读状态，仅消息发送者可查询自己发的消息，需 --group + --msg-id
- `chat message search-advanced` 多维度搜索（推荐首选），所有参数可选但至少传一个；--sender-ids / --at-ids 传 openDingTalkId
- `chat message list-by-ids` --msg-ids 逗号分隔，最多 50 条
- `chat message add-emoji` / `remove-emoji` 需 --group + --msg-id + --emoji；--emoji 取自 chat-emoji-list.md 的 name 列
- `chat message add-text-emotion` / `remove-text-emotion` 六个参数全部必填：--group + --msg-id + --emotion-id + --emotion-name + --text + --background-id
- `chat message create-text-emotion` 创建文字表情模板，返回 emotionId；--background-id 可选
- `chat message send-card` 创建并推送流式卡片，群聊传 --group，单聊传 --user（openDingTalkId）；**必须配合 update-card 使用，最后一次 update-card 的 --flow-status 必须为 3（finish）**，否则卡片会一直显示"生成中"
- `chat set-top` 设置/取消会话置顶（单聊/群聊均可），需 --group，默认置顶，--off 取消
- `chat mute` 开启/关闭会话消息免打扰（单聊/群聊均可），需 --group，默认开启，--off 关闭
- `chat group-mute` 群全员禁言开关，需 --group，默认开启，--off 关闭
- `chat group-mute-member` 指定成员禁言，需 --group + --users（openDingTalkId 逗号分隔）+ --mute-time（毫秒，仅禁言时必填，常用 300000/3600000/86400000/604800000/2592000000），--off 解除
- `chat group set-admin` 设置/取消群管理员，需 --group + --users（openDingTalkId 逗号分隔），默认设为管理员，--off 取消
- `chat group transfer-owner` 转让群主，需 --group + --new-owner（openDingTalkId）
- `chat group invite-url` 获取群邀请链接，需 --group，可选 --expires-seconds（0=永久有效）
- `chat group quit` 退出群聊，需 --group
- `chat group update-icon` 更新群头像，需 --group + --icon-media-id（mediaId）
- `chat group update-settings` 更新群设置，需 --group + --setting-key + --status（字符串类型 "0"/"1"）
- `chat group get-by-group-id` 把数字群号转换为 openConversationId，需 --group-id（数字字符串）
- `chat group member-role` 系列管理群自定义角色：list 查列表，add 创建，update 改名，remove 删除；set-user 覆盖某人全部角色（传空 --role-ids 则清除），remove-user 仅移除指定角色，query-user 查询某人当前角色；--user 传 openDingTalkId（非 userId）
- `chat list-categories` 列出当前用户自定义会话分类（无参数）
- `chat list-conversations` 按分类拉取会话列表，需 --category-id（字符串，由 list-categories 返回）
- **如何获取 openConversationId**（上层已有则直接用，不必重复查）：群聊 `dws chat search --query "群名"`；单聊 `dws chat conversation-info --open-dingtalk-id <openDingTalkId>`
- **如何获取 openDingTalkId**：知道姓名 → `dws contact user search --query "姓名"`；只有 userId → 先 `dws contact user get --ids <userId>` 拿姓名，再 search

## 自动化脚本

| 脚本 | 场景 | 用法 |
|------|------|------|
| [chat_export_messages.py](../../scripts/chat_export_messages.py) | 导出群聊消息到 JSON 文件 | `python chat_export_messages.py --query "项目冲刺" --time "2026-03-10 00:00:00"` |
| [chat_history_with_user.py](../../scripts/chat_history_with_user.py) | 查询与某人的单聊聊天记录 | `python chat_history_with_user.py --name "张三" --time "2026-03-10 00:00:00"` |

## 相关产品

- [contact](./contact.md) — 搜索同事/好友，获取 userId 用于 --user、--at-users、send-by-bot --users、list-by-sender --sender-user-id；获取 openDingTalkId 用于 list-by-sender --sender-open-dingtalk-id、--open-dingtalk-id
- [drive](./drive.md) — 上传文件获取下载链接，用于 Markdown 图片/文件消息
