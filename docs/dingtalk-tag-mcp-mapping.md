# 数字员工 MCP：CLI → 工具 → OpenAPI 字段映射

## 变更与联动边界

本次 CLI 修复基于 main 合并提交 `924394fc9`，开发分支为
`feature/20260915_dingtalk_tag_mcp_agent_scope_1`。
对应 OpenAPI 分支 `feature/20260914_31257090_dws_skill_mcp_update_1`，员工域改动提交 `e19d0d92`。
自动挂载草稿实现为该 OpenAPI 分支提交 `a0dc9c83`；本说明描述配套版本的目标行为，不表示已部署验收。

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

## MCP 平台工具说明（可直接替换）

自动挂载这次不新增入参或出参字段，保留上面的 agentUuid/config 映射；平台管理员还需同步并发布以下工具说明。此处更新只修改仓库文案，不会自动修改 MCP 平台。

| 工具 | 建议标题 | 工具说明 |
| --- | --- | --- |
| create_mcp | 创建 MCP 并挂载到数字员工草稿 | 在必填 agentUuid 对应员工域校验、创建并查询 MCP，自动将原 mcpId 追加到 draft.selectedSkills，保存并回读确认后返回成功。保留已有 Skill/MCP 和其他选择，不克隆、不自动发布。失败时检查 errorMsg 的 stage 和 data/错误中的已创建 mcpId，先查询资源和草稿，必要时按原 ID saveDraft 恢复，不要重复创建。同一员工的创建、保存、发布应串行执行。 |
| check_mcp | 校验 MCP 配置 | 在目标 agentUuid 员工域校验 MCP 配置及工具解析结果，不保存资源、不写草稿、不发布；校验通过不代表已创建或挂载。 |
| update_mcp | 更新数字员工 MCP | 按 agentUuid + mcpId 更新该员工域已有 MCP 的配置或启用状态，保持未传字段。不创建新资源、不改变草稿 selectedSkills、不重新挂回已移除 MCP、不自动发布。配置入参沿用既有字段映射，不使用 file/fileUrl。 |
| list_mcps | 列出数字员工 MCP 资源 | 按 agentUuid 查询员工域 MCP 资源列表，支持 keywords/page/pageSize；只返回安全元数据，不是企业公共列表。资源存在不代表当前仍被选中或已发布，需分别查询员工 draft/published 详情核验。 |
| query_mcp | 查询数字员工 MCP 详情 | 按 agentUuid + mcpId 查询员工域实际资源定义、工具列表和脱敏配置，不跨域回退、不返回凭据。查询成功不代表当前挂载或已发布；恢复创建后的部分失败时复用此 ID，不要重复创建。 |

失败 envelope 保持 `success/errorCode/errorMsg/data`：`stage=mount_draft` 时 data 保留 mcpId 和安全详情；`stage=query_created_mcp` 时 data 可以为空，原 ID 在 errorMsg。若缺少 ID 或请求超时，先核查资源和草稿，不盲目重试。不要把 `success=false` 的已创建资源当成“什么都没写”。

部署依赖：OpenAPI 自动挂载实现和 Studio internal saveDraft 未传字段保持逻辑必须同时到位。仅发布 CLI 或工具说明不能改变旧服务端行为；跨应用读改写无原子事务保证，不能承诺并发写不会覆盖。

## 预发验收

1. 使用同一授权 Profile 和员工 agentUuid，执行 create 并记录返回 mcpId。
2. 日志核对 configString 非空（不记录值），资源 tenantId=agentUuid；鉴权仍是调用人企业/用户。
3. 对同一 agentUuid 执行 query/list，确认原 mcpId 可查；缺少 agentUuid 本地拒绝。
4. 不额外执行 save-draft，直接查 draft：selectedSkills 自动包含原 mcpId，已有 Skill/MCP/其他 ID 保持，mcps 回显，不产生 clone；published 不因 create 自动改变。用户确认发布后再验证运行态挂载。
5. 独立 check 不写资源/草稿；update 保持选择，移除后 update 也不重新挂载；显式清空后仅新建项被追加，历史未选资源不复活。
6. 覆盖资源创建后 query 失败、挂载失败和挂载回读失败：返回失败阶段及原 ID，可查后恢复、不重复创建；覆盖首次草稿、已有资源及 local_agent 模式保留。
7. 校验未授权员工被拒绝、dry-run/错误不泄露凭据，且旧公共资源克隆及显式清空选择语义不变。资源已创建和草稿已挂载分别记录证据，不用 list 成功代替草稿核验。

本仓库定向单测仅验证 CLI 参数、Schema、脱敏与错误行为，不替代以上服务端端到端验收。
