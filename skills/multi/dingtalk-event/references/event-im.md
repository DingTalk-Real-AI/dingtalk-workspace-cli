# IM 个人事件

先读上层 [SKILL.md](../SKILL.md) 的命令规则、调用流和子进程契约。本参考覆盖当前公开的 IM 个人事件：消息接收、已读、撤回和表情回应。

实时监听、自动回复、订阅事件都必须使用 `dws event consume` 长连接，不要写轮询脚本。

## Prerequisite

个人事件使用当前用户 OAuth 登录态。未登录或 token 失效时，先执行：

```bash
dws auth login
```

查看事件 schema：

```bash
dws event schema user_im_message_receive_at --flatten
dws event schema user_im_message_receive_o2o --flatten
dws event schema user_im_message_receive_group --flatten
dws event schema user_im_message_receive_user --flatten
dws event schema user_im_message_read_o2o --flatten
dws event schema user_im_message_read_group --flatten
dws event schema user_im_message_recall_o2o --flatten
dws event schema user_im_message_recall_group --flatten
dws event schema user_im_message_reaction_o2o --flatten
dws event schema user_im_message_reaction_group --flatten
```

schema 默认 JSON。Agent 使用 `--flatten` schema，业务字段在 `schema.properties`，`jq_root_path` 为 `.`。不传 `--flatten` 时查看兼容 transport envelope，其 `jq_root_path` 为 `.data | fromjson`。

## Event catalog

| 事件码 | 规则 | 用途 | 必填参数 |
|---|---|---|---|
| `user_im_message_receive_at` | `at` | 当前用户被 @ 的消息 | 无 |
| `user_im_message_receive_o2o` | `singleChat` | 当前用户与指定用户的单聊消息 | `--user` 或 `--open-dingtalk-id` |
| `user_im_message_receive_group` | `group` | 当前用户所在指定群聊/会话的消息 | `--group` |
| `user_im_message_receive_user` | `sender` | 当前用户收到的指定用户发送的消息（单聊和群聊） | `--user` 或 `--open-dingtalk-id` |
| `user_im_message_read_o2o` | `singleChat` | 指定单聊中当前用户发送的消息被已读 | `--user` 或 `--open-dingtalk-id` |
| `user_im_message_read_group` | `group` | 指定群聊中当前用户发送的消息被已读 | `--group` |
| `user_im_message_recall_o2o` | `singleChat` | 指定单聊中的消息被撤回 | `--user` 或 `--open-dingtalk-id` |
| `user_im_message_recall_group` | `group` | 指定群聊中的消息被撤回 | `--group` |
| `user_im_message_reaction_o2o` | `singleChat` | 指定单聊中的消息收到表情回应 | `--user` 或 `--open-dingtalk-id` |
| `user_im_message_reaction_group` | `group` | 指定群聊中的消息收到表情回应 | `--group` |

默认身份就是当前用户。不要额外加身份切换 flag，不要使用应用凭证模式，不要使用本表以外的事件码。

## ID resolution

- 人名 → `dws aisearch person --keyword "<name>" --dimension name --format json`，确认后取 `userId`。
- 企业内部 userId → `--user`；明确给出 openDingtalkId，或目标是外部联系人、机器人、跨组织身份 → `--open-dingtalk-id`。
- 两个身份参数严格二选一，不得把 openDingtalkId 放进 `--user`，也不要自动猜测或转换；缺少外部目标的 openDingtalkId 时先追问。
- “我和某人的单聊”选择 `user_im_message_receive_o2o`；“某人发给我的消息/某人发送的消息”选择 `user_im_message_receive_user`。
- 群名 → `dws chat search --query "<group>" --format json`，确认后取 `openConversationId`。
- 多候选 → 展示候选并让用户确认。
- 仍缺必填 ID → 先追问，不要编造。
- “撤回消息”表示执行操作时走 `dws chat`；“监听/订阅消息撤回”才走本事件能力。
- “贴标签”表示给消息贴表情时，对应 `reaction` 表情回应事件。

## Consume commands

