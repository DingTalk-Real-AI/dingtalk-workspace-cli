# chat-message：消息、文件、搜索与卡片

> 返回入口：[chat.md](../chat.md)

## 适用场景

用于当前用户发消息、拉消息、搜索消息、撤回、已读状态、文件/图片/音频/视频/位置/名片发送、话题回复、转发、Pin/Top、表情回应、文本翻译和流式卡片。

## 必读约束

- 发消息前必须核对接收对象、消息内容、@ 对象、附件路径和消息类型；不明确时先问用户。
- `--group`、`--user`、`--open-dingtalk-id` 通常互斥，群聊用 `--group`，单聊用 `--user` 或 `--open-dingtalk-id`。
- 发送本地文件、音频、视频统一用 `chat message send --msg-type file|audio|video --file-path <path>`；`audio` / `video` 底层按 `file` 链路发送；旧图片链路 `--msg-type image --media-id` 仅在已有 mediaId 时使用。
- 发送位置消息前必须确认纬度、经度、地址名称；地图缩略图需先通过旧媒体上传链路拿到 mediaId。
- 分享联系人名片前必须确认联系人 `openDingTalkId`，不要把 userId 直接当 `--contact-id`。
- 消息内容按 Markdown 渲染，换行必须是真实换行符；需要换行效果时用空行、行尾两个空格或 `<br>`。
- 建议发送时带 `--uuid`，失败重试复用同一个值。

## 命令明细

### 发送消息

#### `dws chat message send`

以当前用户身份发送群聊或单聊消息。

```bash
# 文本/Markdown
dws chat message send --group <openConversationId> --text "hello"
dws chat message send --user <userId> --text "请查收"
dws chat message send --open-dingtalk-id <openDingTalkId> --text "请查收"
dws chat message send --group <openConversationId> --title "周报提醒" --text "请大家本周五前提交周报" --uuid <uuid>

# @ 群成员
dws chat message send --group <openConversationId> --at-all "<@all> 请大家注意"
dws chat message send --group <openConversationId> --at-open-dingtalk-ids odt1,odt2 "<@odt1> <@odt2> 请查收"

# 图片/文件/音频/视频，一条命令直发
dws chat message send --group <openConversationId> --msg-type file --file-path ./screenshot.png
dws chat message send --open-dingtalk-id <openDingTalkId> --msg-type file --file-path ./report.pdf
dws chat message send --group <openConversationId> --msg-type audio --file-path ./voice.mp3
dws chat message send --group <openConversationId> --msg-type video --file-path ./demo.mp4

# 位置/联系人名片
dws chat message send --group <openConversationId> --msg-type location --latitude <纬度> --longitude <经度> --location-name <地址名称> --map-thumbnail-url "@mediaId"
dws chat message send --group <openConversationId> --msg-type profile --contact-id <openDingTalkId>
```

关键 flags：

| Flag | 说明 |
|------|------|
| `--group` | 群聊 openConversationId；别名 `--id` / `--chat` / `--conversation-id` |
| `--user` | 单聊接收人 userId |
| `--open-dingtalk-id` | 单聊接收人 openDingTalkId |
| `--text` | 消息内容，推荐使用；也支持位置参数 |
| `--title` | 消息标题，未传时使用安全标题 |
| `--at-all` | 群聊 @所有人，正文需含 `<@all>` |
| `--at-open-dingtalk-ids` | 群聊 @指定 openDingTalkId，正文需含 `<@id>` |
| `--msg-type` | `file` / `audio` / `video` / `image` / `location` / `profile`；本地音视频用 `audio` / `video`，底层按 `file` 发送 |
| `--file-path` | 本地文件路径，`msg-type=file/audio/video` 时自动上传并发送 |
| `--media-id` | 旧图片链路 mediaId |
| `--latitude` / `--longitude` / `--location-name` | 位置消息参数 |
| `--map-thumbnail-url` | 位置消息缩略图 mediaId，形如 `@mediaId` |
| `--contact-id` | 联系人名片 openDingTalkId |
| `--uuid` | 幂等 UUID，24h 内相同值不重复投递 |

