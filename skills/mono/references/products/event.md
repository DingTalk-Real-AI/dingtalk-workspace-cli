# dws event — 个人 IM 与 OA 审批事件

通过个人 Stream 长连接监听当前用户的钉钉消息接收、已读、撤回、表情回应、群生命周期，以及审批任务和审批实例事件，NDJSON 输出到 stdout，用于驱动事件触发的 Agent。普通 IM 监听默认使用 `dws event +listen-im`；OA 审批、群生命周期、显式 EventKey、Filter DSL、subscribe_id 复用或原始 envelope 使用 `dws event consume`。不要写脚本轮询消息历史或审批列表。

## 运行方式

- bus 后台进程持有对钉钉的个人 Stream 长连；`+listen-im` 把高层意图编译为 EventKey，再复用 consume 的订阅、ready、NDJSON、取消、回滚和清理生命周期。监听本身不发消息；回复使用 `dws chat +messages-send`。
- 没有 bus 时监听命令自动拉起；普通任务只跑 `+listen-im`。
- 一个组织一个 bus，互不干扰、可同时跑；同组织内多个 consume 共享一个 bus。
- 非默认组织加全局 `--profile <corpId 或 profile 名>`；漏传会退回默认 profile 而失败。

## Core commands

| Command | Purpose |
|---|---|
| `dws event +listen-im --kind ... [flags]` | 普通 message/reaction/read/recall 监听；支持自然姓名/群名唯一解析 |
| `dws event list --category oa` | 查看当前公开的 OA 个人事件目录 |
| `dws event schema <event_key> --flatten` | 查看 Agent 使用的顶层业务字段 schema |
| `dws event consume <event_key> [event_key...] --flatten [flags]` | 高级入口：阻塞消费显式 EventKey，事件写到 stdout，用 `-f ndjson` |
| `dws event status --event <event_key>` | 查看个人订阅、bus、本地 consume |
| `dws event stop <subscribe_id> --dry-run` / `--yes` | 先预览，再确认取消订阅并停止对应本地消费 |
| `dws event stop --all --dry-run` / `--yes` | 先预览，再确认清理当前身份下全部个人订阅 |

注意区分两个 schema：`dws event schema <event_key>` 查事件的输出字段；`dws schema "event consume" --compact` 查 consume 命令自身的入参（统一内嵌 ToolSpec，含 parameters + 位置参数）。`source` 是 reviewed command identity 的 provenance；`event list/schema` 是 `interface_mode=local`，`event consume/status/stop` 因同时编排远端订阅控制面与本地 bus 而是 `interface_mode=composite`，不要把 identity 与实现机制混为一谈。

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
| `user_im_group_member_added` | 指定群聊有成员加入 | `--group` |
| `user_im_group_member_exited` | 指定群聊有成员退出 | `--group` |
| `user_im_group_disbanded` | 指定群聊被解散 | `--group` |
| `user_oa_approval_task_created` | 审批任务创建，发送给审批人 | 无 |
| `user_oa_approval_task_finished` | 审批任务已完成 | 无 |
| `user_oa_approval_task_redirected` | 审批任务已转交 | 无 |
| `user_oa_approval_instance_started` | 审批实例已发起 | 无 |
| `user_oa_approval_instance_cc` | 审批实例到达抄送节点，发送给被抄送人 | 无 |
| `user_oa_approval_instance_terminated` | 审批实例已终止 | 无 |
| `user_oa_approval_instance_finished` | 审批实例完成，发送给审批单发起人 | 无 |

只承认上表 23 个事件码。默认身份就是当前用户，使用当前用户 OAuth 登录态，不要额外加身份切换 flag。七个 OA 事件订阅当前用户相关的全部审批事件，规则均为 `all`、空 `filterRule`，不需要目标参数。

## Intent mapping

