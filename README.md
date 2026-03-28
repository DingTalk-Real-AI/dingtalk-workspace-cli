<h1 align="center">DingTalk Workspace CLI (dws)</h1>

**一个 CLI 搞定钉钉工作台所有功能 — 为人类和 AI Agent 而生。**<br>
覆盖通讯录、日历、待办、考勤、智能表格等核心能力，无需样板代码即可调用，所有响应均为结构化 JSON 输出，并内置 Agent Skills 让 AI 开箱即用。

<p align="center">
  <img src="https://img.alicdn.com/imgextra/i1/O1CN01oKAc2r28jOyyspcQt_!!6000000007968-2-tps-4096-1701.png" alt="DWS Product Overview" width="100%">
</p>

> [!IMPORTANT]
> **共创阶段**：本项目涉及钉钉企业数据访问，需企业管理员授权后方可使用。当前为灰度共创阶段，请加入钉钉 DWS 共创群，提供以下材料给官方人员完成白名单配置：① 钉钉应用的 Client ID；② 企业主管理员确认开通的凭证。后续将支持企业管理员自助审批开通。
>
> <a href="https://qr.dingtalk.com/action/joingroup?code=v1,k1,v9/YMJG9qXhvFk5juktYnQziN70rF7QHebC/JLztTVRuRVJIwrSsXmL8oFqU5ajJ&_dt_no_comment=1&origin=11"><img src="https://img.alicdn.com/imgextra/i1/O1CN01ZqtgeV1cImFmTZPAH_!!6000000003578-2-tps-398-372.png" alt="DingTalk Group QR Code" width="150"></a>

<p>
  <img src="https://img.shields.io/badge/Go-1.25+-green?logo=go&logoColor=white" alt="Go 1.25+">
  <a href="https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/blob/main/LICENSE"><img src="https://img.shields.io/badge/License-Apache_2.0-blue" alt="License Apache-2.0"></a>
  <a href="https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases"><img src="https://img.shields.io/badge/release-v1.0.0-red" alt="v1.0.0"></a>
  <a href="https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/actions/workflows/ci.yml"><img src="https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <img src=".github/badges/coverage.svg" alt="Coverage">
</p>

[中文版](./README.md) | [English](./README_en.md)

## 目录

