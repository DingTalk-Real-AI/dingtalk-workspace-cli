# 听记事件

先读事件产品入口 [SKILL.md](../SKILL.md) 的命令规则、调用流和子进程契约。本参考覆盖当前公开的两个听记个人事件：创建听记、纪要生成。

<!-- dws-intent: event.listen.minutes -->实时监听听记创建或纪要生成必须使用 `dws event consume` 长连接，不要轮询听记列表或反复读取纪要来模拟事件。

## Prerequisite

听记个人事件使用当前用户 OAuth 登录态。未登录或 token 失效时，先执行：

```bash
dws auth login
```

非默认组织使用全局 `--profile <corpId 或 profile 名>`。事件范围由服务端按订阅和读取权限决定，不需要也不接受目标用户、群、角色或消息过滤参数。

## Event catalog

| 事件码 | 订阅规则 | 接收语义 | 必填参数 |
|---|---|---|---|
| `user_minutes_task_created` | `all` | 听记对象、必要关联及默认权限初始化成功；创建失败、仅受理请求、复用已有听记不通知 | 无 |
| `user_minutes_summary_generated` | `all` | 默认纪要首次生成成功、保存完成且可读取；成功后重新生成、切换模板、人工修改不通知 | 无 |

只承认上表 2 个听记事件码。CLI 为每个事件发送 `ruleType=all`、`filterRule={}` 的独立订阅请求；不要添加 `--user`、`--open-dingtalk-id`、`--group`、`--query`、`--role-types` 或 `--filter-json`。

## Intent mapping

| 用户说 | 下一步 |
|---|---|
| “新建听记时通知我” | `dws event consume user_minutes_task_created --flatten -f ndjson` |
| “纪要生成好了通知我” / “会议纪要出来后处理” | `dws event consume user_minutes_summary_generated --flatten -f ndjson` |
| “同时监听全部听记事件” | 一个 consume 放入两个听记 event key，不加目标或过滤参数 |
| “查看听记事件目录” | `dws event list --category minutes` |
| “查看听记事件输出字段” | 对对应事件运行 `dws event schema <event_key> --flatten` |

## Commands

查看稳定的扁平输出 schema：

```bash
dws event schema user_minutes_task_created --flatten
dws event schema user_minutes_summary_generated --flatten
```

单独监听一种事件：

```bash
dws event consume user_minutes_task_created --flatten -f ndjson
dws event consume user_minutes_summary_generated --flatten --duration 10m -f ndjson
```

同时监听两种事件：

```bash
dws event consume \
  user_minutes_task_created \
  user_minutes_summary_generated \
  --flatten \
  -f ndjson
```

多事件 consume 会为两个 event key 分别创建订阅和逻辑 consumer，并共享当前组织的 personal bus、远程连接、stdout 和生命周期。

## Output contract

`--flatten` 模式的两个听记事件字段相同：

```json
{
  "type": "user_minutes_summary_generated",
  "event_id": "...",
  "timestamp": 0,
  "subscribe_id": "...",
  "biz_id": "...",
  "event_time": 0,
  "schema_version": 1,
  "task_uuid": "...",
  "creator_open_dingtalk_id": "10001"
}
```

- `type`、`event_id`、`timestamp`、`subscribe_id` 是平台传输元数据；`event_id` 不假定等于 `biz_id`。
- `biz_id`、`event_time` 由生产方生成，同一次业务事件重试保持不变，消费端需按 `biz_id` 幂等处理。事件可能重复或乱序，不要等待创建事件才处理纪要事件。
- `schema_version` 首版为 `1`；`task_uuid` 是听记稳定标识。
- `creator_open_dingtalk_id` 是创建者标识：正 int64 转为十进制字符串以保持精度，非空字符串原样保留，缺失、null、0、负数、布尔值视为非法；它不一定是当前接收人，与内部 uid 的映射以平台确认结果为准。
- 事件只携带标识，不含听记或纪要正文。创建事件不表示纪要已就绪；需要内容时用同一 `--profile` 执行 `dws minutes get info --id <task_uuid>` 或 `dws minutes get summary --id <task_uuid>`。CLI 不自动读取内容或执行业务动作。
- payload 缺失、为空、无法解析，或 `schemaVersion` 不为 1、`taskUuid` 为空、`creatorOpenDingtalkId` 非法时，consume 会在 stderr 记录 warning（同一进程只提示一次），并把原始 transport envelope 写到 stdout，保证事件不被静默丢弃。
- 不传 `--flatten` 时保持兼容 transport envelope，业务 payload 位于 `.data | fromjson`。

## Lifecycle

- 单事件等待 `[event] ready event_key=<key> bus_pid=<pid> subscribe_id=<id>`。
- 两事件先保存两条 `[event] subscription event_key=<key> subscribe_id=<id>`，再等待 `[event] ready event_count=2 bus_pid=<pid>`。
- 不补推订阅前的历史事件；取消后停止新投递，已进入下游的消息仍可能到达。
- 临时验证使用 `--max-events 1` 或 `--duration 10m`；任务完成后优雅结束 consume，本次新建的订阅会自动取消。
- 外部停止已有订阅时先运行 `dws event stop <subscribe_id> --dry-run`，确认后再加 `--yes`。不要 `kill -9`，否则会跳过自动退订。
