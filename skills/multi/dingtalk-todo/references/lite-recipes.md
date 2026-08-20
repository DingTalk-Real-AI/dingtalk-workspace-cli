# Todo 单步与短流程

只在请求是单一 Todo 意图时使用本文件；创建后还要继续操作资源时，改读 [02-task.md](02-task.md)。

## 创建一条待办

给自己：

```bash
dws todo +remind --task "<标题>" [--at "<截止ISO>"] --format json
```

按姓名指派：

```bash
dws todo +assign --to "<姓名>" --task "<标题>" --format json
dws todo +assign-multi --to "<姓名1>,<姓名2>" --task "<标题>" --format json
```

已经有真实 `userId`：

```bash
dws todo +create --title "<标题>" --executors <USER_ID> [--priority 10|20|30|40] [--due "<截止ISO>"] --format json
```

成功结果必须含稳定 `taskId` 并完成读回。超时、缺少 ID 或 `verified!=true` 时先用搜索/列表对账，禁止重放非幂等创建。

## 查询与定位

```bash
dws todo +get-my-tasks --all --status false --format json
dws todo +get-related-tasks --format json
dws todo +search --query "<标题关键词>" --format json
dws todo +get --task-id <TASK_ID> --format json
```

- list 用于枚举，search 用于标题关键词，get 用于已知稳定 ID。
- 空集合是成功；零匹配或多匹配时不得自行选第一条。
- 用户明确要求状态、优先级、角色、日期或页码筛选时，使用 `todo task list` 的对应 flag，不要拉全后猜。

## 完成、重开与更新

```bash
dws todo +complete --task-id <TASK_ID> --format json
dws todo +reopen --task-id <TASK_ID> --format json
dws todo +update --task-id <TASK_ID> --title "<新标题>" --format json
```

只记得标题时用 `+todo-done --task "<关键词>"`；它只在唯一命中时修改。

## 提醒、汇总与批量

- `+remind --at` 设置截止时间；独立提醒使用 `+reminder --base-time customTime --at "<提醒ISO>"`。
- 截止前提醒使用 `+reminder --base-time dueTime --due-date-offset -30`，待办必须已有截止时间。
- 今天/明天/本周汇总：`python scripts/todo_daily_summary.py today|tomorrow|week`。
- 逾期扫描：`python scripts/todo_overdue_check.py`。
- 批量创建：`python scripts/todo_batch_create.py <todos.json>`；单批最多 30 条，以逐项 ledger 为准。
