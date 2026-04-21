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

## PAT Architecture / 第三方 Agent 对接架构

本节描述 `dws` 与第三方 Agent 宿主之间的 wire 面 —— 即 **PAT（个人授权令牌）** 通道。完整契约见 [docs/pat/README.md](./pat/README.md)。

### 全景 / Overview

```mermaid
flowchart TD
    Host[Third-party Agent Host] -->|spawn subprocess<br/>DINGTALK_DWS_AGENTCODE env (host-owned trigger)<br/>DINGTALK_AGENT env (optional x-dingtalk-agent tag)<br/>DWS_* trace env| CLI[dws CLI]

    subgraph CLIInternal[CLI internal]
        App[internal/app<br/>root command wiring]
        PAT[internal/pat<br/>pat chmod / apply / status / scopes]
        Auth[internal/auth<br/>OAuth / keychain / channel / identity]
        ErrPAT[internal/errors<br/>PATError + cleanPATJSON]
        Transport[internal/transport<br/>JSON-RPC client + trusted domains]
        RT[pkg/runtimetoken<br/>thin wrapper]
    end

    App --> PAT
    PAT -->|pat.grant tool call| Transport
    Auth -->|identity headers<br/>claw-type| Transport
    Transport -->|response JSON| ErrPAT
    ErrPAT -->|exit code 4 + stderr JSON| Host
    RT -.delegates.-> Auth

    Transport -->|HTTPS| GW[DingTalk MCP Gateway]
    GW --> RemoteAgents[Remote MCP servers]

    Host -->|exit 0| HostOK[render business result]
    Host -->|exit 2<br/>auth failure| HostReLogin[trigger re-login]
    Host -->|exit 4<br/>PAT JSON| HostPAT[render approval UI<br/>+ spawn dws pat chmod<br/>+ re-run]
    Host -->|exit 5<br/>internal error| HostFatal[log full stderr]
```

### 模块边界 / Module boundaries

| 模块 | 路径 | 职责 | 对外契约 |
|---|---|---|---|
| **CLI app** | `internal/app/` | Cobra 根命令装配、panic recovery、身份 header 导出 | `MCPIdentityHeaders()` 复用身份头 |
| **PAT orchestrator** | `internal/pat/` | `dws pat` 命令组；chmod / apply / status / scopes **全部已注册**（`pat.go: RegisterCommands`）| 所有 PAT 类子命令均为 thin wrapper：参数校验 → MCP tool call → classifier 分类。`chmod` / `apply` 使用 `callPATToolWithLegacyFallback` 做英文工具名→legacy 别名的灰度回落；`status` / `scopes` 直接调用 `caller.CallTool`，无 legacy fallback |
| **Auth** | `internal/auth/` | OAuth device flow / code 交换 / refresh、keychain 存储、`channel.go` 暴露 `HostOwnsPATFlow()` + `DWS_CHANNEL` 转发、`identity.go` 注入 `x-dws-agent-id` 等 | 通过 `edition.Hook` 让 embedded 宿主覆盖存储策略；`claw-type` 由 `pkg/edition/default.go` 硬编码为 `openClaw` |
| **Errors** | `internal/errors/` | `PATError` 定义、`ExitCodePermission=4`、`cleanPATJSON` 归一化、`IsPATNoPermissionCode` 分类 | `hostControl.clawType` 统一由 `cleanPATJSON` 注入 |
| **Transport** | `internal/transport/` | HTTPS JSON-RPC 客户端 + `DWS_TRUSTED_DOMAINS` 域名白名单 + 指数退避 | 下游 PAT 共用 |
| **Runtime token** | `pkg/runtimetoken/` | 绕过 MCP runner 的场景下解析 access token 的薄封装 | `ResolveAccessToken(ctx, configDir, explicit)` |

### 关键不变式 / Invariants

1. **Exit code 4 只能是 PAT**：`internal/errors/pat.go` 的 `PATError.ExitCode()` 返回 4；其他失败类型禁止复用该码。
2. **hostControl.clawType 单值来源**：`clawType` 取值必须由 `apperrors.HostControlBlock()`（内部经 `hostControlProvider` 委派到 `pkg/edition` 的 `MergeHeaders` 钩子，开源构建硬编码为 `edition.DefaultOSSClawType` / `openClaw`）统一读取；classifier 与 retry 两条路径可各自将 block 写入 `data.hostControl`，但值必须字节对齐。
3. **身份不通过 env 跨进程**：CLI 不读任何身份相关 env；身份由 `dws auth login/exchange` 写入本地凭证文件。
4. **PAT tool name 英文 ASCII**：`dws pat chmod` 向服务端调用的 tool 名必须是 ASCII（例 `pat.grant`），禁止在 wire 协议位置使用非 ASCII 字符。
5. **`DWS_CHANNEL` 不是宿主控制位**：它只携带上游 `channelCode` 元数据；host-owned PAT 的**唯一**触发信号是 `DINGTALK_DWS_AGENTCODE`。`DINGTALK_AGENT` 仅在非空时原样注入 `x-dingtalk-agent` 请求头，不参与 `claw-type` 派生、也不参与 host-owned 决策；`claw-type` 在开源构建中硬编码为 `openClaw`（见 `pkg/edition/default.go`）。详见 [docs/pat/contract.md §7](./pat/contract.md#7-host-owned-pat-开关--host-owned-pat-trigger)。

### 数据流 / Data flow

两条主流：

1. **业务命令流（带 PAT）**：Host → spawn CLI → `internal/app` → `internal/transport` → Gateway → 返回权限错误 → `internal/errors.cleanPATJSON` 归一化 → stderr JSON + `exit=4` → Host UI
2. **PAT chmod 流**：Host 渲染 UI → spawn `dws pat chmod ...` → `internal/pat` → `internal/transport` → Gateway 签发 scope → `exit=0` → Host 重跑原命令

### 与参考宿主实现的关系 / Relation to reference host

参考宿主实现（外部、非本仓）负责：登录 UI、token 生命周期反向校验、PAT 授权 UI 渲染、高敏异步审批通道、trace id 下发、跨进程 / 跨组织身份切换。

本仓只承诺开源 CLI 侧：wire contract、exit code、stderr JSON 归一化、`dws pat` 命令面。任何非开源宿主的实现细节**不**出现在本仓代码与文档中。

### 演进路径 / Roadmap

Shipped（已完成，进入 Stable 表面）：

- ✅ **PAT 子命令齐全**：`dws pat chmod / apply / status / scopes` 四条子命令全部注册并可用（`internal/pat/{chmod,apply,status,scopes}.go` + `pat.go:RegisterCommands`）。

In progress / Planned：

- **P1**：`dws pat status` / `dws pat scopes` 的 legacy tool-name fallback —— 当前仅 `chmod` / `apply` 支持回落到 legacy 别名；灰度期若有需要可补齐（目前评估为无必要，因这两条命令上线与服务端新工具名同步）。
- **P1**：`dws pat apply` stdout JSON 补 `clientGenerated` flag —— 现状是客户端 UUID v4 回落对宿主不可见，属潜在契约模糊点；见 [docs/pat/contract.md §2.5](./pat/contract.md#25-pat-apply-stdout-contract)。
