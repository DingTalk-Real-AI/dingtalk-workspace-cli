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

单个权限项的状态看这几个字段：

- `authed`（布尔）：是否已授权/已开通。true=已开通，不要重复申请。
- `allowedActions`（数组）：本权限点当前允许的动作，如 `["view","detail","apply"]`。含 `apply` 才能申请，含 `remove` 才能取消。
- `authedStatusDesc`（中文文案）：状态的中文说明，如"已开通"/"未开通"，直接展示给用户。
- `apiStatus`：权限点本身的开放状态，如 `FULLY_OPEN`。
- `requiredApproval`（布尔）：申请是否需审批。true 的变更走版本通道，审批在版本发布时处理。
- `displayMessage`（中文文案）：服务端给的提示语，能否申请的原因看它。

list 默认同时返回 APP 和 SNS 权限；列表模式和 `--scope-value` 详情模式，权限项里的 API 信息字段都叫 `apiPreview`。`permission search` 是 `list` 的别名。

scopeValue 选择顺序：
1. 用户给了 `scopeValue`，精确匹配
2. 给了 API 名，用 `keyword` 搜，匹配 `apiPreview.name`
3. 给了权限名，匹配 `scopeName/scopeDesc`
4. 多个候选，展示列表让用户选，不自动取第一条

## 申请权限

`--scope-values` 传 `scopeValue`，多个逗号分隔，必须来自 `permission list` 返回。已开通跳过、不可编辑拒绝。`requiredApproval=true` 允许申请——写入版本变更，审批在版本发布时处理。不在此处选审批人。

## 取消权限

`--scope-values` 多个逗号分隔。返回：`removed`（布尔，整体成败）、`removedScopeValues`（成功取消的）、`rejectedScopeValues`（被拒的）、`message`。逐条看 `removedScopeValues`/`rejectedScopeValues` 判断每个权限点的结果。
