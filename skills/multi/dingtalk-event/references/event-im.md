# IM 事件任务路由

本页是任务索引，不再混放 16 个 EventKey、输出字段和运维细节。先用上层
[SKILL.md](../SKILL.md) 的 Golden Route；只加载当前任务对应的一份子 reference。

<!-- dws-intent: event.listen.im -->消息、reaction、已读和撤回默认使用 `dws event +listen-im` 长连接；
群生命周期、显式 EventKey、Filter DSL、原始 envelope 或底层订阅控制才使用
`event consume`。不要写轮询脚本。

## 选择哪一页

| 任务 | Reference |
|---|---|
| 选择 16 个 EventKey、区分 user/group/all 规则、组合底层 consume | [event-im-keys.md](event-im-keys.md) |
| 等待 ready、多事件回滚、bounded consume、退出清理 | [event-im-lifecycle.md](event-im-lifecycle.md) |
| 扁平字段、引用/转发、动作与群成员事件、事件驱动回复 | [event-im-output.md](event-im-output.md) |
| Filter、status/stop、重试预算、本地保护与排障 | [event-im-operations.md](event-im-operations.md) |

## 共同硬规则

- 个人事件使用当前用户 OAuth；解析、consume、status、stop 必须是同一 `--profile`。
- `+listen-im --user-query/--chat-query` 在 CLI 内唯一解析；零命中、多候选或分页不完整时，
  在创建订阅前停止。底层 fallback 只传真实稳定 ID。
- 默认 `--flatten -f ndjson`；stdout 只处理事件，stderr 等待明确 ready marker。
- 指定目标优先于 `*_all`；只有用户明确说“全部单聊/全部群消息”才订阅全量事件。
- 当前用户自己发送的消息会被 self-loop 过滤；自测由另一用户或机器人触发。
- 事件只负责监听。回复必须使用事件里的真实 `conversation_id` 或
  `sender_open_dingtalk_id`，禁止把展示名重新做自然查询。

## 跨页契约索引

以下索引保留跨页必须一致的机器可检验契约；解释、示例和操作步骤仍按上表按需加载：

- 16 个事件都使用同一 `--profile`。就绪以 `[event] ready` 为准；全量事件是
  `user_im_message_receive_o2o_all` / `user_im_message_receive_group_all`，群生命周期是
  `user_im_group_updated` / `user_im_group_member_added` /
  `user_im_group_member_exited` / `user_im_group_disbanded`。
- 扁平动作字段包括 `reader_open_dingtalk_id`、`recaller_open_dingtalk_id`、
  `operator_open_dingtalk_id`、`reaction_name`、`operation_type` 和 `members`；媒体 lower
  fallback 是 `dws chat message download-media`，外部身份参数是 `--open-dingtalk-id`。
- 创建失败遵循 Agent/host 的 `0/2/1` 预算：`retryable=false` 对应
  `max_additional_attempts=0`，`retryable=true` 对应 `max_additional_attempts=2`，
  `retryable=unknown` 对应 `max_additional_attempts=1`，并遵守 `retry_after_seconds` /
  `next_retry_at`。这不是 CLI 持久化硬总次数上限；进程内不会自动重试，也不持久化或计算跨调用的
  Agent/host 尝试次数。`subscribe_id` / `trace_id` 不重置预算，`in_flight` / `cooldown` /
  `terminal_hold` 不得被并发绕过。
- open 版保护文件是
  `~/.dws/events/open/personal_stream/<identity_hash>/personal_subscription_attempts.json`；
  `DWS_CONFIG_DIR` 可改变根目录。目录权限 `0700`，`personal_subscription_attempts.json` 与
  `personal_subscription_attempts.lock` 权限 `0600`；连续 `24h` 无失败后重置，
  `terminal_hold` 为 `1h`。紧急恢复只删除 `personal_subscription_attempts.json`，
  不要删除 lock 文件；这会清空该 identity 的全部保护记录。

## Schema 边界

- 业务 payload：`dws event schema <event_key> --flatten`。
- CLI 参数/安全：`dws schema --cli-path "event +listen-im" --compact -f json` 或精确 compact consume leaf。
- `--flatten` 的 `jq_root_path` 为 `.`；兼容 transport envelope 才使用 `.data | fromjson`。
