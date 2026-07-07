# OA 审批 (oa) 命令参考

## 命令总览

### 查询待我处理的审批
```
Usage:
  dws oa approval list-pending [flags]
Example:
  dws oa approval list-pending --start "2026-03-10T00:00:00+08:00" --end "2026-03-10T23:59:59+08:00"
  dws oa approval list-pending --start "2026-03-10T00:00:00+08:00" --end "2026-03-10T23:59:59+08:00" --query 关键词
Flags:
      --end string   结束时间 ISO-8601 (如 2026-03-10T23:59:59+08:00) (必填)
      --page string  分页页码 (可选)
      --size string  每页大小 (可选)
      --start string 开始时间 ISO-8601 (如 2026-03-10T00:00:00+08:00) (必填)
      --query string  关键字搜索 (可选)
```

### 获取审批实例详情
```
Usage:
  dws oa approval detail [flags]
Example:
  dws oa approval detail --instance-id <processInstanceId>
Flags:
      --instance-id string   审批实例 ID (必填)
```

### 同意审批

> **CAUTION:** 审批决策不可撤回 — 执行前必须向用户确认。

```
Usage:
  dws oa approval approve [flags]
Example:
  dws oa approval approve --instance-id <id> --task-id <taskId>
  dws oa approval approve --instance-id <id> --task-id <taskId> --remark "同意"
Flags:
      --instance-id string   审批实例 ID (必填)
      --remark string        审批意见 (可选)
      --task-id string       审批任务 ID (必填)
```

### 拒绝审批

> **CAUTION:** 审批决策不可撤回 — 执行前必须向用户确认。

```
Usage:
  dws oa approval reject [flags]
Example:
  dws oa approval reject --instance-id <id> --task-id <taskId> --remark "不同意"
Flags:
      --instance-id string   审批实例 ID (必填)
      --remark string        审批意见 (可选)
      --task-id string       审批任务 ID (必填)
```

### 撤销已发起的审批
```
Usage:
  dws oa approval revoke [flags]
Example:
  dws oa approval revoke --instance-id <id> --yes
  dws oa approval revoke --instance-id <id> --remark "误发起" --yes
Flags:
      --instance-id string   审批实例 ID (必填)
      --remark string        撤销说明 (可选)
```

### 获取审批操作记录
```
Usage:
  dws oa approval records [flags]
Example:
  dws oa approval records --instance-id <processInstanceId>
Flags:
      --instance-id string   审批实例 ID (必填)
```

### 查询我已发起的审批实例记录
```
Usage:
  dws oa approval list-initiated [flags]
Example:
  dws oa approval list-initiated --process-code <code> --start "2026-03-10T00:00:00+08:00" --end "2026-03-10T23:59:59+08:00" --cursor 0 --limit 20
Flags:
      --end string            结束时间 ISO-8601 (如 2026-03-10T23:59:59+08:00) (必填)
      --limit string          每页大小，最大 20 (必填)
      --cursor string         分页游标，首次传 0 (必填)
      --process-code string   表单 processCode (必填)
      --start string          开始时间 ISO-8601 (如 2026-03-10T00:00:00+08:00) (必填)
```

### 获取当前用户可见的审批表单列表
```
Usage:
  dws oa approval list-forms [flags]
Example:
  dws oa approval list-forms --cursor 0 --size 100
Flags:
      --cursor string  分页游标，首次传 0 (默认 "0")
      --size string    每页大小，最大 100 (默认 "100")
```
MCP 工具: `list_user_visible_process`；参数: cursor, pageSize（对应 --cursor/--size）。返回结果含 processCode，可用于 list-initiated 的 --process-code。

### 按关键字模糊搜索审批表单
```
Usage:
  dws oa approval search-forms [flags]
Example:
  dws oa approval search-forms --query AI
  dws oa approval search-forms --query 报销
Flags:
      --query string  关键字，匹配 processCode 或表单名称 (必填)
```

### 获取审批任务的被催办人 userId

> **催办必须两步串联：** ① `ding-info` 获取被催办人 `userId` → ② `ding message send` 发送催办消息。禁止跳过第一步直接猜测 userId。

```
Usage:
  dws oa approval ding-info [flags]
Example:
  dws oa approval ding-info --task-id <taskId>
Flags:
      --task-id string  审批任务 ID (必填)，来自 list-pending 或 tasks
```

返回值字段：
- `userId` — 被催办人用户 ID（必取），作为 `ding message send` 的 `--users` 入参，多个以逗号拼接

