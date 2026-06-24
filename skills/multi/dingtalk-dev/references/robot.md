# 机器人能力

> 机器人是应用的能力扩展之一；建号/配置在此，接到本地 agent 调试用 `dws dev connect`（见 connect.md）。

为开放平台企业内部应用创建和配置机器人。参数查 `dws schema dev.app.robot.<method>`。分两类场景：

1. 新建智能体机器人：异步创建一个新的 Agent 应用 + 承载机器人（`submit` / `result`），当前不绑定已有开放平台应用。
2. 现有应用配置机器人：在已存在的应用上配置/启用/停用机器人（`get` / `config`(upsert) / `enable` / `disable`），用 `--unified-app-id` 定位。

> `corpId` / `userId` 由系统上下文自动注入，CLI 不传。所有写操作先 `--dry-run`，确认后再 `--yes`。

## 一、新建智能体机器人（异步建号）

`submit` 提交任务拿 `taskId`，`result --task-id <taskId>` 轮询。`submit` 返回 `taskId/status/expiresInSeconds/intervalSeconds/retryCount/bindsUnifiedApp`，提交成功通常是 `WAITING`，且 `bindsUnifiedApp=false` 表示异步建号任务不挂到现有应用。失败重试：把上次 `taskId` 通过 `--task-id` 传回 `submit`，避免重复创建。`result` 返回 `SUCCESS` 或 `APPROVAL_REQUIRED` 时可能带 `agentId/robotCode/clientId/clientSecret`；凭证可用于本地建联，但线上搜索、加群、路由消息必须等版本发布到 `RELEASE`。

异步任务状态：

| status | 含义 | 下一步 |
|--------|------|--------|
| `WAITING` | 创建中 | 按 `intervalSeconds` 轮询 `robot result` |
| `SUCCESS` | 创建完成 | 保存 `robotCode/clientId/clientSecret`，凭据按敏感处理；若结果含明确 `unifiedAppId` 才继续版本发布，否则要求用户提供 |
| `APPROVAL_REQUIRED` | 已建号但线上使用需审核 | 不要重复建号；若结果含明确 `unifiedAppId` 才提交版本发布审核，否则要求用户提供 |
| `FAIL` | 创建失败 | 读 `errorCode/errorMsg/failReason`；可带原 `taskId` 重新 `submit` |
| `EXPIRED` | `taskId` 不存在或过期 | 重新 `submit` |

`robot result` 的 JSON 会额外补 `lifecycle` 与 `nextSteps`，用于把链路闭环到版本发布：

- 顶层 `completionState=BLOCKED_BY_VERSION_PUBLISH`、`mustContinue=true`、`terminal=false` 是硬门禁；看到它就继续执行 blocking `nextSteps`，不能把后续 `dev connect` 当完成。
- 顶层 `completionState=BLOCKED_BY_MISSING_UNIFIED_APP_ID`、`actionRequired=provide_unified_app_id` 时，说明缺少可安全写版本的应用主键；必须要求用户提供明确的 `unifiedAppId`，不能用 `clientId/appKey` 自动反查后继续写版本。
- 后续顺序是 `create_version` → `check_approval` → `publish_version` → `wait_release`。所有写操作仍先 `--dry-run`，确认后再 `--yes`。
- `check-approval` 若返回 `approvalMode=SELECT_APPROVER`，展示候选审批人的 `name/userId/mainAdmin`，等待用户选择后再把该 `userId` 传给 `publish --approver-user-id`；不要默认取第一个。
- `connect_local` 的命令只用 `<clientSecret-from-result>` 占位，不能把真实 `clientSecret` 写进回答或脚本；它是 `optional=true` / `scope=local_debug_only`，不能抵消版本发布审核。
- `lifecycle.overallComplete=false` 或版本未进入 `RELEASE` / `AUDIT` / `UNDER_REVIEW` 时，不要总结“全部完成”“机器人已创建并成功连接”“可以在钉钉中 @机器人使用”。只能说“本地建联成功，线上发布/审批未完成”或继续执行阻塞步骤。

完成态门禁规则的完整说明见 [SKILL.md](../SKILL.md)「核心规则」。

## 二、现有应用的机器人配置

`robot get` 返回机器人基础信息、回调、模式、状态、技能列表；应用尚未配置机器人时返回空态 `robotStatus=UNCONFIGURED`，不是业务错误。

状态判断：
- `robotStatus=UNCONFIGURED`：应用未配置机器人，走 `robot config`。
- `robotStatus=OFFLINE`：配置存在但停用/下线，可走 `robot enable`。
- `robotStatus=ONLINE`：配置已启用；`robotCode` 可用于加群、机器人身份发消息或后续建联。
- `mode` 是字符串枚举：`HTTPS` / `STREAM` / `AISKILL`。
- `robot get` 正常返回是平铺字段（`configured`/`mode`/`robotStatus`/`robotCode`/`name`/`brief`/`desc`），没有 `success` 字段；拿到这组字段就是配置已落库，不是异步等待态。
- ONLINE 只代表能力已开启。要让机器人自动处理消息，还需配 `--outgoing-url`/`--event-callback-url`，或用 `dev connect` 接本地 Agent（见 connect.md）。

`config` 是 upsert：建或改都用它，不存在则建、存在则改，至少给一个配置字段。国际化字段（`--i18n-name` 等）传 JSON，如 `'{"en_US":"Bot"}'`。`enable` 是纯启用：只开启能力，不带配置字段（只传 `--unified-app-id`）。`config/enable/disable` 成功统一返回 `success/operation/unifiedAppId/robotCode/robotStatus/configured`；回读 `robot get` 看到 `robotStatus=ONLINE` 就别再误判"待生效"。

## 错误处理

| 情况 | 处理 |
|------|------|
| `robotStatus=UNCONFIGURED` | 应用未配置机器人，先用 `robot config` 创建 |
| 应用名重复 | `app-name` 企业内需唯一，换名 |
| `ServiceResult.success=false` | 透传 `errorCode/errorMsg` |
| 创建任务 `EXPIRED` | 任务过期，重新 `submit`（可带原 taskId） |

> 把机器人接到本地 agent 调试/值守见 [connect.md](connect.md)。
