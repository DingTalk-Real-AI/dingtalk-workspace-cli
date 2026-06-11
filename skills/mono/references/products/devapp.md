# 开放平台应用 (devapp) 命令参考

管理钉钉开放平台企业内部应用。覆盖应用 CRUD、生命周期启停、凭证读取、权限管理、网页应用配置、成员管理和安全配置。

> `dws devapp ...` 是内置 helper 命令，不依赖 MCP 服务发现。`dws app ...` 是兼容别名。执行前用 `dws devapp --help` 验证可用。

## 核心规则

1. 所有命令加 `--format json`。
2. 写操作先 `--dry-run`，确认后才加 `--yes`。
3. 应用名/appKey/customKey 命中多条时展示候选，不取第一条。
4. 权限申请/取消只接受 `scopeValue`，不传 API 名或分组名。
5. 主动读取密钥走 `credentials get`；任何 `devapp get`/详情返回里的 `clientSecret/appSecret` 都按敏感凭证脱敏，不向用户展开。

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

应用状态字段：列表/详情原始字段里如果出现 `status` / `appStatus`，`0=IN_ACTIVE` 已停用、`1=ACTIVE` 已激活、`2=WAIT_ACTIVE` 待激活、`3=EXPIRED` 已过期。执行 `inactive/active` 后必须回读 `get` 或 `list`：看到 `status=0` 才算停用完成，看到 `status=1` 才算启用完成。`versionStatus` 是版本状态，不要和应用启停状态混用。

### 详情

```bash
dws devapp get --unified-app-id UNIFIED_APP_ID --format json
```

MCP tool: `get_open_dev_app_detail`

详情主要用于定位和核验应用。若上游偶尔随详情返回 `clientSecret/appSecret`，必须脱敏处理，不要复制到回答里；主动读取凭证仍走 `credentials get`。

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

MCP tool: `get_open_dev_app_credentials`。返回含 `clientSecret/appSecret`，按敏感凭证处理。不能用 `devapp get` 代替；如果 `devapp get` 偶尔返回密钥，也只用于内部判断并脱敏。

### 网页应用

```bash
dws devapp webapp get --agent-id AGENT_ID --format json
dws devapp webapp config --agent-id AGENT_ID --homepage-link https://example.com --dry-run --format json
```

MCP tools: `get_webapp_config` / `set_webapp_config`

`h5PageType` 未显式传入时，不要假设固定默认值；配置后以 `webapp get` 回读为准（实跑可能返回 `mobile`）。

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

权限状态判断：

- `--status` 是查询过滤条件：`ALL` 不过滤，`AUTHED` 只看已授权/已开通，`UNAUTHED` 只看未授权/未开通。
- 单个权限项的 `status` 是内部操作态：`0` 已获得，`1` 申请中，`2` 可以申请，`3` 不可以申请。
- `status=0` 不要重复申请；如需取消，确认 `canRemove=true` 后走 `permission remove`。
- `status=1` 不要重复申请；查看 `authedStatusDesc`，通常等待审批或版本发布。
- `status=2` 可走 `permission add --dry-run`。
- `status=3` 停止申请，展示 `applyDisabledReason/displayMessage`。
- `authedStatusDesc` 细分展示：`OPENED`/`APPLIED`/`TO_BE_PUBLISHED` 表示已开通、已申请或待发布；`NOT_OPEN`/`NOT_APPLIED` 表示未开通/未申请；`AUDIT_PROCESSING` 表示审批中；`AUDIT_REFUSE` 表示审批未通过。能否操作仍以 `status`、`canEdit`、`canApplyDirectly`、`allowedActions` 为准。

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

异步创建任务状态：

| status | 含义 | 下一步 |
|--------|------|--------|
| `WAITING` | 任务已提交，仍在创建中 | 按 `interval` 轮询 `robot result` |
| `SUCCESS` | 创建完成 | 保存 `robotCode/clientId/clientSecret`，凭据按敏感信息处理 |
| `APPROVAL_REQUIRED` | 创建编排返回需审批 | 不要重复建号；按返回信息或开发者后台审批后再继续 |
| `FAIL` | 创建失败 | 读取 `errorCode/errorMsg/failReason`；可带原 `taskId` 重新 `submit` |
| `EXPIRED` | `taskId` 不存在或超过有效期 | 重新 `submit`，必要时换新 `taskId` |

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

状态判断：

