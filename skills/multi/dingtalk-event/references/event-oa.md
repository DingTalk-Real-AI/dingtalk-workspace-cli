# OA 个人审批事件

先读事件产品入口 [SKILL.md](../SKILL.md) 的命令规则、调用流和子进程契约。本参考覆盖当前公开的七个 OA 个人事件：审批实例发起、抄送、终止和完成，以及审批任务创建、完成和转交。

<!-- dws-intent: event.listen.oa -->实时监听审批事件必须使用 `dws event consume` 长连接，不要轮询 OA 待办或审批实例列表来模拟事件。

## Prerequisite

OA 个人事件使用当前用户 OAuth 登录态。未登录或 token 失效时，先执行：

```bash
dws auth login
```

非默认组织使用全局 `--profile <corpId 或 profile 名>`。事件范围始终是该 OAuth 用户相关的全部 OA 事件，不需要也不接受审批人、发起人或审批模板选择参数。

## Event catalog

| 事件码 | 订阅规则 | 接收语义 | 必填参数 |
|---|---|---|---|
| `user_oa_approval_task_created` | `all` | 审批任务创建，发送给审批人 | 无 |
| `user_oa_approval_task_finished` | `all` | 审批任务已完成 | 无 |
| `user_oa_approval_task_redirected` | `all` | 审批任务已转交 | 无 |
| `user_oa_approval_instance_started` | `all` | 审批实例已发起 | 无 |
| `user_oa_approval_instance_cc` | `all` | 审批实例到达抄送节点，发送给被抄送人 | 无 |
| `user_oa_approval_instance_terminated` | `all` | 审批实例已终止 | 无 |
| `user_oa_approval_instance_finished` | `all` | 审批实例完成，发送给审批单发起人 | 无 |

只承认上表 7 个 OA 事件码。CLI 为每个事件发送 `ruleType=all`、`filterRule={}` 的独立订阅请求；不要添加 `--user`、`--open-dingtalk-id`、`--group`、`--query` 或 `--filter-json`。

## Intent mapping

| 用户说 | 下一步 |
|---|---|
| “监听新的待我审批任务” / “有审批任务创建时通知我” | `dws event consume user_oa_approval_task_created --flatten -f ndjson` |
| “审批任务完成时通知我” | `dws event consume user_oa_approval_task_finished --flatten -f ndjson` |
| “审批任务被转交时通知我” | `dws event consume user_oa_approval_task_redirected --flatten -f ndjson` |
| “有审批单发起时通知我” | `dws event consume user_oa_approval_instance_started --flatten -f ndjson` |
| “有审批抄送给我时通知我” | `dws event consume user_oa_approval_instance_cc --flatten -f ndjson` |
| “有审批单终止时通知我” | `dws event consume user_oa_approval_instance_terminated --flatten -f ndjson` |
| “监听我发起的审批何时完成” / “审批实例完成时通知我” | `dws event consume user_oa_approval_instance_finished --flatten -f ndjson` |
| “同时监听全部已公开 OA 事件” | 一个 consume 放入七个 OA event key，不加目标或过滤参数 |
| “查看 OA 事件目录” | `dws event list --category oa` |
| “查看 OA 事件输出字段” | 对对应事件运行 `dws event schema <event_key> --flatten` |

三个审批任务事件分别表达任务已创建、已完成和已转交；四个审批实例事件分别表达实例已发起、到达抄送节点、已终止和已完成。`status` 和 `result` 保留服务端原值，不把当前样本值推断为完整枚举。

## Commands

查看稳定的扁平输出 schema：

```bash
dws event schema user_oa_approval_task_created --flatten
dws event schema user_oa_approval_task_finished --flatten
dws event schema user_oa_approval_task_redirected --flatten
dws event schema user_oa_approval_instance_started --flatten
dws event schema user_oa_approval_instance_cc --flatten
dws event schema user_oa_approval_instance_terminated --flatten
dws event schema user_oa_approval_instance_finished --flatten
```

单独监听一种事件：

