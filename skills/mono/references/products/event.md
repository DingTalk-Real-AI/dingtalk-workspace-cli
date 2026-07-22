# dws event — 个人 IM 事件

通过个人 Stream 长连接监听当前用户的钉钉消息接收、已读、撤回和表情回应事件，NDJSON 输出到 stdout，用于驱动事件触发的 Agent。实时监听、自动回复、订阅事件都必须使用 `dws event consume`，不要写脚本轮询消息历史。

## 运行方式

- bus 后台进程持有对钉钉的个人 Stream 长连；consume 从 bus 读事件、按 NDJSON 打到 stdout。consume 只读，不发消息（回复用 `dws chat message send`）。
- 没有 bus 时 consume 自动拉起；通常只跑 consume。
- 一个组织一个 bus，互不干扰、可同时跑；同组织内多个 consume 共享一个 bus。
- 非默认组织加全局 `--profile <corpId 或 profile 名>`；漏传会退回默认 profile 而失败。

## Core commands

| Command | Purpose |
|---|---|
| `dws event schema <event_key> --flatten` | 查看 Agent 使用的顶层业务字段 schema |
| `dws event consume <event_key> --flatten [flags]` | 阻塞消费，事件写到 stdout，用 `-f ndjson` |
| `dws event status --event <event_key>` | 查看个人订阅、bus、本地 consume |
| `dws event stop <subscribe_id> --dry-run` / `--yes` | 先预览，再确认取消订阅并停止对应本地消费 |
| `dws event stop --all --dry-run` / `--yes` | 先预览，再确认清理当前身份下全部个人订阅 |

注意区分两个 schema：`dws event schema <event_key>` 查事件的输出字段；`dws schema "event consume"` 查 consume 命令自身的入参（统一内嵌 ToolSpec，含 parameters + 位置参数）。`source` 是 reviewed command identity 的 provenance；`event list/schema` 是 `interface_mode=local`，`event consume/status/stop` 因同时编排远端订阅控制面与本地 bus 而是 `interface_mode=composite`，不要把 identity 与实现机制混为一谈。

## Event catalog

| 事件码 | 场景 | 必填参数 |
|---|---|---|
| `user_im_message_receive_at` | 当前用户被 @ 的消息 | 无 |
| `user_im_message_receive_o2o` | 当前用户与指定用户的单聊消息 | `--user` 或 `--open-dingtalk-id` |
| `user_im_message_receive_group` | 当前用户所在指定群聊/会话的消息 | `--group` |
| `user_im_message_receive_user` | 当前用户收到的指定用户发送的消息（单聊和群聊） | `--user` 或 `--open-dingtalk-id` |
| `user_im_message_read_o2o` | 指定单聊中当前用户发送的消息被已读 | `--user` 或 `--open-dingtalk-id` |
| `user_im_message_read_group` | 指定群聊中当前用户发送的消息被已读 | `--group` |
| `user_im_message_recall_o2o` | 指定单聊中的消息被撤回 | `--user` 或 `--open-dingtalk-id` |
| `user_im_message_recall_group` | 指定群聊中的消息被撤回 | `--group` |
| `user_im_message_reaction_o2o` | 指定单聊中的消息收到表情回应 | `--user` 或 `--open-dingtalk-id` |
| `user_im_message_reaction_group` | 指定群聊中的消息收到表情回应 | `--group` |

只承认上表 10 个事件码。默认身份就是当前用户，不要额外加身份切换 flag。

## Intent mapping

