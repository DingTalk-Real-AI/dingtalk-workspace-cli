# 单命令产品合集

以下产品命令较少，合并参考。

---

## devdoc — 开放平台文档

### 搜索开放平台文档
```
Usage:
  dws devdoc article search [flags]
Example:
  dws devdoc article search --keyword "OAuth2 接入" --page 1 --size 10
Flags:
      --keyword string   搜索关键词 (必填)
      --page string      页码 (默认 1)
      --size string      每页数量 (默认 10)
```

### 搜索开放平台错误码
> ⚠️ 真实路径是 `dws devdoc search-open-platform-error-code`（顶级），不是 `devdoc article search-error`。

```
Usage:
  dws devdoc search-open-platform-error-code [flags]
Example:
  dws devdoc search-open-platform-error-code --error-code "88" --format json
Flags:
      --error-code string   错误码或关键词 (必填)
```

---

## oa — 审批

### approval 子树（基于 processInstanceId 的审批操作）

#### 查询可见审批流程
```
Usage:
  dws oa approval list-forms [flags]
Example:
  dws oa approval list-forms --size 20
  dws oa approval list-forms --cursor <nextCursor> --size 20
Flags:
      --size string     每页数量 pageSize
      --cursor string   分页游标
```

#### 查询审批实例详情
```
Usage:
  dws oa approval detail [flags]
Example:
  dws oa approval detail --instance-id <processInstanceId> --format json
Flags:
      --instance-id string   审批实例 ID processInstanceId (必填)
```

#### 查询审批记录（操作日志）
```
Usage:
  dws oa approval records [flags]
Example:
  dws oa approval records --instance-id <processInstanceId> --format json
Flags:
      --instance-id string   审批实例 ID processInstanceId (必填)
```

#### 查询审批实例内的任务列表
> 此命令按 **instance-id** 查指定实例下的所有任务，而不是「我的待办」。查「我的待办」用 `oa approval list-pending` 或 `oa get-todo-tasks`。

```
Usage:
  dws oa approval tasks [flags]
Example:
  dws oa approval tasks --instance-id <processInstanceId> --format json
Flags:
      --instance-id string   审批实例 ID processInstanceId (必填)
```

#### 查询待我处理的审批（approval 版）
```
Usage:
  dws oa approval list-pending [flags]
Example:
  dws oa approval list-pending --start "2026-04-01 00:00:00" --end "2026-04-30 23:59:59" --page 1 --size 20
Flags:
      --start string   起始时间 starTime
      --end string     结束时间 endTime
      --page string    页码 pageNum
      --size string    每页数量 pageSize
```

#### 查询我发起的审批（approval 版）
```
Usage:
  dws oa approval list-initiated [flags]
Example:
  dws oa approval list-initiated --start "2026-04-01T00:00:00+08:00" --end "2026-04-30T23:59:59+08:00" --max-results 20
  dws oa approval list-initiated --process-code <processCode> --start "..." --end "..."
  dws oa approval list-initiated --start "..." --end "..." --next-token <nextToken>
Flags:
      --start string          起始时间 startTime
      --end string            结束时间 endTime
      --process-code string   审批流 processCode（可选，按审批类型筛选）
      --max-results string    单次最大返回数量 maxResults
      --next-token string     分页游标 nextToken
```

#### 同意审批
```
Usage:
  dws oa approval approve [flags]
Example:
  dws oa approval approve --instance-id <ID> --task-id <TASK_ID> --remark "同意" --format json
Flags:
      --instance-id string   审批实例 ID processInstanceId (必填)
      --task-id string       任务 ID taskId (必填)
      --remark string        审批意见（可选）
```

#### 拒绝审批
```
Usage:
  dws oa approval reject [flags]
Example:
  dws oa approval reject --instance-id <ID> --task-id <TASK_ID> --remark "不符合要求" --format json
Flags:
      --instance-id string   审批实例 ID processInstanceId (必填)
      --task-id string       任务 ID taskId (必填)
      --remark string        拒绝理由（建议填写）
```

#### 撤销审批
```
Usage:
  dws oa approval revoke [flags]
Example:
  dws oa approval revoke --instance-id <ID> --remark "补充材料" --format json
Flags:
      --instance-id string   审批实例 ID processInstanceId (必填)
      --remark string        撤销原因（可选）
```

### oa 顶级命令（基于 userId 的任务查询，与 approval 子树功能有交集）

