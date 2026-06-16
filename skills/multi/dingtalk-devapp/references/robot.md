# 机器人能力

为开放平台企业内部应用创建和配置机器人。分两类场景：

1. **新建智能体机器人**：一次性创建一个新的 Agent 应用 + 承载机器人（`create` / `submit` / `result`）。
2. **现有应用配置机器人**：在已存在的应用上开启/配置/停用机器人（`get` / `config` / `enable` / `disable`），通过 `--unified-app-id` 定位。

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
dws devapp robot get --unified-app-id <unifiedAppId> --format json
```

MCP tool: `get_robot_config`。返回机器人基础信息、回调地址、模式、状态和技能列表。应用尚未配置机器人时后端会返回 `robot info is not exist`。

状态判断：

- `status=1`：OFFLINE，机器人配置存在但处于停用/下线状态。
- `status=2`：ONLINE，机器人配置已生效；`robotCode` 可用于加群、机器人身份发消息或后续建联。
- `robot get` 返回 `success=true` 且包含 `robotCode` 时，说明配置已落库，不是异步等待态。
- ONLINE 只代表开放平台机器人能力已开启。若要让机器人自动处理消息，还需要配置 `--outgoing-url` / `--event-url`，或用 `robot connect` 接到本地 Agent。
- 未配置机器人时不会返回 `status`，而是业务错误 `robot info is not exist`；这时走 `robot config`，不是 `enable`。

### 创建或更新 / 启用配置

`config` 是 upsert：未配置机器人时创建，已存在时更新；`enable` 用于启用/重新启用。

首次 `config` 成功后必须回读 `robot get`：如果返回 `status=2`，不要再误判为“待生效”；只有 `status=1` 或需要重新上架时才调用 `enable`。

```bash
dws devapp robot config --unified-app-id <unifiedAppId> --name 小助手 --brief 审批助手 \
  --description "处理审批相关问答" --outgoing-url https://example.com/msg \
  --event-url https://example.com/event --mode 2 --skills qa,approval --dry-run --format json

dws devapp robot enable --unified-app-id <unifiedAppId> --name 小助手 --dry-run --format json
```

MCP tools: `set_extension_robot_config` / `enable_dev_app_robot`

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
dws devapp robot disable --unified-app-id <unifiedAppId> --dry-run --format json
dws devapp robot disable --unified-app-id <unifiedAppId> --yes --format json
```

MCP tool: `disable_dev_app_robot`

### 建联（把机器人接到本地 agent）

```bash
# 用现成机器人凭证起 Stream，接到当前渠道的本地 agent（前台运行，Ctrl-C 退出）
dws devapp robot connect --channel auto --robot-client-id <clientId> --robot-client-secret <clientSecret>

# 用统一应用 ID，复用 credentials get 自动取凭证
dws devapp robot connect --unified-app-id <unifiedAppId> --channel qoderwork

# Codex 渠道推荐显式覆盖 agent 命令；否则默认空白工作目录可能触发
# "Not inside a trusted directory and --skip-git-repo-check was not specified"
CODEX_BIN="$(command -v codex || echo /Applications/Codex.app/Contents/Resources/codex)"
DWS_AGENT_CMD="$CODEX_BIN exec --skip-git-repo-check" \
  dws devapp robot connect --unified-app-id <unifiedAppId> --channel codex --format json

# 预览建联方案不实际起连接
dws devapp robot connect --robot-client-id <clientId> --robot-client-secret <clientSecret> --dry-run --format json
```

只建联、不建号：缺凭证先用 `robot create` / `robot submit` 建号拿 clientId/clientSecret。

Codex 渠道注意：

- `--channel codex` 真实建联时，优先使用上面的 `DWS_AGENT_CMD="$CODEX_BIN exec --skip-git-repo-check"` 形式，避免 Codex CLI 在默认临时目录内因非可信 Git 目录拒绝执行。
- 如需给 Codex 固定知识/项目上下文，可把可信目录直接写进覆盖命令：`DWS_AGENT_CMD="$CODEX_BIN exec --skip-git-repo-check -C /path/to/repo"`。路径不要包含空格。
- 设置 `DWS_AGENT_CMD` 后，DWS 不再自动拼接 `--agent-model` / `--agent-workdir` / 会话参数；需要的参数要一起写进 `DWS_AGENT_CMD`。