```bash
# 被 @ 消息
dws event consume user_im_message_receive_at --flatten -f ndjson

# 指定单聊消息
dws event consume user_im_message_receive_o2o \
  --user test-user-001 \
  --flatten \
  -f ndjson

# 通过 openDingtalkId 指定单聊对端
dws event consume user_im_message_receive_o2o \
  --open-dingtalk-id open-user-1 \
  --flatten \
  -f ndjson

# 指定群消息
dws event consume user_im_message_receive_group \
  --group cidxxxxxxxx \
  --flatten \
  -f ndjson

# 指定发送人的消息（单聊和群聊）
dws event consume user_im_message_receive_user \
  --user test-user-001 \
  --flatten \
  -f ndjson

# 通过 openDingtalkId 指定发送人
dws event consume user_im_message_receive_user \
  --open-dingtalk-id open-user-1 \
  --flatten \
  -f ndjson

# 指定单聊已读事件
dws event consume user_im_message_read_o2o \
  --user test-user-001 \
  --flatten \
  -f ndjson

# 指定群聊已读事件
dws event consume user_im_message_read_group \
  --group cidxxxxxxxx \
  --flatten \
  -f ndjson

# 指定单聊撤回事件
dws event consume user_im_message_recall_o2o \
  --user test-user-001 \
  --flatten \
  -f ndjson

# 指定群聊撤回事件
dws event consume user_im_message_recall_group \
  --group cidxxxxxxxx \
  --flatten \
  -f ndjson

# 指定单聊表情回应事件
dws event consume user_im_message_reaction_o2o \
  --user test-user-001 \
  --flatten \
  -f ndjson

# 指定群聊表情回应事件
dws event consume user_im_message_reaction_group \
  --group cidxxxxxxxx \
  --flatten \
  -f ndjson
```

## Self-test triggers

| 事件码 | 自测参数 | 触发方式 |
|---|---|---|
| `user_im_message_receive_at` | `--flatten --duration 10m -f ndjson` | 让任意可触达用户在群里 @ 当前登录用户 |
| `user_im_message_receive_o2o` | `--user <userId>` 或 `--open-dingtalk-id <id>`，加 `--flatten --duration 10m -f ndjson` | 让对端用户给当前登录用户发送单聊消息 |
| `user_im_message_receive_group` | `--group <openConversationId> --flatten --duration 10m -f ndjson` | 让任意用户在该群发送消息 |
| `user_im_message_receive_user` | `--user <userId>` 或 `--open-dingtalk-id <id>`，加 `--flatten --duration 10m -f ndjson` | 让指定用户分别在单聊或共同群聊中发送消息 |
| `user_im_message_read_o2o` | `--user <userId>` 或 `--open-dingtalk-id <id>`，加 `--flatten --duration 10m -f ndjson` | 当前用户给对端发送单聊消息，再让对端打开并阅读 |
| `user_im_message_read_group` | `--group <openConversationId> --flatten --duration 10m -f ndjson` | 当前用户在群内发送消息，再让群成员打开并阅读 |
| `user_im_message_recall_o2o` | `--user <userId>` 或 `--open-dingtalk-id <id>`，加 `--flatten --duration 10m -f ndjson` | 在指定单聊中发送并撤回一条消息 |
| `user_im_message_recall_group` | `--group <openConversationId> --flatten --duration 10m -f ndjson` | 在指定群聊中发送并撤回一条消息 |
| `user_im_message_reaction_o2o` | `--user <userId>` 或 `--open-dingtalk-id <id>`，加 `--flatten --duration 10m -f ndjson` | 在指定单聊中给消息添加表情回应 |
| `user_im_message_reaction_group` | `--group <openConversationId> --flatten --duration 10m -f ndjson` | 在指定群聊中给消息添加表情回应 |

stderr 出现固定就绪行 `[event] ready event_key=<key> bus_pid=<pid> subscribe_id=<id>` 表示本地 consume 已连接到事件 bus；父进程等这行再读 stdout。stdout 每行是一个扁平事件 JSON。

## Runtime flags

| 参数 | 用途 |
|---|---|
| `--flatten` | 将 `ndjson/json/pretty` 的默认 transport envelope（或原 compact processor）投影为 Agent 可直接读取的顶层业务字段；不能与 `-f raw` 或 `--debug-raw-events` 同时使用 |
| `-f ndjson` | 控制序列化为一行一个 JSON；不改变数据结构 |
| `-f json` | 人工查看单条或少量样本；必须配合 `--max-events` 或 `--duration` |
| `--max-events <n>` | 收到 N 条后退出 |
| `--duration <duration>` | 到时退出，例如 `30s`、`10m` |
| `--output-dir <dir>` | 每个事件写入一个文件 |
| `--route '<regex>=dir:<path>'` | 按事件类型路由到目录 |
| `--subscribe-id <id>` | 复用已有个人订阅 |
| `--ephemeral` | 即使复用已有订阅，也在 consume 退出时取消订阅 |
| `--query <csv>` | 按消息正文关键词过滤，逗号分隔 |
| `--filter-json <json>` | 使用个人事件 Filter DSL 过滤 |
| `--debug-raw-events` | 联调用：绕过本地过滤，输出当前 personal stream 实际收到的可解析事件 |

正常 Agent 消费不要使用 `--debug-raw-events`。它会输出当前连接收到的所有可解析事件，只用于判断服务端是否推到了本机连接，并且不能与 `--flatten` 同时使用。

## Output parsing

Agent 使用 `--flatten -f ndjson`，stdout 每行是一个扁平业务事件对象。消息接收事件常见顶层字段：

