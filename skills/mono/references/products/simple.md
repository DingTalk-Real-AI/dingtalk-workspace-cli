# 单命令产品合集

以下产品命令较少，合并参考。

---

## devdoc — 开放平台文档

### 搜索开放平台文档
```
Usage:
  dws devdoc article search [flags]
Example:
  dws devdoc article search --query "OAuth2 接入" --page 1 --size 10
Flags:
      --query string   搜索关键词 (必填)
      --page string    页码 (默认 1)
      --cursor string  分页游标，翻页传上次返回的 nextCursor；传入后不再使用 --page
      --size string    每页数量 (默认 10)
```

### 排查开放平台调用错误
```
Usage:
  dws devdoc error diagnose [flags]
Example:
  dws devdoc error diagnose --error-code "40014" --query "获取用户信息 access_token" --format json
  dws devdoc error diagnose --request-id "<REQUEST_ID>" --query "接口调用失败" --format json
Flags:
      --query string           原始排查问题
      --error-code string      错误码
      --error-message string   错误描述
      --api string             API 名称
      --context string         额外排查上下文
      --request-id string      开放平台 requestId
      --cursor string          分页游标，翻页传上次返回的 nextCursor；传入后不再使用 --page
```

---

## oa — 审批

### 查询可见审批流程
```
Usage:
  dws oa approval list-forms [flags]
Example:
  dws oa approval list-forms --format json
```

### 查询审批实例详情
```
Usage:
  dws oa approval detail --instance-id <ID> [flags]
Example:
  dws oa approval detail --instance-id <ID> --format json
```

### 查询审批记录
```
Usage:
  dws oa approval records --instance-id <ID> [flags]
Example:
  dws oa approval records --instance-id <ID> --format json
```

### 查询待我审批的任务
```
Usage:
  dws oa approval tasks [flags]
Example:
  dws oa approval tasks --format json
```

### 查询待我处理的审批
```
Usage:
  dws oa approval list-pending [flags]
Example:
  dws oa approval list-pending --format json
```

### 查询我发起的审批
```
Usage:
  dws oa approval list-initiated [flags]
Example:
  dws oa approval list-initiated --format json
```

### 同意审批
```
Usage:
  dws oa approval approve --instance-id <ID> --task-id <TASK_ID> [flags]
Example:
  dws oa approval approve --instance-id <ID> --task-id <TASK_ID> --format json
```

### 拒绝审批
```
Usage:
  dws oa approval reject --instance-id <ID> --task-id <TASK_ID> [flags]
Example:
  dws oa approval reject --instance-id <ID> --task-id <TASK_ID> --remark "不符合要求" --format json
```

### 撤销审批
```
Usage:
  dws oa approval revoke --instance-id <ID> [flags]
Example:
  dws oa approval revoke --instance-id <ID> --format json
```

---

## 意图判断

- 用户说"开发文档/API 文档/接口文档" → `devdoc article search`
- 用户说"调用报错/错误码/requestId" → `devdoc error diagnose`；若返回 `PARAM_ERROR - 未找到指定工具`，降级 `devdoc article search` 并标记后端工具注册待闭环
- 用户说"审批/请假/报销/出差" → `oa approval`
- 用户说"同意审批/批准" → `oa approval approve`
- 用户说"拒绝审批/驳回" → `oa approval reject`
- 用户说"撤销审批/撤回" → `oa approval revoke`
- 用户说"待我审批/我要审批的" → `oa approval list-pending` 或 `oa approval tasks`
- 用户说"我发起的审批" → `oa approval list-initiated`

## 上下文传递表

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `devdoc article search` | 文档链接、nextCursor | 直接展示给用户；hasMore=true 时用 --cursor 翻页 |
| `devdoc error diagnose` | 诊断材料/文档链接、nextCursor | 排查错误码；hasMore=true 时用 --cursor 翻页；不可用时退回文档搜索 |
| `oa approval list-forms` | processCode | detail / records 等 |
| `oa approval tasks` | taskId, instanceId | approve / reject |
| `oa approval list-pending` | instanceId | detail / approve / reject |
| `oa approval list-initiated` | instanceId | detail / revoke |
