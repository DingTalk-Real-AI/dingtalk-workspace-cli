# 好友事件

先读事件产品入口 [SKILL.md](../SKILL.md) 的命令规则、调用流和子进程契约。本参考覆盖当前公开的两个好友个人事件：收到好友申请、好友添加成功。

<!-- dws-intent: event.listen.contact -->实时监听好友事件必须使用 `dws event consume` 长连接，不要轮询好友列表或好友申请列表来模拟事件。

## Prerequisite

好友个人事件使用当前用户 OAuth 登录态。未登录或 token 失效时，先执行：

```bash
dws auth login
```

非默认组织使用全局 `--profile <corpId 或 profile 名>`。事件范围始终是该 OAuth 用户相关的好友事件，不需要也不接受目标用户、组织或消息过滤参数。

## Event catalog

| 事件码 | 订阅规则 | 接收语义 | 必填参数 |
|---|---|---|---|
| `user_contact_friend_request_received` | `all` | 当前用户收到好友申请 | 无 |
| `user_contact_friend_added` | `all` | 当前用户与目标用户建立好友关系 | 无 |

只承认上表 2 个好友事件码。CLI 为每个事件发送 `ruleType=all`、`filterRule={}` 的独立订阅请求；不要添加 `--user`、`--open-dingtalk-id`、`--group`、`--query`、`--role-types` 或 `--filter-json`。

## Intent mapping

| 用户说 | 下一步 |
|---|---|
| “收到好友申请时通知我” | `dws event consume user_contact_friend_request_received --flatten -f ndjson` |
| “有人加我好友时通知我” | `dws event consume user_contact_friend_request_received --flatten -f ndjson` |
| “好友添加成功时通知我” | `dws event consume user_contact_friend_added --flatten -f ndjson` |
| “同时监听全部好友事件” | 一个 consume 放入两个好友 event key，不加目标或过滤参数 |
| “查看好友事件目录” | `dws event list --category contact` |
| “查看好友事件输出字段” | 对对应事件运行 `dws event schema <event_key> --flatten` |

## Commands

查看稳定的扁平输出 schema：

```bash
dws event schema user_contact_friend_request_received --flatten
dws event schema user_contact_friend_added --flatten
```

单独监听一种事件：

```bash
dws event consume user_contact_friend_request_received --flatten -f ndjson
dws event consume user_contact_friend_added --flatten -f ndjson
```

同时监听两种事件：

```bash
dws event consume \
  user_contact_friend_request_received \
  user_contact_friend_added \
  --flatten \
  -f ndjson
```

多事件 consume 会为两个 event key 分别创建订阅和逻辑 consumer，并共享当前组织的 personal bus、远程连接、stdout 和生命周期。不要给好友事件命令加 `--query` 或 `--filter-json`。

## Output contract

`--flatten` 模式的两个好友事件都包含以下顶层字段：

```json
{
  "type": "user_contact_friend_request_received",
  "event_id": "...",
  "timestamp": 0,
  "subscribe_id": "..."
}
```

- `type` 是当前 event key；`event_id` 可用于去重；`timestamp` 是 transport 事件发生时间；`subscribe_id` 标识对应的独立订阅。
- 收到好友申请事件额外提供 `src_open_dingtalk_id`、`src_name`、`dest_open_dingtalk_id`、`remark`、`source`、`biz_type`、`apply_time`。
- 好友添加成功事件额外提供 `friend_open_dingtalk_id`、`friend_name`、`establish_time`、`direction`。
- `direction` 表示当前用户视角：`active` 表示当前用户主动添加对方，`passive` 表示对方主动添加当前用户。
- `apply_time`、`establish_time` 是毫秒时间戳。
- 服务端 payload 缺失、为空或无法解析时，consume 会在 stderr 记录 warning，并把原始 transport envelope 写到 stdout，保证事件不被静默丢弃。
- 不传 `--flatten` 时保持兼容 transport envelope，业务 payload 位于 `.data | fromjson`。需要联调完整原始协议时使用不带 `--flatten` 的 `-f raw` 或 `--debug-raw-events`。

## Lifecycle

- 单事件等待 `[event] ready event_key=<key> bus_pid=<pid> subscribe_id=<id>`。
- 两事件先保存两条 `[event] subscription event_key=<key> subscribe_id=<id>`，再等待 `[event] ready event_count=2 bus_pid=<pid>`。
- 临时验证使用 `--max-events 1` 或 `--duration 10m`；任务完成后优雅结束 consume，本次新建的订阅会自动取消。
- 外部停止已有订阅时先运行 `dws event stop <subscribe_id> --dry-run`，确认后再加 `--yes`。不要 `kill -9`，否则会跳过自动退订。
