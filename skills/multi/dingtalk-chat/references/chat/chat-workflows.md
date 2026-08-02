# chat-workflows：高频流程与上下文传递

> 返回入口：[chat.md](../chat.md)

## 适用场景

用于复合任务编排、字段传递、openDingTalkId 获取、自动化脚本和跨产品衔接。

## 必读约束

- 已知上游字段时直接使用，不要重复搜索。
- 群聊 `openConversationId` 通常来自 `chat search`；单聊 `openConversationId` 来自 `conversation-info`。
- `userId` 与 `openDingTalkId` 不是同一种标识，按命令要求传递。

## openDingTalkId 获取方式

| 场景 | 命令 |
|------|------|
| 搜人员 openDingTalkId | `dws aisearch person --keyword "姓名" --dimension name --format json` |
| 给机器人发单聊 | `dws chat bot find --query "机器人名" --format json`，不要用 `bot search` |
| 已有 userId 需要单聊会话 ID | `dws chat conversation-info --user <userId> --format json` |
| 已有 openDingTalkId 需要单聊会话 ID | `dws chat conversation-info --open-dingtalk-id <openDingTalkId> --format json` |

## 高频工作流

### 群聊消息

```bash
# 1. 搜索群，提取 openConversationId
dws chat search --query "项目冲刺" --format json

# 2. 拉取群消息
dws chat message list --group <openConversationId> --time "2025-03-01 00:00:00" --direction older --format json

# 3. 拉未读会话
dws chat message list-unread-conversations --count 20 --format json
```

### 发送消息

```bash
# 个人身份发群消息
dws chat message send --group <openConversationId> --title "周报提醒" --text "请大家本周五前提交周报" --uuid <uuid> --format json

# 个人身份发单聊
dws chat message send --user <userId> --text "你好" --uuid <uuid> --format json
dws chat message send --open-dingtalk-id <openDingTalkId> --text "你好" --uuid <uuid> --format json

# 发送图片/文件/音频/视频
dws chat message send --group <openConversationId> --msg-type file --file-path ./screenshot.png --format json
dws chat message send --open-dingtalk-id <openDingTalkId> --msg-type file --file-path ./report.pdf --format json
dws chat message send --group <openConversationId> --msg-type audio --file-path ./voice.mp3 --format json
dws chat message send --group <openConversationId> --msg-type video --file-path ./demo.mp4 --format json

# 文件后补文字说明
dws chat message send --open-dingtalk-id <openDingTalkId> --msg-type file --file-path ./screenshot.png --format json
dws chat message send --open-dingtalk-id <openDingTalkId> --text "这是本周数据汇总" --format json
```

### 机器人消息

```bash
# 查我的机器人，提取 robotCode
dws chat bot search --name "日报" --format json

# 机器人发群消息，提取 processQueryKey；Markdown 稳定换行用 \n\n，不要只用 \n
dws chat message send-by-bot --robot-code <robot-code> --group <openConversationId> --title "日报" --text "## 今日完成\n\n- 事项 A\n\n- 事项 B" --format json

# 撤回机器人消息
dws chat message recall-by-bot --robot-code <robot-code> --group <openConversationId> --keys <processQueryKey> --format json

# 机器人不在群内时先邀请
dws chat group members add-bot --id <openConversationId> --robot-code <robot-code> --format json
dws chat message send-by-bot --robot-code <robot-code> --group <openConversationId> --title "通知" --text "内容" --format json

# 给机器人发单聊，必须用 find 拿 openDingTalkId
dws chat bot find --query "玉澜" --format json
dws chat message send --open-dingtalk-id <openDingTalkId> --text "你好" --format json
```

### 机器人 @ 指定人

```bash
dws aisearch person --keyword "张三" --dimension name --format json

dws chat message send-by-bot --robot-code <robot-code> --group <openConversationId> \
  --at-user-ids userId1,userId2 \
  --title "提醒" --text "@userId1 @userId2 请查收本周报告" --format json

dws chat message send-by-bot --robot-code <robot-code> --group <openConversationId> \
  --at-open-dingtalk-ids openDingtalkId1,openDingtalkId2 \
  --title "提醒" --text "@openDingtalkId1 @openDingtalkId2 请查收本周报告" --format json

dws chat message send-by-bot --robot-code <robot-code> --group <openConversationId> \
  --at-all --title "通知" --text "请所有人注意" --format json
```

### Webhook 告警

```bash
dws chat message send-by-webhook --token <webhook-token> --title "告警" --text "CPU 超 90% @10" --at-all --format json
```

### 话题完整读取与回复

```bash
dws chat message list --group <openConversationId> --time "2026-03-10 00:00:00" --direction older --format json
dws chat message list-topic-replies --group <openConversationId> --topic-id <openConvThreadId> --format json
dws chat message send --group <openConvThreadId> --text "回复话题内容" --format json
```

