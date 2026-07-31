# Reference / 参考手册

## Environment Variables / 环境变量

| Variable | Purpose / 用途 |
|---------|---------|
| `DWS_CONFIG_DIR` | Override default config directory / 覆盖默认配置目录 |
| `DWS_AGENT_PRODUCT` | Optional, caller-declared Agent product sent as `x-dws-agent-product` (for example `qwenwork`) for downstream logs/BI and used as the IM `clawType` display label when `--ai-tag` is enabled. `--ai-tag` defaults to `true`, so a configured Product changes the displayed label by default. With `--ai-tag=false`, native `chat message send` / `reply` calls send an empty `clawType`, while shortcut calls omit the argument. Surrounding ASCII spaces/tabs are trimmed; the remaining value must be at most 64 bytes and match `^[A-Za-z0-9][A-Za-z0-9_-]*$`. Unset or empty values omit the Header and use the edition's IM display default. This client never uses Product to change the separate HTTP `claw-type` PAT/routing label. / 可选、由调用方声明的 Agent 产品标识，经校验后作为 `x-dws-agent-product` 发送，并用于 IM 小尾巴；`--ai-tag` 默认为 `true`，因此配置 Product 后默认会改变展示标签。使用 `--ai-tag=false` 时，原生 `chat message send` / `reply` 发送空的 `clawType`，shortcut 调用则省略该参数。未设置时省略请求头且 IM 使用发行版默认值；本客户端不会用 Product 修改独立的 HTTP `claw-type` |
| `DWS_AGENT_HOST` | Optional, caller-declared Agent runtime form sent as `x-dws-agent-host` (for example `cloud` or `desktop`) for downstream logs/BI. Surrounding ASCII spaces/tabs are trimmed; the remaining value must be at most 64 bytes and match `^[a-z0-9][a-z0-9_-]*$`; unset values are omitted. This client does not use Host for PAT, authentication, Discovery, or MCP endpoint selection. / 可选、由调用方声明的 Agent 运行形态，经校验后作为 `x-dws-agent-host` 发送给下游日志/BI；本客户端不使用该值进行 PAT、鉴权、Discovery 或 MCP 端点选择，未设置时省略 |
| `DWS_<PRODUCT>_MCP_URL` | Override a product MCP endpoint for local development / 本地开发时覆盖指定产品 MCP endpoint |
| `DWS_CLIENT_ID` | OAuth client ID (DingTalk AppKey) |
| `DWS_CLIENT_SECRET` | OAuth client secret (DingTalk AppSecret) |
| `DWS_TRUSTED_DOMAINS` | Comma-separated trusted domains for bearer token (default: `*.dingtalk.com`). `*` for dev only / Bearer token 允许发送的域名白名单，默认 `*.dingtalk.com`，仅开发环境可设为 `*` |
| `DWS_ALLOW_HTTP_ENDPOINTS` | Set `1` to allow HTTP for loopback during dev / 设为 `1` 允许回环地址 HTTP，仅用于开发调试 |
| `DWS_DISABLE_KEYCHAIN` | macOS only. Set `1` to skip system Keychain for the encryption key and use file-based storage (same scheme as Linux). For sandboxed runtimes (e.g. Codex App) that block Keychain APIs. Weakens at-rest protection — DEK and ciphertext live in the same directory. / 仅 macOS。设为 `1` 时跳过系统 Keychain，密钥以文件形式存储（与 Linux 一致）。用于 Keychain API 被拦截的沙盒环境（如 Codex App）。代价是 DEK 与密文同目录，保护强度低于默认方案 |

### Agent Product, Host, and `claw-type` / Agent 产品、运行形态与 `claw-type`

`DWS_AGENT_PRODUCT` and `DWS_AGENT_HOST` are caller-declared observation
signals. They are not credentials, attestations, or proof of the calling
host's identity. The CLI validates and emits `x-dws-agent-product` and
`x-dws-agent-host`, but does not use either value to derive its authentication,
PAT mode, Discovery behaviour, or ordinary MCP endpoint selection. Downstream
services own and must document their own contracts for these caller-declared
Headers.

Service integrators should treat both Headers as untrusted input, allowlist
expected values, and should not grant access, bypass authentication, or skip
authorization solely because a Header claims a particular Product or Host.

