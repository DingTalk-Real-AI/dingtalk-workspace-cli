# Todo 单步与短流程

用于单步 Todo 意图，也用于组合请求中被 Shortcut 完整覆盖的独立步骤。组合请求先按 [02-task.md](02-task.md) 拆分；涉及列表筛选、资源写入、多对象或动态子资源 ID 时，不要用创建 Shortcut 代替原子 `task create`。

## 创建

### 给自己

```bash
dws todo +remind --task "提交周报" --at "2026-08-19T18:00:00+08:00" --format json
```

`--at` 是截止时间。需要 9 点弹出独立提醒时，在取得 `taskId` 后再执行：

```bash
dws todo +reminder --task-id <TASK_ID> --base-time customTime --at "2026-08-19T09:00:00+08:00" --format json
```

### 按姓名指派

```bash
dws todo +assign --to "张三" --task "周五前提交排期" --format json
dws todo +assign-multi --to "张三,李四" --task "周五前提交排期" --format json
```

姓名必须唯一解析。多人场景任一姓名失败时整条待办不会创建。

### 已有 userId

```bash
dws todo +create --title "修复线上问题" --executors <USER_ID> --priority 40 --format json
```

成功结果必须含稳定 `taskId` 且 `verified=true`。缺少 ID、写后读回失败或超时都不能重放创建；先搜索/列表对账。创建后还要按状态/优先级/角色/日期/页码列举，或更新、完成/重开、提醒、评论、附件、成员、子待办、标签，或创建多个对象时，创建步骤改用原子 `task create`。

## 查询与定位

```bash
dws todo +get-my-tasks --all --status false --format json
dws todo +get-related-tasks --format json
dws todo +search --query "周报" --format json
dws todo +get --task-id <TASK_ID> --format json
```

- list 用于枚举，search 用于标题关键词，get 用于已知稳定 ID。
- 空集合是正常成功；响应结构缺失、分页未耗尽或 ID 缺失必须失败。
- 零匹配或多匹配时不得自行选择第一条。
- 用户明确要求状态、优先级、角色、日期或页码筛选时，使用 `todo task list` 的对应 flag；不要先拉取无关数据再在本地猜范围。

## 完成、重开与更新

```bash
dws todo +complete --task-id <TASK_ID> --format json
dws todo +reopen --task-id <TASK_ID> --format json
dws todo +update --task-id <TASK_ID> --title "新标题" --format json
```

只记得标题时用 `+todo-done --task "<关键词>"`；它只会在唯一命中时修改。单次变更优先使用这些自带读回的 Shortcut；需要中间列表、多对象对比或原子特有参数时切到 [组合生命周期](02-task.md) 的原子路线。

## 汇总与批量

```bash
python scripts/todo_daily_summary.py today
python scripts/todo_overdue_check.py
python scripts/todo_batch_create.py todos.json --dry-run
```

批量脚本单批最多 30 条。先用 dry-run 展示精确批次和稳定 `planDigest`；取得用户对该摘要的明确确认后，执行时必须同时传 `--confirm-digest <PLAN_DIGEST>` 与 `--yes`。脚本重新规范化输入并核对摘要，不匹配时零调用拒绝；匹配后只把 Runtime 确认传给逐条创建，回读不携带该标记。输出保留逐项 ledger；`unknown` 表示写可能已提交，`unverified` 表示已有 `taskId` 但读回未通过，两者都需对账，不能自动重试。
