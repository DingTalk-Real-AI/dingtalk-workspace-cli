# MCP 故障定位

## 五步法

1. 对静态命令读取一次精确 leaf：`dws schema --cli-path "dev mcp <leaf>" --compact --format json`。
2. 仅当 Schema 与当前二进制表现冲突时，再读取同一 leaf 的 `--help`。
3. 写操作执行 `--dry-run --format json`，核对实际 helper、对象 ID 和参数。
4. 用 `service get`、`tool get`、`tool versions` 回读平台状态和草稿 `versionId`。
5. 发布后用 `dws mcp published tools <mcpId>` 做实时发现，再用 `published invoke ... --dry-run` 核对调用参数。

同一业务错误在参数、版本和平台状态都没有变化时不要重复调用。得到新证据并修正命令后最多重试一次。

## 常见错误

| 现象 | 判断与处理 |
|---|---|
| `endpoint_not_resolved` | 当前 CLI edition 未提供 `mcpdev` 管理端点，不是 flag 错误。停止真实管理调用并报告，不能用 dry-run 冒充完成 |
| `mcp_not_found` | 当前身份或组织下没有该 mcpId，或管理端点指向错误环境；重新查询 service list |
| MCP 数据初始化中 | 等待短时间后只重试一次 |
| `no_draft_to_publish` | 没有待发布草稿；先 tool get/versions 确认版本 |
| `business_error_invalid_params` | 常见于旧参数名；当前使用 `--tool-id` 和 `--http-info`，不要使用 actionId/http 旧键 |
| debug 成功但接口说缺参数或返回空 | 映射静默失效；检查 HTTP target 是否 Pascal、express 是否使用 `expression`、source/target 是否在字段树声明 |
| debug 通过但发布后仍是旧逻辑 | 调试时可能漏传草稿 versionId，实际命中了已发布版本 |
| tool output 丢失复杂字段 | `apiOutputs` 未声明到真实响应层级，或 output source 超出声明范围；按真实 debug 响应补全后全量 update |
| 管理台显示“变量已失效” | 映射 source/target 无法在同批 Schema 中解析；不要发布，修正字段树和映射 |
| publish 后取址成功但无 `mcpUrl` | 空成功不等于可用；回查服务、已发布工具数和凭证绑定时序 |
| published tools 为空 | 当前身份看不到工具，或服务没有已发布工具；不要尝试猜工具名调用 |
| published invoke 提示工具不存在 | 工具列表在本次调用前已实时刷新；重新执行 tools，使用当前返回的精确名称 |
| published invoke 提示 input schema validation failed | `--params` 缺 required 字段、类型/枚举错误，或包含被 `additionalProperties: false` 禁止的未声明字段；按实时 `inputSchema` 修正 |
| published invoke 提示 unsupported JSON Schema keyword | 实时 Schema 含 oneOf/anyOf/allOf/$ref/pattern/范围等当前核心校验器不能完整执行的约束；为避免带着错误安全保证调用未知副作用工具，CLI 会失败关闭，不发送 `tools/call` |

## HTTP 映射检查

- `apiInputs` 分组是小写数组，映射 target 是 `$.Body/Query/Head/Path`。
- reference/fixed 使用 `source`；express 使用 `expression`，不要放进 `source`。
- 整体输出透传仍需非空 `apiOutputs.body`。
- 字段级 output target 需在 `toolOutputs` 声明。
- array 需检查 `children.items` 和 `[*]` 路径。

完整规则见 [mapping-rules.md](mapping-rules.md)。

## HSF 检查

- 先用 `hsf method-list` 获取方法和 DTO 字段，不凭 Java 记忆猜字段。
- target 是 `$.<DTO简名>.<字段>`，不是 HTTP 位置路径。
- DTO 需要 corpId/userId 时显式注入系统变量。
- output source 不使用 HTTP 的 `.Body` 前缀。
- `update-hsf` 是部分更新；不要传空数组表达“不修改”。

## 发布后调用边界

`dws mcp published tools/invoke` 在运行期按当前 profile 和组织身份重新解析 endpoint，不缓存含凭据 URL，也不注册动态顶层 Cobra 命令。真实 invoke 会先 `tools/list`，确认工具存在并使用当前 `inputSchema` 的 required/type/enum/properties/items 核心约束校验 JSON 参数，再执行 `tools/call`。

当前内置 published transport 面向可直接接受 JSON-RPC `tools/list`/`tools/call` 的端点。它不宣称覆盖必须显式 `initialize`、SSE 响应或 `Mcp-Session-Id` 的严格会话型服务；这类服务应使用 `dws mcp url get` 获得脱敏处理之外的本地连接配置，并交给完整 MCP 客户端，且不得传播 URL 中的凭据。
