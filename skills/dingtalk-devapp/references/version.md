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

## 版本状态

```bash
dws devapp version status --unified-app-id <unifiedAppId> --version-id <versionId> --format json
```

MCP tool: `get_open_dev_app_version_status`。返回版本状态、流程实例 ID、审批状态和审批意见。审批详情可能只在钉钉客户端可见。

## 错误处理

| 情况 | 处理 |
|------|------|
| `check-approval` 提示需审批 | 按返回选审批人，再 `publish --approver` |
| 发布报高敏权限未确认 | 加 `--confirm-sensitive` 重新发布 |
| `ServiceResult.success=false` | 透传 `errorCode/errorMsg` |
