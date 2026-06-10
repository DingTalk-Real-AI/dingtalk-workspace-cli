# 机器人能力

为开放平台企业内部应用创建和配置机器人。分两类场景：

1. **新建智能体机器人**：一次性创建一个新的 Agent 应用 + 承载机器人（`create` / `submit` / `result`）。
2. **现有应用配置机器人**：在已存在的应用上开启/配置/停用机器人（`get` / `config` / `update` / `enable` / `offline`），通过 `--unified-app-id` 定位。

> `corpId` / `userId` 由 MCP 系统上下文注入，CLI 不传。所有写操作先 `--dry-run`，确认后再 `--yes`。

## 一、新建智能体机器人

### 同步创建

```bash
dws devapp robot create --app-name 我的智能体 --robot-name 小助手 --desc "处理审批问答" --dry-run --format json
dws devapp robot create --app-name 我的智能体 --robot-name 小助手 --desc "处理审批问答" --yes --format json
```

MCP tool: `create_dingtalk_robot`。成功返回 `agentId / robotCode / clientId / clientSecret`（凭据按敏感信息处理）。

| CLI | MCP | 必填 | 说明 |
|-----|-----|------|------|
| `--app-name` | `appName` | 是 | 智能体应用名称，长度 2-20，企业内唯一 |
| `--robot-name` | `robotName` | 是 | 承载机器人名称，客户端展示 |
| `--desc` | `desc` | 是 | 机器人功能描述，≤200 字 |
| `--icon` | `robotMediaId` | 否 | 图标 mediaId；空则用默认图标 |
| `--preview` | `previewMediaId` | 否 | 预览图 mediaId；空则复用图标 |

### 异步创建 + 查询结果

适合创建耗时较长或需要失败重试的场景。

```bash
# 提交任务
dws devapp robot submit --app-name 我的智能体 --robot-name 小助手 --desc "处理审批问答" --dry-run --format json
dws devapp robot submit --app-name 我的智能体 --robot-name 小助手 --desc "处理审批问答" --yes --format json
# → 返回 taskId

# 轮询结果
dws devapp robot result --task-id <taskId> --format json
```

MCP tools: `submit_robot_create_task` / `query_robot_create_result`

- `submit` 返回 `taskId / status / expiresIn / interval / retryCount`。
- 失败重试：把上次的 `taskId` 通过 `--task-id` 传入 `submit`，避免重复创建。
- `result` 返回 `WAITING / SUCCESS / FAIL / EXPIRED`；`SUCCESS` 时返回 `agentId / robotCode / clientId / clientSecret`。

## 二、现有应用的机器人配置

### 查询配置

```bash
dws devapp robot get --unified-app-id <unifiedAppId> --format json
```

MCP tool: `get_open_dev_app_robot_config`。返回机器人基础信息、回调地址、模式、状态和技能列表。应用尚未配置机器人时后端会返回 `robot info is not exist`。

### 创建 / 更新 / 启用配置

三者字段一致，区别在语义：`config`=首次创建、`update`=修改、`enable`=启用/重新启用。

```bash
dws devapp robot config --unified-app-id <unifiedAppId> --name 小助手 --brief 审批助手 \
  --description "处理审批相关问答" --outgoing-url https://example.com/msg \
  --event-url https://example.com/event --mode 2 --skills qa,approval --dry-run --format json

dws devapp robot update --unified-app-id <unifiedAppId> --brief "新的简介" --dry-run --format json

dws devapp robot enable --unified-app-id <unifiedAppId> --name 小助手 --dry-run --format json
```

MCP tools: `create_open_dev_app_robot_config` / `update_open_dev_app_robot_config` / `enable_open_dev_app_robot`

| CLI | MCP | 说明 |
|-----|-----|------|
| `--name` | `name` | 机器人名称 |
| `--brief` | `brief` | 简介 |
| `--description` | `description` | 描述 |
| `--icon` | `iconMediaId` | 图标 mediaId |
| `--outgoing-url` | `outgoingUrl` | 消息回调地址 |
| `--event-url` | `chatBotEventUrl` | 事件回调地址 |
| `--mode` | `mode` | 机器人模式枚举（整数） |
| `--skills` | `skillList` | 技能列表，逗号分隔 |
| `--add-scope` | `isAddScope` | 自动添加机器人相关权限 |
| `--disable-ssl-verify` | `disableSSLVerify` | 回调关闭 SSL 校验 |
| `--i18n-name` | `i18nName` | 名称国际化 JSON，如 `'{"en_US":"Bot"}'` |
| `--i18n-brief` | `i18nBrief` | 简介国际化 JSON |
| `--i18n-description` | `i18nDescription` | 描述国际化 JSON |

