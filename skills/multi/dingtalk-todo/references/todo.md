# 待办 (todo) 命令参考

## 命令总览

### 创建待办
```
Usage:
  dws todo task create [flags]
Example:
  dws todo task create --title "修复线上Bug" --executors <USER_ID_1>,<USER_ID_2> --priority 40
  dws todo task create --title "每日站会" --executors <USER_ID> --due "2026-03-20T10:00:00+08:00" --recurrence "DTSTART:20260320T020000Z\nRRULE:FREQ=DAILY;INTERVAL=1"
Flags:
      --due string         截止时间 ISO-8601 (如 2026-03-10T18:00:00+08:00；这是 deadline，不是 reminder)
      --executors string   执行者 userId 列表 (必填)
      --priority string    优先级: 10低/20普通/30较高/40紧急
      --recurrence string  循环待办 (需先设置 --due); 仅支持按天循环，格式见下方说明
      --title string       待办标题 (必填)
```

### 创建子待办
```
Usage:
  dws todo task create-sub [flags]
Example:
  dws todo task create-sub --parent-id <PARENT_TASK_ID> --title "子任务标题" --executors <USER_ID_1>,<USER_ID_2> --priority 40
  dws todo task create-sub --parent-id <PARENT_TASK_ID> --title "子任务标题" --executors <USER_ID> --due "2026-03-20T10:00:00+08:00"
Flags:
      --due string         截止时间 ISO-8601 (如 2026-03-10T18:00:00+08:00；这是 deadline，不是 reminder)
      --executors string   执行者 userId 列表 (必填)
      --parent-id string   父待办任务 ID (必填，该信息可以通过创建待办接口或者查询待办列表接口返回)
      --priority string    优先级: 10低/20普通/30较高/40紧急
      --recurrence string  循环待办 (需先设置 --due); 仅支持按天循环，格式见下方说明
      --title string       子待办标题 (必填)
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
      --due string        截止时间 ISO-8601 (如 2026-03-10T18:00:00+08:00；这是 deadline，不是 reminder)
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

> **CAUTION:** 不可逆操作 — 执行前必须向用户确认。

```
Usage:
  dws todo task delete [flags]
Example:
  dws todo task delete --task-id <taskId>
  dws todo task delete --task-id <taskId> --yes
Flags:
      --task-id string   待办任务 ID (必填)
```

### 新增待办评论
```
Usage:
  dws todo comment add [flags]
Example:
  dws todo comment add --task-id <taskId> --content "评论内容"
Flags:
      --task-id string   待办任务 ID (必填)
      --content string   评论内容 (必填)
```

### 查询待办评论列表
```
Usage:
  dws todo comment list [flags]
Example:
  dws todo comment list --task-id <taskId>
  dws todo comment list --task-id <taskId> --page 1 --size 20
Flags:
      --task-id string   待办任务 ID (必填)
      --page string      页码 (默认 1)
      --size string      每页数量 (默认 20)
```

### 删除待办评论

> **CAUTION:** 不可逆操作 — 执行前必须向用户确认。

```
Usage:
  dws todo comment delete [flags]
Example:
  dws todo comment delete --task-id <taskId> --comment-id <commentId>
  dws todo comment delete --task-id <taskId> --comment-id <commentId> --yes
Flags:
      --task-id string      待办任务 ID (必填)
      --comment-id string   评论 ID (必填)
      --yes                 跳过二次确认 (慎用)
```

### 添加待办执行人
```
Usage:
  dws todo task add-executor [flags]
Example:
  dws todo task add-executor --task-id <taskId> --executors <USER_ID_1>,<USER_ID_2>
Flags:
      --executors string   执行者 userId 列表 (必填)
      --task-id string     待办任务 ID (必填)
```
### 移除待办执行人
```
Usage:
  dws todo task remove-executor [flags]
Example:
  dws todo task remove-executor --task-id <taskId> --executors <USER_ID_1>,<USER_ID_2>
Flags:
      --executors string   执行者 userId 列表 (必填)
      --task-id string     待办任务 ID (必填)
```
### 添加待办参与人
```
Usage:
  dws todo task add-participant [flags]
Example:
  dws todo task add-participant --task-id <taskId> --participants <USER_ID_1>,<USER_ID_2>
Flags:
      --participants string   参与人 userId 列表 (必填)
      --task-id string        待办任务 ID (必填)
```
### 移除待办参与人
```
Usage:
  dws todo task remove-participant [flags]
Example:
  dws todo task remove-participant --task-id <taskId> --participants <USER_ID_1>,<USER_ID_2>
Flags:
      --participants string   参与人 userId 列表 (必填)
      --task-id string        待办任务 ID (必填)
```

### 添加待办提醒
```
Usage:
  dws todo task add-reminder [flags]