**不返回** robotCode 和 content，需由 agent 自行处理：
- `--robot-code`：优先取环境变量 `$DINGTALK_DING_ROBOT_CODE`；若无则向用户确认
- `--content`：由 agent 根据审批上下文撰写催办文案，建议格式 `"请尽快审批《{表单名}》（提交人：{发起人}，提交时间：{时间}）"`
- `--users`：取本接口返回的 `userId`

**叮消息类型（`--type`）：**
- 默认不发 `--type` → 应用内 DING 提醒（免费，推荐）
- `--type sms` → 短信提醒（有成本，需向用户确认）
- `--type call` → 电话提醒（有成本，需向用户确认）

**完整催办流程：**
```bash
# Step 1: 获取被催办人 userId
dws oa approval ding-info --task-id <taskId> --format json
# Step 2: 发送催办消息（robotCode 优先走环境变量，content 由 agent 撰写）
dws ding message send --robot-code $DINGTALK_DING_ROBOT_CODE --users <userId逗号拼接> --content "请尽快审批《XXX》" --format json
# Step 3 (可选): 如需短信/电话提醒，加 --type sms 或 --type call
dws ding message send --robot-code $DINGTALK_DING_ROBOT_CODE --users <userId逗号拼接> --content "请尽快审批《XXX》" --type sms --format json
```

### 获取任务可回退的节点信息

> **IMPORTANT:** 退回任务前**必须先调用此命令**获取可回退节点列表，从中提取 `activityId` 和 `revertAction` 作为 `revert-task` 的入参。若无返回值，明确告知用户"当前任务无可回退节点"。

```
Usage:
  dws oa approval revert-activities [flags]
Example:
  dws oa approval revert-activities --task-id <taskId>
Flags:
      --task-id string  审批任务 ID (必填)
```

返回字段说明：
- `instRevertActivities[]` — 可回退的节点列表
  - `activityId` — 节点 ID，即 `revert-task` 的 `--target-activity-id`
  - `activityName` — 节点名称（如"发起人"、"审批人"），用于向用户展示
  - `revertAction` — 退回方式，即 `revert-task` 的 `--action`
    - `REVERT_FOR_RESUBMIT` → 退回到发起人重交（此时 `activityId` 为 `sid-startevent`）
    - `REVERT_FOR_APPROVAL` → 退回到某审批节点重新审批
  - `activityActioners[]` — 该节点的审批人列表
  - `actualActioners[]` — 该节点的实际处理人列表
  - `approvalIndex` — 审批节点序号（仅审批节点有）
  - `actType` — 审批类型（如 `one_by_one` 依次审批）

**无返回值处理：** 若 `instRevertActivities` 为空或不存在，必须明确告知用户"当前任务无可回退节点"，不得继续执行退回操作。

### 查询待我审批的任务 ID
```
Usage:
  dws oa approval tasks [flags]
Example:
  dws oa approval tasks --instance-id <processInstanceId>
Flags:
      --instance-id string   审批实例 ID (必填)
```
MCP 工具: `list_pending_tasks`。


### 查询我处理过的审批单
```
Usage:
  dws oa approval list-executed [flags]
Example:
  dws oa approval list-executed --limit <pageSize> --page <pageNumber> --query 关键词
Flags:
      --page string   分页页码，可选，默认是 1
      --limit string   分页大小，可选，默认是 20
      --query string   查询关键词，可选
```
### 查询我已经提交的审批单
```
Usage:
  dws oa approval list-submitted [flags]
Example:
  dws oa approval list-submitted --limit <pageSize> --page <pageNumber> --query 关键词
Flags:
      --page string   分页页码，可选，默认是 1
      --limit string   分页大小，可选，默认是 20
      --query string   查询关键词，可选
```
### 查询抄送我的审批单
```
Usage:
  dws oa approval list-cc [flags]
Example:
  dws oa approval list-cc --limit <pageSize> --page <pageNumber> --query 关键词
Flags:
      --page string   分页页码，可选，默认是 1
      --limit string   分页大小，可选，默认是 20
      --query string   查询关键词，可选
```

### 转交审批任务
```
Usage:
  dws oa approval redirect-task [flags]
Example:
  dws oa approval redirect-task --task-id <taskId> --to-actioner-id <userId>
  dws oa approval redirect-task --task-id <taskId> --to-actioner-id <userId> --remark "请帮忙处理"
Flags:
      --task-id string          审批任务 ID (必填)
      --to-actioner-id string   转交目标用户 ID (必填)
      --remark string           转交说明 (可选)
```
MCP 工具: `redirect_task`；参数: taskId, toActionerId, remark（对应 --task-id/--to-actioner-id/--remark）。taskId 可通过 `tasks` 命令获取，toActionerId 可通过 `dws contact user search` 获取。