- `robot get` 返回 `success=true` 且包含 `robotCode` 时，说明机器人配置已落库，不是异步等待态。
- `status=1`：OFFLINE，机器人配置存在但处于停用/下线状态。
- `status=2`：ONLINE，机器人配置已生效；`robotCode` 可用于加群、机器人身份发消息或后续建联。
- ONLINE 只代表开放平台机器人能力已开启。若要让机器人自动处理消息，还需要配置 `--outgoing-url` / `--event-url`，或用 `robot connect` 接到本地 Agent。
- 首次 `config` 成功后必须回读 `robot get`：如果返回 `status=2`，不要再误判为“待生效”；只有 `status=1` 或需要重新上架时才调用 `enable`。
- 未配置机器人时不会返回 `status`，而是业务错误 `robot info is not exist`；这时走 `robot config`，不是 `enable`。

### 建联（把机器人接到本地 agent）

```bash
# 用现成机器人凭证起 Stream，接到当前渠道的本地 agent（前台运行，Ctrl-C 退出）
dws devapp robot connect --channel auto --robot-client-id CLIENT_ID --robot-client-secret CLIENT_SECRET

# 用统一应用 ID，复用 credentials get 自动取凭证（省得手填）
dws devapp robot connect --unified-app-id UNIFIED_APP_ID --channel qoderwork

# Codex 渠道推荐显式覆盖 agent 命令；否则默认临时目录可能触发
# "Not inside a trusted directory and --skip-git-repo-check was not specified"
CODEX_BIN="$(command -v codex || echo /Applications/Codex.app/Contents/Resources/codex)"
DWS_AGENT_CMD="$CODEX_BIN exec --skip-git-repo-check" \
  dws devapp robot connect --unified-app-id UNIFIED_APP_ID --channel codex --format json

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
- **富回复 `--reply-card`**（默认开）：Thinking/Done 表态永远生效；**卡片需配 `--card-template` 才启用**（同 hermes 语义：没配模板=纯文字回复+表态），失败自动回退文字（env `DWS_REPLY_CARD=0` 全关）。
- **卡片模板 `--card-template`**：模板按应用授权，建议在开发者后台为自己应用注册 AI 卡片模板并填其 ID（env `DWS_CARD_TEMPLATE`）；默认公共模板 best-effort。
- **依赖预检（agent 必做）**：建联前先 `--dry-run` 看输出的 `cli` 字段——`installed:false` 且 `autoInstall:true`（npm 系）告知用户会自动安装；`autoInstall:false`（桌面 App/官方渠道）**先引导用户按 `installHint` 安装**，不要直接起连接。
- **Codex 渠道**：真实建联优先用 `DWS_AGENT_CMD="$CODEX_BIN exec --skip-git-repo-check"`，避免 Codex CLI 在默认临时目录内拒绝执行。需要固定上下文时，把可信目录写进覆盖命令：`DWS_AGENT_CMD="$CODEX_BIN exec --skip-git-repo-check -C /path/to/repo"`（路径不要包含空格）。设置 `DWS_AGENT_CMD` 后，DWS 不再自动拼接 `--agent-model/--agent-workdir/会话参数`。
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
- 新应用如果 `version list` 返回空，先执行 `version create`，用返回的 `versionId` 继续 `check-approval/publish`；不要因为列表为空误判无可发布内容。
- **审批人选择**：`check-approval` 返回候选审批人列表（userId+姓名）→ **展示给用户让其选择（不要自行挑选或默认第一个）** → `publish --approver <选中的userId>` 自动向该审批人发起审批 → `version status` 跟踪。机器人等能力需审批通过、应用发布后才可被搜索/加群/路由消息。

发布响应 `result`：

| result | 含义 | 下一步 |
|--------|------|--------|
| `NOT_REQUIRED` | 不需要审批；预检时表示可直接发布，真实发布时表示已直接发布上线 | 真实发布后回读 `version status/get` 验证 `RELEASE` |
| `APPROVAL_REQUIRED` | 需要审批，通常出现在 `check-approval` 或未指定审批人时 | 查看 `approvalMode/approvalCandidates`，让用户选择审批人后再 `publish --approver` |
| `SUBMITTED` | 已提交审批 | 保存 `processId`，轮询 `version status` |

审批模式 `approvalMode`：`SELECT_APPROVER` 表示灰度选人模式，需要展示候选审批人并让用户选择；`ENTERPRISE_SELF_BUILT` 表示企业自建审核模式，不传 `--approver`，按企业自建审核流程等待。

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
