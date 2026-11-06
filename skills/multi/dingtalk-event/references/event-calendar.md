# 日历个人事件

<!-- dws-intent: event.listen.calendar -->实时日程变化使用 `dws event consume` 个人事件长连接。当前事件码为
`user_calendar_event_changed`，支持主日历创建及新增参与 `eventCreated`、修改
`eventUpdated`、取消 `eventCancelled`。创建覆盖普通日程、循环主日程，以及邀请或主动
加入产生的新增参与用户；不补发历史。其他日历本和循环例外等范围以服务端实际支持为准。
查询、创建、修改日程仍使用 `dws calendar`。取消日程通知与停止事件订阅是两个操作。

## 发现与监听

```bash
dws event list --category calendar --format json
dws event schema user_calendar_event_changed --flatten --format json
dws event consume user_calendar_event_changed --calendar-ids primary --change-types eventCreated,eventUpdated,eventCancelled --flatten --duration 10m -f ndjson
```

使用目标用户的 OAuth 登录态与同一 `--profile`。等待 stderr 的 `[event] ready` 后再触发
测试日程；stdout 只有事件。时限到达或 SIGTERM 干净退出会取消本次新建订阅，复用的
`--subscribe-id` 保留；停止已有订阅先用 `event stop <subscribe_id> --dry-run` 预览。

## 过滤规则

`--calendar-ids` 和 `--change-types` 为逗号分隔的数组，规范化后由既有订阅控制面把
JSON 对象序列化为 **String 类型**的 `filterRule`：

```json
{"calendarIds":["primary"],"changeTypes":["eventCancelled","eventCreated","eventUpdated"]}
```

- 省略其中一个维度，该维度通配；全部省略下发 `{}`。通配可自然覆盖服务端后续能力。
- 显式 `--calendar-ids=` 或 `--change-types=` 表示空数组，不匹配任何消息。
- `*` 不可和具体值混用。数组按集合规范化，重复值和顺序不影响订阅请求的幂等键。
- `primary` 表示目标用户主日历；其他值必须是有查看详情权限的 OpenAPI 日历本 ID，
  不要传内部 folderId 或 uid。能保存过滤范围不表示本期会生产非主日历事件。
- 具体变化类型接受 `eventCreated`、`eventUpdated`、`eventCancelled`，可单独或组合订阅。
  只传 `eventCreated` 的旧订阅仍只匹配创建及新增参与；要接收修改和取消，应新建订阅并
  扩展过滤或省略 `--change-types`。不要用 `eventDeleted` 代替 `eventCancelled`。
- 不接受 `--user`、`--open-dingtalk-id`、`--group`、`--role-types`、`--query` 或
  `--filter-json`。过滤参数不能覆盖已有 `--subscribe-id` 的范围，也不能与其他业务
  事件混用；没有业务过滤时可和其他兼容事件共享 bus。

## 扁平输出

| 字段 | 含义 |
|---|---|
| `type` | 固定为事件码 |
| `event_id` / `timestamp` | 事件中心事件 ID 和时间 |
| `subscribe_id` | 本次接收的个人事件订阅 ID |
| `biz_id` | 业务投递唯一 ID；按此幂等，重试不变 |
| `event_time` | 业务时间戳（毫秒），重试不变，不表示消息顺序 |
| `change_type` | `eventCreated`、`eventUpdated`、`eventCancelled`；保留未来类型 |
| `calendar_id` | OpenAPI 日历本 ID，`primary` 按目标用户解释 |
| `calendar_event_id` | OpenAPI 日程资源 ID；取消后可能已不可查询 |
| `operator_uid` | 可选操作人内部 uid；源载荷字符串或整数均输出为字符串，缺失时省略 |

`calendar_event_id` 不能用于投递去重：同一日程可以命中多个订阅并产生不同的
`biz_id`。消息可能重复或乱序；按资源 ID 通过已授权的日历 API 查询最新状态。
收到 `eventCancelled` 时记录该资源已取消；详情可能不存在，不把无法查询详情视为通知失败。
不要把接收人当操作人。缺失或错误的必填业务字段会输出安全的基础身份并告警，
不把原始路由 uid/orgId 回退到扁平结果。默认不加 `--flatten` 时仍保留 transport envelope。

CLI 使用现有个人事件订阅与 Stream 接口。客户端目录中的 enabled/public 只表示客户端支持，不证明服务端已部署或事件投递已验证。事件开通、接收范围及三种变化类型的端到端投递都需要在目标环境单独验收。

取消后仍可能收到在途消息；重新订阅使用平台生成的新订阅 ID，不手工复用已取消 ID。
