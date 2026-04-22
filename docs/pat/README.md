# PAT 第三方 Agent 对接文档包

> 面向第三方 Agent / 业务宿主 / 自研平台的 `dws` CLI 对接规范。
> 本目录内文档构成 wire contract；外部实现应以本目录为唯一事实来源。

---

## 1. 读者地图

| 读者 | 主要入口 | 辅助阅读 |
|---|---|---|
| 第三方 Agent 工程（Rust / Go / Node / Python 等宿主） | [host-integration.md](./host-integration.md) | [contract.md](./contract.md) |
| 业务调用方 / Skill 作者（直接在 shell / 子进程里用 `dws`） | 下方 Quick Start | [contract.md](./contract.md) |
| 运维 / SRE（监控、告警、回归） | [contract.md](./contract.md) §6 三档表 | [../reference.md](../reference.md) |
| CLI 贡献者（PR / 实现 lane） | [contract.md](./contract.md) + [../architecture.md](../architecture.md) 的 《PAT Architecture》章节 | 全部 |

---

## 2. 什么是 PAT，为什么需要它

**PAT (Personal Authorization Token)** 是 DingTalk Workspace 在 OAuth 之上做的一层**细粒度、按场景下发**的授权。OAuth 让你拿到「能访问平台」的令牌；PAT 让你回答「这次命令能不能读写某个 `资源:动作`（例如 `aitable.record:read`）」。

与 OAuth 的关系：

- **OAuth 决定身份**：`dws auth login` → 本地凭证，用于所有后续请求。
- **PAT 决定行为**：同一个身份，在不同 agent / 场景下，仍可能被服务端以 `PAT_*_NO_PERMISSION` 打回；需要先 `dws pat chmod ...` 拿到 scope 才能执行。

PAT 的签发与吊销在**服务端**；CLI 端只做两件事：①把 chmod 申请透传给服务端；②捕获服务端打回的权限不足，用 **exit code 4 + stderr JSON** 让宿主接管 UI。

---

## 3. 最短可运行示例（Quick Start，4 步）

下列示例假定你已完成 `brew install dingtalk-workspace-cli` + `dws auth login` 的一次性准备。

```bash
# 1) 声明当前 shell / subprocess 的业务 Agent 身份
#    DINGTALK_DWS_AGENTCODE = 业务 Agent 的唯一 code（例 agt-xxxx）
#    每次宿主 spawn 子进程时作为 per-shell 临时 env 注入；
#    同一机器每次 dws 指令各开一个 shell，彼此独立。
export DINGTALK_DWS_AGENTCODE=agt-xxxx

# 2) 声明当前 trace 上下文（env-first，不走 argv）
export DINGTALK_SESSION_ID=conv-001     # 注入 x-dingtalk-session-id 头
export DINGTALK_TRACE_ID=req-001        # 注入 x-dingtalk-trace-id 头
export DINGTALK_MESSAGE_ID=msg-001      # 注入 x-dingtalk-message-id 头
export DWS_SESSION_ID=conv-001          # 可选：仅当你用 `pat chmod --grant-type session` 回退取 session id

# 3) 调用业务命令
dws aitable record list --sheet-id <id>
#    ↑ 若被 PAT 拦截，进程 exit_code=4，stderr 是单行结构化 JSON。
#      宿主据此渲染授权 UI，用户选定后再调：

# 4) 用户确认后，按卡片选项回调 chmod 进行授权
dws pat chmod aitable.record:read \
    --agentCode agt-xxxx \
    --grant-type session \
    --session-id conv-001

# 5) [可选] Agent 厂商选择指令重放
dws aitable record list --sheet-id <id>
```

**四个关键约定**（对齐 contract.md §2）：

1. **`exit_code == 4` = PAT 权限不足**（唯一触发授权卡片的信号）；`0` = 成功，走 stdout；其他非 0 值一律按通用失败处理，不进入授权卡流程。
2. **stderr 是单行结构化 JSON**（不是日志、不是 pretty-print），可直接 `json.Unmarshal`；按 [contract.md](./contract.md) §2 解析。
3. **身份绑定在 token 文件里**，CLI **不读 env / argv 里的任何身份参数**；宿主通过 env 把 agent / 会话 / trace 上下文透传给 CLI。
4. **env-first**：身份与上下文都走 env 注入；CLI 不从 argv 读身份字段。

**环境变量注入对照**（宿主 spawn 每次都显式注入）：

