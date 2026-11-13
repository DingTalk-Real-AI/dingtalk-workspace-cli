# dws event — 个人事件监听与应用三方事件发送

通过个人 Stream 长连接监听当前用户的钉钉消息接收、已读、撤回、表情回应、群生命周期、审批任务/实例、VoIP 通话邀请、待办创建/更新/删除、好友申请/添加成功、互动卡片回调，以及听记创建/纪要生成、主日历创建/修改/取消事件，NDJSON 输出到 stdout，用于驱动事件触发的 Agent。普通 IM 监听默认使用 `dws event +listen-im`；OA 审批、VoIP、Todo、好友、互动卡片、听记、日历、群生命周期、显式 EventKey、Filter DSL、subscribe_id 复用或原始 envelope 使用 `dws event consume`。不要写脚本轮询消息历史、审批列表、通话记录、待办列表、好友列表、好友申请列表、听记列表或日程列表。

企业自建应用发送三方事件使用 `dws event send`，以应用 Token 对接现有事件上报接口。完整应用凭证即可调用，事件 ID、业务版本、接收人和 payload 由调用方提供。

## 应用事件发送

```bash
dws event send --event-key app_user_order_created --event-id order-1001-created --payload-schema-version 3 --user-id '<企业内 userId>' --payload '{"orderId":"order-1001"}' -f json
```

- `--event-key`、`--event-id`、`--payload-schema-version`、`--user-id` 和 `--payload` 必填。`--user-id` 可重复，去重后最多 100 人。`--payload` 支持 JSON 对象、`@文件` 或 `-` 标准输入并保留数字精度。可选 `--occurred-at` 使用带时区的 ISO 8601 时间。
- 凭证优先使用完整 `--client-id` / `--client-secret`，再到完整 `DWS_CLIENT_ID` / `DWS_CLIENT_SECRET`，最后读取应用配置和 Keychain。显式来源只提供一项时按错误提示补齐。凭证只用于应用 Token 换取，结果和日志保持脱敏。
- 统一结果的 `data` 包含 `eventId`、`status`、`duplicate`。`ACCEPTED` 与 `DUPLICATE` 都是事件已接纳的成功回执，退出码 0。实际收件由订阅端核验。
- 每次调用上报一次。重发同一事件沿用原 `--event-id` 和完整请求内容，发送新事件使用新 ID。同键不同内容会返回冲突。结果不确定时保留原事件 ID，按服务端错误与重试提示处理。
- `--dry-run` 校验直接 JSON，文件和 stdin 仅显示来源。预览阶段不读取文件内容、stdin 或 Keychain，也不调用网络。

## 运行方式

- bus 后台进程持有对钉钉的个人 Stream 长连；`+listen-im` 把高层意图编译为 EventKey，再复用 consume 的订阅、ready、NDJSON、取消、回滚和清理生命周期。监听本身不发消息；回复使用 `dws chat +messages-send`。
- 没有 bus 时监听命令自动拉起；普通任务只跑 `+listen-im`。
- 一个组织一个 bus，互不干扰、可同时跑；同组织内多个 consume 共享一个 bus。
- 非默认组织加全局 `--profile <corpId 或 profile 名>`；漏传会退回默认 profile 而失败。

## 监听的 AppKey 与认证排障

开源版默认 `normal` 模式只需要用户 token 和 AppKey，不要求用户提供 AppSecret。已有 AppKey 沿用本地身份；本地元数据缺失时，CLI 从当前事件 MCP 的 `/cli/clientId` 自动获取，仅用于本次调用并传给后台 bus，不写入 token/profile/app 配置。`custom` 模式和定制版不使用此兜底；取得 AppKey 不代表已验证 token 的应用归属，仍由服务端校验。

自动获取失败时按结构化 `reason` 和 `retryable` 处理；网络暂时失败、429、5xx 可按预算重试，取消、拒绝或无效数据不盲目重试。不要引导默认模式用户提供 AppSecret。排查云端差异时只记录配置目录及字段存在性，不输出凭据；`auth status` 不能证明 AppKey 完整；官方分类始终读取 CLI 内置目录；应用事件登录后读取远端目录。未登录时提示应用目录不可用；无筛选浏览的远端故障可能降级为官方项，带应用筛选时远端故障报错。不要把降级结果当作完整应用目录。

## Core commands

