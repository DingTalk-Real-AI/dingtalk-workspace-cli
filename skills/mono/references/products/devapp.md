# 开放平台应用 (devapp) 命令参考

管理钉钉开放平台企业内部应用。覆盖应用 CRUD、生命周期启停、凭证读取、权限管理、网页应用配置、成员管理和安全配置。

> `dws devapp ...` 是内置 helper 命令，不依赖 MCP 服务发现。`dws app ...` 是兼容别名。执行前用 `dws devapp --help` 验证可用。

## 核心规则

1. 所有命令加 `--format json`。
2. 写操作先 `--dry-run`，确认后才加 `--yes`。
3. 应用名/appKey/customKey 命中多条时展示候选，不取第一条。
4. 权限申请/取消只接受 `scopeValue`，不传 API 名或分组名。
5. `app get` 不读完整 secret，secret 走 `credentials get`。

## 应用定位

| 优先级 | 标识 | 处理 |
|--------|------|------|
| 1 | `--unified-app-id` | 直接使用 |
| 2 | `--agent-id` / `--app-id` | 直接使用 |
| 3 | `--app-key` / `--custom-key` | 先查询，唯一命中才继续 |
| 4 | `--name` | 模糊搜索，写操作必须唯一命中 |

---

## 一、应用基础操作

### 列表

```bash
dws devapp list --format json
dws devapp list --name DemoApp --format json
dws devapp list --agent-id 123456 --format json
```

MCP tool: `list_open_dev_apps_by_condition`

| CLI | MCP | 说明 |
|-----|-----|------|
| `--page` | `currentPage` | 1-based，默认 1 |
| `--page-size` | `pageSize` | 默认 20 |
| `--name` / `--keyword` | `appName` | 应用名搜索 |
| `--agent-id` | `agentId` | 精确定位 |
| `--app-key` | `appKey` | appKey/clientId |
| `--creator` | `creator` | 创建人关键词 |
| `--sort` | `sortType` | 如 `gmt_modified` |
| `--order` | `sortOrder` | `asc` / `desc` |

### 详情

```bash
dws devapp get --unified-app-id UNIFIED_APP_ID --format json
```

MCP tool: `get_open_dev_app_detail`

### 创建

```bash
dws devapp create --name DemoApp --desc "内部应用" --type internal --dry-run --format json
dws devapp create --name DemoApp --desc "内部应用" --type internal --yes --format json
```

MCP tool: `create_inner_app`。`--type` 只做 CLI 校验，当前仅支持 `internal`。

### 修改

```bash
dws devapp update --unified-app-id ID --name DemoApp2 --desc "新描述" --dry-run --format json
```

MCP tool: `update_inner_app`。至少一个更新字段：`--name` / `--desc` / `--icon`。

### 停用 / 启用

```bash
dws devapp inactive --unified-app-id ID --dry-run --format json
dws devapp active --unified-app-id ID --dry-run --format json
```

MCP tools: `inactive_inner_app` / `active_inner_app`

### 删除

```bash
dws devapp delete --unified-app-id ID --dry-run --format json
```

MCP tool: `delete_inner_app`。删除前必须展示应用摘要，异步生效。

---

## 二、凭证与网页应用

### 凭证读取

```bash
dws devapp credentials get --unified-app-id UNIFIED_APP_ID --format json
```

MCP tool: `get_open_dev_app_credentials`。返回含 `clientSecret/appSecret`，按敏感凭证处理。不能用 `devapp get` 代替。

### 网页应用

```bash
dws devapp webapp get --agent-id AGENT_ID --format json
dws devapp webapp config --agent-id AGENT_ID --homepage-link https://example.com --dry-run --format json
```

MCP tools: `get_webapp_config` / `set_webapp_config`

---

## 三、权限管理

### 权限列表

```bash
dws devapp permission list --unified-app-id ID --format json
dws devapp permission list --unified-app-id ID --keyword "机器人" --status UNAUTHED --format json
dws devapp permission list --unified-app-id ID --scope qyapi_robot_sendmsg --format json
```

MCP tool: `list_open_dev_app_permissions`

| CLI | MCP | 说明 |
|-----|-----|------|
| `--keyword` | `keyword` | 关键词搜索 |
| `--status` | `authStatus` | `ALL/AUTHED/UNAUTHED` |
| `--scope-type` | `firstLevelType` | `APP/SNS`，空返回两者 |
| `--scope` | `scopeValue` | 单权限详情模式 |
| `--limit` | `limit` | 每页数量，默认 20，建议不超过 50 |
| `--offset` | `offset` | 翻页偏移量，默认 0 |

