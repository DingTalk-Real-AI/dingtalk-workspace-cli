# dws event — 个人消息事件

通过个人 Stream 长连接监听当前用户收到的钉钉消息事件，NDJSON 输出到 stdout，用于驱动事件触发的 Agent。

当前只暴露 4 个个人消息事件：

| 事件码 | 场景 | 必填参数 |
|--------|------|----------|
| `user_im_message_receive_at` | 当前用户被 @ 的消息 | 无 |
| `user_im_message_receive_o2o` | 当前用户与指定用户的单聊消息 | `--peer-user-id` 或 `--peer-union-id` |
| `user_im_message_receive_group` | 当前用户所在指定群聊/会话的消息 | `--open-conversation-id` |
| `user_im_message_receive_user` | 当前用户收到的指定发送人消息 | `--sender-user-id` 或 `--sender-union-id` |

## 当用户说……

| 用户说 | 命令 |
|--------|------|
| "监听有人 @ 我的消息" | event consume，事件码 `user_im_message_receive_at`，参数 `-f ndjson` |
| "监听我和 507971 的单聊消息" | event consume，事件码 `user_im_message_receive_o2o`，参数 `--peer-user-id 507971 -f ndjson` |
| "订阅某个 unionId 的单聊事件" | event consume，事件码 `user_im_message_receive_o2o`，参数 `--peer-union-id <unionId> -f ndjson` |
| "监听 XX 群消息" | 先做群聊搜索获取 `openConversationId`，再 event consume，事件码 `user_im_message_receive_group`，参数 `--open-conversation-id <id> -f ndjson` |
| "监听张三发给我的消息" | 先做 AI 搜问人员搜索获取 `userId`，再 event consume，事件码 `user_im_message_receive_user`，参数 `--sender-user-id <id> -f ndjson` |
| "查看个人消息事件 schema" | event schema + 事件码 |
| "看个人事件订阅状态" | event status + `--event <event_code>` |
| "停止这个个人事件订阅" | event stop + `subscribe_id` |

## 核心规则

- `user` 是默认身份，不要加身份 flag。
- 不要主动运行事件目录列表作为能力菜单；本参考只承认上表 4 个事件码。
- 人名 → AI 搜问人员搜索，多候选必须让用户确认。
- 群名 → 群聊搜索，多候选必须让用户确认。
- 缺少必填 ID 且无法解析时先追问，不要猜测。
- 当前 event 参考只覆盖个人消息事件；其它身份模式不在本参考范围内。

## 常用命令

```bash
dws event schema user_im_message_receive_at
dws event schema user_im_message_receive_o2o
dws event schema user_im_message_receive_group
dws event schema user_im_message_receive_user
```

```bash
dws event consume user_im_message_receive_at \
  -f ndjson
```

```bash
dws event consume user_im_message_receive_o2o \
  --peer-user-id 507971 \
  -f ndjson
```

```bash
dws event consume user_im_message_receive_group \
  --open-conversation-id <openConversationId> \
  -f ndjson
```

```bash
dws event consume user_im_message_receive_user \
  --sender-user-id <userId> \
  -f ndjson
```

```bash
dws event status --event user_im_message_receive_at
dws event status --event user_im_message_receive_o2o
dws event status --event user_im_message_receive_group
dws event status --event user_im_message_receive_user
dws event stop <subscribe_id>
```

## 输出格式

- 推荐 `-f ndjson`：一行一个事件 JSON，适合 Agent 管道读取。
- 人工取样可用 `-f json --max-events 1`。
- `data` 以服务端实际推送样本为准，不要把 CLI schema 的简化结构当作权威 payload 协议。
- `--debug-raw-events` 仅用于服务端联调，正常消费不要使用。

## 完整文档

详细见 multi 模式：`skills/multi/dingtalk-event/SKILL.md`
自测流程：`skills/multi/dingtalk-event/references/runbook.md`
