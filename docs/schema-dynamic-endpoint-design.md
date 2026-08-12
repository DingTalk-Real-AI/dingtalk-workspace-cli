# DWS Agent Schema 统一方案

## 1. 核心定义

DWS Schema 是当前二进制公开 CLI 的版本化 Agent 执行契约。它描述真实 Cobra 命令，并补充 Agent 选择、参数映射、组合约束、安全确认和接口事实。

设计遵循三条硬规则：

1. **Schema 描述 CLI，不制造 CLI。** `CommandRegistry`、ProductDecl / leaf `Contract`、metadata 和 Catalog 都不能凭空创建 Cobra 命令或 flag；registry 中的每个路径都必须精确绑定真实 runnable Cobra leaf。interface 事实由 leaf `Contract` / `ParamDecl` 声明（`schema_mcp_metadata` 已退役），**不得**从 MCP meta 生成 CLI flag（见 §4.1 同源决策）。
2. **所有来源只解析一次。** 来源经过统一 resolver 进入 typed `SchemaRegistry`，所有查询、导出和门禁都消费同一个 `SchemaRegistry/SchemaIndex`。
3. **Collector-first，Catalog 只出不进。** identity collector（`CollectIdentitySpecs`，遍历携带 `ContractFinal.Identity` 的 live Cobra leaves）是稳定 command identity/navigation 的唯一事实源；reviewed `schema_command_registry/` 已退役，identity 由声明（Contract）即代码提供，不再有独立的 reviewed identity 文件。production 通过 `RegisterSchemaSourceRoot` → `ResolveSchemaBuild` 组装 `SchemaRegistry`，并从它投影 ToolSpec wire 与 `ResolveMeta`。`cmd_schema_catalog` 只能生成 CI/local dump，`internal/cli/schema_catalog/`、`schema_meta_index.gob` 和 `schema_meta_index.json` 不得提交或成为运行时来源。`schema_agent_metadata/` / `schema_hints/` / `schema_command_registry/` 已退役；若存在则 policy 失败。生产 Agent selection / safety / interface 权威为 leaf `ContractFinal` 与 `ProductDecl`；`agent_metadata_inject.go` / `InstallBuildTimeAgentMetadataJSON` 仅作 `cmd_schema_catalog` CI/local dump 辅助，不得作为生产权威。

Schema 不调用 MCP `tools/list`，不访问网络，也不读取用户本地 discovery cache。

**flag / help / schema 参数面同源**（已决策）：Contract / LeafSpec 为 CLI 表面权威；分层字段归属与门禁 ID 见 [`flag-help-schema-homology.md`](flag-help-schema-homology.md)。

## 2. 单向数据流

```text
identity collector (CollectIdentitySpecs)
  walks live Cobra leaves carrying ContractFinal.Identity;
  reviewed exclusions applied; single identity source
  (reviewed schema_command_registry/ retired)
                         |
                         v
             EffectiveCommandRegistry
                         |
                         v
          exact binder to live Cobra tree
       + native identity consistency assertions
                         |
                         v
               BoundCommandRegistry
                         |
                         v
live Cobra flag facts / typed parameter metadata
  + leaf Contract (Safety / ContractDecl / ParamDecl → contract_final)
  + ProductDecl (product routing prose; production Agent authority)
  + schema_parameter_mapping_ledger.go (reviewed mapping exclusions / removals)
  + leaf Contract.Interface / ParamDecl (declared interface facts)
  + skills/mono Markdown              (evidence only; not concatenated)
                         |
                         v
              source adapters + resolvers
                         |
                         v
             one typed SchemaRegistry
              (one ToolSpec per command)
                         +
                    typed SchemaIndex
             +-----------+-----------+
             |                       |
             v                       v
     build-time typed gates   RegisterSchemaSourceRoot
                              -> ResolveSchemaBuild
                              (runtime assembly; lazy Once)
                                     |
                                     v
                         SchemaRegistry + SchemaIndex
                         + ResolveMeta projection cache
                           (in-process map; not gob/json fixture)
                                     |
               +---------------------+------------------+
               |                     |                  |
      overview/product/group        leaf              --all
            projections          projection      full projection
               |                     |                  |
               +---------------------+------------------+
                                     |
                                     v
                         runtime query + delivery gates
                         (cmd_schema_catalog = CI/local dump only;
                          InstallBuildTimeAgentMetadataJSON = dump helper,
                          not production Agent authority)
```

