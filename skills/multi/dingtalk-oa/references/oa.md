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
      --limit string  每页大小 (可选)
      --start string 开始时间 ISO-8601 (如 2026-03-10T00:00:00+08:00) (必填)
      --query string  关键字搜索 (可选)

**默认时间窗口：** 当用户未指定 --start / --end 时，默认查询最近 30 天的待处理审批。

> **IMPORTANT:** 当 `list-pending` 返回空时，必须明确告知用户"当前暂无待处理审批"，并建议扩大时间范围或检查关键词。
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
      --limit string    每页大小，最大 20 (必填)
      --cursor string     分页游标，首次传 0 (必填)
      --process-code string   表单 processCode (必填)
      --start string          开始时间 ISO-8601 (如 2026-03-10T00:00:00+08:00) (必填)
```

### 获取当前用户可见的审批表单列表
```
Usage:
  dws oa approval list-forms [flags]
Example:
  dws oa approval list-forms --cursor 0 --limit 100
Flags:
      --cursor string  分页游标，首次传 0 (默认 "0")
      --limit string    每页大小，最大 100 (默认 "100")
```

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

### 对审批实例添加评论
```
Usage:
  dws oa approval oa-comments [flags]
Example:
  dws oa approval oa-comments --instance-id <processInstanceId> --content "同意，请尽快处理"
Flags:
      --instance-id string   审批实例 ID (必填)
      --content string          评论内容 (必填)
```

### 对审批实例进行抄送
```
Usage:
  dws oa approval oa-cc-noticer [flags]
Example:
  dws oa approval oa-cc-noticer --instance-id <processInstanceId> --users "68674200835816"
  dws oa approval oa-cc-noticer --instance-id <processInstanceId> --users "userId1,userId2"
Flags:
      --instance-id string   审批实例 ID (必填)
      --users string     抄送用户 ID 列表，多个用逗号分隔 (必填)
