# dws dev 一键安装与 Agent 接入指南

面向希望用 Codex、Claude、Cursor 等开发 Agent 管理钉钉开放平台应用的开发者。

这份指南参考 Notion Developer Platform 的引导方式：先给出一条可复制的安装命令，再用最短路径完成验证、登录、Agent 调用和排障。

## 一键安装

`dws dev` 能力已经合入主干并随正式版发布。专用安装脚本会下载预编译二进制 + `dingtalk-dev` skill，**只需要 curl + tar，不需要 git / go / make**。

### macOS / Linux

```bash
curl -fsSL https://raw.githubusercontent.com/DingTalk-Real-AI/dingtalk-workspace-cli/main/scripts/install-devapp.sh | sh
```

### Windows（PowerShell）

```powershell
irm https://raw.githubusercontent.com/DingTalk-Real-AI/dingtalk-workspace-cli/main/scripts/install-devapp.ps1 | iex
```

这个脚本会：

1. 从 `DingTalk-Real-AI/dingtalk-workspace-cli` 的最新 Release 下载对应平台的预编译二进制。
2. 安装 `dws` 到默认目录 `~/.local/bin`。
3. 从 Release 的 skills 包里安装 `dingtalk-dev` skill 到本机已检测到的 Agent 目录。

支持这些环境变量（全部可选）：

| 变量 | 说明 |
|---|---|
| `DEVAPP_REPO` | 覆盖发布仓库，默认 `DingTalk-Real-AI/dingtalk-workspace-cli` |
| `DEVAPP_VERSION` | 钉某个 release tag，默认取最新 release |
| `DWS_INSTALL_DIR` | 二进制安装目录，默认 `~/.local/bin` |
| `DWS_NO_SKILLS` | 设为 `1` 跳过 `dingtalk-dev` skill 安装 |

> `dws dev` 已在正式版里，所以你也可以直接用标准安装脚本 `install.sh`，二者都会带上 `dws dev`。

### 国内加速

`dws dev` 已在正式版里，国内用户直接用标准安装脚本的 Gitee 镜像即可（二进制和 skill 都从 Gitee 拉，避免 GitHub 网络问题）：

```bash
DWS_GITEE_REPO=DingTalk-Real-AI/dingtalk-workspace-cli curl -fsSL https://gitee.com/DingTalk-Real-AI/dingtalk-workspace-cli/raw/main/scripts/install.sh | sh
```

## 安装后验证

先确认 `dws` 可执行：

```bash
dws version
```

确认 `dws dev app` 命令存在：

```bash
dws dev app --help --format json
```

如果能看到 `list`、`get`、`create`、`update`、`permission`、`member`、`robot`、`security`、`version`、`webapp`、`event`、`credentials` 等子命令，说明已安装成功。

确认登录状态：

```bash
dws auth status
```

如果尚未登录：

```bash
dws auth login
```

登录完成后读取应用列表：

```bash
dws dev app list --format json
```

## dws dev 是什么

`dws dev` 是钉钉开放平台开发者命令组，三块能力：

- `dws dev app` — 开放平台企业内部应用的全生命周期管理（创建、配置、权限、成员、安全、机器人、版本发布、事件订阅）。
- `dws dev connect` — 把现成机器人接到当前本地 agent（起 Stream 连接做本地转发，不建号、不产生审批工单）。
- `dws dev doc` — 开放平台开发文档搜索。

安装后，开发者和 Agent 可以用统一命令管理企业内部应用，而不需要反复进入开发者后台页面。它让 Agent 可以完成这些工作：

- 查询、创建、更新、启用、停用、删除开放平台应用。
- 查询应用凭证，读取 `clientId` / `appKey`，敏感凭证走专用命令。
- 配置网页应用首页和管理后台地址。
- 查询、申请、移除权限点。
- 管理应用成员。
- 配置安全项，包括 IP 白名单、登录重定向 URL、端内免登地址。
- 异步创建机器人、配置/启停现有机器人。
- 创建版本、发起发布、查询审批和发布状态。

