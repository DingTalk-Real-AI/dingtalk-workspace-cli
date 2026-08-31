# MCP 服务与工具开发

命令分为两层：

- `dws dev mcp ...`：开发端，管理服务、工具、鉴权、凭证、协作者和 HSF 方法。
- `dws mcp published ...`：消费端，按 `mcpId` 查看或调用已发布工具。

远端 `serverName` 和工具名不会注册成新的顶层命令，也不写入跨身份缓存。消费端每次按当前登录 profile、用户和组织身份解析服务地址；输出只展示脱敏地址。

## MUST DO

1. 先用对应分组或叶子的 `--help` 确认当前参数。
2. 开发写操作先 `--dry-run --format json`，检查 invocation，再改为 `--yes`。
3. 发布前执行 `tool get` 和 `tool debug`，核对输入、输出与映射。
4. 调用已发布工具前先执行 `mcp published tools <mcpId>`，按返回的 `inputSchema` 构造 `--params`。
5. `mcp published invoke` 无法静态判断远端工具副作用，真实调用一律需要 `--yes`；未明确影响时只做 dry-run。
6. MCP URL、凭证内容、token 和密钥不得写入回答、日志、文档或代码仓库。

## 服务

```bash
dws dev mcp service list --format json
dws dev mcp service get --mcp-id <mcpId> --format json
dws dev mcp service create \
  --name 示例服务 \
  --description "服务用途" \
  --server-name example-service \
  --dry-run --format json
dws dev mcp service update --mcp-id <mcpId> --description "新描述" --dry-run --format json
dws dev mcp service delete --mcp-id <mcpId> --dry-run --format json
```

`server-name` 使用 kebab-case，作为服务的稳定语义标识；它不再生成 DWS 顶层命令。

## HTTP 工具

创建和更新前先准备：

- `http-info`：HTTP method、URL 与 auth。
- `api-inputs` / `api-outputs`：下游接口真实字段树。
- `tool-inputs` / `tool-outputs`：暴露给 Agent 的字段树。
- `input-mappings` / `output-mappings`：工具字段与下游字段的映射。

HTTP 出参整体透传示例：

```json
[{"target":"$","type":"reference","source":"$.node_service_activator.Body"}]
```

整体透传仍必须如实声明 `api-outputs.body`；未声明字段会被裁剪。字段级映射引用的路径必须存在于同批提交的字段树中。

```bash
dws dev mcp tool list --mcp-id <mcpId> --format json
dws dev mcp tool get --mcp-id <mcpId> --tool-id <toolId> --format json
dws dev mcp tool debug --mcp-id <mcpId> --tool-id <toolId> \
  --value '{"query":"example"}' --no-credential --dry-run --format json
dws dev mcp tool publish --mcp-id <mcpId> --tool-id <toolId> --dry-run --format json
```

HTTP `tool update` 是全量提交：先 `tool get` 读回现状，再在完整定义上修改，不要只传单个字段。

## HSF 工具

先发现方法，再创建或部分更新：

```bash
dws dev mcp hsf method-list --interface-name <fully.qualified.Interface> --format json
dws dev mcp tool create-hsf --help
dws dev mcp tool update-hsf --help
```

HSF 的 `apiInputs/apiOutputs` 由服务端按方法 Schema 生成。映射 target 使用 `$.<DTO简名>.<字段>`；输出 source 使用 `$.node_service_activator.<字段>`，不带 HTTP 的 `.Body`。

## 鉴权与凭证

```bash
dws dev mcp auth get --mcp-id <mcpId> --format json
dws dev mcp auth save --mcp-id <mcpId> --auth-type NO_AUTH --dry-run --format json
dws dev mcp credential list --mcp-id <mcpId> --format json
dws dev mcp credential save --mcp-id <mcpId> --name 示例账号 \
  --content-file <local-json-file> --dry-run --format json
dws dev mcp credential bind --mcp-id <mcpId> --credential-id <credentialId> --dry-run --format json
dws dev mcp credential unbind --mcp-id <mcpId> --dry-run --format json
```

敏感内容优先用 `--content-file` 或 stdin，不要放进 shell history。`credential debug` 会真实访问下游；`tool debug` 必须明确二选一：`--credential-id` 或 `--no-credential`。

## 协作者

```bash
dws dev mcp member list --mcp-id <mcpId> --format json
dws dev mcp member add --mcp-id <mcpId> --user-ids <staffId1,staffId2> --dry-run --format json
dws dev mcp member remove --mcp-id <mcpId> --user-ids <staffId> --dry-run --format json
```

## 调用已发布工具

```bash
dws mcp published tools <mcpId> --format json
dws mcp published invoke <mcpId> <toolName> \
  --params '{"query":"example"}' --dry-run --format json
```

检查 dry-run 后，只有用户明确同意本次真实调用，调用方才可在执行时追加确认标志；不要把确认标志固化进模板、脚本或可复制示例。

`tools` 返回当前身份看到的实时工具列表。`invoke` 不接受动态命令别名，不根据工具名猜读写属性，也不持久化含凭据的 endpoint。