```bash
dws event consume user_oa_approval_task_created --flatten -f ndjson
dws event consume user_oa_approval_task_finished --flatten -f ndjson
dws event consume user_oa_approval_task_redirected --flatten -f ndjson
dws event consume user_oa_approval_instance_started --flatten -f ndjson
dws event consume user_oa_approval_instance_cc --flatten -f ndjson
dws event consume user_oa_approval_instance_terminated --flatten -f ndjson
dws event consume user_oa_approval_instance_finished --flatten -f ndjson
```

同时监听七种事件：

```bash
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

多事件 consume 会为七个 event key 分别创建订阅和逻辑 consumer，并共享当前组织的 personal bus、远程连接、stdout 和生命周期。不要给 OA 命令加 `--query` 或 `--filter-json`；这两个 flag 只用于兼容的 IM 消息接收事件。

## Output contract

`--flatten` 模式的所有 OA 事件都包含以下顶层字段：

```json
{
  "type": "user_oa_approval_task_created",
  "event_id": "...",
  "timestamp": 0,
  "subscribe_id": "...",
  "process_instance_id": "...",
  "process_code": "...",
  "title": "...",
  "status": "RUNNING",
  "create_time": 0,
  "event_time": 0
}
```

- `type` 是当前 event key；`event_id` 可用于去重；`timestamp` 是 transport 事件发生时间；`subscribe_id` 标识对应的独立订阅。
- `process_instance_id` 是审批实例 ID，可传给 OA 审批命令的 `--instance-id`；`process_code` 是审批流程模板编码。
- `create_time`、`finish_time` 和 `event_time` 都是毫秒时间戳。`event_time` 是审批业务事件时间，`timestamp` 是 transport 事件时间。
- 七类事件的额外字段如下；具体事件始终以 `dws event schema <event_key> --flatten` 为准。

| 事件 | 额外顶层字段 |
|---|---|
| `user_oa_approval_task_created` | `task_id` |
| `user_oa_approval_task_finished` | `task_id`、`result`、`finish_time` |
| `user_oa_approval_task_redirected` | `task_id`、`result`、`finish_time` |
| `user_oa_approval_instance_started` | 无 |
| `user_oa_approval_instance_cc` | 无 |
| `user_oa_approval_instance_terminated` | `finish_time` |
| `user_oa_approval_instance_finished` | `result`、`finish_time` |

任务完成事件示例：

```json
{
  "type": "user_oa_approval_task_finished",
  "event_id": "...",
  "timestamp": 0,
  "subscribe_id": "...",
  "process_instance_id": "...",
  "process_code": "...",
  "task_id": "...",
  "title": "测试审批",
  "status": "FINISHED",
  "result": "agree",
  "create_time": 0,
  "finish_time": 0,
  "event_time": 0
}
```

- `task_id` 是当前审批任务 ID，可传给接受任务 ID 的 OA 审批命令。
- `status` 和 `result` 是服务端字符串；不要只根据当前样本把 `RUNNING/FINISHED/TERMINATED` 或 `agree/redirect` 写成封闭枚举。
- payload 缺失、为空、缺少对应事件的稳定 ID 或无法解析时，consume 会在 stderr 记录 warning，并把原始 transport envelope 写到 stdout，保证事件不被静默丢弃。
- 不传 `--flatten` 时保持兼容 transport envelope，业务 payload 位于 `.data | fromjson`。需要联调完整原始协议时使用不带 `--flatten` 的 `-f raw` 或 `--debug-raw-events`。

## Lifecycle

- 单事件等待 `[event] ready event_key=<key> bus_pid=<pid> subscribe_id=<id>`。
- 七事件先保存七条 `[event] subscription event_key=<key> subscribe_id=<id>`，再等待 `[event] ready event_count=7 bus_pid=<pid>`。
- 临时验证使用 `--max-events 1` 或 `--duration 10m`；任务完成后优雅结束 consume，本次新建的订阅会自动取消。
- 外部停止已有订阅时先运行 `dws event stop <subscribe_id> --dry-run`，确认后再加 `--yes`。不要 `kill -9`，否则会跳过自动退订。
