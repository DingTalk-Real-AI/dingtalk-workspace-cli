# dws connect — 渠道感知建联

`dws connect` 是独立的建联命令体系，一句话完成「探测渠道 → 按需建号 → 按渠道建联」。
不在 `chat` 命令树下，不与消息收发混在一起。

## 命令

```bash
# 一键建号 + 建联（机器人不存在时）
dws connect --channel <ch> --app-name <名> --robot-name <名> --desc <描述>

# 只建号
dws connect bot create --app-name <名> --robot-name <名> --desc <描述>

# 已有机器人，直接起 Stream 建联
dws connect start --channel <ch> --client-id <id> --client-secret <secret>
```

## 渠道路由

**核心原则：每个渠道把机器人转发到「该渠道对应的本地 agent CLI 产品」**，无头一次性（每条消息起一个新实例），**永久本地常驻、可 7×24 无人值守**，不依赖任何交互会话窗口。dws 按运行时环境信号自动识别当前宿主，auto 模式无需手动 `--channel`（如在 Claude Code 里跑就自动 claudecode）。

### 支持的 agent（命令行型，可自动安装）

| 渠道 | binary | 无头调用 | 安装 | 自动装 |
|------|--------|----------|------|--------|
| `claudecode` | `claude` | `claude -p` | `npm i -g @anthropic-ai/claude-code` | ✅ npm |
| `codex` | `codex` | `codex exec` | `npm i -g @openai/codex` | ✅ npm |
| `gemini` | `gemini` | `gemini -p` | `npm i -g @google/gemini-cli` | ✅ npm |
| `opencode` | `opencode` | `opencode run` | `npm i -g opencode-ai` | ✅ npm |
| `amp` | `amp` | `amp -x` | `npm i -g @sourcegraph/amp` | ✅ npm |
| `crush` | `crush` | `crush run` | `npm i -g @charmland/crush` | ✅ npm |
| `aider` | `aider` | `aider --message` | `pipx install aider-chat` | ✅ pipx |
| `cursor` | `cursor-agent` | `cursor-agent -p` | `curl https://cursor.com/install \| bash` | ⚠️ 提示（curl\|bash 不静默跑） |
| `goose` | `goose` | `goose run -t` | 官方脚本 | ⚠️ 提示 |

### 桌面 App 自带 CLI（装 App，CLI 随 App；App 装了就自动接）

| 渠道 | binary | app 自带位置(glob) | 复用登录 |
|------|--------|-------------------|---------|
| `qoder` | `qodercli` | `Qoder.app/.../bin/*/qodercli` | — |
| `qoderwork` | `qodercli` | `QoderWork.app/.../bin/qodercli` | — |
| `codebuddy` / `workbuddy` | `codebuddy` | `WorkBuddy.app/.../cli/bin/codebuddy` | 设 `CODEBUDDY_CONFIG_DIR=~/.workbuddy` 复用 WorkBuddy 登录，**同账号、不用单独登录** |

> `workbuddy` 和 `codebuddy` 都转发到 WorkBuddy 的 CLI 产品（codebuddy -p），同账号。

### 外部 / 官方

| 渠道 | 识别信号 | 方式 |
|------|----------|------|
| `openclaw` | `DINGTALK_AGENT=DING_DWS_CLAW` | 外部连接器 dingtalk-openclaw-connector |
| `hermes` | `HERMES_AGENT` | 钉钉官方 channel |

### 依赖解析 + 自动安装

exec 型渠道**不写死路径**，按 `DWS_AGENT_CMD 覆盖 > PATH > app 自带 CLI（glob，跨架构/版本）` 定位：
- 找到 → 直接用。
- **没找到 + 是包管理器装的（npm/pipx）→ 建联时自动安装**，装完再用（`DWS_CONNECT_NO_INSTALL=1` 可关）。
- **没找到 + 是 curl\|bash 远程脚本 / 桌面 App → 不静默装**，报清楚"请先装 X：<链接/命令>"（建联时报，不是收到消息才炸）。
- 确切 headless 参数若某 agent 版本不同，用 `DWS_AGENT_CMD="<cmd>"` 覆盖。

## 建联流程

```
dws connect
  ├── ① 建号 (connect bot create)
  │      MCP 异步两步：submit_robot_create_task → 轮询 query_robot_create_result
  │      返回 agentId / robotCode / clientId / clientSecret（secret 仅返回一次）
  │
  ├── ② Stream 建联 (connect start)
  │      Go 原生进程内 WebSocket，订阅 TOPIC_ROBOT
  │      按渠道解析/安装本地 agent CLI，收到消息起一个无头实例处理
  │      ack-first + MsgId 去重；回复经 sessionWebhook 发回钉钉
  │
  └── ③ 审批 + 验证
         qyapi_robot_sendmsg scope 审批（建号 ≠ 可用）
         + 真实发消息探测 + chat bot search 交叉验证
```

> 加新 agent = 在 `internal/helpers/connect_stream.go` 的 `agentSpecs` 加一行（binary / 无头 argv / 安装命令 / 复用登录 env），其余自动生效。