话题回复：`chat message list` 返回的 `openConvThreadId` 可直接作为 `--group` 值发送到话题内，禁止自行拼接。

### 拉取消息

| 命令 | 用途 | 示例与要点 |
|------|------|------------|
| `message list` | 拉取指定群聊或单聊消息 | `dws chat message list --group <cid> --time "2025-03-01 00:00:00" --direction older`；目标三选一，`--direction newer/older` 优先于旧 `--forward` |
| `message list-all` | 时间范围内全部会话消息 | `dws chat message list-all --start <ISO> --end <ISO> --limit 100 --cursor 0`；四个参数每次请求都传，翻页用 `nextCursor` |
| `message list-by-sender` | 查指定发送者消息 | `--sender-user-id` 与 `--sender-open-dingtalk-id` 二选一，跨单聊+群聊 |
| `message list-mentions` | 查 @ 我的消息 | 可传 `--group` 限定群，不传查全部 |
| `message list-focused` | 查特别关注人消息 | 零参数可用，可加 `--limit` / `--cursor` |
| `message list-unread-conversations` | 未读会话列表 | 可选 `--count` |
| `message list-topic-replies` | 拉取话题回复 | `--group <openConversationId> --topic-id <openConvThreadId>` |
| `message list-by-ids` | 按消息 ID 批量查询 | `--msg-ids msgId1,msgId2`，最多 50 条 |

`message list` 注意事项：

- `--group`、`--user`、`--open-dingtalk-id` 互斥且必须指定一个。
- `--time` 格式为 `yyyy-MM-dd HH:mm:ss`。
- `hasMore=true` 时，用结果中的边界 `createTime` 作为下次 `--time`。
- 返回 `openConvThreadId` 表示话题消息，完整内容需再拉 `list-topic-replies`。

### 搜索消息

优先使用 `message search-advanced`，它是 `message search` 的严格超集。

```bash
dws chat message search-advanced --query "周报" --start "2026-04-01T00:00:00+08:00" --end "2026-04-15T00:00:00+08:00"
dws chat message search-advanced --user <userId> --start <ISO> --end <ISO>
dws chat message search-advanced --at-me --start <ISO> --end <ISO>
dws chat message search-advanced --conversation-ids <cid1>,<cid2> --query "合同" --limit 50 --cursor 0
dws chat message search-advanced --message-type file --search-conv-type group_chat --query "附件"
dws chat message search-advanced --only-robot-messages --query "通知"
```

| 参数 | 说明 |
|------|------|
| `--query` | 搜索关键词，可选 |
| `--user` / `--users` | 发送者 userId |
| `--sender-ids` | 发送者 openDingTalkId |
| `--at-me` / `--at-ids` | @ 我 / @ 指定 openDingTalkId |
| `--conversation-ids` | 多个群聊或单聊 openConversationId；别名 `--groups` |
| `--message-type` | 按消息类型过滤，例如 `file` |
| `--search-conv-type` | 按会话类型过滤，例如 `group_chat` |
| `--only-robot-messages` | 只搜索机器人消息 |
| `--start` / `--end` | ISO-8601 时间范围 |
| `--cursor` / `--limit` | 分页，翻页用 `nextCursor` |

仅简单关键词搜索时可用：

```bash
dws chat message search --query "changefree" --start <ISO> --end <ISO> --limit 50 --cursor 0
dws chat message search --query "codereview" --group <openConversationId> --start <ISO> --end <ISO>
```

### 消息状态与撤回

| 命令 | 用途 | 必填参数 |
|------|------|----------|
| `message query-send-status` | 查询当前用户发消息任务状态 | `--open-task-id`，来自 `message send` 返回 |
| `message recall` | 撤回当前用户消息；群主/管理员可撤回群内他人消息 | `--conversation-id` `--msg-id` |
| `message edit` | 编辑已发送消息内容 | `--conversation-id` `--msg-id`，并在 `--text` / `--content` 中二选一；可选 `--title` `--at-all` `--at-open-dingtalk-ids` |
| `message read-status` | 查消息已读/未读状态 | `--group` `--message-id`；可选目标用户 |