The HTTP `claw-type` Header is a separate, edition-fixed PAT/routing label:
`openClaw` in the open-source build. `DWS_AGENT_PRODUCT` never changes it or
PAT `hostControl.clawType`. On IM send/reply operations with `--ai-tag`,
however, a valid non-empty Product value is used as the `clawType` tool
argument so the delivered message carries the matching “Send from AI” label.
Because `--ai-tag` defaults to `true`, this display change is enabled by
default for callers that set Product. With `--ai-tag=false`, native
`chat message send` / `reply` calls serialize `clawType: ""`, while shortcut
calls omit the argument; this client does not assume downstream services treat
an empty value and an absent key as equivalent. The display-value precedence
when the tag is enabled is valid non-empty `DWS_AGENT_PRODUCT`, then the active
edition's `ClawTypeValue`, then `openClaw`.

Do not set arbitrary Product values that the target downstream and IM services
have not explicitly enabled; an unknown value may be ignored or may not render
the expected label.

For QwenWork, report the dimensions separately:

```bash
DWS_AGENT_PRODUCT=qwenwork
DWS_AGENT_HOST=cloud       # or desktop
```

Older combined Host labels such as `qwenwork_cloud` still satisfy the generic
syntax for compatibility, but new integrations should use the two-dimensional
convention above.

`DWS_AGENT_PRODUCT` 和 `DWS_AGENT_HOST` 均由调用方声明，不是认证凭据，也不能证明
真实宿主身份。CLI 只负责校验并发送 `x-dws-agent-product` 与 `x-dws-agent-host`，
不会用它们派生本客户端的鉴权、PAT 模式、Discovery 行为或 MCP 端点；下游服务的
使用契约由对应服务自行定义和说明。HTTP `claw-type` 是发行版固定的 PAT/路由标签，
开源版固定为 `openClaw`，不受 `DWS_AGENT_PRODUCT` 影响。

服务集成方应将这两个请求头视为不可信输入并对白名单值做校验，不应仅因请求头声明了
某个 Product 或 Host 就授予访问、绕过认证或跳过鉴权。

`--ai-tag` 默认为 `true`，因此配置合法非空 Product 后，默认发送的 IM 工具参数
`clawType` 及小尾巴会随之改变。传入 `--ai-tag=false` 时，原生
`chat message send` / `reply` 会发送 `clawType: ""`，shortcut 调用则省略该参数；
本客户端不假定下游会将空值与键缺失等价处理。启用小尾巴时，展示值优先级依次为
`DWS_AGENT_PRODUCT`、当前发行版的 `ClawTypeValue`、`openClaw`。不要传入目标下游及
IM 服务未明确支持的 Product 值，否则可能被忽略或无法展示预期标签。

## Exit Codes / 退出码

| Code | Category | Description / 描述 |
|------|----------|-------------|
| 0 | Success | Command completed successfully / 命令执行成功 |
| 1 | API | MCP tool call or upstream API failure / MCP 工具调用或上游 API 失败 |
| 2 | Auth | Authentication or authorization failure / 身份认证或授权失败 |
| 3 | Validation | Invalid input, flags, or parameter schema mismatch / 输入参数校验失败 |
| 4 | PAT | PAT authorization interception; stderr carries raw machine-readable PAT JSON / PAT 授权拦截；stderr 返回原始机器可解析 JSON |
| 5 | Internal | Unexpected internal error / 未预期的内部错误 |
| 6 | Discovery | Static endpoint resolution or protocol negotiation failure / 静态端点解析或协议协商失败 |

With `-f json`, error responses include structured payloads: `category`, `reason`, `hint`, `actions`.

使用 `-f json` 时，错误响应包含结构化字段：`category`、`reason`、`hint`、`actions`。

## Output Formats / 输出格式

```bash
dws contact user search --query "Alice" -f table   # Table (default, human-friendly / 表格，默认)
dws contact user search --query "Alice" -f json    # JSON (for agents and piping / 适合 agent)
dws contact user search --query "Alice" -f raw     # Raw API response / 原始响应
dws schema -f pretty "calendar event create"         # Pretty Agent schema view / Agent Schema 彩色查看
```

## Dry Run / 试运行

```bash
dws todo task list --dry-run    # Preview MCP call without executing / 预览但不执行
```

## Output to File / 输出到文件

```bash
dws contact user search --query "Alice" -o result.json
```

## Schema Introspection / Schema 查询

`--help` 展示当前二进制的 Cobra 命令和可接受 flag，`dws schema` 查询同版本内嵌的 Agent 命令契约。Schema 查询不访问 MCP endpoint、不执行 `tools/list`，也不搜索钉钉文档或任何业务数据。