### 群公告与邀请分享

```bash
# 分享 A 群邀请链接到 B 群
dws chat group share-invite --source <sourceOpenConversationId> --target <targetOpenConversationId> --format json

# 发布群公告，Markdown 段落换行用空行
dws chat group notice create --group <openConversationId> --content "# 重要通知\n\n请大家查收" --send-ding --format json

# 修改公告前先列表查询 notice-id/dataId
dws chat group notice list --group <openConversationId> --format json
dws chat group notice edit --group <openConversationId> --notice-id <dataId> --content "完整的新公告内容" --format json
```

### 智能分组与文本翻译

```bash
dws aisearch person --keyword "张三" --dimension name --format json
dws chat category create-smart --name "重点群" --keywords "重点,项目" --members openDingTalkId1 --format json
dws chat text translate --query "你好世界" --to en_US --format json
```

## 上下文传递表

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `chat search` | `openConversationId` | `message send/list`、`group members`、`set-top`、`group-mute` |
| `chat group create` | `openConversationId` | 新群后续发消息、成员管理、群设置 |
| `chat group get-by-group-id` | `openConversationId` | 将数字群号转为可用会话 ID |
| `chat conversation-info` | `openConversationId` | 单聊消息搜索、转发、会话状态操作 |
| `aisearch person` | `userId` | `message send --user`、`send-by-bot --users`、`--at-user-ids`、`list-by-sender --sender-user-id` |
| `aisearch person` | `openDingTalkId` | `message send --open-dingtalk-id`、`--at-open-dingtalk-ids`、`send-by-bot --open-dingtalk-ids` |
| `aisearch person` | `userId` | 可替代人员搜索结果，继续用于 userId 参数 |
| `chat bot search` | `robotCode` | `send-by-bot`、`recall-by-bot` |
| `chat bot find` | `openDingTalkId` | 给机器人发单聊 |
| `chat group bots` | `openBotId` | `group members remove-bot --bot-id` |
| `chat message send-by-bot` | `processQueryKey` | `recall-by-bot --keys` |
| `chat message send` | `openTaskId` | `query-send-status --open-task-id` |
| `chat message list` | `openMessageId` | `message recall --msg-id`、`reply --ref-msg-id`、`forward --msg-id` |
| `chat message list` | `openMsgId` | `read-status --message-id`、表情回应命令 |
| `chat message list` | `openConvThreadId` | `list-topic-replies --topic-id`、话题回复 `message send --group` |
| `chat message search` | `nextCursor` | 下次 `message search --cursor` |
| `chat message search-advanced` | `nextCursor` | 下次 `search-advanced --cursor` |
| `chat message list-all` | `nextCursor` | 下次 `list-all --cursor` |
| `chat search-common` | `openConversationId` | 后续消息发送/拉取 |
| `chat group notice list/get/create` | `dataId` / `notice-id` | `group notice get/edit --notice-id` |
| `chat group-role list` | `openRoleId` | `group-role update/remove/set-user/remove-user --role-id` |
| `chat message create-text-emotion` | `emotionId` | `add-text-emotion --emotion-id` |
| `chat message list/search-advanced` | `openMessageId` / `openMsgId` | `list-emotion-replies --msg-ids` |
| `chat category list` | `categoryId` | `category list-conversations/add-conv/remove-conv --category-id(s)` |
| `chat category create-smart` | 智能分组 ID/规则结果 | 后续查看或调整智能分组 |
| `chat message send-card` | `bizId` | `update-card --biz-id` |

## 自动化脚本

| 脚本 | 场景 | 用法 |
|------|------|------|
| [chat_export_messages.py](../../scripts/chat_export_messages.py) | 导出群聊消息到 JSON 文件 | `python chat_export_messages.py --query "项目冲刺" --time "2026-03-10 00:00:00"` |
| [chat_history_with_user.py](../../scripts/chat_history_with_user.py) | 查询与某人的单聊聊天记录 | `python chat_history_with_user.py --name "张三" --time "2026-03-10 00:00:00"` |
| [extract_media_id.py](../../scripts/extract_media_id.py) | 旧链路：从 dt_media_upload URL 提取 mediaId | 新场景不要用，文件/音视频直接 `--msg-type file|audio|video --file-path` |

## 常见错误与回退

- 字段已由上游返回：直接传递，不要再次搜索。
- `chat bot search` 没有 openDingTalkId：改用 `chat bot find`。
- 群号是纯数字：先 `chat group get-by-group-id`。
- 翻页不要自造 cursor：使用响应中的 `nextCursor` 原值。
- 文件/音视频发送不要走旧上传链路：优先 `message send --msg-type file|audio|video --file-path`；`audio/video` 底层按 `file` 发送。