`recall` 与 `recall-by-bot` 不同：前者通过 IM 接口撤回用户消息，需要 `openConversationId + openMessageId`，支持撤回自己发出的消息，也支持群主/管理员撤回群内他人消息；后者撤回机器人消息，需要 `robot-code + processQueryKey`。

编辑消息使用 `message edit`。推荐传 `--text`，CLI 会生成 markdown content JSON：`{"title":"标题","text":"正文"}`；可选 `--title`，不传时会从正文自动生成标题。高级场景可直接传 `--content`，此时必须是完整 markdown content JSON，且不能同时传 `--text`。

@ 规则：`--at-all` 会传 `atAll=true`，正文应包含 `<@all>`，未包含时 CLI 会自动补到开头；`--at-open-dingtalk-ids` 会传 `atOpenDingTalkIds`，正文需包含对应 `<@openDingTalkId>` 占位符，裸 `@openDingTalkId` 会自动补成尖括号格式。

```bash
dws chat message edit --conversation-id <openConversationId> --msg-id <openMessageId> --text "更新后的内容"
dws chat message edit --group <openConversationId> --msg-id <openMessageId> --title "标题" --text "更新后的内容"
dws chat message edit --group <openConversationId> --msg-id <openMessageId> --text "<@all> 请查看" --at-all
dws chat message edit --group <openConversationId> --msg-id <openMessageId> --text "<@openDingTalkId1> 请查看" --at-open-dingtalk-ids <openDingTalkId1>
dws chat message edit --group <openConversationId> --msg-id <openMessageId> --content '{"title":"标题","text":"更新后的内容"}'
```

### 回复与转发

| 命令 | 用途 | 必填参数 |
|------|------|----------|
| `message reply` | 引用回复，单聊/群聊均可 | `--conversation-id` `--ref-msg-id` `--ref-sender` `--text` |
| `message forward` | 转发单条消息，源/目标均支持单聊/群聊 | `--src-conversation-id` `--msg-id` `--dest-conversation-id` |
| `message combine-forward` | 多条消息合并为一条转发 | `--src-conversation-id` `--msg-ids` `--dest-conversation-id`，可选 `--uuid` |
| `message forward-topic` | 转发话题消息 | `--src-msg-id` `--src-conversation-id` `--src-thread-id` `--dest-conversation-id` |

### 话题与卡片

话题完整读取流程：

1. `dws chat message list --group <openConversationId> --time ...` 获取话题主消息。
2. 如果返回 `openConvThreadId`，执行 `dws chat message list-topic-replies --group <openConversationId> --topic-id <openConvThreadId>`。

流式卡片必须 `send-card` 与 `update-card` 搭配：

```bash
dws chat message send-card --group <openConversationId>
dws chat message send-card --receiver <openDingTalkId>
dws chat message update-card --biz-id <bizId> --content "更新的卡片内容" --flow-status 2
dws chat message update-card --biz-id <bizId> --content "最终内容" --flow-status 3
```

`flow-status`：1=处理中，2=输入中，3=完成，4=执行中，5=错误。最后一次必须设为 3，否则卡片会停留在“生成中”。

### Pin / Top / Favorite

| 命令 | 用途 | 必填参数 |
|------|------|----------|
| `message set-pin-msg` / `unset-pin-msg` | 钉住/取消钉住消息 | `--open-conversation-id` `--msg-id` |
| `message list-pin-msg` | 拉取钉住消息列表 | `--open-conversation-id`，可选 `--cursor` `--size` |
| `message set-top-msg` / `unset-top-msg` | 置顶/取消置顶会话内某条消息 | `--open-conversation-id` `--msg-id` |
| `message add-favorite` | 收藏消息 | `--open-message-id` `--open-conversation-id` |
| `message remove-favorite` | 取消收藏消息 | `--open-message-id` `--open-conversation-id` |
| `message list-favorites` | 查询收藏消息列表 | 可选 `--cursor` `--size` |

消息置顶 `set-top-msg` 与会话置顶 `chat set-top` 不同：前者置顶会话内消息，后者置顶整个会话。

### 表情回应

优先查 [chat-emoji-list.md](../chat-emoji-list.md) 中的默认表情名称。

