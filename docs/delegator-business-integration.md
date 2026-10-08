# 业务接入委托身份透传：DWS 侧指南

## 1. 当前支持范围

本 PR 先完成 AI 表格（`aitable`）的调用链修复与 DWS 侧回归验证。全局参数已存在，其他产品需完成本指南的调用链核查和真实命令测试，才能声明完成接入。

| 产品 | 本次接入状态 | 后续工作 |
| --- | --- | --- |
| AI 表格 `aitable` | 本次修复和验证范围：主服务、辅助服务、记录分页、视图更新、工作流发布及重试的上下文传递 | 新增或修改入口时持续补充真实命令测试 |
| 在线文档 `doc` | 尚未完成产品级接入；`doc read` 的普通、JSONML 和 scope 分支仍有丢弃上下文的旧入口 | 迁移旧 helper，覆盖各读取模式、辅助读取和写入续调；单个 `doc +fetch` 路径保留上下文不代表整个产品已接入 |
| 原生 Markdown / HTML | `markdown fetch`、`html fetch` 共用的文本读取流程仍从 Background 创建上下文 | 迁移主请求及路由探测、下载信息查询等辅助请求 |
| 钉盘 `drive`、知识库 `wiki`、电子表格 `sheet` | 未完成本协议的产品级迁移与验证 | 分别列出全部命令入口及跨产品调用，逐项接入 |
| 消息、日历、待办及其他业务 | 未完成本协议的产品级迁移与验证 | 按下文核查各自调用链并补充证据 |

此表记录接入和验证范围，DWS 没有据此增加产品放行名单。其他产品的某条现有路径可能已经能透传，但不能用一次成功调用推导该产品的所有命令均受支持。每个产品的后续 PR 应更新本表，列明命令、调用路径、测试和剩余缺口。

本地测试验证 DWS 发出的请求。网关接收、委托人 UID 解析和业务联合权限生效仍需单独联调；通过 DWS 测试不代表服务端授权已上线。

## 2. 复用全局入口

参数和输入组合规则见[委托身份透传协议](delegator-user-id.md)。业务命令无需重复注册 flag，也无需增加对应的叶子 `Contract.Parameters`、MCP arguments 或新的身份缓存。

| 全局 hidden flag | 含义 |
| --- | --- |
| `--delegator-user-id` | 委托人在指定组织内的 User ID |
| `--delegator-corp-id` | 上述 User ID 所属组织，与 User ID 成对提供 |
| `--delegator-open-dingtalk-id` | 委托人的 Open DingTalk ID，可独立提供 |

宿主从可信任务上下文注入这些参数。DWS 在根命令执行入口生成本次调用的身份快照，并写入叶子命令的 `cmd.Context()`。业务只负责把该上下文完整传递到公共调用入口。

AI 表格的只读调用示例：

```sh
dws aitable base list --limit 10 --delegator-user-id '<user-id>' --delegator-corp-id '<corp-id>' --format json
dws aitable form list --base-id '<base-id>' --table-id '<table-id>' --delegator-open-dingtalk-id '<open-dingtalk-id>' --format json
dws aitable record query --base-id '<base-id>' --table-id '<table-id>' --all --delegator-user-id '<user-id>' --delegator-corp-id '<corp-id>' --format json
```

示例中的 ID 为占位符；执行真实命令会访问业务服务。离线验证使用下文的模拟 HTTP 测试。

## 3. 让上下文贯穿完整调用链

目标链路为：

```text
真实 CLI 参数
  → 根命令创建身份快照
  → 业务 RunE / Shortcut 的 cmd.Context()
  → 业务 helper、路由解析、分页、辅助读取、允许的重试
  → ToolCaller / runtimeRunner
  → 公共 Header 组装
  → MCP 网关 HTTP 请求
```

### 3.1 先列出业务入口和所有请求

按命令检查普通调用、别名、shortcut、原始命令及可选分支。对每个入口记录主请求和全部辅助请求，例如字段目录查询、资源定位、工作流校验、写后回读、分页和任务轮询。

代码搜索可从以下模式开始；搜索结果需要逐个判断是否在业务执行链内：

```sh
rg -n 'context\.(Background|TODO)\(|callMCPTool\(|callMCPToolOnServer\(|CallMCPToolOnServer\(|CallMCPToolTextOnServer\(|CallMCPReadToolTextOnServer\(' internal/helpers internal/shortcut -g '*.go' -g '!**/*_test.go'
```

