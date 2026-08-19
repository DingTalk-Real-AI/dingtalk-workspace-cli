# 任务管理

> **SKILL.md** 中 #2 仅内联 **lite**：`create-todo`、`todo-query-ops`。其中 `todo-query-ops` 统一覆盖 list/get/complete/reopen/topic-filter。下列 recipe 已迁出速查表，命中时读本文件对应行。重型 **full** 见下表「行动指南」。命令细节见 [todo.md](./todo.md)。

## Recipe 速查（非 SKILL lite）

| Recipe | 步骤（命令均须 `--format json`，下略）                                                                                                                                                                                                                        |
|--------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `create-priority-todo` | 1. 确定执行者（同 [SKILL.md](../SKILL.md) 中 `create-todo` 步骤 1）<br>2. `todo task create --title "<标题>" --executors <userId>[,<userId2>...] --priority <10/20/30/40>`（可选 `--due "<截止ISO>"`；10低/20普通/30较高/40紧急）→ 取 `todoTaskId`<br>3. `todo task get --task-id <todoTaskId>` 回读标题、执行者、优先级和截止时间 |
| `create-recurring-todo` | 1. 确定执行者（同 `create-todo` 步骤 1）<br>2. `todo task create --title "<标题>" --executors <userId> --due "<首次截止ISO>" --priority <10/20/30/40> --recurrence "DTSTART:<UTC时间>\nRRULE:FREQ=DAILY;INTERVAL=1"`（`--due` 必填；仅支持按天循环，见 [todo.md](./todo.md)）→ 取 `todoTaskId`<br>3. `todo task get --task-id <todoTaskId>` 回读循环规则和任务字段 |
| `reschedule-todo` | 1. `todo task list --status false` → 取 `todoTaskId`<br>2. `todo task update --task-id <todoTaskId> --due "<新截止时间>"`                                                                                                                                |

## Full / 组合（固定路线）

| Recipe | 行动指南（固定路线）                                                                                                                                                                                                                                                                                                                                                                                    |
|--------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| generate-progress-report | 1. 按[「多源并行采集」](recipes/conventions.md#多源并行采集公共模式)执行<br>2. 交叉比对各源数据<br>3. `doc create --name "<报告名>" --content "<报告内容>"`                                                                                                                                                                                                                                                                    |
| batch-create-todo | 1. 按[「多源并行采集」](recipes/conventions.md#多源并行采集公共模式)执行 → 从结果提取任务条目<br>2. 每条：`aisearch person --query "<姓名>" --dimension name` → 取真实 `userId`；同名时先消歧<br>3. 逐条执行 `todo task create --title "<标题>" --executors <userId> --priority <10/20/30/40>`，从每次响应收集真实 `todoTaskId`；单批超 30 条须用户确认<br>4. 对全部 `todoTaskId` 并行执行 `todo task get --task-id <todoTaskId>`，逐项核对标题、执行者、优先级和截止时间；不能只以退出码或创建响应作为成功证据 |
| assign-and-notify | 1. `aisearch person --query "<姓名>" --dimension name` → 取 `userId`<br>2. `todo task create --title "<标题>" --executors <userId> --priority <10/20/30/40>` → 取 `todoTaskId`<br>3. `todo task get --task-id <todoTaskId>` 回读任务，确认无误后再通知<br>4. `chat search --query "<群名>"` → 取 `openConversationId` → `chat message send --conversation-id <openConversationId> --content "<通知内容>"` |