| 命令 | 场景 | 必填参数 |
|------|------|----------|
| `message add-emoji` / `remove-emoji` | 默认表情命中时使用 | `--conversation-id` `--msg-id` `--emoji` |
| `message create-text-emotion` | 默认表情没有合适项时先创建 | `--emotion-name` `--text`，可选 `--background-id` |
| `message add-text-emotion` / `remove-text-emotion` | 添加/移除文字表情 | `--conversation-id` `--msg-id` `--emotion-id` `--emotion-name` `--text` `--background-id` |
| `message update-text-emotion` | 用新的文字表情替换消息上的原回应 | `--conversation-id` `--msg-id` `--old-emotion-id` `--emotion-id` `--emotion-name` `--text` `--background-id` |
| `message list-emotion-replies` | 批量查询消息的表情回复和文字回复 | `--msg-ids` |

```bash
dws chat message list-emotion-replies --msg-ids msgId1,msgId2,msgId3
```

消息 ID 可通过 `dws chat message list` 获取；该命令用于一次性查看多条消息上的 emoji 回应和文字表情回应。

### 文本工具

#### `dws chat text translate`

将指定文本翻译成目标语言。用户只说“翻译这段文字”时使用；不要误走 `message send`。

```bash
dws chat text translate --query "你好世界" --to en_US
dws chat text translate --query "Hello World" --to zh_CN
dws chat text translate --query "Bonjour" --to ja_JP
```

关键 flags：

| Flag | 说明 |
|------|------|
| `--query` | 待翻译文本，必填 |
| `--to` | 目标语言代码，必填，默认 `en_US` |

### 文件与媒体

#### `dws chat message download-media`

```bash
dws chat message download-media --type mediaId --resource-id <mediaId> --message-id <openMessageId> --open-conversation-id <openConversationId> --output ./downloads/
```

`resource-id` 来自消息内容中的 mediaId，`message-id` 来自 `openMessageId`，会话 ID 来自 `chat search` 或 `conversation-info`。

## 常见工作流

### 群聊发文字与文件

```bash
dws chat search --query "项目冲刺" --format json
dws chat message send --group <openConversationId> --text "请大家本周五前提交周报" --uuid <uuid> --format json
dws chat message send --group <openConversationId> --msg-type file --file-path ./report.pdf --format json
dws chat message send --group <openConversationId> --msg-type audio --file-path ./voice.mp3 --format json
dws chat message send --group <openConversationId> --msg-type video --file-path ./demo.mp4 --format json
dws chat message send --group <openConversationId> --msg-type location --latitude <纬度> --longitude <经度> --location-name <地址名称> --map-thumbnail-url "@mediaId" --format json
dws chat message send --group <openConversationId> --msg-type profile --contact-id <openDingTalkId> --format json
```

### 查消息并撤回

```bash
dws chat message list --group <openConversationId> --time "2026-03-10 00:00:00" --direction older --format json
dws chat message recall --conversation-id <openConversationId> --msg-id <openMessageId> --format json
```

### 多维度搜索

```bash
dws chat message search-advanced --query "合同" --conversation-ids <cid1>,<cid2> --start "2026-04-01T00:00:00+08:00" --end "2026-04-15T00:00:00+08:00" --limit 50 --cursor 0 --format json
dws chat message search-advanced --message-type file --search-conv-type group_chat --query "附件" --format json
```

## 常见错误与回退

- 发送目标不唯一：先确认群/人；群用 `chat search`，单聊用 `aisearch person` + `conversation-info`。
- `unknown flag`：立即执行对应命令 `--help`，不要猜参数。
- 文件/音视频发送失败：确认本地路径可读；新链路使用 `--msg-type file|audio|video --file-path`。
- 位置消息参数不完整：先确认经纬度、地址名称和缩略图 mediaId。
- 名片发送失败：确认 `--contact-id` 是 openDingTalkId，不是 userId。
- 话题回复缺失：检查是否只拉了主消息，需继续用 `list-topic-replies`。
- `search-advanced` 无条件：至少提供 query、sender、@、conversation、时间等任一有效过滤条件。
