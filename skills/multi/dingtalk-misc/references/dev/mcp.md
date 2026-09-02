# MCP 服务与工具开发

命令分为两层：

- `dws dev mcp ...`：开发端，管理服务、工具、鉴权、凭证、协作者和 HSF 方法。
- `dws mcp published ...`：消费端，按 `mcpId` 查看或调用已发布工具。

远端 `serverName` 和工具名不会注册成新的顶层命令，也不写入跨身份缓存。消费端每次按当前登录 profile、用户和组织身份解析服务地址；输出只展示脱敏地址。

## 渐进参考

先使用本页确定阶段，只在进入对应步骤时加载一份深层参考：

| 任务阶段 | 参考 |
|---|---|
| OpenAPI/Swagger/Postman/curl/接口文档转工具 | [API 材料转工具](mcp/api-to-tool.md) |
| 编写 input/output mappings | [字段与映射规则](mcp/mapping-rules.md) |
| 配置 API key、token、签名或凭证 | [鉴权与凭证](mcp/auth-and-credentials.md) |
| 使用复杂 express 变换 | [7 组 82 个表达式函数](mcp/expression-functions.md) |
| debug、发布或调用异常 | [故障定位](mcp/troubleshooting.md) |

不要在普通服务查询或已发布工具调用时预读全部参考。

## MUST DO

1. 下列已知 leaf 直接执行，不先读分组 help。只有创建/更新等复杂字段或确认语义确有不确定性时，读取一次 `dws schema --cli-path "dev mcp <leaf>" --compact --format json`；Schema 与 Cobra 不一致时才读取同一 leaf 的一次 help。
2. 开发写操作的最初请求只允许 `--dry-run --format json`。展示 invocation 中的准确对象、动作、业务参数和影响，等用户对该预览明确确认后，才把同一命令仅由 `--dry-run` 换成 `--yes`；目标或业务参数变化就重新预检并确认，确认前不得真实写入。
3. 发布前执行 `tool get` 和 `tool debug`，核对输入、输出与映射。
4. 调用已发布工具前先执行 `mcp published tools <mcpId>`，按返回的 `inputSchema` 构造 `--params`；真实 invoke 还会在本次调用中重新发现工具并按实时 Schema 校验。
5. `mcp published invoke` 无法静态判断远端工具副作用，真实调用一律需要 `--yes`；未明确影响时只做 dry-run。
6. MCP URL、凭证内容、token 和密钥不得写入回答、日志、文档或代码仓库。
7. 若真实命令返回 `endpoint_not_resolved`，表示当前 CLI 的 `mcpdev` 运行端点不可用，不是 flag 错误；立即报告并停止，不用 help、改参数、Raw API 或 dry-run 冒充业务完成。

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

用户给 API 材料要求转换为 MCP 时，先按 [API 材料转工具](mcp/api-to-tool.md) 完成业务拆分和设计评审，再执行创建命令。任何 input/output mapping 写入前必须读取 [字段与映射规则](mcp/mapping-rules.md)。

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
dws schema --cli-path "dev mcp tool create-hsf" --compact --format json
# 更新时同理，只在本次确需 update-hsf 且字段不确定时读取它的 compact leaf Schema
```

HSF 的 `apiInputs/apiOutputs` 由服务端按方法 Schema 生成。映射 target 使用 `$.<DTO简名>.<字段>`；输出 source 使用 `$.node_service_activator.<字段>`，不带 HTTP 的 `.Body`。

## 鉴权与凭证

鉴权类型选择、静态 API key 和动态 token 模板见 [鉴权与凭证](mcp/auth-and-credentials.md)。

```bash
dws dev mcp auth get --mcp-id <mcpId> --format json
dws dev mcp auth save --mcp-id <mcpId> --auth-type NO_AUTH --dry-run --format json
dws dev mcp credential list --mcp-id <mcpId> --format json
dws dev mcp credential save --mcp-id <mcpId> --name 示例账号 \
  --content-file <local-json-file> --dry-run --format json
dws dev mcp credential bind --mcp-id <mcpId> --credential-id <credentialId> --dry-run --format json
dws dev mcp credential unbind --mcp-id <mcpId> --dry-run --format json
```

敏感内容优先用 `--content-file <path>`；需要 stdin 时精确写成 `--content-file -`，不要把 JSON 放进 shell history。`credential debug` 会真实访问下游；`tool debug` 必须明确二选一：`--credential-id` 或 `--no-credential`。

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

展示 dry-run 的准确对象、参数和潜在影响后，只有用户对该预览明确同意本次真实调用，调用方才可在执行时把同一命令仅由 `--dry-run` 换成确认标志；最初请求不算该确认，参数变化需重新预检。不要把确认标志固化进模板、脚本或可复制示例。

`tools` 返回当前身份看到的实时工具列表。真实 `invoke` 会再次执行 `tools/list`，拒绝不存在的工具、缺失 `inputSchema` 的工具以及不符合 required/type/enum/properties/items 核心约束的参数，然后才执行 `tools/call`。成功输出以 `inputSchemaValidation=core` 说明本次门禁范围。它不接受动态命令别名，不根据工具名猜读写属性，也不持久化含凭据的 endpoint。

当前内置调用面适用于可直接接受 JSON-RPC `tools/list`/`tools/call` 的服务，不宣称覆盖必须显式 initialize、SSE 或 `Mcp-Session-Id` 的严格会话型端点。异常处理见 [故障定位](mcp/troubleshooting.md)。