Example:
  dws todo task add-reminder --task-id <taskId> --base-time dueTime --due-date-offset -30
  dws todo task add-reminder --task-id <taskId> --base-time customTime --reminder-time-stamp "2026-03-10T18:00:00+08:00"
Flags:
      --base-time string              提醒基准时间: dueTime/customTime (必填)
      --due-date-offset string        截止时间偏移量 (baseTime=dueTime 时必填)
      --reminder-time-stamp string    自定义提醒时间 ISO-8601 (如 2026-03-10T18:00:00+08:00；baseTime=customTime 时必填)
      --task-id string                待办任务 ID (必填)
```

参数说明：

| 参数 | 类型 | 说明 |
|------|------|------|
| `--base-time` | string | 提醒基准时间，必填。`dueTime` = 基于截止时间偏移；`customTime` = 自定义时间戳 |
| `--due-date-offset` | number | 截止时间偏移量（分钟），`baseTime=dueTime` 时必填。负数表示提前，如 `-30` 表示截止前 30 分钟 |
| `--reminder-time-stamp` | string | 自定义提醒时间，ISO-8601 格式（如 `2026-03-10T18:00:00+08:00`），`baseTime=customTime` 时必填 |

### 重置待办提醒
```
Usage:
  dws todo task reset-reminder [flags]
Example:
  dws todo task reset-reminder --task-id <taskId>
  dws todo task reset-reminder --task-id <taskId> --reminder-rules '[{"dueDateOffset":-30,"baseTime":"dueTime"},{"reminderTimeStamp":"2026-03-10T18:00:00+08:00","baseTime":"customTime"}]'
Flags:
      --reminder-rules string   提醒规则 JSON 数组 (可选，为空则清除提醒)
      --task-id string          待办任务 ID (必填)
```

`--reminder-rules` 数据结构说明：

JSON 数组，每个元素为一条提醒规则，支持两种 `baseTime` 模式混合使用：

| 字段 | 类型 | 说明 |
|------|------|------|
| `baseTime` | string | 提醒基准时间，必填。`dueTime` = 基于截止时间偏移；`customTime` = 自定义时间戳 |
| `dueDateOffset` | number | 截止时间偏移量（分钟），`baseTime=dueTime` 时必填。负数表示提前，如 `-30` 表示截止前 30 分钟 |
| `reminderTimeStamp` | string | 自定义提醒时间，ISO-8601 格式（如 `2026-03-10T18:00:00+08:00`），`baseTime=customTime` 时必填 |

示例：
```json
[
  {"dueDateOffset": -30, "baseTime": "dueTime"},
  {"reminderTimeStamp": "2026-03-10T18:00:00+08:00", "baseTime": "customTime"}
]
```
以上表示两条提醒规则：第一条在截止时间前 30 分钟提醒，第二条在指定时间（ISO-8601）提醒。

## 意图判断

用户说"加个待办/记一下/TODO" → `task create`
用户说"每天重复/循环待办/按天重复" → `task create`（需 `--due` + `--recurrence`）
用户说"加个子任务/创建子待办" → `task create-sub`
用户说"看看待办/我有啥要做" → `task list`
用户说"改个待办/修改待办标题/改优先级" → `task update`
用户说"做完了/完成待办/标记完成" → `task done`
用户说"看看待办详情" → `task get`
用户说"删除待办/取消待办" → `task delete`
用户说"给待办加条评论/留个备注" → `comment add`
用户说"看看这个待办的评论" → `comment list`
用户说"删除这条评论" → `comment delete`
用户说"加个执行人/添加执行者" → `task add-executor`
用户说"移除执行人/删除执行者" → `task remove-executor`
用户说"加个参与人/添加参与者" → `task add-participant`
用户说"移除参与人/删除参与者" → `task remove-participant`
用户说"给待办加个提醒/设置提醒" → `task add-reminder`
用户说"重置提醒/清除提醒/修改提醒规则" → `task reset-reminder`

关键区分: todo(个人待办)

## 核心工作流

```bash
# 1. 创建待办 — 提取 todoTaskId
dws todo task create --title "修复线上Bug" --executors userId1,userId2 \
  --priority 40 --due "2026-03-10T18:00:00+08:00" --format json

# 1b. 创建按天循环的待办（必须先有 --due；recurrence 与 MCP create_personal_todo 一致）
dws todo task create --title "每日站会" --executors userId1 \
  --due "2026-03-20T10:00:00+08:00" \
  --recurrence "DTSTART:20260320T020000Z\nRRULE:FREQ=DAILY;INTERVAL=1" --format json

# 1c. 创建子待办（需先获取父待办 ID）
dws todo task create-sub --parent-id <PARENT_TASK_ID> --title "子任务标题" --executors userId1 \
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

