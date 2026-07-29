# chat-bot：机器人与 Webhook

> 返回入口：[chat.md](../chat.md)

## 适用场景

用于搜索机器人、机器人发送/撤回消息、Webhook 告警、机器人加入/移出群，以及给机器人发单聊消息。

## 必读约束

- 用户明确要求“用机器人/机器人身份/robot”发送时，必须用 `chat message send-by-bot`，严禁改用 `chat message send`。
- `chat bot search` 只返回我创建的机器人，没有 `openDingTalkId`；给机器人发单聊必须用 `chat bot find`。
- 机器人发群消息前需确认机器人已在群中；报“机器人不存在”时先 `group members add-bot`。
- `send-by-bot --text` 支持 Markdown；需要稳定换行时用空行分隔段落。若以转义形式组织文本，写 `\n\n`，不要只写 `\n`。
- `recall-by-bot` 使用 `processQueryKey`，不是 `openMessageId`。

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

#### `dws chat message send-by-bot`

```bash
# 群聊
dws chat message send-by-bot --robot-code <robot-code> --group <openConversationId> --title "日报" --text "## 今日完成\n\n- 事项 A\n\n- 事项 B"

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
| `--group` | 群聊 openConversationId |
| `--users` | 单聊 userId 列表，逗号分隔，最多 20 个 |
| `--open-dingtalk-ids` | 单聊 openDingTalkId 列表 |
| `--title` / `--text` | 消息标题与 Markdown 内容，必填；Markdown 换行用空行，转义表示为 `\n\n` |
| `--at-user-ids` / `--at-open-dingtalk-ids` | 群聊 @ 指定成员，正文需含对应 `@id` 文本 |
| `--at-all` | 群聊 @所有人 |

#### `dws chat message recall-by-bot`

```bash
dws chat message recall-by-bot --robot-code <robot-code> --group <openConversationId> --keys <processQueryKey>
dws chat message recall-by-bot --robot-code <robot-code> --keys key1,key2
```

群聊撤回传 `--group`；单聊撤回不传 `--group`。`--keys` 来自 `send-by-bot` 返回的 `processQueryKey`。

### Webhook

```bash
dws chat message send-by-webhook --token <webhook-token> --title "告警" --text "CPU 超 90% @10" --at-all
dws chat message send-by-webhook --token <webhook-token> --title "test" --text "hi @118785" --at-users 118785
```

关键规则：

- `--token`、`--title`、`--text` 必填。
- `--at-all` 时 `--text` 中需包含 `@10`。
- `--at-users` 或 `--at-mobiles` 时，`--text` 中需包含对应 `@userId` 或 `@手机号`，否则 @ 不生效。

### 机器人进群

| 命令 | 用途 | 必填参数 |
|------|------|----------|
| `group members add-bot` | 将自定义机器人加入群 | `--id` `--robot-code` |
| `group members remove-bot` | 从群移除机器人 | `--id` `--bot-id` |
| `group bots` | 查看群内机器人列表 | `--group` |

```bash
dws chat group members add-bot --id <openConversationId> --robot-code <robot-code>
dws chat group bots --group <openConversationId>
dws chat group members remove-bot --id <openConversationId> --bot-id <openBotId>
```

## 常见工作流

### 机器人发消息后撤回

```bash
dws chat bot search --name "日报" --format json
dws chat message send-by-bot --robot-code <robot-code> --group <openConversationId> --title "日报" --text "## 今日完成\n\n- 事项 A\n\n- 事项 B" --format json
dws chat message recall-by-bot --robot-code <robot-code> --group <openConversationId> --keys <processQueryKey> --format json
```

### 机器人不在群内时先邀请再发送

```bash
dws chat bot search --name "日报" --format json
dws chat group members add-bot --id <openConversationId> --robot-code <robot-code> --format json
dws chat message send-by-bot --robot-code <robot-code> --group <openConversationId> --title "通知" --text "内容" --format json
```

### 给机器人发单聊

```bash
dws chat bot find --query "玉澜" --format json
dws chat message send --open-dingtalk-id <openDingTalkId> --text "你好" --format json
```

### 机器人 @ 指定人

```bash
dws aisearch person --keyword "张三" --dimension name --format json
dws chat message send-by-bot --robot-code <robot-code> --group <openConversationId> --at-user-ids userId1 --title "提醒" --text "@userId1 请查收" --format json
```

## 常见错误与回退

- 机器人单聊没有 openDingTalkId：改用 `chat bot find`，不要用 `bot search`。
- 机器人发群消息报“机器人不存在”：先 `group members add-bot`。
- 撤回失败：确认使用 `processQueryKey`，不是 `openMessageId`。
- @ 不生效：检查正文是否包含 `@userId` / `@openDingTalkId` / `@10`。