## 给 Agent 使用

安装完成后，可以直接让 Agent 操作 `dws dev`。

示例：

```text
帮我查一下最近创建的开放平台应用。
```

```text
帮我给 unifiedAppId=<unifiedAppId> 的应用配置机器人，先 dry-run 给我确认。
```

```text
帮我查询这个应用缺哪些权限点，并申请 Contact.User.mobile。
```

```text
帮我发布这个应用版本，先预检是否需要审批。
```

Agent 写操作必须遵循：

1. 先查询定位应用。
2. 先 dry-run 预览。
3. 明确展示将要修改的应用、字段和值。
4. 用户确认后加 `--yes` 执行。
5. 执行后回读验证。

## 第一个写操作

推荐用机器人配置作为 smoke test。建号是异步的，分两步。

提交建号任务（记下返回的 `taskId`）：

```bash
dws dev app robot submit \
  --name "告警助手" \
  --robot-name "告警机器人" \
  --desc "处理告警通知和事件回调" \
  --dry-run \
  --format json
```

确认预览无误后去掉 `--dry-run`、加 `--yes` 执行，再用返回的 `taskId` 查结果，直到 `status` 变成 `SUCCESS`：

```bash
dws dev app robot result --task-id <taskId> --format json
```

对**已有机器人**的应用，改配置/启停用 `robot config` / `robot enable` / `robot disable`：

```bash
dws dev app robot get --unified-app-id <unifiedAppId> --format json
dws dev app robot config --unified-app-id <unifiedAppId> --name "新机器人名称" --dry-run --format json
```

## 常用命令

### 应用管理

```bash
dws dev app list --format json
dws dev app get --unified-app-id <unifiedAppId> --format json
dws dev app create --name "考勤应用" --dry-run --format json
dws dev app update --unified-app-id <unifiedAppId> --name "新应用名" --dry-run --format json
dws dev app enable --unified-app-id <unifiedAppId> --dry-run --format json
dws dev app disable --unified-app-id <unifiedAppId> --dry-run --format json
dws dev app delete --unified-app-id <unifiedAppId> --confirm-name "<应用名>" --format json
```

> 删除不可逆，需要用 `--confirm-name` 传入应用名做二次确认。

### 凭证查询

```bash
dws dev app credentials get --unified-app-id <unifiedAppId> --format json
```

凭证输出可能包含敏感字段，不要把完整结果写入文档、日志或长期记忆。

### 权限点管理

```bash
dws dev app permission list --unified-app-id <unifiedAppId> --format json
dws dev app permission add --unified-app-id <unifiedAppId> --scope-values Contact.User.mobile --dry-run --format json
dws dev app permission remove --unified-app-id <unifiedAppId> --scope-values Contact.User.mobile --dry-run --format json
```

权限申请和移除只使用 `scopeValue`，不要传 API 名或权限分组名。

### 机器人能力

```bash
dws dev app robot get --unified-app-id <unifiedAppId> --format json
dws dev app robot submit --name "<智能体名>" --robot-name "<机器人名>" --desc "<描述>" --dry-run --format json
dws dev app robot result --task-id <taskId> --format json
dws dev app robot config --unified-app-id <unifiedAppId> --name "机器人名称" --dry-run --format json
dws dev app robot enable --unified-app-id <unifiedAppId> --dry-run --format json
dws dev app robot disable --unified-app-id <unifiedAppId> --dry-run --format json
```

### 成员与安全

```bash
dws dev app member list --unified-app-id <unifiedAppId> --format json
dws dev app member add --unified-app-id <unifiedAppId> --user-ids <userId> --dry-run --format json
dws dev app member remove --unified-app-id <unifiedAppId> --user-ids <userId> --dry-run --format json
dws dev app security config --unified-app-id <unifiedAppId> --redirect-urls <url> --dry-run --format json
dws dev app security config --unified-app-id <unifiedAppId> --ip-whitelist <ip> --dry-run --format json
```