| 用户说 | 下一步 |
|---|---|
| "监听有人 @ 我的消息" | `dws event +listen-im --kind at-me -f ndjson` |
| "监听我和 userId test-user-001 的单聊消息" | `dws event +listen-im --kind sender --user test-user-001 -f ndjson` |
| "监听我和 openDingtalkId abc 的单聊消息" | `dws event +listen-im --kind sender --open-dingtalk-id abc -f ndjson` |
| "监听 XX 群消息" | `dws event +listen-im --kind group --chat-query "XX" -f ndjson`；多候选停止消歧 |
| "监听某人发给我的消息" | `dws event +listen-im --kind sender --user-query "<姓名>" -f ndjson` |
| "监听我的所有单聊消息" | `dws event +listen-im --kind all-direct -f ndjson` |
| "监听我所在的所有群消息" | `dws event +listen-im --kind all-group -f ndjson` |
| "监听某人的消息已读/撤回/reaction" | `dws event +listen-im --kind sender --user-query "<姓名>" --events read,recall,reaction -f ndjson` |
| "监听 XX 群消息已读/撤回/reaction" | `dws event +listen-im --kind group --chat-query "XX" --events read,recall,reaction -f ndjson` |
| "监听 XX 群改名" | 先解析群 ID，再 consume `user_im_group_updated --group <id>` |
| "监听有人加入 XX 群" | 先解析群 ID，再 consume `user_im_group_member_added --group <id>` |
| "监听有人退出 XX 群" | 先解析群 ID，再 consume `user_im_group_member_exited --group <id>` |
| "监听 XX 群解散" | 先解析群 ID，再 consume `user_im_group_disbanded --group <id>`；破坏性自测只能用测试群 |
| "监听并自动回复某人的单聊消息" | `+listen-im --kind sender --user-query "<姓名>"`；事件到达后把稳定 ID 交给 `chat +messages-send` |
| "同时监听同一人的消息、已读和撤回" | 一个 `+listen-im --kind sender --events message,read,recall` 生命周期 |
| "同时监听同一群的消息、改名和解散" | 生命周期事件不在 Shortcut 范围；一个高级 consume 放入 3 个 EventKey，共享 `--group` |
| "监听新的待我审批任务" / "有审批任务创建时通知我" | `event consume`，事件码 `user_oa_approval_task_created`，参数 `--flatten -f ndjson` |
| "审批任务完成时通知我" | `event consume`，事件码 `user_oa_approval_task_finished`，参数 `--flatten -f ndjson` |
| "审批任务被转交时通知我" | `event consume`，事件码 `user_oa_approval_task_redirected`，参数 `--flatten -f ndjson` |
| "有审批单发起时通知我" | `event consume`，事件码 `user_oa_approval_instance_started`，参数 `--flatten -f ndjson` |
| "有审批抄送给我时通知我" | `event consume`，事件码 `user_oa_approval_instance_cc`，参数 `--flatten -f ndjson` |
| "有审批单终止时通知我" | `event consume`，事件码 `user_oa_approval_instance_terminated`，参数 `--flatten -f ndjson` |
| "监听我发起的审批何时完成" / "审批实例完成时通知我" | `event consume`，事件码 `user_oa_approval_instance_finished`，参数 `--flatten -f ndjson` |
| "同时监听全部已公开 OA 事件" | 一个 consume 放入七个 OA event key，不加目标或消息过滤参数 |
| "查看个人事件 schema" | `dws event schema <event_key> --flatten` |
| "看个人事件订阅状态" | `dws event status --event <event_key>` |
| "停止这个个人事件订阅" | `dws event stop <subscribe_id> --dry-run`，确认后改用 `--yes` |

多候选让用户确认。缺必填 ID 且解析不出先追问，不要猜。企业内部 userId 使用 `--user`；明确给出 openDingtalkId，或目标是外部联系人、机器人、跨组织身份时使用 `--open-dingtalk-id`。两者严格二选一，不得混填、猜测或自动转换身份类型。

“我和某人的单聊”使用 `receive_o2o`；“某人发给我的消息/某人发送的消息”使用 `receive_user`，后者覆盖该发送人的单聊和群聊消息。只有明确说“所有”时才使用 `receive_o2o_all/receive_group_all`，指定对象仍使用范围更小的事件。用户要求执行“撤回消息”时走 `dws chat`；只有“监听/订阅消息撤回”才走 `dws event`。“贴标签”表示给消息贴表情时，对应 `reaction` 表情回应事件。OA 事件不接受 `--user`、`--open-dingtalk-id`、`--group`、`--query` 或 `--filter-json`。

## Call flow

