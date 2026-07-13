---
name: dingtalk-event
description: 钉钉个人消息事件长连接监听、订阅与消费，输出 NDJSON 到 stdout。Use when 用户提到 监听个人消息事件、被@消息、监听我和某人的单聊消息、监听某个群消息、实时接收钉钉消息事件、dws event consume user_im_message_receive_at/user_im_message_receive_o2o/user_im_message_receive_group、用事件驱动 Agent 处理钉钉消息、监听并自动回复消息。命令前缀：dws event。
---

# 钉钉个人消息事件

只使用 `dws event consume` 建立个人消息事件长连接。用户要求实时监听、订阅、自动回复或驱动 Agent 时，不要写轮询脚本，不要用消息历史查询模拟事件。

## 运行方式

- bus 后台进程持有对钉钉的个人 Stream 长连；consume 从 bus 读事件、按 NDJSON 打到 stdout。consume 只读，不发消息（回复用 `dws chat message send`）。
- 没有 bus 时 consume 自动拉起；通常只跑 consume。
- 一个组织一个 bus，可同时跑；同组织内多个 consume 共享一个 bus。
- 非默认组织加全局 `--profile <corpId 或 profile 名>`；漏传会退回默认 profile 而失败。

## Core commands

| Command | Purpose |
|---|---|
| `dws event list` | 查看当前个人事件目录；不要把它当能力菜单主动展示 |
| `dws event schema <event_key>` | 查看事件参数和输出字段 schema，默认 JSON |
| `dws event consume <event_key> [flags]` | 阻塞消费；事件写到 stdout，推荐 `-f ndjson` |
| `dws event status --event <event_key>` | 查看个人订阅、personal bus 和本地 consume |
| `dws event stop <subscribe_id>` | 取消个人订阅并停止对应本地消费 |
| `dws event stop --all` | 清理当前身份下本地记录的全部个人订阅 |

区分两个 schema：`dws event schema <event_key>` 查事件输出字段；`dws schema "event consume"` 查 consume 命令入参（机读结构，source=cobra，含 parameters + 位置参数）。

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
- 监听非默认组织时带 `--profile <corpId 或 profile 名>`；漏传会退回默认 profile 而失败。
- 自己发的消息不作为事件回来（`isSelfLoop` 过滤）：边监听边 `dws chat message send` 回复不成环；测试投递用别人 / 机器人发（自发会看到 0 事件）。
- `--debug-raw-events` 只用于联调确认服务端推送是否到达本地连接；正常任务不要使用。
- 排查：consume 报 bus 启动失败 → 报错已带真实原因，先查 `dws --profile <x> auth status`（非默认组织带对 `--profile`）；本地日志见 `~/.dws/events/<edition>/personal_stream/<hash>/bus.log`（`hash` 见 `dws event status` 的 Workdir）；有残留用 `dws event stop --all` 清理。看着"挂住"无输出多是误加了 `--foreground`（那是跑 bus、不打印事件），去掉即可。

## Call flow

1. 从用户意图选择事件码；人名或群名先解析成必填 ID。
2. 需要了解字段时运行 `dws event schema <event_key>`，读取 `jq_root_path` 和 `schema.properties`。
3. 启动 `dws event consume <event_key> ... -f ndjson`，阻塞等 stderr 出现固定就绪行 `[event] ready event_key=<key> ...` 后再开始读 stdout（不要用 sleep 猜）。
4. stdout 每行是一个事件 JSON；业务字段在 `data` JSON 字符串内，按 `jq_root_path` 解析。
5. 需要确认监听状态时运行 `dws event status --event <event_key>`，查看 `Subscriptions` 和 `Consumers`。
6. 任务完成后用 `dws event stop <subscribe_id>` 取消订阅；如果是临时测试，可以在 consume 上加 `--max-events` 或 `--duration` 自动退出。

## Subprocess contract

- `event consume` 阻塞式长连接。stdout 只出事件；stderr 只出状态 / debug / 错误。
- 就绪：连上后 stderr 打 `[event] ready event_key=<key> bus_pid=<pid>`，父进程等这行再读 stdout。不要 `--quiet`（会抑制它）。
- 退出：末行 `[event] exited — received N event(s) in Xs (reason: limit|timeout|signal|bus_shutdown)`；受控退出码 0，失败非 0 无 exited 行。
- stdin 关闭 = 停机：仅当 stdin 是管道且未设 `--max-events/--duration` 时生效；交互终端和 `< /dev/null` 不触发。用管道 stdin 又要常驻就喂 `< <(tail -f /dev/null)`。
- 无界监听需外部进程管理；有界自测用 `--max-events N` 或 `--duration 10m`。
- 订阅清理：本次新建的订阅任意干净退出即自动退订；`--subscribe-id` 复用的保留，`--ephemeral` 强制退订。优雅停用 SIGTERM、关 stdin，或外部 `dws event stop <subscribe_id>`。不要 `kill -9`（跳过退订、泄漏服务端订阅）。
- 批量清理用 `dws event stop --all`。
- 一 consume 一事件订阅；监听多个对象起多个 consume，本机连接可复用，输出按 `subscribe_id` 隔离。

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
