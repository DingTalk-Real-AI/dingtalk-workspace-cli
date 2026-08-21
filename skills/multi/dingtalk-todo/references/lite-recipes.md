# Todo 单步与短流程

用于单步 Todo 意图，也用于组合请求中被 shortcut 完整覆盖的独立步骤。涉及动态子资源 ID 或原子特有操作时，同时读 [02-task.md](02-task.md)。

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

后续只有搜索、详情回读和清理时可保留上述创建 shortcut；若还要列表筛选、更新状态/字段、提醒、评论、附件、成员、子待办、标签或创建多个对象，创建步骤改用原子 `task create`。成功结果必须含稳定 `taskId` 并完成读回。超时、缺少 ID 或 `verified!=true` 时先用搜索/列表对账，禁止重放非幂等创建。

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
