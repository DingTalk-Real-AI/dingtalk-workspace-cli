# 机器人能力

> 概念锚点：机器人是应用的能力扩展之一；建号/配置在此，接到本地 agent 调试用 `dws dev connect`（见 connect.md）。

为开放平台企业内部应用创建和配置机器人。分两类场景：

1. **新建智能体机器人**：异步创建一个新的 Agent 应用 + 承载机器人（`submit` / `result`）。同步建号（一次性创建应用+机器人）已下线，统一走异步。
2. **现有应用配置机器人**：在已存在的应用上配置/启用/停用机器人（`get` / `config`(upsert) / `enable` / `disable`），通过 `--unified-app-id` 定位。

> `corpId` / `userId` 由系统上下文自动注入，CLI 不传。所有写操作先 `--dry-run`，确认后再 `--yes`。

## 一、新建智能体机器人（异步建号）

同步建号已下线，新建智能体一律用异步 `submit` 提交任务 + `result` 轮询。也适合创建耗时较长或需要失败重试的场景。

```bash
# 提交任务
dws dev app robot submit --name 我的智能体 --robot-name 小助手 --desc "处理审批问答" --dry-run --format json
dws dev app robot submit --name 我的智能体 --robot-name 小助手 --desc "处理审批问答" --yes --format json
# → 返回 taskId

# 轮询结果
dws dev app robot result --task-id <taskId> --format json
```

- `submit` 返回 `taskId / status / expiresIn / interval / retryCount`，提交成功后通常是 `WAITING`。
- 失败重试：把上次的 `taskId` 通过 `--task-id` 传入 `submit`，避免重复创建。
- `result` 返回任务状态；只有 `SUCCESS` 时才能使用返回的 `agentId / robotCode / clientId / clientSecret`。

异步创建任务状态：

| status | 含义 | 下一步 |
|--------|------|--------|
| `WAITING` | 任务已提交，仍在创建中 | 按 `interval` 轮询 `robot result` |
| `SUCCESS` | 创建完成 | 保存 `robotCode/clientId/clientSecret`，凭据按敏感信息处理 |
| `APPROVAL_REQUIRED` | 创建编排返回需审批 | 不要重复建号；按返回信息或开发者后台审批后再继续 |
| `FAIL` | 创建失败 | 读取 `errorCode/errorMsg/failReason`；可带原 `taskId` 重新 `submit` |
| `EXPIRED` | `taskId` 不存在或超过有效期 | 重新 `submit`，必要时换新 `taskId` |

## 二、现有应用的机器人配置

### 查询配置

```bash
dws dev app robot get --unified-app-id <unifiedAppId> --format json
```

返回机器人基础信息、回调地址、模式、状态和技能列表。应用尚未配置机器人时后端会返回 `robot info is not exist`。

状态判断：

- `status=1`：OFFLINE，机器人配置存在但处于停用/下线状态。
- `status=2`：ONLINE，机器人配置已生效；`robotCode` 可用于加群、机器人身份发消息或后续建联。
- `robot get` 返回 `success=true` 且包含 `robotCode` 时，说明配置已落库，不是异步等待态。
- ONLINE 只代表开放平台机器人能力已开启。若要让机器人自动处理消息，还需要配置 `--outgoing-url` / `--event-callback-url`，或用 `dev connect` 接到本地 Agent（见 connect.md）。
- 未配置机器人时不会返回 `status`，而是业务错误 `robot info is not exist`；这时走 `robot config`，不是 `enable`。

### 配置（upsert）/ 启用

- `config` 是 **upsert**：建或改都用它，配置不存在则建、存在则改，无需区分。需至少一个配置字段。
- `enable` 是**纯启用**：只开启机器人能力，不带配置字段（只传 `--unified-app-id`）。
- `config` 成功后必须回读 `robot get`：如果返回 `status=2`，不要再误判为"待生效"；只有 `status=1` 或需要重新上架时才调用 `enable`。

```bash
dws dev app robot config --unified-app-id <unifiedAppId> --name 小助手 --brief 审批助手 \
  --desc "处理审批相关问答" --outgoing-url https://example.com/msg \
  --event-callback-url https://example.com/event --mode 2 --skills qa,approval --dry-run --format json

dws dev app robot enable --unified-app-id <unifiedAppId> --dry-run --format json
```

> 旧的独立 `update` 命令已删除（并入 `config`）。

参数查 `dws schema dev.app.robot.config`；至少提供一个配置字段，否则 CLI 报错。国际化字段（`--i18n-name` 等）传 JSON，如 `'{"en_US":"Bot"}'`。

### 停用

```bash
dws dev app robot disable --unified-app-id <unifiedAppId> --dry-run --format json
dws dev app robot disable --unified-app-id <unifiedAppId> --yes --format json
```

## 错误处理

| 情况 | 处理 |
|------|------|
| `robot info is not exist` | 应用未配置机器人，先用 `robot config` 创建 |
| 应用名重复 | `app-name` 企业内需唯一，换个名字 |
| `ServiceResult.success=false` | 透传 `errorCode/errorMsg` |
| 创建任务 `EXPIRED` | 任务过期，重新 `submit`（可带原 taskId 重试） |

> 把机器人接到本地 agent 调试/值守（渠道、AI 卡片、会话记忆、依赖预检）见 [connect.md](connect.md)。