| 用户说 | 下一步 |
|---|---|
| "监听有人 @ 我的消息" | `event consume`，事件码 `user_im_message_receive_at`，参数 `--flatten -f ndjson` |
| "监听我和 userId test-user-001 的单聊消息" | `event consume`，事件码 `user_im_message_receive_o2o`，参数 `--user test-user-001 --flatten -f ndjson` |
| "监听我和 openDingtalkId abc 的单聊消息" | `event consume`，事件码 `user_im_message_receive_o2o`，参数 `--open-dingtalk-id abc --flatten -f ndjson` |
| "监听 XX 群消息" | 先 `dws chat search --query "XX" --format json`，确认后 consume group |
| "监听 userId test-user-001 发给我的消息" | `event consume`，事件码 `user_im_message_receive_user`，参数 `--user test-user-001 --flatten -f ndjson` |
| "监听 openDingtalkId abc 发给我的消息" | `event consume`，事件码 `user_im_message_receive_user`，参数 `--open-dingtalk-id abc --flatten -f ndjson` |
| "监听我发给 userId test-user-001 的消息是否已读" | `event consume`，事件码 `user_im_message_read_o2o`，参数 `--user test-user-001 --flatten -f ndjson` |
| "监听 XX 群消息已读" | 先解析群 ID，再 consume `user_im_message_read_group --group <id>` |
| "监听我和 userId test-user-001 的消息撤回" | `event consume`，事件码 `user_im_message_recall_o2o`，参数 `--user test-user-001 --flatten -f ndjson` |
| "监听 XX 群消息撤回" | 先解析群 ID，再 consume `user_im_message_recall_group --group <id>` |
| "监听我和 userId test-user-001 的消息贴表情" | `event consume`，事件码 `user_im_message_reaction_o2o`，参数 `--user test-user-001 --flatten -f ndjson` |
| "监听 XX 群消息表情回应" | 先解析群 ID，再 consume `user_im_message_reaction_group --group <id>` |
| "监听并自动回复某人的单聊消息" | 先解析对端 userId，再启动 o2o consume；不要写轮询脚本 |
| "查看个人消息事件 schema" | `dws event schema <event_key> --flatten` |
| "看个人事件订阅状态" | `dws event status --event <event_key>` |
| "停止这个个人事件订阅" | `dws event stop <subscribe_id> --dry-run`，确认后改用 `--yes` |

多候选让用户确认。缺必填 ID 且解析不出先追问，不要猜。企业内部 userId 使用 `--user`；明确给出 openDingtalkId，或目标是外部联系人、机器人、跨组织身份时使用 `--open-dingtalk-id`。两者严格二选一，不得混填、猜测或自动转换身份类型。

“我和某人的单聊”使用 `receive_o2o`；“某人发给我的消息/某人发送的消息”使用 `receive_user`，后者覆盖该发送人的单聊和群聊消息。用户要求执行“撤回消息”时走 `dws chat`；只有“监听/订阅消息撤回”才走 `dws event`。“贴标签”表示给消息贴表情时，对应 `reaction` 表情回应事件。

## Call flow

1. 从用户意图选择事件码；人名或群名先解析成必填 ID。
2. 需要了解字段时运行 `dws event schema <event_key> --flatten`，读取 `schema.properties`；此模式的 `jq_root_path` 为 `.`。
3. 启动 `dws event consume <event_key> ... --flatten -f ndjson`，等待 stderr 出现 `[event] ready event_key=<key> bus_pid=<pid> subscribe_id=<id>` 后开始处理 stdout，不要用 `sleep` 猜测。
4. stdout 每行是一个扁平事件 JSON；直接按该事件的 `schema.properties` 读取顶层字段。
5. 需要确认监听状态时运行 `dws event status --event <event_key>`，查看 `Subscriptions` 和 `Consumers`。
6. 任务完成后优雅结束 consume；本次新建的订阅会自动取消。复用已有订阅或需要从外部主动取消时，先运行 `dws event stop <subscribe_id> --dry-run`，向用户确认后再以 `--yes` 执行；自测可在 consume 加 `--max-events` 或 `--duration` 自动退出。

## Commands

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

```bash
dws event consume user_im_message_receive_at --flatten -f ndjson
dws event consume user_im_message_receive_o2o --user test-user-001 --flatten -f ndjson
dws event consume user_im_message_receive_o2o --open-dingtalk-id abc --flatten -f ndjson
dws event consume user_im_message_receive_group --group <openConversationId> --flatten -f ndjson
dws event consume user_im_message_receive_user --user test-user-001 --flatten -f ndjson
dws event consume user_im_message_receive_user --open-dingtalk-id abc --flatten -f ndjson
dws event consume user_im_message_read_o2o --user test-user-001 --flatten -f ndjson
dws event consume user_im_message_read_group --group <openConversationId> --flatten -f ndjson
dws event consume user_im_message_recall_o2o --user test-user-001 --flatten -f ndjson
dws event consume user_im_message_recall_group --group <openConversationId> --flatten -f ndjson
dws event consume user_im_message_reaction_o2o --user test-user-001 --flatten -f ndjson
dws event consume user_im_message_reaction_group --group <openConversationId> --flatten -f ndjson
```

