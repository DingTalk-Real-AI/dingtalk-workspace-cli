# OA 审批 (oa) 命令参考

钉钉 OA 审批：待办审批列表 / 查看详情 / 同意 / 拒绝 / 撤销 / 流程记录 / 表单模板。

## 命令总览

### 查询待我处理的审批实例
```
Usage:
  dws oa approval list-pending [flags]
Example:
  dws oa approval list-pending
  dws oa approval list-pending --page 1 --size 20
  dws oa approval list-pending --start "2026-03-01T00:00:00+08:00" --end "2026-03-31T23:59:59+08:00"
Flags:
      --page string    页码 (默认 1)
      --size string    每页数量 (默认 10)
      --start string   提交开始时间 ISO-8601 (可选)
      --end string     提交结束时间 ISO-8601 (可选)
```

"待办收件箱"视图：列出当前用户仍需动作的审批 `processInstanceId`。注意返回的是**实例 ID**，真正驱动 approve/reject 还需要用 `tasks` 再换成 `taskId`。

### 查询我发起的审批实例
```
Usage:
  dws oa approval list-initiated [flags]
Example:
  dws oa approval list-initiated
  dws oa approval list-initiated --process-code <processCode> --max-results 20
  dws oa approval list-initiated --start "2026-03-01T00:00:00+08:00" --end "2026-03-31T23:59:59+08:00" --next-token <nextToken>
Flags:
      --process-code string   按指定表单/流程模板过滤 (可选)
      --start string          发起开始时间 ISO-8601 (可选)
      --end string            发起结束时间 ISO-8601 (可选)
      --max-results string    每页条数 (可选)
      --next-token string     分页 token (首页留空)
```

查询当前用户作为申请人发起的审批，用来查"我报销到哪一步了"、"我之前提的请假单"。

### 查询可发起的审批表单模板
```
Usage:
  dws oa approval list-forms [flags]
Example:
  dws oa approval list-forms
  dws oa approval list-forms --cursor 0 --size 20
Flags:
      --cursor string   分页游标，首次传 0 (默认 0)
      --size string     每页大小 (可选)
```

列出当前用户被授权发起的审批表单（如"请假"、"报销"），返回 `processCode`，后续用于提交新的申请或按模板筛选历史实例。

### 获取审批实例详情
```
Usage:
  dws oa approval detail [flags]
Example:
  dws oa approval detail --instance-id <processInstanceId>
Flags:
      --instance-id string   审批实例 processInstanceId (必填)
```

拉取单个实例的完整内容：表单字段、附件、当前状态、参与人。适合让 AI 读懂审批单再决定动作或生成摘要。

### 查询审批实例的操作记录
```
Usage:
  dws oa approval records [flags]
Example:
  dws oa approval records --instance-id <processInstanceId>
Flags:
      --instance-id string   审批实例 processInstanceId (必填)
```

列出实例的流转历史（谁在什么时候同意/加签/转交/评论），用于复盘或解释"这单卡在哪"。

### 查询实例下待处理的任务
```
Usage:
  dws oa approval tasks [flags]
Example:
  dws oa approval tasks --instance-id <processInstanceId>
Flags:
      --instance-id string   审批实例 processInstanceId (必填)
```

针对具体实例拿到当前用户名下的 `taskId`。approve / reject 要的是 `taskId`，不是 `processInstanceId`，这一步不能省。

### 同意审批
```
Usage:
  dws oa approval approve [flags]
Example:
  dws oa approval approve --instance-id <processInstanceId> --task-id <taskId>
  dws oa approval approve --instance-id <processInstanceId> --task-id <taskId> --remark "同意，按流程推进"
Flags:
      --instance-id string   审批实例 processInstanceId (必填)
      --task-id string       待办任务 taskId (必填)，来自 approval tasks
      --remark string        审批意见 (可选)
```

以当前用户身份同意某个审批任务。`--task-id` 必须是当前用户的待办 taskId，否则 MCP 会拒绝。

### 拒绝审批
```
Usage:
  dws oa approval reject [flags]
Example:
  dws oa approval reject --instance-id <processInstanceId> --task-id <taskId> --remark "预算不足，下月再议"
Flags:
      --instance-id string   审批实例 processInstanceId (必填)
      --task-id string       待办任务 taskId (必填)，来自 approval tasks
      --remark string        拒绝理由 (可选，但强烈建议填)
```

以当前用户身份驳回审批任务；拒绝后一般由 MCP 端决定流程是打回申请人还是整体终止。