### 网页应用与事件

```bash
dws dev app webapp get --unified-app-id <unifiedAppId> --format json
dws dev app webapp config --unified-app-id <unifiedAppId> --homepage-url <url> --dry-run --format json
dws dev app event list --unified-app-id <unifiedAppId> --format json
dws dev app event subscribe --unified-app-id <unifiedAppId> --dry-run --format json
dws dev app event unsubscribe --unified-app-id <unifiedAppId> --dry-run --format json
```

### 版本发布

```bash
dws dev app version list --unified-app-id <unifiedAppId> --format json
dws dev app version create --unified-app-id <unifiedAppId> --dry-run --format json
dws dev app version check-approval --unified-app-id <unifiedAppId> --version-id <versionId> --format json
dws dev app version publish --unified-app-id <unifiedAppId> --version-id <versionId> --dry-run --format json
dws dev app version status --unified-app-id <unifiedAppId> --version-id <versionId> --format json
```

> 发布前先用 `version check-approval` 预检是否需要审批。含高敏权限的版本，`publish` 需加 `--confirmed-sensitive`。

## 安全边界

`dws dev` 的目标不是绕过开发者后台权限，而是让 CLI、MCP 和 Web 后台保持一致。

默认安全策略：

- 写操作先 dry-run。
- 删除、停用、发布必须由用户确认（删除还需 `--confirm-name` 二次确认）。
- Agent 不接收用户手动传入的 access token、cookie、`clientSecret`、`appSecret`。
- 应用定位优先使用 `unifiedAppId`、`agentId`、`appKey`。
- 对权限点申请、成员变更、安全配置、版本发布记录操作结果，便于审计和回滚。

## 排障

### `dws dev app` 不存在

先确认装上的是带 `dws dev` 的版本：

```bash
dws version
dws dev app --help --format json
```

如果命令缺失，重新执行本文的一键安装命令（或标准 `install.sh`）升级到最新正式版。

### `dws dev app list` 失败

优先检查登录态：

```bash
dws auth status
dws auth login
```

然后确认当前账号能访问目标企业，并且当前用户在目标企业内。

### 提示"当前用户没有开发者身份"

创建应用需要开放平台开发者权限。请企业管理员在钉钉开放平台（open-dev.dingtalk.com）的「权限管理」中把你的账号添加为开发者，然后重试。

### 页面能操作，但 CLI 或 MCP 提示无权限

通常说明 CLI/MCP 后端鉴权和 Web 后台权限没有对齐。先确认当前用户是否满足以下任一条件：

- 应用 owner。
- 应用管理员。
- 应用开发者。
- 企业管理员或具备开放平台应用管理权限的角色。

### 机器人配置失败

先查当前机器人状态：

```bash
dws dev app robot get --unified-app-id <unifiedAppId> --format json
```

如果机器人不存在，用 `robot submit` 异步创建；如果已存在，用 `robot config` 修改，或用 `robot enable` 重新启用。

## 页面文案建议

用于产品页顶部：

```text
Install dws dev in one command.

Let your coding agents manage DingTalk Open Platform apps from the terminal:
create apps, configure robots, apply permissions, manage security settings,
and publish versions with dry-run safety built in.
```

中文版本：

```text
一行命令接入 dws dev。

让 Codex、Claude、Cursor 等开发 Agent 直接管理钉钉开放平台应用：
创建应用、配置机器人、申请权限、管理安全配置、发布版本。
所有写操作先预览，再确认执行。
```

## 参考

- Notion Developer Platform: https://www.notion.com/product/dev
- Notion CLI Help: https://www.notion.com/help/use-notion-from-your-terminal-with-notion-cli
- Notion Developer Platform Blog: https://www.notion.com/blog/introducing-developer-platform
