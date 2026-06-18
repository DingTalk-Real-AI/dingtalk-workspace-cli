# 版本发布

> 版本是配置变更生效的唯一通道——改配置不等于上线，发布到 RELEASE 才生效（见 SKILL.md 生效模型）。

管理应用版本：基于当前配置建版本、查列表/详情、预检审批、发布、查状态。参数查 `dws schema dev.app.version.<method>`。版本用 `--unified-app-id` 定位，单个版本再加 `--version-id`；`corpId`/`userId` 系统注入，CLI 不传。

## 典型流程

```text
permission add（requiredApproval=true 写入版本变更）
  → version create        创建版本
  → version check-approval 预检是否需审批 / 审批人
  → version publish        发布（含高敏权限需 --confirmed-sensitive）
  → version status         轮询发布/审批状态
```

新应用如果 `version list` 返回空，先 `version create`，用返回的 `versionId` 继续 check-approval/publish；不要因列表空误判无可发布内容。

## check-approval 与 publish

`check-approval` 只查审批要求和候选审批人，不发布。`publish` 是真实发布；含高敏权限要加 `--confirmed-sensitive`，灰度选人模式用 `--approver-user-id` 指定审批人。

区分两个"预检"：`--dry-run` 是 CLI 层的预览不调上游；`check-approval` 是服务端查审批要求不发布。发布前建议先 `check-approval`。

发布响应 `result`：

| result | 含义 | 下一步 |
|--------|------|--------|
| `NOT_REQUIRED` | 不需审批；预检=可直接发布，真实发布=已直接上线 | 回读 `version status/get` 验证 `RELEASE` |
| `APPROVAL_REQUIRED` | 需审批，常见于 `check-approval` 或未指定审批人 | 看 `approvalMode/approvalCandidates`，让用户选审批人后再 `publish --approver-user-id` |
| `SUBMITTED` | 已提交审批 | 保存 `processId`，轮询 `version status` |

审批模式 `approvalMode`：

| approvalMode | 含义 | 下一步 |
|--------------|------|--------|
| `SELECT_APPROVER` | 灰度选人，需在候选审批人里选一个 | 展示候选，不自动取第一个 |
| `ENTERPRISE_SELF_BUILT` | 企业自建审核 | 不传 `--approver-user-id`，按企业自建流程等待 |

## 版本状态

`version create/list/get/status` 统一返回 `versionStatus`：

| versionStatus | 含义 | 下一步 |
|---------------|------|--------|
| `INIT` | 已创建或有待发布变更，未发布 | 可 `check-approval`/`publish` |
| `AUDIT` | 发布审核中 | 不要重复发布；即使没返回 `processStatus` 也按审核中处理 |
| `RELEASE` | 已发布生效 | 完成，可验证权限/机器人/网页等能力 |
| `GRAY` | 灰度 | 按灰度流程，不要当全量已发布 |

`version status` 的 `processStatus` 只在存在审批流程且后端透出时有值。`versionStatus=AUDIT` 但没 `processStatus/processInstanceId` 时不要判失败，仍是审核中。

| processStatus | 含义 | 下一步 |
|---------------|------|--------|
| `UNDER_REVIEW` | 审批中 | 等待，必要时把 `processInstanceId` 给用户去钉钉客户端看 |
| `PASS` | 审批通过 | 继续回读，确认是否进 `RELEASE` |
| `FAIL` | 审批拒绝 | 展示 `processComment`，改后重新建/发版本 |
| `WITHDRAW` / `CANCEL` | 撤回或取消 | 回到发布前，重新 `check-approval`/`publish` |
| `PUBLISH_FAILED` | 审批后发布失败 | 展示错误，重查版本状态和后端错误 |

遇到未列出的状态值，不要猜语义；原样展示，回读 `version get/status` 或查文档/后台。

## 错误处理

| 情况 | 处理 |
|------|------|
| `check-approval` 提示需审批 | 按返回选审批人，再 `publish --approver-user-id` |
| 发布报高敏权限未确认 | 加 `--confirmed-sensitive` 重新发布 |
| `ServiceResult.success=false` | 透传 `errorCode/errorMsg` |
