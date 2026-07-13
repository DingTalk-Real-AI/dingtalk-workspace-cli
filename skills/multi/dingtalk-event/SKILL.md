---
name: dingtalk-event
description: 钉钉个人消息事件长连接监听、订阅与消费，输出 NDJSON 到 stdout。Use when 用户提到 监听个人消息事件、被@消息、监听我和某人的单聊消息、监听某个群消息、实时接收钉钉消息事件、dws event consume user_im_message_receive_at/user_im_message_receive_o2o/user_im_message_receive_group、用事件驱动 Agent 处理钉钉消息、监听并自动回复消息。命令前缀：dws event。
---

# 钉钉个人消息事件

只使用 `dws event consume` 建立个人消息事件长连接。用户要求实时监听、订阅、自动回复或驱动 Agent 时，不要写轮询脚本，不要用消息历史查询模拟事件。

## Core commands

| Command | Purpose |
|---|---|
| `dws event list` | 查看当前个人事件目录；不要把它当能力菜单主动展示 |
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

只承认上表 3 个事件码。其它身份模式、应用凭证模式、非个人消息事件不在本 skill 范围内。

## Command rules

- 默认身份就是当前用户，不要额外加身份切换 flag。
- 使用当前用户 OAuth 登录态；未登录或 token 失效时，引导用户执行 `dws auth login`。
- 不主动运行 `dws event list` 作为能力菜单；按用户意图直接选择上表事件。
- 缺少必填 ID 时先解析或追问，不要猜测 ID。
- 用户只给单聊对端人名时，先运行 `dws aisearch person --keyword "<name>" --dimension name --format json` 解析 userId；多候选必须让用户确认。
- 用户只给群名时，先运行 `dws chat search --query "<group>" --format json` 解析 openConversationId；多候选必须让用户确认。
- 正常 Agent 消费使用 `-f ndjson`。抓一条样本可用 `--max-events 1 -f json`。
- `--debug-raw-events` 只用于联调确认服务端推送是否到达本地连接；正常任务不要使用。

## Call flow

1. 从用户意图选择事件码；人名或群名先解析成必填 ID。
2. 需要了解字段时运行 `dws event schema <event_key>`，读取 `jq_root_path` 和 `schema.properties`。
3. 启动 `dws event consume <event_key> ... -f ndjson`，等待 stderr 出现 `connected bus pid=...` 后开始读 stdout。
4. stdout 每行是一个事件 JSON；业务字段在 `data` JSON 字符串内，按 `jq_root_path` 解析。
5. 需要确认监听状态时运行 `dws event status --event <event_key>`，查看 `Subscriptions` 和 `Consumers`。
6. 任务完成后用 `dws event stop <subscribe_id>` 取消订阅；如果是临时测试，可以在 consume 上加 `--max-events` 或 `--duration` 自动退出。

## Subprocess contract

- `event consume` 是阻塞式长连接命令。stdout 只处理事件；stderr 只处理状态、debug 和错误。
- 不要使用 `--quiet`，否则 Agent 会看不到建联状态和排障信息。
- 无界监听需要外部进程管理；有界自测优先用 `--max-events N` 或 `--duration 10m`。
- 不要 `kill -9` 消费进程。Ctrl+C、duration、max-events 只结束本地前台进程；取消订阅必须使用 `dws event stop <subscribe_id>`。
- 不要运行裸 `dws event stop`；批量清理必须显式使用 `dws event stop --all`。
- 一个 consume 对应一个事件订阅。监听多个对象时启动多个 consume；底层本机连接可以复用，但输出按 `subscribe_id` 隔离。

## Examples

```bash
# 当前用户被 @ 的消息
dws event consume user_im_message_receive_at -f ndjson

# 当前用户与指定用户的单聊消息
dws event consume user_im_message_receive_o2o \
  --user 507971 \
  -f ndjson

# 指定群聊/会话消息
dws event consume user_im_message_receive_group \
  --group cidxxxxxxxx \
  -f ndjson

# 有界自测
dws event consume user_im_message_receive_at \
  --duration 10m \
  -f ndjson

# 抓一条样本
dws event consume user_im_message_receive_o2o \
  --user 507971 \
  --max-events 1 \
  -f json
```

## 输出处理

- `dws event schema <event_key>` 是写解析逻辑的依据。
- 顶层 `jq_root_path` 说明业务字段起点；当前值是 `.data | fromjson`。
- `schema.properties` 是业务字段列表，例如 `content`、`sender`、`conversation_id`、`message_id`、`event_time`。
- 不要假设 `data` 已经展开。读取消息正文时先解析 `data`，再读 `payload.body.content` 或 schema 中对应字段。

## Topic index

| Topic | Reference | Coverage |
|---|---|---|
| IM | [references/event-im.md](references/event-im.md) | 三类个人消息事件命令、参数、生命周期、输出解析、自测、过滤和排障 |
