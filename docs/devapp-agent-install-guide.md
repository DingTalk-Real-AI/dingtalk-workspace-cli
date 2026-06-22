# DevApp 一键安装与 Agent 接入指南

面向希望用 Codex、Claude、Cursor 等开发 Agent 管理钉钉开放平台应用的开发者。

这份指南参考 Notion Developer Platform 的引导方式：先给出一条可复制的安装命令，再用最短路径完成验证、登录、Agent 调用和排障。

## 一键安装

当前 DevApp 能力在 `feat/dws-devapp` 预览分支上。要安装这个分支里的最新能力，请使用 DevApp 专用安装脚本：

```bash
curl -fsSL https://raw.githubusercontent.com/wxianfeng/dingtalk-workspace-cli/feat/dws-devapp/scripts/install-devapp.sh | sh
```

这个脚本会：

1. 拉取 `wxianfeng/dingtalk-workspace-cli` 的 `feat/dws-devapp` 分支。
2. 使用本地源码构建 `dws`。
3. 安装 `dws` 到默认目录 `~/.local/bin`。
4. 安装 Agent Skill 到本机已检测到的 Agent 目录，只安装通用 `dws` 和 DevApp 专用 `dws-devapp` 两个 skill。

> 预览分支安装需要本机已有 `git`、`go` 和 `make`。Go 版本要求以仓库 `go.mod` 为准。

如果 DevApp 能力已经发布到正式 Release，可以改用正式安装命令：

```bash
curl -fsSL https://raw.githubusercontent.com/DingTalk-Real-AI/dingtalk-workspace-cli/main/scripts/install.sh | sh
```

Windows PowerShell 正式安装命令：

```powershell
irm https://raw.githubusercontent.com/DingTalk-Real-AI/dingtalk-workspace-cli/main/scripts/install.ps1 | iex
```

安装脚本支持这些环境变量：

| 变量 | 说明 |
|---|---|
| `DEVAPP_REPO_URL` | 覆盖源码仓库地址，默认 `https://github.com/wxianfeng/dingtalk-workspace-cli.git` |
| `DEVAPP_BRANCH` | 覆盖安装分支，默认 `feat/dws-devapp` |
| `DEVAPP_SOURCE_DIR` | 使用已有源码目录安装，跳过 clone |
| `DEVAPP_KEEP_SOURCE=1` | 保留临时源码目录，便于调试 |
| `DEVAPP_SKIP_SKILL_SETUP=1` | 跳过自动安装 `dws` 与 `dws-devapp` skill |
| `DEVAPP_SKILL_NAME` | 覆盖 DevApp skill 安装名称，默认 `dws-devapp` |
| `DWS_INSTALL_DIR` | 传给底层 `scripts/install.sh`，覆盖 `dws` 安装目录 |
| `DWS_SKILL_MODE` | 传给底层 `scripts/install.sh`，选择 `mono` 或 `multi` |

## 安装后验证

先确认 `dws` 可执行：

```bash
dws version
```

确认 DevApp 命令存在：

```bash
dws devapp --help --format json
```

如果能看到 `list`、`get`、`create`、`permission`、`robot`、`security`、`version` 等能力，说明 DevApp 已安装成功。

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
dws devapp list --format json
```

## DevApp 是什么

DevApp 是开放平台应用管理能力的 CLI 和 Agent Skill 入口。安装后，开发者和 Agent 可以用统一命令管理企业内部应用，而不需要反复进入开发者后台页面。

它让 Agent 可以完成这些工作：

- 查询、创建、更新、启用、停用、删除开放平台应用。
- 查询应用凭证，读取 `clientId` / `appKey`，敏感凭证走专用命令。
- 配置网页应用首页和管理后台地址。
- 查询、申请、移除权限点。
- 管理应用成员。
- 配置安全项，包括 IP 白名单、登录重定向 URL、端内免登地址。
- 创建、查询、更新、启用、停用机器人。
- 创建版本、发起发布、查询审批和发布状态。

## 给 Agent 使用

安装完成后，可以直接让 Agent 操作 DevApp。

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
帮我发布这个应用版本，先检查发布前置条件。
```

Agent 写操作必须遵循：

1. 先查询定位应用。
2. 先 dry-run 预览。
3. 明确展示将要修改的应用、字段和值。
4. 用户确认后加 `--yes` 执行。
5. 执行后回读验证。

## 第一个写操作

推荐用机器人配置作为 smoke test。先 dry-run：

```bash
dws devapp robot config \
  --unified-app-id <unifiedAppId> \
  --name "告警机器人" \
  --brief "告警通知" \
  --desc "处理告警通知和事件回调" \
  --dry-run \
  --format json
```

确认预览无误后执行：

```bash
dws devapp robot config \
  --unified-app-id <unifiedAppId> \
  --name "告警机器人" \
  --brief "告警通知" \
  --desc "处理告警通知和事件回调" \
  --yes \
  --format json
```

回读验证：

