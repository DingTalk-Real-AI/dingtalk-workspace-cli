# DING 消息 (ding) 命令参考

## 命令总览

### 发送 DING 消息
```
Usage:
  dws ding message send [flags]
Example:
  dws ding message send --robot-code <ROBOT_CODE> --users <USER_ID_1>,<USER_ID_2> --content "请查看"
Flags:
      --content string      消息内容 (必填)
      --robot-code string   机器人 ID (必填, 可从 应用管理→机器人 获取, 或设 DINGTALK_DING_ROBOT_CODE)
      --users string         接收人 userId 列表 (必填)
      --type string         提醒类型: app/sms/call (默认 app)
```

### 撤回 DING 消息
```
Usage:
  dws ding message recall [flags]
Example:
  dws ding message recall --robot-code <ROBOT_CODE> --id <OPEN_DING_ID>
Flags:
      --id string           DING 消息 ID (必填)
      --robot-code string   机器人 ID (必填, 或设 DINGTALK_DING_ROBOT_CODE)
```

### 查询 DING 消息历史

列表项会同时返回 DING 的 `content`、`openDingId` 与状态，可直接读取钉内容，无需再调用详情接口。

```
Usage:
  dws ding message list [flags]
Example:
  dws ding message list
  dws ding message list --type UNREAD
  dws ding message list --type SEND --cursor 10
Flags:
      --cursor int     分页游标 (首次传 0, 翻页传返回的 nextCursor)
      --type string    消息类型: ALL / UNREAD / SEND / NEW_COMMENT / DELETED (可选, 不传返回全部)
```

### 查看 DING 接收状态
```
Usage:
  dws ding message receiver-status [flags]
Example:
  dws ding message receiver-status --ding-id <OPEN_DING_ID>
  # 查询 dingId: dws ding message list
Flags:
      --ding-id string   DING 消息 openDingId (必填)
```

### 以用户身份发送 DING — 以当前用户身份（非机器人）发送 DING 消息
```
Usage:
  dws ding message send-personal [flags]
Example:
  dws ding message send-personal --users openDingTalkId1,openDingTalkId2 --content "请查看"
  dws ding message send-personal --type call --users openDingTalkId1 --content "紧急告警"
  # 查询 openDingTalkId: dws contact user search --query "姓名"
Flags:
      --users string     接收者 openDingTalkId 列表，逗号分隔 (必填)
      --content string   DING 内容 (必填)
      --type string      提醒类型: app/sms/call (默认 app)
      --uuid string      幂等唯一标识（可选，不传由服务端生成）

注意:
  - 与 `ding message send`（机器人身份）不同：send-personal 以当前用户身份发送，无需 --robot-code
  - 接收者使用 openDingTalkId（非 userId），可通过 `dws contact user search --query "姓名"` 获取
  - sms/call 类型有通信费用，使用前需和用户确认
```

### 以用户身份撤回 DING — 以当前用户身份撤回已发送的 DING 消息
```
Usage:
  dws ding message recall-personal [flags]
Example:
  dws ding message recall-personal --id <openDingId>
  # 查询 openDingId: dws ding message list
Flags:
      --id string   DING 消息 openDingId (必填)

注意:
  - 与 `ding message recall`（机器人身份）不同：recall-personal 以当前用户身份撤回，无需 --robot-code
  - openDingId 可通过 `dws ding message list` 或 `send-personal` 返回值获取
```

### 消息转 DING — 将聊天消息转为 DING 通知发送给指定接收者
```
Usage:
  dws ding message send-by-message [flags]
Example:
  dws ding message send-by-message --group <openConversationId> --message-id <openMessageId> --users id1,id2
  dws ding message send-by-message --group <openConversationId> --message-id <openMessageId> --users id1 --type sms
  # 查询 openDingTalkId: dws contact user search --query "姓名"
  # 查询 openConversationId: dws chat search --query "群名"
Flags:
      --group string       原消息所在会话 openConversationId (必填)
      --message-id string  原消息 openMessageId (必填)
      --users string       接收者 openDingTalkId 列表，逗号分隔 (必填)
      --type string        提醒类型: app/sms/call (默认 app)
      --uuid string        幂等唯一标识（可选，不传由服务端生成）

注意:
  - 与 `send-personal` 不同: send-by-message 是将已有聊天消息转发为 DING，需要指定原消息的会话和消息 ID
  - 接收者使用 openDingTalkId，可通过 `dws contact user search --query "姓名"` 获取
  - sms/call 类型有通信费用，使用前需和用户确认
```

## 意图判断

用户说"DING 一下/紧急通知/电话提醒" → `message send`
用户说"以我的名义 DING/个人发 DING/用户身份 DING" → `message send-personal`
用户说"消息转 DING/把这条消息 DING 给某人/转发为 DING" → `message send-by-message`
用户说"撤回 DING" → `message recall`
用户说"以我的名义撤回 DING/个人撤回 DING" → `message recall-personal`
用户说"DING 消息/查 DING/DING 历史/我的 DING" → `message list`
用户说"DING 接收状态/谁收到了 DING/DING 已读" → `message receiver-status`

关键区分:
- `ding message send`（机器人身份，需 --robot-code） vs `ding message send-personal`（用户身份，无需 robot-code） vs `ding message send-by-message`（消息转 DING，需指定原消息）
- `ding message recall`（机器人身份） vs `ding message recall-personal`（用户身份）
- ding(紧急提醒, 支持电话/短信) vs bot(常规群/单聊消息)
- sms/call 类型有通信费用

## 核心工作流

```bash
# 机器人身份: 应用内 DING (免费)
dws ding message send --robot-code <ROBOT_CODE> --type app --users userId1,userId2 --content "请查看" --format json

# 机器人身份: 电话 DING (紧急, 有成本!)
dws ding message send --robot-code <ROBOT_CODE> --type call --users userId1 --content "紧急告警" --format json

# 机器人身份: 撤回
dws ding message recall --robot-code <ROBOT_CODE> --id <OPEN_DING_ID> --format json

# 用户身份: 应用内 DING
dws ding message send-personal --users openDingTalkId1,openDingTalkId2 --content "请查看" --format json

# 用户身份: 电话 DING (紧急, 有成本!)
dws ding message send-personal --type call --users openDingTalkId1 --content "紧急告警" --format json

# 用户身份: 消息转 DING
dws ding message send-by-message --group <openConversationId> --message-id <openMessageId> --users openDingTalkId1,openDingTalkId2 --format json

# 用户身份: 撤回
dws ding message recall-personal --id <OPEN_DING_ID> --format json
```
## 上下文传递表
| 操作 | 提取 | 用于 |
|------|------|------|
| `message send` | `openDingId` | message recall 的 --id |
| `message send-personal` | `openDingId` | message recall-personal 的 --id |
| `message list` | `openDingId` | message receiver-status 的 --ding-id |
| `message send-by-message` | `openDingId` | message recall-personal 的 --id |
## 注意事项
- `--robot-code` 从钉钉开放平台 应用管理 → 机器人 中获取，也可设环境变量 `DINGTALK_DING_ROBOT_CODE`
- `send` / `recall` 是机器人身份，需要 --robot-code；`send-personal` / `recall-personal` / `send-by-message` 是用户身份，无需 robot-code
- `send` 接收者使用 userId；`send-personal` / `send-by-message` 接收者使用 openDingTalkId（可通过 `dws contact user search --query "姓名"` 获取）
- `send-by-message` 是将已有聊天消息转发为 DING，需指定 --group 和 --message-id
- sms/call 类型有通信费用，使用前需和用户确认
- 默认 `--type app` 为应用内 DING（免费）
