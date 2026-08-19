# chat-bot：机器人与 Webhook

> 返回入口：[chat.md](../chat.md)

## 适用场景

用于搜索机器人、机器人发送/撤回消息、Webhook 告警、机器人加入/移出群，以及给机器人发单聊消息。

<!-- dws-intent: chat.send.advanced -->Bot/Webhook 默认统一使用 `dws chat +messages-send`，通过
`--as bot|webhook` 选择身份；原子发送命令只保留 Shortcut 未发布字段的底层 fallback。

## 必读约束

- 用户明确要求“用机器人/机器人身份/robot”发送时，使用
  `dws chat +messages-send --as bot --robot-code <robotCode>`；不得改成当前用户身份。
- `chat bot search` 只返回我创建的机器人，没有 `openDingTalkId`；给机器人发单聊必须用 `chat bot find`。
- 机器人发群消息前需确认机器人已在群中；报“机器人不存在”时先 `group members add-bot`。
- `send-by-bot` 支持 Markdown、图片 URL 和文件，具体参数见下方消息类型路由。
- 机器人在群聊中引用回复已有消息时，使用原子命令 `send-by-bot --reply --ref-sender`；该能力仅支持 Markdown，不走 `+messages-reply` 的当前用户身份。
- 公网图片 URL 使用 `--msg-type image --image-url`，按图片消息发送。
- 本地图片和其他本地文件一样使用 `--msg-type file --file-path`，由 CLI 上传并按文件附件发送。
- 群聊传 `--group`；单聊可传 `--users`、`--open-dingtalk-ids` 或两者组合。
- Markdown 必须同时传 `--title` 和 `--text`；需要稳定换行时用空行分隔段落。若以转义形式组织文本，写 `\n\n`，不要只写 `\n`。
- `recall-by-bot` 使用 `processQueryKey`，不是 `openMessageId`。
- Bot 多群文本/Markdown 直接使用 `+messages-send --groups <cid...>` 或
  `--groups-file <工作目录内相对文件>`；最多 100 个稳定 ID，Runtime 去重并返回
  `im.batch-write.v1` 逐目标 ledger。
- `+messages-send` 的 Bot 路由和 Webhook 没有与 current-user 等价的文件、图片、音视频发送接口；
  机器人富媒体必须使用原子 `chat message send-by-bot`，不得转成文本或换身份静默发送。

## 命令明细

### 机器人搜索

| 命令 | 范围 | 返回 openDingTalkId | 典型触发词 |
|------|------|---------------------|------------|
| `chat bot search` | 仅当前用户自己创建的机器人 | 否 | “我的机器人”“我创建的机器人” |
| `chat bot find` | 当前用户可用的全部机器人（含他人/官方） | 是 | “找机器人”“搜索机器人”“给机器人发单聊” |

```bash
dws chat bot search --page 1 --size 10 --name "日报"
dws chat bot find --query "日报" --limit 20
dws chat bot find --query "日报" --limit 20 --cursor <nextCursor>
```

`bot find` 翻页时 `cursor` 必须使用上次返回的 `nextCursor` 字符串原值，不要传 `"0"` 或数字字面量。

### 机器人发送与撤回

多群正式入口：

```bash
dws chat +messages-send --as bot --robot-code <robot-code> \
  --groups <openConversationId1>,<openConversationId2> \
  --markdown "## 通知\n\n请提交周报" --format json
```

读取 `requestedCount/succeededCount/failedCount/results/failures`；unknown 或失败目标不自动重发。

#### `dws chat message send-by-bot`（底层 fallback）

普通文本/Markdown、群聊/批量单聊和 @ 已由 `+messages-send --as bot` 覆盖。只有 Shortcut
缺失真实必需字段且精确 leaf Schema 允许时才使用以下原子命令。

```bash
# 群聊
dws chat message send-by-bot --robot-code <robot-code> --conversation-id <openConversationId> --title "日报" --text "## 今日完成\n\n- 事项 A\n\n- 事项 B"
dws chat message send-by-bot --robot-code <robot-code> --conversation-id <openConversationId> --reply <openMessageId> --ref-sender <senderOpenDingTalkId> --text "收到"
dws chat message send-by-bot --robot-code <robot-code> --conversation-id <openConversationId> --msg-type image --image-url "https://example.com/image.png"
dws chat message send-by-bot --robot-code <robot-code> --conversation-id <openConversationId> --msg-type file --file-path ./report.pdf

# 单聊 userId
dws chat message send-by-bot --robot-code <robot-code> --users userId1,userId2 --title "提醒" --text "请提交周报"

# 单聊 openDingTalkId
dws chat message send-by-bot --robot-code <robot-code> --open-dingtalk-ids openDingTalkId1,openDingTalkId2 --title "提醒" --text "请提交周报"

# 群聊 @ 人
dws chat message send-by-bot --robot-code <robot-code> --group <openConversationId> --at-user-ids userId1,userId2 --title "提醒" --text "@userId1 @userId2 请查收"
dws chat message send-by-bot --robot-code <robot-code> --group <openConversationId> --at-open-dingtalk-ids openDingTalkId1,openDingTalkId2 --title "提醒" --text "@openDingTalkId1 @openDingTalkId2 请查收"
dws chat message send-by-bot --robot-code <robot-code> --group <openConversationId> --at-all --title "通知" --text "请所有人注意"
```

关键 flags：