### 对审批实例添加评论
```
Usage:
  dws oa approval oa-comments [flags]
Example:
  dws oa approval oa-comments --instance-id <processInstanceId> --text "同意，请尽快处理"
Flags:
      --instance-id string   审批实例 ID (必填)
      --text string          评论内容 (必填)
```
MCP 工具: `dingflow_comments`；参数: processInstanceId, text（对应 --instance-id/--text）。processInstanceId 可通过 `list-pending` 或 `detail` 获取。

### 对审批实例进行抄送
```
Usage:
  dws oa approval oa-cc-noticer [flags]
Example:
  dws oa approval oa-cc-noticer --instance-id <processInstanceId> --user-list "68674200835816"
  dws oa approval oa-cc-noticer --instance-id <processInstanceId> --user-list "userId1,userId2"
Flags:
      --instance-id string   审批实例 ID (必填)
      --user-list string     抄送用户 ID 列表，多个用逗号分隔 (必填)
```
MCP 工具: `oa_cc_noticer`；参数: processInstanceId, userList（对应 --instance-id/--user-list）。processInstanceId 可通过 `list-pending` 或 `detail` 获取，抄送用户 ID 可通过 `dws contact user search` 获取。

### 对审批任务进行加签

> **CAUTION:** 加签操作不可撤回 — 执行前必须向用户确认加签类型、被加签人和激活方式。
```
Usage:
  dws oa approval append-task [flags]
Example:
  dws oa approval append-task --instance-id <processInstanceId> --task-id <taskId> --type before --appender-user-ids "userId1,userId2" --activate-type ALL --agree-all true
  dws oa approval append-task --instance-id <processInstanceId> --task-id <taskId> --type after --appender-user-ids "userId1" --activate-type ONE_BY_ONE --agree-all false
Flags:
      --instance-id string        审批实例 ID (必填)
      --task-id string            审批任务 ID (必填)
      --type string               加签类型：before（前加签），after（后加签），Parallel（并加签）(必填)
      --appender-user-ids string  被加签用户 ID 列表，多个用逗号分隔 (必填)
      --activate-type string      任务激活类型：ALL（或签），ONE_BY_ONE（依次审批）(必填)
      --agree-all string          是否需要全部同意 (必填) 是 true 否 false
```

### 退回审批任务

> **CAUTION:** 退回操作不可撤回 — 执行前必须向用户确认退回方式及目标节点。
> **前置步骤：** 必须先调用 `revert-activities` 获取可回退节点列表，从中提取 `activityId` 和 `revertAction`。若无返回值，明确告知用户"当前任务无可回退节点"。

```
Usage:
  dws oa approval revert-task [flags]
Example:
  # 退回到发起人（targetActivityId 固定传 sid-startevent）
  dws oa approval revert-task --instance-id <processInstanceId> --task-id <taskId> --target-activity-id sid-startevent --action REVERT_FOR_RESUBMIT --remark "补充说明后重提"
  # 退回到某个审批节点（targetActivityId 从 revert-activities 返回中获取 activityId）
  dws oa approval revert-task --instance-id <processInstanceId> --task-id <taskId> --target-activity-id <activityId> --action REVERT_FOR_APPROVAL --remark "重新审批"
Flags:
      --instance-id string          审批实例 ID (必填)
      --task-id string              审批任务 ID (必填)
      --target-activity-id string   退回到的节点 ID；退回发起人时固定传 sid-startevent (必填)
      --action string               退回方式：REVERT_FOR_APPROVAL（退回到审批人）/ REVERT_FOR_RESUBMIT（退回到发起人）(必填)
      --remark string               退回说明 (可选)
```


## 意图判断

用户说"待审批/待处理审批" → `approval list-pending`
用户说"审批详情/看审批" → `approval detail`
用户说"同意审批/批准" → 先 `tasks` 获取 taskId，再 `approve`
用户说"拒绝审批/驳回" → 先 `tasks` 获取 taskId，再 `reject`
用户说"撤回审批/取消审批" → `approval revoke`
用户说"审批记录/操作历史" → `approval records`
用户说"我发起的审批" → `approval list-initiated`（需 --process-code，可从 list-forms 或 detail 获取）
用户说"有哪些审批表单/可见表单" → `approval list-forms`
用户说"我有哪些待审的任务" → `approval tasks`
用户说"我发起的审批单" -> `approval list-submitted`
用户说"我审批/处理过的审批单" -> `approval list-executed`
用户说"抄送我的审批单" -> `approval list-cc`
用户说"转交审批/转交任务" → `approval redirect-task`（需 --task-id 和 --to-actioner-id）
用户说"评论审批/添加评论/写评论" → `approval oa-comments`（需 --instance-id 和 --text）
用户说"抄送审批/添加抄送人" → `approval oa-cc-noticer`（需 --instance-id 和 --user-list）