#### 查询我的待办任务
```
Usage:
  dws oa get-todo-tasks [flags]
Example:
  dws oa get-todo-tasks --user-id <userId> --page-number 1 --page-size 20
  dws oa get-todo-tasks --user-id <userId> --create-before "2026-04-30 23:59:59"
Flags:
      --user-id string         用户 userId
      --create-before string   仅返回该时间之前创建的任务 createBefore
      --page-number string     页码 pageNumber
      --page-size string       每页数量 pageSize
```

#### 查询我的已办任务
```
Usage:
  dws oa get-done-tasks [flags]
Example:
  dws oa get-done-tasks --user-id <userId> --page-number 1 --page-size 20
Flags:
      --user-id string       用户 userId
      --page-number string   页码
      --page-size string     每页数量
```

#### 查询我发起的审批实例（顶级版）
```
Usage:
  dws oa get-submitted-instances [flags]
Example:
  dws oa get-submitted-instances --user-id <userId> --page-number 1 --page-size 20
Flags:
      --user-id string       用户 userId
      --page-number string   页码
      --page-size string     每页数量
```

#### 查询抄送给我的审批实例
```
Usage:
  dws oa get-noticed-instances [flags]
Example:
  dws oa get-noticed-instances --user-id <userId> --page-number 1 --page-size 20
Flags:
      --user-id string       用户 userId
      --page-number string   页码
      --page-size string     每页数量
```

#### 查询待我审批（抄送变体，按创建时间范围）
```
Usage:
  dws oa list-pending-approvals-for-me [flags]
Example:
  dws oa list-pending-approvals-for-me --create-time-from "2026-04-01 00:00:00" --create-time-to "2026-04-30 23:59:59" --page-num 1 --page-size 20
Flags:
      --create-time-from string   起始创建时间 createTimeFrom
      --create-time-to string     结束创建时间 createTimeTo
      --page-num string           页码 pageNum
      --page-size string          每页数量 pageSize
```

---

## 意图判断

- 用户说"开发文档/API 文档/接口文档" → `devdoc article search`
- 用户说"API 报错/错误码查询" → `devdoc search-open-platform-error-code`
- 用户说"审批/请假/报销/出差/查审批" → `oa approval *` 或 `oa get-*`
- 用户说"同意审批/批准" → `oa approval approve`
- 用户说"拒绝审批/驳回" → `oa approval reject`
- 用户说"撤销审批/撤回" → `oa approval revoke`
- 用户说"待我审批/我要审批的" → 优先 `oa approval list-pending`（支持时间范围）或 `oa get-todo-tasks`（按 userId）
- 用户说"我发起的审批/我提交的" → `oa approval list-initiated` 或 `oa get-submitted-instances`
- 用户说"我已办的审批/处理过的" → `oa get-done-tasks`
- 用户说"抄送给我的审批/知会我的" → `oa get-noticed-instances`

### 关键区分

- `oa approval tasks --instance-id` — **查指定实例下的任务列表**，不是我的待办
- `oa approval list-pending` — 我的待办（按时间范围）
- `oa approval list-initiated` — 我发起的（按时间范围 + processCode 可选）
- `oa get-todo-tasks` / `oa get-done-tasks` — 基于 userId 的顶级简化接口
- `oa get-submitted-instances` ≈ `oa approval list-initiated`，顶级版不支持 processCode 过滤
- `oa list-pending-approvals-for-me` — 变体，按创建时间范围过滤

## 上下文传递表

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `devdoc article search` | 文档链接 | 直接展示给用户 |
| `oa approval list-forms` | `processCode` | `approval list-initiated --process-code` |
| `oa approval list-pending` / `oa get-todo-tasks` | `taskId`, `processInstanceId` | `approval approve` / `reject` 的 `--task-id` + `--instance-id` |
| `oa approval list-initiated` / `get-submitted-instances` | `processInstanceId` | `approval detail` / `records` / `revoke` 的 `--instance-id` |
| `oa approval tasks` | `taskId` | `approval approve` / `reject` 的 `--task-id` |

## 注意事项

- **路径易错**：搜错误码是 `devdoc search-open-platform-error-code`（顶级），不是 `devdoc article search-error`
- **oa approval tasks 语义易错**：它是「查某个实例下的任务」，**不是**「我的待办」
- **待我处理** 推荐用 `oa approval list-pending`（可按时间范围过滤）；简化接口用 `oa get-todo-tasks`（按 userId）
- **同意/拒绝/撤销** 都是敏感操作，agent 模式下执行前必须向用户确认，同意后才加 `--yes`
- 所有数值/日期 flag 真实类型都是 `string`（自动生成 stub），按示例传字符串即可
