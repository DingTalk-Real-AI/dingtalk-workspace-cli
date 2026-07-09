# dws event — 个人消息事件

通过个人 Stream 长连接监听当前用户收到的钉钉消息事件，NDJSON 输出到 stdout，用于驱动事件触发的 Agent。实时监听、自动回复、订阅事件都必须使用 `dws event consume`，不要写脚本轮询消息历史。

## Core commands

| Command | Purpose |
|---|---|
| `dws event schema <event_key>` | 查看事件参数和输出字段 schema，默认 JSON |
| `dws event consume <event_key> [flags]` | 阻塞消费；事件写到 stdout，推荐 `-f ndjson` |
| `dws event status --event <event_key>` | 查看个人订阅、personal bus 和本地 consume |
| `dws event stop <subscribe_id>` | 取消个人订阅并停止对应本地消费 |
| `dws event stop --all` | 清理当前身份下本地记录的全部个人订阅 |

## Event catalog

| 事件码 | 场景 | 必填参数 |
|---|---|---|
| `user_im_message_receive_at` | 当前用户被 @ 的消息 | 无 |
| `user_im_message_receive_o2o` | 当前用户与指定用户的单聊消息 | `--user` |
| `user_im_message_receive_group` | 当前用户所在指定群聊/会话的消息 | `--group` |

只承认上表 3 个事件码。默认身份就是当前用户，不要额外加身份切换 flag。

## Intent mapping

| 用户说 | 下一步 |
|---|---|
| "监听有人 @ 我的消息" | `event consume`，事件码 `user_im_message_receive_at`，参数 `-f ndjson` |
| "监听我和 userId 507971 的单聊消息" | `event consume`，事件码 `user_im_message_receive_o2o`，参数 `--user 507971 -f ndjson` |
| "监听 XX 群消息" | 先 `dws chat search --query "XX" --format json`，确认后 consume group |
| "监听并自动回复某人的单聊消息" | 先解析对端 userId，再启动 o2o consume；不要写轮询脚本 |
| "查看个人消息事件 schema" | `dws event schema <event_key>` |
| "看个人事件订阅状态" | `dws event status --event <event_key>` |
| "停止这个个人事件订阅" | `dws event stop <subscribe_id>` |

多候选必须让用户确认。缺少必填 ID 且无法解析时先追问，不要猜测。

## Call flow

1. 从用户意图选择事件码；人名或群名先解析成必填 ID。
2. 需要了解字段时运行 `dws event schema <event_key>`，读取 `jq_root_path` 和 `schema.properties`。
3. 启动 `dws event consume <event_key> ... -f ndjson`，等待 stderr 出现 `connected bus pid=...` 后开始读 stdout。
4. stdout 每行是一个事件 JSON；业务字段在 `data` JSON 字符串内，按 `jq_root_path` 解析。
5. 需要确认监听状态时运行 `dws event status --event <event_key>`，查看 `Subscriptions` 和 `Consumers`。
6. 任务完成后用 `dws event stop <subscribe_id>` 取消订阅；临时测试可以在 consume 上加 `--max-events` 或 `--duration`。

## Common commands

```bash
dws event schema user_im_message_receive_at
dws event schema user_im_message_receive_o2o
dws event schema user_im_message_receive_group
```

```bash
dws event consume user_im_message_receive_at -f ndjson
```

```bash
dws event consume user_im_message_receive_o2o \
  --user 507971 \
  -f ndjson
```

```bash
dws event consume user_im_message_receive_group \
  --group <openConversationId> \
  -f ndjson
```

```bash
dws event status --event user_im_message_receive_at
dws event status --event user_im_message_receive_o2o
dws event status --event user_im_message_receive_group
dws event stop <subscribe_id>
```

`status` 的 `Consumers` 表展示本地 consume 的 PID、事件码、`subscribe_id` 和 received/dropped 计数，可用于确认监听是否仍在 personal bus 上。

裸 `dws event stop` 不会取消订阅；批量清理必须显式使用 `dws event stop --all`。Ctrl+C、`--duration`、`--max-events` 只结束本地前台消费进程，不等价于取消服务端订阅。

## Output parsing

- 推荐 `-f ndjson`：一行一个事件 JSON，适合 Agent 管道读取。
- 人工取样可用 `-f json --max-events 1`。
- `data` 是服务端业务 payload 的 JSON 字符串；读取消息内容前按 schema 的 `jq_root_path` 再解析一次。
- 当前消息正文在 `payload.body.content`，发送人展示名在 `payload.body.sender`，会话 ID 在 `payload.body.openConversationId`。
- `--debug-raw-events` 仅用于服务端联调，正常消费不要使用。

## Full reference

- multi skill: `skills/multi/dingtalk-event/SKILL.md`
- IM reference: `skills/multi/dingtalk-event/references/event-im.md`