1. 普通 IM 意图选择 `+listen-im` 的 `--kind` 与 `--events`；人名/群名直接用 `--user-query`/`--chat-query` 唯一解析。OA 审批与高级事件控制手选 EventKey。
2. 需要了解字段时运行 `dws event schema <event_key> --flatten`，读取 `schema.properties`；此模式的 `jq_root_path` 为 `.`。
3. 普通 IM 启动 `dws event +listen-im ... -f ndjson`；OA 或高级任务启动 `dws event consume <event_key> [event_key...] ... --flatten -f ndjson`。等待真实 ready marker；多事件记录每条 subscription，再等待整体 ready。不要用 `sleep` 猜测。
4. stdout 每行是一个扁平事件 JSON；消息、动作、群成员加入/退出及 OA 审批事件读取顶层业务字段。群标题变更和群解散只读取公共字段和 `payload` 中实际存在的字段。
5. 需要确认监听状态时运行 `dws event status --event <event_key>`，查看 `Subscriptions` 和 `Consumers`。
6. 任务完成后优雅结束 consume；本次新建的订阅会自动取消。复用已有订阅或需要从外部主动取消时，先运行 `dws event stop <subscribe_id> --dry-run`，向用户确认后再以 `--yes` 执行；自测可在 consume 加 `--max-events` 或 `--duration` 自动退出。

## Commands

普通 IM 监听：

```bash
dws event +listen-im --kind at-me -f ndjson
dws event +listen-im --kind sender --user-query "张三" --events message,reaction -f ndjson
dws event +listen-im --kind group --chat-query "项目冲刺" --events message,read,recall -f ndjson
dws event +listen-im --kind all-direct --max-events 10 -f ndjson
```

以下 EventKey 与 consume 命令是高级控制面：

```bash
dws event schema user_im_message_receive_at --flatten
dws event schema user_im_message_receive_o2o --flatten
dws event schema user_im_message_receive_group --flatten
dws event schema user_im_message_receive_user --flatten
dws event schema user_im_message_receive_o2o_all --flatten
dws event schema user_im_message_receive_group_all --flatten
dws event schema user_im_message_read_o2o --flatten
dws event schema user_im_message_read_group --flatten
dws event schema user_im_message_recall_o2o --flatten
dws event schema user_im_message_recall_group --flatten
dws event schema user_im_message_reaction_o2o --flatten
dws event schema user_im_message_reaction_group --flatten
dws event schema user_im_group_updated --flatten
dws event schema user_im_group_member_added --flatten
dws event schema user_im_group_member_exited --flatten
dws event schema user_im_group_disbanded --flatten
dws event schema user_oa_approval_task_created --flatten
dws event schema user_oa_approval_task_finished --flatten
dws event schema user_oa_approval_task_redirected --flatten
dws event schema user_oa_approval_instance_started --flatten
dws event schema user_oa_approval_instance_cc --flatten
dws event schema user_oa_approval_instance_terminated --flatten
dws event schema user_oa_approval_instance_finished --flatten
```

```bash
dws event consume user_im_message_receive_at --flatten -f ndjson
dws event consume user_im_message_receive_o2o --user test-user-001 --flatten -f ndjson
dws event consume user_im_message_receive_o2o --open-dingtalk-id abc --flatten -f ndjson
dws event consume user_im_message_receive_group --group <openConversationId> --flatten -f ndjson
dws event consume user_im_message_receive_user --user test-user-001 --flatten -f ndjson
dws event consume user_im_message_receive_user --open-dingtalk-id abc --flatten -f ndjson
dws event consume user_im_message_receive_o2o_all --flatten -f ndjson
dws event consume user_im_message_receive_group_all --flatten -f ndjson
dws event consume user_im_message_read_o2o --user test-user-001 --flatten -f ndjson
dws event consume user_im_message_read_group --group <openConversationId> --flatten -f ndjson
dws event consume user_im_message_recall_o2o --user test-user-001 --flatten -f ndjson
dws event consume user_im_message_recall_group --group <openConversationId> --flatten -f ndjson
dws event consume user_im_message_reaction_o2o --user test-user-001 --flatten -f ndjson
dws event consume user_im_message_reaction_group --group <openConversationId> --flatten -f ndjson
dws event consume user_im_group_updated --group <openConversationId> --flatten -f ndjson
dws event consume user_im_group_member_added --group <openConversationId> --flatten -f ndjson
dws event consume user_im_group_member_exited --group <openConversationId> --flatten -f ndjson
dws event consume user_im_group_disbanded --group <openConversationId> --flatten -f ndjson
dws event consume user_oa_approval_task_created --flatten -f ndjson
dws event consume user_oa_approval_task_finished --flatten -f ndjson
dws event consume user_oa_approval_task_redirected --flatten -f ndjson
dws event consume user_oa_approval_instance_started --flatten -f ndjson
dws event consume user_oa_approval_instance_cc --flatten -f ndjson
dws event consume user_oa_approval_instance_terminated --flatten -f ndjson
dws event consume user_oa_approval_instance_finished --flatten -f ndjson
```

