# Architecture

`dws` is a Go CLI that turns DingTalk MCP metadata into a command-line surface for both humans and AI agents.

## High-Level Flow

1. `internal/market` fetches the registry and server metadata.
2. `internal/discovery` resolves runtime server capabilities and caches results.
3. `internal/ir` normalizes discovery output into one canonical tool catalog.
4. `internal/cli` and `internal/app` mount that catalog into the public Cobra command tree.
5. `internal/transport` executes MCP JSON-RPC calls and `internal/output` formats responses.

## Repository Structure

- `cmd`: CLI entrypoint
- `internal/app`: root command wiring and static utility commands
- `internal/discovery`, `internal/market`, `internal/transport`: runtime discovery and execution
- `internal/ir`: canonical intermediate representation for discovered tools
- `internal/generator`: docs, schema, and skill generation pipeline
- `internal/compat`, `internal/helpers`: legacy-compatible overlays and helper commands
- `skills/`: bundled agent skills source and generated skill docs
- `test/`: CLI, compatibility, integration, contract, and script tests

## Public Repository Contract

This repository ships source, docs, tests, packaging templates, and install scripts. Generated or release-only artifacts are produced by repository scripts and are not required to exist in a clean checkout unless explicitly committed as part of a release workflow.

## PAT + A2A Architecture / 第三方 Agent 对接架构

本节描述 `dws` 与第三方 Agent 宿主之间的 wire 面 —— 即 **PAT（个人授权令牌）** 与 **A2A（Agent-to-Agent 协议）** 两条通道。完整契约见 [docs/pat/README.md](./pat/README.md)。

### 全景 / Overview

```mermaid
flowchart TD
    Host[Third-party Agent Host] -->|spawn subprocess<br/>DINGTALK_DWS_AGENTCODE env (host-owned trigger)<br/>DINGTALK_AGENT env (optional x-dingtalk-agent tag)<br/>DWS_* trace env| CLI[dws CLI]

    subgraph CLIInternal[CLI internal]
        App[internal/app<br/>root command wiring]
        PAT[internal/pat<br/>pat chmod / apply / status / scopes]
        A2A[internal/a2a<br/>token + headers + clawType + plugin auth]
        Auth[internal/auth<br/>OAuth / keychain / channel / identity]
        ErrPAT[internal/errors<br/>PATError + cleanPATJSON]
        Transport[internal/transport<br/>JSON-RPC client + trusted domains]
        RT[pkg/runtimetoken<br/>thin wrapper]
    end

    App --> PAT
    App --> A2A
    PAT -->|pat.grant tool call| Transport
    A2A -->|A2A v1.0 HTTP + SSE| Transport
    Auth -->|identity headers<br/>claw-type<br/>x-user-access-token| A2A
    Auth -->|identity headers<br/>claw-type| Transport
    Transport -->|response JSON| ErrPAT
    ErrPAT -->|exit code 4 + stderr JSON| Host
    A2A -.uses.-> RT
    RT -.delegates.-> Auth

    Transport -->|HTTPS| GW[DingTalk MCP / A2A Gateway]
    GW --> RemoteAgents[Remote Agents<br/>and MCP servers]

    Host -->|exit 0| HostOK[render business result]
    Host -->|exit 2<br/>auth failure| HostReLogin[trigger re-login]
    Host -->|exit 4<br/>PAT JSON| HostPAT[render approval UI<br/>+ spawn dws pat chmod<br/>+ re-run]
    Host -->|exit 5<br/>internal error| HostFatal[log full stderr]
```

### 模块边界 / Module boundaries

