# 业务接入委托身份透传：DWS 侧指南

## 1. 当前支持范围

全局参数由根命令统一消费。本次执行作用域持有校验后的不可变身份快照，公共 `runtimeRunner` 在业务请求边界绑定该快照。**经过公共 runner 且满足网关边界的业务请求，无需逐条修改命令即可透传委托身份。** 即使旧 helper 使用 `context.Background()`，runner 仍会补入本次身份。

接入检查应按请求通道分类，而不是为每个产品重复实现委托逻辑：

| 请求通道 | DWS 侧行为 | 业务需要做什么 |
| --- | --- | --- |
| 公共 runner → 四个允许的 HTTPS 内置 MCP 网关 | 使用当前 root 本次执行的身份快照；旧无 context helper 也由 runner 补入 | 核查主请求、辅助请求、分页和重试是否都经过该路径，补真实命令到 HTTP 的测试 |
| 公共 runner → 插件、非允许网关、自定义非网关端点、stdio | 不添加委托身份 | 保持排除，不以业务私有 Header 绕过 |
| `mcp-meta` 发现、使用 scoped token 的登录/换票辅助请求 | 不添加委托身份 | 保持调用类型及 scoped token 标记，不借用普通业务请求路径 |
| 独立 published MCP 客户端、直接 OpenAPI、文件字节上传下载、事件长连接 | 不属于当前公共 runner 透传协议 | 如需委托，先定义该通道的身份与传输契约，再评审公共接入方案并补测试 |

例如，文档下载的元信息查询若走公共 runner，会按网关规则携带身份；随后访问下载 URL 的文件字节请求属于另一通道，不因此获得委托 Header。一个命令可能同时包含两种路径，应分别记录。

本次真实命令回归矩阵包括 AI 表格，以及 `doc read`、`doc +fetch`、`calendar event get`、`todo task get`、`wiki feed list`；还需检查同一 root 串行复用和多个预建 root 串行执行的身份隔离。矩阵说明验证范围，执行结果以实际测试报告为准，不能据此声明所有产品、全部分支或绕行通道均已验收。

本地测试验证 DWS 发出的请求。网关接收、委托人 UID 解析和业务联合权限生效仍需单独联调；通过 DWS 测试不代表服务端授权已上线。

## 2. 复用全局入口

参数和输入组合规则见[委托身份透传协议](delegator-user-id.md)。业务命令无需重复注册 flag，也无需增加对应的叶子 `Contract.Parameters`、MCP arguments 或新的身份缓存。

| 全局 hidden flag | 含义 |
| --- | --- |
| `--delegator-user-id` | 委托人在指定组织内的 User ID |
| `--delegator-corp-id` | 上述 User ID 所属组织，与 User ID 成对提供 |
| `--delegator-open-dingtalk-id` | 委托人的 Open DingTalk ID，可独立提供 |

宿主从可信任务上下文注入这些参数。DWS 在根命令执行入口消费参数并生成本次身份快照，绑定到当前 root 的执行作用域和公共 runner。未提供或无效输入对应空快照，每次执行都重新绑定，不继承上次身份。业务层不读取或缓存委托参数，也不自行创建 runner 身份。

请求 context 已有快照时优先保留，显式空快照也不被覆盖；只有快照缺失才从当前执行作用域补入。重试沿用首次捕获的请求快照。执行完成或错误退出后清理作用域，已创建的请求 context 保持不可变。

输入组合协议保持不变：User ID 与 corpId 成对提供，或仅提供 Open DingTalk ID，也可三项齐全；缺一项配对值、显式空值或其他非法输入均整组省略，不改成保留“看起来有效”的部分。

AI 表格的只读调用示例：

```sh
dws aitable base list --limit 10 --delegator-user-id '<user-id>' --delegator-corp-id '<corp-id>' --format json
dws aitable form list --base-id '<base-id>' --table-id '<table-id>' --delegator-open-dingtalk-id '<open-dingtalk-id>' --format json
dws aitable record query --base-id '<base-id>' --table-id '<table-id>' --all --delegator-user-id '<user-id>' --delegator-corp-id '<corp-id>' --format json
```

示例中的 ID 为占位符；执行真实命令会访问业务服务。离线验证使用下文的模拟 HTTP 测试。

## 3. 核查公共 runner 路径

目标链路为：

```text
真实 CLI 参数
  → 根命令建立本次执行作用域及校验后的身份快照
  → 为当前执行绑定公共 runner
  → 业务 RunE / Shortcut、helper、路由解析、分页、辅助读取、已有重试
  → runtimeRunner 从本次作用域补入身份快照
  → 公共 Header 组装及网关边界检查
  → MCP 网关 HTTP 请求
```