不要仅替换首个请求，也不要全仓机械替换 Background：进程启动、登录或独立后台任务有各自的生命周期和透传边界。

### 3.2 普通命令改用带 context 的 helper

`internal/helpers` 包内已有自动路由入口：

```go
// 旧入口内部使用 Background，会丢失当前命令的身份。
return callMCPTool("get_document_content", args)

// 改为传递真实叶子命令的上下文。
return callMCPToolContext(cmd.Context(), "get_document_content", args)
```

需要指定服务或取得中间结果时，使用现有 API：

| 用途 | 现有入口 |
| --- | --- |
| 指定服务并沿用输出行为 | `helpers.CallMCPToolOnServerContext(ctx, server, tool, args)` |
| 取得原始文本结果 | `helpers.CallMCPToolTextOnServerContext(ctx, server, tool, args)` |
| 取得解析后的业务值 | `helpers.CallMCPToolDataOnServer(ctx, server, tool, args)` |
| 明确只读的辅助调用，含已有 dry-run 只读通道 | `helpers.CallMCPReadToolTextOnServerContext(ctx, server, tool, args)` |

包内调用省略 `helpers.` 前缀。选择接口时保留原有输出、错误分类、服务路由和 dry-run 行为；为透传身份不能把写请求改走只读通道。

多层 helper 应显式增加 `ctx context.Context` 参数，从 `cmd.Context()` 一层层传入。超时从父上下文派生：

```go
ctx, cancel := context.WithTimeout(cmd.Context(), 2*time.Minute)
defer cancel()

text, err := helpers.CallMCPToolTextOnServerContext(ctx, server, tool, args)
```

不得在辅助 helper 中重新读取全局 flag、保存全局“当前委托人”，或在 runner 中用共享状态补回已经丢失的 context。这些做法会破坏复用命令树、并发请求及身份隔离。

### 3.3 Shortcut 复用 RuntimeContext

现有 `shortcut.RuntimeContext` 的调用入口会取真实命令上下文：

- 普通工具调用：`rt.CallMCP(tool, args)`。
- 读取并解析中间结果：`rt.CallMCPData(server, tool, args)`。
- 写入并解析结果：`rt.CallMCPWriteData(server, tool, args)`；已有严格回执契约可继续使用 `CallMCPWriteDataStrict`。
- 向其他 helper 传递上下文：使用 `rt.Command().Context()`。

遇到 shortcut 自己创建 Background、绕过 RuntimeContext 或调用旧无上下文 helper 的分支，仍需修复。仅使用 `+` 命名不能证明该命令已经接入。

### 3.4 分页、重试和辅助请求

- 主服务及辅助服务调用都使用同一父上下文；切换服务不能换身份。
- 每一页、每次轮询、每次已允许的重试均传递该上下文或其派生上下文。重试等待应响应取消；取消后不得继续发送下一次请求。
- 保留现有重试资格和幂等约束。委托失败不得触发“去掉身份后重放”的新分支。
- 已有辅助读取和写后核验也必须携带身份，不能只对最终写请求断言。
- 保持原 dry-run 语义：本地预览不发请求；明确允许的只读解析沿原通道执行并继承身份，不以接入为由新增网络访问。

AI 表格可参考 `internal/helpers/aitable.go` 的 `callAitableToolContext`、`callAitableHelperToolContext` 及其只读重试路径。业务如已采用标准带 context 入口，无需增加一套委托专用 caller。

## 4. 保持统一传输和身份边界

Header 的唯一组装位置仍是 `runtimeRunner` 调用的 `applyDelegatorHeaders`（`internal/app/delegator.go`）；业务层不得自行拼 Header 或绕过公共 transport。

- 只向已识别的 HTTPS MCP 网关业务请求透传；插件、第三方及自定义非网关端点、stdio、直接 OpenAPI、登录换票和发现请求遵循现有排除规则。
- 跨源重定向清理全部委托 Header，离开原始源后再返回也不恢复。同源重定向沿用公共 transport 行为。
- Actor 的 token、profile 和已有业务参数保持原语义。委托人的 corpId 不用于切换数字员工的登录组织。
- DWS 不解析 UID，不发出 `delegator-uid`，不把三项身份塞入 MCP `arguments` 或 `DWS_AGENT_EXT`。网关负责可信解析及向业务传递 UID。
- 保留大小写不敏感的保留字段清理和日志脱敏；Header 数量不是身份透传证据。
- `--principal-user-id` 仍走旧文档域流程。非空旧参数与任一显式新参数同时出现时，保持 `conflicting_delegation_protocols` 校验错误；业务不得改成别名或自行选择一方。

