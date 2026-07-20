---
name: dingtalk-event
description: 钉钉个人 IM 事件长连接监听、订阅与消费，覆盖消息接收、全部单聊/群消息、指定发送人、已读、撤回、表情回应和群生命周期，输出 NDJSON 到 stdout。Use when 用户提到 监听个人消息事件、监听所有单聊或群消息、被@消息、监听单聊或群消息、监听某人发送的消息、监听消息已读、监听消息撤回、监听消息贴表情或表情回应、监听群改名或解散、实时接收钉钉事件、用事件驱动 Agent。命令前缀：dws event。
---

# 钉钉个人 IM 事件

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
| `dws event stop <subscribe_id> --dry-run` / `--yes` | 先预览，再确认取消个人订阅并停止对应本地消费 |
| `dws event stop --all --dry-run` / `--yes` | 先预览，再确认清理当前身份下本地记录的全部个人订阅 |

区分两个 schema：`dws event schema <event_key>` 查事件输出字段；`dws schema "event consume"` 查 consume 命令入参（统一内嵌 ToolSpec，含 parameters + 位置参数）。`source` 是 reviewed command identity 的 provenance；`event list/schema` 是 `interface_mode=local`，`event consume/status/stop` 因同时编排远端订阅控制面与本地 bus 而是 `interface_mode=composite`，不要把 identity 与实现机制混为一谈。

## Event catalog

| 事件码 | 场景 | 必填参数 |
|---|---|---|
| `user_im_message_receive_at` | 当前用户被 @ 的消息 | 无 |
| `user_im_message_receive_o2o` | 当前用户与指定用户的单聊消息 | `--user` 或 `--open-dingtalk-id` |
| `user_im_message_receive_group` | 当前用户所在指定群聊/会话的消息 | `--group` |
| `user_im_message_receive_user` | 当前用户收到的指定用户发送的消息（单聊和群聊） | `--user` 或 `--open-dingtalk-id` |
| `user_im_message_receive_o2o_all` | 当前用户收到的所有单聊消息 | 无 |
| `user_im_message_receive_group_all` | 当前用户收到的所有群聊消息 | 无 |
| `user_im_message_read_o2o` | 指定单聊中当前用户发送的消息被已读 | `--user` 或 `--open-dingtalk-id` |
| `user_im_message_read_group` | 指定群聊中当前用户发送的消息被已读 | `--group` |
| `user_im_message_recall_o2o` | 指定单聊中的消息被撤回 | `--user` 或 `--open-dingtalk-id` |
| `user_im_message_recall_group` | 指定群聊中的消息被撤回 | `--group` |
| `user_im_message_reaction_o2o` | 指定单聊中的消息收到表情回应 | `--user` 或 `--open-dingtalk-id` |
| `user_im_message_reaction_group` | 指定群聊中的消息收到表情回应 | `--group` |
| `user_im_group_updated` | 指定群聊的标题发生变更 | `--group` |
| `user_im_group_disbanded` | 指定群聊被解散 | `--group` |

只承认上表 14 个事件码。其它身份模式、应用凭证模式、非个人 IM 事件不在本 skill 范围内。

## Command rules