`--help` 是 Cobra 自身的人类可读投影，不从 Catalog 生成。Schema projections 和 `--help` 共享同一真实 Cobra 命令面，但承担不同职责。Binder 之后不得再从 annotation、已退役 hint overlay 或生成 JSON 重新解析 command identity。

## 3. 与 Lark 的关系

DWS 与 Lark 保持**架构同构**，而不是强行复制字段：

| Lark 分层 | DWS 对应层 |
|---|---|
| typed command/metadata registry | `EffectiveCommandRegistry`、`BoundCommandRegistry` 与最终 `SchemaRegistry` |
| navigation catalog/index | 从同一 `ToolSpec` 派生的 `SchemaIndex` |
| schema renderer/envelope | overview、product/group、leaf、`--all` projections |
| API Commands ← 平台 OAPI meta | **不**作为 DWS 主路径；可选 1:1 MCP 透传子集见同源文档 §5 |
| Shortcuts ← 手写声明 + Execute | LeafSpec / Shortcut + Contract 表面（路径 A） |

共同点是：强类型 registry 持有已审核、已绑定、已解析的事实，index 只负责确定性导航，renderer 只投影，不重新读取来源或做 precedence。DWS 的 identity collector 从携带 `ContractFinal.Identity` 的 live Cobra leaves 收集 identity，绑定前生成唯一的 `EffectiveCommandRegistry`（reviewed `schema_command_registry/` 已退役），因此不存在 “native-first”、“legacy registry fallback” 或 Catalog fallback。飞书也是**分层单权威**（API 用平台 meta，Shortcut 用手写契约），不是全家只有 meta——DWS 对齐的是这一分层，而不是「用 MCP meta 生成全部 CLI」。

DWS 内部 resolved model 为：

```text
SchemaRegistry
  -> []ProductSpec
       -> []ToolSpec
            -> ToolIdentitySpec
            -> []ParameterSpec
            -> RuntimeSchemaConstraints + []RuntimeSchemaPositional
            -> SafetySpec
            -> InterfaceSpec
            -> SelectionSpec
            -> map[field]FieldProvenance
```

字段合并和 precedence 在进入该模型前完成。`map[string]any`/flat JSON 只允许存在于 renderer 和 snapshot/wire boundary，不能作为内部 resolver、navigation 或 gate 的第二套数据模型。

DWS 当前对外仍保留兼容 wire：leaf 使用 flat `parameters`，安全和选择字段也保持现有键名。架构对齐不等于未版本化地切换到 Lark `inputSchema/outputSchema/_meta` envelope；若未来提供该格式，应作为明确版本的新投影，并保留现有兼容输出。

## 4. 来源职责

| 来源 | 负责内容 | 明确不负责 |
|---|---|---|
| Contract / LeafSpec / `corecmd.Spec` | **CLI 表面权威**：flags、defaults、required、enum、关系约束、运行时 Risk；编译为 cobra 与 help | canonical identity、selection 文案、虚构 RPC |
| identity collector（`CollectIdentitySpecs`） | 从 live Cobra leaves 的 `ContractFinal.Identity` 声明收集稳定 canonical identity、primary CLI path、alias、exposure 和导航（reviewed `schema_command_registry/` 已退役） | 创建 Cobra 命令/flag、参数、安全、endpoint/token |
| reviewed exclusions（`ReviewedRuntimeSchemaExclusions`） | exact、reviewed、带 reason 地将指定 public runnable leaf 排除出 effective 表面 | 运行时 fallback、prefix/wildcard 排除、创建命令 |
| Go/Cobra | Contract 编译后的可执行投影：路径是否真实可执行、Cobra 接受的 flag、DefValue、help 文本 | 稳定 canonical identity、Agent 场景选择、虚构 RPC；**不得**成为与 Contract 平行的第二套 flag 权威 |
| native Schema identity annotations | implementation-side consistency evidence；存在时必须与 `EffectiveCommandRegistry` 精确一致 | 提供、补全、推断或覆盖 identity |
| typed parameter metadata / constraints | 由 Contract 约束投影而来的 `require_one_of` / 互斥等；以及仍需 reviewed 的 `required_when` 等 | 命令 identity |
| `schema_parameter_mapping_ledger.go` | CLI flag 无直接 RPC property 的 exclusions / removals | 命令发现、risk 推断、创建 CLI flag；property 交付归 ParamDecl.Property |
| leaf `Contract.Interface` / `ParamDecl` | 声明的 RPC identity 与 interface_* 事实（`schema_mcp_metadata.json` 已退役） | CLI identity、运行时路由、**创建 CLI flag**、risk 推断 |
| ProductDecl + leaf `Contract.Selection` | reviewed selection / product routing prose（`contract_final`） | 创建 Cobra 命令或参数、改写 safety；`schema_hints/` 已退役 |
| Skills/Markdown | 产品路由、工作流和使用建议 | 命令存在性和 flag 事实 |
| `cmd_schema_catalog` CI/local dump（可选 `schema_catalog/` / meta-index） | resolved registry 的兼容序列化快照，仅供 jq/determinism；不得提交为 runtime 来源 | production delivery、`ResolveMeta` 权威、identity fallback、手工修复源 |

