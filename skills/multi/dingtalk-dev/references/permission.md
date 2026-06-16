# 权限管理

> 概念锚点：权限点 scopeValue 是授权单元，一个权限点授权一组 OpenAPI；requiredApproval=true 的变更走版本通道生效（见 SKILL.md 生效模型）。

查询、申请、取消开放平台应用的 APP 应用权限和 SNS 个人权限。

## 权限列表

```bash
dws dev app permission list --unified-app-id ID --format json
dws dev app permission list --unified-app-id ID --keyword "机器人发消息" --status UNAUTHED --format json
dws dev app permission list --unified-app-id ID --scope-type SNS --format json
dws dev app permission list --unified-app-id ID --scope qyapi_robot_sendmsg --format json
```

| CLI | 说明 |
|-----|------|
| `--unified-app-id` | 应用定位 |
| `--keyword` | 权限名/API 名关键词 |
| `--status` | `ALL` / `AUTHED` / `UNAUTHED` |
| `--scope-type` | `APP` / `SNS`，为空返回两者 |
| `--scope` | 单权限详情模式 |
| `--cursor` | 游标令牌：首次留空，续翻传上次 `nextCursor` |
| `--page-size` | 单页条数，默认 20，建议不超过 50 |

**状态判断：**

`--status` 是查询过滤条件：

| authStatus | 含义 |
|------------|------|
| `ALL` | 不按授权状态过滤 |
| `AUTHED` | 只看已授权/已开通的权限点 |
| `UNAUTHED` | 只看未授权/未开通的权限点 |

单个权限项返回的 `status` 是内部操作态：

| status | 枚举 | 含义 | 下一步 |
|--------|------|------|--------|
| `0` | `STATUS_OBTAINED` | 权限已获得 | 不要重复申请；如需取消，确认 `canRemove=true` 后走 `permission remove` |
| `1` | `STATUS_APPLYING` | 权限申请中 | 不要重复申请；查看 `authedStatusDesc`，通常等待审批或版本发布 |
| `2` | `STATUS_CAN_APPLY` | 权限可以申请 | 可走 `permission add --dry-run` |
| `3` | `STATUS_CAN_NOT_APPLY` | 权限不可以申请 | 停止申请，展示 `applyDisabledReason/displayMessage` |

`authedStatusDesc` 是给用户看的细分状态：`OPENED`/`APPLIED`/`TO_BE_PUBLISHED` 表示已开通、已申请或待发布；`NOT_OPEN`/`NOT_APPLIED` 表示未开通/未申请；`AUDIT_PROCESSING` 表示审批中；`AUDIT_REFUSE` 表示审批未通过。判断能否操作仍以 `status`、`canEdit`、`canApplyDirectly`、`allowedActions` 为准。

**翻页：**

权限列表用游标分页（与 app/version list 一致）。一个应用可能有 150+ 个权限点，逐页用 `nextCursor` 续翻：

```bash
dws dev app permission list --unified-app-id ID --page-size 50 --format json
# → 出参 nextCursor=<token>，续翻：
dws dev app permission list --unified-app-id ID --cursor <token> --page-size 50 --format json
```

出参 `nextCursor` 为空（或 `hasMore=false`）时表示已到末尾。

**规则：**
- `permission search` 和 `permission detail` 是 `list` 的 CLI 别名。
- 默认同时返回 APP 和 SNS 权限。
- 列表模式只返回 `apiPreview`；`--scope` 详情模式返回完整 `apiList`。

**scopeValue 选择顺序：**

1. 用户给了 `scopeValue` → 精确匹配
2. 用户给了 API 名 → `keyword` 搜索，匹配 `apiPreview.name`
3. 用户给了权限名 → 匹配 `scopeName/scopeDesc`
4. 多个候选 → 展示列表让用户选择，不自动取第一条

## 申请权限

```bash
dws dev app permission add --unified-app-id ID --permissions qyapi_robot_sendmsg --dry-run --format json
dws dev app permission add --unified-app-id ID --permissions Contact.User.mobile,qyapi_robot_sendmsg --yes --format json
```

**规则：**
- `--permissions` 传 `scopeValue`，多个逗号分隔，必须来自 `permission list` 的返回。
- 已开通跳过，不可编辑拒绝。
- `requiredApproval=true` 允许申请——写入版本变更，审批在版本发布时处理。
- 不在此处选审批人。

## 取消权限

```bash
dws dev app permission remove --unified-app-id ID --permissions qyapi_robot_sendmsg --dry-run --format json
dws dev app permission remove --unified-app-id ID --permissions qyapi_robot_sendmsg --yes --format json
```

`--permissions` 多个逗号分隔；上游一次只取消一个权限点，多条时 CLI 逐条调用并返回 `results` 聚合数组。未开通返回 `NOT_AUTHED`；不可编辑返回 no-edit 原因。
