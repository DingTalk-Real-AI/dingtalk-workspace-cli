<h1 align="center">DingTalk Workspace CLI (dws)</h1>

<p align="center"><code>dws</code> — 钉钉工作台命令行工具，为人类和 AI Agent 而生。</p>

<p align="center">
  <img src="https://img.alicdn.com/imgextra/i1/O1CN01oKAc2r28jOyyspcQt_!!6000000007968-2-tps-4096-1701.png" alt="DWS Product Overview" width="100%">
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.25+-green?logo=go&logoColor=white" alt="Go 1.25+">
  <a href="https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/blob/main/LICENSE"><img src="https://img.shields.io/badge/License-Apache_2.0-blue" alt="License Apache-2.0"></a>
  <a href="https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases"><img src="https://img.shields.io/badge/release-v1.1.0-red" alt="v1.1.0"></a>
  <a href="https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/actions/workflows/ci.yml"><img src="https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <img src=".github/badges/coverage.svg" alt="Coverage">
</p>

<p align="center">
  <a href="./README.md">中文版</a> · <a href="./README_en.md">English</a> · <a href="./docs/reference.md">参考手册</a> · <a href="./CHANGELOG.md">更新日志</a>
</p>

> [!IMPORTANT]
> **共创阶段**：本项目涉及钉钉企业数据访问，需企业管理员授权后方可使用。当前为灰度共创阶段，请加入钉钉 DWS 共创群完成白名单配置。详见下方 [开始使用](#开始使用)。
>
> <a href="https://qr.dingtalk.com/action/joingroup?code=v1,k1,v9/YMJG9qXhvFk5juktYnQziN70rF7QHebC/JLztTVRuRVJIwrSsXmL8oFqU5ajJ&_dt_no_comment=1&origin=11"><img src="https://img.alicdn.com/imgextra/i1/O1CN01ZqtgeV1cImFmTZPAH_!!6000000003578-2-tps-398-372.png" alt="DingTalk Group QR Code" width="150"></a>

<details>
<summary><strong>目录</strong></summary>

- [为什么选择 dws？](#why-dws)
- [安装](#安装)
- [开始使用](#开始使用)
- [快速开始](#快速开始)
- [核心服务](#核心服务)
- [安全设计](#安全设计)
- [AI Agent Skills](#ai-agent-skills)
- [参考与文档](#参考与文档)
- [贡献指南](#贡献指南)

</details>

---

<h2 id="why-dws">为什么选择 dws？</h2>

- **为人类而设计** — `--help` 查看用法，`--dry-run` 预览请求，`-f table/json/raw` 切换格式。
- **为 AI Agent 而设计** — 结构化 JSON 响应 + 内置 Agent Skills，开箱即用。
- **为企业管理员而设计** — 零信任架构：OAuth 设备流认证 + 域名白名单 + 权限最小化。**没有一个字节能绕过安全鉴权和审计。**

## 安装

**macOS / Linux：**

```bash
curl -fsSL https://raw.githubusercontent.com/DingTalk-Real-AI/dingtalk-workspace-cli/main/scripts/install.sh | sh
```

**Windows（PowerShell）：**

```powershell
irm https://raw.githubusercontent.com/DingTalk-Real-AI/dingtalk-workspace-cli/main/scripts/install.ps1 | iex
```

<details>
<summary>其他安装方式</summary>

**预编译二进制文件**：从 [GitHub Releases](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases) 下载。

**从源码构建**：

```bash
git clone https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli.git
cd dingtalk-workspace-cli
make build
```

> 二进制文件默认安装到 `~/.local/bin`。如找不到 `dws`，请将其添加到 PATH：`export PATH="$HOME/.local/bin:$PATH"`

</details>

## 开始使用

### 步骤 1：创建钉钉应用

进入 [开放平台应用开发后台](https://open-dev.dingtalk.com/fe/app?hash=%23%2Fcorp%2Fapp#/corp/app)，在「企业内部应用 - 钉钉应用」点击**创建应用**。

<details>
<summary>查看截图</summary>
<p align="center">
  <img src="https://img.alicdn.com/imgextra/i4/O1CN01VIkwvV1a5NQzCIFO0_!!6000000003278-2-tps-2690-1462.png" alt="创建应用" width="600">
</p>
</details>

### 步骤 2：配置重定向 URL

进入应用 → **安全设置**，在「重定向 URL」中添加以下地址并保存：

```
http://127.0.0.1
https://login.dingtalk.com
```

> `http://127.0.0.1` 用于本地浏览器登录；`https://login.dingtalk.com` 用于 `--device` 设备流登录（Docker 容器、远程服务器等无浏览器环境）。建议两个都配置。

<details>
<summary>查看截图</summary>
<p align="center">
  <img src="https://img.alicdn.com/imgextra/i4/O1CN017xQGWb1ycrAG0uxBO_!!6000000006600-2-tps-2000-1032.png" alt="配置重定向URL" width="600">
</p>
</details>

### 步骤 3：发布应用

点击「应用发布 - 版本管理与发布」，发布版本使应用上线。

<details>
<summary>查看截图</summary>
<p align="center">
  <img src="https://img.alicdn.com/imgextra/i4/O1CN01WOLZFz244P46B3FPu_!!6000000007337-2-tps-2000-1100.png" alt="发布应用" width="600">
</p>
</details>

### 步骤 4：申请白名单

加入钉钉 DWS 共创群，提供 **Client ID** 和**管理员确认凭证**完成白名单配置。

### 步骤 5：登录认证

```bash
dws auth login --client-id <your-app-key> --client-secret <your-app-secret>
```

或通过环境变量：

```bash
export DWS_CLIENT_ID=<your-app-key>
export DWS_CLIENT_SECRET=<your-app-secret>
dws auth login
```

> CLI 参数优先于环境变量。凭证用于钉钉 OAuth 设备流认证。

## 快速开始

```bash
dws contact user search --keyword "悟空"           # 搜索联系人
dws calendar event list                            # 查看日历日程
dws todo task create --title "季度汇报" --executors "<userId>"   # 创建待办
dws todo task list --dry-run                       # 预览操作但不执行
```

## 核心服务

| 服务 | 命令 | 描述 |
|---------|---------|-------------|
| 通讯录 | `contact` | 用户 / 部门 |
| 群聊 | `chat` | 群管理 / 群成员 / 机器人消息 / Webhook |
| 日历 | `calendar` | 日程 / 会议室 / 闲忙 |
| 待办 | `todo` | 任务管理 |
| 审批 | `approval` | 流程 / 表单 / 实例 |
| 考勤 | `attendance` | 打卡 / 排班 / 统计 |
| DING | `ding` | DING 消息 / 发送 / 撤回 |
| 日志 | `report` | 日志 / 模版 / 统计 |
| 智能表格 | `aitable` | AI 表格操作 |
| 工作台 | `workbench` | 应用查询 |
| 开发者文档 | `devdoc` | 开放平台文档搜索 |

运行 `dws --help` 查看完整列表，或 `dws <service> --help` 查看子命令。

<details>
<summary>即将推出</summary>

`doc`（文档）· `mail`（邮箱）· `minutes`（AI 听记）· `drive`（钉盘）· `conference`（视频会议）· `tb`（Teambition）· `aiapp`（AI 应用）· `live`（直播）· `skill`（技能市场）

</details>

## 安全设计

`dws` 从架构层面将安全作为一等公民，而非事后补丁。**凭证不落盘、Token 不出域、权限不越界、操作不脱审** — 每一次 API 调用都必须经过钉钉开放平台的鉴权和审计链路，无例外。

<details>
<summary><strong>开发者安全机制</strong></summary>

| 机制 | 说明 |
|------|------|
| **Token 加密存储** | **PBKDF2（600,000 次迭代）+ AES-256-GCM** 加密，密钥由设备 MAC 地址派生，文件拷贝到其他设备无法解密 |
| **域名白名单** | `DWS_TRUSTED_DOMAINS` 默认仅信任 `*.dingtalk.com`，Bearer Token 不会发送到非白名单域 |
| **HTTPS 强制** | 除 loopback 开发调试外，所有请求强制 TLS |
| **Dry-run 预览** | `--dry-run` 展示调用参数但不执行，防止误操作生产数据 |
| **凭证零落盘** | Client ID / Secret 仅在内存中使用，不写入配置文件或日志 |

</details>

<details>
<summary><strong>企业管理员安全机制</strong></summary>

| 机制 | 说明 |
|------|------|
| **OAuth 设备流认证** | 用户必须通过管理员授权的钉钉应用认证，未授权应用无法获取 Token |
| **权限最小化** | CLI 仅能调用管理员授予该应用的 API 权限范围，无法越权 |
| **白名单准入** | 共创阶段需管理员主动确认开通，后续支持自助审批 |
| **操作全链路审计** | 每一次数据读写都经过钉钉开放平台 API，企业管理员可在管理后台实时追溯完整调用日志，任何异常操作无处隐藏 |

</details>

<details>
<summary><strong>ISV / 企业服务商安全机制</strong></summary>

| 机制 | 说明 |
|------|------|
| **租户数据隔离** | 以已授权应用身份调用 API，不同租户数据严格隔离 |
| **Skill 沙箱** | Agent Skills 是 Markdown 文档（`SKILL.md`），仅提供 prompt 描述，不执行任意代码 |
| **集成链路零盲区** | ISV Skill 与 dws Skill 联调时，每一次 API 调用都强制经过钉钉开放平台鉴权，完整调用链路可追溯，不存在绕过审计的旁路 |

</details>

> 发现安全漏洞？请通过 [GitHub Security Advisories](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/security/advisories/new) 报告，详见 [SECURITY.md](./SECURITY.md)。

## AI Agent Skills

仓库为每个钉钉产品提供 Agent Skill（`SKILL.md`），安装脚本会自动部署到 `~/.agents/skills/dws`。

```bash
# 仅安装 skills 到当前项目
curl -fsSL https://raw.githubusercontent.com/DingTalk-Real-AI/dingtalk-workspace-cli/main/scripts/install-skills.sh | sh
```

> `install.sh` 安装到 `$HOME/.agents/skills/dws`（全局）；`install-skills.sh` 安装到 `./.agents/skills/dws`（当前项目）。

### ISV Skill 联调

编写您自己的 Agent Skill，与 dws 内置 skill 搭配构建跨产品工作流：**ISV Skill → dws Skill → 钉钉开放平台 API（强制鉴权 + 全链路审计）**。

**示例**：CRM Skill 调用日历 Skill 为客户创建会议，再通过待办 Skill 分配跟进任务 — AI Agent 在一次对话中完成跨系统协作。

## 参考与文档

- [参考手册](./docs/reference.md) — 环境变量、退出码、输出格式、Shell 补全
- [架构设计](./docs/architecture.md) — 发现驱动管道、IR、Transport 层
- [更新日志](./CHANGELOG.md) — 版本历史与迁移说明

## 贡献指南

参见 [CONTRIBUTING.md](./CONTRIBUTING.md) 了解构建、测试和开发工作流。

## 许可证

Apache-2.0
