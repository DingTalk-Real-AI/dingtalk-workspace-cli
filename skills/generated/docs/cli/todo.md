# Canonical Product: todo

Generated from shared Tool IR. Do not edit by hand.

- Display name: 钉钉待办
- Description: 钉钉待办MCP服务提供高效的任务管理能力，支持创建待办事项、更新任务状态（如完成/未完成）、以及按条件查询待办列表。
- Server key: `b693ef3984e8d311`
- Endpoint: `https://mcp-gw.dingtalk.com/server/0f51140eddcd913106c5821a4d0cd577b2d1a0b6cb452dd0e51ab41facf3a83c`
- Protocol: `2025-03-26`
- Degraded: `false`

## Tools

- `task create`
  - Path: `todo.create_personal_todo`
  - CLI route: `dws todo task create`
  - Description: 在当前企业组织内创建一条个人待办事项，支持设置标题、执行人列表（用户 ID）、截止时间、优先级（如高/中/低）。待办将归属于当前用户，并对有权限的协作者可见。
  - Flags: `--due`, `--executors`, `--priority`, `--title`
  - Schema: `skills/generated/docs/schema/todo/create_personal_todo.json`
- `task delete`
  - Path: `todo.delete_todo`
  - CLI route: `dws todo task delete`
  - Description: 删除待办（所有执行者都删除）
  - Flags: `--task-id`
  - Schema: `skills/generated/docs/schema/todo/delete_todo.json`
- `task list`
  - Path: `todo.get_user_todos_in_current_org`
  - CLI route: `dws todo task list`
  - Description: 获取当前用户在所属组织中的个人待办事项列表，返回每项待办的标题、截止日期、优先级（如高/中/低）、完成状态。
  - Flags: `--page`, `--size`, `--status`
  - Schema: `skills/generated/docs/schema/todo/get_user_todos_in_current_org.json`
- `task get`
  - Path: `todo.query_todo_detail`
  - CLI route: `dws todo task get`
  - Description: 查询待办详情
  - Flags: `--task-id`
  - Schema: `skills/generated/docs/schema/todo/query_todo_detail.json`
- `task done`
  - Path: `todo.update_todo_done_status`
  - CLI route: `dws todo task done`
  - Description: 修改执行者的待办完成状态
  - Flags: `--status`, `--task-id`
  - Schema: `skills/generated/docs/schema/todo/update_todo_done_status.json`
- `task update`
  - Path: `todo.update_todo_task`
  - CLI route: `dws todo task update`
  - Description: 修改整个待办任务
  - Flags: `--due`, `--done`, `--priority`, `--title`, `--task-id`
  - Schema: `skills/generated/docs/schema/todo/update_todo_task.json`
