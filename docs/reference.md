# Reference / 参考手册

## Environment Variables / 环境变量

| Variable | Purpose / 用途 |
|---------|---------|
| `DWS_CONFIG_DIR` | Override default config directory / 覆盖默认配置目录 |
| `DWS_SERVERS_URL` | Point discovery at a custom server registry endpoint / 将服务发现指向自定义端点 |
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
| 6 | Discovery | Server discovery, cache, or protocol negotiation failure / 服务发现、缓存或协议协商失败 |

With `-f json`, error responses include structured payloads: `category`, `reason`, `hint`, `actions`.

使用 `-f json` 时，错误响应包含结构化字段：`category`、`reason`、`hint`、`actions`。

## Output Formats / 输出格式

```bash
dws contact user search --query "Alice" -f table   # Table (default, human-friendly / 表格，默认)
dws contact user search --query "Alice" -f json    # JSON (for agents and piping / 适合 agent)
dws contact user search --query "Alice" -f raw     # Raw API response / 原始响应
dws schema -f pretty ding.send_ding_message          # Pretty (ANSI-colored, schema-aware / 彩色分区，专为 schema 设计)
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

`dws schema` 查询已发现的 MCP 产品和工具元数据。不带参数列出所有产品，带路径输出单个工具的完整 schema。

### 路径写法

```bash
dws schema                                  # 列出所有产品 + 工具名
dws schema ding.send_ding_message           # canonical: product.rpc_name
dws schema ding.message.send                # CLI 点路径: product.group.cli_name
dws schema "ding message send"              # CLI 空格路径（同上）
dws schema --cli-path "ding message send"   # 显式 flag（脚本友好，免转义）
dws schema -f pretty ding.send_ding_message # ANSI 着色分区展示（人肉查看最舒服）
```

Canonical 路径先匹配；落空后走 CLI 路径（product → group.. → cli_name）。

### 单工具输出字段

| 字段 | 说明 |
|------|------|
| `name` / `cli_name` / `canonical_path` | MCP RPC 名 / CLI 叶子名 / `product.rpc_name` |
| `group` | CLI 父级 group 路径（dot-separated） |
| `title` / `description` | 工具名/说明（overlay 优先） |
| `parameters` / `required` | MCP 输入 JSON Schema 的 properties / required |
| `output_schema` | MCP 输出 Schema（上游下发时才有） |
| `sensitive` | 敏感写操作，需 `--yes` 确认 |
| `annotations.destructive_hint` | 对齐 MCP 2025+ annotations，目前从 `sensitive` 映射 |
| `flag_overlay[param]` | CLI 层对 MCP 参数的改写：`alias` / `transform` / `transform_args` / `env_default` / `default` / `hidden` |

**调试 `--flag` 行为的第一站**是 `flag_overlay` —— 比如 `--users 0232...` 能不能直接用，看 `receiverUserIdList.transform == "csv_to_array"` 即可判断。

### 筛选输出

```bash
dws schema ding.send_ding_message --jq '.tool.flag_overlay'       # 只看 overlay
dws schema --jq '.products[] | {id, count: (.tools|length)}'      # 各产品工具数
dws schema aitable.delete_base --jq '.tool.annotations'           # 敏感操作提示
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
