# 待办 (todo) 命令参考

## 命令总览

### 创建待办
```
Usage:
  dws todo task create [flags]
Example:
  dws todo task create --title "修复线上Bug" --executors <USER_ID_1>,<USER_ID_2> --priority 40
  dws todo task create --title "提交报告" --executors <USER_ID> --due "2026-03-20T10:00:00+08:00"
  dws todo task create --title "每日站会记录" --executors <USER_ID> \
    --due "2026-03-10T10:00:00+08:00" \
    --recurrence $'DTSTART:20260310T100000\nRRULE:FREQ=DAILY;INTERVAL=1'
Flags:
      --title string        待办标题 (必填)
      --executors string    执行者 userId 列表 (必填)
      --due string          截止时间 ISO-8601 (如 2026-03-10T18:00:00+08:00)
      --priority string     优先级: 10低/20普通/30较高/40紧急
      --recurrence string   循环待办规则（需先设置 --due）；格式：DTSTART:...\nRRULE:FREQ=DAILY;INTERVAL=1
```

### 查询待办列表
```
Usage:
  dws todo task list [flags]
Example:
  dws todo task list --page 1 --size 20 --status false
Flags:
      --page string     页码 (默认 1)
      --size string     每页数量 (默认 20)
      --status string   true=已完成, false=未完成
```

### 修改待办任务
```
Usage:
  dws todo task update [flags]
Example:
  dws todo task update --task-id <taskId> --title "新标题"
  dws todo task update --task-id <taskId> --priority 40 --due "2026-03-10T18:00:00+08:00"
  dws todo task update --task-id <taskId> --done true
Flags:
      --done string       完成状态: true/false
      --due string        截止时间 ISO-8601 (如 2026-03-10T18:00:00+08:00)
      --priority string   优先级: 10低/20普通/30较高/40紧急
      --task-id string    待办任务 ID (必填)
      --title string      新标题
```

### 修改执行者的待办完成状态
```
Usage:
  dws todo task done [flags]
Example:
  dws todo task done --task-id <taskId> --status true
  dws todo task done --task-id <taskId> --status false
Flags:
      --status string    完成状态: true=已完成, false=未完成 (必填)
      --task-id string   待办任务 ID (必填)
```

### 待办详情
```
Usage:
  dws todo task get [flags]
Example:
  dws todo task get --task-id <taskId>
Flags:
      --task-id string   待办任务 ID (必填)
```

### 删除待办
```
Usage:
  dws todo task delete [flags]
Example:
  dws todo task delete --task-id <taskId>
  dws todo task delete --task-id <taskId> --yes
Flags:
      --task-id string   待办任务 ID (必填)
```

### 获取待办详情（顶级，兼容接口）
```
Usage:
  dws todo get-todo-detail [flags]
```
> 顶级 `get-todo-detail` 与 `task get` 功能重叠；agent 优先用 `task get`。

## 意图判断

用户说"加个待办/记一下/TODO" → `task create`
用户说"看看待办/我有啥要做" → `task list`
用户说"改个待办/修改待办标题/改优先级" → `task update`
用户说"做完了/完成待办/标记完成" → `task done`
用户说"看看待办详情" → `task get`
用户说"删除待办/取消待办" → `task delete`
用户说"循环待办/每天/每周提醒我" → `task create --recurrence`（需同时 `--due`）

关键区分: todo(个人待办)

## 核心工作流

```bash
# 1. 创建待办 — 提取 todoTaskId
dws todo task create --title "修复线上Bug" --executors userId1,userId2 \
  --priority 40 --due "2026-03-10T18:00:00+08:00" --format json

# 2. 查看未完成待办
dws todo task list --page 1 --size 20 --status false --format json

# 3. 查看待办详情
dws todo task get --task-id <taskId> --format json

# 4. 修改待办信息
dws todo task update --task-id <taskId> --title "新标题" --priority 40 --format json

# 5. 标记待办完成
dws todo task done --task-id <taskId> --status true --format json

# 6. 删除待办
dws todo task delete --task-id <taskId> --yes --format json
```

## 上下文传递表

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `task create` | `todoTaskId` | update/done/get/delete 的 --task-id |
| `task list` | `result[].id` | update/done/get/delete 的 --task-id |

## 注意事项

- 优先级值: 10=低, 20=普通, 30=较高, 40=紧急
- `--due` 截止时间使用 ISO-8601 格式（如 2026-03-10T18:00:00+08:00）
- `task list` 的 `--status` 对应 MCP `get_user_todos_in_current_org` 的 `todoStatus` 参数
- todo 是个人待办管理产品
- `task update` 可同时修改标题/优先级/截止时间/完成状态
- `task done` 专用于修改执行者的完成状态，与 `task update --done` 作用不同
- `task delete` 为不可逆操作，建议加 `--yes` 并与用户确认

## 自动化脚本

| 脚本 | 场景 | 用法 |
|------|------|------|
| [todo_daily_summary.py](../../scripts/todo_daily_summary.py) | 查看今天/明天/本周未完成待办汇总 | `python todo_daily_summary.py today` |
| [todo_batch_create.py](../../scripts/todo_batch_create.py) | 从 JSON 文件批量创建待办 | `python todo_batch_create.py todos.json` |
| [todo_overdue_check.py](../../scripts/todo_overdue_check.py) | 扫描逾期待办输出逾期清单 | `python todo_overdue_check.py` |