```

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
      --agree-all                 是否需要全部同意 (必填) 是 true 否 false
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

用户说"待审批/待处理审批/查询XX审批/查XX审批/有没有XX审批/XX的审批单" → `approval list-pending`，将 XX 作为 `--query` 关键字传入（可搜索表单名称或表单详情内容）
  - 示例："帮我查询补卡的审批单" → `approval list-pending --query 补卡`
  - 示例："有没有外出申请的审批" → `approval list-pending --query 外出申请`
  - 示例："待审批"（无关键词）→ `approval list-pending`
用户说"审批详情/看审批" → `approval detail`
用户说"同意审批/批准" → 先 `tasks` 获取 taskId，再 `approve`
用户说"拒绝审批/驳回" → 先 `tasks` 获取 taskId，再 `reject`
用户说"撤回审批/取消审批" → `approval revoke`
用户说"审批记录/操作历史" → `approval records`
用户说"我发起的审批" → `approval list-initiated`（需 --process-code，可从 list-forms 或 detail 获取）
用户说"有哪些审批表单/可见表单" → `approval list-forms`
用户说"搜索审批表单/查找xx审批表单/有没有xx表单" → `approval search-forms`（需 --query）
用户说"催办审批/DING 一下审批人/提醒审批/催一下审批/催批/提醒审批人" → 先 `approval ding-info`（拿到被催办人 `userId`），再 `ding message send`（将 userId 作为 `--users` 传入；`--robot-code` 优先走 `$DINGTALK_DING_ROBOT_CODE` 或向用户确认；`--content` 由 agent 根据审批上下文撰写）
  - **禁止跳过 ding-info：** 不得自行猜测或编造 userId，必须先调用 `ding-info` 获取
  - **机器人编码获取顺序：** ① `$DINGTALK_DING_ROBOT_CODE` 环境变量 → ② 用户显式提供 → ③ 询问用户
  - **催办内容建议：** `"请尽快审批《{表单名}》（提交人：{发起人}，提交时间：{时间}）"`
  - **ding-info 返回空：** 若接口返回空或报错，告知用户"无法获取该任务的被催办人信息"并停止
用户说"我有哪些待审的任务" → `approval tasks`
用户说"我发起的审批单/我发起的XX审批/我提交的XX审批/查我发起的XX" → `approval list-submitted`，将 XX 作为 `--query` 关键字传入（可搜索表单名称或表单详情内容）
  - 示例："查我发起的补卡审批单" → `approval list-submitted --query 补卡`
  - 示例："我发起的审批单"（无关键词）→ `approval list-submitted`
用户说"我审批/处理过的审批单/我处理过的XX审批/我审批过的XX/查我处理过的XX" → `approval list-executed`，将 XX 作为 `--query` 关键字传入（可搜索表单名称或表单详情内容）
  - 示例："查我处理过的补卡审批单" → `approval list-executed --query 补卡`
  - 示例："我审批过的审批单"（无关键词）→ `approval list-executed`
用户说"抄送我的审批单/抄送我的XX审批/CC我的XX/查抄送我的XX" → `approval list-cc`，将 XX 作为 `--query` 关键字传入（可搜索表单名称或表单详情内容）
  - 示例："查抄送我的补卡审批单" → `approval list-cc --query 补卡`
  - 示例："抄送我的审批单"（无关键词）→ `approval list-cc`
用户说"转交审批/转交任务" → `approval redirect-task`（需 --task-id 和 --to-actioner-id）
用户说"评论审批/添加评论/写评论" → `approval oa-comments`（需 --instance-id 和 --content）
用户说"抄送审批/添加抄送人" → `approval oa-cc-noticer`（需 --instance-id 和 --users）
用户说"加签/前加签/后加签/并加签/增加审批人/追加审批人" → `approval append-task`（需 --instance-id, --task-id, --type, --appender-user-ids, --activate-type, --agree-all）
  - `--type` 映射：前加签 → before，后加签 → after，并加签 → Parallel
  - `--activate-type` 映射：或签 → ALL，依次审批 → ONE_BY_ONE
  - `--appender-user-ids` 可通过 `dws contact user search` 获取目标用户 userId
用户说"退回审批/退回发起人/退回到XX节点/打回重交/重新审批/回退/退回到" → `approval revert-task`（需 --instance-id, --task-id, --target-activity-id, --action）
  - **前置步骤：** 必须先调用 `approval revert-activities --task-id <taskId>` 获取可回退节点列表，提取 `activityId` 和 `revertAction`
  - **无节点处理：** 若 `revert-activities` 返回空 (`instRevertActivities` 为空)，必须明确告知用户"当前任务无可回退节点"，不得继续执行退回操作
  - `--action` 映射：退回发起人/打回重交 → REVERT_FOR_RESUBMIT；退回到审批人/重新审批 → REVERT_FOR_APPROVAL
  - `--target-activity-id`：退回发起人时固定传 `sid-startevent`；退回到审批人时从 `revert-activities` 返回中获取 `activityId`

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
dws oa approval list-forms --cursor 0 --limit 100 --format json

# 7b. 按关键字模糊搜索表单（快速定位 processCode）
dws oa approval search-forms --query AI --format json

# 8. 查看自己发起的审批列表（--process-code 来自 list-forms 或 detail）
dws oa approval list-initiated --process-code <code> \
  --start "2026-03-10T00:00:00+08:00" --end "2026-03-10T23:59:59+08:00" \
  --cursor 0 --limit 20 --format json

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
dws oa approval oa-comments --instance-id <processInstanceId> --content "同意，请尽快处理" --format json

# 14. 对审批实例进行抄送（processInstanceId 来自 list-pending 或 detail）
dws oa approval oa-cc-noticer --instance-id <processInstanceId> --users "68674200835816" --format json
dws oa approval oa-cc-noticer --instance-id <processInstanceId> --users "userId1,userId2" --format json

# 15. 催办审批（必须两步串联：先拿被催办人 userId，再发 DING）
# 15a. Step 1: 调用 ding-info 拿到被催办人 userId（来自 list-pending 或 tasks 中的 taskId）
dws oa approval ding-info --task-id <taskId> --format json
# 15b. Step 2: 将 userId 填入 --users；robot-code 优先走环境变量 $DINGTALK_DING_ROBOT_CODE（或向用户确认）；content 由 agent 根据审批上下文撰写
dws ding message send --robot-code $DINGTALK_DING_ROBOT_CODE --users <userId1,userId2> --content "请尽快审批《XXX》" --format json
# 15c (可选): 如需短信/电话提醒，加 --type sms 或 --type call
dws ding message send --robot-code $DINGTALK_DING_ROBOT_CODE --users <userId1,userId2> --content "请尽快审批《XXX》" --type sms --format json

# 16. 对审批任务进行加签（instanceId 来自 list-pending/list-submitted/list-executed/detail，taskId 来自list-pending/list-submitted/list-executed/detail中 ，appenderUserIds 来自 contact user search）
dws oa approval append-task --instance-id <processInstanceId> --task-id <taskId> --type before --appender-user-ids "userId1,userId2" --activate-type ALL --agree-all --format json
dws oa approval append-task --instance-id <processInstanceId> --task-id <taskId> --type Parallel --appender-user-ids "userId1" --activate-type ONE_BY_ONE --agree-all --format json

# 17. 退回审批任务（instanceId/taskId 来自 list-pending、tasks；targetActivityId 和 action 来自 revert-activities）
# 17a. 获取可回退节点（必须先调用，从此返回中提取 activityId 和 revertAction）
dws oa approval revert-activities --task-id <taskId> --format json
# 17b. 退回到发起人重提（targetActivityId 固定 sid-startevent，action=REVERT_FOR_RESUBMIT）
dws oa approval revert-task --instance-id <processInstanceId> --task-id <taskId> --target-activity-id sid-startevent --action REVERT_FOR_RESUBMIT --remark "补充说明后重提" --format json
# 17c. 退回到某个审批节点重新审批（targetActivityId 和 action 从 revert-activities 返回中获取）
dws oa approval revert-task --instance-id <processInstanceId> --task-id <taskId> --target-activity-id <activityId> --action REVERT_FOR_APPROVAL --remark "重新审批" --format json
```

