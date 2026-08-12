# IM 订阅过滤、状态与排障

## Filter

优先用订阅规则缩小范围：单聊/发送人用 user 身份，群用 `--group`。接收消息事件需要额外
正文过滤时才用 `--query` 或 `--filter-json`；已读、撤回、reaction 和群生命周期不使用
消息内容过滤。业务别名包括 `content`、`sender`、`conversation_id`、
`sender_open_dingtalk_id`。

## Status 与 stop

```bash
dws event status --event <event_key>
dws event stop <subscribe_id> --dry-run
dws event stop <subscribe_id> --yes
dws event stop --all --dry-run
dws event stop --all --yes
```

`status` 同时看服务端 Subscriptions 与本地 Consumers；用 PID、EventKey、subscribe_id 和
received/dropped 判断 consumer。裸 `event stop` 不取消任何订阅。stop 是有影响操作，先预览，
按 Runtime gate 确认后执行。

## 创建失败与 `0/2/1` 编排预算

这些约束只治理 ready 之前的创建；ready 后断线由长连接重连：

- `retryable=false`：额外尝试 0 次。
- `retryable=true`：最多额外 2 次，并遵守 `retry_after_seconds/next_retry_at`。
- 未给 retryable：最多额外 1 次，然后停止并保留 trace。

这是 Agent/host 预算，不是 CLI 跨进程计数器。一个逻辑订阅由 profile/身份、EventKey、
rule、目标和 filter 确定；换 subscribe_id、trace 或进程不能重置预算。遇到 `in_flight`、
`cooldown`、`terminal_hold` 不并发、递归或拆分多事件绕过保护。

## 本地保护状态

open 版路径为
`~/.dws/events/open/personal_stream/<identity_hash>/personal_subscription_attempts.json`；
`DWS_CONFIG_DIR` 会改变根目录。identity 目录为 `0700`，JSON 与 lock 为 `0600`。
连续 24h 无失败后重置，terminal hold 为 1h。

紧急恢复前确认没有创建进程，只删除 attempts JSON，不删除 lock；这会清空该 identity 的
全部保护记录，不是常规重试方式。

## 最短排障

- 无输出：先确认正确 ready marker，再看 subscribe_id 和 received/dropped。
- 目标错误：o2o/user 检查身份 flag，group 检查 openConversationId。
- 判断服务端是否推到连接：临时单事件 `--debug --debug-raw-events`；它不能与 `--flatten`
  并用，排查后立即移除。
- 长期运行：交给宿主进程管理，不写历史消息轮询脚本。