### 3.1 先判断请求是否经过公共 runner

按命令检查普通调用、别名、shortcut、原始命令及可选分支。对每个入口记录主请求和全部辅助请求，例如字段目录查询、资源定位、工作流校验、写后回读、分页和任务轮询。核对最终是否进入当前 root 执行绑定的 `runtimeRunner`，以及目标是否属于允许的网关；仅看到一个 helper 名称不足以判断。

先搜索请求边界和可能绕行的客户端，再追踪到真实命令：

```sh
rg -n 'executeInvocation|CallTool\(|CallMCP|publishedmcp|http\.NewRequest|http\.Client' internal/app internal/helpers internal/shortcut -g '*.go' -g '!**/*_test.go'
```

如果全部业务请求已走公共 runner，委托身份由框架统一处理，业务不需要新增 flag、身份参数或 Header。若有绕行通道，记录具体客户端和请求类型；保持现有排除行为，先定义通道契约再决定是否接入，不能把任意 HTTP 请求接到同一身份逻辑上。

### 3.2 保留取消与超时的 context 规范

业务新增代码仍应使用带 context 的调用入口。该规范用于保留取消、deadline 及其他调用信息，**不是公共 runner 路径透传委托身份的前提**。runner 补入身份不会恢复旧 helper 已丢弃的取消信号或 deadline。

`internal/helpers` 包内已有自动路由入口，修改或新增业务代码时优先使用：

```go
// 保留当前命令的取消、超时和其他调用信息。
return callMCPToolContext(cmd.Context(), "get_document_content", args)
```

需要指定服务或取得中间结果时，使用现有 API：

| 用途 | 现有入口 |
| --- | --- |
| 指定服务并沿用输出行为 | `helpers.CallMCPToolOnServerContext(ctx, server, tool, args)` |
| 取得原始文本结果 | `helpers.CallMCPToolTextOnServerContext(ctx, server, tool, args)` |
| 取得解析后的业务值 | `helpers.CallMCPToolDataOnServer(ctx, server, tool, args)` |
| 明确只读的辅助调用，含已有 dry-run 只读通道 | `helpers.CallMCPReadToolTextOnServerContext(ctx, server, tool, args)` |

包内调用省略 `helpers.` 前缀。选择接口时保留原有输出、错误分类、服务路由和 dry-run 行为；不能把写请求改走只读通道。

多层 helper 应显式增加 `ctx context.Context` 参数，从 `cmd.Context()` 一层层传入。超时从父上下文派生：

```go
ctx, cancel := context.WithTimeout(cmd.Context(), 2*time.Minute)
defer cancel()

text, err := helpers.CallMCPToolTextOnServerContext(ctx, server, tool, args)
```

不得在辅助 helper 中重新读取全局 flag、保存进程级“当前委托人”，或自行拼 Header。公共 runner 使用的是当前 root 本次执行持有的快照，不是进程共享身份。构造 root 时不能固定另一个 root 的调用绑定；每次执行都应使用自己的作用域，保持串行复用隔离。

不要为接入协议而全仓机械替换 Background。进程启动、登录及独立后台任务各有生命周期；业务代码的 context 清理可以按取消和超时需求独立推进。

### 3.3 Shortcut 复用 RuntimeContext

现有 `shortcut.RuntimeContext` 的调用入口会取真实命令上下文：

- 普通工具调用：`rt.CallMCP(tool, args)`。
- 读取并解析中间结果：`rt.CallMCPData(server, tool, args)`。
- 写入并解析结果：`rt.CallMCPWriteData(server, tool, args)`；已有严格回执契约可继续使用 `CallMCPWriteDataStrict`。
- 向其他 helper 传递上下文：使用 `rt.Command().Context()`。

Shortcut 调用旧无上下文 helper 时，只要最终仍到达当前执行的公共 runner，身份仍由 runner 补入；但取消和 deadline 需要业务继续传递 context。绕过 runner 的分支按独立通道检查。仅使用 `+` 命名不能证明请求路径。

### 3.4 分页、重试和辅助请求