- [为什么选择 dws？](#why-dws)
- [核心服务](#核心服务)
- [安装](#安装)
- [开始使用](#开始使用)
- [快速开始](#快速开始)
- [AI Agent Skills](#ai-agent-skills)
- [高级用法](#高级用法)
- [环境变量](#环境变量)
- [退出码](#退出码)
- [架构设计](#架构设计)
- [开发指南](#开发指南)
- [测试](#测试)
- [更新日志](#更新日志)
- [安全策略](#安全策略)
- [贡献指南](#贡献指南)

<h2 id="why-dws">为什么选择 dws？</h2>

**为人类而设计** — 告别手写 API 调用。`dws` 为每个资源提供 `--help`，用 `--dry-run` 预览请求，支持表格/JSON/原始格式的结构化输出。

**为 AI Agent 而设计** — 每个响应都是结构化 JSON。配合内置的 agent skills，您的 LLM 无需自定义工具即可管理钉钉工作台。

```bash
# 搜索联系人
dws contact user search --keyword "悟空"

# 创建待办事项
dws todo task create --title "准备季度汇报材料" --executors "<userId>"

# 预览操作但不执行
dws todo task list --dry-run

# JSON 输出供 agent 使用
dws contact user search --keyword "悟空" -f json
```

## 核心服务

`dws` 通过统一的命令界面覆盖钉钉产品：

| 服务 | 命令 | 描述 |
|---------|---------|-------------|
| 通讯录 | `contact` | 通讯录 / 用户 / 部门 |
| 群聊 | `chat` | 群管理 / 群成员 / 机器人消息 / Webhook |
| 智能表格 | `aitable` | AI 表格操作 |
| 日历 | `calendar` | 日历日程 / 会议室 / 闲忙 |
| 待办 | `todo` | 待办任务管理 |
| 审批 | `approval` | 审批流程 / 表单 / 实例 |
| 考勤 | `attendance` | 考勤打卡 / 排班 / 统计 |
| DING | `ding` | DING 消息 / 发送 / 撤回 |
| 日志 | `report` | 日志 / 模版 / 统计 |
| 工作台 | `workbench` | 工作台应用查询 |
| 开发者文档 | `devdoc` | 开放平台文档搜索 |
| 文档 | `doc` | 文档操作（即将推出） |
| 邮箱 | `mail` | 邮件管理（即将推出） |
| AI 听记 | `minutes` | AI 听记 / 会议纪要（即将推出） |
| 钉盘 | `drive` | 云盘 / 文件存储（即将推出） |
| 视频会议 | `conference` | 视频会议（即将推出） |
| Teambition | `tb` | 项目管理（即将推出） |
| AI 应用 | `aiapp` | AI 应用管理（即将推出） |
| 直播 | `live` | 直播管理（即将推出） |
| 技能市场 | `skill` | 技能搜索与下载（即将推出） |

运行 `dws --help` 查看完整列表，或 `dws <service> --help` 查看特定服务的命令。

## 安装

### 一键安装（推荐）

**macOS / Linux：**

```bash
curl -fsSL https://raw.githubusercontent.com/DingTalk-Real-AI/dingtalk-workspace-cli/main/scripts/install.sh | sh
```

**Windows（请先打开 PowerShell / Windows Terminal）：**

```powershell
irm https://raw.githubusercontent.com/DingTalk-Real-AI/dingtalk-workspace-cli/main/scripts/install.ps1 | iex
```

> 请在**已经打开的 PowerShell 窗口**中执行上面的命令，不要直接粘贴到 `Win + R` 的“运行”对话框或 `cmd.exe`。
> 如果你只能从“运行”或 `cmd.exe` 启动，请改用：
> `powershell -NoExit -ExecutionPolicy Bypass -Command "irm https://raw.githubusercontent.com/DingTalk-Real-AI/dingtalk-workspace-cli/main/scripts/install.ps1 | iex"`
>
>
> 自动检测操作系统和架构，从 [GitHub Releases](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases) 下载预编译二进制文件，并安装 Agent Skills 到 `~/.agents/skills/dws` — 无需 Go、Node.js 或其他依赖。大多数 AI Agent（Claude Code、Cursor、Windsurf 等）可自动发现 `.agents/skills/` 目录下的技能。

> [!TIP]
> 二进制文件默认安装到 `~/.local/bin`。如果安装后找不到 `dws` 命令，请将其添加到 PATH：
> ```bash
> export PATH="$HOME/.local/bin:$PATH"
> ```
> 将此行添加到 `~/.bashrc` 或 `~/.zshrc` 以永久生效。

### 预编译二进制文件（手动）

从 [GitHub Releases](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases) 下载适合您平台的最新二进制文件。

### 从源码构建

```bash
git clone https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli.git
cd dingtalk-workspace-cli
make build
./dws version
```

这只会构建二进制文件。如需同时将 agent skills 安装到主目录：

```bash
sh scripts/install.sh
```

这会检测本地源码目录，无需从 GitHub 下载即可安装二进制文件和 skills。

## 开始使用

### 步骤 1：创建钉钉应用

进入 [开放平台应用开发后台](https://open-dev.dingtalk.com/fe/app?hash=%23%2Fcorp%2Fapp#/corp/app)，在「企业内部应用 - 钉钉应用」点击右上角的**创建应用**，新建一个应用。

<p align="center">
  <img src="https://img.alicdn.com/imgextra/i4/O1CN01VIkwvV1a5NQzCIFO0_!!6000000003278-2-tps-2690-1462.png" alt="创建应用" width="600">
</p>

### 步骤 2：配置重定向 URL

创建应用后，进入应用内，点击**安全设置**。在「重定向 URL（回调设置）」里，输入 `http://127.0.0.1` 并保存。

<p align="center">
  <img src="https://img.alicdn.com/imgextra/i4/O1CN017xQGWb1ycrAG0uxBO_!!6000000006600-2-tps-2000-1032.png" alt="配置重定向URL" width="600">
</p>

### 步骤 3：发布应用

点击「应用发布 - 版本管理与发布」，发布版本，使应用变成上线状态。

<p align="center">
  <img src="https://img.alicdn.com/imgextra/i4/O1CN01WOLZFz244P46B3FPu_!!6000000007337-2-tps-2000-1100.png" alt="发布应用" width="600">
</p>

### 步骤 4：申请白名单

参照页面顶部的 [共创阶段说明](#important)，加入钉钉 DWS 共创群完成白名单配置。

### 步骤 5：使用凭证登录

获取 Client ID（AppKey）和 Client Secret（AppSecret）后，可通过 CLI 参数指定：

```bash
dws auth login --client-id <your-app-key> --client-secret <your-app-secret>
```

或者通过环境变量设置：

```bash
export DWS_CLIENT_ID=<your-app-key>
export DWS_CLIENT_SECRET=<your-app-secret>
dws auth login
```

> [!NOTE]
> CLI 参数优先级高于环境变量。这些凭证用于钉钉的 OAuth 设备流认证。

### Token 加密

Token 使用 **PBKDF2（600,000 次迭代）+ AES-256-GCM** 加密存储，密钥由您的设备 MAC 地址生成。

## 快速开始

```bash
dws auth login                                    # 钉钉身份认证
dws contact user search --keyword "悟空"           # 搜索联系人
dws calendar event list                            # 列出日历事件
dws todo task create --title "准备季度汇报材料" --executors "<userId>"   # 创建待办
```

## AI Agent Skills

仓库为每个支持的钉钉产品提供 agent skills（`SKILL.md` 文件）。

Skills 由[安装](#安装)脚本自动安装。如需单独将 skills 安装到现有项目：

```bash
# macOS / Linux — 仅将 skills 安装到当前项目
curl -fsSL https://raw.githubusercontent.com/DingTalk-Real-AI/dingtalk-workspace-cli/main/scripts/install-skills.sh | sh
```

一键安装器（`install.sh`）将 skills 安装到 `~/.agents/skills/dws`（主目录）。
当您想要为特定项目仓库添加 skills 时，请使用 `install-skills.sh`，它会安装到 `./.agents/skills/dws`（当前工作目录）。

> [!NOTE]
> **主目录 vs. 项目 skills**：`install.sh` 将 skills 放在 `$HOME/.agents/skills/dws`。`install-skills.sh` 安装到**当前工作目录**（`./.agents/skills/dws`），适用于为特定项目仓库添加 skills。

## 高级用法

### 输出格式

所有命令支持多种输出格式：

```bash
# 表格（默认，适合人类阅读）
dws contact user search --keyword "悟空" -f table

# JSON（适合 agent 和管道处理）
dws contact user search --keyword "悟空" -f json

# 原始 API 响应
dws contact user search --keyword "悟空" -f raw
```

### 试运行

预览 MCP 工具调用但不执行：

```bash
dws todo task list --dry-run
```

### 输出到文件

```bash
dws contact user search --keyword "李明" -o result.json
```

### Shell 自动补全

```bash
# Bash
dws completion bash > /etc/bash_completion.d/dws

# Zsh
dws completion zsh > "${fpath[1]}/_dws"

# Fish
dws completion fish > ~/.config/fish/completions/dws.fish
```

## 环境变量

常用的运行时和开发覆盖项：

| 变量 | 用途 |
|---------|---------|
| `DWS_CONFIG_DIR` | 覆盖默认配置目录 |
| `DWS_SERVERS_URL` | 将服务发现指向自定义服务器注册端点 |
| `DWS_CLIENT_ID` | OAuth client ID（钉钉 AppKey） |
| `DWS_CLIENT_SECRET` | OAuth client secret（钉钉 AppSecret） |
| `DWS_TRUSTED_DOMAINS` | Bearer token 允许发送的域名列表，逗号分隔（默认 `*.dingtalk.com`）。仅开发环境可设为 `*` |
| `DWS_ALLOW_HTTP_ENDPOINTS` | 设为 `1` 允许对回环地址使用 HTTP（非 TLS），仅用于开发调试 |

## 退出码

| 退出码 | 类别 | 描述 |
|--------|------|------|
| 0 | 成功 | 命令执行成功 |
| 1 | API | MCP 工具调用或上游 API 失败 |
| 2 | 认证 | 身份认证或授权失败 |
| 3 | 校验 | 输入参数、命令行标志或参数 schema 不匹配 |
| 4 | 发现 | 服务发现、缓存或协议协商失败 |
| 5 | 内部 | 未预期的内部错误 |

使用 `-f json` 时，错误响应包含结构化信息（`category`、`reason`、`hint`、`actions` 字段），便于机器消费。

## 架构设计

`dws` 使用 **发现驱动的管道** — 不硬编码任何产品命令：

```
Market Registry ──► Discovery ──► IR (规范化目录) ──► CLI (Cobra) ──► Transport (MCP JSON-RPC)
      │                 │
      ▼                 ▼
  mcp.dingtalk.com   缓存（TTL + 过期降级）
```

1. **Market** — 从 `mcp.dingtalk.com` 获取 MCP 服务注册表
2. **Discovery** — 解析服务运行时能力，支持磁盘缓存和过期降级保证离线可用
3. **IR** — 将服务规范化为统一的产品/工具目录
4. **CLI** — 将目录挂载到 Cobra 命令树，映射 flag 到 MCP 输入参数
5. **Transport** — 执行 MCP JSON-RPC 调用，支持重试、认证注入和响应大小限制

使用 `-f json` 时，所有输出 — 成功、错误和元数据 — 都是结构化 JSON。

## 开发指南

```bash
make build                        # 开发构建
make test                         # 单元测试
make lint                         # 格式化 + lint 检查
make package                      # 本地构建所有发布产物（goreleaser snapshot）
make release                      # 通过 goreleaser 构建和发布
make publish-homebrew-formula     # 将 dist/homebrew/dingtalk-workspace-cli.rb 推送到 tap 仓库
```

### 包管理器产物

构建并验证本地包管理器产物：

```bash
make package                                       # 生成所有平台归档、npm 资源、Homebrew formula
./scripts/release/verify-package-managers.sh        # 验证 dws 二进制文件和 skills 包含在内
```

## 测试

### CLI 测试

运行完整的 CLI 测试套件（单元测试、golden 测试和集成测试）：

```bash
bash test/scripts/run_all_tests.sh --jobs 8
```

### 打包测试

运行打包契约测试和本地包管理器验证：

```bash
go test ./test/scripts/... -count=1
make package
./scripts/release/verify-package-managers.sh
```

### Skills 测试

安装 skills 后，使用 [`test/skill_tests.md`](./test/skill_tests.md) 进行验证。将该文件中的测试提示输入您的 AI agent 并确认预期输出。

## 更新日志

参见 [CHANGELOG.md](./CHANGELOG.md) 了解版本历史和迁移说明。

## 安全策略

报告安全漏洞请参见 [SECURITY.md](./SECURITY.md)。

## 贡献指南

参见 [CONTRIBUTING.md](./CONTRIBUTING.md) 了解开发工作流和本地验证步骤。

## 许可证

Apache-2.0
