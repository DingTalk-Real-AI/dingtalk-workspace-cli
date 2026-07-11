# Reference / 参考手册

## Environment Variables / 环境变量

| Variable | Purpose / 用途 |
|---------|---------|
| `DWS_CONFIG_DIR` | Override default config directory / 覆盖默认配置目录 |
| `DWS_<PRODUCT>_MCP_URL` | Override a product MCP endpoint for local development / 本地开发时覆盖指定产品 MCP endpoint |
| `DWS_CLIENT_ID` | OAuth client ID (DingTalk AppKey) |
| `DWS_CLIENT_SECRET` | OAuth client secret (DingTalk AppSecret) |
| `DWS_TRUSTED_DOMAINS` | Comma-separated trusted domains for bearer token (default: `*.dingtalk.com`). `*` for dev only / Bearer token 允许发送的域名白名单，默认 `*.dingtalk.com`，仅开发环境可设为 `*` |
| `DWS_ALLOW_HTTP_ENDPOINTS` | Set `1` to allow HTTP for loopback during dev / 设为 `1` 允许回环地址 HTTP，仅用于开发调试 |
| `DWS_DISABLE_KEYCHAIN` | macOS only. Set `1` to skip system Keychain for the encryption key and use file-based storage (same scheme as Linux). For sandboxed runtimes (e.g. Codex App) that block Keychain APIs. Weakens at-rest protection — DEK and ciphertext live in the same directory. / 仅 macOS。设为 `1` 时跳过系统 Keychain，密钥以文件形式存储（与 Linux 一致）。用于 Keychain API 被拦截的沙盒环境（如 Codex App）。代价是 DEK 与密文同目录，保护强度低于默认方案 |

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

静态端点模式下，`--help` 展示当前二进制的人类用法，`dws schema` 查询同版本内嵌的 Agent Command Catalog。Schema 查询不访问 MCP endpoint，也不执行 `tools/list`。

### 路径写法

```bash
dws schema                                      # 21 个产品的紧凑概览
dws schema calendar                             # 展开一个产品
dws schema "calendar event"                     # 展开一个命令分组
dws schema "calendar event create"              # 按 CLI 空格路径查询工具
dws schema calendar.create_calendar_event       # 按 canonical path 查询工具
dws schema --cli-path "calendar event create"   # 显式 CLI path
dws schema --all                                # 完整目录，用于审计/CI
```

兼容入口 `dws schema list` 等价于根概览。命令拼法和人类说明仍可用 `dws <path> --help` 查看。

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

`parameters.<flag>.required` 表示该 CLI flag 本身无条件必填。条件必填或别名选择通过 `required_when` 和 `constraints.require_one_of` 表达；它不会直接复制 MCP input schema 的 `required`。

### 筛选输出

```bash
dws schema "calendar event create" --jq '.parameters'                              # 只看参数
dws schema "calendar event create" --jq '[.parameters | to_entries[] | select(.value.required)]'  # 只看必填参数
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
