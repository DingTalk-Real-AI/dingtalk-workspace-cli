# dws event — 个人消息事件

监听当前用户收到的钉钉消息事件，NDJSON 输出到 stdout。实时监听 / 自动回复 / 订阅一律用 `dws event consume`，不要轮询消息历史。

## 运行方式

- bus 后台进程持有对钉钉的个人 Stream 长连；consume 从 bus 读事件、按 NDJSON 打到 stdout。consume 只读，不发消息（回复用 `dws chat message send`）。
- 没有 bus 时 consume 自动拉起；通常只跑 consume。
- 一个组织一个 bus，互不干扰、可同时跑；同组织内多个 consume 共享一个 bus。
- 非默认组织加全局 `--profile <corpId 或 profile 名>`；漏传会退回默认 profile 而失败。

## Core commands

| Command | Purpose |
|---|---|
| `dws event schema <event_key>` | 查看事件参数和输出字段 schema |
| `dws event consume <event_key> [flags]` | 阻塞消费，事件写到 stdout，用 `-f ndjson` |
| `dws event status --event <event_key>` | 查看个人订阅、bus、本地 consume |
| `dws event stop <subscribe_id>` | 取消订阅并停止对应本地消费 |
| `dws event stop --all` | 清理当前身份下全部个人订阅 |

注意区分两个 schema：`dws event schema <event_key>` 查事件的输出字段；`dws schema "event consume"` 查 consume 命令自身的入参（机读结构，source=cobra，含 parameters + 位置参数）。

## Event catalog

| 事件码 | 场景 | 必填参数 |
|---|---|---|
| `user_im_message_receive_at` | 当前用户被 @ 的消息 | 无 |
| `user_im_message_receive_o2o` | 与指定用户的单聊消息 | `--user` |
| `user_im_message_receive_group` | 指定群聊 / 会话的消息 | `--group` |

只承认这 3 个事件码。默认身份即当前用户，不加身份切换 flag。

## Intent mapping

| 用户说 | 下一步 |
|---|---|
| 监听有人 @ 我 | consume `user_im_message_receive_at` |
| 监听我和某用户的单聊 | 解析对端 userId，consume `user_im_message_receive_o2o --user <id>` |
| 监听某群 | 先 `dws chat search --query "<群名>"` 拿 openConversationId，再 consume group |
| 监听并自动回复某人单聊 | 解析对端 userId，启动 o2o consume；回复用 `dws chat message send` |
| 查看事件 schema | `dws event schema <event_key>` |
| 看订阅状态 | `dws event status --event <event_key>` |
| 停止订阅 | `dws event stop <subscribe_id>` |

多候选让用户确认。缺必填 ID 且解析不出先追问，不要猜。

## Call flow

1. 按意图选事件码；人名 / 群名先解析成必填 ID。
2. 需要字段时 `dws event schema <event_key>`，读 `jq_root_path` 和 `schema.properties`。
3. 启动 `dws event consume <event_key> ... -f ndjson`，等 stderr 出现 `[event] ready event_key=<key> ...` 再读 stdout，不要 sleep。
4. stdout 每行一个事件 JSON；`data` 字段是 JSON 字符串，按 `jq_root_path` 再 parse。
5. `dws event status --event <event_key>` 看 Subscriptions / Consumers。
6. `dws event stop <subscribe_id>` 取消订阅；自测可加 `--max-events` / `--duration` 自动退出。

## Commands

```bash
dws event schema user_im_message_receive_o2o

dws event consume user_im_message_receive_at -f ndjson
dws event consume user_im_message_receive_o2o --user 507971 -f ndjson
dws event consume user_im_message_receive_group --group <openConversationId> -f ndjson

dws event status --event user_im_message_receive_o2o
dws event stop <subscribe_id>
dws event stop --all
```

## Subprocess contract

- 就绪：连上后 stderr 打 `[event] ready event_key=<key> bus_pid=<pid>`，父进程等这行再读 stdout。不要 `--quiet`（会抑制它）。
- 退出：末行 `[event] exited — received N event(s) in Xs (reason: limit|timeout|signal|bus_shutdown)`；受控退出码 0，失败非 0 且无 exited 行、有 Error 行。
- stdin 关闭 = 停机：仅当 stdin 是管道且未设 `--max-events/--duration` 时生效；交互终端和 `< /dev/null` 不触发。用管道 stdin 又要常驻就喂 `< <(tail -f /dev/null)`。
- 订阅清理：本次新建的订阅任意退出即自动退订；`--subscribe-id` 复用的保留；`--ephemeral` 强制退订。优雅停用 SIGTERM、关 stdin，或外部 `dws event stop <subscribe_id>`。不要 `kill -9`（跳过退订、泄漏服务端订阅）。
- 一 consume 一 event_key；监听 N 个就起 N 个 consume，共用一个 bus。

## Output parsing

- 用 `-f ndjson`，一行一个事件 JSON。抓样本用 `-f json --max-events 1`。
- 两层解析：外层事件 JSON 的 `data` 字段是 JSON 字符串，`fromjson` 后取 `payload.body.content`（正文）/ `payload.body.sender`（发送人）/ `payload.body.openConversationId`（会话）。样例：

```json
{"type":"event","event_type":"user_im_message_receive_group",
 "data":"{\"payload\":{\"body\":{\"sender\":\"张三\",\"content\":\"你好\",\"openConversationId\":\"cid...==\"}},\"subject\":{\"isSelfLoop\":false}}"}
```

- 自己发的消息不作为事件回来（`isSelfLoop` 过滤）：边听边回不成环；自发验证会看到 0 事件，测投递用别人 / 机器人发。
- `--jq <表达式>` 把过滤 / 投影下推到 consume，减少输出。
- `--debug-raw-events` 仅联调用。

## Troubleshooting

- consume 报 bus 启动失败：报错已带子进程真实原因。多为登录问题，`dws --profile <x> auth status` 看登录态（非默认组织带对 `--profile`），过期就 `auth login` 重登。
- 本地日志：`~/.dws/events/<edition>/personal_stream/<hash>/bus.log`（`edition` 一般 `open`，`hash` 见 `dws event status` 的 Workdir）；极早期失败可能无日志，以 consume 报错为准。
- 有残留 / 连不上：`dws event status` 查 stale，`dws event stop --all` 清理重试。
- 挂住无输出：多是误加 `--foreground`（跑 bus、不打印事件），去掉。

## Full reference

- multi skill: `skills/multi/dingtalk-event/SKILL.md`
- IM reference: `skills/multi/dingtalk-event/references/event-im.md`