Schema 的稳定 `canonical_path`、主 CLI 路径和 aliases 来自 reviewed `CommandRegistry`，并在发布时逐项绑定当前 Cobra tree。编辑 `internal/cli/schema_command_registry.json` 时必须遵守同目录的 `schema_command_registry.schema.json`；普通生成流程只校验该 reviewed input，不会覆盖它。Native annotation 只做实现一致性校验；Catalog 是该统一强类型契约的发布输出，不作为命令发现或下一轮生成的输入。

### 路径写法

```bash
dws schema                                      # 当前公开产品面的紧凑概览
dws schema calendar                             # 展开一个产品
dws schema "calendar event"                     # 展开一个命令分组
dws schema "calendar event create"              # 按 CLI 空格路径查询工具
dws schema calendar.create_calendar_event       # 按 canonical path 查询工具
dws schema --cli-path "calendar event create"   # 显式 CLI path
dws schema "calendar event create" --compact    # 支持：省略 provenance/debug 字段
dws schema --all                                # 全部工具的完整 leaf Schema，用于审计/CI/baseline
```

兼容入口 `dws schema list` 等价于根概览。`schema --all` 是完整导出：每个工具都包含完整 leaf 参数、约束和安全语义。它输出很大，只用于明确要求的全量导出、审计、CI 或参数 baseline；普通 Agent 任务应按概览、产品/分组、leaf 渐进查询，不要把 `--all` 直接注入上下文。`schema --all --compact` 虽受支持，但会裁掉 provenance 和接口映射字段，不能作为完整 baseline。

Leaf 查询、`--all` 中对应工具和 Catalog full tool 均由同一个 resolved `ToolSpec` 投影，内容必须一致；概览、产品/分组和 Catalog summary 也由该 `ToolSpec` 的统一 summary 投影生成。通过 alias 查询时，只允许 `cli_path` 和 `is_alias` 发生视图变化，参数、安全和接口契约不得变化。

`--compact` 是 Schema 的展示选项。当前版本支持该 flag；若兼容旧二进制时收到 `unknown_flag: --compact`，用同一个 Schema 查询去掉 `--compact` 重试。这只降低输出裁剪能力，不表示 leaf 不存在，也不能改用 Schema 查询业务数据。

### Schema、Help 与业务数据的边界

| 问题 | 事实源 |
|------|--------|
| 命令是否由当前二进制暴露、Cobra 接受哪些 flags | `dws <path> --help` |
| Agent 选哪个命令、参数映射与组合约束、risk/confirmation | 对应的 leaf `dws schema "<path>"` |
| 当前钉钉中的文档、文件、日程、消息等业务数据 | 实际执行 `dws doc read`、`dws drive search` 等 read/search/list 命令 |

Schema 与 Help 冲突表示发布契约漂移，不能静默猜测。执行参数必须以 Cobra 实际接受的 flag 为准；安全语义冲突时采用更保守的处理（例如先确认）或停止执行并报告漂移。完成命令发现后，仍必须执行真实业务命令；`dws schema` 本身不会读取或搜索业务内容。

### 单工具输出字段

| 字段 | 说明 |
|------|------|
| `canonical_path` / `primary_cli_path` / `aliases` | 稳定工具 ID、主 CLI 路径和兼容路径 |
| `product_id` / `interface_ref` | CLI 产品与实际 MCP product/RPC binding |
| `title` / `description` / `agent_summary` | 人类说明、接口说明和 Agent 摘要 |
| `parameters.<flag>` | CLI flag 的类型、属性名、required、默认值、格式、枚举和条件必填 |
| `constraints` | one-of、互斥、联动等组合约束 |
| `effect` / `risk` / `confirmation` / `idempotency` | Agent 执行与安全策略 |
| `use_when` / `avoid_when` / `examples` | Agent 选择提示和示例 |
| `reviewed` / `agent_source_refs` | 语义审核状态与来源追踪 |

`parameters.<flag>.required` 是按来源 precedence 解析后的 Agent 参数契约；`cli_required=true` 才表示 Cobra 将该 flag 标记为硬必填。条件必填或别名选择通过 `required_when` 和 `constraints.require_one_of` 表达。`required` 不直接复制 MCP input schema，也不取代 Cobra 的实际执行校验。

### 筛选输出

```bash
dws schema "calendar event create" --jq '.parameters'                              # 只看参数
dws schema "calendar event create" --jq '[.parameters | to_entries[] | select(.value.required)]'  # 只看 Agent required 参数
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