```bash
dws devapp robot get --unified-app-id <unifiedAppId> --format json
```

## 常用命令

### 应用管理

```bash
dws devapp list --format json
dws devapp get --unified-app-id <unifiedAppId> --format json
dws devapp create --name "考勤应用" --dry-run --format json
dws devapp update --unified-app-id <unifiedAppId> --name "新应用名" --dry-run --format json
dws devapp inactive --unified-app-id <unifiedAppId> --dry-run --format json
dws devapp active --unified-app-id <unifiedAppId> --dry-run --format json
dws devapp delete --unified-app-id <unifiedAppId> --dry-run --format json
```

### 凭证查询

```bash
dws devapp credentials get --unified-app-id <unifiedAppId> --format json
```

凭证输出可能包含敏感字段，不要把完整结果写入文档、日志或长期记忆。

### 权限点管理

```bash
dws devapp permission list --unified-app-id <unifiedAppId> --format json
dws devapp permission add --unified-app-id <unifiedAppId> --permissions Contact.User.mobile --dry-run --format json
dws devapp permission remove --unified-app-id <unifiedAppId> --permissions Contact.User.mobile --dry-run --format json
```

权限申请和移除只使用 `scopeValue`，不要传 API 名或权限分组名。

### 机器人配置

```bash
dws devapp robot get --unified-app-id <unifiedAppId> --format json
dws devapp robot config --unified-app-id <unifiedAppId> --name "机器人名称" --dry-run --format json
dws devapp robot enable --unified-app-id <unifiedAppId> --dry-run --format json
dws devapp robot disable --unified-app-id <unifiedAppId> --dry-run --format json
```

### 成员与安全

```bash
dws devapp member list --unified-app-id <unifiedAppId> --format json
dws devapp member add --unified-app-id <unifiedAppId> --users <userId> --dry-run --format json
dws devapp member remove --unified-app-id <unifiedAppId> --users <userId> --dry-run --format json
dws devapp security config --unified-app-id <unifiedAppId> --redirect-url <url> --dry-run --format json
dws devapp security config --unified-app-id <unifiedAppId> --ip-whitelist <ip> --dry-run --format json
```

### 版本发布

```bash
dws devapp version list --unified-app-id <unifiedAppId> --format json
dws devapp version list --unified-app-id <unifiedAppId> --cursor <nextCursor> --format json
dws devapp version create --unified-app-id <unifiedAppId> --dry-run --format json
dws devapp version publish --unified-app-id <unifiedAppId> --version-id <versionId> --dry-run --format json
dws devapp version status --unified-app-id <unifiedAppId> --version-id <versionId> --format json
```

## 安全边界

DevApp 的目标不是绕过开发者后台权限，而是让 CLI、MCP 和 Web 后台保持一致。

默认安全策略：

- 写操作先 dry-run。
- 删除、停用、发布必须由用户确认。
- Agent 不接收用户手动传入的 access token、cookie、`clientSecret`、`appSecret`。
- 应用定位优先使用 `agentId`、`unifiedAppId`、`appKey`。
- 对权限点申请、成员变更、安全配置、版本发布记录操作结果，便于审计和回滚。

## 排障

### `dws devapp` 不存在

先确认安装的是预览分支源码，而不是正式 Release：

```bash
dws version
dws devapp --help --format json
```

如果正式 Release 尚未包含 DevApp，请重新执行本文的一键源码安装命令。

### `dws devapp list` 失败

优先检查登录态：

```bash
dws auth status
dws auth login
```

然后确认当前账号能访问目标企业，并且当前用户在目标企业内。

### 页面能操作，但 CLI 或 MCP 提示无权限

通常说明 CLI/MCP 后端鉴权和 Web 后台权限没有对齐。

先确认当前用户是否满足以下任一条件：

- 应用 owner。
- 应用管理员。
- 应用开发者。
- 企业管理员或具备开放平台应用管理权限的角色。

### 机器人配置失败

先查当前机器人状态：

```bash
dws devapp robot get --unified-app-id <unifiedAppId> --format json
```

如果机器人不存在，使用 `robot config` 创建或配置。
如果机器人已存在，继续用 `robot config` 修改配置，或用 `robot enable` 重新启用。

## 页面文案建议

用于产品页顶部：

```text
Install DevApp in one command.

Let your coding agents manage DingTalk Open Platform apps from the terminal:
create apps, configure robots, apply permissions, manage security settings,
and publish versions with dry-run safety built in.
```

中文版本：

```text
一行命令接入 DevApp。

让 Codex、Claude、Cursor 等开发 Agent 直接管理钉钉开放平台应用：
创建应用、配置机器人、申请权限、管理安全配置、发布版本。
所有写操作先预览，再确认执行。
```

## 参考

- Notion Developer Platform: https://www.notion.com/product/dev
- Notion CLI Help: https://www.notion.com/help/use-notion-from-your-terminal-with-notion-cli
- Notion Developer Platform Blog: https://www.notion.com/blog/introducing-developer-platform
