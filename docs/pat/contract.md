# PAT + A2A Wire Contract

> 本文档是 `dws` CLI 与第三方宿主之间的 **wire contract**（线上契约）。
> 所有外部可见字符串、退出码、字段名、枚举值均以本文档为准。
> 双语并列表示 CLI ↔ 宿主必须以**完全一致**的常量跨进程 / 跨语言识别。

<!-- evidence: docs/_research/w1-lane1-wukong-pat-a2a.md, w1-lane2-rewind-pat-consumer.md, w1-lane3-oss-pat-current-gap.md -->

---

## 0. 契约等级 / Contract tiers

| 等级 Tier | 含义 Meaning | 变更策略 Change policy |
|---|---|---|
| **Frozen** | 进入本字段后，除非 major 版本升级否则不改 | 需 RFC + 预告 ≥ 2 个 minor 版本 |
| **Stable** | 稳定表面，允许兼容扩展 | 新增字段允许；语义变化需 minor 版本升级 |
| **Evolving** | 正在演进，宿主必须容忍缺失 / 变化 | 可随 minor 版本变化 |

---

## 1. Exit Code 契约 / Exit code contract

CLI 仅对外承诺下列 exit code。其余任何进程退出都视为异常。

CLI exposes the following exit codes. Anything else is undefined behavior.

| Code | Tier | Semantics (EN) | 语义（中文） |
|---:|---|---|---|
| **0** | Frozen | Success. stdout is the primary channel; stderr MAY contain diagnostic lines | 成功。stdout 是主通道；stderr 可能有诊断日志 |
| **2** | Frozen | Authentication / authorization failure at the **identity** layer (OAuth token missing, expired, revoked, org-unauthorized). Host MUST re-run `dws auth login` or host-managed re-exchange before retry | 身份层认证 / 授权失败（OAuth token 缺失、过期、被撤销、组织未授权）。宿主必须重走登录或等价再交换流程后才能重试 |
| **4** | Frozen | **PAT permission insufficient**. stderr is a single-line JSON payload conforming to §2. Host MUST parse stderr and decide between rendering approval UI or aborting. This code is **reserved exclusively for PAT**; do not overload it | **PAT 权限不足**。stderr 是符合 §2 的单行 JSON。宿主必须解析 stderr，决定渲染授权 UI 或终止。本 code 仅用于 PAT，禁止复用 |
| **5** | Frozen | Unexpected internal error (panic, unrecoverable IO, programmer bug). Host SHOULD log full stderr and surface a generic failure to the user. Not automatically retryable | 未预期内部错误（panic / 不可恢复 IO / bug）。宿主应记录完整 stderr，对用户展示通用错误。不自动重试 |
| **6** | Stable | Discovery / catalog failure (market/registry unreachable, tool catalog decode failed, endpoint resolution broke). Host SHOULD surface "service discovery unavailable" and MAY retry with backoff. **Distinct from 4**: discovery failures never carry a PAT stderr JSON | 发现层失败（市场/注册表不可达、工具目录解码失败、端点解析错误）。宿主应提示"服务发现暂不可用"，可按退避重试。**与 4 区分**：本 code 绝不携带 PAT stderr JSON |

<!-- evidence: internal/errors/pat.go ExitCodePermission, internal/errors/errors.go ExitCodeDiscovery, w1-lane2 §4.3 -->

> **迁移提示 / Migration note**：旧版本曾把 exit code `4` 用于「发现层失败」。自本契约起 `4` 仅用于 PAT，发现层失败改用 `6`。对接方遇到 4 必须先按 §2 解析 stderr JSON；解析失败再退化为其他分类。<!-- evidence: w1-lane3 §内置 exit code 表冲突说明 -->

---

## 2. Stderr JSON Schema

触发条件 / Trigger：`exit_code == 4`。

CLI 保证 stderr 首个合法 JSON object（允许前后有沙箱 / 日志行）满足下列 schema。

CLI guarantees the **first parsable JSON object** on stderr matches the schema (leading / trailing sandbox or log lines are tolerated by hosts).

