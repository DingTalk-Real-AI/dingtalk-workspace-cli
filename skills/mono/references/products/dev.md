# dev — 开放平台开发者命令

`dws dev` 是面向**开发者**的命令组，分三个子树：

| 子命令 | 职责 |
|--------|------|
| `dev app` | 应用生命周期（创建/查询/更新/删除/凭证/权限/成员/安全/网页/机器人/**建号**/版本/事件订阅） |
| `dev connect` | **建联**：把现成机器人接到当前本地 agent（起 Stream，不建号） |
| `dev doc` | 开放平台开发文档搜索入口（当前网关未注册该工具键，`dev doc search` 会报「未找到指定工具」不可用；文档搜索一律走 `dws devdoc article search --query <关键词>`） |

> ⚠️ **关键区分**：`dws chat bot search/find` 只查询已有机器人（IM 视角）；**创建/建号**机器人走 `dws dev app robot submit`；**建联**走 `dws dev connect`。"创建机器人"/"建联"一律走 `dev`，禁止走 `chat`。

---

## 典型工作流：创建应用 → 配置机器人 → 版本发布/审批 → 建联

```bash
# Step 1：创建开放平台应用，拿 unifiedAppId
dws dev app create --name "我的 AI 机器人" --desc "接 opencode" --dry-run --format json
dws dev app create --name "我的 AI 机器人" --desc "接 opencode" --yes --format json
# → 返回 unifiedAppId

# Step 2：在明确的 unifiedAppId 上配置机器人能力
dws dev app robot config --unified-app-id <unifiedAppId> --name "小助手" --desc "接 opencode" --dry-run --format json
dws dev app robot config --unified-app-id <unifiedAppId> --name "小助手" --desc "接 opencode" --yes --format json

# Step 3：版本发布/审批，线上搜索、加群、路由消息以 RELEASE 为准
dws dev app version create --unified-app-id <unifiedAppId> --desc "发布机器人能力" --dry-run --format json
dws dev app version create --unified-app-id <unifiedAppId> --desc "发布机器人能力" --yes --format json
dws dev app version check-approval --unified-app-id <unifiedAppId> --version-id <versionId> --format json
dws dev app version publish --unified-app-id <unifiedAppId> --version-id <versionId> --dry-run --format json
dws dev app version publish --unified-app-id <unifiedAppId> --version-id <versionId> --yes --format json
dws dev app version status --unified-app-id <unifiedAppId> --version-id <versionId> --format json

# Step 4：本地建联 — 把机器人接到本地 agent，前台常驻（Ctrl-C 退出）
dws dev connect --unified-app-id <unifiedAppId> --channel opencode
# 若只有 robot result 返回的一次性凭证，clientSecret 只能用占位并按敏感信息处理：
dws dev connect --robot-client-id <clientId> --robot-client-secret <clientSecret-from-result>
```

`check-approval` 若返回 `approvalMode=SELECT_APPROVER`，让用户从候选里选择；不要默认取第一个审批人。用户选定后再给 `publish` 追加 `--approver-user-id <userId>`。

选择题优先原样展示 `approvalPromptText`（带 `A.`/`B.` 序号 + `姓名（userId: xxx）` 的成品文案）；需结构化时用 `approvalOptions[].label`，不得退化成多个泛化的“候选审批人”。`completionState=WAITING_FOR_APPROVER_SELECTION` / `mustAskUser=true` 表示必须等待用户选择。

完成态规则：

- `robot result` 顶层 `completionState=BLOCKED_BY_VERSION_PUBLISH` / `mustContinue=true` / `terminal=false` 是硬门禁，必须继续执行 blocking `nextSteps`。
- `robot result` 顶层 `completionState=BLOCKED_BY_MISSING_UNIFIED_APP_ID` / `actionRequired=provide_unified_app_id` 时，必须停下要求明确的 `unifiedAppId`；不能用 `clientId/appKey` 自动反查后写版本。
- `dev connect` 成功只代表本地 Stream 调试可用，不能代表机器人线上可用。
- `dev connect` dry-run 或启动输出里的 `completionState=LOCAL_DEBUG_ONLY` / `doesNotPublish=true` 表示只完成本地调试，不得作为最终完成。
- `robot result` 返回 `lifecycle.overallComplete=false`，或版本未进入 `RELEASE` / `AUDIT` / `UNDER_REVIEW` 前，不要总结“全部完成”“机器人已创建并成功连接”“可以在钉钉中 @机器人使用”。
- “创建机器人并连接 qoder”类任务的闭环必须包含：建号完成、本地 qoder Stream 建联成功、版本已发布或已提交审批；若 `SELECT_APPROVER` 需要选审批人，则停在候选审批人选择。

