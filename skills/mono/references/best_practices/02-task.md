# 任务管理

> **SKILL.md** 中 #2 仅内联 **lite**：`create-todo`、`list-todo`、`get-todo-detail`、`update-todo-status`、`query-todo-by-topic`。其中 `list-todo` 统一覆盖 open/completed/all（`--status false|true` 或不传），`update-todo-status` 统一覆盖 complete/reopen（`--status true|false`）。下列 recipe 已迁出速查表，命中时读本文件对应行。重型 **full** 见下表「行动指南」。命令细节见 [todo.md](../products/todo.md)。

## Recipe 速查（非 SKILL lite）

| Recipe | 步骤（命令均须 `--format json`，下略） |
|--------|----------------------------------------|
| `create-priority-todo` | 1. 确定执行者（同 [SKILL.md](../../SKILL.md) 中 `create-todo` 步骤 1）<br>2. `todo task create --title "<标题>" --executors <userId>[,<userId2>...] --priority <10/20/30/40>`（可选 `--due "<截止ISO>"`；10低/20普通/30较高/40紧急）→ 取 `todoTaskId` |
| `create-recurring-todo` | 1. 确定执行者（同 `create-todo` 步骤 1）<br>2. `todo task create --title "<标题>" --executors <userId> --due "<首次截止ISO>" --recurrence "DTSTART:<UTC时间>\nRRULE:FREQ=DAILY;INTERVAL=1"`（`--due` 必填；仅支持按天循环，见 [todo.md](../products/todo.md)）→ 取 `todoTaskId` |
| `reschedule-todo` | 1. `todo task list --status false` → 取 `todoTaskId`<br>2. `todo task update --task-id <todoTaskId> --due "<新截止时间>"` |

## Full / 组合（固定路线）

| Recipe | 行动指南（固定路线） |
|--------|---------------------|
| generate-progress-report | 1. 按[「多源并行采集」](_common/conventions.md#多源并行采集公共模式)执行<br>2. 交叉比对各源数据<br>3. `doc create --name "<报告名>" --content "<报告内容>"` |
| batch-create-todo | 1. 按[「多源并行采集」](_common/conventions.md#多源并行采集公共模式)执行 → 从结果提取任务条目<br>2. 每条：`aisearch person --keyword "<姓名>" --dimension name` → 取 `userId`<br>3. **优先**：将待办写入 `todos.json`（格式见 [todo_batch_create.py](../../scripts/todo_batch_create.py)），执行 `python scripts/todo_batch_create.py todos.json`<br>备选：逐条 `todo task create --title "<标题>" --executors <userId>` → 汇总回显<br>**单批超 30 条须用户确认** |
| assign-and-notify | 1. `aisearch person --keyword "<姓名>" --dimension name` → 取 `userId`<br>2. `todo task create --title "<标题>" --executors <userId>` → 取 `todoTaskId`<br>3. `chat search --query "<群名>"` → 取 `openConversationId` → `chat message send --group <openConversationId> --text "<通知内容>"` 通知 |