| Command | Purpose |
|---|---|
| `dws event send --event-key ... --event-id ... --payload-schema-version ... --user-id ... --payload ...` | 使用企业自建应用凭证向指定企业内用户上报三方事件 |
| `dws event +listen-im --kind ... [flags]` | 普通 message/reaction/read/recall 监听；支持自然姓名/群名唯一解析 |
| `dws event list --category oa` | 查看当前公开的 OA 个人事件目录 |
| `dws event list --category voip` | 查看当前公开的 VoIP 个人事件目录 |
| `dws event list --category todo` | 查看当前公开的 Todo 个人事件目录 |
| `dws event list --category contact` | 查看当前公开的好友个人事件目录 |
| `dws event list --category card` | 查看当前公开的互动卡片个人事件目录 |
| `dws event list --category minutes` | 查看当前公开的听记个人事件目录 |
| `dws event list --category calendar` | 查看当前公开的日历个人事件目录 |
| `dws event schema <event_key> --flatten` | 查看 Agent 使用的顶层业务字段 schema |
| `dws event consume <event_key> [event_key...] --flatten [flags]` | 高级入口：阻塞消费显式 EventKey，事件写到 stdout，用 `-f ndjson` |
| `dws event status --event <event_key>` | 查看个人订阅、bus、本地 consume |
| `dws event stop <subscribe_id> --dry-run` / `--yes` | 先预览，再确认取消订阅并停止对应本地消费 |
| `dws event stop --all --dry-run` / `--yes` | 先预览，再确认清理当前身份下全部个人订阅 |

注意区分两个 schema：`dws event schema <event_key>` 查事件的输出字段；`dws schema "event consume" --compact` 查 consume 命令自身的入参（统一内嵌 ToolSpec，含 parameters + 位置参数）。`source` 是 reviewed command identity 的 provenance；`event list` 与 `event schema` 登录后都会访问远端（组织事件目录 / 应用事件业务 Schema）并与内置定义合并，`event consume/status/stop` 同时编排远端订阅控制面与本地 bus，均为 `interface_mode=composite`，不要把 identity 与实现机制混为一谈。

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
| `user_voip_call_receive_invite` | 当前用户作为被叫收到语音通话邀请 | 无 |
| `user_todo_task_create` | 当前用户相关的待办被创建 | `--role-types` 可选，默认全部角色 |
| `user_todo_task_update` | 当前用户相关的待办被更新 | `--role-types` 可选，默认全部角色 |
| `user_todo_task_delete` | 当前用户相关的待办被删除 | `--role-types` 可选，默认全部角色 |
| `user_contact_friend_request_received` | 当前用户收到好友申请 | 无 |
| `user_contact_friend_added` | 当前用户与目标用户建立好友关系 | 无 |
| `user_card_action_triggered` | 当前用户收到互动卡片业务回调 | 无 |
| `user_minutes_task_created` | 听记对象、必要关联及默认权限初始化成功；复用已有听记不通知 | 无 |
| `user_minutes_summary_generated` | 默认纪要首次生成成功且可读取；重新生成、切换模板、人工修改不通知 | 无 |
| `user_calendar_event_changed` | 当前用户主日历创建及新增参与、修改、取消 | `--calendar-ids` / `--change-types` 可选 |

个人官方事件使用上表 33 个事件码。默认身份就是当前用户，使用当前用户 OAuth 登录态，不要额外加身份切换 flag。七个 OA 事件、一个 VoIP 事件、两个好友事件、互动卡片事件和两个听记事件均使用 `all` 规则和 `{}` `filterRule`，不需要目标参数。三个 Todo 事件用 `--role-types creator,executor,participant` 控制当前用户作为创建者、执行者或参与者的范围；省略时默认三种角色，并下发为 `filterRule.roleTypes`。

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
| "收到语音通话邀请时通知我" / "监听 VoIP 来电" | `dws event consume user_voip_call_receive_invite --flatten -f ndjson` |
| "查看 VoIP 事件目录" | `dws event list --category voip` |
| "收到好友申请时通知我" / "有人加我好友时通知我" | `dws event consume user_contact_friend_request_received --flatten -f ndjson` |
| "好友添加成功时通知我" | `dws event consume user_contact_friend_added --flatten -f ndjson` |
| "同时监听全部好友事件" | 一个 consume 放入两个好友 event key，不加目标或过滤参数 |
| "查看好友事件目录" | `dws event list --category contact` |
| "查看或操作好友关系" | 使用 `dws contact`，不使用事件监听 |
| "互动卡片发生操作时通知我" / "监听卡片回调" | `dws event consume user_card_action_triggered --flatten -f ndjson` |
| "查看互动卡片事件目录" | `dws event list --category card` |
| "新建听记时通知我" | `dws event consume user_minutes_task_created --flatten -f ndjson` |
| "纪要生成后通知我" / "会议纪要出来后处理" | `dws event consume user_minutes_summary_generated --flatten -f ndjson`，再用 `task_uuid` 执行 `dws minutes get summary --id <task_uuid>` |
| "查看听记事件目录" | `dws event list --category minutes` |
| "我的主日历新建、修改或取消日程时通知我" | `dws event consume user_calendar_event_changed --calendar-ids primary --change-types eventCreated,eventUpdated,eventCancelled --flatten -f ndjson` |
| "查看日历事件目录" | `dws event list --category calendar` |
| "查询、创建或修改日程" | 使用 `dws calendar`；取消日程通知与停止事件订阅是两个操作 |
| "监听指派给我的待办创建/更新/删除" | 对应 `user_todo_task_create/update/delete`，使用 `--role-types executor --flatten -f ndjson` |
| "监听我创建的待办变化" | 对应 Todo EventKey，使用 `--role-types creator --flatten -f ndjson` |
| "监听所有与我相关的待办变化" | 一个 consume 放入三个 Todo EventKey，省略 `--role-types` 以使用三种角色并集 |
| "查看个人事件 schema" | `dws event schema <event_key> --flatten` |
| "看个人事件订阅状态" | `dws event status --event <event_key>` |
| "停止这个个人事件订阅" | `dws event stop <subscribe_id> --dry-run`，确认后改用 `--yes` |

