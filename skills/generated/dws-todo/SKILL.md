---
name: dws-todo
description: "钉钉待办MCP服务提供高效的任务管理能力，支持创建待办事项、更新任务状态（如完成/未完成）、以及按条件查询待办列表。"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws todo --help"
---

# todo

> **PREREQUISITE:** Read `../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

- Display name: 钉钉待办
- Description: 钉钉待办MCP服务提供高效的任务管理能力，支持创建待办事项、更新任务状态（如完成/未完成）、以及按条件查询待办列表。
- Endpoint: `https://mcp-gw.dingtalk.com/server/0f51140eddcd913106c5821a4d0cd577b2d1a0b6cb452dd0e51ab41facf3a83c`
- Protocol: `2025-03-26`
- Degraded: `false`

```bash
dws todo <group> <command> --json '{...}'
```

## Helper Commands

| Command | Tool | Description |
|---------|------|-------------|
| [`dws-todo-task-create`](./task/create.md) | `create_personal_todo` | 在当前企业组织内创建一条个人待办事项，支持设置标题、执行人列表（用户 ID）、截止时间、优先级（如高/中/低）。待办将归属于当前用户，并对有权限的协作者可见。 |
| [`dws-todo-task-delete`](./task/delete.md) | `delete_todo` | 删除待办（所有执行者都删除） |
| [`dws-todo-task-list`](./task/list.md) | `get_user_todos_in_current_org` | 获取当前用户在所属组织中的个人待办事项列表，返回每项待办的标题、截止日期、优先级（如高/中/低）、完成状态。 |
| [`dws-todo-task-get`](./task/get.md) | `query_todo_detail` | 查询待办详情 |
| [`dws-todo-task-done`](./task/done.md) | `update_todo_done_status` | 修改执行者的待办完成状态 |
| [`dws-todo-task-update`](./task/update.md) | `update_todo_task` | 修改整个待办任务 |

## API Tools

### `create_personal_todo`

- Canonical path: `todo.create_personal_todo`
- CLI route: `dws todo task create`
- Description: 在当前企业组织内创建一条个人待办事项，支持设置标题、执行人列表（用户 ID）、截止时间、优先级（如高/中/低）。待办将归属于当前用户，并对有权限的协作者可见。
- Required fields: `PersonalTodoCreateVO.executorIds`, `PersonalTodoCreateVO.subject`
- Sensitive: `false`

### `delete_todo`

- Canonical path: `todo.delete_todo`
- CLI route: `dws todo task delete`
- Description: 删除待办（所有执行者都删除）
- Required fields: `taskId`
- Sensitive: `true`

### `get_user_todos_in_current_org`

- Canonical path: `todo.get_user_todos_in_current_org`
- CLI route: `dws todo task list`
- Description: 获取当前用户在所属组织中的个人待办事项列表，返回每项待办的标题、截止日期、优先级（如高/中/低）、完成状态。
- Required fields: `pageNum`, `pageSize`
- Sensitive: `false`

### `query_todo_detail`

- Canonical path: `todo.query_todo_detail`
- CLI route: `dws todo task get`
- Description: 查询待办详情
- Required fields: `taskId`
- Sensitive: `false`

### `update_todo_done_status`

- Canonical path: `todo.update_todo_done_status`
- CLI route: `dws todo task done`
- Description: 修改执行者的待办完成状态
- Required fields: `isDone`, `taskId`
- Sensitive: `false`

### `update_todo_task`

- Canonical path: `todo.update_todo_task`
- CLI route: `dws todo task update`
- Description: 修改整个待办任务
- Required fields: `TodoUpdateRequest.taskId`
- Sensitive: `false`

## Discovering Commands

```bash
dws schema                       # list available products (JSON)
dws schema todo                     # inspect product tools (JSON)
dws schema todo.<tool>              # inspect tool schema (JSON)
```