### 2.1 顶层字段 / Top-level fields

| Field | Type | Tier | Required | Semantics (EN) | 语义（中文） |
|---|---|---|:---:|---|---|
| `success` | `boolean` | Frozen | Y | Always `false` when exit code is 4 | `exit=4` 时恒为 `false` |
| `code` | `string` | Frozen | Y (if `error_code` absent) | Primary machine-readable selector. One of §2.3 enum | 主识别字段，取 §2.3 枚举值 |
| `error_code` | `string` | Stable | Y (if `code` absent) | Legacy alias of `code`. Host MUST accept both; new CLIs always emit `code` | `code` 的旧别名；宿主必须兼容两者；新版 CLI 只写 `code` |
| `data` | `object` | Frozen | Y | Payload; see §2.2 | 负载对象；见 §2.2 |

> 宿主解析规则：先读 `code`，缺失时回落到 `error_code`。两者都缺失视为非 PAT 响应。<!-- evidence: w1-lane1 §2 +  w1-lane2 §3.2 + internal/errors/pat.go:71-77 -->

### 2.2 `data.*` 字段 / Payload fields

| Field | Type | Tier | Required | Semantics (EN) | 语义（中文） |
|---|---|---|:---:|---|---|
| `requiredScopes` | `string[]` | Stable | N | Scopes that were missing; each element is a canonical `<product>.<entity>:<permission>` string (see §4) | 命令缺失的 scope 列表；每项为规范化字符串（见 §4） |
| `grantOptions` | `string[]` | Stable | N | Allowed grant lifetimes (subset of §3 enum). If missing, host SHOULD default to `["session"]` | 允许的授权时长（§3 枚举子集）；缺失时默认 `["session"]` |
| `authRequestId` | `string` | Stable | N | Opaque correlation id. Required for high-risk flows (§6). Host uses it to bind async callbacks | 服务端签发的相关性 id；高敏必带（§6）；宿主用它绑定异步回执 |
| `displayName` | `string` | Stable | N | Human-readable name of the resource to show in approval UI | 授权 UI 中展示的资源名 |
| `productName` | `string` | Stable | N | Human-readable product name | 授权 UI 中展示的产品名 |
| `hostControl` | `object` | Stable | N | Marks that the host owns UI / polling / retry. See §5 | 指示宿主接管 UI / 轮询 / 重试；见 §5 |
| `missingScope` | `string` | Stable | N | Single missing scope for `PAT_SCOPE_AUTH_REQUIRED` (a special scope-auth flow) | `PAT_SCOPE_AUTH_REQUIRED` 专用的单 scope 字段 |
| `flowId` | `string` | Evolving | N | Optional approval-flow id for host polling. Hosts MUST NOT assume presence | 可选审批流 id；宿主不得假设一定存在 |

**禁止 / Forbidden**：CLI MUST NOT put PAT payload anywhere other than stderr; MUST NOT split across lines; MUST NOT wrap in markdown or ANSI escapes. <!-- evidence: w1-lane2 §3.3 + §7.1 -->

**宿主兼容 / Host tolerance**：Host MUST preserve unknown `data.*` keys for diagnostics and forward compatibility. <!-- evidence: docs/pat-integration.md:92 → normalized -->

### 2.3 `code` / `error_code` 枚举 / Selector enum

> **Risk tier assignment is server-side.** The CLI and hosts never expose a configuration surface for risk level; the `code` field in stderr JSON reflects the classification performed by the server based on scope and organization policy. 风险档位由服务端决定，CLI 与宿主不提供任何配置入口；`code` 字段只是服务端分类结果的被动回显。

Frozen：

- `PAT_NO_PERMISSION`
- `PAT_LOW_RISK_NO_PERMISSION`
- `PAT_MEDIUM_RISK_NO_PERMISSION`
- `PAT_HIGH_RISK_NO_PERMISSION`
- `PAT_SCOPE_AUTH_REQUIRED`
- `AGENT_CODE_NOT_EXISTS`

