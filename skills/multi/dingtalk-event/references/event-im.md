# IM 个人消息事件

先读上层 [SKILL.md](../SKILL.md) 的命令规则、调用流和子进程契约。本参考覆盖当前公开的 IM 个人消息事件：被 @、指定单聊、指定群聊。

实时监听、自动回复、订阅事件都必须使用 `dws event consume` 长连接，不要写轮询脚本。

## Prerequisite

个人事件使用当前用户 OAuth 登录态。未登录或 token 失效时，先执行：

```bash
dws auth login
```

查看事件 schema：

```bash
dws event schema user_im_message_receive_at
dws event schema user_im_message_receive_o2o
dws event schema user_im_message_receive_group
```

schema 默认 JSON。业务字段说明在 `schema.properties`，当前业务 payload 解析起点看 `jq_root_path`。

## Event catalog

| 事件码 | 规则 | 用途 | 必填参数 |
|---|---|---|---|
| `user_im_message_receive_at` | `at` | 当前用户被 @ 的消息 | 无 |
| `user_im_message_receive_o2o` | `singleChat` | 当前用户与指定用户的单聊消息 | `--user` |
| `user_im_message_receive_group` | `group` | 当前用户所在指定群聊/会话的消息 | `--group` |

默认身份就是当前用户。不要额外加身份切换 flag，不要使用应用凭证模式，不要使用本表以外的事件码。

## ID resolution

- 人名 → `dws aisearch person --keyword "<name>" --dimension name --format json`，确认后取 `userId`。
- 群名 → `dws chat search --query "<group>" --format json`，确认后取 `openConversationId`。
- 多候选 → 展示候选并让用户确认。
- 仍缺必填 ID → 先追问，不要编造。

## Consume commands

```bash
# 被 @ 消息
dws event consume user_im_message_receive_at -f ndjson

# 指定单聊消息
dws event consume user_im_message_receive_o2o \
  --user 507971 \
  -f ndjson

# 指定群消息
dws event consume user_im_message_receive_group \
  --group cidxxxxxxxx \
  -f ndjson
```

## Self-test triggers

| 事件码 | 自测参数 | 触发方式 |
|---|---|---|
| `user_im_message_receive_at` | `--duration 10m -f ndjson` | 让任意可触达用户在群里 @ 当前登录用户 |
| `user_im_message_receive_o2o` | `--user <userId> --duration 10m -f ndjson` | 让对端用户给当前登录用户发送单聊消息 |
| `user_im_message_receive_group` | `--group <openConversationId> --duration 10m -f ndjson` | 让任意用户在该群发送消息 |

stderr 出现固定就绪行 `[event] ready event_key=<key> bus_pid=<pid>` 表示本地 consume 已连接到事件 bus；父进程等这行再读 stdout。stdout 每行是一个事件 JSON。

## Runtime flags

| 参数 | 用途 |
|---|---|
| `-f ndjson` | 推荐输出，一行一个事件 JSON |
| `-f json` | 人工查看单条或少量样本；必须配合 `--max-events` 或 `--duration` |
| `--max-events <n>` | 收到 N 条后退出 |
| `--duration <duration>` | 到时退出，例如 `30s`、`10m` |
| `--output-dir <dir>` | 每个事件写入一个文件 |
| `--route '<regex>=dir:<path>'` | 按事件类型路由到目录 |
| `--subscribe-id <id>` | 复用已有个人订阅 |
| `--query <csv>` | 按消息正文关键词过滤，逗号分隔 |
| `--filter-json <json>` | 使用个人事件 Filter DSL 过滤 |
| `--debug-raw-events` | 联调用：绕过本地过滤，输出当前 personal stream 实际收到的可解析事件 |

正常 Agent 消费不要使用 `--debug-raw-events`。它会输出当前连接收到的所有可解析事件，只用于判断服务端是否推到了本机连接。

## Output parsing

`-f ndjson` 的 stdout 每行是一个外层事件对象，常见顶层字段：

| 字段 | 说明 |
|---|---|
| `event_type` | 个人事件码 |
| `event_id` | 本地输出事件 ID |
| `subscribe_id` | 个人订阅 ID，也是本地输出隔离键 |
| `source_id` | 当前 sourceId |
| `data` | 服务端业务 payload 的 JSON 字符串 |
| `received_at_unix_ms` | 本地接收时间 |

读取业务字段前先运行 `dws event schema <event_key>`。当前 schema 顶层的 `jq_root_path` 是 `.data | fromjson`，表示先把 `data` 再解析一次，然后读取业务字段。

解析后的常用业务字段：

| schema 字段 | 当前真实路径 | 说明 |
|---|---|---|
| `content` | `payload.body.content` | 消息正文 |
| `sender` | `payload.body.sender` | 发送人展示名 |
| `conversation_id` | `payload.body.openConversationId` | 开放会话 ID |
| `message_id` | `payload.body.openMessageId` | 开放消息 ID |
| `sender_open_dingtalk_id` | `payload.body.senderOpenDingTalkId` | 发送人的开放钉钉 ID |
| `create_time` | `payload.body.createTime` | 消息创建时间 |
| `event_time` | `payload.event_time` | 消息事件时间戳 |

## Filtering

优先用订阅规则参数缩小服务端推送范围：

- 单聊用 `--user`。
- 群消息用 `--group`。

额外文本过滤再用 `--query` 或 `--filter-json`：

```bash
dws event consume user_im_message_receive_group \
  --group cidxxxxxxxx \
  --query "报警,故障" \
  -f ndjson
```

`--filter-json` 可以使用业务别名，也可以使用真实路径。优先使用 schema 字段名表达意图；需要和服务端联调时再使用真实路径。

## Status and stop

```bash
dws event status --event user_im_message_receive_at
dws event status --event user_im_message_receive_o2o
dws event status --event user_im_message_receive_group
```

`status` 同时展示服务端 `Subscriptions` 和本地 `Consumers`。`Consumers` 表里的 PID、事件码、`subscribe_id`、received/dropped 计数用于确认当前前台 consume 是否还挂在 personal bus 上。

停止指定订阅：

```bash
dws event stop <subscribe_id>
```

清理当前身份下本地记录的全部个人订阅：

```bash
dws event stop --all
```

裸 `dws event stop` 不会取消订阅。前台 `consume` 进程用 Ctrl+C 只会停止本地进程；需要取消服务端订阅时，使用事件输出或 `status` 里的 `subscribe_id` 执行 `dws event stop <subscribe_id>`。

## Troubleshooting

- 没有输出：先确认 stderr 已出现 `[event] ready event_key=...`。
- 参数缺失：单聊必须有对端 ID，群消息必须有 openConversationId。
- 收到非预期消息：检查 stdout 的 `subscribe_id` 是否等于当前命令创建/复用的订阅 ID。
- 需要判断服务端是否推到当前连接：临时加 `--debug --debug-raw-events`，排查后去掉。
- 需要长期运行：交给外部进程管理；不要把消息历史查询写成轮询脚本。
- 安装或更新 skill 后，已打开的 Agent 旧会话需要新开会话或重新加载 skills。
