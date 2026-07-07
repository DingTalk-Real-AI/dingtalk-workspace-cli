# 个人消息事件消费参考

本参考只覆盖个人消息事件：

| 事件码 | 规则 | 用途 | 必填参数 |
|--------|------|------|----------|
| `user_im_message_receive_at` | `at` | 当前用户被 @ 的消息 | 无 |
| `user_im_message_receive_o2o` | `singleChat` | 当前用户与指定用户的单聊消息 | `--peer-user-id` 或 `--peer-union-id` |
| `user_im_message_receive_group` | `group` | 当前用户所在指定群聊/会话的消息 | `--open-conversation-id` |
| `user_im_message_receive_user` | `sender` | 当前用户收到的指定发送人消息 | `--sender-user-id` 或 `--sender-union-id` |

不要使用本参考以外的事件码。默认身份就是当前用户，不要额外加身份 flag。

## 参数解析规则

- 人名 → 先执行 AI 搜问人员搜索，取确认后的 `userId`。
- 群名 → 先执行群聊搜索，取确认后的 `openConversationId`。
- 多个候选 → 展示候选并让用户确认。
- 未提供必要 ID 且无法解析 → 先追问，不要猜测。

## 常用参数

| 参数 | 说明 |
|------|------|
| `-f, --format ndjson` | 推荐输出，一行一个事件 JSON |
| `--max-events <n>` | 收到 N 条后退出 |
| `--duration <duration>` | 到时退出，如 `30s`、`10m` |
| `--output-dir <dir>` | 每个事件写入一个文件 |
| `--route '<regex>=dir:<path>'` | 按事件类型路由到目录 |
| `--subscribe-id <id>` | 复用已有个人订阅 |
| `--personal-event-base-url <url>` | 联调环境覆盖控制面 base URL |
| `--stream-ticket-url <url>` | 联调环境覆盖取票 URL |
| `--stream-source-id <id>` | 联调环境覆盖 sourceId |
| `--debug-raw-events` | 联调用：输出当前 personal stream bus 收到的全部可解析事件 |

`--debug-raw-events` 只用于排查服务端推送是否到达本地连接；正常 Agent 消费不要使用。

## 示例

### 被 @ 消息

```bash
dws event consume user_im_message_receive_at \
  -f ndjson
```

### 指定单聊消息

```bash
dws event consume user_im_message_receive_o2o \
  --peer-user-id 507971 \
  -f ndjson
```

只有 unionId 时：

```bash
dws event consume user_im_message_receive_o2o \
  --peer-union-id union123 \
  -f ndjson
```

### 指定群消息

```bash
dws event consume user_im_message_receive_group \
  --open-conversation-id cidxxxxxxxx \
  -f ndjson
```

### 指定发送人消息

```bash
dws event consume user_im_message_receive_user \
  --sender-user-id 507971 \
  -f ndjson
```

只有 unionId 时：

```bash
dws event consume user_im_message_receive_user \
  --sender-union-id union123 \
  -f ndjson
```

### 获取一条 JSON 样本

```bash
dws event consume user_im_message_receive_at \
  --max-events 1 \
  -f json
```

### 联调预发环境

```bash
dws event consume user_im_message_receive_o2o \
  --peer-user-id 507971 \
  --personal-event-base-url https://pre-mcp.dingtalk.com/dws \
  --stream-ticket-mode normal \
  --stream-source-id pre_open_source \
  --stream-ticket-url https://pre-mcp.dingtalk.com/stream/connections/ticket \
  -f ndjson
```

## 输出

`-f ndjson` 每行是一个事件对象，常见字段：

| 字段 | 说明 |
|------|------|
| `event_type` | 个人事件码 |
| `subscribe_id` | 个人订阅 ID |
| `source_id` | 当前 sourceId |
| `data` | 服务端业务 payload 原文 |
| `headers` | Stream 帧 headers |
| `received_at_unix_ms` | 本地接收时间 |

`data` 当前是 JSON 字符串，不是已展开对象。读取业务字段前先对 `data` 再做一次 JSON 解析。

解析后的常用字段：

| 字段 | 说明 |
|------|------|
| `payload.body.content` | 消息文本内容 |
| `payload.body.sender` | 发送人展示名 |
| `payload.body.openConversationId` | 开放会话 ID |
| `payload.body.openMessageId` | 开放消息 ID |
| `payload.body.senderOpenDingTalkId` | 发送人的开放钉钉 ID |
| `payload.uid` | 当前个人事件主体 uid |
| `eventKey` | 个人事件码 |
| `subId` | 服务端订阅 ID |

## 状态与停止

```bash
dws event status --event user_im_message_receive_at
dws event status --event user_im_message_receive_o2o
dws event status --event user_im_message_receive_group
dws event status --event user_im_message_receive_user
```

停止指定订阅：

```bash
dws event stop <subscribe_id>
```

清理当前身份下本地记录的全部个人订阅：

```bash
dws event stop --all
```