Host MUST treat unknown selectors that start with `PAT_` as a generic PAT event and degrade to a generic approval UI. See [error-catalog.md](./error-catalog.md) for per-code behaviors.

<!-- evidence: w1-lane1 §6 + internal/errors/pat.go:44-60 -->

### 2.4 JSON Schema (draft 2020-12) 片段

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/docs/pat/contract.md#pat-stderr",
  "type": "object",
  "required": ["success", "data"],
  "properties": {
    "success": { "const": false },
    "code": {
      "type": "string",
      "enum": [
        "PAT_NO_PERMISSION",
        "PAT_LOW_RISK_NO_PERMISSION",
        "PAT_MEDIUM_RISK_NO_PERMISSION",
        "PAT_HIGH_RISK_NO_PERMISSION",
        "PAT_SCOPE_AUTH_REQUIRED",
        "AGENT_CODE_NOT_EXISTS"
      ]
    },
    "error_code": { "type": "string" },
    "data": {
      "type": "object",
      "properties": {
        "requiredScopes":  { "type": "array",  "items": { "type": "string", "pattern": "^[a-z][a-z0-9_-]*(\\.[a-z][a-z0-9_-]*)+:[a-z][a-z0-9_-]*$" } },
        "grantOptions":    { "type": "array",  "items": { "enum": ["once", "session", "permanent"] } },
        "authRequestId":   { "type": "string" },
        "displayName":     { "type": "string" },
        "productName":     { "type": "string" },
        "missingScope":    { "type": "string" },
        "flowId":          { "type": "string" },
        "hostControl": {
          "type": "object",
          "properties": {
            "clawType":      { "type": "string" },
            "mode":          { "const": "host" },
            "pollingOwner":  { "const": "host" },
            "retryOwner":    { "const": "host" }
          }
        }
      },
      "additionalProperties": true
    }
  },
  "oneOf": [
    { "required": ["code"] },
    { "required": ["error_code"] }
  ],
  "additionalProperties": true
}
```

---

## 3. PAT Grant Type / 授权时长

`dws pat chmod` 仅接受下列三值：

CLI accepts exactly three grant types:

| Value | Tier | Meaning (EN) | 语义（中文） | 适用场景 / Typical use |
|---|---|---|---|---|
| `once` | Frozen | Token is consumed by a single successful operation then invalidated | 仅一次有效，一次成功调用后立即失效 | 一次性高风险动作（大批量删除、迁移） |
| `session` | Frozen | Valid within the scope of the host-declared session (see §3.1) | 在宿主声明的会话范围内有效（见 §3.1） | 对话轮内多次复用同一 scope |
| `permanent` | Frozen | Valid until user revokes or scope is rotated server-side | 在用户撤销或服务端轮换前持续有效 | 需要长期授权的工具 |

<!-- evidence: w1-lane1 §2 + wukong/pat/chmod.go:14-18 -->

### 3.1 session-id 来源链 / session-id resolution chain

当 `--grant-type session` 时，CLI 按下列优先级解析 session id，解析失败立即报错退出：

When `--grant-type session`, the CLI resolves the session id via the following priority chain and exits immediately on failure:

1. `--session-id <id>` flag — highest priority
2. `DWS_SESSION_ID` environment variable — **primary** env path (Stable; recommended for new hosts)
3. `REWIND_SESSION_ID` environment variable — **compatibility alias** (Stable; kept only to interoperate with a reference host implementation that already sets the legacy trace triple)
4. Error (CLI refuses to run without a session id)

> 约定 / Convention：`DWS_SESSION_ID` 是开源 CLI 的推荐环境变量；`REWIND_SESSION_ID` 作为兼容别名保留。当两者同时设置且不等时，CLI 以 `DWS_SESSION_ID` 为准并在日志中打 `Warn`。新宿主 SHOULD 直接用 `--session-id` flag 或 `DWS_SESSION_ID`，不再依赖 `REWIND_SESSION_ID`。<!-- evidence: internal/pat/chmod.go resolveSessionIDFromEnv + w1-lane1 §2 + w1-lane2 §4.4 -->

---

## 4. Scope 字符串标准 / Canonical scope string

### 4.1 Frozen 正则 / Frozen regex

```
^[a-z][a-z0-9_-]*(\.[a-z][a-z0-9_-]*)+:[a-z][a-z0-9_-]*$
```

即 `<product>.<entity>[.<sub>]:<permission>`，全部小写 ASCII。

Lowercase ASCII, dot-separated resource path followed by `:` and a single verb. Examples:

- `aitable.record:read`
- `aitable.record:write`
- `contact.user:read`
- `doc.file:create`

### 4.2 单 `scope` 字段 / Single `scope` field

`data.requiredScopes[*]` 与 `dws pat chmod` 的位置参数**均使用同一规范化字符串**。

Both `data.requiredScopes[*]` in stderr JSON and positional args of `dws pat chmod` use the canonical string.

### 4.3 迁移指南（旧三字段 → 单字段）/ Migration from legacy triple

历史上服务端可能返回下列两种旧格式；新版 CLI **在发送给宿主前**统一归一化为单 `scope` 字符串。宿主对外只需实现对单字符串的解析。

CLI normalizes any of the following legacy shapes to the canonical single-string form before forwarding to the host. Hosts only need to parse the canonical form.

| Legacy shape | Canonicalization rule |
|---|---|
| `{ "scope": "aitable.record:read" }` | 直接取 `scope` 字段 / use as-is |
| `{ "productCode": "aitable", "resourceType": "record", "operate": "read" }` | 拼为 `<productCode>.<resourceType>:<operate>` |
| `{ "productCode": "aitable", "operate": "read" }` | 拼为 `<productCode>:<operate>`（无 resourceType 时省略） |

> 归一化发生在 **CLI 侧**；宿主不应重复实现。若宿主仍看到旧格式，说明 CLI 版本过低，应升级。 <!-- evidence: w1-lane2 §3.2 + §7.3 -->

---

## 5. `hostControl` 与 `clawType` 注入点 / host-control contract

### 5.1 字段语义

```json
{
  "hostControl": {
    "clawType": "<effective claw-type header value>",
    "mode": "host",
    "pollingOwner": "host",
    "retryOwner": "host"
  }
}
```

- `clawType`：与 CLI 实际发送的请求头 `claw-type` 同值（见 §7b 身份头）；开源构建中该值被 `pkg/edition/default.go` 的 `MergeHeaders` 钩子**硬编码**为 `edition.DefaultOSSClawType`（字面量 `openClaw`），与 `DINGTALK_AGENT` 或任何宿主环境变量**无关**。宿主**不**需要自己计算；下游 edition 若覆盖了 `MergeHeaders`，其返回的 `claw-type` 取值会同步反映在本字段上。
- `mode`/`pollingOwner`/`retryOwner`：当取值均为 `host` 时，CLI 已放弃 UI / 轮询 / 重试所有权，完全交给宿主。

<!-- evidence: w1-lane2 §6 + w1-lane3 §4 -->

### 5.2 注入点（CLI 实现约束）/ Injection invariant (CLI invariant)

**Invariant**：`data.hostControl` 由 CLI 在 `cleanPATJSON` / classifier 层**统一注入**——**当且仅当** 进程启动时设置了 `DINGTALK_DWS_AGENTCODE`（即 host-owned PAT，见 §7）——不论 PAT 是在主动 retry 路径还是被动 classify 路径上被识别。宿主必须能无条件读到该字段。

**Invariant**: CLI MUST inject `data.hostControl` inside the PAT classifier layer (single code path) **iff** the spawn-time `DINGTALK_DWS_AGENTCODE` is non-empty (i.e. host-owned PAT, see §7), regardless of whether the PAT error was surfaced via the active retry path or the passive classification path. Hosts MUST be able to read it unconditionally.

> 历史实现曾有两条路径不对称，lane3 §4 明确标为 P1 gap；本契约要求收敛到单一注入点。 <!-- evidence: w1-lane3 §4 -->

---

## 6. Risk Level 三档 / Risk levels

| Risk | `code` selector | Host behavior (EN) | 宿主行为（中文） |
|---|---|---|---|
| **Low** | `PAT_LOW_RISK_NO_PERMISSION` | Single-click approval; show one grant option by default (`session`) | 一键授权；默认一个时长选项（`session`） |
| **Medium** | `PAT_MEDIUM_RISK_NO_PERMISSION` | Multi-option approval card; render all `grantOptions`; run `dws pat chmod` after user picks | 多选卡片；渲染所有 `grantOptions`；用户确认后调 `dws pat chmod` |
| **High** | `PAT_HIGH_RISK_NO_PERMISSION` | Requires async approval bound to `authRequestId`; host provides its own approval channel (webhook / sync protocol / second-factor) | 异步审批，用 `authRequestId` 关联；宿主自备通道（Webhook / 同步协议 / 二次确认） |
| — | `PAT_NO_PERMISSION` | Generic fallback; treat as medium-risk unless response states otherwise | 通用回退；无特殊语义时按中敏处理 |
| — | `PAT_SCOPE_AUTH_REQUIRED` | Scope-level re-auth; run `dws auth login --scope <missingScope>` or equivalent host flow | Scope 级别再鉴权；调 `dws auth login --scope ...` 或宿主等价流程 |

<!-- evidence: w1-lane1 §6 + w1-lane2 §3.1 + internal/errors/pat.go:44-60 -->

---

## 7. Host-owned PAT 开关 / Host-owned PAT trigger

CLI 运行时有两种 PAT 模式：

| Mode | 触发条件 | CLI 行为 | 宿主行为 |
|---|---|---|---|
| **CLI-owned**（默认） | `DINGTALK_DWS_AGENTCODE` 未设置 | 遇到 PAT 时，CLI 自己拉浏览器 / 轮询 / 重试 | 仅读 stdout 结果 |
| **Host-owned** | `DINGTALK_DWS_AGENTCODE` 非空 | 遇到 PAT 即 `exit=4` + 单行 stderr JSON，不拉浏览器、不轮询、不重试 | 解析 stderr 渲染自定义授权卡 |

**唯一触发信号 / Sole trigger**：Host-owned 模式的开关**只有** `DINGTALK_DWS_AGENTCODE`。`DINGTALK_AGENT` / `claw-type` / `DWS_CHANNEL` **均不影响** 该决策。

- The sole trigger for host-owned PAT mode is `DINGTALK_DWS_AGENTCODE`. `DINGTALK_AGENT` / `claw-type` / `DWS_CHANNEL` do NOT influence this decision.
- `DINGTALK_AGENT` (if set) is only forwarded as the `x-dingtalk-agent` request header (see §8); it neither derives `claw-type` nor gates host-owned PAT.
- `claw-type` is hard-wired to `openClaw` by the open-source edition's `MergeHeaders` hook (`pkg/edition/default.go`). It is a server-side routing tag, not a host-control knob.

```bash
# Host-owned：弹自己的授权卡
export DINGTALK_DWS_AGENTCODE=agt-cursor
dws aitable record list --sheet-id <id>
# → 命中 PAT 时 exit=4, stderr 是单行 JSON（含 data.hostControl）, CLI 不做任何 UI