identity collector 从 live Cobra leaves 的 `ContractFinal.Identity` 声明生成 `CommandSpec`，应用 reviewed exclusions 后按确定性规则索引为 effective registry；从 binder 开始，下游只看到一个稳定 identity/navigation 模型。reviewed `schema_command_registry/` 已退役，其历史角色（稳定 canonical identity/navigation 事实源）由 collector 承接。旧 wire 中的 `surface_hash` / `surface_tools` 字段仅为兼容名称，语义已经是 effective Registry hash/coverage，不构成第二事实源。

### 4.1 flag / help / schema 同源（路径 A + 嵌入）

完整决策、字段归属表、`HOM-*` 门禁规划与可选 MCP 透传准入条件见 [`flag-help-schema-homology.md`](flag-help-schema-homology.md)。

摘要：

- **同源面**：Contract → cobra flags ≡ `--help` Flags ≡ **嵌入注解后的** schema `parameters` / 关系约束；显式 `Risk` 经 `dws.schema.risk` overlay 进 Schema Safety。
- **嵌入点**：`command.embedContractIntoSchema` 写入 `dws.schema.contract` / property / type / required；`AnnotateConstraints` 写入 constraints；Schema 组装（`runtimeToolSpecFromMetadata` / `ResolveSchemaBuild`）消费这些注解进入 typed `SchemaRegistry`（runtime assembly；非 `go:embed` catalog 交付）。
- **硬规则**：CLI 表面事实 = **声明（Contract 数据字段）OR 人工标注**；禁止纯推断。非 CLI 表面字段（identity / selection / interface）必须有**评审源**。声明写法见同源文档 §1.2；标注见 §1.3；**`ToolSpec` 全字段权威见 RFC §5.0.4 / 同源 §1.4**。
- **非同源面（有意）**：identity（collector）、selection 文案（ProductDecl / leaf `Contract.Selection`）、RPC 形状（MCP meta 仅 `interface_*`）、dry-run 正能力 registry。
- **禁止**：以 MCP meta 为主通道生成 Leaf/Shortcut 的 flag；已退役的 hint overlay 改写 type/required/default；Schema 字段无权威归属。

## 5. 统一解析与 precedence

### 5.1 Identity

- identity collector（`CollectIdentitySpecs`）是 stable canonical identity、primary path、alias 和 navigation 的唯一基础事实源：每个携带 `ContractFinal.Identity` 的 runnable Cobra leaf（含 Hidden deprecated/migration shims）贡献一条 identity，reviewed exclusions 精确应用；reviewed `schema_command_registry/` 已退役，其历史角色由 collector 承接。
- Collector 输出的 `CommandSpec` 在索引时 fail-closed 校验 canonical/product/path/alias/visibility 的合法性与唯一性；重复 identity、alias 复用 primary path 或 alias collision 全部失败，不能按 precedence 静默覆盖。
- Binder 必须把 effective entry 的 primary path 和每个 alias 精确解析到同一个真实 executable leaf；stale path、phantom path、重复 identity 或 alias collision 全部失败。
- Native identity annotation 是可选的一致性证据：存在时必须与 effective entry 精确一致；缺失不触发补写、推断或 fallback。
- Public runnable Cobra leaf 未进入 effective registry 时，必须存在 exact、reviewed、带 reason 的 exclusion；不得用 prefix/wildcard 排除。
- Identity 不做名称推断，不从 Catalog/generated metadata fallback，也没有多来源 winner。

