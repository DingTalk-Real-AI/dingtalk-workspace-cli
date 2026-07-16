# DWS Agent Schema 统一方案

## 1. 核心定义

DWS Schema 是当前二进制公开 CLI 的版本化 Agent 执行契约。它描述真实 Cobra 命令，并补充 Agent 选择、参数映射、组合约束、安全确认和接口事实。

设计遵循三条硬规则：

1. **Schema 描述 CLI，不制造 CLI。** `CommandRegistry`、manual hint、metadata 和 Catalog 都不能凭空创建 Cobra 命令或 flag；registry 中的每个路径都必须精确绑定真实 runnable Cobra leaf。
2. **所有来源只解析一次。** 来源经过统一 resolver 进入 typed `SchemaRegistry`，所有查询、导出和门禁都消费同一个 `SchemaRegistry/SchemaIndex`。
3. **Registry-first，Catalog 只出不进。** reviewed `CommandRegistry` 是稳定 command identity/navigation 的唯一事实源；`schema_catalog.json` 和其他生成 JSON 只是下游发布物，不能成为命令、metadata 或下一轮 Catalog 的来源。运行时 production loader 解码 embedded snapshot 只是交付边界，不是 source resolution。

Schema 不调用 MCP `tools/list`，不访问网络，也不读取用户本地 discovery cache。

## 2. 单向数据流

```text
schema_command_registry.json           (reviewed CommandRegistry source)
  + reviewed manual command additions
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
                         +----------------------+
                                                |
skills/mono Markdown + internal/cli/schema_hints/*.json |
  + schema_mcp_metadata.json                     |
                         |                       |
                         v                       |
             Agent-metadata normalization       |
                         |                       |
                         v                       |
             schema_agent_metadata/*.json       |
               (generated normalized input)     |
                         |                       |
                         +-----------------------+
                         |
live Cobra flag facts / typed parameter metadata
  + schema_hints/metadata/*.json      (reviewed parameter overlay + safety)
  + schema_hints/selection/*.json     (reviewed Agent selection prose)
  + schema_parameter_bindings.json    (reviewed flag -> RPC property)
  + schema_mcp_metadata.json          (pinned, sanitized interface facts)
  + normalized Agent metadata
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
     build-time typed gates    snapshot serializer
                                     |
                                     v
                              schema_catalog.json
                              (release output only)
                                     |
                                     v
                           go:embed -> typed loader
                                     |
                                     v
                         SchemaRegistry + SchemaIndex
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
```

`--help` 是 Cobra 自身的人类可读投影，不从 Catalog 生成。Schema projections 和 `--help` 共享同一真实 Cobra 命令面，但承担不同职责。Binder 之后不得再从 annotation、manual hint 或生成 JSON 重新解析 command identity。

## 3. 与 Lark 的关系

DWS 与 Lark 保持**架构同构**，而不是强行复制字段：

| Lark 分层 | DWS 对应层 |
|---|---|
| typed command/metadata registry | `EffectiveCommandRegistry`、`BoundCommandRegistry` 与最终 `SchemaRegistry` |
| navigation catalog/index | 从同一 `ToolSpec` 派生的 `SchemaIndex` |
| schema renderer/envelope | overview、product/group、leaf、`--all` projections |

共同点是：强类型 registry 持有已审核、已绑定、已解析的事实，index 只负责确定性导航，renderer 只投影，不重新读取来源或做 precedence。DWS 的 base Registry 与 reviewed manual command additions 在绑定前合并为唯一的 `EffectiveCommandRegistry`，因此不存在 “native-first”、“legacy registry fallback” 或 Catalog fallback。

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
| `schema_command_registry.json` | reviewed `CommandRegistry`：稳定 canonical identity、primary CLI path、alias、exposure 和导航 | 创建 Cobra 命令/flag、参数、安全、endpoint/token |
| reviewed manual command additions | 将一个精确存在的 runnable Cobra leaf 合并进 `EffectiveCommandRegistry`；必须 reviewed 且带 reason | 运行时 fallback、覆盖冲突 identity、创建命令 |
| Go/Cobra | 路径是否真实可执行、Cobra 接受的 flag、CLI 类型/默认值、执行校验、help 文本 | 稳定 canonical identity、Agent 场景选择、虚构 RPC |
| native Schema identity annotations | implementation-side consistency evidence；存在时必须与 `EffectiveCommandRegistry` 精确一致 | 提供、补全、推断或覆盖 identity |
| `schema_hints/metadata/*.json` parameter overlays | 精确覆盖现有 flag 的描述、映射、类型和 required 语义；并承载 safety / `runtime_gate` / interface | 创建命令/flag、绕过 completeness、虚构 RPC |
| typed parameter metadata / constraints | `required_when`、one-of、互斥、联动、格式、枚举、位置参数 | 命令 identity |
| `schema_parameter_bindings.json` | 稳定 CLI flag 到 RPC property 的映射 | 命令发现、risk 推断 |
| `schema_mcp_metadata.json` | pinned RPC identity、接口描述和脱敏参数事实 | CLI identity、运行时路由、risk 推断 |
| `schema_hints/selection/*.json` | reviewed selection prose（summary / use_when / avoid_when / examples） | 创建 Cobra 命令或参数、改写 safety |
| Skills/Markdown | 产品路由、工作流和使用建议 | 命令存在性和 flag 事实 |
| `schema_catalog.json` 及其他 generated JSON | resolved registry 的兼容发布序列化；运行时由 production loader 解回 typed registry/index | generation/source resolution 输入、identity fallback、手工修复源 |