## 5. 从真实命令验证到 HTTP Header

参考 `internal/app/delegator_aitable_test.go` 中的 `TestCrossPlatformCoverageDelegatorAITableRealCommands`。测试需覆盖业务调用链，不能只自行创建一个 fixture 命令后直接调用 runner。

推荐测试结构：

1. 在 `internal/app` 测试中通过 `NewRootCommand()` 构造已准备好的真实命令树，使用真实业务 argv；需要扩展树时使用组装回调，不在工厂返回后替换命令执行钩子。
2. 用 `testseam.Swap` 注入假 Actor token 和测试 runner。只固定服务发现/端点，不替换待验证的业务 helper、Header 组装或 HTTP 请求构建。
3. 用自定义 `http.RoundTripper` 截获真实 transport 构造的请求并返回模拟 MCP 响应，全程不访问线上。请求逻辑端点使用协议允许的网关域名，以便实际执行 Header 边界判断。
4. 用真实响应形状驱动各分支，记录每次请求的 MCP 服务、工具、参数和 Header。分页返回至少两页；辅助读取、写后回读和重试需要实际触发，不能只看首个请求。
5. 对每次请求断言三个委托 Header 的具体值、Actor token 未变化、`delegator-uid` 缺失，且业务 arguments 未混入委托身份。
6. 同一命令树执行一次带身份、一次不带身份，确认后者不继承。公共输入组合、无效值、冲突、取消、认证重试及重定向边界继续由对应回归测试验证。

所有假 ID 和 token 使用固定测试值；不复制真实凭据。用 package seam 的测试不要 `t.Parallel`。测试名称沿用 `TestCrossPlatformCoverage*`。单独构造 Cobra 树的 helper 测试通过 `corecmd.*ForTest` 执行；app 工厂构造的树直接执行即可。

AI 表格重试和取消可参考 `internal/helpers/aitable_context_test.go` 的 `TestCrossPlatformCoverageAitableRetryPreservesContext`。公共边界分别见 `internal/app/delegator_user_id_test.go`、`internal/transport/delegator_test.go` 和 `internal/requestmeta` 下的测试。测试报告应注明哪些路径走到了 HTTP，哪些只是 helper 级上下文断言。

本地在工作树根目录运行相关测试的示例：

```sh
mkdir -p .worktrees/delegator-validation/go-work .worktrees/delegator-validation/test-work
GOTMPDIR="$PWD/.worktrees/delegator-validation/go-work" \
TMPDIR="$PWD/.worktrees/delegator-validation/test-work" \
DWS_PACKAGE_VERSION=0.0.0-test \
go test ./internal/app ./internal/helpers ./internal/transport ./internal/requestmeta \
  -run '^(TestCrossPlatformCoverageDelegator|TestCrossPlatformCoverageAitable(RetryPreservesContext|CommandContextChains))' -count=1
```

这条命令覆盖已有公共边界及 AI 表格样例，不是其他业务的完整验收。接入新产品时添加其真实命令用例，并按改动范围运行该产品的原有测试和仓库要求的门禁。

## 6. 每个产品的接入验收记录

业务 PR 中逐项记录以下结果，尚未完成的项目保留为待接入，不声明全产品支持：

- [ ] 命令、别名及可选分支已列全，主请求和辅助请求的服务/工具清单可核对。
- [ ] 从真实 `cmd.Context()` 到所有业务请求的链路连续；分页、重试和取消分支已覆盖。
- [ ] 真实 CLI 到 HTTP 测试检查每次请求的身份值、Actor token 和业务参数，无需真实账号或线上写入。
- [ ] 连续执行不串身份；dry-run、错误分类、业务输出和原有重试规则保持既有契约。
- [ ] 仍复用统一网关、插件、换票、发现、重定向和日志边界；未新增业务私有 Header 入口。
- [ ] 当前版本的测试命令、结果及未覆盖路径已记录，本页产品状态同步更新。
- [ ] 服务端联调状态单独列出；DWS 测试通过未被表述为网关或业务鉴权已生效。