| 模块 | 路径 | 职责 | 对外契约 |
|---|---|---|---|
| **CLI app** | `internal/app/` | Cobra 根命令装配、panic recovery、身份 header 导出 | `MCPIdentityHeaders()` 给 A2A 客户端复用 |
| **PAT orchestrator** | `internal/pat/` | `dws pat` 命令组；chmod / apply / status / scopes **全部已注册**（`pat.go: RegisterCommands`）| 所有 PAT 类子命令均为 thin wrapper：参数校验 → MCP tool call → classifier 分类。`chmod` / `apply` 使用 `callPATToolWithLegacyFallback` 做英文工具名→legacy 别名的灰度回落；`status` / `scopes` 直接调用 `caller.CallTool`，无 legacy fallback |
| **A2A client (SDK helpers only)** | `internal/a2a/` | A2A v1.0 Go SDK helpers —— access token 解析、身份头、clawType / channel 规范化、三方 MCP server 独立 bearer token 注册表 | `a2a/*.go` 汇总 `headers`/`channel`/`registry`/`token`/`a2a`；**已 shipped** 的仅为 SDK helper 包（CLI 进程内复用 + 第三方宿主 import）。**`dws a2a` CLI 子命令树仍为 Planned**，无 `cobra.Command` 注册；见 `dws-a2a-cli` milestone |
| **Auth** | `internal/auth/` | OAuth device flow / code 交换 / refresh、keychain 存储、`channel.go` 暴露 `HostOwnsPATFlow()` + `DWS_CHANNEL` 转发、`identity.go` 注入 `x-dws-agent-id` 等 | 通过 `edition.Hook` 让 embedded 宿主覆盖存储策略；`claw-type` 由 `pkg/edition/default.go` 硬编码为 `openClaw` |
| **Errors** | `internal/errors/` | `PATError` 定义、`ExitCodePermission=4`、`cleanPATJSON` 归一化、`IsPATNoPermissionCode` 分类 | `hostControl.clawType` 统一由 `cleanPATJSON` 注入 |
| **Transport** | `internal/transport/` | HTTPS JSON-RPC 客户端 + `DWS_TRUSTED_DOMAINS` 域名白名单 + 指数退避 | 下游 PAT / A2A 共用 |
| **Runtime token** | `pkg/runtimetoken/` | A2A gateway 等绕过 MCP runner 的场景下解析 access token 的薄封装 | `ResolveAccessToken(ctx, configDir, explicit)` |

### 关键不变式 / Invariants