| flag | 说明 |
|------|------|
| `--channel` | `auto`(默认,运行时信号自动识别) / openclaw / qoder / qoderwork / hermes / workbuddy / claudecode / codebuddy / codex / gemini / opencode |
| `--robot-client-id` / `--robot-client-secret` | 现成机器人凭证（clientId=AppKey, clientSecret=AppSecret）。命名带 `robot-` 前缀以避开全局 OAuth `--client-id` flag |
| `--unified-app-id` | 统一应用 ID，内部调 `get_dev_app_credentials` 自动取凭证。⚠️ 字段名待预发真机验证；clientSecret 仅建号时返回一次、未必可取，必要时回退手填 |
| `--agent-memory` | 按会话续聊（默认开）：同一群/单聊共享 agent 会话，追问保留上下文。仅 claudecode/codebuddy/workbuddy（CLI 有 `--session-id`/`--resume`）；qoder 系/codex/gemini/opencode 无寻址会话，自动保持无状态。`--agent-memory=false` 关闭 |
| `--agent-model` | 覆盖本地 agent 模型（如 claudecode 默认锁 haiku 求快，可改 `claude-sonnet-4-6` 换聪明）。env: `DWS_AGENT_MODEL` |
| `--agent-workdir` | agent 运行目录：放知识文件（如 CLAUDE.md）可给机器人企业上下文。默认空白临时目录（冷启动快 ~4s vs 大目录 ~29s，慢了会错过钉钉响应窗口）。env: `DWS_AGENT_WORKDIR` |
| `--reply-card` | 富回复（默认开）：🤔Thinking/🥳Done 表态永远生效；**卡片需配 `--card-template` 才启用**（同 hermes：没配模板=纯文字回复），失败自动回退文字；env `DWS_REPLY_CARD=0` 全关 |
| `--card-template` | AI 卡片模板 ID。**模板按应用授权**：去开发者后台→你的应用→AI 卡片设置注册/获取模板 ID（同 hermes 的配置方式），可去掉公共模板的第三方角标；默认用公共模板 best-effort。env `DWS_CARD_TEMPLATE` |

#### 建联前的依赖预检（agent 必做）

渠道背后是本地 agent CLI，用户可能没装。先 `--dry-run` 看 `cli` 字段再决定下一步：

```bash
dws devapp robot connect --channel <ch> --robot-client-id x --robot-client-secret y --dry-run --format json
# 输出里的 cli 字段：
#   "cli": {"required":"Claude Code","installed":false,"autoInstall":true,"installHint":"npm i -g @anthropic-ai/claude-code"}
```

| cli 状态 | agent 应该做什么 |
|----------|----------------|
| `installed: true` | 直接建联 |
| `installed: false, autoInstall: true` | 告知用户缺哪个 CLI，说明启动建联时会自动 `npm` 安装（或先手动执行 installHint 里的命令再连）；`DWS_CONNECT_NO_INSTALL=1` 可禁自动安装 |
| `installed: false, autoInstall: false` | **不要直接起连接**——桌面 App 渠道（qoder/qoderwork/workbuddy）需要用户先安装对应 App（installHint 是下载地址），装好后 CLI 随 App 自带；openclaw/hermes 引导用户走官方 onboarding |

- **stream-bridge 渠道**：Go 原生进程内 Stream 转发器，订阅 `TOPIC_ROBOT`，每条 @机器人消息起一个无头 CLI 实例 → stdout 回钉钉，可 7×24 无人值守。
- **会话记忆**：首条消息 `--session-id <uuid>` 建会话，后续 `--resume <uuid>` 续聊；会话状态在内存里，连接器重启后从新会话开始；某会话坏了会自愈（下条消息换新会话）。
- **官方渠道**（openclaw/hermes）：dws 不代建机器人，输出官方 onboarding 指引。
- 环境覆盖：`DWS_AGENT_CMD`(整条命令覆盖,覆盖时不再注入模型/会话参数) / `DWS_AGENT_MODEL` / `DWS_AGENT_WORKDIR` / `DWS_CONNECT_CMD` / `DWS_CONNECT_NO_INSTALL=1` / `DWS_AGENT_TIMEOUT_MS`。

## 错误处理

| 情况 | 处理 |
|------|------|
| `robot info is not exist` | 应用未配置机器人，先用 `robot config` 创建 |
| 应用名重复 | `app-name` 企业内需唯一，换个名字 |
| 本地 Codex agent 返回 `Not inside a trusted directory and --skip-git-repo-check was not specified` | 用 `DWS_AGENT_CMD="$CODEX_BIN exec --skip-git-repo-check"` 重启 `robot connect --channel codex` |
| `ServiceResult.success=false` | 透传 `errorCode/errorMsg` |
| 创建任务 `EXPIRED` | 任务过期，重新 `submit`（可带原 taskId 重试） |
