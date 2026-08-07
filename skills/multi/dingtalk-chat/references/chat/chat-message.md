# chat-message：消息、文件、搜索与卡片

> 返回入口：[chat.md](../chat.md)

## 适用场景

用于当前用户发消息、拉消息、搜索消息、撤回、已读状态、文件/图片/音频/视频/位置/名片发送、话题回复、转发、Pin/Top、表情回应、文本翻译和流式卡片。

## 默认入口

| 用户终点 | 默认入口 |
|---|---|
| <!-- dws-intent: chat.send.dm -->按姓名发送简单文本/Markdown | `dws chat +dm` |
| <!-- dws-intent: chat.send.group -->按群名发送简单文本/Markdown | `dws chat +send-to-group` |
| <!-- dws-intent: chat.send.advanced -->文件、Bot、Webhook、复杂 @、已知 ID 或幂等发送 | `dws chat +messages-send` |
| <!-- dws-intent: chat.read.conversation -->读取或导出指定群聊/单聊，可附带发送者姓名 | `dws chat +chat-messages`；姓名用非必填 `--sender-query` |
| <!-- dws-intent: chat.search.filtered -->直接按发送者、关键词、@对象或消息类型搜索，可限定单个或跨多个会话 | `dws chat +search-msg` |
| <!-- dws-intent: chat.reply.quote -->引用回复已有消息 | `dws chat +messages-reply` |
| 查看指定群内 @我的消息 | `dws chat +at-me --group <群名> --page-all` |
| 撤回当前用户消息 | `dws chat +messages-recall --msg-id <openMessageId>` |

以下原子命令只用于 Shortcut 未暴露的底层字段、原始响应或精确分页控制。不得把它们重新写成
上述高频任务的默认路径；写入原子命令若与 Golden Shortcut 的 confirmation 不一致，停止并
报告交付漂移，不以文档确认代替 Runtime gate。

## 必读约束

- 发消息前必须核对接收对象、消息内容、@ 对象、附件路径和消息类型；不明确时先问用户。
- `--group`、`--user`、`--open-dingtalk-id` 通常互斥，群聊用 `--group`，单聊用 `--user` 或 `--open-dingtalk-id`。
- 发送本地文件、音频、视频默认用 `dws chat +messages-send --as user --file <相对路径>`；
  `audio` / `video` 的具体类型以 leaf Schema 为准。只有 Shortcut 未暴露的位置、名片等底层
  类型才进入本文件的原子 fallback。
- 发送位置消息前必须确认纬度、经度、地址名称；地图缩略图需先通过旧媒体上传链路拿到 mediaId。
- 分享联系人名片前必须确认联系人 `openDingTalkId`，不要把 userId 直接当 `--contact-id`。
- 消息内容按 Markdown 渲染，换行必须是真实换行符；需要换行效果时用空行、行尾两个空格或 `<br>`。
- 建议发送时带 `--uuid`，失败重试复用同一个值。
- Bot/Webhook 只支持文本/Markdown；Bot 多群使用 `+messages-send --groups/--groups-file` 的逐项
  ledger。不要把 user 文件/图片能力外推到 Bot。
- `+at-me` 和 `+messages-list-direct` 要求全量时使用 `--page-all`，并检查 `complete`、
  `hasMore`、`stopReason` 和 `failures`；`+messages-list-direct` 的续页时间来自下层毫秒
  `nextCursor`，不得用只有秒精度的消息展示时间手工拼接。

## 原子 fallback 命令明细

### 发送消息的底层 fallback

#### `dws chat message send`（非默认入口）

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

当前没有经过验证的 Thread writer。`openConvThreadId` 只用于 `+thread-replies` 读取或
`+messages-forward-topic` 转发；不要把它作为普通 `--group` 猜测写入。引用回复使用
`+messages-reply`，但这不等于 Thread 内新增回复。

读取话题回复可直接传话题主消息 `--message-id`，CLI 会先通过只读消息详情解析出
`conversationId/threadId`；也可显式传 `--group` 加 `--thread-id/--topic-id`。前一种模式如果同时传
`--group`，会校验它与消息解析出的会话一致；解析失败只会报错，不会错误转去查询通讯录。