`robot result` 异步状态：

| status | 含义 | 下一步 |
|--------|------|--------|
| `WAITING` | 创建中 | 按 `intervalSeconds` 继续轮询 |
| `SUCCESS` | 创建完成 | 保存 `robotCode/clientId/clientSecret`；若结果含明确 `unifiedAppId` 才能继续版本发布，否则停下要求用户提供 |
| `APPROVAL_REQUIRED` | 已建号但线上使用需审核 | 不要重复建号；若结果含明确 `unifiedAppId` 才能提交版本发布审核，否则停下要求用户提供 |
| `FAIL` | 失败 | 读 `errorCode/errorMsg`，可带原 `taskId` 重新 `submit` |
| `EXPIRED` | 任务过期 | 重新 `submit` |

---

## dev app — 应用生命周期

```bash
# 查询应用列表
dws dev app list --format json

# 查询单个应用详情
dws dev app get --unified-app-id <unifiedAppId> --format json
dws dev app get --app-key <appKey> --format json

# 创建应用
dws dev app create --name <名称> --desc <描述> --format json

# 更新应用信息（写操作：先 --dry-run 预览，确认后加 --yes 执行；不加 --yes 会被拦下）
dws dev app update --unified-app-id <unifiedAppId> --name <新名称> --dry-run --format json
dws dev app update --unified-app-id <unifiedAppId> --name <新名称> --yes --format json

# 停用/启用应用
dws dev app disable --unified-app-id <unifiedAppId> --yes --format json
dws dev app enable  --unified-app-id <unifiedAppId> --yes --format json

# 删除应用（不可逆，需 --confirm-name 二次确认）
dws dev app delete --unified-app-id <unifiedAppId> --confirm-name <应用名> --yes --format json
```

---

## dev app robot — 机器人（建号与配置）

```bash
# 建号：异步提交
dws dev app robot submit --name <智能体名> --robot-name <机器人名> --desc <描述> --dry-run --format json
dws dev app robot submit --name <智能体名> --robot-name <机器人名> --desc <描述> --yes --format json

# 建号：轮询结果
dws dev app robot result --task-id <taskId> --format json
# SUCCESS/APPROVAL_REQUIRED 后只有结果含明确 unifiedAppId 才能走版本发布
# 若顶层有 completionState=BLOCKED_BY_MISSING_UNIFIED_APP_ID，要求用户提供 unifiedAppId；不能用 appKey/clientId 自动反查后写版本
# 若顶层有 completionState=BLOCKED_BY_VERSION_PUBLISH 或 mustContinue=true，不得停在 dev connect

# 查询现有机器人配置
dws dev app robot get --unified-app-id <unifiedAppId> --format json
# robotStatus=UNCONFIGURED → 未配置，走 config；OFFLINE → 走 enable；ONLINE → 已就绪
# 创建/更新机器人配置（upsert）
dws dev app robot config --unified-app-id <unifiedAppId> --name <机器人名> --format json

# 启用/停用机器人
dws dev app robot enable  --unified-app-id <unifiedAppId> --format json
dws dev app robot disable --unified-app-id <unifiedAppId> --format json
```

---

## dev app credentials — 凭证

```bash
# 查询 clientId/clientSecret（credentials 下只有 get 子命令）
dws dev app credentials get --unified-app-id <unifiedAppId> --format json
```

---

## dev connect — 建联

