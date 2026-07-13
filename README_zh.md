<h1 align="center">DingTalk Workspace CLI (dws)</h1>

<p align="center"><code>dws</code> — 钉钉工作台命令行工具，为人类和 AI Agent 而生。</p>

<p align="center">
  <img src="https://img.alicdn.com/imgextra/i1/O1CN01oKAc2r28jOyyspcQt_!!6000000007968-2-tps-4096-1701.png" alt="DWS Product Overview" width="100%">
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.25+-green?logo=go&logoColor=white" alt="Go 1.25+">
  <a href="https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/blob/main/LICENSE"><img src="https://img.shields.io/badge/License-Apache_2.0-blue" alt="License Apache-2.0"></a>
  <a href="https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases"><img src="https://img.shields.io/github/v/release/DingTalk-Real-AI/dingtalk-workspace-cli?color=red&label=release" alt="Latest Release"></a>
  <a href="https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/actions/workflows/ci.yml"><img src="https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <img src=".github/badges/coverage.svg" alt="Coverage">
</p>

<p align="center">
  <a href="./README_zh.md">中文版</a> · <a href="./README.md">English</a> · <a href="./docs/reference.md">参考手册</a> · <a href="./CHANGELOG.md">更新日志</a>
</p>

> [!IMPORTANT]
> **共创阶段**：本项目涉及钉钉企业数据访问，需企业管理员授权后方可使用。欢迎加入钉钉 DWS 共创群获取支持与最新动态。详见下方 [开始使用](#开始使用)。
>
> <img src="https://img.alicdn.com/imgextra/i1/O1CN01WJyAsJ1prD2ovQACM_!!6000000005413-2-tps-718-720.png" alt="dws 开源沟通群二维码" width="150">

<details>
<summary><strong>目录</strong></summary>

- [为什么选择 dws？](#why-dws)
- [安装](#安装)
- [升级](#升级)
- [开始使用](#开始使用)
- [快速开始](#快速开始)
- [在 Agent 中使用](#在-agent-中使用)
- [功能特性](#功能特性)
- [核心服务](#核心服务)
- [安全设计](#安全设计)
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
<summary><strong>Skill 模式：mono 与 multi</strong></summary>

安装时可以选择两种 skill 组织方式。两种模式下 CLI 命令完全一样（`dws aitable ...` / `dws calendar ...`），区别只在 Agent 那边读到的 skill 文档结构。

| 模式 | 安装内容 | 适合场景 |
|------|----------|----------|
| **mono**（稳定，默认） | 一个 `dws` skill，覆盖全部产品 | 跨产品组合操作；单一入口召唤 |
| **multi** 🧪 **试验版 / Preview** | 22 个独立产品 skill（`dingtalk-aitable` / `dingtalk-calendar` / `dingtalk-chat` ...） | 单产品任务；每次召唤上下文更小 |

> 🧪 **multi 模式当前为 EXPERIMENTAL（试验版 / Preview）**。22 个独立 skill 全部通过 dispatch verifier，但接口、命名、跨 skill 引用后续可能调整。生产 / 共享环境建议优先用 `mono`。问题请提 issue 反馈。

怎么选：

- **快速安装**（上方一行 curl）：非交互，默认装 `mono`。
- **TTY 安装**（先下载再执行）：`curl -O .../install.sh && bash install.sh`，会弹出 `1) mono  2) multi` 选项（默认 1）。
- **环境变量覆盖**：`DWS_SKILL_MODE=multi curl -fsSL ... | sh`。
- **装完之后再切换**：`dws skill setup --mode multi`（或 `--mode mono`），随时重跑都行。

</details>

<details>
<summary>其他安装方式</summary>

**npm**（需要 Node.js（npm/npx））：

```bash
npm install -g dingtalk-workspace-cli
```

**预编译二进制文件**：从 [GitHub Releases](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases) 下载。

> **macOS 用户注意**：如果提示“无法打开，因为 Apple 无法检查其是否包含恶意软件”，请执行：
> ```bash
> xattr -d com.apple.quarantine /path/to/dws
> ```

**从源码构建**：

```bash
git clone https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli.git
cd dingtalk-workspace-cli
go build -o dws ./cmd       # 编译到当前目录
cp dws ~/.local/bin/         # 安装到 PATH
```

> 需要 Go 1.25+。也可以用 `make package` 构建所有平台产物（macOS / Linux / Windows × amd64 / arm64）。
> 静态端点数据由悟空基线生成并提交在本仓库 `internal/syncdata`，源码构建不需要额外 checkout 数据仓库。

</details>

## 国内加速安装

国内用户可使用以下通道，避免 GitHub 网络问题。默认（不设置这些环境变量）走 GitHub。

**1. 安装脚本 + 预编译二进制（Gitee 镜像）：**

仓库镜像地址：`https://gitee.com/DingTalk-Real-AI/dingtalk-workspace-cli`

```bash
DWS_GITEE_REPO=DingTalk-Real-AI/dingtalk-workspace-cli curl -fsSL https://gitee.com/DingTalk-Real-AI/dingtalk-workspace-cli/raw/main/scripts/install.sh | sh
```

> 设置 `DWS_GITEE_REPO` 后，安装脚本会改从 Gitee API 解析最新版本和各个 release 产物（二进制、校验和、skills 包），而不是走 GitHub。不设置时默认从 GitHub 安装。

**2. npm 包（npmmirror 镜像）：**

```bash
npm install -g dingtalk-workspace-cli --registry=https://registry.npmmirror.com
```

> npmmirror 会自动同步公网 npm 的公开包，国内可直接使用。

**3. 单独安装 Skills（Gitee 镜像）：**

```bash
DWS_GITEE_REPO=DingTalk-Real-AI/dingtalk-workspace-cli curl -fsSL https://gitee.com/DingTalk-Real-AI/dingtalk-workspace-cli/raw/main/scripts/install-skills.sh | sh
```

> 同样设置 `DWS_GITEE_REPO`，`install-skills.sh` 会从 Gitee 解析版本和 skills 包；GitHub 不可达时也会自动回退到 Gitee 镜像。

## 升级

> 需要 **v1.0.7** 及以上版本。更早版本请重新执行[安装脚本](#安装)进行升级。

dws 内置自升级能力，直接从 [GitHub Releases](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/releases) 拉取更新，支持 SHA256 完整性校验和自动备份。

```bash
dws upgrade                    # 交互式升级到最新版本
dws upgrade --check            # 仅检查是否有新版本
dws upgrade --list             # 列出正式 release 版本
dws upgrade --beta             # 升级到最新 beta 预发布版本
dws upgrade --check --beta     # 仅检查 beta 轨道是否有新版本
dws upgrade --list --beta      # 列出 beta 预发布版本
dws upgrade --version v1.0.7   # 升级到指定版本
dws upgrade --version v1.0.8-beta.1  # 升级到指定 beta 版本
dws upgrade --rollback         # 回滚到上一版本
dws upgrade -y                 # 跳过确认直接升级
```

默认情况下，`dws upgrade` 只跟随正式 release 轨道。只有显式传入 `--beta` 时，才会选择 GitHub pre-release 里的 beta 构建。

<details>
<summary><strong>工作原理</strong></summary>

升级过程采用两阶段原子流程，确保一致性：

1. **准备阶段** — 将平台对应的二进制文件和技能包下载到临时目录，校验 SHA256 校验和，解压并验证所有文件。任何步骤失败则立即中止，不会修改现有安装。
2. **执行阶段** — 仅在所有准备工作成功后，替换二进制文件并将技能包安装到所有已检测到的 Agent 目录（`~/.agents/skills/dws`、`~/.claude/skills/dws`、`~/.cursor/skills/dws` 等）。

每次升级前自动备份当前版本，可通过 `dws upgrade --rollback` 随时回滚。

| Flag | 说明 |
|------|------|
| `--check` | 仅检查更新，不安装 |
| `--list` | 列出正式 release 版本及更新日志 |
| `--beta` | 对 `upgrade`、`--check`、`--list` 使用 beta 预发布轨道 |
| `--version` | 升级到指定版本（如 `v1.0.7` 或 `v1.0.8-beta.1`） |
| `--rollback` | 回滚到上一个备份版本 |
| `--force` | 强制重新安装，即使已是最新版本 |
| `--skip-skills` | 跳过技能包更新 |
| `-y` | 跳过确认提示 |

</details>

## 开始使用

```bash
dws auth login            # 自动唤起浏览器
dws auth login --device   # 无浏览器环境（Docker、SSH、CI）
```

选择组织并授权即可。

> 如果组织尚未开启 CLI 访问权限，系统会引导你向管理员发送申请。审批通过后重新执行 `dws auth login` 即可。

<details>
<summary><strong>组织未开启 CLI 访问权限？</strong></summary>

1. 选择组织后，点击「立即申请」通知管理员
2. 管理员收到申请卡片，一键审批
3. 审批通过后，重新执行 `dws auth login`

<p align="center">
  <img src="https://img.alicdn.com/imgextra/i2/O1CN01wtsYuQ1CTbboVTlsD_!!6000000000082-2-tps-2696-1544.png" alt="申请权限" width="600">
</p>

</details>

<details>
<summary><strong>管理员：为组织开启 CLI 访问权限</strong></summary>

进入 [开发者平台](https://open-dev.dingtalk.com) →「CLI 访问管理」→ 开启。

<p align="center">
  <img src="https://img.alicdn.com/imgextra/i4/O1CN01M8K7Wj1rZ0WikrZby_!!6000000005644-2-tps-2940-1596.png" alt="CLI访问管理" width="600">
</p>

</details>

<details>
<summary><strong>自建应用模式（CI/CD、ISV 集成）</strong></summary>

企业自主管控场景，可创建自有钉钉应用：

1. [开放平台应用开发后台](https://open-dev.dingtalk.com/fe/app#/corp/app) → 创建应用
2. 安全设置 → 添加重定向 URL：`http://127.0.0.1,https://login.dingtalk.com`
3. 发布应用
4. 登录：

```bash
dws auth login --client-id <your-app-key> --client-secret <your-app-secret>
```

首次登录后凭证安全存储（Keychain），后续自动刷新 Token。

</details>

<details>
<summary><strong>多组织（profile）</strong></summary>

`dws` 可以同时登录多个钉钉组织。一个组织就是一个 **profile**，当前 profile 决定本次命令操作哪个组织（凭证按组织分别存储）。

```bash
dws auth login                              # 再登录一个组织 → 新增一个 profile（首次登录的为主组织）
dws profile list                            # 列出已登录组织（主 / 当前标记、状态）
dws profile switch <名称|corpId>            # 切换默认组织（用 - 切回上一个）
dws --profile <名称|corpId> contact user search --query "..."   # 单次对指定组织执行，不改默认组织
```

跨组织读取由 agent 编排，而非内置 `--all-orgs`：先 `dws profile list` 拿到组织，再对每个组织带 `--profile` 各查一遍，然后合并。写操作默认只在当前组织进行——跨组织写之前先确认目标组织。

macOS 下，如果已登记的 token slot 无法解密，为避免把系统 Keychain 和 file-DEK 写成混合状态，新的 OAuth 登录会直接拒绝。如果普通终端仍能读取登录态、只有设置 `DWS_DISABLE_KEYCHAIN=1` 的沙箱读不到，可在不暴露 token 的情况下迁移 legacy 与各 profile 的认证条目：

```bash
env -u DWS_DISABLE_KEYCHAIN dws auth migrate-keychain --to file-dek --dry-run --format json
env -u DWS_DISABLE_KEYCHAIN dws auth migrate-keychain --to file-dek --yes --format json
DWS_DISABLE_KEYCHAIN=1 dws auth status --format json
```

迁移会先验证全部认证密文再写入、忽略无关的应用密钥；提交中断后可安全重跑。如果预检确认是密文本身损坏，报错会给出对应 `corpId`；只清理这个组织可执行 `dws auth logout --profile <名称|corpId>`，再重新登录。只有确认要丢弃全部本地 profile 时才用 `dws auth reset`。

</details>

<details>
<summary><strong>沙箱间迁移登录态（Linux）</strong></summary>

仅拷贝 `~/.dws/app.json` 无法带走 refresh token；access token 约 2 小时后会失效。请使用官方导出/导入：

```bash
# A 沙箱（已登录）
dws auth export -o /tmp/dws-auth.tar.gz
# 或便于分片复制：dws auth export --base64 -o /tmp/dws-auth.b64

# B 沙箱
dws auth import -i /tmp/dws-auth.tar.gz
# 或：dws auth import -i /tmp/dws-auth.b64 --base64
dws auth status   # 确认 Refresh Token: 有效
```

包内包含 `~/.local/share/dws-cli` 加密 keychain（含 `auth-token.enc` 与 `dek`）及 `~/.dws` 必要配置。

</details>

## 快速开始

```bash
dws contact user search --query "悟空"             # 搜索联系人
dws calendar event list                            # 查看今天的日程
dws doc search --query "季度"                      # 搜索钉钉文档
dws minutes list mine                              # 列出我创建的 AI 听记
dws drive list                                     # 列出钉盘文件
dws todo task create --title "季度汇报" --executors "<your-userId>"   # 创建待办（请替换为真实 userId）
dws todo task list --dry-run                       # 预览操作但不执行
```

> **完整命令列表**：[`docs/command-index.md`](./docs/command-index.md) — 全部命令，带描述和使用场景。

## 在 Agent 中使用

dws 是为 AI Agent 设计的 CLI 工具。请先完成[安装](#安装)和[开始使用](#开始使用)，然后配置 Agent 环境：

### Agent 调用模式

```bash
# 使用 --yes 跳过确认提示（Agent 必须）
dws todo task create --title "Review PR" --executors "<your-userId>" --yes

# 使用 --dry-run 预览操作（安全执行）
dws contact user search --query "张三" --dry-run

# 使用 --jq 精确提取（节省 token）
dws contact user get-self --jq '.result[0].orgEmployeeModel | {name: .orgUserName, dept: .depts[0].deptName, userId}'
```

### 命令帮助与 Schema

产品命令在静态端点模式下已经编译进二进制。Agent 以 `--help` 和内置 Skill 为事实源；`dws schema` 仅保留给 `dev.*` 等 helper-only schema 查询。

```bash
# 查看当前编译出的命令面
dws aitable record query --help

# helper-only schema 自省
dws schema "dev app create"

# 构造正确的调用
dws aitable record query --base-id BASE_ID --table-id TABLE_ID --limit 10
```

### Agent Skills

仓库内置完整的 Agent Skill 体系（`skills/` 目录），目前重组为两套布局：

- `skills/mono/` — 单 skill 布局（一个 `SKILL.md` + `references/products/`），默认推荐。
- `skills/multi/` — 每个产品一个独立 skill（`dingtalk-aitable/` / `dingtalk-calendar/` / `dingtalk-chat/` ... 共 22 个），每个 skill 自带 `SKILL.md`。🧪 **试验版 / Preview — 各 multi `SKILL.md` 头部有详细注意事项。**

安装之后，Claude Code / Cursor 等 AI 工具就能通过自然语言直接操作钉钉：

```bash
# 安装 skills 到当前项目（默认 mono）
curl -fsSL https://raw.githubusercontent.com/DingTalk-Real-AI/dingtalk-workspace-cli/main/scripts/install-skills.sh | sh
```

> `install.sh` 安装到 `$HOME/.agents/skills/dws`（全局）；`install-skills.sh` 安装到 `./.agents/skills/dws`（当前项目）。
>
> 国内用户加 `DWS_GITEE_REPO` 走 Gitee 镜像，见 [国内加速安装](#国内加速安装)。

**用 `dws skill setup` 切换或重装：**

```bash
# 交互式：提示选模式 + 目标 Agent
dws skill setup

# 把 mono skill 铺到所有检测到的 Agent home（claude / cursor / codex / opencode / qoder）
dws skill setup --mode mono --target all --yes

# 只装到某一个 Agent home
dws skill setup --mode multi --target cursor --yes

# 指定本地源目录（比如 fork 或正在改的版本）
DWS_SKILL_SOURCE=/path/to/skills dws skill setup --mode multi
```

| 参数 | 取值 | 说明 |
|------|------|------|
| `--mode` | `mono` \| `multi` | skill 布局，不指定则交互式询问 |
| `--target` | `all` \| `claude` \| `cursor` \| `codex` \| `opencode` \| `qoder` | 安装目标，`all` 表示铺到所有检测到的 Agent home |
| `--source` | 路径 | 本地源目录（覆盖内置 skills） |
| `--yes` | — | 跳过确认提示 |

环境变量：`DWS_SKILL_MODE=mono|multi`（`install.sh` / `install.ps1` 也认）、`DWS_SKILL_SOURCE=<路径>`。

**包含内容（mono 布局）：**

| 组件 | 路径 | 说明 |
|------|------|------|
| 主 Skill | `skills/mono/SKILL.md` | 意图路由、决策树、安全规则、错误处理 |
| 产品参考 | `skills/mono/references/products/*.md` | 各产品命令详细参考（aitable、chat、calendar 等） |
| 意图指南 | `skills/mono/references/intent-guide.md` | 易混淆场景消歧（如 report vs todo） |
| 全局参考 | `skills/mono/references/global-reference.md` | 认证、输出格式、全局 flag |
| 错误码 | `skills/mono/references/error-codes.md` | 错误码 + 调试流程 |
| Recovery 指南 | `skills/mono/references/recovery-guide.md` | `RECOVERY_EVENT_ID` 处理 |
| 现成脚本 | `skills/mono/scripts/*.py` | 13 个批量操作脚本（见下方） |

<details>
<summary><strong>现成脚本</strong> — 13 个 Python 脚本，覆盖常见多步工作流</summary>

| 脚本 | 说明 |
|------|------|
| `calendar_schedule_meeting.py` | 一键创建日程 + 添加参与者 + 搜索并预定空闲会议室 |
| `calendar_free_slot_finder.py` | 查询多人共同空闲时段，推荐最佳会议时间 |
| `calendar_today_agenda.py` | 查看今天/明天/本周的日程安排 |
| `import_records.py` | 从 CSV/JSON 批量导入记录到 AI 表格 |
| `bulk_add_fields.py` | 批量添加字段到 AI 表格数据表 |
| `upload_attachment.py` | 上传附件到 AI 表格 attachment 字段 |
| `todo_batch_create.py` | 从 JSON 文件批量创建待办（含优先级、截止时间、执行者） |
| `todo_daily_summary.py` | 汇总今天/本周未完成的待办 |
| `todo_overdue_check.py` | 扫描已过截止时间但未完成的待办，输出逾期清单 |
| `contact_dept_members.py` | 按部门名称搜索并列出所有成员 |
| `attendance_my_record.py` | 查看我今天/本周/指定日期的考勤记录 |
| `attendance_team_shift.py` | 查询团队成员本周排班和出勤统计 |
| `report_inbox_today.py` | 查看今天收到的日志列表及详情 |

</details>

**ISV 集成**：编写您自己的 Agent Skill，与 dws 内置 Skill 搭配构建跨产品工作流：**ISV Skill → dws Skill → 钉钉开放平台 API（强制鉴权 + 全链路审计）**。

## 功能特性

<details>
<summary><strong>个人事件订阅</strong> — 实时接收钉钉消息，驱动事件触发的 Agent</summary>

`dws event consume` 使用当前 OAuth 登录用户建立托管的 Stream WebSocket 长连接，并把每条事件以 NDJSON 一行输出到 stdout。当前公开目录包括：当前用户被 @ 的消息、与指定用户的单聊消息、指定群的消息。

> **前置条件**：先运行 `dws auth login`。个人身份从 OAuth token 解析，不允许通过命令行伪造。

只需要 event 能力时，可以使用官方便捷安装脚本：

```bash
curl -fsSL https://raw.githubusercontent.com/DingTalk-Real-AI/dingtalk-workspace-cli/main/scripts/install-event.sh | sh
```

```bash
# 查看公开个人事件目录和 schema
dws event list
dws event schema user_im_message_receive_o2o

# 监听当前用户被 @ 的消息
dws event consume user_im_message_receive_at -f ndjson

# 监听与指定用户的单聊消息
dws event consume user_im_message_receive_o2o --user <userId> -f ndjson

# 监听指定群的消息
dws event consume user_im_message_receive_group --group <openConversationId> -f ndjson

# 查看本地 consume，并取消指定订阅
dws event status
dws event stop <subscribe_id>
```

| 特性 | 说明 |
|------|------|
| 自动编排 | `consume` 创建或复用个人订阅，`stop` 取消订阅并清理本地状态 |
| 共享连接 | 同一用户的多个 consumer 共享本地 bus 和云端长连接 |
| 订阅隔离 | 正常 consumer 同时按事件类型和 `subscribe_id` 匹配 |
| Agent 友好输出 | Stream 事件写入 stdout，连接状态和诊断信息写入 stderr |
| 状态可观测 | `status` 同时显示服务端订阅、personal bus 和本地 consumers |
| 跨平台 | macOS/Linux 使用 Unix Socket，Windows 使用 Named Pipe |

Agent 工作流和事件参数详见 `skills/multi/dingtalk-event/SKILL.md`。

</details>

<details>
<summary><strong>Raw API 调用</strong> — 直接调用钉钉 OpenAPI</summary>

`dws api` 让你直接调用任意钉钉 OpenAPI，无需 SDK，Token 自动获取和刷新。

> **前置条件**：必须使用自有应用凭证登录（见[自建应用模式](#开始使用)）。通过 MCP 默认凭证登录 不支持 raw API 调用。

```bash
# 登录（仅首次）
dws auth login --client-id <APP_KEY> --client-secret <APP_SECRET>

# === api.dingtalk.com ===

# 获取企业所有应用列表
dws api GET /v1.0/microApp/allApps

# 搜索用户 (POST + JSON body)
dws api POST /v1.0/contact/users/search \
  --data '{"queryWord":"张三","offset":0,"size":10}'

# === oapi.dingtalk.com ===

# 获取用户详情（使用 --base-url 指定域名）
dws api POST /topapi/v2/user/get \
  --base-url https://oapi.dingtalk.com \
  --data '{"userid":"<USER_ID>"}'

# 也可以直接使用完整 URL
dws api POST https://oapi.dingtalk.com/topapi/v2/user/get \
  --data '{"userid":"<USER_ID>"}'

# === 通用功能 ===
dws api GET /v1.0/microApp/allApps --page-all   # 自动翻页
dws api GET /v1.0/microApp/allApps --dry-run     # 预览请求
dws api GET /v1.0/microApp/allApps --jq '.agentId'  # jq 过滤
```

| 特性 | 说明 |
|------|------|
| 双形态自动识别 | 根据 URL 自动选择 api.dingtalk.com（Header 认证）或 oapi.dingtalk.com（Query 参数认证） |
| Token 自动管理 | 首次调用自动获取应用级 accessToken，有效期内缓存，过期自动刷新 |
| 域名白名单 | 仅允许 `api.dingtalk.com` 和 `oapi.dingtalk.com`，防止 Token 泄露 |
| 自动分页 | `--page-all` 自动遍历所有分页。`--page-limit` 控制翻页上限（默认 10，设为 0 不限制，硬上限 500 防止死循环） |

</details>

<details>
<summary><strong>智能输入纠错</strong> — 自动修正 AI 模型常见的参数错误</summary>

内置 Pipeline 纠错引擎，支持命名风格转换、粘连参数拆分、拼写模糊匹配：

```bash
# 命名风格自动转换 (camelCase / snake_case / UPPER → kebab-case)
dws aitable record query --baseId BASE_ID --tableId TABLE_ID         # 自动纠正为 --base-id --table-id

# 粘连参数自动拆分
dws contact user search --query "张三" --timeout30                  # 自动拆分为 --timeout 30

# 拼写错误模糊匹配
dws aitable record query --base-id BASE_ID --tabel-id TABLE_ID       # --tabel-id → --table-id

# 参数值归一化 (布尔 / 数字 / 日期 / 枚举)
# "yes" → true, "1,000" → 1000, "2024/03/29" → "2024-03-29", "ACTIVE" → "active"
```

| Agent 输出 | dws 自动纠正为 |
|-----------|--------------|
| `--userId` | `--user-id` |
| `--limit100` | `--limit 100` |
| `--tabel-id` | `--table-id` |
| `--USER-ID` | `--user-id` |
| `--user_name` | `--user-name` |

</details>

<details>
<summary><strong>jq 过滤 & 字段筛选</strong> — 精确控制输出，减少 token 消耗</summary>

```bash
# 内置 jq 表达式
dws aitable record query --base-id BASE_ID --table-id TABLE_ID --jq '.invocation.params'
dws schema "dev app create" --jq '.tool.required'

# 只返回指定字段
dws aitable record query --base-id BASE_ID --table-id TABLE_ID --fields invocation,response
```

</details>

<details>
<summary><strong>Schema 自省</strong> — 静态端点模式下的 helper-only schema</summary>

```bash
dws schema                                              # 静态端点模式提示
dws schema "dev app create"                             # 查看 helper-only schema
dws schema "dev app create" --jq '.tool.required'        # 查看必填字段
```

</details>

<details>
<summary><strong>管道 & 文件输入</strong> — 从文件或 stdin 读取 flag 值</summary>

```bash
# 从文件读取消息内容
dws chat message send-by-bot --robot-code BOT_CODE --group GROUP_ID \
  --title "周报" --text @report.md

# 通过管道传入内容
cat report.md | dws chat message send-by-bot --robot-code BOT_CODE --group GROUP_ID \
  --title "周报"

# 显式从 stdin 读取
dws chat message send-by-bot --robot-code BOT_CODE --group GROUP_ID \
  --title "周报" --text @-
```

> **说明**：`@` 仅在其后是 ASCII 路径前缀字符（`A-Z` / `a-z` / `0-9` / `.` / `/` / `~` / `_` / `-`）或 `@-`（stdin）时，才会被识别为 `@<path>` 文件注入语法。`--text "@所有人 周报"` / `--text "@张三 看一下"` 这类机器人消息中的字面 `@` 提及会原样透传到 API。

</details>

## 钉钉机器人 —— 把机器人接到你本地的 AI

`dws dev connect` 把一个钉钉机器人接到本地 AI CLI（Claude Code / Codex / opencode / Qoder / Gemini，或用 `--agent-cmd` 接任意工具）：群里 @ 机器人提问，它用你本地的 agent 回答，按会话保留多轮上下文。

```bash
dws dev connect --channel auto --robot-client-id <id> --robot-client-secret <secret>
```

聊天里的**会话指令**（整条消息就是指令时生效，不消耗一次 AI 调用）：

| 指令 | 作用 |
|------|------|
| `/new`（别名 `/start`、`/reset`） | 开启新会话；旧会话保留（agent 支持的话仍可回溯） |
| `/clear` | 清空当前会话 —— 调 agent 真实会话原语真删（opencode 走 `DELETE /session/:id`）；驱动接口没有删除原语的渠道退化为重置 |

完整四步教程见 [`docs/robot-quickstart.md`](./docs/robot-quickstart.md)（装工具 → 建机器人 → 接上 AI → 拉进群）。

## 核心服务

| 服务 | 命令 | 能力 |
|------|------|------|
| 通讯录 | `contact` | 按姓名 / 手机号 / 工号查人，部门、角色标签、花名册与离职 |
| 群聊 | `chat`（`im`）| 发送 / 回复 / 搜索消息，群与成员管理，机器人与 Webhook 发消息，表情反应，撤回 |
| 日历 | `calendar` | 日程 CRUD、参与者、会议室、闲忙与时间建议 |
| 待办 | `todo` | 创建 / 列表 / 修改 / 完成待办及评论 |
| 审批 | `oa` | 同意 / 拒绝 / 撤销 / 转交，查待办 / 已发起 / 抄送及表单 |
| 考勤 | `attendance` | 打卡记录、排班、考勤摘要、考勤组规则（只读） |
| DING | `ding` | 发送 / 撤回 DING 消息 |
| 日志 | `report` | 创建 / 提交日志，收发件箱，模版，统计 |
| AI 表格 | `aitable` | Base / 数据表 / 记录 / 字段 / 视图，权限与角色，自动化，图表与仪表盘，导入导出 |
| 文档 | `doc` | 搜索 / 读写文档，块级编辑，评论，权限，媒体，上传 / 下载 |
| 钉盘 | `drive` | 列表 / 搜索 / 下载，文件夹，上传，复制 / 移动 / 重命名，权限 |
| AI 听记 | `minutes` | 听记列表、摘要 / 关键词 / 转写 / 待办、思维导图、发言人、标签 |
| 邮箱 | `mail` | 邮箱、KQL 搜索、读 / 发、草稿、文件夹、模版、联系人 |
| 在线电子表格 | `sheet` | 在线表格：工作表与区域读写、筛选、条件格式、图片、CSV |
| 知识库 | `wiki` | 知识库：空间、成员、节点树、文档与文件 |
| 开发者文档 | `devdoc` | 搜索开放平台文档并排查 API 错误 |
| AI 搜问 | `aisearch` | 企业人员搜索：按姓名 / 部门 / 角色 / 职责 / 上下级 / 手机号 / 工号 |
| 直播 | `live` | 查看我的直播列表 |
| Raw API | `api` | 直接调用任意钉钉 OpenAPI，自动管理应用级 Token |

> 完整命令清单（带描述与使用场景）：[`docs/command-index.md`](./docs/command-index.md)。运行 `dws --help` 查看顶层命令树，或 `dws <service> --help` 查看任一服务的子命令。

> **关于 `chat bot`**：机器人能力（`send-by-bot` / `recall-by-bot` / `add-bot` / `send-by-webhook` / bot 搜索）已合并到对应的 `chat` 子树下（例如 `dws chat message send-by-bot`、`dws chat group members add-bot`），保持 agent 视角下的命令面扁平易发现。不再有独立的顶层 `bot` 产品。

<details>
<summary>即将推出</summary>

- `conference`（视频会议）
- 多 skill 模式（实验中）— 每产品一个独立 skill，位于 `skills/multi/`，通过 `dws skill setup --mode multi` 启用

</details>

## 安全设计

`dws` 从架构层面将安全作为一等公民，而非事后补丁。**凭证不落盘、Token 不出域、权限不越界、操作不脱审** — 每一次 API 调用都必须经过钉钉开放平台的鉴权和审计链路，无例外。

<details>
<summary><strong>开发者安全机制</strong></summary>

| 机制 | 说明 |
|------|------|
| **Token 加密存储** | **PBKDF2（600,000 次迭代 + SHA-256）+ AES-256-GCM** 加密，密钥绑定设备物理 MAC 地址；macOS 集成系统 Keychain、Windows 集成 DPAPI 提供额外保护，跨设备无法解密 |
| **输入安全防护** | 路径遍历防护（符号链接解析 + 工作目录约束）、CRLF 注入拦截、Unicode 视觉欺骗字符过滤，防止 AI Agent 被恶意指令诱导 |
| **域名白名单** | `DWS_TRUSTED_DOMAINS` 默认仅信任 `*.dingtalk.com`，Bearer Token 不会发送到非白名单域 |
| **并发安全** | 双层锁机制（进程内 + 跨进程文件锁）保障 Token 刷新原子性，适配高并发 MCP Server 场景 |
| **数据完整性** | 所有配置写入采用原子操作（temp + fsync + rename），确保进程中断时数据不损坏 |
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

## 参考与文档

- [命令索引](./docs/command-index.md) — 全部运行时命令，带描述与使用场景
- [参考手册](./docs/reference.md) — 环境变量、退出码、输出格式、Shell 补全
- [架构设计](./docs/architecture.md) — 静态端点管道、命令面、Transport 层
- [开放平台应用指令设计](./docs/dev-yulan-command-routing.md) — yulan dev app 应用侧命令、MCP overlay、权限流程与 Agent 路由
- [更新日志](./CHANGELOG.md) — 版本历史与迁移说明

## 贡献指南

参见 [CONTRIBUTING.md](./CONTRIBUTING.md) 了解构建、测试和开发工作流。

## 许可证

Apache-2.0
