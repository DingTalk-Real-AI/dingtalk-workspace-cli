---
name: dingtalk-event
description: 钉钉个人消息事件监听、订阅与消费。Use when 用户提到 监听个人消息事件、被@消息、监听我和某人的单聊消息、监听某个群消息、监听指定发送人发给我的消息、实时接收钉钉消息事件、dws event consume user_im_message_receive_at/user_im_message_receive_o2o/user_im_message_receive_group/user_im_message_receive_user、用事件驱动 Agent 处理钉钉消息。命令前缀：dws event。
---

# 钉钉个人消息事件 Skill

本 skill 只暴露个人消息事件里已经实现的 4 个事件：

| 事件码 | 场景 | 必填参数 |
|--------|------|----------|
| `user_im_message_receive_at` | 当前用户被 @ 的消息 | 无 |
| `user_im_message_receive_o2o` | 当前用户与指定用户的单聊消息 | `--peer-user-id` 或 `--peer-union-id` |
| `user_im_message_receive_group` | 当前用户所在指定群聊/会话的消息 | `--open-conversation-id` |
| `user_im_message_receive_user` | 当前用户收到的指定发送人消息 | `--sender-user-id` 或 `--sender-union-id` |

当前 skill 只覆盖个人消息事件；其它身份模式不在本 skill 范围内。

## 前置规则

- 使用当前用户 OAuth 登录态；需要时先让用户执行 auth login。
- `user` 是 event 命令的默认身份，不要额外加身份 flag。
- 不主动运行事件目录列表作为能力菜单；只承认上表 4 个事件码。
- 缺少必填 ID 时先解析或追问，不要猜测 ID。
- 用户只给人名时，先用 AI 搜问人员搜索按姓名解析 userId；多候选必须让用户确认。
- 用户只给群名时，先用群聊搜索解析 `openConversationId`；多候选必须让用户确认。

## 核心命令

| 意图 | 命令 |
|------|------|
| 查看事件说明 | event schema + 事件码 |
| 监听被 @ 消息 | event consume + `user_im_message_receive_at` + `-f ndjson` |
| 监听指定单聊 | event consume + `user_im_message_receive_o2o` + `--peer-user-id <userId>` 或 `--peer-union-id <unionId>` + `-f ndjson` |
| 监听指定群消息 | event consume + `user_im_message_receive_group` + `--open-conversation-id <openConversationId>` + `-f ndjson` |
| 监听指定发送人 | event consume + `user_im_message_receive_user` + `--sender-user-id <userId>` 或 `--sender-union-id <unionId>` + `-f ndjson` |
| 查看订阅状态 | event status + `--event <event_code>` |
| 停止指定订阅 | event stop + `subscribe_id` |
| 停止当前身份下所有本地记录的个人订阅 | event stop all |

## 常用模式

### 监听被 @ 消息

```bash
dws event consume user_im_message_receive_at \
  -f ndjson
```

### 监听指定用户的单聊消息

```bash
dws event consume user_im_message_receive_o2o \
  --peer-user-id 507971 \
  -f ndjson
```

### 监听指定群消息

```bash
dws event consume user_im_message_receive_group \
  --open-conversation-id cidxxxxxxxx \
  -f ndjson
```

### 监听指定发送人的消息

```bash
dws event consume user_im_message_receive_user \
  --sender-user-id 507971 \
  -f ndjson
```

### 有界自测

```bash
dws event consume user_im_message_receive_at \
  --duration 10m \
  -f ndjson
```

### 抓一条样本

```bash
dws event consume user_im_message_receive_o2o \
  --peer-user-id 507971 \
  --max-events 1 \
  -f json
```

## 输出处理

- 默认推荐 `-f ndjson`：stdout 每行一个事件 JSON，适合 Agent 管道读取。
- 人工查看单条样本可用 `-f json --max-events 1`。
- 长时间监听时用 `--duration` 或外部进程管理控制生命周期。
- `data` 以服务端实际推送样本为准，不要把 CLI schema 的简化结构当作权威 payload 协议。
- `--debug-raw-events` 只用于和服务端联调，正常 Agent 消费不要使用。

## 参考

- 详细消费参数：[references/dingtalk-event-consume.md](references/dingtalk-event-consume.md)
- 自测流程：[references/runbook.md](references/runbook.md)