- 主服务及辅助服务调用都应到达当前执行的公共 runner；切换服务不能切换到另一次执行的快照。
- 每一页、每次轮询、每次已允许的重试沿用本次作用域。业务仍应传递相同父上下文或派生上下文，使重试等待响应取消，取消后不发送下一次请求。
- 保留现有重试资格和幂等约束。委托失败不得触发“去掉身份后重放”的新分支。
- 已有辅助读取和写后核验也必须携带身份，不能只对最终写请求断言。
- 保持原 dry-run 语义：本地预览不发请求；明确允许的只读解析沿原通道执行并继承身份，不以接入为由新增网络访问。

AI 表格可参考 `internal/helpers/aitable.go` 的 `callAitableToolContext`、`callAitableHelperToolContext` 及其只读重试路径，作为取消、超时和请求生命周期的示例。其他公共 runner 业务无需照抄一套委托专用 caller。

### 3.5 串行复用与并发边界

支持同一 root 串行执行，也支持预先构造多个 root 后交替串行执行；每次都应重新绑定当前 root 的作用域，未提供身份的调用必须使用空快照。Help、解析失败、执行失败之后的下一次调用也不能沿用旧身份。

现有 helpers/auth 含进程共享状态，Cobra 树本身也不支持并发 Execute。**同进程多 root 并发执行不在本次支持范围内**，创建多个 root 并不能使其安全。需要并发 CLI 调用时使用独立进程。不可变身份快照不能作为整个运行时并发安全的证明。

## 4. 保持统一传输和身份边界

Header 的唯一组装位置仍是 `runtimeRunner` 调用的 `applyDelegatorHeaders`（`internal/app/delegator.go`）；业务层不得自行拼 Header 或绕过公共 transport。

- 仅允许 HTTPS 主机 `mcp-gw.dingtalk.com`、`pre-mcp-gw.dingtalk.com`、`mcp-gw.dingtalk.io`、`pre-mcp-gw.dingtalk.io`，端口为默认或 443；匹配主机仍需同时满足业务请求类型和身份来源边界。
- 插件、第三方及自定义非网关端点、stdio、`mcp-meta` 发现和 scoped token 登录/换票请求保持排除。独立 published MCP、直接 OpenAPI、文件字节传输和事件长连接没有因全局参数而接入本协议。
- 跨源重定向清理全部委托 Header，离开原始源后再返回也不恢复。同源重定向沿用公共 transport 行为。
- Actor 的 token、profile 和已有业务参数保持原语义。委托人的 corpId 不用于切换数字员工的登录组织。
- DWS 不解析 UID，不发出 `delegator-uid`，不把三项身份塞入 MCP `arguments` 或 `DWS_AGENT_EXT`。网关负责可信解析及向业务传递 UID。
- 保留大小写不敏感的保留字段清理和日志脱敏；Header 数量不是身份透传证据。
- `--principal-user-id` 仍走旧文档域流程。非空旧参数与任一显式新参数同时出现时，保持 `conflicting_delegation_protocols` 校验错误；业务不得改成别名或自行选择一方。

## 5. 从真实命令验证到 HTTP Header

参考 `internal/app/delegator_aitable_test.go` 中的 `TestCrossPlatformCoverageDelegatorAITableRealCommands`。测试需覆盖业务调用链和 root 执行绑定，不能只自行创建一个 fixture 命令后直接调用 runner。

代表性回归矩阵如下；逐条记录实际结果与未覆盖分支，不把“存在测试用例”记作通过：

| 验证对象 | 应证明的行为 |
| --- | --- |
| AI 表格真实命令 | 主服务、辅助服务、记录分页、视图更新、工作流发布等请求使用本次身份 |
| `doc read`、`doc +fetch` | 旧无 context helper 与 Shortcut 两条入口均经过当前 runner，到 HTTP 时身份一致 |
| `calendar event get`、`todo task get`、`wiki feed list` | 多产品的真实入口复用公共绑定，无需逐业务拼 Header |
| 同一 root 连续执行 | 带身份后再不带身份，后一次请求不继承前一次身份；失败、Help 和解析失败后也不串值 |
| 多个预建 root 交替串行执行 | 每次使用正在执行 root 的快照，未提供身份的 root 保持空快照 |
| 边界和取消 | 排除通道不带 Header；绑定保留调用 context 已有的取消/deadline；重定向和重试沿用既有协议 |

推荐测试结构：