### 撤回已发起的审批
```
Usage:
  dws oa approval revoke [flags]
Example:
  dws oa approval revoke --instance-id <processInstanceId>
  dws oa approval revoke --instance-id <processInstanceId> --remark "金额填错，重新提交" --yes
Flags:
      --instance-id string   审批实例 processInstanceId (必填)
      --remark string        撤回原因 (可选)
```

只能撤销**自己发起且尚未终态**的实例。命令标了 `isSensitive`，AI 调用前建议与用户再确认一次，可配合全局 `--yes` 跳过确认。

## 意图判断

用户说"我要审的单/待我处理的审批/审批收件箱" → `list-pending` 拿 `processInstanceId`
用户说"这单我同意/批了它/通过" → `approval tasks` 换 `taskId` → `approval approve`
用户说"这单不行/驳回/拒绝" → `approval tasks` 换 `taskId` → `approval reject --remark "<理由>"`
用户说"这单详情/看看内容/这单在说什么" → `approval detail`
用户说"这单卡在谁那/谁还没审/流程走到哪" → `approval records`
用户说"我发起的审批/我的申请" → `list-initiated`（可用 `--process-code` 按模板筛）
用户说"能发起哪些审批/有哪些表单" → `list-forms`
用户说"撤回我的申请/我不提了" → `approval revoke`（仅限自己发起且未终态）

关键区分: oa(钉钉 OA 审批流程) vs todo(个人待办) vs report(日志汇报)

## 核心工作流

```bash
# 1. 看我有哪些待审的单 — 提取 processInstanceId
dws oa approval list-pending --page 1 --size 20 --format json

# 2. 看单子内容（表单字段 / 附件 / 当前状态）
dws oa approval detail --instance-id <processInstanceId> --format json

# 3. 想动作（同意/拒绝）前，先拿 taskId
dws oa approval tasks --instance-id <processInstanceId> --format json

# 4. 同意 / 拒绝（taskId 来自步骤 3）
dws oa approval approve --instance-id <processInstanceId> --task-id <taskId> --remark "同意" --format json
dws oa approval reject  --instance-id <processInstanceId> --task-id <taskId> --remark "预算不足" --format json

# 5. 查看流程记录 — 谁何时做了什么
dws oa approval records --instance-id <processInstanceId> --format json

# 6. 查我发起的审批 / 按模板过滤
dws oa approval list-initiated --format json
dws oa approval list-initiated --process-code <processCode> --format json

# 7. 能发起哪些表单
dws oa approval list-forms --cursor 0 --size 20 --format json

# 8. 撤回我发起的单
dws oa approval revoke --instance-id <processInstanceId> --remark "填错了" --yes --format json
```

## 上下文传递表

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `list-pending` | `processInstanceId`、`nextToken` | detail / tasks / records 的 --instance-id |
| `list-initiated` | `processInstanceId`、`nextToken` | detail / records / revoke 的 --instance-id |
| `list-forms` | `processCode` | `list-initiated --process-code` 按模板过滤，或后续发起新审批 |
| `approval tasks` | `taskId` | approve / reject 的 --task-id |
| `approval detail` | 表单字段、参与人、当前状态 | 供 AI 决策是否批准、生成摘要 |

## 注意事项

- `processInstanceId`（实例 ID）与 `taskId`（待办任务 ID）**不是一回事**，不能混用：
  - `list-pending` / `list-initiated` / `detail` / `records` / `revoke` 用 `processInstanceId`
  - `approve` / `reject` 用 `taskId`，必须先 `approval tasks --instance-id <processInstanceId>` 换一次
- `list-pending` 是"需要我动作"视图；同一个实例如果流转到下一节点就会从这里消失
- `list-initiated` 是"我发起的"视图；和 `list-pending` 是两套语义，不要互相替代
- `--start` / `--end` 使用 ISO-8601 格式，内部会自动转毫秒时间戳
- `revoke` 只能撤销**当前用户发起且未完结**的实例；已流转结束、已作废、或不是自己发起的都会失败
- `approve` / `reject` 的 `--remark` 可选但强烈建议填，尤其是 `reject`——审批流下游会把理由展示给申请人
- 所有敏感动作（`approve` / `reject` / `revoke`）在 AI 调用时应与用户二次确认，可用全局 `--yes` 跳过确认
- `list-forms` 返回的 `processCode` 是未来发起新审批、或筛选历史实例的钩子；目前本命令集只覆盖审批的"处理侧"，发起新审批需走业务方自己的流程