| Flag | 说明 |
|------|------|
| `--robot-code` | 机器人 Code，必填 |
| `--conversation-id` | 群聊 openConversationId；`--group` 为兼容别名 |
| `--users` | 单聊 userId 列表，逗号分隔，最多 20 个 |
| `--open-dingtalk-ids` | 单聊 openDingTalkId 列表 |
| `--msg-type` | `markdown`、`image` 或 `file`；省略时为 Markdown；公网图片使用 `image --image-url`，本地图片和文件使用 `file --file-path` |
| `--text` | Markdown 消息内容，Markdown 模式必填；换行用空行，转义表示为 `\n\n` |
| `--title` | 普通 Markdown 消息标题；引用回复省略时由 CLI 从正文生成 |
| `--image-url` | 公网图片 URL，`--msg-type image` 时必填 |
| `--file-path` | 本地图片或文件路径，`--msg-type file` 时由 CLI 上传并按文件附件发送 |
| `--at-user-ids` / `--at-open-dingtalk-ids` | 群聊 @ 指定成员，正文需含对应 `@id` 文本 |
| `--at-all` | 群聊 @所有人 |
| `--reply` | 被引用消息的 `openMessageId`；仅群聊 Markdown，必须与 `--ref-sender` 同时使用 |
| `--ref-sender` | 被引用消息发送者的 `openDingTalkId`；仅群聊 Markdown，必须与 `--reply` 同时使用 |

引用回复不会设置 `msgType=reply`；CLI 在普通群消息参数顶层透传 `referenceOpenMessageId` 和 `srcMsgSendOpenDingTalkId`。只传其中一个参数、用于单聊或用于图片/文件消息都会在本地失败。

#### `dws chat message recall-by-bot`

```bash
dws chat message recall-by-bot --robot-code <robot-code> --group <openConversationId> --keys <processQueryKey>
dws chat message recall-by-bot --robot-code <robot-code> --keys key1,key2
```

群聊撤回传 `--group`；单聊撤回不传 `--group`。`--keys` 来自 `send-by-bot` 返回的 `processQueryKey`。

### Webhook

默认使用：

```bash
dws chat +messages-send --as webhook --webhook-token <webhook-token> --title "告警" --text "CPU 超 90% @10" --at-all
```

以下是底层 fallback，不作为默认选路：

```bash
dws chat message send-by-webhook --token <webhook-token> --title "告警" --content "CPU 超 90% @10" --at-all
dws chat message send-by-webhook --token <webhook-token> --title "test" --content "hi @118785" --at-users 118785
```

关键规则：

- `--token`、`--title`、`--content` 必填。
- `--at-all` 时 `--content` 中需包含 `@10`。
- `--at-users` 或 `--at-mobiles` 时，`--content` 中需包含对应 `@userId` 或 `@手机号`，否则 @ 不生效。

### 机器人进群

| 命令 | 用途 | 必填参数 |
|------|------|----------|
| `group members add-bot` | 将自定义机器人加入群 | `--id` `--robot-code` |
| `group members remove-bot` | 从群移除机器人 | `--id` `--bot-id` |
| `+chat-bots` | 查看群内机器人列表 | `--group <群名或openConversationId>`；自然群名内部唯一解析 |

```bash
dws chat group members add-bot --id <openConversationId> --robot-code <robot-code>
dws chat +chat-bots --group "项目群"
dws chat group members remove-bot --id <openConversationId> --bot-id <openBotId>
```

## 常见工作流

### 机器人发消息后撤回

```bash
dws chat bot search --name "日报" --format json
dws chat +messages-send --as bot --robot-code <robot-code> --group <openConversationId> --title "日报" --markdown "## 今日完成\n\n- 事项 A\n\n- 事项 B" --format json
dws chat message recall-by-bot --robot-code <robot-code> --group <openConversationId> --keys <processQueryKey> --format json
```

### 机器人不在群内时先邀请再发送

```bash
dws chat bot search --name "日报" --format json
dws chat group members add-bot --id <openConversationId> --robot-code <robot-code> --format json
dws chat +messages-send --as bot --robot-code <robot-code> --group <openConversationId> --title "通知" --text "内容" --format json
```

### 给机器人发单聊

```bash
dws chat bot find --query "玉澜" --format json
dws chat message send --open-dingtalk-id <openDingTalkId> --content "你好" --format json
```

### 机器人 @ 指定人

```bash
dws aisearch person --query "张三" --dimension name --format json
dws chat +messages-send --as bot --robot-code <robot-code> --group <openConversationId> --at-user-ids userId1 --title "提醒" --text "@userId1 请查收" --format json
```

## 常见错误与回退

- 机器人单聊没有 openDingTalkId：改用 `chat bot find`，不要用 `bot search`。
- 机器人发群消息报“机器人不存在”：先 `group members add-bot`。
- 撤回失败：确认使用 `processQueryKey`，不是 `openMessageId`。
- 机器人引用回复失败：确认目标是群聊 Markdown，且 `--reply` 来自被引用消息的 `openMessageId`、`--ref-sender` 来自同一消息的发送者 `openDingTalkId`。
- @ 不生效：检查正文是否包含 `@userId` / `@openDingTalkId` / `@10`。
- 需要 Bot 图片/文件/音视频：停止并说明当前身份矩阵不支持；只有用户明确同意改为当前用户身份时，才重新确认目标和内容。
