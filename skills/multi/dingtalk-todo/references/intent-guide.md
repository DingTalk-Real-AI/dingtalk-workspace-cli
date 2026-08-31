# Todo 局部意图消歧

| 用户说 | 应该用 | 不要用 | 边界 |
|---|---|---|---|
| “帮我记一下明天要做的事” | `todo +remind` | `doc` | 这是个人待办；`--at` 写截止时间 |
| “给自己留一个明天下午的时间块” | `calendar event create` | `todo` | 时间块属于日历事件 |
| “明早 9 点提醒我提交周报” | 先创建待办，再用 `todo +reminder --base-time customTime --at ...` | 把 `+remind --at` 当提醒 | 独立提醒与截止时间是两种资源字段 |
| “截止前 30 分钟提醒” | `todo +reminder --base-time dueTime --due-date-offset -30` | `calendar` | 待办必须先有截止时间 |
| “每天重复提醒我” | `todo task create --due ... --recurrence ...`，必要时再加 reminder | 只写 reminder | recurrence 管重复待办；reminder 管单条待办提醒规则 |
| “创建/改名/删除待办标签” | `dws todo tag create/update/delete` | `git tag`、通讯录标签、其他产品标签 | “待办标签”由 Todo 产品拥有；后续只传真实 `tagCode` |
| “提交日报/周报” | `dingtalk-misc` | `todo` | 日志产品不是个人待办 |
| “审批这个申请” | `dingtalk-misc` | `todo` | OA 审批任务与个人待办不同 |
| “把会议行动项建成待办” | 先走 `dingtalk-minutes` 取真实行动项，再进入 Todo 创建路线 | 凭标题猜行动项 | 来源证据归听记，任务对象归 Todo |

提醒写入目前没有对应的查询接口，成功响应只能证明服务端接受了写请求，不能声称已经读回核验规则。
