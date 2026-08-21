# chat-conversation：会话状态、红点、置顶与分组

> 返回入口：[chat.md](../chat.md)

## 适用场景

用于获取会话基础信息、全部会话/置顶会话列表、会话置顶、免打扰、隐藏、红点、已读未读、清空聊天记录、自定义会话分组和智能会话分组。

- <!-- dws-intent: chat.conversation.list-top -->查看置顶会话默认使用 `dws chat +conversation-list-top`；
  原子 `list-top-conversations` 只在需要原始响应时作为 fallback。
- <!-- dws-intent: chat.read.conversation -->取得目标会话后读取或导出消息记录，默认使用 `dws chat +chat-messages`；
  可附带非必填的 `--sender-query` 解析姓名：唯一解析成功后按 `senderId` 筛选同一次读取结果；
  解析失败、不完整或存在歧义时抑制未过滤消息并返回 `sender_resolution_failed`。不要回到原子
  `message list`，也不要补跑 `+search-msg`。直接条件检索优先使用 `+search-msg`。

## 必读约束

- 会话状态类命令通常需要 `openConversationId`。群聊只用 `+chat-search --query` 获取唯一候选，单聊可由 `chat conversation-info --user/--open-dingtalk-id` 获取。
- `set-top` 是会话置顶；`message set-top-msg` 是会话内消息置顶，二者不能混用。
- `clear-messages` 只清空当前用户视角的消息，不影响其他成员。
- 智能分组规则中的成员使用 openDingTalkId；如果用户只给姓名，先用 `aisearch person --dimension name` 获取。

## 命令明细

### 会话基础信息

```bash
dws chat conversation-info --group <openConversationId> --format json
dws chat conversation-info --user <userId> --format json
dws chat conversation-info --open-dingtalk-id <openDingTalkId> --format json
```

`--group`、`--user`、`--open-dingtalk-id` 互斥且必须指定一个。文件/音视频发送不再依赖调用方读取 spaceId；直接用 `message send --msg-type file|audio|video --file`。

### 会话列表与红点

| 命令 | 用途 | 参数 |
|------|------|------|
| `+conversation-list` | 获取当前用户会话 | 要求“全部”时加 `--page-all`；检查 `complete` / `failures` |
| `+chat-list` | 列出当前用户会话（默认群聊，可选单聊） | 默认只返回群聊；`--types group,p2p` 可包含单聊；要求全部时加 `--page-all`，合并去重后再过滤类型 |
| `+chat-list-all` | 获取当前用户加入的全部群 | 要求全部时加 `--page-all`；沿数字 `nextCursor` 去重聚合 |
| `+my-groups` | 获取并投影当前用户加入的群 | 要求全部时加 `--page-all`；读完后再应用 `--type` 本地过滤 |
| `+conversation-list-top` | 获取置顶会话列表 | 可选 `--limit` `--cursor` `--exclude-muted`；使用稳定 `conversations[]` |
| `message list-unread-conversations` | 获取未读会话列表 | 可选 `--count` `--exclude-muted` |
| `clear-red-point` | 清除指定会话红点 | `--conversation-id`，别名 `--id` / `--chat` |
| `clear-all-red-point` | 清除所有会话红点，一键全部已读 | 无参数 |

翻页时，`hasMore=true` 用返回的 `nextCursor` 作为下次 `--cursor`。Shortcut 全量读取应检查
`complete`、`stopReason` 和 `failures`；达到 `--page-limit` 时会保留可继续的 `nextCursor`。

### 会话置顶与通知

| 命令 | 用途 | 必填参数 |
|------|------|----------|
| `set-top` | 设置/取消会话置顶 | `--conversation-id`；默认置顶，`--off` 取消 |
| `mute` | 开启/关闭会话免打扰 | `--conversation-id`；默认开启，`--off` 关闭 |
| `hide` | 隐藏会话 | `--conversation-id` |
| `mute-at-all` | 关闭/恢复 @所有人通知 | `--conversation-id`；默认关闭，`--off` 恢复；必须先开启会话总免打扰 |
| `mute-red-envelope` | 关闭/恢复红包通知 | `--conversation-id`；默认关闭，`--off` 恢复；必须先开启会话总免打扰 |