1. **Exit code 4 只能是 PAT**：`internal/errors/pat.go` 的 `PATError.ExitCode()` 返回 4；其他失败类型禁止复用该码。<!-- evidence: internal/errors/pat.go:23-24 + contract.md §1 -->
2. **hostControl.clawType 单值来源**：`clawType` 取值必须由 `apperrors.HostControlBlock()`（内部经 `hostControlProvider` 委派到 `pkg/edition` 的 `MergeHeaders` 钩子，开源构建硬编码为 `edition.DefaultOSSClawType` / `openClaw`）统一读取；classifier 与 retry 两条路径可各自将 block 写入 `data.hostControl`，但值必须字节对齐。<!-- evidence: internal/errors/pat.go HostControlBlock + internal/app/pat_hostcontrol_wire.go init + pkg/edition/default.go MergeHeaders -->
3. **身份不通过 env 跨进程**：CLI 不读任何身份相关 env；身份由 `dws auth login/exchange` 写入本地凭证文件。
4. **A2A 分层**：`internal/a2a/` 的 SDK helpers **已 shipped**，CLI 进程内复用 + 第三方宿主可直接 import；`dws a2a` CLI 子命令树**仍为 Planned**，无 `cobra.Command` 注册。未来 CLI 层入口引入后仍按"命令路径以 `dws a2a` 开头才构造完整客户端"的按需装配原则执行。<!-- evidence: internal/a2a/ 目录 + grep RegisterCommands 无匹配 -->
5. **PAT tool name 英文 ASCII**：`dws pat chmod` 向服务端调用的 tool 名必须是 ASCII（例 `pat.grant`），禁止在 wire 协议位置使用非 ASCII 字符。<!-- evidence: internal/pat/chmod.go patGrantToolName + callPATToolWithLegacyFallback -->
6. **`DWS_CHANNEL` 不是宿主控制位**：它只携带上游 `channelCode` 元数据；host-owned PAT 的**唯一**触发信号是 `DINGTALK_DWS_AGENTCODE`。`DINGTALK_AGENT` 仅在非空时原样注入 `x-dingtalk-agent` 请求头，不参与 `claw-type` 派生、也不参与 host-owned 决策；`claw-type` 在开源构建中硬编码为 `openClaw`（见 `pkg/edition/default.go`）。详见 [docs/pat/contract.md §7](./pat/contract.md#7-host-owned-pat-开关--host-owned-pat-trigger)。

### 数据流 / Data flow

三条主流：

1. **业务命令流（带 PAT）**：Host → spawn CLI → `internal/app` → `internal/transport` → Gateway → 返回权限错误 → `internal/errors.cleanPATJSON` 归一化 → stderr JSON + `exit=4` → Host UI
2. **PAT chmod 流**：Host 渲染 UI → spawn `dws pat chmod ...` → `internal/pat` → `internal/transport` → Gateway 签发 scope → `exit=0` → Host 重跑原命令
3. **A2A 流（SDK helpers shipped，CLI 命令树 Planned）**：宿主可直接 import `internal/a2a/` 辅助包 → 通过 `pkg/runtimetoken.ResolveAccessToken`（薄 wrapper，委托给 `internal/auth/`）取 access token → 由宿主自身进程发起 HTTPS JSON-RPC (+SSE) → Gateway 转发到 remote agent。当 `dws a2a send` 等 CLI 命令未来落地后，上述步骤会以子进程方式由 CLI 端承担；当前开源 CLI 无 `dws a2a ...` 入口。

### 与参考宿主实现的关系 / Relation to reference host

参考宿主实现（外部、非本仓）负责：登录 UI、token 生命周期反向校验、PAT 授权 UI 渲染、高敏异步审批通道、trace id 下发、跨进程 / 跨组织身份切换。

本仓只承诺开源 CLI 侧：wire contract、exit code、stderr JSON 归一化、A2A 客户端、`dws pat chmod` 命令面。任何非开源宿主的实现细节**不**出现在本仓代码与文档中。

### 演进路径 / Roadmap

Shipped（已完成，进入 Stable 表面）：

- ✅ **PAT 子命令齐全**：`dws pat chmod / apply / status / scopes` 四条子命令全部注册并可用（`internal/pat/{chmod,apply,status,scopes}.go` + `pat.go:RegisterCommands`）。
- ✅ **A2A SDK helpers 抽取**：原先散落在 `pkg/runtimetoken` / `internal/app/identity_export.go` / `internal/auth/channel.go` / `internal/app/auth_registry.go` 的 A2A 相关 helper 已收拢到 `internal/a2a/`（`a2a.go` / `channel.go` / `headers.go` / `registry.go` / `token.go`），对外作为 SDK 可复用。
- ✅ **Secure store 合并**：`internal/auth/secure_store.go` 与 `internal/security/storage.go` 的重复 AES-GCM+MAC 实现已统一。

In progress / Planned：

- **P0**：`dws a2a` **CLI 子命令树**（`agents list/info`、`send [--stream]`）—— SDK helpers 已就绪，CLI 层入口尚未注册；跟踪 milestone `dws-a2a-cli`。
- **P1**：`AgentCard.supportedInterfaces[*].protocolVersion` 协商替换硬编码 `A2A-Version: 1.0`（契约已标注，待实现协商逻辑）。
- **P1**：`dws pat status` / `dws pat scopes` 的 legacy tool-name fallback —— 当前仅 `chmod` / `apply` 支持回落到 legacy 别名；灰度期若有需要可补齐（目前评估为无必要，因这两条命令上线与服务端新工具名同步）。
- **P1**：`dws pat apply` stdout JSON 补 `clientGenerated` flag —— 现状是客户端 UUID v4 回落对宿主不可见，属潜在契约模糊点；见 [docs/pat/contract.md §2.5](./pat/contract.md#25-pat-apply-stdout-contract)。

<!-- evidence: w1-lane1 §8 + w1-lane3 §3/§7 -->