## 上下文传递表

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `list-pending` | `processInstanceId` | detail / tasks / records / revoke / oa-comments / oa-cc-noticer / append-task / revert-task 的 --instance-id |
| `tasks` | `taskId` | approve / reject / redirect-task / append-task / revert-task 的 --task-id |
| `detail` | `processCode` | list-initiated 的 --process-code |
| `list-forms` | `processCode` | list-initiated 的 --process-code |
| `search-forms` | `processCode` | list-initiated 的 --process-code |
| `ding-info` | `userId` | ding message send 的 --users（多个逗号拼接）；**robotCode 优先走 `$DINGTALK_DING_ROBOT_CODE` 环境变量，content 由 agent 根据审批上下文撰写；返回空时报错并停止** |
| `revert-activities` | `activityId`, `revertAction`, `activityName` | revert-task 的 --target-activity-id 和 --action；**返回空时必须告知用户"无可回退节点"** |

## 注意事项

- `--start` / `--end` 使用 ISO-8601 格式（如 2026-03-10T00:00:00+08:00）
- `list-pending` 默认时间窗口为最近 30 天；若用户未指定 `--start` / `--end`，自动使用当前时间往前推 7 天
- `list-pending` 返回空时必须明确告知用户"当前暂无待处理审批"，不得沉默或跳过
- `approve` / `reject` / `redirect-task` / `append-task` / `revert-task` 需先通过 `tasks` 获取 `taskId`
- `redirect-task` 的 `--to-actioner-id` 可通过 `dws contact user search` 获取目标用户 userId
- `append-task` 的 `--appender-user-ids` 可通过 `dws contact user search` 获取目标用户 userId 但不能是自己
- `append-task` 的 `--type` 值：before（前加签）、after（后加签）、Parallel（并加签）
- `append-task` 的 `--activate-type` 值：ALL（或签）、ONE_BY_ONE（依次审批）
- `append-task` 的 `--agree-all` 值：true（需要全部同意）、false（不需要全部同意）
- `revert-task` 的 `--action` 值：REVERT_FOR_APPROVAL（退回到某审批节点）、REVERT_FOR_RESUBMIT（退回到发起人）
- `revert-task` 退回前**必须先调用** `revert-activities --task-id <taskId>` 获取可回退节点列表
- `revert-task` 的 `--target-activity-id` 和 `--action` **必须来自** `revert-activities` 返回，禁止自行编造或猜测
- `revert-activities` 返回空 (`instRevertActivities` 为空) 时，必须明确告知用户"当前任务无可回退节点"，禁止继续执行退回
- `revert-task` 是不可撤回操作，执行前必须向用户确认退回方式及目标节点
- `revoke` 只能撤销自己发起的审批
- `--remark` 审批意见虽为可选，但建议填写以留存审批痕迹
- `list-initiated` 的 `--process-code` 可从 `list-forms`、`search-forms` 或 `detail` 返回中提取
- 已知表单名称关键字时优先用 `search-forms`；需枚举全部表单时用 `list-forms`
- 催办必须两步串联：`ding-info` 仅返回被催办人 `userId`，不返回 robotCode/content；需再调用 `dws ding message send`，其中 `--robot-code` 优先使用环境变量 `$DINGTALK_DING_ROBOT_CODE`，若无则向用户确认；`--content` 由 agent 根据审批上下文撰写催办文案；**严禁跳过 `ding-info` 直接猜测 userId**
- 催办文案建议格式：`"请尽快审批《{表单名}》（提交人：{发起人}，提交时间：{时间}）"`
- `ding-info` 返回空或报错时，必须明确告知用户"无法获取该任务的被催办人信息"并停止
- DING 默认发应用内提醒（无成本）；如需短信/电话提醒可加 `--type sms` 或 `--type call`（有成本，建议向用户确认）

## 自动化脚本

| 脚本 | 场景 | 用法 |
|------|------|------|
| [oa_pending_review.py](../scripts/oa_pending_review.py) | 查看待审批列表+逐条显示详情 | `python oa_pending_review.py --days 7` |
| [oa_batch_approve.py](../scripts/oa_batch_approve.py) | 批量同意/拒绝审批项 | `python oa_batch_approve.py --action approve --days 7` |
