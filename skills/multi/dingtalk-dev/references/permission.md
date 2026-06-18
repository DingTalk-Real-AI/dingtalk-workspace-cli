# 权限管理

> 权限点 scopeValue 是授权单元，一个权限点授权一组 OpenAPI；requiredApproval=true 的变更走版本通道生效（见 SKILL.md 生效模型）。

查询、申请、取消开放平台应用的 APP 应用权限和 SNS 个人权限。参数查 `dws schema dev.app.permission.<method>`。

## 权限列表

`--scope-value` 传入即进单权限详情模式；`--scope-type` 取 `APP`/`SNS`，留空返回两者；一个应用可能 150+ 权限点，游标分页续翻、`--page-size` 不超过 50。

`--auth-status` 是查询过滤条件：

| authStatus | 含义 |
|------------|------|
| `ALL` | 不按授权状态过滤 |
| `AUTHED` | 只看已授权/已开通 |
| `UNAUTHED` | 只看未授权/未开通 |

单个权限项返回的 `status` 是内部操作态：

| status | 枚举 | 含义 | 下一步 |
|--------|------|------|--------|
| `0` | `STATUS_OBTAINED` | 权限已获得 | 不要重复申请；如需取消，确认 `canRemove=true` 后走 `permission remove` |
| `1` | `STATUS_APPLYING` | 权限申请中 | 不要重复申请；看 `authedStatusDesc`，通常等待审批或版本发布 |
| `2` | `STATUS_CAN_APPLY` | 可以申请 | 走 `permission add` |
| `3` | `STATUS_CAN_NOT_APPLY` | 不可申请 | 停止，展示 `applyDisabledReason/displayMessage` |

`authedStatusDesc` 是给用户看的细分状态：`OPENED`/`APPLIED`/`TO_BE_PUBLISHED`=已开通/已申请/待发布；`NOT_OPEN`/`NOT_APPLIED`=未开通/未申请；`AUDIT_PROCESSING`=审批中；`AUDIT_REFUSE`=审批未通过。能否操作仍以 `status`、`canEdit`、`canApplyDirectly`、`allowedActions` 为准。

list 默认同时返回 APP 和 SNS 权限；列表模式只返回 `apiPreview`，`--scope-value` 详情模式返回完整 `apiList`。`permission search`/`detail` 是 `list` 的别名。

scopeValue 选择顺序：
1. 用户给了 `scopeValue`，精确匹配
2. 给了 API 名，用 `keyword` 搜，匹配 `apiPreview.name`
3. 给了权限名，匹配 `scopeName/scopeDesc`
4. 多个候选，展示列表让用户选，不自动取第一条

## 申请权限

`--scope-values` 传 `scopeValue`，多个逗号分隔，必须来自 `permission list` 返回。已开通跳过、不可编辑拒绝。`requiredApproval=true` 允许申请——写入版本变更，审批在版本发布时处理。不在此处选审批人。

## 取消权限

`--scope-values` 多个逗号分隔；上游一次只取消一个权限点，多条时 CLI 逐条调用并返回 `results` 聚合数组。未开通返回 `NOT_AUTHED`；不可编辑返回 no-edit 原因。