# CLI-owned：CLI 拉浏览器
unset DINGTALK_DWS_AGENTCODE
dws aitable record list --sheet-id <id>
# → 命中 PAT 时 CLI 打印授权链接 + 轮询批准
```

<!-- evidence: internal/auth/channel.go HostOwnsPATFlow + internal/app/pat_auth_retry.go + internal/app/pat_hostcontrol_wire.go -->

---

## 7b. Agent Identity Headers / 身份请求头

`dws` 向 MCP gateway 发起的每一个请求都会注入下列头。宿主通常**不需要**知道它们，但开源治理要求透明化。

Every MCP request from `dws` carries the following headers. Hosts typically do not need to set them; listed here for transparency.

| Header | Tier | Semantics (EN) | 语义（中文） | Source |
|---|---|---|---|---|
| `x-dws-agent-id` | Stable | Stable CLI-side identifier derived from local identity.json | CLI 侧稳定标识，源自本地 identity.json | `internal/auth/identity.go` <!-- evidence: w1-lane3 §1.2 --> |
| `x-dws-source` | Stable | Source / distribution channel, e.g. `github` for OSS builds | 来源渠道；开源构建固定 `github` | `internal/auth/identity.go:91` |
| `x-dingtalk-scenario-code` | Stable | Scenario code (edition-specific); OSS default omits it | 场景码（edition 特异）；开源构建默认不写 | edition hook |
| `x-dingtalk-source` | Stable | High-level source marker; OSS default `github` | 高层来源标记；开源默认 `github` | `internal/auth/identity.go` |
| `x-dingtalk-agent` | Stable | Business agent tag forwarded verbatim from the `DINGTALK_AGENT` env when set; omitted otherwise. **Does NOT** affect `claw-type` or host-owned PAT. | 业务 Agent 标签：`DINGTALK_AGENT` 非空时原样转发，否则省略。**不**影响 `claw-type` / host-owned 决策。 | `internal/app/runner.go resolveIdentityHeaders` |
| `claw-type` | **Frozen** | Server-side routing tag; hard-wired to `edition.DefaultOSSClawType` (`openClaw`) by the open-source edition `MergeHeaders` hook. **Does NOT** gate the host-owned PAT decision (see §7) | 上行服务端路由标签；开源构建由 `pkg/edition/default.go` 的 `MergeHeaders` 钩子硬编码为 `openClaw`（字面量）。**不**参与 host-owned 决策（见 §7） | `pkg/edition/default.go` |
| `x-dws-channel` | Stable | Upstream channel metadata; never a host-control switch | 上游通道元数据；**绝不**作为宿主控制位 | `internal/auth/channel.go:22-24` |

---

## 8. `DINGTALK_AGENT` 与 `claw-type` / Env → header mapping

**关键语义 / Key semantics**：`DINGTALK_AGENT` 与 `claw-type` 在当前契约下**完全解耦**。

- **`claw-type`** —— 开源构建恒定注入字面量 `openClaw`，由 `pkg/edition/default.go` 的 `MergeHeaders` 钩子硬编码写入，不接受任何宿主环境变量覆盖。它只是服务端路由标签，**不**决定 host-owned PAT 模式（见 §7）。
- **`DINGTALK_AGENT`** —— 可选的业务 Agent 标签。非空时，CLI 将其原样转发为 HTTP 请求头 `x-dingtalk-agent`（见 §7b）；未设置则该头省略。它**不**派生 `claw-type`、**不**影响 host-owned PAT 决策、**不**进入 `hostControl.clawType` 字段。
- **`DINGTALK_DWS_AGENTCODE`** —— Host-owned PAT 的唯一触发信号（见 §7）。

| Env variable | On-the-wire effect | Affects `claw-type`? | Affects host-owned PAT? |
|---|---|---|---|
| `DINGTALK_DWS_AGENTCODE` | — (not forwarded as a header; consumed as mode trigger + `--agentCode` fallback) | No | **Yes (sole trigger)** |
| `DINGTALK_AGENT` | `x-dingtalk-agent: <value>` when non-empty; omitted when unset | No | No |
| `DWS_CHANNEL` | `x-dws-channel: <value>` when non-empty | No | No |
| — (all above unset) | `claw-type: openClaw` (hard-wired by edition hook) | — | — |

```bash
# Host-owned：单纯触发 host-owned PAT；claw-type 仍固定 openClaw
export DINGTALK_DWS_AGENTCODE=agt-cursor
dws aitable record list --sheet-id <id>
# → 请求头 claw-type: openClaw
# → 命中 PAT 时 stderr 单行 JSON，data.hostControl.clawType=openClaw，exit=4

