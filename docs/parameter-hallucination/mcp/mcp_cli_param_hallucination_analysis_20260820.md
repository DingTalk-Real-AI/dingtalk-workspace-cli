# MCP 产品 CLI 参数幻觉分析

## 结论摘要

本分析以线上 `origin/main` 冻结提交
`aa4ae9a90323aa97e5cebdb5045b129f0f14e0df` 为唯一基线，使用该提交重新构建的
`dws`、运行时 Schema、官方 Cobra 树、MCP URL 实现、测试与冻结正式
`internal/cli/param_concepts.json`。仓库和现有钉钉技能包没有 MCP URL 产品专属 Skill，
因此未拿其他产品中的 `interface_mode=mcp` 作为本产品参数事实。未使用固定 Catalog、历史
badcase、用户 Shortcut 或已安装插件，也没有修改当前工作区正式别名表。

MCP 只有 1 个 Agent 可见叶 `mcp url get <mcpId>`。它接收恰好 1 个必填位置参数，trim 后以
字符串原样传给 helper-only `mcp-meta/get_mcp_server_url`，没有业务 flag。返回的 `mcpURL`、
`mcpJSON`、transport、header 或 token 可能携带身份凭据，只是敏感输出，不能反向成为输入。

候选不新增 alias 或 concept，只增加 1 个 `scope_strict` 纯保护 override 和 16 个 guard
fixture。生成结果为 0 alias、31 blocked、11 ambiguous。第一次生成审计发现隐藏真实全局
`--token` 与初稿 block 冲突，已从规则删除并加入冲突检查；最终所有保护均不覆盖真实业务、
隐藏或全局 flag。候选已通过 canonical/opaque-string/global 原生行为、空值校验、16 组
dispatch 前保护、非目标结构恒等、MCP 专项测试、`internal/cli`、`internal/pipeline`、generated
drift、Schema Catalog 政策与完整 `internal/app`（250.336 秒）。候选可以进入正式
fail-closed 规则落地评审。

## 参数问题

### 1. 唯一业务参数是位置参数，不能由 flag 别名补造

Help 为 `mcp url get <mcpId>`，Schema 声明 `mcp_id` 为 index 0、required string，
`parameters={}`。实现只读取 `args[0]`，没有 `--mcp-id`、`--server-id` 或 `--app-id`。
当前参数字典只归一化 flag 名，不能安全地把 `--mcp-id 2480` 改写为位置参数 `2480`；若这样
做会越过 Cobra 的位置参数语法和数量校验。

候选 block 明确的 flag 化 ID 与错误对象 ID，裸 `id/server/service/app/tool/plugin` ambiguous。
正确调用仍须显式写成 `dws mcp url get 2480`。

### 2. `mcpId` 是非空不透明字符串，不能猜数值类型或其他 ID 值域

示例使用数字，但 Schema 类型和实现均为 string；运行时只 trim 并拒绝空串，没有数值解析、
格式转换或自动查询市场的逻辑。`abc-001` 在 dry-run 中原样进入 `mcpId`，空白值在 dispatch
前返回“mcpId 不能为空”。

参数字典不能把 server/service/app/plugin/tool ID 当成 MCP 市场 ID，也不能替用户发现、搜索
或修复 ID。候选只保护名称角色，不修改值。

### 3. 敏感连接结果不能反向成为请求参数

命令返回的 `mcpURL` 和 `mcpJSON` 可能包含连接 URL、transport、headers、token 或其他凭据。
这些字段描述输出或客户端连接配置，不是 `mcp url get` 的输入。把 `--endpoint`、`--mcp-url`、
`--transport`、`--access-token` 等传入会伪造配置能力，并可能诱导 Agent 传播敏感信息。

候选 block 精确的输出/凭据字段，裸 `key/type/name` ambiguous。它不读取、记录或传播真实凭据；
行为验证只使用 dry-run。

### 4. 精确 get 命令不是搜索、列表或身份选择接口

冻结公开面没有 query、keyword、page、limit、cursor、sort，也不接受 corp-id、org-id 或
user-id。当前身份来自登录上下文，`--profile` 是真实全局组织/账号选择；不能把业务 ID flag
映射到 profile。`--format/--fields/--jq` 只控制输出投影，也不是 MCP 服务参数。

候选 block 搜索、分页和显式身份字段，保留 profile/output/timeout/mock/dry-run/OAuth override
等真实全局参数原生。

## 当前别名表可以实施的方案

1. 为 `mcp url get` 增加一个不含 aliases 的 `scope_strict` override。
2. block flag 化 mcpId、错误对象 ID、返回连接配置、凭据、搜索分页和显式身份字段。
3. 对裸 id/server/service/app/tool/plugin/name/type/key/user/org 提示歧义。
4. 保持 token、client-id、client-secret、format、fields、jq、profile、timeout、mock、dry-run 等
   真实全局参数原生。
5. 用 guard fixture 锁定“1 个位置参数、0 个业务 flag”的事实；未来新增真实 flag 时由生成冲突
   强制重新评审。

## 当前能力支持不了的事项

