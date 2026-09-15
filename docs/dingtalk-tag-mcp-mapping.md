# 数字员工 MCP：CLI → 工具 → OpenAPI 字段映射

## 变更与联动边界

本次 CLI 修复基于 main 合并提交 `924394fc9`，开发分支为
`feature/20260915_dingtalk_tag_mcp_agent_scope_1`。
对应 OpenAPI 分支 `feature/20260914_31257090_dws_skill_mcp_update_1`，员工域改动提交 `e19d0d92`。

- CLI 现有 capability mcp create/list/query 均必填 --agent-uuid。
- MCP 平台配置不在 Git 仓库内；下表是需要平台管理员发布的映射，不代表已经生效。
- OpenAPI 的 check/create/query/list/update 五个 MCP 方法均新增必填 agentUuid。
- 本版本不增加 CLI check/update 子命令；这些工具若直接使用，也应同步映射。
- 旧服务端忽略 agentUuid 仍可能写企业域。必须先在预发对齐服务端和映射再验证；仅升级 CLI 不能保证资源域正确。

## 公共身份与员工域

HSF 接口：`com.alibaba.dingtalk.albert.openapi.common.hsf.DeapMcpRemoteService`。
Provider：albert-open-api；group HSF，version 1.0.0。
请求 DTO 包：`com.alibaba.dingtalk.albert.openapi.hsf.dto.request`。

| 工具 | HSF 方法 / DTO | 新增映射 |
| --- | --- | --- |
| check_mcp | checkMcp / McpCheckRequest | 工具.agentUuid → 请求.agentUuid |
| create_mcp | createMcp / McpCreateRequest | 工具.agentUuid → 请求.agentUuid |
| query_mcp | queryMcp / McpQueryRequest | 工具.agentUuid → 请求.agentUuid |
| list_mcps | listMcps / McpListRequest | 工具.agentUuid → 请求.agentUuid |
| update_mcp | updateMcp / McpUpdateRequest | 工具.agentUuid → 请求.agentUuid |

五个工具的 inputSchema 根节点增加 agentUuid，type=string，required，
说明为“目标数字员工 UUID，MCP 所属资源域”。不能取当前企业 corpId 代替。

identity.userId 仍取系统调用工具用户 userId；identity.orgId 仍取系统调用工具组织 corpId 经
CORPID2ORGID 换算。不要开放给业务输入覆盖，也不要用 agentUuid 替换 identity。
OpenAPI 校验目标员工访问权限后，只把资源请求体 tenantId 设为 agentUuid；代理鉴权保留调用人企业身份。

## 创建 / 校验：不要多包一层 config

公开 create_mcp/check_mcp 工具的配置字段是根节点，HSF 的配置字段位于 config 对象内。
这两个形状不一样；CLI 必须发送工具形状，由 MCP 映射组装 HSF 请求。

| 工具根字段 | HSF 字段 | 类型 / 必填 |
| --- | --- | --- |
| agentUuid | agentUuid | string / 是 |
| name | config.name | string / 是 |
| configString | config.configString | string / 是 |
| description、detailIntro、configType | config 下对应字段 | string / 否 |
| userQuestionTips | config.userQuestionTips | string array / 否 |
| envs | config.envs | object，值为 string / 否 |
| toolsDisabled | config.toolsDisabled | object，值为 boolean / 否 |

CLI 配置文件只包含上表配置字段，不能包含 agentUuid/identity/config 包装。
例如（仅说明结构，example.invalid 不是可调用的服务）：

```json
{
  "name": "示例 MCP",
  "configType": "JSON",
  "configString": "{\"mcpServers\":{\"example\":{\"url\":\"https://example.invalid/mcp\"}}}"
}
```

CLI 发送的 tools/call.arguments 为
`{agentUuid, name, configType, configString, ...}`；
HSF 请求为 `{identity, agentUuid, config: {name, configType, configString, ...}}`。
不得将 CLI 请求发成 `{agentUuid, config: {...}}`，否则当前工具映射读不到根节点 configString。
dry-run 只展示字段和脱敏占位值；不调用远端，也不能用预览占位值直接提交服务。

query_mcp 的 mcpId、list_mcps 的 keywords/page/pageSize 保持同名映射。
update_mcp 按现有服务端更新契约映射 mcpId、enabled、config，保留未传与 false/空容器的区别，
不要用默认值填充未传字段。

返回 envelope 的 success/errorCode/errorMsg/data 不变。MCP 资源接口返回该员工域实际 mcpId；
员工详情对历史克隆资源可能保留源 mcpId，更新实例时应使用 instanceId 或员工域 query/list 的实际 mcpId。

## 预发验收

1. 使用同一授权 Profile 和员工 agentUuid，执行 create 并记录返回 mcpId。
2. 日志核对 configString 非空（不记录值），资源 tenantId=agentUuid；鉴权仍是调用人企业/用户。
3. 对同一 agentUuid 执行 query/list，确认原 mcpId 可查；缺少 agentUuid 本地拒绝。
4. save-draft 选择新 MCP，确认 selectedSkills 使用原 ID，不产生 clone；发布后再验证运行态挂载。
5. 校验未授权员工被拒绝、dry-run/错误不泄露凭据，且旧公共资源克隆及显式清空选择语义不变。

本仓库定向单测仅验证 CLI 参数、Schema、脱敏与错误行为，不替代以上服务端端到端验收。