# Host-owned + 业务 Agent 标签（纯观测用途）
export DINGTALK_DWS_AGENTCODE=agt-cursor
export DINGTALK_AGENT=my-copilot
dws aitable record list --sheet-id <id>
# → 请求头 claw-type: openClaw           （不受 DINGTALK_AGENT 影响）
# → 请求头 x-dingtalk-agent: my-copilot  （DINGTALK_AGENT 原样转发）
# → 命中 PAT 时 data.hostControl.clawType=openClaw
```

<!-- evidence: pkg/edition/default.go MergeHeaders + internal/app/runner.go resolveIdentityHeaders + internal/app/pat_hostcontrol_wire.go -->

---

## 9. 环境变量契约 / Environment variable contract

| Variable | Tier | Consumer | Semantics (EN) | 语义（中文） |
|---|---|---|---|---|
| `DINGTALK_AGENT` | Stable | CLI | Optional business agent tag; when non-empty it is forwarded verbatim as the `x-dingtalk-agent` HTTP header. **Does NOT** derive `claw-type` (hard-wired to `openClaw` by the open-source edition hook) and **does NOT** gate host-owned PAT (see §7) | 可选的业务 Agent 标签；非空时原样转发为 `x-dingtalk-agent` 请求头。**不**派生 `claw-type`（由 `pkg/edition/default.go` 硬编码为 `openClaw`），**不**决定 host-owned PAT（见 §7） |
| `DINGTALK_DWS_AGENTCODE` | Frozen | CLI | **Dual role** (SSOT §1 + §2): (1) **sole** trigger for host-owned PAT mode (see §7), and (2) **sole** per-shell fallback for `--agentCode` on `dws pat` commands. Regex `^[A-Za-z0-9_-]{1,64}$`; `--agentCode` flag wins when both are set. `DWS_AGENTCODE` / `DINGTALK_AGENTCODE` / `REWIND_AGENTCODE` are **not** consulted | **双重作用**（SSOT §1 + §2）：(1) Host-owned PAT 模式的**唯一**触发信号（见 §7）；(2) `dws pat *` 命令 `--agentCode` 的**唯一**每-shell 回退。正则 `^[A-Za-z0-9_-]{1,64}$`；flag 与 env 同时设置时 flag 优先；`DWS_AGENTCODE` / `DINGTALK_AGENTCODE` / `REWIND_AGENTCODE` **不再识别** |
| `DWS_CHANNEL` | Stable | CLI | Upstream `channelCode` metadata only; **not** a host-control switch | 上游 `channelCode` 元数据；**非**宿主控制位 |
| `DWS_CONFIG_DIR` | Frozen | CLI | Override token / identity storage directory | 覆盖凭证 / 身份存储目录 |
| `DWS_SESSION_ID` | Stable | CLI | **Primary** trace / session correlation; also fallback for `--session-id` in `pat chmod` / `pat apply`. Injected as `x-dingtalk-session-id` | **主**路径：trace / 会话相关性；也是 `pat chmod --session-id` 的回退值；注入 `x-dingtalk-session-id` 头 |
| `DWS_TRACE_ID` | Stable | CLI | **Primary** trace request id; injected as `x-dingtalk-trace-id` | **主**路径：trace 请求 id；注入 `x-dingtalk-trace-id` 头 |
| `DWS_MESSAGE_ID` | Stable | CLI | **Primary** trace message id; injected as `x-dingtalk-message-id` | **主**路径：trace 消息 id；注入 `x-dingtalk-message-id` 头 |
| `DINGTALK_SESSION_ID` | Stable | CLI | Backward-compatibility alias for `DWS_SESSION_ID` (pre-rename env). Accepted when `DWS_SESSION_ID` is unset | `DWS_SESSION_ID` 的兼容别名（早期改名前的环境变量）；当主变量未设置时生效 |
| `DINGTALK_TRACE_ID` | Stable | CLI | Backward-compatibility alias for `DWS_TRACE_ID` | `DWS_TRACE_ID` 的兼容别名 |
| `DINGTALK_MESSAGE_ID` | Stable | CLI | Backward-compatibility alias for `DWS_MESSAGE_ID` | `DWS_MESSAGE_ID` 的兼容别名 |
| `REWIND_SESSION_ID` | Stable (compatibility alias) | CLI | Additional compatibility alias for `DWS_SESSION_ID`; kept only to interoperate with a reference host implementation that already sets the legacy trace triple. New integrations SHOULD use `DWS_SESSION_ID` | `DWS_SESSION_ID` 的兼容别名；仅为兼容已有参考宿主实现；新集成应直接使用 `DWS_SESSION_ID` |
| `REWIND_REQUEST_ID` | Stable (compatibility alias) | CLI | Reserved legacy trace-request-id alias; only consumed by edition hooks that predate the `DWS_TRACE_ID` rename | 早期版本的 trace 请求 id 别名；仅被早于重命名的 edition hook 消费 |
| `REWIND_MESSAGE_ID` | Stable (compatibility alias) | CLI | Reserved legacy trace-message-id alias | 早期版本的 trace 消息 id 别名 |
| `DWS_TRUSTED_DOMAINS` | Stable | CLI | Comma-separated host whitelist for bearer-token outbound (default `*.dingtalk.com`) | 携带 bearer token 的出站域名白名单（默认 `*.dingtalk.com`） |
| `DWS_ALLOW_HTTP_ENDPOINTS` | Stable | CLI | Set `1` to allow loopback HTTP during dev | 设 `1` 允许 loopback HTTP，仅开发用 |

**优先级规则 / Resolution rule**：

- **Trace 三件套（`DWS_SESSION_ID` / `DWS_TRACE_ID` / `DWS_MESSAGE_ID`）**：当主路径 (`DWS_*`) 与兼容别名 (`DINGTALK_*` / `REWIND_*`) 同时设置为**不同**非空值时，CLI 以 `DWS_*` 为准，并在日志中以 `Warn` 级别打印冲突。新集成 **SHOULD** 只设置 `DWS_*` 前缀；兼容别名仅为了让已有宿主无须同步改造即可继续工作。
- **Agent code（`DINGTALK_DWS_AGENTCODE`）**：`--agentCode` flag > `DINGTALK_DWS_AGENTCODE` > error。`DWS_AGENTCODE` / `DINGTALK_AGENTCODE` / `REWIND_AGENTCODE` 不再作为 fallback 被识别；宿主若注入这些历史别名，CLI 会视作未设置 agent code 并以硬错误提示迁移到 `DINGTALK_DWS_AGENTCODE`。

**Backwards compatibility**: `REWIND_*` env prefix for trace IDs is retained as an optional alias specifically to avoid forcing existing reference-host integrations to migrate in lock-step; the canonical naming is `DWS_*` for trace IDs. Agent code has a single canonical env (`DINGTALK_DWS_AGENTCODE`) — the pre-SSOT `DWS_AGENTCODE` alias was hard-removed once the public integration surface landed and is no longer recognized.

<!-- evidence: internal/pat/chmod.go resolveAgentCodeFromEnv / resolveSessionIDFromEnv + internal/app/runner.go resolveTraceEnv + w1-lane1 §5.3 + w1-lane2 §4.4 -->

---

## 10. 非契约项 / Non-contract (explicitly excluded)

下列内容**不**属于本 wire contract，CLI 保留无预告修改的权利：

The following are **not** part of this wire contract; CLI reserves the right to change without notice:

1. 具体 MCP gateway 域名、MCP server sha、内部 tool 名（宿主必须通过发现端点而非硬编码获取）
2. CLI 日志格式、tracing span 名
3. `dws` 二进制相对于宿主可执行目录的摆放位置（由宿主自定）
4. 本地凭证文件的加密方案（OS Keychain / 加密文件等，由 edition 决定）
5. 三方 MCP server 的 bearer token 存储细节（由 `PluginAuth` 管理，内部实现）

<!-- evidence: w1-lane1 §8 + w1-lane3 §5 -->

---

## 11. 变更记录 / Change log

| Version | Date | Change |
|---|---|---|
| 1.0 | 2026-04 | Initial open-source contract; supersedes `pat-integration.md` / `pat-agent-quickstart.md` / `third-party-pat-integration.md` |