```bash
# 前台建联，自动探测渠道
dws dev connect \
  --robot-client-id <clientId> --robot-client-secret <clientSecret>

# 明确指定渠道（opencode/claudecode/qoder/qoderwork/workbuddy/codex/gemini/hermes/openclaw/custom）
dws dev connect --channel opencode \
  --robot-client-id <clientId> --robot-client-secret <clientSecret>

# 建联前预览方案（不实际起连接，检查 cli.installed 字段）
dws dev connect --channel opencode \
  --robot-client-id <clientId> --robot-client-secret <clientSecret> \
  --dry-run --format json

# 后台守护进程模式（崩溃自拉起）
dws dev connect --daemon \
  --robot-client-id <clientId> --robot-client-secret <clientSecret>

# 查看/停止后台连接器（status/list 忽略全局 --format，出 JSON 用专属 --json）
dws dev connect status --json
dws dev connect stop
```

常用 flag：

| Flag | 说明 |
|------|------|
| `--channel` | 渠道，默认 `auto` 自动探测 |
| `--agent-model` | 覆盖 agent 模型（如 `claude-sonnet-4-6`） |
| `--agent-workdir` | agent 运行目录（放知识文件可给机器人项目上下文） |
| `--reply-card` | AI 卡片回复，默认开启（`--reply-card=false` 关闭） |
| `--card-template` | AI 卡片模板 ID（开发者后台→本应用→AI 卡片设置获取） |
| `--knowledge-dir` | 本地知识目录（.md/.txt），每条消息检索后拼入 prompt |
| `--daemon` | 后台守护进程 |
| `--owner-user-id` | 数字分身：执行类请求先发给主人审批 |

**预检 cli.installed**：`--dry-run` 出参的 `cli` 字段含 `installed/autoInstall/installHint`。`installed:false, autoInstall:false`（桌面 App 渠道）时先引导用户安装对应 App，不要直接起连接。

`dev connect` 是本地调试步骤。dry-run JSON 的 `invocation` 会包含 `scope=local_debug_only`、`doesNotPublish=true`、`completionState=LOCAL_DEBUG_ONLY`、`terminal=false`；真实前台/daemon 启动也会提示“本地调试，不代表线上发布完成”。这些信号不能抵消版本发布/审批闭环。

---

## dev app event — 事件订阅

事件码定位优先用 `event list --keyword` 搜索；只有用户明确要「全部事件」时才翻全量（逐页）。

```bash
dws dev app event list        --unified-app-id <unifiedAppId> --keyword <关键词> --format json
dws dev app event subscribe   --unified-app-id <unifiedAppId> --event-codes <code1>,<code2> --dry-run --format json
dws dev app event unsubscribe --unified-app-id <unifiedAppId> --event-codes <code1>,<code2> --yes --format json
```

---

## dev app permission — 权限

```bash
dws dev app permission list  --unified-app-id <unifiedAppId> --format json
dws dev app permission add   --unified-app-id <unifiedAppId> --scope-values <scopeValue> --format json
dws dev app permission remove --unified-app-id <unifiedAppId> --scope-values <scopeValue> --yes --format json
```

---

## dev app version — 版本发布

配置变更（权限/机器人/网页等）需通过版本通道才生效。

```bash
dws dev app version create         --unified-app-id <unifiedAppId> --format json
dws dev app version check-approval --unified-app-id <unifiedAppId> --version-id <versionId> --format json
dws dev app version publish        --unified-app-id <unifiedAppId> --version-id <versionId> --format json
dws dev app version status         --unified-app-id <unifiedAppId> --version-id <versionId> --format json
```

---

## 注意事项

- **`clientSecret` 只在 `robot result` 返回一次**，务必立即保存；dws 只能 `credentials get` 读取，不支持重置，遗失后到开放平台控制台重置密钥
- 改配置后机器人不自动生效，需走 `version create → publish` 才上线
- `hermes`/`openclaw` 渠道走官方建联，`dws dev connect` 不代建机器人，会输出指引后退出
- 应用名在企业内唯一；`app get` 支持 `--unified-app-id` 或只读 `--app-key` 查详情；写操作定位单应用须用 `--unified-app-id`
