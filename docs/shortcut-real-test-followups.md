# Shortcut 真实测试跟进清单

生成时间：`2026-07-15T16:58:39`

来源：`docs/shortcut-real-read-results.json` 与 `docs/shortcut-real-write-results.json`。

口径：记录真实后端测试中需要继续定位的 case，用于 CR 和问题分派；Agent 使用入口以公开 shortcut catalog 和产品 skill 为准。

总计：156 条。

| # | suite | shortcut | risk | status | category | fixability | 处理依据 |
|---:|---|---|---|---|---|---|---|
| 1 | read | `aitable +base-get-primary-doc-id` | read | real-error | backend-or-mcp-error | not-cli-fixable-first | 后端/MCP 服务返回内部错误；CLI 无法直接修复，但报告保留 trace/stdout 供服务端排查。 |
| 2 | read | `aitable +chart-share-get` | read | real-error | auth-or-permission | not-cli-fixable | 真实账号、应用 scope 或资源权限不足；CLI 只能如实暴露，不能在本仓库内修复权限。 |
| 3 | read | `aitable +dashboard-share-get` | read | real-error | missing-real-resource | not-cli-fixable-without-fixture | 真实测试使用的资源/单据/消息/群/文档不存在；需要准备对应 fixture 后才能期望成功，不属于 shortcut 参数投影错误。 |
| 4 | read | `aitable +export-data` | read | real-error | backend-or-mcp-error | not-cli-fixable-first | 后端/MCP 服务返回内部错误；CLI 无法直接修复，但报告保留 trace/stdout 供服务端排查。 |
| 5 | read | `aitable +record-primary-doc-get` | read | real-error | backend-or-mcp-error | not-cli-fixable-first | 后端/MCP 服务返回内部错误；CLI 无法直接修复，但报告保留 trace/stdout 供服务端排查。 |
| 6 | read | `aitable +role-get` | read | real-error | backend-or-mcp-error | not-cli-fixable-first | 后端/MCP 服务返回内部错误；CLI 无法直接修复，但报告保留 trace/stdout 供服务端排查。 |
| 7 | read | `aitable +workflow-get` | read | real-error | backend-or-mcp-error | not-cli-fixable-first | 后端/MCP 服务返回内部错误；CLI 无法直接修复，但报告保留 trace/stdout 供服务端排查。 |
| 8 | read | `aitable +workflow-list` | read | real-error | backend-or-mcp-error | not-cli-fixable-first | 后端/MCP 服务返回内部错误；CLI 无法直接修复，但报告保留 trace/stdout 供服务端排查。 |
| 9 | read | `attendance +get-class` | read | real-error | missing-real-resource | not-cli-fixable-without-fixture | 真实测试使用的资源/单据/消息/群/文档不存在；需要准备对应 fixture 后才能期望成功，不属于 shortcut 参数投影错误。 |
| 10 | read | `attendance +get-global-setting` | read | real-error | auth-or-permission | not-cli-fixable | 真实账号、应用 scope 或资源权限不足；CLI 只能如实暴露，不能在本仓库内修复权限。 |
| 11 | read | `attendance +get-group` | read | real-error | missing-real-resource | not-cli-fixable-without-fixture | 真实测试使用的资源/单据/消息/群/文档不存在；需要准备对应 fixture 后才能期望成功，不属于 shortcut 参数投影错误。 |
| 12 | read | `attendance +get-group-filtered` | read | real-error | missing-real-resource | not-cli-fixable-without-fixture | 真实测试使用的资源/单据/消息/群/文档不存在；需要准备对应 fixture 后才能期望成功，不属于 shortcut 参数投影错误。 |
| 13 | read | `attendance +get-leave-balance` | read | real-error | input-or-business-validation | test-input-or-backend-rule | 命令已真实进入本地/后端校验；若该项仍使用安全负向输入，则失败符合预期；若使用真实 fixture 仍失败，再作为 CLI bug 处理。 |
| 14 | read | `attendance +list-report-columns` | read | real-error | auth-or-permission | not-cli-fixable | 真实账号、应用 scope 或资源权限不足；CLI 只能如实暴露，不能在本仓库内修复权限。 |
| 15 | read | `attendance +query-report-leave` | read | real-error | auth-or-permission | not-cli-fixable | 真实账号、应用 scope 或资源权限不足；CLI 只能如实暴露，不能在本仓库内修复权限。 |
| 16 | read | `calendar +find-room` | read | real-error | input-or-business-validation | test-input-or-backend-rule | 命令已真实进入本地/后端校验；若该项仍使用安全负向输入，则失败符合预期；若使用真实 fixture 仍失败，再作为 CLI bug 处理。 |
| 17 | read | `calendar +room-find` | read | real-error | input-or-business-validation | test-input-or-backend-rule | 命令已真实进入本地/后端校验；若该项仍使用安全负向输入，则失败符合预期；若使用真实 fixture 仍失败，再作为 CLI bug 处理。 |
| 18 | read | `chat +category-list-conversations` | read | real-error | input-or-business-validation | test-input-or-backend-rule | 命令已真实进入本地/后端校验；若该项仍使用安全负向输入，则失败符合预期；若使用真实 fixture 仍失败，再作为 CLI bug 处理。 |
| 19 | read | `chat +chat-get-by-id` | read | real-error | missing-real-resource | not-cli-fixable-without-fixture | 真实测试使用的资源/单据/消息/群/文档不存在；需要准备对应 fixture 后才能期望成功，不属于 shortcut 参数投影错误。 |
| 20 | read | `chat +chat-members-get` | read | real-error | backend-or-mcp-error | not-cli-fixable-first | fake MCP 已证明 CLI 已装配会话 ID 字段；真实后端仍报 openConversationId/openCid/cid 缺失，优先按 MCP schema/服务端字段映射问题处理。 |
| 21 | read | `chat +chat-messages` | read | real-error | backend-or-mcp-error | not-cli-fixable-first | fake MCP 已证明 CLI 已装配会话 ID 字段；真实后端仍报 openConversationId/openCid/cid 缺失，优先按 MCP schema/服务端字段映射问题处理。 |
| 22 | read | `chat +messages-list` | read | real-error | backend-or-mcp-error | not-cli-fixable-first | fake MCP 已证明 CLI 已装配会话 ID 字段；真实后端仍报 openConversationId/openCid/cid 缺失，优先按 MCP schema/服务端字段映射问题处理。 |
| 23 | read | `chat +messages-resource-url` | read | real-error | missing-real-resource | not-cli-fixable-without-fixture | 真实测试使用的资源/单据/消息/群/文档不存在；需要准备对应 fixture 后才能期望成功，不属于 shortcut 参数投影错误。 |
| 24 | read | `chat +search-msg` | read | real-error | input-or-business-validation | test-input-or-backend-rule | 命令已真实进入本地/后端校验；若该项仍使用安全负向输入，则失败符合预期；若使用真实 fixture 仍失败，再作为 CLI bug 处理。 |
| 25 | read | `chat +thread-replies` | read | real-error | missing-real-resource | not-cli-fixable-without-fixture | 真实测试使用的资源/单据/消息/群/文档不存在；需要准备对应 fixture 后才能期望成功，不属于 shortcut 参数投影错误。 |
| 26 | read | `contact +get-roster` | read | real-error | auth-or-permission | not-cli-fixable | 真实账号、应用 scope 或资源权限不足；CLI 只能如实暴露，不能在本仓库内修复权限。 |
| 27 | read | `contact +list-roster-fields` | read | real-error | auth-or-permission | not-cli-fixable | 真实账号、应用 scope 或资源权限不足；CLI 只能如实暴露，不能在本仓库内修复权限。 |
| 28 | read | `devapp +credentials-get` | read | held | held | manual-approval | 高风险或无安全目标，需人工逐项授权后执行。 |
| 29 | read | `drive +download` | read | real-error | input-or-business-validation | test-input-or-backend-rule | 命令已真实进入本地/后端校验；若该项仍使用安全负向输入，则失败符合预期；若使用真实 fixture 仍失败，再作为 CLI bug 处理。 |
| 30 | read | `drive +list` | read | real-error | input-or-business-validation | test-input-or-backend-rule | 命令已真实进入本地/后端校验；若该项仍使用安全负向输入，则失败符合预期；若使用真实 fixture 仍失败，再作为 CLI bug 处理。 |
| 31 | read | `minutes +action-items` | read | real-error | missing-real-minutes-fixture | not-cli-fixable-without-fixture | 当前账号没有满足条件的妙记/听记或录制会话；需准备真实会议产物后复测。 |
| 32 | read | `minutes +latest-minutes` | read | real-error | missing-real-minutes-fixture | not-cli-fixable-without-fixture | 当前账号没有满足条件的妙记/听记或录制会话；需准备真实会议产物后复测。 |
| 33 | read | `minutes +minutes-search` | read | real-error | input-or-business-validation | test-input-or-backend-rule | 命令已真实进入本地/后端校验；若该项仍使用安全负向输入，则失败符合预期；若使用真实 fixture 仍失败，再作为 CLI bug 处理。 |
| 34 | read | `minutes +transcript` | read | real-error | missing-real-minutes-fixture | not-cli-fixable-without-fixture | 当前账号没有满足条件的妙记/听记或录制会话；需准备真实会议产物后复测。 |
| 35 | read | `oa +done-approvals` | read | real-error | input-or-business-validation | test-input-or-backend-rule | 命令已真实进入本地/后端校验；若该项仍使用安全负向输入，则失败符合预期；若使用真实 fixture 仍失败，再作为 CLI bug 处理。 |
| 36 | read | `oa +pending` | read | real-error | input-or-business-validation | test-input-or-backend-rule | 命令已真实进入本地/后端校验；若该项仍使用安全负向输入，则失败符合预期；若使用真实 fixture 仍失败，再作为 CLI bug 处理。 |
| 37 | read | `report +report-latest` | read | real-error | input-or-business-validation | test-input-or-backend-rule | 命令已真实进入本地/后端校验；若该项仍使用安全负向输入，则失败符合预期；若使用真实 fixture 仍失败，再作为 CLI bug 处理。 |
| 38 | read | `todo +due-today` | read | real-error | input-or-business-validation | test-input-or-backend-rule | 命令已真实进入本地/后端校验；若该项仍使用安全负向输入，则失败符合预期；若使用真实 fixture 仍失败，再作为 CLI bug 处理。 |
| 39 | read | `todo +related-tasks` | read | real-error | input-or-business-validation | test-input-or-backend-rule | 命令已真实进入本地/后端校验；若该项仍使用安全负向输入，则失败符合预期；若使用真实 fixture 仍失败，再作为 CLI bug 处理。 |
| 40 | read | `wiki +node-list` | read | real-error | input-or-business-validation | test-input-or-backend-rule | 命令已真实进入本地/后端校验；若该项仍使用安全负向输入，则失败符合预期；若使用真实 fixture 仍失败，再作为 CLI bug 处理。 |
| 41 | read | `wiki +resolve-space` | read | real-error | input-or-business-validation | test-input-or-backend-rule | 命令已真实进入本地/后端校验；若该项仍使用安全负向输入，则失败符合预期；若使用真实 fixture 仍失败，再作为 CLI bug 处理。 |
| 42 | read | `wiki +space-list` | read | real-error | input-or-business-validation | test-input-or-backend-rule | 命令已真实进入本地/后端校验；若该项仍使用安全负向输入，则失败符合预期；若使用真实 fixture 仍失败，再作为 CLI bug 处理。 |
| 43 | write | `aitable +advperm-disable` | high-risk-write | real-error | auth-or-permission | not-cli-fixable | 真实账号、应用 scope 或资源权限不足；CLI 只能如实暴露，不能在本仓库内修复权限。 |
| 44 | write | `aitable +advperm-enable` | write | real-error | auth-or-permission | not-cli-fixable | 真实账号、应用 scope 或资源权限不足；CLI 只能如实暴露，不能在本仓库内修复权限。 |
| 45 | write | `aitable +attachment-upload` | write | real-error | missing-real-aitable-fixture | not-cli-fixable-without-fixture | AI 表格命令需要真实 Base/Table/View/Record 等资源；安全负向 ID 只能验证调用链，不能让后端成功。 |
| 46 | write | `aitable +base-copy` | write | real-error | missing-real-aitable-fixture | not-cli-fixable-without-fixture | AI 表格命令需要真实 Base/Table/View/Record 等资源；安全负向 ID 只能验证调用链，不能让后端成功。 |
| 47 | write | `aitable +base-delete` | high-risk-write | real-error | missing-real-resource | not-cli-fixable-without-fixture | 真实测试使用的资源/单据/消息/群/文档不存在；需要准备对应 fixture 后才能期望成功，不属于 shortcut 参数投影错误。 |
| 48 | write | `aitable +base-update` | write | real-error | missing-real-aitable-fixture | not-cli-fixable-without-fixture | AI 表格命令需要真实 Base/Table/View/Record 等资源；安全负向 ID 只能验证调用链，不能让后端成功。 |
| 49 | write | `aitable +chart-delete` | high-risk-write | real-error | missing-real-aitable-fixture | not-cli-fixable-without-fixture | AI 表格命令需要真实 Base/Table/View/Record 等资源；安全负向 ID 只能验证调用链，不能让后端成功。 |
| 50 | write | `aitable +chart-share-update` | write | real-error | missing-real-aitable-fixture | not-cli-fixable-without-fixture | AI 表格命令需要真实 Base/Table/View/Record 等资源；安全负向 ID 只能验证调用链，不能让后端成功。 |
| 51 | write | `aitable +chart-update` | write | real-error | backend-or-mcp-error | not-cli-fixable-first | 后端/MCP 服务返回内部错误；CLI 无法直接修复，但报告保留 trace/stdout 供服务端排查。 |
| 52 | write | `aitable +dashboard-arrange` | write | real-error | missing-real-aitable-fixture | not-cli-fixable-without-fixture | AI 表格命令需要真实 Base/Table/View/Record 等资源；安全负向 ID 只能验证调用链，不能让后端成功。 |
| 53 | write | `aitable +dashboard-delete` | high-risk-write | real-error | missing-real-aitable-fixture | not-cli-fixable-without-fixture | AI 表格命令需要真实 Base/Table/View/Record 等资源；安全负向 ID 只能验证调用链，不能让后端成功。 |
| 54 | write | `aitable +dashboard-share-update` | write | real-error | missing-real-aitable-fixture | not-cli-fixable-without-fixture | AI 表格命令需要真实 Base/Table/View/Record 等资源；安全负向 ID 只能验证调用链，不能让后端成功。 |
| 55 | write | `aitable +dashboard-update` | write | real-error | missing-real-aitable-fixture | not-cli-fixable-without-fixture | AI 表格命令需要真实 Base/Table/View/Record 等资源；安全负向 ID 只能验证调用链，不能让后端成功。 |
| 56 | write | `aitable +field-delete` | high-risk-write | real-error | backend-or-mcp-error | not-cli-fixable-first | 后端/MCP 服务返回内部错误；CLI 无法直接修复，但报告保留 trace/stdout 供服务端排查。 |
| 57 | write | `aitable +field-update` | write | real-error | backend-or-mcp-error | not-cli-fixable-first | 后端/MCP 服务返回内部错误；CLI 无法直接修复，但报告保留 trace/stdout 供服务端排查。 |
| 58 | write | `aitable +form-delete` | high-risk-write | real-error | missing-real-aitable-fixture | not-cli-fixable-without-fixture | AI 表格命令需要真实 Base/Table/View/Record 等资源；安全负向 ID 只能验证调用链，不能让后端成功。 |
| 59 | write | `aitable +form-field-hide` | write | real-error | missing-real-aitable-fixture | not-cli-fixable-without-fixture | AI 表格命令需要真实 Base/Table/View/Record 等资源；安全负向 ID 只能验证调用链，不能让后端成功。 |
| 60 | write | `aitable +form-field-update` | write | real-error | missing-real-aitable-fixture | not-cli-fixable-without-fixture | AI 表格命令需要真实 Base/Table/View/Record 等资源；安全负向 ID 只能验证调用链，不能让后端成功。 |
| 61 | write | `aitable +form-share-update` | write | real-error | missing-real-aitable-fixture | not-cli-fixable-without-fixture | AI 表格命令需要真实 Base/Table/View/Record 等资源；安全负向 ID 只能验证调用链，不能让后端成功。 |
| 62 | write | `aitable +form-update` | write | real-error | missing-real-aitable-fixture | not-cli-fixable-without-fixture | AI 表格命令需要真实 Base/Table/View/Record 等资源；安全负向 ID 只能验证调用链，不能让后端成功。 |
| 63 | write | `aitable +import-data` | write | real-error | missing-real-resource | not-cli-fixable-without-fixture | 真实测试使用的资源/单据/消息/群/文档不存在；需要准备对应 fixture 后才能期望成功，不属于 shortcut 参数投影错误。 |
| 64 | write | `aitable +import-upload` | write | real-error | missing-real-aitable-fixture | not-cli-fixable-without-fixture | AI 表格命令需要真实 Base/Table/View/Record 等资源；安全负向 ID 只能验证调用链，不能让后端成功。 |
| 65 | write | `aitable +record-delete` | high-risk-write | real-error | backend-or-mcp-error | not-cli-fixable-first | 后端/MCP 服务返回内部错误；CLI 无法直接修复，但报告保留 trace/stdout 供服务端排查。 |
| 66 | write | `aitable +record-primary-doc-create` | write | real-error | missing-real-aitable-fixture | not-cli-fixable-without-fixture | AI 表格命令需要真实 Base/Table/View/Record 等资源；安全负向 ID 只能验证调用链，不能让后端成功。 |
| 67 | write | `aitable +record-update` | write | real-error | backend-or-mcp-error | not-cli-fixable-first | 后端/MCP 服务返回内部错误；CLI 无法直接修复，但报告保留 trace/stdout 供服务端排查。 |
| 68 | write | `aitable +record-upsert` | write | real-error | backend-or-mcp-error | not-cli-fixable-first | 后端/MCP 服务返回内部错误；CLI 无法直接修复，但报告保留 trace/stdout 供服务端排查。 |
| 69 | write | `aitable +role-create` | write | real-error | missing-real-aitable-fixture | not-cli-fixable-without-fixture | AI 表格命令需要真实 Base/Table/View/Record 等资源；安全负向 ID 只能验证调用链，不能让后端成功。 |
| 70 | write | `aitable +role-delete` | high-risk-write | real-error | backend-or-mcp-error | not-cli-fixable-first | 后端/MCP 服务返回内部错误；CLI 无法直接修复，但报告保留 trace/stdout 供服务端排查。 |
| 71 | write | `aitable +role-update` | write | real-error | backend-or-mcp-error | not-cli-fixable-first | 后端/MCP 服务返回内部错误；CLI 无法直接修复，但报告保留 trace/stdout 供服务端排查。 |
| 72 | write | `aitable +section-create` | write | real-error | missing-real-aitable-fixture | not-cli-fixable-without-fixture | AI 表格命令需要真实 Base/Table/View/Record 等资源；安全负向 ID 只能验证调用链，不能让后端成功。 |
| 73 | write | `aitable +section-delete` | high-risk-write | real-error | missing-real-aitable-fixture | not-cli-fixable-without-fixture | AI 表格命令需要真实 Base/Table/View/Record 等资源；安全负向 ID 只能验证调用链，不能让后端成功。 |
| 74 | write | `aitable +section-move-node` | write | real-error | missing-real-aitable-fixture | not-cli-fixable-without-fixture | AI 表格命令需要真实 Base/Table/View/Record 等资源；安全负向 ID 只能验证调用链，不能让后端成功。 |
| 75 | write | `aitable +section-rename` | write | real-error | missing-real-aitable-fixture | not-cli-fixable-without-fixture | AI 表格命令需要真实 Base/Table/View/Record 等资源；安全负向 ID 只能验证调用链，不能让后端成功。 |
| 76 | write | `aitable +section-reorder` | write | real-error | missing-real-aitable-fixture | not-cli-fixable-without-fixture | AI 表格命令需要真实 Base/Table/View/Record 等资源；安全负向 ID 只能验证调用链，不能让后端成功。 |
| 77 | write | `aitable +table-delete` | high-risk-write | real-error | missing-real-aitable-fixture | not-cli-fixable-without-fixture | AI 表格命令需要真实 Base/Table/View/Record 等资源；安全负向 ID 只能验证调用链，不能让后端成功。 |
| 78 | write | `aitable +table-update` | write | real-error | backend-or-mcp-error | not-cli-fixable-first | 后端/MCP 服务返回内部错误；CLI 无法直接修复，但报告保留 trace/stdout 供服务端排查。 |
| 79 | write | `aitable +view-delete` | high-risk-write | real-error | missing-real-aitable-fixture | not-cli-fixable-without-fixture | AI 表格命令需要真实 Base/Table/View/Record 等资源；安全负向 ID 只能验证调用链，不能让后端成功。 |
| 80 | write | `aitable +view-duplicate` | write | real-error | missing-real-aitable-fixture | not-cli-fixable-without-fixture | AI 表格命令需要真实 Base/Table/View/Record 等资源；安全负向 ID 只能验证调用链，不能让后端成功。 |
| 81 | write | `aitable +view-lock` | write | real-error | missing-real-aitable-fixture | not-cli-fixable-without-fixture | AI 表格命令需要真实 Base/Table/View/Record 等资源；安全负向 ID 只能验证调用链，不能让后端成功。 |
| 82 | write | `aitable +view-set-fill-color-rule` | write | real-error | backend-or-mcp-error | not-cli-fixable-first | 后端/MCP 服务返回内部错误；CLI 无法直接修复，但报告保留 trace/stdout 供服务端排查。 |
| 83 | write | `aitable +view-set-frozen-cols` | write | real-error | missing-real-aitable-fixture | not-cli-fixable-without-fixture | AI 表格命令需要真实 Base/Table/View/Record 等资源；安全负向 ID 只能验证调用链，不能让后端成功。 |
| 84 | write | `aitable +view-set-row-height` | write | real-error | missing-real-aitable-fixture | not-cli-fixable-without-fixture | AI 表格命令需要真实 Base/Table/View/Record 等资源；安全负向 ID 只能验证调用链，不能让后端成功。 |
| 85 | write | `aitable +view-update` | write | real-error | missing-real-aitable-fixture | not-cli-fixable-without-fixture | AI 表格命令需要真实 Base/Table/View/Record 等资源；安全负向 ID 只能验证调用链，不能让后端成功。 |
| 86 | write | `aitable +workflow-disable` | high-risk-write | real-error | missing-real-aitable-fixture | not-cli-fixable-without-fixture | AI 表格命令需要真实 Base/Table/View/Record 等资源；安全负向 ID 只能验证调用链，不能让后端成功。 |
| 87 | write | `aitable +workflow-enable` | write | real-error | missing-real-aitable-fixture | not-cli-fixable-without-fixture | AI 表格命令需要真实 Base/Table/View/Record 等资源；安全负向 ID 只能验证调用链，不能让后端成功。 |
| 88 | write | `attendance +boss-check` | write | real-error | input-or-business-validation | test-input-or-backend-rule | 命令已真实进入本地/后端校验；若该项仍使用安全负向输入，则失败符合预期；若使用真实 fixture 仍失败，再作为 CLI bug 处理。 |
| 89 | write | `attendance +create-class` | write | real-error | input-or-business-validation | test-input-or-backend-rule | 命令已真实进入本地/后端校验；若该项仍使用安全负向输入，则失败符合预期；若使用真实 fixture 仍失败，再作为 CLI bug 处理。 |
| 90 | write | `attendance +create-group` | write | real-error | input-or-business-validation | test-input-or-backend-rule | 命令已真实进入本地/后端校验；若该项仍使用安全负向输入，则失败符合预期；若使用真实 fixture 仍失败，再作为 CLI bug 处理。 |
| 91 | write | `attendance +import-schedule` | write | real-error | input-or-business-validation | test-input-or-backend-rule | 命令已真实进入本地/后端校验；若该项仍使用安全负向输入，则失败符合预期；若使用真实 fixture 仍失败，再作为 CLI bug 处理。 |
| 92 | write | `attendance +save-leave-balance` | write | real-error | auth-or-permission | not-cli-fixable | 真实账号、应用 scope 或资源权限不足；CLI 只能如实暴露，不能在本仓库内修复权限。 |
| 93 | write | `attendance +update-class` | write | real-error | input-or-business-validation | test-input-or-backend-rule | 命令已真实进入本地/后端校验；若该项仍使用安全负向输入，则失败符合预期；若使用真实 fixture 仍失败，再作为 CLI bug 处理。 |
| 94 | write | `attendance +update-group` | write | real-error | input-or-business-validation | test-input-or-backend-rule | 命令已真实进入本地/后端校验；若该项仍使用安全负向输入，则失败符合预期；若使用真实 fixture 仍失败，再作为 CLI bug 处理。 |
| 95 | write | `attendance +update-group-members` | write | real-error | input-or-business-validation | test-input-or-backend-rule | 命令已真实进入本地/后端校验；若该项仍使用安全负向输入，则失败符合预期；若使用真实 fixture 仍失败，再作为 CLI bug 处理。 |
| 96 | write | `attendance +update-leave-type` | write | real-error | auth-or-permission | not-cli-fixable | 真实账号、应用 scope 或资源权限不足；CLI 只能如实暴露，不能在本仓库内修复权限。 |
| 97 | write | `calendar +respond-event` | write | real-error | missing-real-resource | not-cli-fixable-without-fixture | 真实测试使用的资源/单据/消息/群/文档不存在；需要准备对应 fixture 后才能期望成功，不属于 shortcut 参数投影错误。 |
| 98 | write | `chat +category-add-conversation` | write | real-error | auth-or-permission | not-cli-fixable | 真实账号、应用 scope 或资源权限不足；CLI 只能如实暴露，不能在本仓库内修复权限。 |
| 99 | write | `chat +category-remove-conversation` | write | real-error | auth-or-permission | not-cli-fixable | 真实账号、应用 scope 或资源权限不足；CLI 只能如实暴露，不能在本仓库内修复权限。 |
| 100 | write | `chat +chat-add-bot` | write | real-error | auth-or-permission | not-cli-fixable | 真实账号、应用 scope 或资源权限不足；CLI 只能如实暴露，不能在本仓库内修复权限。 |
| 101 | write | `chat +chat-audit-join` | write | real-error | backend-or-mcp-error | not-cli-fixable-first | dry-run 已证明 CLI 装配了 applicantUid/inviterUid；真实后端仍报 applicantUid 缺失，优先按 MCP schema/服务端字段映射问题处理。 |
| 102 | write | `chat +chat-mute-member` | write | real-error | backend-or-mcp-error | not-cli-fixable-first | fake MCP 已证明 CLI 已装配会话 ID 字段；真实后端仍报 openConversationId/openCid/cid 缺失，优先按 MCP schema/服务端字段映射问题处理。 |
| 103 | write | `chat +chat-quit` | write | real-error | input-or-business-validation | test-input-or-backend-rule | 命令已真实进入本地/后端校验；若该项仍使用安全负向输入，则失败符合预期；若使用真实 fixture 仍失败，再作为 CLI bug 处理。 |
| 104 | write | `chat +chat-remove-bot` | high-risk-write | real-error | missing-real-resource | not-cli-fixable-without-fixture | 真实测试使用的资源/单据/消息/群/文档不存在；需要准备对应 fixture 后才能期望成功，不属于 shortcut 参数投影错误。 |
| 105 | write | `chat +chat-role-remove` | high-risk-write | real-error | input-or-business-validation | test-input-or-backend-rule | 命令已真实进入本地/后端校验；若该项仍使用安全负向输入，则失败符合预期；若使用真实 fixture 仍失败，再作为 CLI bug 处理。 |
| 106 | write | `chat +chat-role-remove-user` | write | real-error | input-or-business-validation | test-input-or-backend-rule | 命令已真实进入本地/后端校验；若该项仍使用安全负向输入，则失败符合预期；若使用真实 fixture 仍失败，再作为 CLI bug 处理。 |
| 107 | write | `chat +chat-transfer-owner` | write | real-error | backend-or-mcp-error | not-cli-fixable-first | fake MCP 已证明 CLI 已装配会话 ID 字段；真实后端仍报 openConversationId/openCid/cid 缺失，优先按 MCP schema/服务端字段映射问题处理。 |
| 108 | write | `chat +chat-update-icon` | write | real-error | input-or-business-validation | test-input-or-backend-rule | 命令已真实进入本地/后端校验；若该项仍使用安全负向输入，则失败符合预期；若使用真实 fixture 仍失败，再作为 CLI bug 处理。 |
| 109 | write | `chat +chat-update-settings` | write | real-error | input-or-business-validation | test-input-or-backend-rule | 命令已真实进入本地/后端校验；若该项仍使用安全负向输入，则失败符合预期；若使用真实 fixture 仍失败，再作为 CLI bug 处理。 |
| 110 | write | `chat +conversation-clear-messages` | high-risk-write | real-error | backend-or-mcp-error | not-cli-fixable-first | fake MCP 已证明 CLI 已装配会话 ID 字段；真实后端仍报 openConversationId/openCid/cid 缺失，优先按 MCP schema/服务端字段映射问题处理。 |
| 111 | write | `chat +conversation-clear-red-point` | write | real-error | backend-or-mcp-error | not-cli-fixable-first | fake MCP 已证明 CLI 已装配会话 ID 字段；真实后端仍报 openConversationId/openCid/cid 缺失，优先按 MCP schema/服务端字段映射问题处理。 |
| 112 | write | `chat +conversation-hide` | write | real-error | backend-or-mcp-error | not-cli-fixable-first | fake MCP 已证明 CLI 已装配会话 ID 字段；真实后端仍报 openConversationId/openCid/cid 缺失，优先按 MCP schema/服务端字段映射问题处理。 |
| 113 | write | `chat +conversation-mark-read` | write | real-error | missing-real-resource | not-cli-fixable-without-fixture | 真实测试使用的资源/单据/消息/群/文档不存在；需要准备对应 fixture 后才能期望成功，不属于 shortcut 参数投影错误。 |
| 114 | write | `chat +conversation-mark-unread` | write | real-error | backend-or-mcp-error | not-cli-fixable-first | fake MCP 已证明 CLI 已装配会话 ID 字段；真实后端仍报 openConversationId/openCid/cid 缺失，优先按 MCP schema/服务端字段映射问题处理。 |
| 115 | write | `chat +conversation-mute` | write | real-error | backend-or-mcp-error | not-cli-fixable-first | fake MCP 已证明 CLI 已装配会话 ID 字段；真实后端仍报 openConversationId/openCid/cid 缺失，优先按 MCP schema/服务端字段映射问题处理。 |
| 116 | write | `chat +conversation-mute-at-all` | write | real-error | backend-or-mcp-error | not-cli-fixable-first | fake MCP 已证明 CLI 已装配会话 ID 字段；真实后端仍报 openConversationId/openCid/cid 缺失，优先按 MCP schema/服务端字段映射问题处理。 |
| 117 | write | `chat +conversation-mute-red-envelope` | write | real-error | backend-or-mcp-error | not-cli-fixable-first | fake MCP 已证明 CLI 已装配会话 ID 字段；真实后端仍报 openConversationId/openCid/cid 缺失，优先按 MCP schema/服务端字段映射问题处理。 |
| 118 | write | `chat +conversation-set-top` | write | real-error | backend-or-mcp-error | not-cli-fixable-first | fake MCP 已证明 CLI 已装配会话 ID 字段；真实后端仍报 openConversationId/openCid/cid 缺失，优先按 MCP schema/服务端字段映射问题处理。 |
| 119 | write | `chat +messages-add-emoji` | write | real-error | missing-real-resource | not-cli-fixable-without-fixture | 真实测试使用的资源/单据/消息/群/文档不存在；需要准备对应 fixture 后才能期望成功，不属于 shortcut 参数投影错误。 |
| 120 | write | `chat +messages-add-text-emotion` | write | real-error | missing-real-resource | not-cli-fixable-without-fixture | 真实测试使用的资源/单据/消息/群/文档不存在；需要准备对应 fixture 后才能期望成功，不属于 shortcut 参数投影错误。 |
| 121 | write | `chat +messages-batch-recall-by-bot` | write | real-error | missing-real-resource | not-cli-fixable-without-fixture | 真实测试使用的资源/单据/消息/群/文档不存在；需要准备对应 fixture 后才能期望成功，不属于 shortcut 参数投影错误。 |
| 122 | write | `chat +messages-batch-send-by-bot` | write | real-error | missing-real-resource | not-cli-fixable-without-fixture | 真实测试使用的资源/单据/消息/群/文档不存在；需要准备对应 fixture 后才能期望成功，不属于 shortcut 参数投影错误。 |
| 123 | write | `chat +messages-combine-forward` | write | real-error | missing-real-resource | not-cli-fixable-without-fixture | 真实测试使用的资源/单据/消息/群/文档不存在；需要准备对应 fixture 后才能期望成功，不属于 shortcut 参数投影错误。 |
| 124 | write | `chat +messages-create-text-emotion` | write | real-error | input-or-business-validation | test-input-or-backend-rule | 命令已真实进入本地/后端校验；若该项仍使用安全负向输入，则失败符合预期；若使用真实 fixture 仍失败，再作为 CLI bug 处理。 |
| 125 | write | `chat +messages-forward` | write | real-error | missing-real-resource | not-cli-fixable-without-fixture | 真实测试使用的资源/单据/消息/群/文档不存在；需要准备对应 fixture 后才能期望成功，不属于 shortcut 参数投影错误。 |
| 126 | write | `chat +messages-forward-topic` | write | real-error | missing-real-resource | not-cli-fixable-without-fixture | 真实测试使用的资源/单据/消息/群/文档不存在；需要准备对应 fixture 后才能期望成功，不属于 shortcut 参数投影错误。 |
| 127 | write | `chat +messages-recall` | write | real-error | missing-real-resource | not-cli-fixable-without-fixture | 真实测试使用的资源/单据/消息/群/文档不存在；需要准备对应 fixture 后才能期望成功，不属于 shortcut 参数投影错误。 |
| 128 | write | `chat +messages-recall-by-bot` | write | real-error | input-or-business-validation | test-input-or-backend-rule | 命令已真实进入本地/后端校验；若该项仍使用安全负向输入，则失败符合预期；若使用真实 fixture 仍失败，再作为 CLI bug 处理。 |
| 129 | write | `chat +messages-remove-emoji` | write | real-error | missing-real-resource | not-cli-fixable-without-fixture | 真实测试使用的资源/单据/消息/群/文档不存在；需要准备对应 fixture 后才能期望成功，不属于 shortcut 参数投影错误。 |
| 130 | write | `chat +messages-remove-text-emotion` | write | real-error | missing-real-resource | not-cli-fixable-without-fixture | 真实测试使用的资源/单据/消息/群/文档不存在；需要准备对应 fixture 后才能期望成功，不属于 shortcut 参数投影错误。 |
| 131 | write | `chat +messages-send-by-bot` | write | real-error | missing-real-resource | not-cli-fixable-without-fixture | 真实测试使用的资源/单据/消息/群/文档不存在；需要准备对应 fixture 后才能期望成功，不属于 shortcut 参数投影错误。 |
| 132 | write | `chat +messages-send-card` | write | real-error | backend-or-mcp-error | not-cli-fixable-first | dry-run 已证明 CLI 装配了 receiverUid；真实后端仍报 receiverUid/openConversationId 为空，优先按 MCP schema/服务端字段映射问题处理。 |
| 133 | write | `chat +messages-set-pin` | write | real-error | backend-or-mcp-error | not-cli-fixable-first | fake MCP 已证明 CLI 已装配会话 ID 字段；真实后端仍报 openConversationId/openCid/cid 缺失，优先按 MCP schema/服务端字段映射问题处理。 |
| 134 | write | `chat +messages-set-top` | write | real-error | missing-real-resource | not-cli-fixable-without-fixture | 真实测试使用的资源/单据/消息/群/文档不存在；需要准备对应 fixture 后才能期望成功，不属于 shortcut 参数投影错误。 |
| 135 | write | `chat +messages-unset-pin` | write | real-error | backend-or-mcp-error | not-cli-fixable-first | fake MCP 已证明 CLI 已装配会话 ID 字段；真实后端仍报 openConversationId/openCid/cid 缺失，优先按 MCP schema/服务端字段映射问题处理。 |
| 136 | write | `chat +messages-unset-top` | write | real-error | missing-real-resource | not-cli-fixable-without-fixture | 真实测试使用的资源/单据/消息/群/文档不存在；需要准备对应 fixture 后才能期望成功，不属于 shortcut 参数投影错误。 |
| 137 | write | `devapp +event-subscribe` | write | real-error | auth-or-permission | not-cli-fixable | 真实账号、应用 scope 或资源权限不足；CLI 只能如实暴露，不能在本仓库内修复权限。 |
| 138 | write | `devapp +event-unsubscribe` | high-risk-write | real-error | auth-or-permission | not-cli-fixable | 真实账号、应用 scope 或资源权限不足；CLI 只能如实暴露，不能在本仓库内修复权限。 |
| 139 | write | `devapp +permission-add` | write | real-error | auth-or-permission | not-cli-fixable | 真实账号、应用 scope 或资源权限不足；CLI 只能如实暴露，不能在本仓库内修复权限。 |
| 140 | write | `devapp +permission-remove` | high-risk-write | real-error | auth-or-permission | not-cli-fixable | 真实账号、应用 scope 或资源权限不足；CLI 只能如实暴露，不能在本仓库内修复权限。 |
| 141 | write | `devapp +robot-config` | write | real-error | auth-or-permission | not-cli-fixable | 真实账号、应用 scope 或资源权限不足；CLI 只能如实暴露，不能在本仓库内修复权限。 |
| 142 | write | `devapp +robot-disable` | high-risk-write | real-error | auth-or-permission | not-cli-fixable | 真实账号、应用 scope 或资源权限不足；CLI 只能如实暴露，不能在本仓库内修复权限。 |
| 143 | write | `devapp +robot-enable` | write | real-error | auth-or-permission | not-cli-fixable | 真实账号、应用 scope 或资源权限不足；CLI 只能如实暴露，不能在本仓库内修复权限。 |
| 144 | write | `devapp +security-config` | write | real-error | auth-or-permission | not-cli-fixable | 真实账号、应用 scope 或资源权限不足；CLI 只能如实暴露，不能在本仓库内修复权限。 |
| 145 | write | `devapp +version-create` | write | real-error | input-or-business-validation | test-input-or-backend-rule | 命令已真实进入本地/后端校验；若该项仍使用安全负向输入，则失败符合预期；若使用真实 fixture 仍失败，再作为 CLI bug 处理。 |
| 146 | write | `devapp +version-publish` | write | real-error | input-or-business-validation | test-input-or-backend-rule | 命令已真实进入本地/后端校验；若该项仍使用安全负向输入，则失败符合预期；若使用真实 fixture 仍失败，再作为 CLI bug 处理。 |
| 147 | write | `ding +send-by-message` | write | real-error | input-or-business-validation | test-input-or-backend-rule | 命令已真实进入本地/后端校验；若该项仍使用安全负向输入，则失败符合预期；若使用真实 fixture 仍失败，再作为 CLI bug 处理。 |
| 148 | write | `doc +comment-create-inline` | write | real-error | missing-real-resource | not-cli-fixable-without-fixture | 真实测试使用的资源/单据/消息/群/文档不存在；需要准备对应 fixture 后才能期望成功，不属于 shortcut 参数投影错误。 |
| 149 | write | `doc +template-apply` | write | real-error | auth-or-permission | not-cli-fixable | 真实账号、应用 scope 或资源权限不足；CLI 只能如实暴露，不能在本仓库内修复权限。 |
| 150 | write | `minutes +record-pause` | write | real-error | input-or-business-validation | test-input-or-backend-rule | 命令已真实进入本地/后端校验；若该项仍使用安全负向输入，则失败符合预期；若使用真实 fixture 仍失败，再作为 CLI bug 处理。 |
| 151 | write | `minutes +record-resume` | write | real-error | input-or-business-validation | test-input-or-backend-rule | 命令已真实进入本地/后端校验；若该项仍使用安全负向输入，则失败符合预期；若使用真实 fixture 仍失败，再作为 CLI bug 处理。 |
| 152 | write | `minutes +record-stop` | write | real-error | input-or-business-validation | test-input-or-backend-rule | 命令已真实进入本地/后端校验；若该项仍使用安全负向输入，则失败符合预期；若使用真实 fixture 仍失败，再作为 CLI bug 处理。 |
| 153 | write | `oa +approve-by` | write | real-error | missing-real-resource | not-cli-fixable-without-fixture | 真实测试使用的资源/单据/消息/群/文档不存在；需要准备对应 fixture 后才能期望成功，不属于 shortcut 参数投影错误。 |
| 154 | write | `wiki +node-copy` | write | real-error | missing-real-resource | not-cli-fixable-without-fixture | 真实测试使用的资源/单据/消息/群/文档不存在；需要准备对应 fixture 后才能期望成功，不属于 shortcut 参数投影错误。 |
| 155 | write | `wiki +node-move` | write | real-error | missing-real-resource | not-cli-fixable-without-fixture | 真实测试使用的资源/单据/消息/群/文档不存在；需要准备对应 fixture 后才能期望成功，不属于 shortcut 参数投影错误。 |
| 156 | write | `wiki +wiki-new-doc` | write | real-error | missing-real-resource | not-cli-fixable-without-fixture | 真实测试使用的资源/单据/消息/群/文档不存在；需要准备对应 fixture 后才能期望成功，不属于 shortcut 参数投影错误。 |