`schema_command_registry.json` 承载 reviewed `CommandRegistry`。Manual command addition 先以确定性规则合并进 effective registry；从 binder 开始，下游只看到一个稳定 identity/navigation 模型。旧 wire 中的 `surface_hash` / `surface_tools` 字段仅为兼容名称，语义已经是 effective Registry hash/coverage，不构成第二事实源。

## 5. 统一解析与 precedence

### 5.1 Identity

- Reviewed base `CommandRegistry` 是 stable canonical identity、primary path、alias 和 navigation 的唯一基础事实源。
- Reviewed manual command addition 只能引用精确存在的 runnable Cobra leaf；它在绑定前合并进 `EffectiveCommandRegistry`。若与 base Registry 的 identity/path/alias 冲突，生成失败，不能按 precedence 静默覆盖。
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

#### CommandRegistry 输入审计

`schema_command_registry.json` 是 reviewed source，不是生成快照。它必须保留
`$schema: ./schema_command_registry.schema.json`。该 JSON Schema 对 root、product
和 CommandSpec 全部使用 `additionalProperties: false`，并约束：

- canonical identity、`source_product_id` 和精确 CLI path 的格式；
- `aliases` 唯一且不能复用 primary path；
- `visibility` 只允许 `public | compat | internal`，省略时明确归一化为
  `public`；
- primary path、alias、canonical 和 product 之间无法由 JSON Schema 表达的
  交叉约束，继续由 Go strict loader 和 Cobra binder fail-closed 校验。

Registry semantic hash 覆盖 canonical、primary CLI path、alias 集合、
`source_product_id` 和 normalized visibility。格式、顺序以及省略的等价默认值
不改变 hash；上述任一稳定契约字段变化都必须改变 hash。测试逐字段验证这一点，
不使用当前命令数量作为常量。

普通 `go generate ./internal/cli` 只把 Registry 作为 validation-only 输入并生成
Agent metadata/Catalog 等单向下游资产，不生成或覆盖 Registry。drift policy 在生成
前后对 reviewed Registry 做 byte-for-byte guard；独立的
`check-schema-command-registry.sh` 在 interface/provenance/Catalog policy 之前检查
JSON 输入契约、禁用旧 native materialization 符号，并从 Registry 动态计算审计
数量，不能硬编码某次快照的 tool count。

### 5.2 Parameter

每个字段按明确的来源 precedence 选择一次，并把 winner、候选值和来源写入 provenance。precedence **与值无关**：不能因为 `required=true` 看起来更严格就让它越级获胜。更高优先级的 reviewed manual override 可以把 `required`、映射、interface type 或描述调高，也可以调低。

实现中的参数字段顺序固定为：

```text
reviewed manual > versioned binding > command constraint > typed metadata
                > native/Cobra contract > ToolSchemaHint > MCP metadata
                > inference/default
```

命令 `title` / `description` 使用独立但同样确定的文本顺序：