- 默认身份就是当前用户，不要额外加身份切换 flag。
- 使用当前用户 OAuth 登录态；未登录或 token 失效时，引导用户执行 `dws auth login`。
- 不主动运行 `dws event list` 作为能力菜单；按用户意图直接选择上表事件。
- 缺少必填 ID 时先解析或追问，不要猜测 ID。
- 用户只给单聊对端人名时，先运行 `dws aisearch person --keyword "<name>" --dimension name --format json` 解析 userId；多候选必须让用户确认。
- 企业内部 userId 使用 `--user`；用户明确提供 openDingtalkId，或目标是外部联系人、机器人、跨组织身份时，使用 `--open-dingtalk-id`。
- `--user` 与 `--open-dingtalk-id` 严格二选一。不要把 openDingtalkId 填入 `--user`，不要自动猜测或转换身份类型；缺少外部目标的 openDingtalkId 时先追问。
- “监听我和某人的单聊”使用 `user_im_message_receive_o2o`；“监听某人发给我的消息/监听某人发送的消息”使用 `user_im_message_receive_user`，后者覆盖该发送人的单聊和群聊消息。
- 只有用户明确要求“所有单聊消息”或“所有群消息”时才使用 `user_im_message_receive_o2o_all` / `user_im_message_receive_group_all`；指定人或指定群仍使用范围更小的事件。
- 用户只给群名时，先运行 `dws chat search --query "<group>" --format json` 解析 openConversationId；多候选必须让用户确认。
- “监听群改名/群标题变更”使用 `user_im_group_updated`；“监听群解散”使用 `user_im_group_disbanded`。群解散自测只能使用明确的测试群，并在执行解散操作前再次提示其不可逆影响。
- 用户要求执行“撤回消息”时使用 `dws chat`；只有“监听/订阅消息撤回”才使用 `dws event consume user_im_message_recall_*`。
- 用户说“贴标签”且语义是给消息贴表情时，按消息表情回应事件处理，event key 使用 `reaction`。
- 正常 Agent 消费使用 `-f ndjson`。抓一条样本可用 `--max-events 1 -f json`。
- 监听非默认组织时带 `--profile <corpId 或 profile 名>`；漏传会退回默认 profile 而失败。
- 自己发的消息不作为事件回来（`isSelfLoop` 过滤）：边监听边 `dws chat message send` 回复不成环；测试投递用别人 / 机器人发（自发会看到 0 事件）。
- `--debug-raw-events` 只用于联调确认服务端推送是否到达本地连接；正常任务不要使用。
- 排查：consume 报 bus 启动失败 → 报错已带真实原因，先查 `dws --profile <x> auth status`（非默认组织带对 `--profile`）；本地日志见 `~/.dws/events/<edition>/personal_stream/<hash>/bus.log`（`hash` 见 `dws event status` 的 Workdir）；有残留先用 `dws event stop --all --dry-run` 预览，确认后加 `--yes` 清理。看着"挂住"无输出多是误加了 `--foreground`（那是跑 bus、不打印事件），去掉即可。

## Call flow

1. 从用户意图选择事件码；人名或群名先解析成必填 ID。
2. 需要了解字段时运行 `dws event schema <event_key>`，读取 `schema.properties`；`jq_root_path` 当前固定为 `.`。
3. 启动 `dws event consume <event_key> ... -f ndjson`，等待 stderr 出现 `[event] ready event_key=<key> bus_pid=<pid> subscribe_id=<id>` 后开始处理 stdout，不要用 `sleep` 猜测。
4. stdout 每行是一个结构化事件 JSON；消息和动作事件直接读取顶层业务字段，群生命周期事件只读取公共字段与 `payload` 中实际存在的字段。
5. 需要确认监听状态时运行 `dws event status --event <event_key>`，查看 `Subscriptions` 和 `Consumers`。
6. 任务完成后优雅结束 consume；本次新建的订阅会自动取消。复用已有订阅或需要从外部主动取消时，先用 `dws event stop <subscribe_id> --dry-run` 预览，向用户确认后再加 `--yes`；临时测试可用 `--max-events` 或 `--duration` 自动退出。

## Subprocess contract

- `event consume` 阻塞式长连接。stdout 只出事件；stderr 只出状态 / debug / 错误。
- 就绪：连上后 stderr 打 `[event] ready event_key=<key> bus_pid=<pid> subscribe_id=<id>`，父进程等这行再读 stdout。不要 `--quiet`（会抑制它）。
- 退出：末行 `[event] exited — received N event(s) in Xs (reason: limit|timeout|signal|bus_shutdown)`；受控退出码 0，失败非 0 无 exited 行。
- stdin 关闭 = 停机：仅当 stdin 是管道且未设 `--max-events/--duration` 时生效；交互终端和 `< /dev/null` 不触发。用管道 stdin 又要常驻就喂 `< <(tail -f /dev/null)`。
- 正常事件处理持续读取 stdout 管道，不要改写为 `--output-dir` watcher。
- 无界监听需外部进程管理；有界自测用 `--max-events N` 或 `--duration 10m`。
- 订阅清理：本次新建的订阅任意干净退出即自动退订；`--subscribe-id` 复用的保留，`--ephemeral` 强制退订。优雅停用 SIGTERM、关 stdin，或外部先用 `dws event stop <subscribe_id> --dry-run` 预览、确认后加 `--yes`。不要 `kill -9`（跳过退订、泄漏服务端订阅）。
- 批量清理先用 `dws event stop --all --dry-run` 预览，确认后加 `--yes`。
- 一 consume 一事件订阅；监听多个对象起多个 consume，本机连接可复用，输出按 `subscribe_id` 隔离。