至少提供一个配置字段，否则 CLI 报错。

### 停用

```bash
dws devapp robot offline --unified-app-id <unifiedAppId> --dry-run --format json
dws devapp robot offline --unified-app-id <unifiedAppId> --yes --format json
```

MCP tool: `offline_open_dev_app_robot`

### 建联（把机器人接到本地 agent）

```bash
# 用现成机器人凭证起 Stream，接到当前渠道的本地 agent（前台运行，Ctrl-C 退出）
dws devapp robot connect --channel auto --robot-client-id <clientId> --robot-client-secret <clientSecret>

# 用统一应用 ID，复用 credentials get 自动取凭证
dws devapp robot connect --unified-app-id <unifiedAppId> --channel qoderwork

# 预览建联方案不实际起连接
dws devapp robot connect --robot-client-id <clientId> --robot-client-secret <clientSecret> --dry-run --format json
```

只建联、不建号：缺凭证先用 `robot create` / `robot submit` 建号拿 clientId/clientSecret。

| flag | 说明 |
|------|------|
| `--channel` | `auto`(默认,运行时信号自动识别) / openclaw / qoder / qoderwork / hermes / workbuddy / claudecode / codebuddy / codex / gemini / opencode |
| `--robot-client-id` / `--robot-client-secret` | 现成机器人凭证（clientId=AppKey, clientSecret=AppSecret）。命名带 `robot-` 前缀以避开全局 OAuth `--client-id` flag |
| `--unified-app-id` | 统一应用 ID，内部调 `get_open_dev_app_credentials` 自动取凭证。⚠️ 字段名待预发真机验证；clientSecret 仅建号时返回一次、未必可取，必要时回退手填 |
| `--agent-memory` | 按会话续聊（默认开）：同一群/单聊共享 agent 会话，追问保留上下文。仅 claudecode/codebuddy/workbuddy（CLI 有 `--session-id`/`--resume`）；qoder 系/codex/gemini/opencode 无寻址会话，自动保持无状态。`--agent-memory=false` 关闭 |
| `--agent-model` | 覆盖本地 agent 模型（如 claudecode 默认锁 haiku 求快，可改 `claude-sonnet-4-6` 换聪明）。env: `DWS_AGENT_MODEL` |
| `--agent-workdir` | agent 运行目录：放知识文件（如 CLAUDE.md）可给机器人企业上下文。默认空白临时目录（冷启动快 ~4s vs 大目录 ~29s，慢了会错过钉钉响应窗口）。env: `DWS_AGENT_WORKDIR` |
| `--reply-card` | AI 卡片回复（默认开）：思考中→完成状态，同 hermes/openclaw 官方渠道体验；卡片任一步失败自动回退普通 text/markdown；env `DWS_REPLY_CARD=0` 关闭 |

- **stream-bridge 渠道**：Go 原生进程内 Stream 转发器，订阅 `TOPIC_ROBOT`，每条 @机器人消息起一个无头 CLI 实例 → stdout 回钉钉，可 7×24 无人值守。
- **会话记忆**：首条消息 `--session-id <uuid>` 建会话，后续 `--resume <uuid>` 续聊；会话状态在内存里，连接器重启后从新会话开始；某会话坏了会自愈（下条消息换新会话）。
- **官方渠道**（openclaw/hermes）：dws 不代建机器人，输出官方 onboarding 指引。
- 环境覆盖：`DWS_AGENT_CMD`(整条命令覆盖,覆盖时不再注入模型/会话参数) / `DWS_AGENT_MODEL` / `DWS_AGENT_WORKDIR` / `DWS_CONNECT_CMD` / `DWS_CONNECT_NO_INSTALL=1` / `DWS_AGENT_TIMEOUT_MS`。

## 错误处理

| 情况 | 处理 |
|------|------|
| `robot info is not exist` | 应用未配置机器人，先用 `robot config` 创建 |
| 应用名重复 | `app-name` 企业内需唯一，换个名字 |
| `ServiceResult.success=false` | 透传 `errorCode/errorMsg` |
| 创建任务 `EXPIRED` | 任务过期，重新 `submit`（可带原 taskId 重试） |