多候选让用户确认。缺必填 ID 且解析不出先追问，不要猜。企业内部 userId 使用 `--user`；明确给出 openDingtalkId，或目标是外部联系人、机器人、跨组织身份时使用 `--open-dingtalk-id`。两者严格二选一，不得混填、猜测或自动转换身份类型。

“我和某人的单聊”使用 `receive_o2o`；“某人发给我的消息/某人发送的消息”使用 `receive_user`，后者覆盖该发送人的单聊和群聊消息。只有明确说“所有”时才使用 `receive_o2o_all/receive_group_all`，指定对象仍使用范围更小的事件。用户要求执行“撤回消息”时走 `dws chat`；只有“监听/订阅消息撤回”才走 `dws event`。“贴标签”表示给消息贴表情时，对应 `reaction` 表情回应事件。OA、VoIP 与好友事件不接受 `--user`、`--open-dingtalk-id`、`--group`、`--query`、`--filter-json` 或 `--role-types`。Todo 事件只接受角色范围，不接受用户、群或消息 Filter 参数。

## Call flow

1. 普通 IM 意图选择 `+listen-im` 的 `--kind` 与 `--events`；人名/群名直接用 `--user-query`/`--chat-query` 唯一解析。OA、VoIP、Todo、好友与高级事件控制手选 EventKey。
2. 需要了解字段时运行 `dws event schema <event_key> --flatten`，读取 `schema.properties`；此模式的 `jq_root_path` 为 `.`。
3. 普通 IM 启动 `dws event +listen-im ... -f ndjson`；OA、VoIP、Todo、好友或高级任务启动 `dws event consume <event_key> [event_key...] ... --flatten -f ndjson`。等待真实 ready marker；多事件记录每条 subscription，再等待整体 ready。不要用 `sleep` 猜测。
4. stdout 每行是一个扁平事件 JSON；消息、动作、群成员加入/退出、OA 审批、VoIP 通话邀请、Todo 与好友事件读取顶层业务字段。群标题变更和群解散只读取公共字段和 `payload` 中实际存在的字段。
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
dws event schema user_voip_call_receive_invite --flatten
dws event schema user_todo_task_create --flatten
dws event schema user_todo_task_update --flatten
dws event schema user_todo_task_delete --flatten
dws event schema user_contact_friend_request_received --flatten
dws event schema user_contact_friend_added --flatten
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
dws event consume user_voip_call_receive_invite --flatten -f ndjson
dws event consume user_todo_task_create --role-types executor --flatten -f ndjson
dws event consume user_todo_task_update --role-types executor --flatten -f ndjson
dws event consume user_todo_task_delete --role-types executor --flatten -f ndjson
dws event consume user_contact_friend_request_received --flatten -f ndjson
dws event consume user_contact_friend_added --flatten -f ndjson
dws event consume user_card_action_triggered --flatten -f ndjson
dws event consume user_minutes_task_created --flatten -f ndjson
dws event consume user_minutes_summary_generated --flatten -f ndjson
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

dws event consume \
  user_todo_task_create \
  user_todo_task_update \
  user_todo_task_delete \
  --role-types executor \
  --flatten \
  -f ndjson

