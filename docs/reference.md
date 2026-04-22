# Reference / 参考手册

## Environment Variables / 环境变量

### Core / 核心

| Variable | Purpose / 用途 |
|---------|---------|
| `DWS_CONFIG_DIR` | Override default config directory / 覆盖默认配置目录 |
| `DWS_SERVERS_URL` | Point discovery at a custom server registry endpoint / 将服务发现指向自定义端点 |
| `DWS_CLIENT_ID` | OAuth client ID (DingTalk AppKey) |
| `DWS_CLIENT_SECRET` | OAuth client secret (DingTalk AppSecret) |
| `DWS_TRUSTED_DOMAINS` | Comma-separated trusted domains for bearer token (default: `*.dingtalk.com`). `*` for dev only / Bearer token 允许发送的域名白名单，默认 `*.dingtalk.com`，仅开发环境可设为 `*` |
| `DWS_ALLOW_HTTP_ENDPOINTS` | Set `1` to allow HTTP for loopback during dev / 设为 `1` 允许回环地址 HTTP，仅用于开发调试 |

### PAT

| Variable | Purpose / 用途 | Tier |
|---------|---------|---|
| `DINGTALK_AGENT` | Optional business agent tag. When non-empty the CLI forwards it verbatim as the `x-dingtalk-agent` request header; omitted otherwise. Does **not** derive `claw-type` (hard-wired to `openClaw` by the open-source edition hook) and does **not** gate host-owned PAT — see `DINGTALK_DWS_AGENTCODE` below. / 可选的业务 Agent 标签；非空时原样转发为 `x-dingtalk-agent` 请求头，空则省略。**不**派生 `claw-type`（由 `pkg/edition/default.go` 硬编码为 `openClaw`），**不**决定 host-owned PAT（见下方 `DINGTALK_DWS_AGENTCODE`） | Stable |
| `DWS_CHANNEL` | Upstream `channelCode` metadata only. **Not** a host-control switch / 仅上游 `channelCode` 元数据，**非**宿主控制位 | Stable |
| `DINGTALK_DWS_AGENTCODE` | **Dual role**: (1) **sole** trigger for host-owned PAT mode — when set, the CLI emits `exit=4` + single-line stderr JSON instead of opening a local browser for authorization; (2) **sole** per-shell fallback for `--agentCode` on `dws pat chmod`. Regex `^[A-Za-z0-9_-]{1,64}$`; `--agentCode` flag wins when both are set. See [pat/contract.md §7](./pat/contract.md#7-host-owned-pat-开关--host-owned-pat-trigger). / **双重作用**：(1) Host-owned PAT 模式的**唯一**触发信号；(2) `dws pat chmod` 的 `--agentCode` **唯一**每-shell 回退。正则 `^[A-Za-z0-9_-]{1,64}$`；flag 优先 | Frozen |
| `DINGTALK_SESSION_ID` | Forwarded verbatim as `x-dingtalk-session-id` HTTP header / 原样转发为 `x-dingtalk-session-id` 请求头 | Stable |
| `DINGTALK_TRACE_ID` | Forwarded verbatim as `x-dingtalk-trace-id` HTTP header / 原样转发为 `x-dingtalk-trace-id` 请求头 | Stable |
| `DINGTALK_MESSAGE_ID` | Forwarded verbatim as `x-dingtalk-message-id` HTTP header / 原样转发为 `x-dingtalk-message-id` 请求头 | Stable |
| `DWS_SESSION_ID` | Fallback for `--session-id` on `dws pat chmod` under `--grant-type session`; **not** injected into trace headers / `pat chmod --grant-type session` 的 `--session-id` 回退；**不**注入 trace 头 | Stable |
| `REWIND_SESSION_ID` | Compatibility alias for `DWS_SESSION_ID` / `DWS_SESSION_ID` 的兼容别名 | Stable (compat) |

> **Non-consumed aliases / 不识别的别名**：`DWS_AGENTCODE`、`DINGTALK_AGENTCODE`、`REWIND_AGENTCODE`。

See [docs/pat/contract.md](./pat/contract.md) for field-level tier guarantees.

## Exit Codes / 退出码

CLI 对外承诺 **0 / 2 / 4 / 5 / 6** 五种退出码。所有其他值视为未定义行为。

| Code | Category | Description / 描述 |
|------|----------|-------------|
| 0 | Success | Command completed successfully / 命令执行成功 |
| 2 | Auth | Identity-layer authentication or authorization failure (token missing / expired / revoked / org unauthorized). Host MUST re-login before retry / 身份层认证或授权失败（token 缺失 / 过期 / 吊销 / 组织未授权）；宿主必须重新登录后再重试 |
| 4 | PAT Permission | PAT permission insufficient. stderr is a single-line JSON payload conforming to [docs/pat/contract.md §2](./pat/contract.md#2-stderr-json-schema). **Reserved exclusively for PAT** / PAT 权限不足，stderr 为单行 JSON，遵循契约 §2。**本码仅用于 PAT，不复用** |
| 5 | Internal | Unexpected internal error (panic / unrecoverable IO). Host SHOULD log full stderr / 未预期内部错误（panic / 不可恢复 IO）；宿主应记录完整 stderr |
| 6 | Discovery | Discovery / catalog failure (market registry unreachable, endpoint resolution broken). Host MAY retry with backoff; does **not** carry a PAT JSON / 发现层失败（市场注册表不可达、端点解析失败）；宿主可退避重试，**不**携带 PAT JSON |

With `-f json`, error responses include structured payloads: `category`, `reason`, `hint`, `actions`.

使用 `-f json` 时，错误响应包含结构化字段：`category`、`reason`、`hint`、`actions`。

PAT 场景的 stderr JSON 契约与 code 枚举见 [docs/pat/contract.md §2 / §6](./pat/contract.md#2-stderr-json-schema)。

## Output Formats / 输出格式

```bash
dws contact user search --keyword "Alice" -f table   # Table (default, human-friendly / 表格，默认)
dws contact user search --keyword "Alice" -f json    # JSON (for agents and piping / 适合 agent)
dws contact user search --keyword "Alice" -f raw     # Raw API response / 原始响应
```

## Dry Run / 试运行

```bash
dws todo task list --dry-run    # Preview MCP call without executing / 预览但不执行
```

## Output to File / 输出到文件

```bash
dws contact user search --keyword "Alice" -o result.json
```

## Shell Completion / 自动补全

```bash
# Bash
dws completion bash > /etc/bash_completion.d/dws

# Zsh
dws completion zsh > "${fpath[1]}/_dws"

# Fish
dws completion fish > ~/.config/fish/completions/dws.fish
```

## PAT Subcommands / PAT 子命令

`dws pat` 命令组用于管理第三方 Agent 的个人授权。完整集成指南见 [docs/pat/host-integration.md](./pat/host-integration.md)。

> 本 PR（A-core）仅包含 `dws pat chmod`，其余 `dws pat apply` / `status` / `scopes` 在后续扩展 PR（A-ext）中发布。

| Command | Status | Legacy tool-name fallback | Purpose / 用途 |
|---|---|---|---|
| `dws pat chmod <scope>... --agentCode <id> --grant-type <once\|session\|permanent> [--session-id <id>]` | Available | `pat.grant` → `"个人授权"` (Chinese alias; migration shim) | Grant PAT scopes to an agent / 给 agent 授予指定 scope |

### `dws pat chmod`

```bash
# Grant `aitable.record:read` to agent `agt-xxx` within the current session
dws pat chmod aitable.record:read \
    --agentCode agt-xxx \
    --grant-type session \
    --session-id conv-001

# One-shot grant
dws pat chmod doc.file:create \
    --agentCode agt-xxx \
    --grant-type once

# Permanent grant (triggers server-side high-risk approval if applicable)
dws pat chmod mail:send \
    --agentCode agt-xxx \
    --grant-type permanent
```

Flag semantics:

- `<scope...>`: one or more canonical scope strings (`<product>.<entity>:<permission>`). See [contract.md §4](./pat/contract.md#4-scope-字符串标准--canonical-scope-string).
- `--agentCode` (required): business agent code; host-defined stable identifier; also accepts `$DINGTALK_DWS_AGENTCODE` env fallback. Flag wins when both are set. `DWS_AGENTCODE` / `DINGTALK_AGENTCODE` / `REWIND_AGENTCODE` are not consulted.
- `--grant-type`: one of `once` / `session` / `permanent` (Frozen enum)
- `--session-id`: required when `--grant-type session`; env fallback `DWS_SESSION_ID` → `REWIND_SESSION_ID`

Exit codes:

- `0`: chmod applied; host MAY re-run the original command
- `2`: identity layer failure; re-login required
- `4`: chmod itself hit a higher-risk PAT gate (rare; stderr JSON explains)
- `5`: internal error

## Request Headers Injected by CLI / CLI 注入的请求头

下列请求头由 CLI 统一注入；宿主**不需要**手动设置。

| Header | Derived from | Tier |
|---|---|---|
| `x-dingtalk-agent` | `DINGTALK_AGENT` env (when non-empty; omitted otherwise) | Stable |
| `claw-type` | Hard-wired to `openClaw` by the open-source edition `MergeHeaders` hook (`pkg/edition/default.go`); independent of `DINGTALK_AGENT` | Frozen |
| `x-dws-channel` | `DWS_CHANNEL` env | Stable |
| `x-dws-agent-id` | Local `identity.json` | Stable |
| `x-dws-source` | Distribution channel (OSS default `github`) | Stable |
| `x-dingtalk-scenario-code` | Edition hook (OSS default: unset) | Stable |
| `x-dingtalk-source` | Distribution channel marker | Stable |

字段级契约与 tier 说明见 [docs/pat/contract.md §7](./pat/contract.md)。