| 字段 | 说明 |
|---|---|
| `type` | 个人事件码 |
| `event_id` | 事件 ID，可用于去重 |
| `timestamp` | 事件发生时间戳 |
| `subscribe_id` | 个人订阅 ID，也是本地输出隔离键 |
| `content` | 消息正文 |
| `sender` | 发送人展示名 |
| `conversation_id` | 开放会话 ID |
| `message_id` | 开放消息 ID |
| `sender_open_dingtalk_id` | 发送人的开放钉钉 ID |
| `create_time` | 消息创建时间 |
| `event_time` | 消息事件时间戳 |

在 `--flatten` 模式下直接按顶层字段解析，不要再使用 `fromjson` 或内部 payload 路径。不传 `--flatten` 时保持兼容 transport envelope，字段为 `type/event_type/data/headers`，业务 payload 需从 `.data | fromjson` 读取。图片、文件等媒体消息的 `content` 可能是可读描述；需要实际媒体文件时调用 `dws chat message download-media`。

所有动作事件都包含顶层 `type`、`event_id`、`timestamp`、`subscribe_id`、`message_id`、`conversation_id`、`sender`、`sender_open_dingtalk_id` 和 `event_time`。各类动作的专有字段如下：

| 事件类型 | 顶层业务字段 |
|---|---|
| 已读 | `reader`、`reader_open_dingtalk_id`、`read_time` |
| 撤回 | `recaller`、`recaller_open_dingtalk_id`、`recall_time` |
| 表情回应 | `operator`、`operator_open_dingtalk_id`、`reaction_name`、`reaction_text`、`operation_type`、`operation_time` |

正常输出不会暴露 `payload`、`uid`、`corpid`、`clientId`、`filterSubId`、`bizid` 等内部字段。需要检查原始协议时才使用 `-f raw` 或 `--debug-raw-events`。

## Event-driven replies

- 群消息自动回复：读取顶层 `conversation_id`，再调用 `dws chat message send --group "<conversation_id>" --text "<reply>" --format json`。
- 单聊自动回复：读取顶层 `sender_open_dingtalk_id`，再调用 `dws chat message send --open-dingtalk-id "<sender_open_dingtalk_id>" --text "<reply>" --format json`。
- 正常处理持续读取 consume 的 stdout 管道。不要用 `sleep` 猜测建联，不要改写成 `--output-dir` watcher。

## Filtering

优先用订阅规则参数缩小服务端推送范围：

- 单聊用 `--user`。
- 群消息用 `--group`。

收消息事件需要额外文本过滤时再用 `--query` 或 `--filter-json`：

```bash
dws event consume user_im_message_receive_group \
  --group cidxxxxxxxx \
  --query "报警,故障" \
  --flatten \
  -f ndjson
```

`--filter-json` 使用 `content`、`sender`、`conversation_id`、`sender_open_dingtalk_id` 等业务别名表达意图。

动作事件只使用 `--user` 或 `--group` 限定订阅范围。`--query` 和消息内容 `--filter-json` 面向接收消息事件，不用于已读、撤回或表情回应事件。

## Status and stop

```bash
dws event status --event user_im_message_receive_at
dws event status --event user_im_message_receive_o2o
dws event status --event user_im_message_receive_group
dws event status --event user_im_message_receive_user
dws event status --event user_im_message_read_o2o
dws event status --event user_im_message_recall_group
dws event status --event user_im_message_reaction_o2o
```

`status` 同时展示服务端 `Subscriptions` 和本地 `Consumers`。`Consumers` 表里的 PID、事件码、`subscribe_id`、received/dropped 计数用于确认当前前台 consume 是否还挂在 personal bus 上。

停止指定订阅：

```bash
dws event stop <subscribe_id> --dry-run
dws event stop <subscribe_id> --yes
```

清理当前身份下本地记录的全部个人订阅：

```bash
dws event stop --all --dry-run
dws event stop --all --yes
```

裸 `dws event stop` 不会取消订阅。本次 consume 新建的订阅会在 SIGTERM、Ctrl+C、stdin EOF、duration 或 max-events 等干净退出时自动取消；通过 `--subscribe-id` 复用的订阅默认保留。需要从外部取消时，使用事件输出或 `status` 里的 `subscribe_id`，先执行 `dws event stop <subscribe_id> --dry-run`，确认预览后再加 `--yes`。不要使用 `kill -9`，它会跳过清理。

## Troubleshooting

- 没有输出：先确认 stderr 已出现 `[event] ready event_key=... bus_pid=... subscribe_id=...`。
- 参数缺失：所有 o2o 事件必须有对端 ID，所有 group 事件必须有 openConversationId。
- 收到非预期消息：检查 stdout 的 `subscribe_id` 是否等于当前命令创建/复用的订阅 ID。
- 需要判断服务端是否推到当前连接：临时加 `--debug --debug-raw-events`，排查后去掉。
- 需要长期运行：交给外部进程管理；不要把消息历史查询写成轮询脚本。
- 安装或更新 skill 后，已打开的 Agent 旧会话需要新开会话或重新加载 skills。