删除 native materialization 前已做写入审计：旧
`ApplyNativeRuntimeSchemaContracts` 的唯一写操作是对已存在命令调用
`AttachRuntimeSchema`，只写 command identity 的 product/tool/source annotation；
它不写 flag property/type/required、constraints、positionals、title/description
或 interface mapping。这些字段原本已分别由 parameter binding/metadata、
constraint、Cobra help 和 interface resolver 提供，因此删除该过渡层没有数据迁移缺口。
CI 同时禁止重新加入 generated native contracts 或 materialization 入口。

#### Identity 输入审计（registry 已退役）

历史上 identity 来自 reviewed `schema_command_registry/`（`registry.json` +
`products/*.json`，由随附 JSON Schema 校验）。该 reviewed registry 已退役，
改由 identity collector 承接：identity 现在由 `CollectIdentitySpecs` 从 live
Cobra 树的 `ContractFinal.Identity` 声明收集，输入契约即声明本身。严格 Go
索引（`indexCommandSpecs`）继续 fail-closed 校验：

- canonical identity、`source_product_id` 和精确 CLI path 的格式；
- `aliases` 唯一且不能复用 primary path；
- `visibility` 只允许 `public | compat | internal`，省略时明确归一化为
  `public`；
- primary path、alias、canonical 和 product 之间的交叉唯一性约束。

Registry semantic hash 仍覆盖 canonical、primary CLI path、alias 集合、
`source_product_id` 和 normalized visibility。格式、顺序以及省略的等价默认值
不改变 hash；上述任一稳定契约字段变化都必须改变 hash。测试逐字段验证这一点，
不使用当前命令数量作为常量。

普通 `go generate ./internal/cli` 只生成
`param_aliases_generated.go`；production Catalog / `ResolveMeta` 由 runtime
`ResolveSchemaBuild` 装配（`deliverySchemaCatalog` Once 后缓存 Meta 投影）。
`cmd_schema_catalog` 仅按需打 CI/local dump；其 `InstallBuildTimeAgentMetadataJSON`
inject 仅服务 dump，生产 Agent 权威仍是 leaf `ContractFinal` / `ProductDecl`。
不写也不 embed `schema_agent_metadata/`。drift policy 禁止已退役的
`schema_command_registry/` 在生成前后重新出现；原独立脚本
`check-schema-command-registry.sh` 的 registry-agnostic side guards（禁用旧
native materialization 符号、go:generate 单轨、lazy loader 纪律）已迁入
`check-schema-catalog.sh`。

### 5.2 Parameter

每个字段按明确的来源 precedence 选择一次，并把 winner、候选值和来源写入 provenance。precedence **与值无关**：不能因为 `required=true` 看起来更严格就让它越级获胜。更高优先级的 reviewed manual override 可以把 `required`、映射、interface type 或描述调高，也可以调低。

实现中的参数字段顺序固定为：

```text
versioned binding > command constraint > typed metadata
                > native/Cobra contract (ParamDecl / ContractFinal)
                > MCP metadata > inference/default
```

命令 `title` / `description` 使用独立但同样确定的文本顺序：

```text
description: Cobra Long > ContractDecl description (contract_final) > MCP metadata > inference
             (Long 胜出时 provenance = cobra_help / cobra_help_preferred)
title:       ContractDecl / ContractFinal > Cobra Short > MCP metadata > inference
```

因此多个 CLI leaf 复用同一个 RPC 时，通用 RPC 文案只能作为未选中的
provenance candidate 保留；参数级 RPC 文案可进入 `interface_description`，
但不得覆盖 leaf 自己的标题和执行语义。

Cobra hard-required 是独立的 executable fact，并通过 `cli_required`/provenance 保留；它不应在 renderer 中再次静默改写已经解析的 Agent projection。

### 5.3 Safety、selection 与 interface

`effect`、`risk`、`confirmation`、`idempotency`、selection 和 interface disposition 同样按 source precedence 解析，而不是按值的“严格程度”合并。更高优先级的 reviewed explicit/manual source 可以升高或降低最终值；同 precedence 的不同值必须报冲突。

最终 interface disposition 还必须满足 conflict matrix：