上述所有 `*_o2o` 命令和 `user_im_message_receive_user` 都可将 `--user <userId>` 替换为 `--open-dingtalk-id <openDingtalkId>`，但两个参数不能同时使用。

```bash
dws event status --event user_im_message_receive_at
dws event status --event user_im_message_receive_o2o
dws event status --event user_im_message_receive_group
dws event status --event user_im_message_receive_user
dws event stop <subscribe_id> --dry-run
dws event stop <subscribe_id> --yes
dws event stop --all --dry-run
dws event stop --all --yes
```

## Subprocess contract

- 就绪：连上后 stderr 打 `[event] ready event_key=<key> bus_pid=<pid> subscribe_id=<id>`，父进程等这行再读 stdout。不要 `--quiet`（会抑制它）。
- 退出：末行 `[event] exited — received N event(s) in Xs (reason: limit|timeout|signal|bus_shutdown)`；受控退出码 0，失败非 0 且无 exited 行、有 Error 行。
- stdin 关闭 = 停机：仅当 stdin 是管道且未设 `--max-events/--duration` 时生效；交互终端和 `< /dev/null` 不触发。用管道 stdin 又要常驻就喂 `< <(tail -f /dev/null)`。
- 订阅清理：本次新建的订阅任意退出即自动退订；`--subscribe-id` 复用的保留；`--ephemeral` 强制退订。优雅停用 SIGTERM、关 stdin，或外部先预览 `dws event stop <subscribe_id> --dry-run`、确认后加 `--yes`。不要 `kill -9`（跳过退订、泄漏服务端订阅）。
- 一 consume 一 event_key；监听 N 个就起 N 个 consume，共用一个 bus。

## Output parsing

- 推荐 `--flatten -f ndjson`：顶层业务字段，一行一个事件 JSON，适合 Agent 管道读取。
- 人工取样可用 `--flatten -f json --max-events 1`。`--format` 只控制序列化，`--flatten` 控制数据结构。
- `--flatten` 的 `jq_root_path` 为 `.`；消息正文、发送人和会话 ID 分别直接读取顶层 `content`、`sender`、`conversation_id`。
- Agent 已显式使用 `--flatten`，不要再生成 `fromjson` 或内部 payload 路径。不传时默认保持兼容 envelope，业务 payload 在 `.data | fromjson`。正常处理直接持续读取 stdout，不要改写为 `--output-dir` watcher。
- 群自动回复使用顶层 `conversation_id`；单聊自动回复使用顶层 `sender_open_dingtalk_id`。
- 已读事件直接读取 `reader/reader_open_dingtalk_id/read_time`；撤回事件读取 `recaller/recaller_open_dingtalk_id/recall_time`。
- 表情回应事件直接读取 `operator/operator_open_dingtalk_id/reaction_name/reaction_text/operation_type/operation_time`。
- 图片、文件等媒体消息的 `content` 可能是可读描述；需要实际媒体文件时调用 `dws chat message download-media`。
- 正常动作事件输出不含内部 `payload/uid/corpid/clientId/filterSubId/bizid`；原始排查才使用 `-f raw` 或 `--debug-raw-events`。
- 自己发的消息不作为事件回来（`isSelfLoop` 过滤）；自发验证会看到 0 事件，测试投递使用别人或机器人发消息。
- `--jq <表达式>` 可进一步过滤或投影扁平输出。
- `--debug-raw-events` 仅用于服务端联调，正常消费不要使用；它和 `--flatten` 互斥，`-f raw` 也不能与 `--flatten` 同时使用。

## Troubleshooting

- consume 报 bus 启动失败：报错已带子进程真实原因。多为登录问题，`dws --profile <x> auth status` 看登录态（非默认组织带对 `--profile`），过期就 `auth login` 重登。
- 本地日志：`~/.dws/events/<edition>/personal_stream/<hash>/bus.log`（`edition` 一般 `open`，`hash` 见 `dws event status` 的 Workdir）；极早期失败可能无日志，以 consume 报错为准。
- 有残留 / 连不上：`dws event status` 查 stale，先用 `dws event stop --all --dry-run` 预览，确认后改用 `--yes` 清理重试。
- 挂住无输出：多是误加 `--foreground`（跑 bus、不打印事件），去掉。

## Full reference

- multi skill: `skills/multi/dingtalk-event/SKILL.md`
- IM reference: `skills/multi/dingtalk-event/references/event-im.md`