```text
reviewed ToolSchemaHint > command-specific Cobra Help > MCP metadata > inference
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
| Agent 选哪个命令、参数映射/required/约束、risk/confirmation | 对应 leaf `dws schema "<path>"` |
| 钉钉中的文档、文件、日程、消息等实际数据 | 真正执行 `dws doc read`、`dws drive search` 等 read/search/list 命令 |

Schema 和 Help 冲突是契约漂移，不能静默猜测：

- 执行参数以 Cobra 实际接受的 flags 为准；不要发送 Help 中不存在的 flag。
- 安全语义冲突时不要采用更宽松值。先按更保守的解释确认；如果无法确定安全执行方式，停止并报告漂移。
- Schema/Help 只完成命令发现和契约读取。需要业务结果时，必须继续执行真实 read/search/list 命令。

上述运行时漂移策略不改变构建期的 value-neutral precedence；前者是在契约已经互相矛盾时保护用户，后者是在确定性生成同一契约。

## 7. 查询投影

```bash
dws schema                                      # 产品紧凑概览
dws schema calendar                             # 产品摘要
dws schema "calendar event"                    # 分组摘要
dws schema "calendar event create"             # 完整 leaf
dws schema "calendar event create" --compact   # 支持：裁掉 provenance/debug 字段
dws schema --all                               # 所有工具的完整 leaf 导出
```

`schema list` 是根概览的兼容入口。

`schema --all` 必须包含最终 `SchemaIndex` 中每个 tool 的完整 leaf 参数、约束和安全语义；无业务参数的命令也要包含空 `parameters` 对象。它用于审计、CI 和参数防丢 baseline，但输出很大，普通 Agent 命令发现不得使用，应按 overview -> product/group -> leaf 渐进查询。

`--compact` 当前受支持，适合减少常规 leaf 查询上下文。`schema --all --compact` 也可执行，但会移除 provenance/debug 和接口映射字段，不能作为完整兼容性 baseline。

兼容旧二进制时，如果 Schema 查询返回 `unknown_flag: --compact`，只去掉 `--compact` 重试同一个查询。这是展示能力降级，不代表 leaf 缺失，也不能改用 Schema 查询业务数据。

## 8. 生成与发布

当 Cobra、flag、identity、binding、manual hint、Agent hint 或 Skill 发生变化时：

1. 审核真实 Cobra 变化，确认命令和 flag 已实际存在。新增或修改稳定 command identity、primary CLI path 或 alias 时，精确编辑 reviewed `CommandRegistry`（当前持久化文件为 `schema_command_registry.json`）。参数、Skill 或 metadata 单独变化时不要机械改写 Registry，也不要从旧 Catalog 反向生成它。
2. 仅对明确例外使用 reviewed manual command addition；它必须精确引用现有 runnable leaf、带 reason，并在生成时归一化进 `EffectiveCommandRegistry`。Native identity annotation 若存在，应作为与 Registry 一致的实现断言维护，而不是用来 materialize identity。
3. 生成 Agent metadata：

   ```bash
   make generate-schema-agent-metadata
   ```

4. 从统一 typed registry 生成最终 Catalog：

   ```bash
   make generate-schema-catalog
   ```

也可以运行 `go generate ./internal/cli` 生成正常发布资产。生成文件包括：

- `internal/cli/schema_agent_metadata/index.json`
- `internal/cli/schema_agent_metadata/<product>.json`
- `internal/cli/schema_agent_metadata_audit.json`
- `internal/cli/schema_catalog.json`

只编辑来源；不要手工编辑 Agent metadata 或 Catalog 输出。

## 9. Completeness 与 final-delivery invariant

门禁必须验证最终交付对象，而不是某个中间层或数量：

- 每个 public runnable Cobra leaf 要么能通过最终 embedded `SchemaIndex` 查询，要么有 exact、reviewed、带 reason 的 exclusion。
- 每个最终 canonical path、primary CLI path 和 alias 都必须解析到同一个可执行 leaf；不得有 phantom path 或 collision。
- `EffectiveCommandRegistry`、`SchemaRegistry/SchemaIndex`、Agent metadata 和 Catalog canonical sets 必须精确一致，不能只比较 count。
- Leaf payload、`--all` 中对应 tool 和 Catalog full tool 必须是同一个 resolved `ToolSpec` 的内容级等价投影，并通过 production loader round-trip。
- overview/product/group summary 与 Catalog summary 必须等于同一个 `ToolSpec.ToSummaryPayload()`；alias 查询只允许 `cli_path` 和 `is_alias` 这两个视图字段变化。
- 每个最终字段及 parameter field 的 provenance winner value 必须与 delivered value 精确一致；不能只验证 provenance source、count 或字段是否存在。
- 每个 MCP `interface_ref` 必须在 pinned interface registry 精确存在；local/composite/unavailable 必须满足同一 conflict matrix。
- `--all` 的 tool set 必须与最终 index 一对一，且每个工具包含完整参数契约。
- 连续两次生成必须字节稳定，提交的生成物不得漂移。

推荐本地验证：

```bash
make generate-schema-agent-metadata
make generate-schema-catalog
./scripts/policy/check-generated-drift.sh
./scripts/policy/check-schema-catalog.sh
go test ./internal/cli ./internal/app ./internal/generator/... -count=1
```

## 10. 明确禁止

- 运行时调用 MCP `tools/list` 或访问网络生成 Schema。
- 从旧 `schema_catalog.json` 或其他 generated JSON 反向创建/补齐 Cobra leaf、flag、CommandRegistry 或下一轮 Catalog。
- 把 native annotation、legacy registry 或 Catalog 当作 identity fallback；或在 `EffectiveCommandRegistry` 之后再次选择 identity winner。
- renderer、query 或 gate 在 `SchemaRegistry` 之后重新读取 source 并做第二次 merge。
- 用 prefix/wildcard exclusion 隐藏未来命令。
- 让 manual hint、CommandRegistry 或 interface metadata 宣称一个不存在的命令、flag 或 RPC 可用。
- 把 `schema --all` 当作普通业务数据查询，或把其完整结果无条件注入 Agent 上下文。