# 7. 给待办新增评论
dws todo comment add --task-id <taskId> --content "已开始处理" --format json

# 8. 查看待办评论列表
dws todo comment list --task-id <taskId> --page 1 --size 20 --format json

# 9. 删除待办评论
dws todo comment delete --task-id <taskId> --comment-id <commentId> --yes --format json

# 10. 添加待办执行人
dws todo task add-executor --task-id <taskId> --executors userId1,userId2 --format json
# 11. 移除待办执行人
dws todo task remove-executor --task-id <taskId> --executors userId1 --format json
# 12. 添加待办参与人
dws todo task add-participant --task-id <taskId> --participants userId1,userId2 --format json
# 13. 移除待办参与人
dws todo task remove-participant --task-id <taskId> --participants userId1 --format json

# 14. 添加待办提醒（基于截止时间偏移，待办必须有截止时间）
dws todo task add-reminder --task-id <taskId> --base-time dueTime --due-date-offset <dueDateOffset> --format json
# 15. 添加待办提醒（自定义时间戳）
dws todo task add-reminder --task-id <taskId> --base-time customTime --reminder-time-stamp "2026-03-10T18:00:00+08:00" --format json
# 16. 重置待办提醒
dws todo task reset-reminder --task-id <taskId> --format json
# 17. 重置待办提醒（指定新规则）
dws todo task reset-reminder --task-id <taskId> --reminder-rules '<reminderRules>' --format json
```

## 上下文传递表

| 操作 | 从返回中提取 | 用于                                          |
|------|-------------|---------------------------------------------|
| `task create` | `todoTaskId` | update/done/get/delete 的 --task-id          |
| `task list` | `result[].id` | update/done/get/delete 的 --task-id          |
| `task create` | `todoTaskId` | update/done/get/delete/comment 的 --task-id  |
| `task list` | `result[].id` | update/done/get/delete/comment/add-executor/remove-executor/add-participant/remove-participant 的 --task-id |
| `task get` | `result.todoDetailModel.subTodos[]` | 获取子待办列表，提取子待办的 `taskId` 用于后续操作              |
| `comment list` | `result[].commentId` | `comment delete` 的 --comment-id             |

## 注意事项

- 优先级值: 10=低, 20=普通, 30=较高, 40=紧急
- `--due` 是截止时间 dueTime，不是提醒时间；使用 ISO-8601 格式（如 2026-03-10T18:00:00+08:00）
- 当前不支持单独的 `reminder` / `remind-at` 精确提醒能力；不要把 `--due` 解释成“几点提醒”
- `--recurrence`：仅在与 `--due` 同时设置时有效；当前仅支持按天循环。字符串内需含换行，示例：`DTSTART:20260320T020000Z\nRRULE:FREQ=DAILY;INTERVAL=1`（DTSTART 表示首次截止时间，需与业务约定一致）
- 若用户的真实诉求是“到点提醒我”，需要先说明能力边界；当前 CLI 只能表达 deadline / recurrence，不能表达独立 reminder schedule
- `task list` 的 `--status` 对应 MCP `get_user_todos_in_current_org` 的 `todoStatus` 参数
- todo 是个人待办管理产品
- `task update` 可同时修改标题/优先级/截止时间/完成状态
- `task done` 专用于修改执行者的完成状态，与 `task update --done` 作用不同
- `task delete` 为不可逆操作，建议加 `--yes` 并与用户确认
- `comment delete` 同样为不可逆操作，执行前需用户确认；`--comment-id` 可通过 `comment list` 获取
- `task add-executor` / `task remove-executor` 用于管理待办的执行人，`--executors` 支持逗号分隔的多个 userId
- `task add-participant` / `task remove-participant` 用于管理待办的参与人，`--participants` 支持逗号分隔的多个 userId
- 执行人 (executor) 与参与人 (participant) 的区别：执行人负责完成待办，参与人仅关注待办进度
- `task add-reminder` 用于为待办添加提醒，`--base-time` 支持 `dueTime`（基于截止时间偏移，待办必须有截止时间）和 `customTime`（自定义时间戳）两种模式
- `task reset-reminder` 用于重置待办提醒规则，不传 `--reminder-rules` 则清除所有提醒


## 自动化脚本

| 脚本 | 场景 | 用法 |
|------|------|------|
| [todo_daily_summary.py](../scripts/todo_daily_summary.py) | 查看今天/明天/本周未完成待办汇总 | `python todo_daily_summary.py today` |
| [todo_batch_create.py](../scripts/todo_batch_create.py) | 从 JSON 文件批量创建待办 | `python todo_batch_create.py todos.json` |
| [todo_overdue_check.py](../scripts/todo_overdue_check.py) | 扫描逾期待办输出逾期清单 | `python todo_overdue_check.py` |