默认只读一页；完整读取必须显式加 `--page-all`。可用
`--limit/--page-size` 控制每页条数、用 `--page-limit` 限制最大页数；自动续页使用下层返回的
毫秒级 `nextCursor` 无损生成下一次 `startTime`，不能使用只有秒级精度的回复 `createTime`。
输出默认 `--order desc`（兼容 `--sort`）。由于下层的 `newer/older` 表示读取方向而不是结果排序，
`asc` 只允许与 `--page-all` 一起使用：完整拉取后对整体结果升序排列，避免把单个“最新页”的本地反转
伪装为全局升序。结果中的 `orderScope=complete_result` 表示完整结果排序；读取被页数上限或错误截断时为
`fetched_pages`，并仍须结合完整性 ledger 判断。
必须检查 `complete`、`hasMore`、`stopReason` 和
`failures`，`complete=false` 时不得声称已经拿到全部回复。

```bash
dws chat +thread-replies --message-id <rootOpenMessageId> --page-all --order asc
dws chat +thread-replies --group <openConversationId> --thread-id <openConvThreadId> --page-all --page-limit 50
```

### 拉取消息的底层 fallback

默认使用 `dws chat +chat-messages`。群聊的 `--group` 可传群名或 openConversationId；
也可用 `--chat-query` 显式按群名解析、用 `--conversation-id` 显式传稳定 ID。全量读取加 `--page-all`，必要时用 `--page-limit`、
`--max-results` 控制边界，用 `--output <相对.json>` 原子导出。只有需要原始响应或显式手工
continuation 时才使用下表；原子 `message list` 不代表自动全量分页。

可附带非必填的 `--sender-query <姓名>` 做读取后筛选。未传姓名时正常返回全部；姓名未解析出稳定 ID 时保留全部并记录失败；唯一解析出 userId/openDingTalkId 后按消息 `senderId` 筛选，覆盖最终 `messages/count` 并返回 `resolvedFilters`。该调用已经完成读取、解析与筛选，不要补跑 `+search-msg`。

时间范围参数同样公开但非必填：`--start`（包含）、`--end`（不包含）、`--order asc|desc`，
兼容别名为 `--start-time/--end-time/--sort`。范围固定为 `[start,end)`；仅开始时间表示到本次执行当前时间，
仅结束时间只支持 `desc`，`asc` 必须提供开始时间。旧 `--time/--direction` 保持兼容但不能和范围模式混用。

```bash
dws chat +chat-messages --group "项目群" --sender-query "测试用户甲" --page-all --format json
dws chat +chat-messages --group "项目群" --start "2026-08-01T00:00:00+08:00" --end "2026-08-02T00:00:00+08:00" --order asc --page-all --format json
```

当会话已经确定、任务只需要完整读取结果中可由消息字段判断的子集时，不新增按条件专用的
Shortcut 参数。在同一次 `+chat-messages` 调用中使用全局 `--jq`，让 Runtime 完成读取后、
在 stdout 前筛选；表达式必须保留根信封、用筛选结果覆盖 `messages` 并同步重算 `count`，
不得丢失 `complete`、`hasMore`、`failures` 等完整性 ledger。不要先输出全量 JSON，再由
Agent 或另一条命令二次处理。

```bash
# 例：读取完整会话后，只返回存在 reaction 的消息
dws chat +chat-messages --group "项目群" --page-all --format json \
  --jq '. as $root | [.messages[] | select((.reactions // []) | length > 0)] as $matched | $root | .messages = $matched | .count = ($matched | length)'
```

发送者姓名不是普通结果字段条件：仍用 `--sender-query <姓名>` 先解析稳定身份，再按
`senderId` 筛选，不能用 `--jq` 对展示名做字符串匹配。

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

### 搜索消息的底层 fallback

直接按发送者、关键词、@对象或消息类型检索时优先使用 `dws chat +search-msg`；搜索范围可以是单个、多个或全部会话。若已选择 `+chat-messages` 读取指定会话，可由其非必填 `--sender-query` 在同一次调用完成姓名解析和筛选。

- 搜索内容使用公开参数 `--query`。
- 已知稳定会话 ID 使用 `--group` / `--groups`；已知稳定发送者 ID 使用 `--senders`。
- 只有群名时使用非必填参数 `--chat-query`，由 CLI 唯一解析会话。
- 只有发送者姓名时使用非必填参数 `--sender-query`，由 CLI 唯一解析人员。
- 不要把群名传给只接受稳定 ID 的会话参数，也不要把姓名传给 `--senders`。零命中或多候选时停止，不选择第一项。
- 不传会话过滤时搜索全部会话；`--page-all` 只翻完当前时间范围内的游标页，默认时间范围是最近 7 天。
- 精确范围使用成对的 `--start/--end`（兼容 `--start-time/--end-time`）；`--order`（兼容 `--sort`）稳定排列本次实际取得的结果。未 `--page-all` 或 `complete=false` 时不能称为完整范围的全局排序。

