# dev — 开放平台开发者命令

`dws dev` 是面向**开发者**的命令组，分三个子树：

| 子命令 | 职责 |
|--------|------|
| `dev app` | 应用生命周期（创建/查询/更新/删除/凭证/权限/成员/安全/网页/机器人/**建号**/版本/事件订阅） |
| `dev connect` | **建联**：把现成机器人接到当前本地 agent（起 Stream，不建号） |
| `dev doc` | 开放平台开发文档搜索（同 `dws devdoc`） |

> ⚠️ **关键区分**：`dws chat bot search/find` 只查询已有机器人（IM 视角）；**创建/建号**机器人走 `dws dev app robot submit`；**建联**走 `dws dev connect`。"创建机器人"/"建联"一律走 `dev`，禁止走 `chat`。

---

## 典型工作流：建号 → 建联

```bash
# Step 1：创建开放平台应用（若已有可跳过，用 dev app list 查 unifiedAppId）
dws dev app create --name "我的 AI 机器人" --desc "接 opencode" --format json
# → 返回 unifiedAppId

# Step 2：异步提交机器人创建任务（建号）
dws dev app robot submit --unified-app-id <unifiedAppId> --format json
# → 返回 taskId

# Step 3：轮询任务结果（按 intervalSeconds 轮询）
dws dev app robot result --task-id <taskId> --format json
# → status=SUCCESS 时返回 clientId/clientSecret（clientSecret 只返回一次，务必保存）

# Step 4：建联 — 把机器人接到本地 agent，前台常驻（Ctrl-C 退出）
dws dev connect --channel opencode \
  --robot-client-id <clientId> --robot-client-secret <clientSecret>
# 在 agent 宿主内运行时可自动探测渠道：
dws dev connect --robot-client-id <clientId> --robot-client-secret <clientSecret>
# 也可用 unifiedAppId 自动取凭证（secret 须已在服务端）：
dws dev connect --unified-app-id <unifiedAppId> --channel opencode
```

`robot result` 异步状态：

| status | 含义 | 下一步 |
|--------|------|--------|
| `WAITING` | 创建中 | 按 `intervalSeconds` 继续轮询 |
| `SUCCESS` | 创建完成 | 保存 `robotCode/clientId/clientSecret` |
| `APPROVAL_REQUIRED` | 需要审批才能继续 | 不要重复建号；走后台审批后再轮询 |
| `FAIL` | 失败 | 读 `errorCode/errorMsg`，可带原 `taskId` 重新 `submit` |
| `EXPIRED` | 任务过期 | 重新 `submit` |

---

## dev app — 应用生命周期

```bash
# 查询应用列表
dws dev app list --format json

# 查询单个应用详情
dws dev app get --unified-app-id <unifiedAppId> --format json

# 创建应用
dws dev app create --name <名称> --desc <描述> --format json

# 更新应用信息
dws dev app update --unified-app-id <unifiedAppId> --name <新名称> --format json

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
dws dev app robot submit --unified-app-id <unifiedAppId> --format json

# 建号：轮询结果
dws dev app robot result --task-id <taskId> --format json

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
# 查询 clientId/clientSecret
dws dev app credentials get --unified-app-id <unifiedAppId> --format json

# 重置凭证（旧 secret 立即失效）
dws dev app credentials reset --unified-app-id <unifiedAppId> --yes --format json
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

# 查看/停止后台连接器
dws dev connect status --format json
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

---

## dev app event — 事件订阅

```bash
dws dev app event list       --unified-app-id <unifiedAppId> --format json
dws dev app event subscribe  --unified-app-id <unifiedAppId> --event-type <type> --format json
dws dev app event unsubscribe --unified-app-id <unifiedAppId> --event-type <type> --yes --format json
```

---

## dev app permission — 权限

```bash
dws dev app permission list  --unified-app-id <unifiedAppId> --format json
dws dev app permission add   --unified-app-id <unifiedAppId> --scope-code <scopeValue> --format json
dws dev app permission remove --unified-app-id <unifiedAppId> --scope-code <scopeValue> --yes --format json
```

---

## dev app version — 版本发布

配置变更（权限/机器人/网页等）需通过版本通道才生效。

```bash
dws dev app version create         --unified-app-id <unifiedAppId> --format json
dws dev app version check-approval --unified-app-id <unifiedAppId> --version-id <versionId> --format json
dws dev app version publish        --unified-app-id <unifiedAppId> --version-id <versionId> --format json
dws dev app version status         --unified-app-id <unifiedAppId> --format json
```

---

## 注意事项

- **`clientSecret` 只在 `robot result` 返回一次**，务必立即保存；遗失需走 `credentials reset` 重置
- 改配置后机器人不自动生效，需走 `version create → publish` 才上线
- `hermes`/`openclaw` 渠道走官方建联，`dws dev connect` 不代建机器人，会输出指引后退出
- 应用名在企业内唯一；`app list/get` 用 `--app-key` 过滤但不能定位单应用，定位单应用须用 `--unified-app-id`