1. 在 `internal/app` 测试中通过 `NewRootCommand()` 构造已准备好的真实命令树，使用真实业务 argv；需要扩展树时使用组装回调，不在工厂返回后替换命令执行钩子。
2. 用 `testseam.Swap` 注入假 Actor token 和必要的端点测试 seam。若包装 runner，仅固定服务发现/端点，并保留生产 root 到 runner 的执行作用域绑定；不能绕过该绑定后在测试中手工塞入身份，也不替换业务 helper、Header 组装或 HTTP 请求构建。
3. 用自定义 `http.RoundTripper` 截获真实 transport 构造的请求并返回模拟 MCP 响应，全程不访问线上。请求逻辑端点使用协议允许的网关域名，以便实际执行 Header 边界判断。
4. 用真实响应形状驱动各分支，记录每次请求的 MCP 服务、工具、参数和 Header。分页返回至少两页；辅助读取、写后回读和重试需要实际触发，不能只看首个请求。
5. 对每次请求断言三个委托 Header 的具体值、Actor token 未变化、`delegator-uid` 缺失，且业务 arguments 未混入委托身份。
6. 同一命令树执行一次带身份、一次不带身份，确认后者不继承；预先构造多个 root，再交替串行执行并检查各自身份。公共输入组合、无效值、冲突、Help、解析/执行失败、取消、认证重试及重定向边界继续由对应回归测试验证。

所有假 ID 和 token 使用固定测试值；不复制真实凭据。用 package seam 的测试不要 `t.Parallel`。测试名称沿用 `TestCrossPlatformCoverage*`。单独构造 Cobra 树的 helper 测试通过 `corecmd.*ForTest` 执行；app 工厂构造的树直接执行即可。

跨业务真实 HTTP 回归与多 root 隔离见 `internal/app/delegator_business_test.go`；执行清理、快照不可变、认证重试及只读 runner 克隆见 `internal/app/delegator_runner_test.go`。AI 表格重试和取消可参考 `internal/helpers/aitable_context_test.go` 的 `TestCrossPlatformCoverageAitableRetryPreservesContext`。公共边界分别见 `internal/app/delegator_user_id_test.go`、`internal/transport/delegator_test.go` 和 `internal/requestmeta` 下的测试。测试报告应注明哪些路径走到了 HTTP，哪些只是 helper 级上下文断言。

本地在工作树根目录运行相关测试的示例：

```sh
mkdir -p .worktrees/delegator-validation/go-work .worktrees/delegator-validation/test-work
GOTMPDIR="$PWD/.worktrees/delegator-validation/go-work" \
TMPDIR="$PWD/.worktrees/delegator-validation/test-work" \
DWS_PACKAGE_VERSION=0.0.0-test \
go test ./internal/app ./internal/helpers ./internal/transport ./internal/requestmeta \
  -run '^(TestCrossPlatformCoverageDelegator|TestCrossPlatformCoverageAitable(RetryPreservesContext|CommandContextChains))' -count=1
```

这条命令按名称选择公共委托测试及 AI 表格 context 样例；执行前核对新增用例是否被该模式选中。它不代表所有产品的完整验收。新增请求通道或修改业务入口时添加相应真实命令用例，并按改动范围运行业务原有测试和仓库要求的门禁。

## 6. 业务核查与绕行通道接入记录

业务 PR 中逐项记录以下结果。已经走公共 runner 的路径记录为复用公共能力；绕行通道在独立契约明确前继续保持排除。不要把所有业务都列为必须改写 context 才能接入，也不要用一个入口的通过结果替代全产品验证：

- [ ] 命令、别名及可选分支已列全，主请求和辅助请求的服务/工具清单可核对。
- [ ] 每种请求是否经过当前执行绑定的公共 runner 已核查；绕行客户端、排除原因及需要另行定义的契约已记录。
- [ ] 公共 runner 路径无需新增业务私有 flag、身份缓存或 Header；分页、辅助读取、现有重试均使用当前执行的快照。
- [ ] 新增业务代码从真实 `cmd.Context()` 派生上下文以保留取消和 deadline；旧路径的取消缺口单独记录，不与身份绑定能力混为一谈。
- [ ] 真实 CLI 到 HTTP 测试检查每次请求的身份值、Actor token 和业务参数，无需真实账号或线上写入。
- [ ] 同一 root 连续执行及多个预建 root 串行执行不串身份；未以多 root 测试宣称同进程并发安全。
- [ ] dry-run、错误分类、业务输出和原有重试规则保持既有契约。
- [ ] 仍复用统一网关、插件、换票、发现、重定向和日志边界；未新增业务私有 Header 入口。
- [ ] 当前版本的测试命令、结果及未覆盖路径已记录；未把代表性回归矩阵写成所有业务的验收结果。
- [ ] 服务端联调状态单独列出；DWS 测试通过未被表述为网关或业务鉴权已生效。
