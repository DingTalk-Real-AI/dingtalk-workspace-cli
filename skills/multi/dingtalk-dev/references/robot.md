# 机器人能力

> 机器人是应用的能力扩展之一；建号/配置在此，接到本地 agent 调试用 `dws dev connect`（见 connect.md）。

为开放平台企业内部应用创建和配置机器人。参数查 `dws schema dev.app.robot.<method>`。分两类场景：

1. 新建智能体机器人：异步创建一个新的 Agent 应用 + 承载机器人（`submit` / `result`）。同步建号已下线，统一走异步。
2. 现有应用配置机器人：在已存在的应用上配置/启用/停用机器人（`get` / `config`(upsert) / `enable` / `disable`），用 `--unified-app-id` 定位。

> `corpId` / `userId` 由系统上下文自动注入，CLI 不传。所有写操作先 `--dry-run`，确认后再 `--yes`。

## 一、新建智能体机器人（异步建号）

`submit` 提交任务拿 `taskId`，`result --task-id <taskId>` 轮询。`submit` 返回 `taskId/status/expiresIn/interval/retryCount`，提交成功通常是 `WAITING`。失败重试：把上次 `taskId` 通过 `--task-id` 传回 `submit`，避免重复创建。只有 `result` 返回 `SUCCESS` 才能用返回的 `agentId/robotCode/clientId/clientSecret`。

异步任务状态：

| status | 含义 | 下一步 |
|--------|------|--------|
| `WAITING` | 创建中 | 按 `interval` 轮询 `robot result` |
| `SUCCESS` | 创建完成 | 保存 `robotCode/clientId/clientSecret`，凭据按敏感处理 |
| `APPROVAL_REQUIRED` | 创建编排需审批 | 不要重复建号；按返回信息或后台审批后再继续 |
| `FAIL` | 创建失败 | 读 `errorCode/errorMsg/failReason`；可带原 `taskId` 重新 `submit` |
| `EXPIRED` | `taskId` 不存在或过期 | 重新 `submit` |

## 二、现有应用的机器人配置

`robot get` 返回机器人基础信息、回调、模式、状态、技能列表；应用尚未配置机器人时后端返回 `robot info is not exist`。

状态判断：
- `status=1`：OFFLINE，配置存在但停用/下线。
- `status=2`：ONLINE，配置已生效；`robotCode` 可用于加群、机器人身份发消息或后续建联。
- `robot get` 返回 `success=true` 且含 `robotCode` = 配置已落库，不是异步等待态。
- ONLINE 只代表能力已开启。要让机器人自动处理消息，还需配 `--outgoing-url`/`--event-callback-url`，或用 `dev connect` 接本地 Agent（见 connect.md）。
- 未配置机器人时不返回 `status`，而是业务错误 `robot info is not exist`；这时走 `robot config`，不是 `enable`。

`config` 是 upsert：建或改都用它，不存在则建、存在则改，至少给一个配置字段。国际化字段（`--i18n-name` 等）传 JSON，如 `'{"en_US":"Bot"}'`。`enable` 是纯启用：只开启能力，不带配置字段（只传 `--unified-app-id`）。旧的独立 `update` 已并入 `config`。`config` 成功后回读 `robot get`：返回 `status=2` 就别再误判"待生效"，只有 `status=1` 或要重新上架才调 `enable`。

## 错误处理

| 情况 | 处理 |
|------|------|
| `robot info is not exist` | 应用未配置机器人，先用 `robot config` 创建 |
| 应用名重复 | `app-name` 企业内需唯一，换名 |
| `ServiceResult.success=false` | 透传 `errorCode/errorMsg` |
| 创建任务 `EXPIRED` | 任务过期，重新 `submit`（可带原 taskId） |

> 把机器人接到本地 agent 调试/值守见 [connect.md](connect.md)。