翻页：`--limit 50 --offset 0`（第1页）、`--offset 50`（第2页）、`--offset 100`（第3页），返回条数 < limit 表示末尾。

### 申请权限

```bash
dws devapp permission add --unified-app-id ID --permissions qyapi_robot_sendmsg --dry-run --format json
```

MCP tool: `apply_open_dev_app_permissions`。`requiredApproval=true` 允许申请，写入版本变更。

### 取消权限

```bash
dws devapp permission remove --unified-app-id ID --permission qyapi_robot_sendmsg --dry-run --format json
```

MCP tool: `remove_open_dev_app_permission`。一次只取消一个。

---

## 四、成员与安全

### 成员管理

```bash
dws devapp member list --app-id UNIFIED_APP_ID --format json
dws devapp member add --app-id UNIFIED_APP_ID --users userId1,userId2 --member-type DEVELOPER --dry-run --format json
dws devapp member remove --app-id UNIFIED_APP_ID --users userId1 --member-type DEVELOPER --dry-run --format json
```

MCP tools: `list_open_dev_app_members` / `add_open_dev_app_members` / `remove_open_dev_app_members`

### 安全配置

```bash
dws devapp security config --app-id UNIFIED_APP_ID --ip-whitelist 192.0.2.10 --redirect-url https://callback.example.invalid/callback --dry-run --format json
```

MCP tool: `update_app_security_config`

---

## 五、机器人

### 新建智能体机器人

```bash
# 同步创建（返回 agentId/robotCode/clientId/clientSecret）
dws devapp robot create --app-name 我的智能体 --robot-name 小助手 --desc "处理审批问答" --dry-run --format json

# 异步创建 + 查询
dws devapp robot submit --app-name 我的智能体 --robot-name 小助手 --desc "处理审批问答" --dry-run --format json
dws devapp robot result --task-id TASK_ID --format json
```

MCP tools: `create_dingtalk_robot` / `submit_robot_create_task` / `query_robot_create_result`。`submit` 失败可带原 `--task-id` 重试。

### 现有应用配置机器人

```bash
dws devapp robot get --unified-app-id ID --format json
dws devapp robot config --unified-app-id ID --name 小助手 --brief 审批助手 --outgoing-url URL --mode 2 --skills qa,approval --dry-run --format json
dws devapp robot update --unified-app-id ID --brief "新简介" --dry-run --format json
dws devapp robot enable --unified-app-id ID --name 小助手 --dry-run --format json
dws devapp robot offline --unified-app-id ID --dry-run --format json
```

MCP tools: `get_open_dev_app_robot_config` / `create_open_dev_app_robot_config` / `update_open_dev_app_robot_config` / `enable_open_dev_app_robot` / `offline_open_dev_app_robot`。

配置字段：`--name/--brief/--description/--icon/--outgoing-url(outgoingUrl)/--event-url(chatBotEventUrl)/--mode/--skills(skillList)/--add-scope/--disable-ssl-verify/--i18n-name/--i18n-brief/--i18n-description`。应用未配机器人时 `get` 返回 `robot info is not exist`。

### 建联（把机器人接到本地 agent）

```bash
# 用现成机器人凭证起 Stream，接到当前渠道的本地 agent（前台运行，Ctrl-C 退出）
dws devapp robot connect --channel auto --robot-client-id CLIENT_ID --robot-client-secret CLIENT_SECRET

# 用统一应用 ID，复用 credentials get 自动取凭证（省得手填）
dws devapp robot connect --unified-app-id UNIFIED_APP_ID --channel qoderwork

# 预览建联方案不实际起连接
dws devapp robot connect --robot-client-id CLIENT_ID --robot-client-secret CLIENT_SECRET --dry-run --format json
```