- 把 `--mcp-id` 自动改写成位置参数；
- 从 server/app/plugin/tool ID 推断或转换 MCP 市场 mcpId；
- 搜索、列出或发现 MCP 市场条目；
- 校验 mcpId 是否真实存在或自动纠正格式；
- 用 `--endpoint`、`--transport`、`--headers` 配置任意 MCP 客户端；
- 把返回 URL、token、header 或 credential 当成请求输入；
- 查询其他用户或组织的连接地址，而不显式切换受支持的 profile；
- 将敏感连接地址发送到聊天、文档、邮件、日志或仓库；
- 用别名表增加连接测试、安装、启停或调用 MCP 工具的能力；
- 从 `interface_mode=mcp` 的其他产品命令扩张本产品命令面。

## 第一轮改造建议

第一轮只落地纯保护 override，不新增自动 alias。它能在 dispatch 前拦截最常见的“位置参数
flag 化”“错误 ID 值域”“返回字段当输入”和“把 get 当搜索/配置命令”。若未来需要 flag 形式
mcpId、市场搜索或连接配置，应先新增真实 Cobra/Contract/Runtime 能力并明确敏感输出政策，再
重新分析；不能提前在参数字典中预埋假参数。

## 候选 `param_concepts.json` 改动与审核

- 新增 concept 0、自动 alias 0；
- 新增 1 个 `mcp url get` scope_strict override；
- 新增 16 个 fixture，全部为 block/ambiguous，active 为 0；
- `go generate ./internal/cli` 从 569 个命令作用域变为 570 个；
- MCP entry 生成 31 blocked、11 ambiguous，command path fallback 仍为 34；
- 初稿发现 `--token` 为隐藏真实全局 flag，已删除冲突规则；最终真实 flag 冲突 0；
- 删除 MCP 改动后，非目标 concept、override、fixture 与冻结正式表结构恒等；
- 规则没有把位置参数、全局身份、OAuth override、输出投影或敏感返回字段混为一类；
- 生成规模与 1 个单位置参数叶相符，没有扩散到其他产品或 `interface_mode=mcp` 命令。

审核结论：候选为纯 fail-closed 治理，不改变任何成功输入、位置参数值或 payload；可进入正式
落地评审。候选位置：`docs/parameter-hallucination/mcp/param_concepts.json`。

## 验证结果与正式替换前置条件

| 验证项 | 结果 | 说明 |
|---|---|---|
| JSON 解析与生成器 | 通过 | 570 个命令作用域；MCP 0 alias、31 blocked、11 ambiguous |
| canonical/位置参数 | 通过 | `10043` 原样进入 `mcpId`，tool 为 `get_mcp_server_url` |
| 值域与空值 | 通过 | `abc-001` 原样保留；空白位置参数在 dispatch 前拒绝 |
| 全局参数原生行为 | 通过 | format/fields/dry-run 正常；隐藏 `--token` 未被覆盖 |
| block/ambiguous | 通过 | 16 组代表幻觉全部在 MCP dispatch 前停止 |
| alias/canonical | 不适用 | 审核结论为 0 自动 alias，位置参数不由 flag 字典改写 |
| 非目标回归 | 通过 | JSON 结构恒等；generated diff 只新增 MCP entry；fallback 无变化 |
| MCP 专项测试 | 通过 | `TestMCPURL*` 通过，0.919 秒 |
| `internal/cli`、`internal/pipeline` | 通过 | CLI 85.471 秒；pipeline 0.446 秒 |
| generated drift | 通过 | 参数别名与 Schema 双次装配确定 |
| Schema Catalog 政策 | 通过 | 28 产品、1166 工具；Runtime confirmation truth 通过 |
| 完整 `internal/app` | 通过 | 250.336 秒；纯 guard fixture 不要求新增 complete-command 模板 |

正式替换不需要新增 complete-command payload 模板，因为候选没有 active alias fixture；完整应用
与仓库政策已全绿。未来若新增 MCP 业务 flag、修改位置参数语法或公开连接配置能力，必须重新
审核 guard 冲突和敏感信息边界。

## 分析依据

- 冻结提交：`aa4ae9a90323aa97e5cebdb5045b129f0f14e0df`，提交时间
  2026-08-20 10:53:27 +08:00；
- 正式表 SHA-256：
  `e41e7908c26dfdcc23636d27661d705416c001d67a6d0d5d658b1ca4bcc815c1`；
- 候选 SHA-256：
  `81a99ee95f4b4da06752b13d813fa2c555df2510fcb4239e772c69a947754482`；
- 命令实现：`internal/app/mcp_url_command.go`；
- 测试：`internal/app/mcp_url_command_test.go`；
- Skill：无 MCP URL 产品专属 Skill；其他产品的 MCP 接口说明不作为本产品参数事实；
- Schema：同一冻结二进制运行时声明组装，`parameters={}`，1 个 required string positional；
- 接口边界：composite helper-only `mcp-meta/get_mcp_server_url`；
- 官方树边界：`mcp → url → get`，1 个 Agent 可见可执行叶；
- 行为调用：仅 dry-run；没有获取、展示或传播任何真实 MCP 凭据 URL；
- 明确未使用：固定 Catalog、历史 badcase、评测工作簿、用户 Shortcut、已安装插件。

## 可复用分析流程

对“位置参数 + 零业务 flag”的命令，先把位置语法、值类型、空值校验和全局 flag 分开，再区分
请求输入与敏感返回字段；只有真实 flag canonical 存在时才考虑 alias，不能用 flag 字典越过
Cobra 位置参数契约。最后用不透明字符串 dry-run、空值校验、dispatch 前 guard、隐藏 flag 冲突
审计、完整应用测试和仓库政策证明保护规则不会改变成功路径。