| Env | 语义 | 备注 |
|---|---|---|
| `DINGTALK_DWS_AGENTCODE` | 业务 Agent 在组织侧的唯一 code | **唯一识别 env**；亦可通过 `--agentCode <id>` flag 单次覆盖（flag > env）；`DWS_AGENTCODE` / `DINGTALK_AGENTCODE` / `REWIND_AGENTCODE` **不再识别** |
| `DINGTALK_SESSION_ID` / `DINGTALK_TRACE_ID` / `DINGTALK_MESSAGE_ID` | trace / session / message id | 透传为 `x-dingtalk-session-id` / `x-dingtalk-trace-id` / `x-dingtalk-message-id` 请求头 |
| `DWS_SESSION_ID` | `dws pat chmod --grant-type session` 的 `--session-id` 回退 | 仅 PAT 子命令消费；**不**注入 trace 头。兼容别名 `REWIND_SESSION_ID` |
---

## 4. 文档包索引

| 文件 | 内容 | 一句话摘要 |
|---|---|---|
| [contract.md](./contract.md) | wire contract（中英双语对照） | exit code、stderr JSON schema、PAT grant-type、scope 字符串、hostControl、身份头、环境变量、风险等级、错误码三档表 |
| [host-integration.md](./host-integration.md) | 宿主端集成指南 | 如何解析 exit_code=4、如何渲染授权 UI、中敏 vs 高敏分支、参考时序图、negative space |
| [../architecture.md](../architecture.md) | 架构（含《PAT Architecture》章节） | `internal/pat/`、`internal/auth/`、`pkg/runtimetoken` 的职责边界 |
| [../reference.md](../reference.md) | CLI 参考（含 PAT 章节） | `dws pat` 子命令、PAT 相关环境变量、exit code 表 |

---

## 5. 术语表

| 术语 | 含义 |
|---|---|
| **CLI** | 本仓编译出的 `dws` 可执行文件 |
| **Host / 宿主** | 以子进程方式调用 `dws` 的第三方 Agent 桌面 / 服务端程序 |
| **Scope** | 格式 `<product>.<entity>:<permission>`，例如 `aitable.record:read` |
| **Agent Code** | 业务 Agent 的稳定标识；由宿主决定如何生成（例：`md5(openId+corpId+deviceId)`）。CLI 只做透传。env 注入唯一路径：`DINGTALK_DWS_AGENTCODE`（`DWS_AGENTCODE` / `DINGTALK_AGENTCODE` / `REWIND_AGENTCODE` 不再识别）；亦可用 `--agentCode` flag 单次覆盖 |
| **authRequestId** | 服务端为高敏授权签发的相关性 id；宿主用它做异步回执绑定 |
| **clawType** | 请求头 `claw-type` 的取值。**开源构建硬编码为 `openClaw`**（由 `pkg/edition/default.go` 的 `MergeHeaders` 钩子注入），与 `DINGTALK_AGENT` 或任何宿主环境变量**无关**。**仅**作为上行服务端路由标签；**不**决定 CLI 是否进入 host-owned PAT 模式（host-owned 的唯一开关是 `DINGTALK_DWS_AGENTCODE`，见 [contract.md §7](./contract.md#7-host-owned-pat-开关--host-owned-pat-trigger)） |
| **Host-owned PAT** | CLI 的一种 PAT 工作模式：命中 PAT 时 `exit=4` + 单行 stderr JSON，不拉浏览器、不轮询、不重试，全部交给宿主渲染自定义授权卡。**唯一触发**：进程启动时 `DINGTALK_DWS_AGENTCODE` 非空。`DINGTALK_AGENT` / `claw-type` / `DWS_CHANNEL` 均不参与该决策 |

---

## 6. 稳定性承诺

- **稳定 CLI 表面**：`dws pat chmod` 子命令及其 flag 名、exit code `0/2/4/5`、stderr JSON 顶层字段（`success / code / error_code / data.requiredScopes / data.grantOptions / data.authRequestId / data.hostControl`）。
  - 本 PR（A-core）仅包含 `dws pat chmod`；`apply` / `status` / `scopes` 将在后续扩展 PR（A-ext）中上线，届时补齐稳定性承诺。
- **可演进**：`data.*` 下的非核心字段。
- **未承诺**：具体 MCP server 域名 / 端点、tool 内部名称、日志格式。

---

## 7. 反馈

文档问题请在开源仓开 Issue 并附：CLI 版本（`dws version`）、`DINGTALK_DWS_AGENTCODE` 是否设置、`DINGTALK_AGENT` 值、原始 stderr JSON（可脱敏）、宿主侧关键日志切片。