## 核心工作流

```bash
# 1. 查看待我处理的审批 — 提取 processInstanceId
dws oa approval list-pending --start "2026-03-10T00:00:00+08:00" --end "2026-03-10T23:59:59+08:00" --format json

# 2. 查看审批详情 — 了解审批内容
dws oa approval detail --instance-id <processInstanceId> --format json

# 3. 获取待审批任务 ID — 提取 taskId
dws oa approval tasks --instance-id <processInstanceId> --format json

# 4a. 同意审批
dws oa approval approve --instance-id <id> --task-id <taskId> --remark "同意" --format json

# 4b. 拒绝审批
dws oa approval reject --instance-id <id> --task-id <taskId> --remark "不符合要求" --format json

# 5. 撤销自己发起的审批
dws oa approval revoke --instance-id <id> --remark "误发起" --format json

# 6. 查看审批操作记录
dws oa approval records --instance-id <processInstanceId> --format json

# 7. 获取可见审批表单（得到 processCode）
dws oa approval list-forms --cursor 0 --size 100 --format json

# 8. 查看自己发起的审批列表（--process-code 来自 list-forms 或 detail）
dws oa approval list-initiated --process-code <code> \
  --start "2026-03-10T00:00:00+08:00" --end "2026-03-10T23:59:59+08:00" \
  --next-token 0 --max-results 20 --format json
  
# 9. 我处理过的审批单
dws oa approval list-executed --limit <pageSize> --page <pageNumber> --query 关键词 --format json
# 10. 我发起的审批单 
dws oa approval list-submitted --limit <pageSize> --page <pageNumber> --query 关键词 --format json
# 11. 抄送我的审批单 
dws oa approval list-cc --limit <pageSize> --page <pageNumber> --query 关键词 --format json

# 12. 转交审批任务（taskId 来自 tasks，toActionerId 来自 contact user search）
dws oa approval redirect-task --task-id <taskId> --to-actioner-id <userId> --format json
dws oa approval redirect-task --task-id <taskId> --to-actioner-id <userId> --remark "请帮忙处理" --format json

# 13. 对审批实例添加评论（processInstanceId 来自 list-pending 或 detail）
dws oa approval oa-comments --instance-id <processInstanceId> --text "同意，请尽快处理" --format json

# 14. 对审批实例进行抄送（processInstanceId 来自 list-pending 或 detail）
dws oa approval oa-cc-noticer --instance-id <processInstanceId> --user-list "68674200835816" --format json
dws oa approval oa-cc-noticer --instance-id <processInstanceId> --user-list "userId1,userId2" --format json
```

## 上下文传递表

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `list-pending` | `processInstanceId` | detail / tasks / records / revoke / oa-comments / oa-cc-noticer 的 --instance-id |
| `tasks` | `taskId` | approve / reject / redirect-task 的 --task-id |
| `detail` | `processCode` | list-initiated 的 --process-code |
| `list-forms` | `processCode` | list-initiated 的 --process-code |

## 注意事项

- `--start` / `--end` 使用 ISO-8601 格式（如 2026-03-10T00:00:00+08:00）
- `approve` / `reject` / `redirect-task` 需先通过 `tasks` 获取 `taskId`
- `redirect-task` 的 `--to-actioner-id` 可通过 `dws contact user search` 获取目标用户 userId
- `revoke` 只能撤销自己发起的审批
- `--remark` 审批意见虽为可选，但建议填写以留存审批痕迹
- `list-initiated` 的 `--process-code` 可从 `list-forms` 或 `detail` 返回中提取

## 自动化脚本

| 脚本 | 场景 | 用法 |
|------|------|------|
| [oa_pending_review.py](../../scripts/oa_pending_review.py) | 查看待审批列表+逐条显示详情 | `python oa_pending_review.py --days 7` |
| [oa_batch_approve.py](../../scripts/oa_batch_approve.py) | 批量同意/拒绝审批项 | `python oa_batch_approve.py --action approve --days 7` |