## Examples

```bash
# 当前用户被 @ 的消息
dws event consume user_im_message_receive_at -f ndjson

# 当前用户与指定用户的单聊消息
dws event consume user_im_message_receive_o2o \
  --user test-user-001 \
  -f ndjson

# 使用 openDingtalkId 监听外部联系人、机器人或跨组织身份的单聊消息
dws event consume user_im_message_receive_o2o \
  --open-dingtalk-id open-user-1 \
  -f ndjson

# 指定群聊/会话消息
dws event consume user_im_message_receive_group \
  --group cidxxxxxxxx \
  -f ndjson

# 指定发送人的消息（单聊和群聊）
dws event consume user_im_message_receive_user \
  --user test-user-001 \
  -f ndjson

# 当前用户收到的所有单聊消息（仅在用户明确要求“所有”时使用）
dws event consume user_im_message_receive_o2o_all -f ndjson

# 当前用户收到的所有群聊消息（仅在用户明确要求“所有”时使用）
dws event consume user_im_message_receive_group_all -f ndjson

# 使用 openDingtalkId 监听指定发送人的消息
dws event consume user_im_message_receive_user \
  --open-dingtalk-id open-user-1 \
  -f ndjson

# 指定单聊消息已读
dws event consume user_im_message_read_o2o \
  --user test-user-001 \
  -f ndjson

# 指定群聊消息撤回
dws event consume user_im_message_recall_group \
  --group cidxxxxxxxx \
  -f ndjson

# 指定单聊消息收到表情回应
dws event consume user_im_message_reaction_o2o \
  --user test-user-001 \
  -f ndjson

# 指定群标题变更
dws event consume user_im_group_updated \
  --group cidxxxxxxxx \
  -f ndjson

# 指定群解散
dws event consume user_im_group_disbanded \
  --group cidxxxxxxxx \
  -f ndjson

# 有界自测
dws event consume user_im_message_receive_at \
  --duration 10m \
  -f ndjson

# 抓一条样本
dws event consume user_im_message_receive_o2o \
  --user test-user-001 \
  --max-events 1 \
  -f json
```

所有 `*_o2o` 命令和 `user_im_message_receive_user` 都可将 `--user <userId>` 替换为 `--open-dingtalk-id <openDingtalkId>`，但两个参数不能同时使用。

## 输出处理

- `dws event schema <event_key>` 是写解析逻辑的依据。
- 顶层 `jq_root_path` 说明业务字段起点；当前值是 `.`。
- `schema.properties` 是业务字段列表，例如 `content`、`sender`、`conversation_id`、`message_id`、`event_time`。
- 消息接收、已读、撤回和表情回应事件直接读取顶层业务字段；不要生成 `fromjson` 或内部 transport 路径。
- 群标题变更和群解散当前只承诺顶层 `type/event_id/timestamp/subscribe_id/payload`。读取 `payload` 时以实际键为准，不猜测群标题、操作者等尚未确认的字段；完整原始协议用 `-f raw` 或 `--debug-raw-events` 排查。
- 群自动回复使用事件顶层 `conversation_id`；单聊自动回复使用顶层 `sender_open_dingtalk_id`。
- 已读事件读取顶层 `reader`、`reader_open_dingtalk_id`、`read_time`；撤回事件读取 `recaller`、`recaller_open_dingtalk_id`、`recall_time`。
- 表情回应事件读取顶层 `operator`、`operator_open_dingtalk_id`、`reaction_name`、`reaction_text`、`operation_type`、`operation_time`。
- 图片、文件等媒体消息的 `content` 可能是可读描述；需要实际媒体文件时调用 `dws chat message download-media`。

## Topic index

| Topic | Reference | Coverage |
|---|---|---|
| IM | [references/event-im.md](references/event-im.md) | 十四类个人 IM 事件命令、参数、生命周期、输出解析、自测和排障 |