- `mode` 与 `availability` 正交：`mode` 只允许 `mcp | local | composite`，`availability` 只允许 `available | unavailable`；`unavailable` 不是第四种 mode。
- `mcp + available`：只表示命令可由一个 pinned、参数可映射且语义等价的 `interface_ref` 完整表达；本地 wrapper 只是固定默认值或投影返回值时，也必须先证明参数和执行语义没有漂移。
- `local + available`：仅用于纯本地进程、静态数据或策略操作，不得携带 direct `interface_ref`；“远端 RPC 尚未进入 pinned metadata”不能归类为 local。
- `composite + available`：用于多 RPC、条件路由、本地投影，或 reviewed unpinned remote adapter；不得用单个 `interface_ref` 冒充完整实现，且必须提供 reviewed reason。未来需要表达多个 RPC 时使用单独的复合接口模型。
- 任意合法 mode + `unavailable`：不得携带 `interface_ref`，必须提供明确 reason，并且 Agent 不得把它当作可用接口。

## 6. Schema、Help 与业务数据边界

| 问题 | 事实源 |
|---|---|
| 当前二进制是否暴露命令、Cobra 接受哪些 flags | `dws <path> --help` |
| Agent 选哪个命令、CLI 参数/required/约束、risk/confirmation | Agent leaf `dws schema "<path>" --compact` |
| CLI↔RPC 参数映射、接口绑定、provenance | full leaf 配合 `--jq` / `--fields` 精确投影 |
| 钉钉中的文档、文件、日程、消息等实际数据 | 真正执行 `dws doc read`、`dws drive search` 等 read/search/list 命令 |

Schema 和 Help 冲突是契约漂移，不能静默猜测：

- 执行参数以 Cobra 实际接受的 flags 为准；不要发送 Help 中不存在的 flag。
- 安全语义冲突时不要采用更宽松值。先按更保守的解释确认；如果无法确定安全执行方式，停止并报告漂移。
- Schema/Help 只完成命令发现和契约读取。需要业务结果时，必须继续执行真实 read/search/list 命令。

上述运行时漂移策略不改变构建期的 value-neutral precedence；前者是在契约已经互相矛盾时保护用户，后者是在确定性生成同一契约。

## 7. 查询投影

```bash
dws schema                                      # 产品紧凑概览
dws schema calendar --compact                   # Agent 产品摘要
dws schema "calendar event" --compact          # Agent 分组摘要
dws schema "calendar event create" --compact   # Agent leaf
dws schema "calendar event create"             # full leaf，仅用于映射/provenance 审计
dws schema --all                               # 所有工具的完整 leaf 导出
```

`schema list` 是根概览的兼容入口。

`schema --all` 必须包含最终 `SchemaIndex` 中每个 tool 的完整 leaf 参数、约束和安全语义；无业务参数的命令也要包含空 `parameters` 对象。它用于审计、CI 和参数防丢 baseline，但输出很大，普通 Agent 命令发现不得使用，应按 overview -> product/group -> leaf 渐进查询。

`--compact` 是普通 Agent 查询的规范视图：通过正向字段白名单保留选参、约束与安全语义，full 新增字段不会自动进入 Agent 上下文。省略它的 leaf 包含参数 property、接口绑定和 provenance，只用于定向审计；`schema --all --compact` 也可执行，但不能作为完整兼容性 baseline。

兼容旧二进制时，如果 Schema 查询返回 `unknown_flag: --compact`，只去掉 `--compact` 重试同一个查询。这是展示能力降级，不代表 leaf 缺失，也不能改用 Schema 查询业务数据。

## 8. 生成与发布

当 Cobra、flag、identity、binding、leaf `Contract` / ProductDecl 或 Skill 发生变化时：

1. 审核真实 Cobra 变化，确认命令和 flag 已实际存在。新增或修改稳定 command identity、primary CLI path 或 alias 现在通过 leaf Contract 的 `ContractFinal.Identity` 声明完成（identity collector 据此收集；reviewed `schema_command_registry/` 已退役）。参数、Skill 或 metadata 单独变化时不要机械改写 identity 声明，也不要从旧 Catalog 反向生成它。
2. 不应进入稳定 Agent 契约的 public runnable leaf 使用 reviewed exclusions（exact path、带 reason）。Native identity annotation 若存在，应作为与 identity 声明一致的实现断言维护，而不是用来 materialize identity。
3. 生成参数别名并验证运行时 Schema 组装。`go generate ./internal/cli` 只运行 `cmd_param_aliases`；`cmd_schema_catalog` 仅按需生成 CI/local dump。生产权威为 leaf `ContractFinal` / `ProductDecl`；CI dump 可经 `agent_metadata_inject.go` / `InstallBuildTimeAgentMetadataJSON` 在内存中注入 Agent metadata，不写 `schema_agent_metadata/`：

   ```bash
   make generate-schema
   go generate ./internal/cli
   # 可选：生成 artifacts/ 下的 CI/local dump
   make generate-schema-catalog
   ```