- **不建号**：connect 只建联。缺凭证先用 `robot create/submit` 建号拿 clientId/clientSecret。
- **凭证用 `--robot-client-id/--robot-client-secret`**（不叫 `--client-id`，那是全局 OAuth 客户端覆盖 flag，会撞名）。
- **两种来源**：① 直接 `--robot-client-id/--robot-client-secret`；② `--unified-app-id`（内部调 `get_open_dev_app_credentials` 自动取）。⚠️ 来源②**字段名待预发真机验证**，且 clientSecret 一般仅建号时返回一次、该接口未必能返回，取不到时回退手填。
- **渠道 `--channel`**：`auto`(默认，按运行时信号自动识别当前宿主) | `openclaw` | `qoder` | `qoderwork` | `hermes` | `workbuddy` | `claudecode` | `codebuddy` | `codex` | `gemini` | `opencode`。
- **会话记忆 `--agent-memory`**（默认开）：同一群/单聊共享 agent 会话，追问保留上下文。仅 claudecode/codebuddy/workbuddy 支持（CLI 有 `--session-id/--resume`）；其余渠道自动无状态。`--agent-memory=false` 关闭。
- **模型覆盖 `--agent-model`**：claudecode 默认锁 haiku 求快，可改 sonnet/opus 换聪明（env `DWS_AGENT_MODEL`）。
- **运行目录 `--agent-workdir`**：放知识文件给机器人企业上下文；默认空白临时目录求快（env `DWS_AGENT_WORKDIR`）。
- **AI 卡片回复 `--reply-card`**（默认开）：思考中→完成状态卡片，同官方渠道体验；失败自动回退普通消息（env `DWS_REPLY_CARD=0` 关闭）。
- **卡片模板 `--card-template`**：模板按应用授权，建议在开发者后台为自己应用注册 AI 卡片模板并填其 ID（env `DWS_CARD_TEMPLATE`）；默认公共模板 best-effort。
- **stream-bridge 渠道**（qoder/qoderwork/claudecode/workbuddy/codex/gemini/opencode）：Go 原生进程内 Stream 转发器，订阅 `TOPIC_ROBOT`，每条 @机器人消息起一个无头 CLI 实例（如 `claude -p`）→ stdout 回钉钉，可 7×24 无人值守。
- **官方渠道**（openclaw/hermes）：dws 不代建机器人，输出官方 onboarding 指引（连接器自带建号 + AI 卡片回复）。
- 覆盖项：`DWS_AGENT_CMD`(指定 agent 可执行命令) / `DWS_CONNECT_CMD`(指定外部连接器) / `DWS_CONNECT_NO_INSTALL=1`(关闭缺包自动安装) / `DWS_AGENT_TIMEOUT_MS`。

---

## 六、版本发布

```bash
dws devapp version create --unified-app-id ID --version 1.0.1 --desc "新增机器人能力" --dry-run --format json
dws devapp version list --unified-app-id ID --page 1 --page-size 20 --format json
dws devapp version get --unified-app-id ID --version-id VERSION_ID --format json
dws devapp version check-approval --unified-app-id ID --version-id VERSION_ID --format json
dws devapp version publish --unified-app-id ID --version-id VERSION_ID --confirm-sensitive --dry-run --format json
dws devapp version status --unified-app-id ID --version-id VERSION_ID --format json
```

MCP tools: `create_open_dev_app_version` / `list_open_dev_app_versions` / `get_open_dev_app_version_detail` / `publish_open_dev_app_version` / `get_open_dev_app_version_status`。

- `check-approval` = `publish_open_dev_app_version` 的 `dryRun=true` 预检模式，不实际发布。
- `publish` 设 `dryRun=false`；含高敏权限需 `--confirm-sensitive`，灰度选人模式用 `--approver USER_ID`。
- 流程：`permission add`（requiredApproval 写入版本变更）→ `version create` → `check-approval` → `publish` → `status`。
- **审批人选择**：`check-approval` 返回候选审批人列表（userId+姓名）→ **展示给用户让其选择（不要自行挑选或默认第一个）** → `publish --approver <选中的userId>` 自动向该审批人发起审批 → `version status` 跟踪。机器人等能力需审批通过、应用发布后才可被搜索/加群/路由消息。

---

## 七、操作流程

### 创建应用全流程

```text
create --dry-run → 确认 → create --yes → get 确认 → credentials get → webapp config → permission add → member add
```

### 权限管理全流程

```text
permission list → permission list --keyword → permission list --scope → permission add --dry-run → 确认 → --yes → permission list 验证
```

### 生命周期

```text
停用: get → inactive --dry-run → --yes → get 验证
启用: active --dry-run → --yes → get 验证
删除: get 展示 → delete --dry-run → 确认 → --yes → list 验证消失
```

---

## 错误处理

| 情况 | 处理 |
|------|------|
| `unknown command` | CLI 构建不含 devapp helper |
| `endpoint_not_resolved` | 检查 edition endpoint 注入 |
| 多应用命中 | 展示候选，停止写操作 |
| `ServiceResult.success=false` | 透传 `errorCode/errorMsg` |

## 待实现能力

- `dws devapp event list/config` — 事件订阅（待后端发布）