同一目标、同一过滤条件的兼容事件优先使用一个多事件命令：

```bash
dws event consume \
  user_im_message_receive_o2o \
  user_im_message_read_o2o \
  user_im_message_recall_o2o \
  --user test-user-001 \
  --flatten \
  -f ndjson

dws event consume \
  user_im_message_receive_group \
  user_im_group_updated \
  user_im_group_disbanded \
  --group <openConversationId> \
  --flatten \
  -f ndjson

dws event consume \
  user_oa_approval_task_created \
  user_oa_approval_task_finished \
  user_oa_approval_task_redirected \
  user_oa_approval_instance_started \
  user_oa_approval_instance_cc \
  user_oa_approval_instance_terminated \
  user_oa_approval_instance_finished \
  --flatten \
  -f ndjson
```

用户类事件共享 `--user` 或 `--open-dingtalk-id`，群类事件共享 `--group`，无目标 IM 事件可加入任一组合。用户类与群类、不同目标或不同过滤条件要拆成多个进程。七个 OA 事件可以同进程消费并共享 personal bus，但各自建立独立订阅。多事件共享 `--query` / `--filter-json` 时，所选事件必须全部是 IM 消息接收事件；OA 事件单独或组合消费都禁止使用这两个消息过滤参数。

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

- 就绪：单事件等待 `[event] ready event_key=<key> bus_pid=<pid> subscribe_id=<id>`；多事件等待 `[event] ready event_count=<n> bus_pid=<pid>`，并保存此前每条 `[event] subscription ...` 的 subscribe ID。不要 `--quiet`。
- 退出：末行 `[event] exited — received N event(s) in Xs (reason: limit|timeout|signal|bus_shutdown)`；受控退出码 0，失败非 0 且无 exited 行、有 Error 行。
- stdin 关闭 = 停机：仅当 stdin 是管道且未设 `--max-events/--duration` 时生效；交互终端和 `< /dev/null` 不触发。用管道 stdin 又要常驻就喂 `< <(tail -f /dev/null)`。
- 订阅清理：本次新建的订阅任意退出即自动退订；`--subscribe-id` 复用的保留；`--ephemeral` 强制退订。优雅停用 SIGTERM、关 stdin，或外部先预览 `dws event stop <subscribe_id> --dry-run`、确认后加 `--yes`。不要 `kill -9`（跳过退订、泄漏服务端订阅）。
- 一个 consume 可监听多个兼容 event key，每个事件仍有独立订阅和 consumer，共用一个 bus、远程连接及输出。`event stop <subscribe_id>` 只移除目标事件，最后一个被移除后进程退出。

## 订阅创建失败与重试预算

以下约束适用于上表全部 23 个公开个人事件（16 个 IM + 7 个 OA）以及多事件命令中的每一项，只治理 `[event] ready` 之前的订阅创建；ready 之后的 Stream 断线由长连接重连机制处理。

- `0/2/1` 是 **Agent/host 编排约束**，不是 CLI 持久化硬总次数上限。每次 `dws event consume` 调用对每个逻辑订阅最多发送一次订阅创建 HTTP 请求，进程内不会自动重试。CLI 本地状态只持久化 `in_flight`、`cooldown`、`terminal_hold` 三种保护状态，不持久化或计算跨调用的 Agent/host 尝试次数。
- 解析人名或群名、执行 `event consume` 以及后续 `event status/stop` 必须使用同一个 `--profile`。不得把其它 profile 下解析出的 userId、openDingtalkId 或 openConversationId 直接带入当前 profile 的订阅。
- 同一逻辑订阅由当前 profile / 身份、event key、rule type、目标和过滤条件共同确定。`subscribe_id`、`trace_id` 以及重新启动进程都只是诊断或执行信息，不会生成新的逻辑操作；Agent/host 必须自行延续原编排预算。
- Agent/host 收到 `retryable=false`：`max_additional_attempts=0`，立即停止，不得自动重跑。
- Agent/host 收到 `retryable=true`：`max_additional_attempts=2`，初次失败后最多再尝试 2 次；错误若给出 `retry_after_seconds` 或 `next_retry_at`，不得提前重试。
- 未返回 `retryable`（即 `retryable=unknown`）：Agent/host 使用 `max_additional_attempts=1`，最多补偿尝试 1 次；仍无法确认时停止并上报错误与 trace。
- `in_flight` 表示同一逻辑订阅已有请求执行中；`cooldown` 或 `terminal_hold` 表示当前被退避或终态保护。遇到这些状态不得递归调用 `event consume`、并行启动相同订阅或通过新 subId / trace 绕过；等待原请求或保护时间结束，同时由 Agent/host 继续维护自己的编排次数。
- 多事件命令必须作为同一次原始操作治理。不得把失败事件拆成新的单事件命令、调整顺序或反复重启来重置预算；启动中任一项失败时，由 CLI 回滚本次已经创建的订阅。