dws event consume \
  user_contact_friend_request_received \
  user_contact_friend_added \
  --flatten \
  -f ndjson
```

用户类事件共享 `--user` 或 `--open-dingtalk-id`，群类事件共享 `--group`，无目标 IM 事件可加入任一组合。用户类与群类、不同目标或不同过滤条件要拆成多个进程。七个 OA 事件、VoIP、两个好友事件、互动卡片与两个听记事件可以同进程消费；三个 Todo 事件可以共享一个 `--role-types` 范围；每个事件仍建立独立订阅并共享 personal bus。多事件共享 `--query` / `--filter-json` 时，所选事件必须全部是 IM 消息接收事件；OA、VoIP、Todo、好友、互动卡片和听记事件禁止使用消息过滤参数。

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

## 主日历变化订阅

`user_calendar_event_changed` 使用 `all` 规则，监听目标 OAuth 用户主日历创建及新增参与
`eventCreated`、修改 `eventUpdated`、取消 `eventCancelled`；不补发历史，不承诺非主日历或循环例外。

```bash
dws event list --category calendar
dws event schema user_calendar_event_changed --flatten
dws event consume user_calendar_event_changed --calendar-ids primary --change-types eventCreated,eventUpdated,eventCancelled --flatten --duration 10m -f ndjson
```

- 使用同一 OAuth 账号与 `--profile`。`primary` 表示当前用户主日历，不能使用内部 folderId 或 uid。
- 两个数组维度省略即通配；显式 `--calendar-ids=` / `--change-types=` 为空数组、不匹配任何消息。`*` 不可与具体值混用，集合排序去重后序列化为 String 类型的 `filterRule`。
- 三种变化可单独或组合订阅；旧的 `eventCreated` 限定订阅不会自动扩大。取消类型是 `eventCancelled`，不是 `eventDeleted`。
- 日历过滤不能和其他业务事件混用，也不能覆盖 `--subscribe-id`。不接受 `--user`、`--open-dingtalk-id`、`--group`、`--role-types`、`--query` 或 `--filter-json`。
- 扁平结果按 `biz_id` 幂等；`event_id` 是事件中心 ID，`calendar_event_id` 是 OpenAPI 日程资源 ID，三者不要混用。可选 `operator_uid` 接受源载荷字符串或整数，输出为字符串；缺失时不以接收人代替。
- 收到取消通知后继续监听；日程详情可能已不可查询。结束监听按上述生命周期清理订阅。

目录中的 enabled/public 表示 CLI 支持该事件，不代表当前环境已开通投递。事件开通及三种变化类型的端到端投递都需在目标环境单独验收。等待 ready 只证明接入本地 bus，必须实际收到三类事件后再报告链路成功。

## 订阅创建失败与重试预算

以下约束适用于上表全部 33 个公开个人事件（16 个 IM + 7 个 OA + 1 个 VoIP + 3 个 Todo + 2 个好友 + 1 个互动卡片 + 2 个听记 + 1 个日历）以及多事件命令中的每一项，只治理 `[event] ready` 之前的订阅创建；ready 之后的 Stream 断线由长连接重连机制处理。

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
- OA 事件读取顶层 `process_instance_id/process_code/title/status/create_time/event_time`；七类事件还支持可选字符串 `staff_id`、`activity_id`、`corp_id`、`business_id`，仅在服务端提供对应非空值时输出；抄送事件还可提供毫秒时间戳 `cc_time`；任务事件另有 `task_id`，完成、转交或终止事件按对应 schema 提供 `finish_time`，任务完成、任务转交和实例完成还提供 `result`。`status/result` 保留服务端实际值，不推断完整枚举；缺少稳定 ID 或 payload 非法时 stderr 会输出 warning，stdout 回退为原始 transport envelope。
- VoIP 事件读取顶层 `biz_id/corp_id/org_id/target_uid/call_id/caller_uid/callee_uid/call_type/room_id/sdk_app_id/sdk_expire_time/create_time/event_time`。`caller_uid/callee_uid` 是字符串标识，保留前导 `0`、连字符等原始内容，不转换为数字；`caller_uid` 读取新的 `body.callerUserId` 并兼容旧 `body.callerUid`；`biz_id` 是重试稳定的业务去重 ID。`sdk_app_id` 与 `sdk_expire_time` 来自非敏感的 `body.sdkAuth`，过期时间为 Unix 秒。MeetingSDK token 优先读取 `body.sdkAuth.sdkToken`，并兼容旧 header `X-Mozi-Agent-Sdk-Token`；敏感入会码不进入 `--flatten` 输出，默认非扁平 transport envelope 的 `.data` 也会移除 `roomCode` 与 body token，并从 `.headers` 中移除旧版 token；数字员工确需动态 MeetingSDK 凭证时显式加 `--include-voip-sdk-token`，并将 stdout 直接交给受控入会进程，不得记录或转发；只有显式 `--debug-raw-events` 才输出原始值。
- Todo 事件读取顶层 `task_id/subject/creator_id/create_time`；创建和更新事件还提供角色列表、优先级、状态阶段、计划/实际时间、来源与场景字段，更新额外提供 `old_status_stage/update_time`，删除提供 `delete_time`。空的可选时间和 `parent_id` 省略；缺少 `task_id` 或 payload 非法时回退为原始 transport envelope。
- 好友申请事件读取顶层 `src_open_dingtalk_id/src_name/dest_open_dingtalk_id/remark/source/biz_type/apply_time`；好友添加成功事件读取 `friend_open_dingtalk_id/friend_name/establish_time/direction`。`direction` 为当前用户视角，`active` 表示主动添加，`passive` 表示被动添加；`apply_time/establish_time` 是毫秒时间戳。payload 缺失、为空或无法解析时 stderr 输出 warning，stdout 回退为原始 transport envelope。
- 互动卡片事件保持顶层 `type/event_id/timestamp/subscribe_id/payload`，结构化上下文优先读取 `payload.body.actionData.context`。通过 `questions[].id` 关联 `answers[question_id]`，`selected` 中的选项 ID 再按同一问题的 `options[].id` 转换为标签；空数组是合法未选择状态，`custom` 独立保留。操作者使用整数 `operatorDTO.uid`，`createUid/orgId` 保持字符串；`bizInfoDTO.bizId`、`sourceTurnId`、`spaceId`、`conversationContextDTO.cid` 均按不透明 ID 关联。`timestamp/event_time/triggerTimestamp` 分别保留且不假设相等；字符串化的 `body.context` 和 `extension` 只作兼容或诊断回退。各层继续保留未知字段；注册时传递 `filterRule={}`，且不接受用户、群、角色或消息 Filter 参数。
- 听记事件读取顶层 `biz_id/event_time/schema_version/task_uuid/creator_open_dingtalk_id`。`biz_id`、`event_time` 在同一业务事件重试时不变，消费端按 `biz_id` 幂等处理，事件可能重复或乱序；`creator_open_dingtalk_id` 是创建者标识（十进制字符串），不一定是当前接收人。事件不含听记或纪要正文，创建事件不表示纪要已就绪；读取内容用同一 `--profile` 执行 `dws minutes get summary --id <task_uuid>`。`schemaVersion` 不为 1、缺少 `taskUuid` 或创建者标识非法时回退为原始 transport envelope。
- 图片、文件等媒体消息的 `content` 可能是可读描述；合并转发媒体的下载定位信息位于对应 `forward_messages[].content`。需要实际媒体文件时调用 `dws chat message download-media`。
- 普通 IM/OA 动作事件输出不含内部 `payload/uid/corpid/clientId/filterSubId/bizid`；VoIP 按已评审协议将所需字段映射为 `target_uid/corp_id/biz_id`。VoIP 原始排查必须显式使用 `--debug-raw-events`，`-f raw` 也必须和该开关一起使用。
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
- VoIP reference: `skills/multi/dingtalk-event/references/event-voip.md`
- Todo reference: `skills/multi/dingtalk-event/references/event-todo.md`
- Contact reference: `skills/multi/dingtalk-event/references/event-contact.md`

## 应用事件消费

应用事件已在目标分支接入 `dws event list --category appEvent` → `dws event schema <event_key> --flatten` → `dws event consume <event_key> --flatten -f ndjson`。登录后读取远端目录，Schema 描述 `.payload`；订阅采用 `ruleType=all` 且不传 filterRule，不接受目标、角色或消息 Filter。flatten 的 `payload_schema_version` 对应信封/查询参数 `payloadSchemaVersion`；平台协议 `schemaVersion` 不进入 flatten。应用事件可用 `dws event schema <event_key> --payload-schema-version <ver>` 查询指定已发布业务 Schema，省略时查询当前生效版本；官方事件不接受该参数。

详细发现规则、版本映射、NDJSON 示例和开放态验收以 [应用事件参考](../../../multi/dingtalk-event/references/event-app.md) 为准。应用定义配置走 `dws dev event`，接收钉钉回调的配置走 `dws dev app event`，不要混用。
