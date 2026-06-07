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

**核心原则：每个渠道把机器人接到「该渠道对应的那个 agent」**，机器人收到的消息只会进当前这个宿主、由它处理回复——不会串到别的 agent。dws 按运行时环境信号自动识别当前宿主，auto 模式无需手动指定 `--channel`（如在 WorkBuddy 里跑就自动走 workbuddy 渠道、接 WorkBuddy 自己的会话助理）。

| 渠道 | 识别信号 | 机器人接到谁 | 转发方式 |
|------|----------|--------------|----------|
| `claudecode` | `CLAUDECODE` | 本地 Claude Code | exec CLI（一次性）：`claude -p <text>` |
| `qoder` | `QODER_CLI` | 本地 Qoder | exec CLI（一次性）：`qodercli -p` |
| `qoderwork` | `QODERCLI_INTEGRATION_MODE=qoder_work` | **当前 QoderWork 会话助理** | HTTP → bridge `:18791` |
| `workbuddy` | `WORKBUDDY_CONFIG_DIR` / `WORKBUDDY_APP_NAME` | **当前 WorkBuddy 会话助理** | HTTP → bridge `:18790` |
| `openclaw` | `DINGTALK_AGENT=DING_DWS_CLAW` | 本地 OpenClaw | 外部连接器 |
| `hermes` | `HERMES_AGENT` | Hermes | 官方 channel |

> 两类转发：**会话型**（`workbuddy` / `qoderwork`，你正在对话的桌面助理）经 agent-session bridge 接到**当前会话**——不是另起一个一次性 CLI 实例；**一次性型**（`qoder` / `claudecode`）每条消息起一个新 CLI 处理。想接 OpenClaw 就 `--channel openclaw`；**别用 workbuddy/qoderwork 渠道指向别家网关**，否则"在 WorkBuddy 里建联"会把机器人接到别的 agent，渠道语义就错了。

## 建联流程

```
dws connect
  ├── ① 建号 (connect bot create)
  │      MCP 异步两步：submit_robot_create_task → 轮询 query_robot_create_result
  │      返回 agentId / robotCode / clientId / clientSecret（secret 仅返回一次）
  │
  ├── ② Stream 建联 (connect start)
  │      Go 原生进程内 WebSocket，订阅 TOPIC_ROBOT
  │      按渠道选 forwarder：exec CLI 或 HTTP gateway
  │
  └── ③ 审批 + 验证
         qyapi_robot_sendmsg scope 审批（建号 ≠ 可用）
         + 真实发消息探测 + chat bot search 交叉验证
```

## 会话型渠道：workbuddy / qoderwork（经 agent-session bridge）

WorkBuddy、QoderWork 都是**你正在对话的桌面助理会话**，自身不暴露 OpenAI 兼容端点。要把机器人接到
**当前会话**（而不是另起一个一次性 `qodercli -p` / `claude -p` 实例），必须经
`scripts/bridge/agent-session-bridge.py` 把消息中转进当前会话。forwarder 通过 HTTP 把消息 POST 到
OpenAI 兼容 `/v1/chat/completions`，地址/令牌/模型由环境变量配置、代码不写死凭证。

### 环境变量（每个渠道一组，互不影响）

| 渠道 | 网关变量 | 默认 | 模型变量 | 默认 |
|------|----------|------|----------|------|
| workbuddy | `WB_GATEWAY` | `http://localhost:18790` | `WB_MODEL` | `workbuddy-assistant` |
| qoderwork | `QW_GATEWAY` | `http://localhost:18791` | `QW_MODEL` | `qoderwork-assistant` |

各自还有 `WB_AUTH_TOKEN` / `QW_AUTH_TOKEN`（bridge 模式一般不需要）。

> ⚠️ 别把网关指向别家 agent（如 OpenClaw `:18789`）——那样机器人会接到别的 agent 而非当前会话，渠道语义就错了。

### 起法（每个渠道一份 bridge，端口 + 目录各不相同避免打架）

```bash
# WorkBuddy（在 WorkBuddy 里 auto 即识别 workbuddy；:18790 已是默认）
python3 scripts/bridge/agent-session-bridge.py
dws connect start --channel workbuddy --client-id <id> --client-secret <secret>

# QoderWork（在 QoderWork 里 auto 即识别 qoderwork；用 18791 + 独立队列目录）
BRIDGE_PORT=18791 BRIDGE_DIR=~/.dingtalk-bridge-qoderwork python3 scripts/bridge/agent-session-bridge.py
dws connect start --channel qoderwork --client-id <id> --client-secret <secret>
```

```
钉钉消息 → dws Stream → Bridge → 文件队列 → 当前会话助理 → 回复 → dws → 钉钉
```

Bridge 工作方式：
- 收到消息后写入 `<BRIDGE_DIR>/queue/<msg_id>.json`
- 阻塞等待当前会话助理把回复写入 `<BRIDGE_DIR>/responses/<msg_id>.json`
- 解阻塞后返回给 dws，通过 SessionWebhook 发回钉钉

Bridge 配置：

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `BRIDGE_PORT` | `18790` | 监听端口 |
| `BRIDGE_DIR` | `~/.dingtalk-bridge` | 队列/回复/日志根目录（跑多份时各设一个） |
| `BRIDGE_TIMEOUT_SEC` | `120` | 单条消息最长等待秒数 |

端点：
- `GET /health` — 健康检查
- `GET /queue` — 排队消息列表
- 日志：`~/.dingtalk-bridge/log.txt`
