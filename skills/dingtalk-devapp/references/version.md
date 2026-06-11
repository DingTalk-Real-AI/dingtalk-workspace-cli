# 版本发布

管理开放平台企业内部应用的版本：基于当前配置创建版本、查看版本列表/详情、预检审批、发布、查状态。

> `corpId` / `userId` 由 MCP 系统上下文注入，CLI 不传。所有版本通过 `--unified-app-id` 定位，单个版本再加 `--version-id`。

## 典型流程

```text
permission add（requiredApproval=true 写入版本变更）
  → version create        创建版本
  → version check-approval 预检是否需要审批 / 审批人
  → version publish        发布（含高敏权限需 --confirm-sensitive）
  → version status         轮询发布/审批状态
```

## 创建版本

```bash
dws devapp version create --unified-app-id <unifiedAppId> --version 1.0.1 --desc "新增机器人能力" --dry-run --format json
dws devapp version create --unified-app-id <unifiedAppId> --version 1.0.1 --desc "新增机器人能力" --yes --format json
```

MCP tool: `create_open_dev_app_version`

| CLI | MCP | 说明 |
|-----|-----|------|
| `--version` | `version` | 版本号，如 1.0.1 |
| `--desc` | `description` | 版本描述 |

## 版本列表

```bash
dws devapp version list --unified-app-id <unifiedAppId> --page 1 --page-size 20 --format json
```

MCP tool: `list_open_dev_app_versions`（`--page`→`currentPage`，`--page-size`→`pageSize`）

新应用如果 `version list` 返回空，先执行 `version create`，用返回的 `versionId` 继续 `check-approval/publish`；不要因为列表为空误判无可发布内容。

## 版本详情

```bash
dws devapp version get --unified-app-id <unifiedAppId> --version-id <versionId> --format json
```

MCP tool: `get_open_dev_app_version_detail`。返回版本状态、描述、能力列表、权限点、敏感权限、审批要求和脱敏详情。

## 预检审批（不发布）

```bash
dws devapp version check-approval --unified-app-id <unifiedAppId> --version-id <versionId> --format json
```

MCP tool: `publish_open_dev_app_version`，CLI 强制 `dryRun=true`，仅返回审批要求和候选审批人，**不会实际发布**。

## 发布

```bash
dws devapp version publish --unified-app-id <unifiedAppId> --version-id <versionId> --dry-run --format json
dws devapp version publish --unified-app-id <unifiedAppId> --version-id <versionId> --yes --format json

# 含高敏权限时必须确认
dws devapp version publish --unified-app-id <unifiedAppId> --version-id <versionId> --confirm-sensitive --yes --format json

# 灰度选人模式指定审批人
dws devapp version publish --unified-app-id <unifiedAppId> --version-id <versionId> --approver <userId> --yes --format json
```

MCP tool: `publish_open_dev_app_version`，CLI 设 `dryRun=false`。

| CLI | MCP | 说明 |
|-----|-----|------|
| `--confirm-sensitive` | `confirmedSensitive` | 版本含高敏权限时必须确认 |
| `--approver` | `approverUserId` | 灰度选人模式指定审批人 userId |

> 注意：`--dry-run` 是 CLI 层的"预览不执行"开关；服务端的"审批预检"是 `version check-approval`（对应 `dryRun=true`）。二者不同，发布前建议先 `check-approval`。

发布响应 `result`：

| result | 含义 | 下一步 |
|--------|------|--------|
| `NOT_REQUIRED` | 不需要审批；预检时表示可直接发布，真实发布时表示已直接发布上线 | 真实发布后回读 `version status/get` 验证 `RELEASE` |
| `APPROVAL_REQUIRED` | 需要审批，通常出现在 `check-approval` 或未指定审批人时 | 查看 `approvalMode/approvalCandidates`，让用户选择审批人后再 `publish --approver` |
| `SUBMITTED` | 已提交审批 | 保存 `processId`，轮询 `version status` |

审批模式 `approvalMode`：

| approvalMode | 含义 | 下一步 |
|--------------|------|--------|
| `SELECT_APPROVER` | 灰度选人模式，需要在候选审批人中选一个 | 展示候选审批人，不自动选第一个 |
| `ENTERPRISE_SELF_BUILT` | 企业自建审核模式 | 不传 `--approver`，按企业自建审核流程等待 |

## 版本状态

```bash
dws devapp version status --unified-app-id <unifiedAppId> --version-id <versionId> --format json
```

MCP tool: `get_open_dev_app_version_status`。返回版本状态、流程实例 ID、审批状态和审批意见。审批详情可能只在钉钉客户端可见。

版本状态字段：`version create/list/get` 返回 `status`，`version status` 返回 `versionStatus`，二者都对齐版本枚举。

| status/versionStatus | 含义 | 下一步 |
|----------------------|------|--------|
| `INIT` | 版本已创建或有待发布变更，尚未发布 | 可 `check-approval` / `publish` |
| `AUDIT` | 发布审核中 | 不要重复发布；即使没有返回 `processStatus`，也按审核中处理 |
| `RELEASE` | 已发布生效 | 发布完成，可继续验证权限、机器人、网页应用等能力 |
| `GRAY` | 灰度状态 | 按灰度流程处理；不要当全量已发布 |

审批流程状态字段：`version status` 的 `processStatus` 只在存在审批流程且后端透出流程态时有值。`versionStatus=AUDIT` 但没有 `processStatus/processInstanceId` 时，不要判失败，仍表示审核中。

| processStatus | 含义 | 下一步 |
|---------------|------|--------|
| `UNDER_REVIEW` | 审批中 | 等待审批，必要时把 `processInstanceId` 给用户去钉钉客户端查看 |
| `PASS` | 审批通过 | 继续回读版本，确认是否进入 `RELEASE` |
| `FAIL` | 审批拒绝 | 展示 `processComment`，修改后重新创建/发布版本 |
| `WITHDRAW` / `CANCEL` | 审批撤回或取消 | 回到发布前状态，重新 `check-approval` / `publish` |
| `PUBLISH_FAILED` | 审批后发布失败 | 展示错误信息，重新检查版本状态和后端错误 |

遇到未列出的状态值时，不要猜测语义；原样展示状态，并回读 `version get/status` 或查开放平台文档/后台详情。

## 错误处理

| 情况 | 处理 |
|------|------|
| `check-approval` 提示需审批 | 按返回选审批人，再 `publish --approver` |
| 发布报高敏权限未确认 | 加 `--confirm-sensitive` 重新发布 |
| `ServiceResult.success=false` | 透传 `errorCode/errorMsg` |