### 本地保护状态运维

- open 版默认状态文件为 `~/.dws/events/open/personal_stream/<identity_hash>/personal_subscription_attempts.json`。设置 `DWS_CONFIG_DIR` 后配置根目录随之变化；其它 edition 使用对应 edition 目录，不固定为 `open`。
- identity 目录权限为 `0700`；`personal_subscription_attempts.json` 与 `personal_subscription_attempts.lock` 权限均为 `0600`。
- 连续 24h 没有失败后失败计数重置；`terminal_hold` 持续 1h。正常处理优先等待错误中的 `next_retry_at`，不要把删状态文件当成常规重试手段。
- 紧急恢复时，先确认该 identity 没有正在创建订阅的进程；只删除 `personal_subscription_attempts.json`，不要删除 lock 文件。删除 JSON 会清空该 identity 的全部保护记录，不只影响一个事件。

## Output parsing

- 推荐 `--flatten -f ndjson`：顶层业务字段，一行一个事件 JSON，适合 Agent 管道读取。
- 人工取样可用 `--flatten -f json --max-events 1`。`--format` 只控制序列化，`--flatten` 控制数据结构。
- `--flatten` 的 `jq_root_path` 为 `.`；消息正文、发送人和会话 ID 分别直接读取顶层 `content`、`sender`、`conversation_id`。
- 引用回复读取可选的 `quoted_message`；合并转发读取可选的 `forward_messages` 数组。两者保留内部消息的 `message_id/conversation_id/sender/sender_open_dingtalk_id/content/create_time`，不要解析可能随语言变化的外层聊天记录摘要。
- Agent 已显式使用 `--flatten`，不要再生成 `fromjson` 或内部 payload 路径。不传时默认保持兼容 envelope，业务 payload 在 `.data | fromjson`。正常处理直接持续读取 stdout，不要改写为 `--output-dir` watcher。
- 群自动回复使用顶层 `conversation_id`；单聊自动回复使用顶层 `sender_open_dingtalk_id`。
- 已读事件直接读取 `reader/reader_open_dingtalk_id/read_time`；撤回事件读取 `recaller/recaller_open_dingtalk_id/recall_time`。
- 表情回应事件直接读取 `operator/operator_open_dingtalk_id/reaction_name/reaction_text/operation_type/operation_time`。
- 群成员加入/退出事件读取 `conversation_id/operator/operator_open_dingtalk_id/members/event_time`。`operator` 是执行操作的人，`members` 是本次加入或退出的成员数组，成员项包含 `nick/open_dingtalk_id`；系统操作或成员自行退出时操作人字段可能为空。
- 群标题变更和群解散当前只承诺 `type/event_id/timestamp/subscribe_id/payload`；以实际 `payload` 为准，不猜测群标题、操作者等字段。
- OA 事件读取顶层 `process_instance_id/process_code/title/status/create_time/event_time`；任务事件另有 `task_id`，完成、转交或终止事件按对应 schema 提供 `finish_time`，任务完成、任务转交和实例完成还提供 `result`。`status/result` 保留服务端实际值，不推断完整枚举；缺少稳定 ID 或 payload 非法时 stderr 会输出 warning，stdout 回退为原始 transport envelope。
- 图片、文件等媒体消息的 `content` 可能是可读描述；合并转发媒体的下载定位信息位于对应 `forward_messages[].content`。需要实际媒体文件时调用 `dws chat message download-media`。
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
- IM task index: `skills/multi/dingtalk-event/references/event-im.md`
- OA reference: `skills/multi/dingtalk-event/references/event-oa.md`