需要 Shortcut 未暴露的原始过滤字段或响应时，才评估 `message search-advanced`。它是原子 `message search` 的严格超集，但不是 Agent 高频默认入口。

```bash
# 单群 + 发送者姓名
dws chat +search-msg --chat-query "项目群" --sender-query "测试用户甲" --page-all --format json

# 单群 + 关键词
dws chat +search-msg --chat-query "项目群" --query "发布计划" --page-all --format json

# 跨全部会话 + 发送者姓名
dws chat +search-msg --sender-query "测试用户甲" --page-all --format json
dws chat +search-msg --query "发布计划" --start-time "2026-08-01T00:00:00+08:00" --end-time "2026-08-02T00:00:00+08:00" --sort asc --page-all --format json
```

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
| `+messages-recall` | 撤回当前用户消息 | `--msg-id`；可选 `--conversation-id`，省略时 CLI 从消息详情补齐；兼容单值 `--message-ids` |
| `message edit` | 编辑已发送消息内容 | `--conversation-id` `--msg-id`，并在 `--text` / `--content` 中二选一；可选 `--title` `--at-all` `--at-open-dingtalk-ids` |
| `message read-status` | 查消息已读/未读状态 | `--group` `--message-id`；可选目标用户 |

`+messages-recall` 与 `recall-by-bot` 不同：前者使用 `openMessageId`，缺少会话 ID 时先只读查询消息详情；后者撤回机器人消息，需要 `robot-code + processQueryKey`。不要把 `processQueryKey` 当 `openMessageId`。

编辑消息使用 `message edit`。推荐传 `--text`，CLI 会生成 markdown content JSON：`{"title":"标题","text":"正文"}`；可选 `--title`，不传时会从正文自动生成标题。高级场景可直接传 `--content`，此时必须是完整 markdown content JSON，且不能同时传 `--text`。

@ 规则：`--at-all` 会传 `atAll=true`，正文应包含 `<@all>`，未包含时 CLI 会自动补到开头；`--at-open-dingtalk-ids` 会传 `atOpenDingTalkIds`，正文需包含对应 `<@openDingTalkId>` 占位符，裸 `@openDingTalkId` 会自动补成尖括号格式。

```bash
dws chat message edit --conversation-id <openConversationId> --msg-id <openMessageId> --text "更新后的内容"
dws chat message edit --group <openConversationId> --msg-id <openMessageId> --title "标题" --text "更新后的内容"
dws chat message edit --group <openConversationId> --msg-id <openMessageId> --text "<@all> 请查看" --at-all
dws chat message edit --group <openConversationId> --msg-id <openMessageId> --text "<@openDingTalkId1> 请查看" --at-open-dingtalk-ids <openDingTalkId1>
dws chat message edit --group <openConversationId> --msg-id <openMessageId> --content '{"title":"标题","text":"更新后的内容"}'
```

### 回复与转发的底层 fallback

引用回复默认使用 `dws chat +messages-reply`。以下原子 reply 只保留底层字段 fallback；转发
仍按各自精确 Shortcut/leaf Schema 选择。

| 命令 | 用途 | 必填参数 |
|------|------|----------|
| `message reply` | 引用回复，单聊/群聊均可；群聊可 @指定成员或 @所有人 | `--conversation-id` `--ref-msg-id` `--ref-sender` `--text`；可选 `--at-open-dingtalk-ids` `--at-all` |
| `message forward` | 转发单条消息，源/目标均支持单聊/群聊 | `--src-conversation-id` `--msg-id` `--dest-conversation-id` |
| `message combine-forward` | 多条消息合并为一条转发 | `--src-conversation-id` `--msg-ids` `--dest-conversation-id`，可选 `--uuid` |
| `message forward-topic` | 转发话题消息 | `--src-msg-id` `--src-conversation-id` `--src-thread-id` `--dest-conversation-id` |

群聊引用回复使用 `--at-open-dingtalk-ids` 传 `atOpenDingTalkIds`；正文缺少对应 `<@openDingTalkId>` 时自动补齐，已有裸 `@openDingTalkId` 会规范化。`--at-all` 会传 `atAll=true`，正文缺少 `<@all>` 时自动补齐。

```bash
dws chat message reply --conversation-id <openConversationId> --ref-msg-id <openMessageId> --ref-sender <senderOpenDingTalkId> --text "请看一下" --at-open-dingtalk-ids <mentionedOpenDingTalkId>
dws chat message reply --conversation-id <openConversationId> --ref-msg-id <openMessageId> --ref-sender <senderOpenDingTalkId> --text "请大家确认" --at-all
```

### 话题与卡片