`cmd_schema_agent_metadata` 可保留为非交付工具/测试，但不是 `go:generate` 入口，也不应再作为发布步骤。

生成文件只有 `internal/cli/param_aliases_generated.go`（参数别名生成物）。`cmd_schema_catalog` 的 Catalog 和 meta-index 是可选 CI/local dump，不是交付物，且不得写入或提交到 `internal/cli/`。

`schema_agent_metadata/`、`schema_agent_metadata_audit.json` 与 `schema_hints/` 已退役；若存在则 policy 失败。只编辑来源；不要手工编辑或提交 Catalog / meta-index dump。

## 9. Completeness 与 final-delivery invariant

门禁必须验证最终交付对象，而不是某个中间层或数量：

- 每个 public runnable Cobra leaf 要么能通过最终 embedded `SchemaIndex` 查询，要么有 exact、reviewed、带 reason 的 exclusion。
- 每个最终 canonical path、primary CLI path 和 alias 都必须解析到同一个可执行 leaf；不得有 phantom path 或 collision。
- `EffectiveCommandRegistry`、`SchemaRegistry/SchemaIndex` 与 Catalog canonical sets 必须精确一致（含组装时内存 inject 的 Agent metadata 语义），不能只比较 count。
- Leaf payload、`--all` 中对应 tool 和 Catalog full tool 必须是同一个 resolved `ToolSpec` 的内容级等价投影，并通过 production loader round-trip。
- overview/product/group summary 与 Catalog summary 必须等于同一个 `ToolSpec.ToSummaryPayload()`；alias 查询只允许 `cli_path` 和 `is_alias` 这两个视图字段变化。
- 每个最终字段及 parameter field 的 provenance winner value 必须与 delivered value 精确一致；不能只验证 provenance source、count 或字段是否存在。
- 每个 MCP `interface_ref` 必须在 pinned interface registry 精确存在；local/composite/unavailable 必须满足同一 conflict matrix。
- `--all` 的 tool set 必须与最终 index 一对一，且每个工具包含完整参数契约。
- 连续两次生成必须字节稳定，提交的生成物不得漂移。
- **同源门禁（规划，见同源文档 §4）**：受管命令逐步满足 `HOM-P1`–`P3`（parameters ≡ cobra/Contract）、`HOM-S1`–`S3`（Safety/Risk 对齐）、`HOM-I1`（interface 不创建 flag）、`HOM-D1`（help ≡ schema parameters）。hints 不得作为 type/required/default 的 winner。

推荐本地验证：

```bash
make generate-schema
./scripts/policy/check-generated-drift.sh
./scripts/policy/check-schema-catalog.sh
go test ./internal/cli ./internal/app ./internal/generator/... -count=1
```

## 10. 明确禁止

- 运行时调用 MCP `tools/list` 或访问网络生成 Schema。
- 从 `schema_catalog/` 等生成 JSON 反向创建/补齐 Cobra leaf、flag、CommandRegistry 或下一轮 Catalog。
- 重新引入 `schema_agent_metadata/`（或 audit JSON）作为交付物、`go:embed` 目标，或把它写回 `go:generate` 入口。
- 从 MCP meta / 已退役的 `schema_mcp_metadata` **生成或补齐** LeafSpec/Shortcut 主路径的 CLI flag（interface overlay 除外）；未满足同源文档 §5 准入条件时启用「MCP 透传生成通道」。
- 把 native annotation、legacy registry 或 Catalog 当作 identity fallback；或在 `EffectiveCommandRegistry` 之后再次选择 identity winner。
- renderer、query 或 gate 在 `SchemaRegistry` 之后重新读取 source 并做第二次 merge。
- 用 prefix/wildcard exclusion 隐藏未来命令。
- 让 ProductDecl / leaf `Contract`、CommandRegistry 或 interface metadata 宣称一个不存在的命令、flag 或 RPC 可用。
- 重新引入 `schema_hints/`（含 selection/metadata/imported/audit JSON）或任何 HintFile overlay，并在与 Contract/cobra 冲突时赢得 type/required/default。
- 把 `schema --all` 当作普通业务数据查询，或把其完整结果无条件注入 Agent 上下文。
- 将 LeafSpec、`+shortcut` 或 write-guard/cursor/多步命令注册为 `mcp_passthrough` 表面。