若连续操作两个子开关，优先操作红包通知；恢复 @所有人通知后，平台可能清除子开关所需的
总免打扰状态，此时要先重新开启总免打扰，再操作红包通知。

```bash
dws chat set-top --conversation-id <openConversationId>
dws chat set-top --conversation-id <openConversationId> --off
dws chat mute --conversation-id <openConversationId>
dws chat mute --conversation-id <openConversationId> --off
dws chat hide --conversation-id <openConversationId>
```

### 已读未读与清理

| 命令 | 用途 | 必填参数 |
|------|------|----------|
| `mark-unread` | 标记指定会话为未读 | `--conversation-id` |
| `mark-read` | 将指定消息及之前消息标记为已读 | `--conversation-id` `--message-id` |
| `clear-messages` | 清空当前用户指定会话的消息 | `--conversation-id` |

```bash
dws chat mark-unread --conversation-id <openConversationId>
dws chat mark-read --conversation-id <openConversationId> --message-id <openMessageId>
dws chat clear-messages --conversation-id <openConversationId>
```

### 会话分组

| 命令 | 用途 | 必填参数 |
|------|------|----------|
| `category list` | 获取用户自定义会话分组 | 无 |
| `category list-conversations` | 拉取指定分组下会话 | `--category-id`，可选 `--exclude-muted` |
| `category list-by-conv` | 拉取指定会话所属的用户自定义会话分组 | `--group` |
| `category batch-info` | 批量拉取用户自定义会话分组信息 | `--category-ids` |
| `category create` | 创建会话分组 | `--title` |
| `category create-smart` | 创建智能会话分组，可按群名称关键词和群内成员匹配 | `--name`，可选 `--keywords` `--members` |
| `category delete` | 删除会话分组 | `--category-id` |
| `category rename` | 修改分组名称 | `--category-id` `--title` |
| `category add-conv` | 将会话加入分组 | `--group` `--category-ids` |
| `category remove-conv` | 将会话移出分组 | `--group` `--category-ids` |

```bash
dws chat category list
dws chat category list-by-conv --group <openConversationId>
dws chat category batch-info --category-ids 123,456
dws chat category create --title "工作群"
dws chat category create-smart --name "重点群" --keywords "重点,项目" --members openDingTalkId1,openDingTalkId2
dws chat category add-conv --group <openConversationId> --category-ids 123,456
```

`create-smart` 中 `--keywords` 是群名称关键词列表，`--members` 是群内成员 openDingTalkId 列表；两者可单独使用，也可组合使用。

## 常见工作流

### 获取单聊会话 ID 后置顶

```bash
dws aisearch person --query "张三" --dimension name --format json
dws chat conversation-info --user <userId> --format json
dws chat set-top --conversation-id <openConversationId> --format json
```

### 查看置顶会话并拉消息

```bash
dws chat +conversation-list-top --limit 100 --format json
dws chat +chat-messages --group <openConversationId> --time "2026-03-10 00:00:00" --direction older --format json
```

### 会话分组

```bash
dws chat category create --title "重点项目" --format json
dws chat category add-conv --group <openConversationId> --category-ids <categoryId> --format json
dws chat category list-by-conv --group <openConversationId> --format json
dws chat category batch-info --category-ids <categoryId> --format json
dws chat category list-conversations --category-id <categoryId> --format json
```

### 智能会话分组

```bash
dws aisearch person --query "张三" --dimension name --format json
dws chat category create-smart --name "项目组" --keywords "项目,开发" --format json
dws chat category create-smart --name "团队群" --members openDingTalkId1,openDingTalkId2 --format json
dws chat category create-smart --name "重点群" --keywords "重点" --members openDingTalkId1 --format json
```

## 常见错误与回退

- 用户说“置顶消息”：用 `message set-top-msg`，不是 `chat set-top`。
- 用户说“置顶会话”：设置/取消用 `chat set-top`，查看列表用 `+conversation-list-top`。
- 单聊没有会话 ID：先 `conversation-info --user` 或 `--open-dingtalk-id`。
- 清空聊天记录前必须确认目标会话；该操作只影响当前用户视角。
- 智能分组没有匹配条件：至少确认分组名称；关键词和成员规则不明确时先向用户确认，不要自行猜成员。