话题完整读取流程：

1. `dws chat message list --group <openConversationId> --time ...` 获取话题主消息。
2. 如果返回 `openConvThreadId`，执行 `dws chat message list-topic-replies --group <openConversationId> --topic-id <openConvThreadId>`。

流式卡片优先使用公开 Shortcut；创建可选在同一次调用中写入内容：

```bash
dws chat +messages-send-card --group <openConversationId> --content "开始处理" --flow-status 1
dws chat +messages-update-card --biz-id <bizId> --content "更新的卡片内容" --flow-status 2
dws chat +messages-update-card --biz-id <bizId> --content "最终内容" --flow-status 3
```

`flow-status`：1=处理中，2=输入中，3=完成，4=执行中，5=错误，Runtime 拒绝范围外值。
当前只支持 streaming text；不支持 Card JSON 组件或 action callback。精确边界见
[card references](../card/schema.md)。

### Pin / Top / Favorite

| 命令 | 用途 | 必填参数 |
|------|------|----------|
| `message set-pin-msg` / `unset-pin-msg` | 钉住/取消钉住消息 | `--open-conversation-id` `--msg-id` |
| `message list-pin-msg` | 拉取钉住消息列表 | `--open-conversation-id`，可选 `--cursor` `--size` |
| `message set-top-msg` / `unset-top-msg` | 置顶/取消置顶会话内某条消息 | `--open-conversation-id` `--msg-id` |
| `message add-favorite` | 收藏消息 | `--open-message-id` `--open-conversation-id` |
| `message remove-favorite` | 取消收藏消息 | `--open-message-id` `--open-conversation-id` |
| `+flag-list` | 查询收藏消息列表的默认 Shortcut | 默认一页；要求全部时加 `--page-all`；`--page-size/--size` 范围为 1-30，可用 `--page-token` 或兼容的 `--cursor` 续页，检查 `complete` / `failures` |
| `message list-favorites` | 原始单页收藏结果 fallback | 可选 `--cursor` `--size` |

`+flag-list` 查询钉钉 message favorite，底层使用数字 cursor；它与消息 Pin、消息 Top 和会话置顶属于不同对象层级。
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

公开 `+messages-resource-download` 使用工作目录内安全相对路径、默认不覆盖、整文件临时落盘后
原子发布。当前没有 Range/断点续传；失败时保留 ledger 或错误，显式重试整个文件，不拼接残片。

## 常见工作流

### 群聊发文字与文件

```bash
dws chat +send-to-group --group "项目冲刺" --text "请大家本周五前提交周报" --format json
dws chat +messages-send --as user --chat-query "项目冲刺" --file ./report.pdf --uuid <uuid> --format json
dws chat +messages-send --as user --chat-query "项目冲刺" --msg-type audio --file ./voice.mp3 --uuid <uuid> --format json
dws chat +messages-send --as user --chat-query "项目冲刺" --msg-type video --file ./demo.mp4 --uuid <uuid> --format json

# 仅 Shortcut 尚未覆盖的位置/名片底层类型才使用原子 fallback
dws chat message send --group <openConversationId> --msg-type location --latitude <纬度> --longitude <经度> --location-name <地址名称> --map-thumbnail-url "@mediaId" --format json
dws chat message send --group <openConversationId> --msg-type profile --contact-id <openDingTalkId> --format json
```

### 查消息并撤回

```bash
dws chat +chat-messages --group <openConversationId> --direction older --format json
dws chat +messages-recall --msg-id <openMessageId> --format json
```

### 多维度搜索

```bash
dws chat message search-advanced --query "合同" --conversation-ids <cid1>,<cid2> --start "2026-04-01T00:00:00+08:00" --end "2026-04-15T00:00:00+08:00" --limit 50 --cursor 0 --format json
dws chat message search-advanced --message-type file --search-conv-type group_chat --query "附件" --format json
```

## 常见错误与回退

- 发送目标不唯一：保留 resolver 返回的候选并让用户消歧；不要退回手工搜索后选择第一项。
- `unknown flag`：立即执行对应命令 `--help`，不要猜参数。
- 文件/音视频发送失败：确认本地路径可读；新链路使用 `--msg-type file|audio|video --file-path`。
- 位置消息参数不完整：先确认经纬度、地址名称和缩略图 mediaId。
- 名片发送失败：确认 `--contact-id` 是 openDingTalkId，不是 userId。
- 话题回复缺失：检查是否只拉了主消息，需继续用 `list-topic-replies`。
- `search-advanced` 无条件：至少提供 query、sender、@、conversation、时间等任一有效过滤条件。
