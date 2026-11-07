# Todo 待办事件

<!-- dws-intent: event.listen.todo -->待办创建、更新或删除的实时变化使用 `dws event consume` 长连接；查询、创建、更新、完成或删除已有待办使用 `dws todo`，不要轮询待办列表模拟事件。

## EventKey 与订阅范围

| EventKey | 触发时机 | 典型入口 |
|---|---|---|
| `user_todo_task_create` | 当前用户相关的待办被创建 | `dws event consume user_todo_task_create --role-types executor --flatten -f ndjson` |
| `user_todo_task_update` | 当前用户相关的待办被更新 | `dws event consume user_todo_task_update --role-types executor --flatten -f ndjson` |
| `user_todo_task_delete` | 当前用户相关的待办被删除 | `dws event consume user_todo_task_delete --role-types executor --flatten -f ndjson` |

`--role-types` 只接受 `creator`、`executor`、`participant`，可用逗号组合。省略时订阅三种角色的并集；例如只监听自己创建的待办使用 `--role-types creator`，只监听指派给自己执行的待办使用 `--role-types executor`。

CLI 将角色范围规范化后下发为：

```json
{"roleTypes":["creator","executor","participant"]}
```

该对象就是订阅控制面的 `filterRule`。Todo 事件不接受 `--user`、`--open-dingtalk-id`、`--group`、`--query` 或 `--filter-json`；这些参数不会替代 `roleTypes`。

三个 EventKey 可以在一个 consume 进程中共享相同的角色范围：

```bash
dws event consume \
  user_todo_task_create \
  user_todo_task_update \
  user_todo_task_delete \
  --role-types executor \
  --flatten --format ndjson
```

## 扁平输出

三类事件都提供 `type`、`event_id`、`timestamp`、`subscribe_id`、`task_id`、`subject`、`creator_id` 和 `create_time`。

- 创建与更新还提供 `executor_ids`、`participant_ids`、`priority`、`status_stage`、`plan_start_date`、`plan_finish_date`、`start_date`、`finish_date`、`description`、`source`、`source_id`、`biz_tag`、`parent_id`、`is_multi_executor` 和 `scene_type`。
- 更新额外提供 `update_time` 与 `old_status_stage`。
- 删除额外提供 `delete_time`。
- `status_stage`：`0` 未开始、`1` 进行中、`2` 正常完成、`3` 异常完成。
- 服务端返回 `null` 的可选时间或 `parent_id` 在扁平输出中省略。字段契约以 `dws event schema <event_key> --flatten` 为准。

处理事件后若要读取完整待办或执行操作，把真实 `task_id` 交给 `dws todo` 对应命令；不要从标题猜任务 ID。事件监听本身不会创建、修改、完成或删除待办。

## 生命周期与后端边界

- 单事件等待 `[event] ready event_key=<key> bus_pid=<pid> subscribe_id=<id>`；三事件组合保存每条 subscription 后等待 `[event] ready event_count=3 bus_pid=<pid>`。
- 有界消费使用 `--max-events` 或 `--duration`；结束时使用 SIGTERM、关闭符合条件的管道 stdin，或让 bounded consume 自行退出。
- DWS 订阅 SPI 路由到 `lippi-tdp` 的 `com.dingtalk.todo.service.TodoDwsSubscriptionService`，版本 `1.0.0`、协议组 `HSF`。CLI 不直连 HSF；订阅或取消失败时保留服务端错误与 trace 信息，不换 EventKey 或 subscribe_id 绕过重试保护。
