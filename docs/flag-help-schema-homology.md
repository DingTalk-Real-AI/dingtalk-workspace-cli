# flag / help / schema 同源

- **状态**：已决策（路径 A + Contract 嵌入 Schema）
- **相关**：[`rfc-command-framework-convergence.md`](rfc-command-framework-convergence.md)、[`schema-dynamic-endpoint-design.md`](schema-dynamic-endpoint-design.md)
- **实现门面**：`LeafSpec` → `corecmd.Spec` → `corecmd.New`

## 1. 决策

采用 **路径 A：Contract / LeafSpec 为 CLI 表面权威**，并且 **Contract 必须嵌入进 Schema**。

「同源」的含义是：同一份 Contract（今日经 LeafSpec / `corecmd.Spec` 表达）同时决定——且门禁能证明——

1. cobra 实际注册的 flags / required / defaults；
2. `--help` 的 Flags 与「参数约束」段；
3. 运行时**组装后的** Schema 交付中的 parameters、关系约束和 SafetySpec：
   - **SchemaRegistry / Catalog ToolSpec wire** 供 `dws schema` / `--all` / 完整 leaf 载荷（lazy；首次 `dws schema` 触发完整装配）。
   - **`ResolveMeta` / `SafetyForCLIPath` / leaf `--help` Safety** 与装配同源：`deliverySchemaCatalog` sync.Once 装配后同步物化 `map[cli_path]CommandMeta`；稳态为 O(1) map lookup，不为 Meta 单独重做全量 ToolSpec/wire 投影。
   Agent metadata 在组装期间经内存 inject，不落盘、不 embed；`schema_agent_metadata/` 已退役，若存在则 policy 失败。

嵌入机制（已落地）：

```text
corecmd.Spec
  → RegisterFlags + embedContractIntoSchema
  → cobra annotations:
       dws.schema.contract=command
       dws.schema.property / type / required  (per flag)
       dws.schema.constraints
  → RegisterRuntimeContractFinal(SafetySpec + ContractDecl)
  → Schema 组装透传 Contract Final
    （组装时内存 inject Agent metadata）
  → RegisterSchemaSourceRoot → ResolveSchemaBuild
  → SchemaRegistry (+ Meta cache map) / dws schema wire projection
```

command/Leaf 不再写 `dws.schema.risk`；SafetySpec 走类型化 Final 载荷，不使用字符串枚举注解。

### 1.1 硬规则：声明 = 最终数据源（Schema 透传）

受管命令进入 Schema 的叶子数据由 **Contract 声明**定义最终值；框架（`corecmd.New`）做**类型转换**并注册，Schema 组装**透传**，不得：

- 把声明序列化成 JSON 注解再解析；
- 在声明体系里再挂「评审字段」并行权威；
- 用 hints/registry 盖写已声明字段。

**declare-vs-delivery 例外（Title / Description）**：构造期 `ContractDecl.Description` **必填**（声明证据），但这不是「declare = wire 最终值」。Catalog **交付**时：

- **description**：有 Cobra Long → 交付 Long（provenance `cobra_help` / `cobra_help_preferred`）；无 Long → 交付声明（`contract_final`）。**Short 不进入 description**。
- **title**：声明 `ContractDecl.Title` 优先，否则 Cobra Short，再 MCP；组装 stamp 真实 winner。

这是一条权威链上的显式交付偏好，不是双权威（见 RFC §5.0.4）。

迁移期未迁完的叶子可暂走旧组装路径；**新声明面不含 review_reason / reviewed 字段**。写命令未设 `Safety` 时，过渡期仍可用 `runtime_gate` annotate（`HOM-S2`）。

**不采用**路径 B（以 MCP meta / 已退役的 `schema_mcp_metadata` 生成全部 CLI flag/help/schema）作为主权威。钉钉 MCP meta 不是飞书 OAPI：粒度与 CLI 特有语义（二选一、OmitEmpty、ConstParams、write guard）无法从裸 meta 推出；强行生成会违反「Schema 描述 CLI，不制造 CLI」。

路径 B 仅允许作为 **可选的 1:1 MCP 透传叶子通道**（见 §5），不得覆盖 LeafSpec / Shortcut 主路径。

对外叙事：与飞书 **分层单权威** 同构——API/透传叶可用平台事实，产品 CLI / Shortcut 用手写契约 + 执行体——而不是「全家只有平台 meta」。

### 1.2 声明（declare）：写什么、写在哪、投影到哪

**定义**：声明 = 在 Contract 结构体的**数据字段**上写出事实；`corecmd.New` 据此注册 cobra、渲染 help、写入 `dws.schema.*`。钩子闭包（`Validate` / `Call` / `PostMount` / `RunE`）里的逻辑**不算**声明——即使行为正确，也不能单靠钩子让 Schema/help「猜出」该事实。

命令框架边界与今日/目标对应见 RFC [`rfc-command-framework-convergence.md`](rfc-command-framework-convergence.md) **§5.0**（框架上的「声明」定义）；本节给字段级表与示例。

**唯一写入面**（三选一，语义相同）：

| 入口 | 类型 | 归一 |
|---|---|---|
| Leaf 命令 | `helpers.LeafSpec` | `FromLeafSpec` → `corecmd.Spec` |
| Shortcut | Shortcut 声明（经 `FromShortcut`） | → `corecmd.Spec` |
| 直接基座 | `corecmd.Spec` | `corecmd.New` |

今日产品 CLI 叶子以 **`LeafSpec` 为声明门面**；字段与 `corecmd.Spec` 契约面一一对应。

#### 1.2.1 契约字段（算声明）

| 字段 | 声明什么 | 运行时 | 嵌入 Schema / help |
|---|---|---|---|
| `Flags[]`（`FlagSpec` / `LeafFlag`） | 用户可见参数面：名、类型、默认、必填、usage | 注册 cobra flag；装配 toolArgs | `dws.schema.property` / `type` / `required`；`--help` Flags |
| `Constraints[]` | 跨 flag 关系：`at_least_one` / `exactly_one` / `mutually_exclusive`；`custom` 记录钩子校验 | 通用关系由 `ValidateConstraints` 执行；`custom` 由 `Validate` 执行 | `dws.schema.constraints`；`--help`「参数约束」 |
| `Safety`（`contract.SafetySpec`） | effect/risk/confirmation/idempotency 四个独立事实 | `confirmation=user_required` 时 `ConfirmSafety`；`--yes` / `--dry-run` 跳过 | 同一个 SafetySpec 原样进入 Contract Final（`HOM-S1`） |
| `ConstParams` | 固定载荷（不上 flag 表） | 并入 toolArgs；不满足 Required | **不**投影为用户 parameter |
| `Use` / `Short` / `Long` / `Example` | 命令身份文案与示例 | cobra 自身 | help；identity 以 collector 收集的 `ContractFinal.Identity` 声明为准（reviewed registry 已退役） |

`FlagSpec` 子字段（声明细节）：

| 子字段 | 作用 | 是否进 Schema parameters |
|---|---|---|
| `Name`, `Usage`, `Kind`, `Default` | 注册名/说明/类型/DefValue | 是（name/type；default 与 cobra 对齐） |
| `Required` / `MarkRequired` | 非空校验 / cobra 硬必填 | 是（`required`） |
| `RequiredHint`, `Aliases`, `EnvVar` | 校验提示、隐藏别名、环境回退 | 否（执行细节；别名不上主 parameter 表） |
| `ArgDefault`, `Bind`, `OmitEmpty`, `Trim`, `Transform` | toolArgs 装配语义 | 否（载荷细节；`Bind` 可进 property 映射，但不另造 flag） |
| `Input` | 额外取值来源：`@path` 读文件 / `-` 读 stdin，在 required/enum/约束/`Validate` 之前原地解析 | 否（今日：能力由作者写进 `Usage` / `SchemaDescription` 文案，是已声明事实而非推断；不另造 flag。目标形态收敛为类型化投影字段，见 RFC §5.3） |

#### 1.2.2 编排 / 执行字段（不算声明）

| 字段 | 角色 | 禁止用来「冒充」的声明 |
|---|---|---|
| `Validate` | 条件式/领域校验钩子 | 不得在此 `Flags().String(...)` 注册业务 flag；不得只靠钩子表达「必填/互斥」而不写 `Flags`/`Constraints` |
| `Call` / `Invoke` / `Orchestrate` | 执行体 | 不得 `params[k]=…` 装配业务参数（应在 `Flags`/`ConstParams`） |
| `PostMount` | 挂载后收尾（annotate、领域工具注入） | 不得注册业务 flag；分页等横切由领域工具注入并可走 annotate |
| `RunE` | 逃生舱（整段手写） | 表面事实仍须 Flags 声明；框架仍按 Safety 执行确认 |
| `Server` / `Tool` | MCP 路由 | 不构成 CLI parameter 声明 |

#### 1.2.3 最小声明示例

```go
// 读：Safety 四字段显式对齐 Schema
NewLeafCommand(LeafSpec{
    Use: "get", Short: "…", Tool: "…",
    Safety: contract.SafetySpec{
        Effect: "read", Risk: "low",
        Confirmation: "not_required", Idempotency: "idempotent",
    },
    Flags: []LeafFlag{
        {Name: "unified-app-id", Usage: "…", Bind: "unifiedAppId", Trim: true, Required: true},
    },
    Call: devAppCall(runner), PostMount: devAppMeta(tool),
})

// 写：同一个 SafetySpec 同时驱动确认与 Schema
NewLeafCommand(LeafSpec{
    Use: "publish", Short: "…", Tool: "…",
    Flags: []LeafFlag{ /* … */ },
    Safety: contract.SafetySpec{
        Effect: "write", Risk: "medium",
        Confirmation: "user_required", Idempotency: "unknown",
    },
    Call: devAppCall(runner), PostMount: devAppMeta(tool),
})

// 迁移期旧写命令：确认走 annotate，见 §1.3 —— 不是新声明面
NewLeafCommand(LeafSpec{
    Use: "create", /* Flags… */,
    Validate: func(cmd *cobra.Command, _ []string) error {
        return devAppRequireWriteGuard(cmd, "create") // 执行守卫，不是 Contract 声明
    },
    Call: devAppCall(runner),
    PostMount: devAppMetaWrite(tool), // 人工标注 runtime_gate
})
```

**空 `Safety` 的含义**：command 为兼容旧只读叶保留 `read/low/not_required/idempotent` 默认。因此「会改状态却留空 Safety」**不是**合法声明；新 Leaf 必须写完整 SafetySpec，未迁移旧路径则按 §1.3 标注 gate。

### 1.3 人工标注（annotate）：声明的补充通道

当事实无法或不愿放进 Contract 字段时，必须**显式**落注解 / 评审源，禁止组装期推断：

| 标注手段 | 典型值 | 何时用 |
|---|---|---|
| `cli.AnnotateRuntimeGate` / `devAppMetaWrite` | `dws.schema.runtime_gate=devAppRequireWriteGuard` | 尚未迁移到 SafetySpec 的旧写命令（`HOM-S2`） |
| `cli.AnnotateRuntimeRisk` | `dws.schema.risk=…` | Shortcut 暂存的旧兼容路径；command/Leaf 禁止新增 |
| `cli.AnnotateRuntimeFlag` / Constraints | 与 embed 同形 | 手写 cobra 叶补齐表面（长期应迁入 Contract） |
| reviewed `schema_hints/metadata` Safety（已退役） | effect/risk/confirmation | 已删：`schema_hints/` 不得重现；受管命令以 Contract.Safety / gate 为准 |

标注与声明冲突时：**Contract 声明胜**（路径 A）。标注不得发明未注册的 CLI flag。

### 1.4 Schema 全覆盖（`ToolSpec` 无空洞）

`dws schema` 叶子模型是 `cli.ToolSpec`。命令框架必须为**每一个字段组**指定权威；完整矩阵在 RFC **§5.0.4**，摘要：

| ToolSpec 组 | 权威类 | 框架声明字段 / 其它源 |
|---|---|---|
| Identity | 声明源（identity collector 收集 `ContractFinal.Identity`；reviewed `schema_command_registry` 已退役） | `ContractDecl.Identity` 声明 |
| Display / Title / Description | 声明证据 + 交付偏好（非双权威） | **title**：ContractDecl 优先，否则 Cobra Short，再 MCP；**description**：构造期 Description 必填；Catalog 交付 Long→`cobra_help`，无 Long→`contract_final`；**Short 不进 description**（RFC §5.0.4） |
| Parameters.`name/type/required/default/property` | **声明**（或同形 annotate） | `Flags` / `Bind` |
| Parameters.`description` | 声明 usage（`FlagSpec.Usage` / ParamDecl） | `schema_hints/` 已退役；不得用 overlay 改 type/required/default |
| Parameters.`interface_*` | 评审源 | MCP meta / bindings；**不造 flag** |
| Constraints | **声明** | `Constraints` |
| Positionals | **声明** 或显式 annotate | 目标 `Args`；禁止推断 |
| Safety.`effect/risk/confirmation/idempotency` | **声明**完整 `Safety`，或迁移期 `runtime_gate` / reviewed Safety | 四字段独立；不得互相推导 |
| DryRun | 评审源 | dry-run capabilities registry |
| Interface | 评审源 | MCP + 内存 inject 的 Agent metadata |
| Selection | 声明（ContractFinal / ProductDecl） | `ContractDecl.Selection` / `ProductDecl` |
| FieldProvenance / Extensions | 组装派生或评审扩展 | 组装器；与 delivered value 一致 |
| （非 Schema parameter）ConstParams | **声明** | 载荷；不上 parameters 表 |

验收：新增 Schema 字段必须同步改 RFC §5.0.4 + 本表；受管写命令 Safety 不得无主。

## 2. 字段归属（每一类恰好一个写入者）

| 字段类 | 权威 | 投影到 |
|---|---|---|
| flags / defaults / required / enum / 关系约束 / 运行时 Risk | Contract（LeafSpec / `corecmd.Spec` 门面） | cobra、`--help`、Schema `parameters` / constraints / confirmation |
| ConstParams、Bind、OmitEmpty、Transform | 同上（载荷声明，不上 flag 表） | toolArgs；Schema 不把 ConstParams 伪装成用户 flag |
| canonical path / aliases / navigation / exposure | identity collector 收集的 `ContractFinal.Identity` 声明（reviewed `schema_command_registry` 已退役） | Schema identity |
| use_when / avoid_when / examples / agent_summary 文案 | `ContractDecl.Selection` / `ProductDecl` | Schema selection |
| RPC tool 形状、`interface_ref`、interface 描述 | leaf `Contract.Interface` + `ParamDecl`（`schema_parameter_mapping_ledger.go` 仅 mapping_exclusions / removals；`schema_mcp_metadata` 已退役） | Schema `interface_*` 字段；**不得创建 flag** |
| 参数描述 overlay（可选） | 生产 metadata 壳为空；参数事实走 ParamDecl / FlagSpec | **Contract/cobra 胜** |
| 遗留 Safety 文案（迁移期） | 生产 metadata 壳为空；Safety 走 Contract | 以 Contract.Safety / runtime_gate 为准（见 §4） |
| dry-run 正能力 | reviewed dry-run registry | Schema `dry_run` |
| positionals | Contract Args / 显式 annotate | Schema `positionals` |
| FieldProvenance | 组装派生 | Schema provenance（与值一致） |

Selection **刻意不**由单命令 Contract 取代（RFC 决策 8 / schema 设计硬规则）；identity 的历史决策是不进 Contract、归 reviewed `CommandRegistry`，该 registry 已退役，现由 identity collector 收集 `ContractFinal.Identity` 声明（声明即 identity）。**完整无空洞表见 RFC §5.0.4。**

## 3. 当前缺口与目标闭环

已具备：

- Flags / ConstParams / Constraints → 注册、校验与 `ConstraintHelp`；SafetySpec → 运行时 `ConfirmSafety`（command）；
- Call / Execute 作为执行体；业务参数不得在 Call 内装配（helpers 门禁）；
- **Contract → Schema 嵌入**：参数/约束写原生 annotation，SafetySpec 与 ContractDecl 注册为类型化 Contract Final 并由 Schema 组装透传；
- Selection 权威为 `ContractDecl.Selection` / `ProductDecl`（`contract_final`）；`schema_hints/` 已退役。

已进 CI（`make policy` → `check-schema-catalog.sh` / `check-runtime-confirmation-truth.sh`）：

| Gate ID | CI 入口（`-run` 白名单 / 脚本） |
|---|---|
| `HOM-P1` / `HOM-D1` | `./internal/app`：`TestFinalSchemaParametersMatchExecutableHelpFlags`、`TestDeliverySchemaParametersMatchExecutableHelpFlags` |
| `HOM-P2`（参数映射/bindings 子集） | `./internal/cli`：`TestSchemaParameterBindingsMatchReviewedBaselineAndDeliveryCatalog`、`TestDeliveryCatalogMCPParameterMappingsAreComplete` 等 bindings 门禁 |
| `HOM-S1` / `HOM-S2`（confirmation 同源） | `./internal/cli/homology`：`TestUserRequiredSafetyHomologyWithRuntimeGate` + `check-runtime-confirmation-truth.sh`；`./internal/app`：`TestSheetFinalSchemaConfirmationMatchesRuntimeGuards` |
| 词汇/决策钉扎 | `./internal/cli/homology`：`TestHomologyDecisionDocPinsPathAAndGateIDs`、`TestMCPPassthroughAdmissionExcludesLeafAndShortcut`、`TestHomologyCIEntrypointsPinned` |

仍缺（未宣称全量 CI 覆盖）：

1. 独立可执行的 `HOM-P3`（constraints ≡ AnnotateConstraints）与 `HOM-S3`（read 不得误投影 user_required）全量 gate；
2. `HOM-I1` 作为单独 gate ID 的显式用例（MCP bindings ⊆ Contract flags 已有映射审计子集，但未钉 `HOM-I1` 标签）。

已落地（写命令确认语义，`HOM-S2`）：

- `AnnotateRuntimeGate` / `dws.schema.runtime_gate`；Leaf `PostMount: devAppMetaWrite`；手写 delete/robot 等同路径显式标注；
- Schema 组装在无 Contract Safety 但有 gate 时 overlay `confirmation=user_required`（`applyContractGateToSafety`）；
- AST/mount 测试：新 Leaf 须声明完整 SafetySpec；尚未迁移的旧路径须有 runtime_gate。

## 4. Schema 投影与 Safety 门禁规划

以下门禁 ID 稳定，便于 CI 认领。§3 表标明哪些已挂入 `check-schema-catalog.sh`。

| Gate ID | 断言 | 范围 | CI |
|---|---|---|---|
| `HOM-P1` | 受管 leaf 的 schema `parameters[].name` 集合 ≡ cobra 本地 flag 名集合（排除全局 persistent） | LeafSpec / Contract 编译命令 | **已进**（app help↔schema） |
| `HOM-P2` | schema parameter `type` / `required` / `default` 与 cobra DefValue / MarkFlagRequired / FlagSpec 一致；不得用已退役 hints overlay 改写这三项 | 同上 | **部分**（bindings/mapping 门禁） |
| `HOM-P3` | schema 关系约束（require_one_of / mutually_exclusive）≡ Contract/Leaf `Constraints` 投影（与 `AnnotateConstraints` 同构） | 声明了 Constraints 的命令 | 规划 |
| `HOM-S1` | Contract/Leaf `user_required` Safety 与运行时 Confirm/gate 同源，且 help Safety 行同语义 | 受管写/破坏性命令 | **已进** |
| `HOM-S2` | 若命令走显式 write guard（如 `devAppRequireWriteGuard`）而非完整 SafetySpec，则必须人工标注 `dws.schema.runtime_gate`；Schema 不得呈 `confirmation=not_required`；符合 §1.1 declare OR annotate | 今日 devapp 写命令 | **已进**（同源测试含 gate 路径） |
| `HOM-S3` | `Risk=read`（或空→read）不得投影为 `user_required`，除非有 reviewed exclusion reason | 受管读命令 | 规划 |
| `HOM-I1` | `interface_ref` 存在时，bindings 覆盖的 CLI flag ⊆ Contract flags；MCP meta **不**引入额外 CLI flag | 有 MCP 绑定的命令 | **部分**（mapping 审计） |
| `HOM-D1` | `dws <path> --help` Flags 段与 schema leaf parameters 零未解释增量 | 受管公开 leaf | **已进**（与 HOM-P1 同测） |

落地顺序建议：

1. 保持 `HOM-P1`/`HOM-D1`/`HOM-S1`/`HOM-S2` 在 `check-schema-catalog.sh` 白名单中（勿再只靠词汇钉扎）；
2. 补独立 `HOM-P3` / `HOM-S3` 可执行 gate，并把 `HOM-P2`/`HOM-I1` 从「部分」升到全量标签；
3. 新 Leaf 继续走 Contract 嵌入；禁止 `schema_hints/` 回潮。

## 5. 路径 B 子通道：1:1 MCP 透传叶（可选，非主路径）

仅当同时满足以下条件时，才允许「从 MCP meta 生成 flag/help/schema 参数」：

1. CLI path ↔ 单一 MCP tool **严格 1:1**，无多步、无按名解析、无本地 effect；
2. 无不在 MCP input schema 中的 CLI 特有 flag（含 guard 专用语义 flag 除外的全局 `--yes`/`--dry-run`）；
3. 无 ConstParams / Transform / 跨 flag 约束 / Call 内业务逻辑；
4. Risk/confirmation 在 meta 或并列 reviewed Safety 中有显式来源，不靠生成器猜测；
5. 在 identity 声明面标记 `surface_kind=mcp_passthrough`（名称可调整；原计划标在 reviewed registry，该 registry 已退役），且 **不得**与 LeafSpec/Shortcut 手写定义双注册同一 `cli_path`；
6. 文档与门禁写明：该通道是子集优化，失败时回退/禁止扩张到产品 CLI。

显式排除（永远走路径 A / Shortcut）：

- 全部 `LeafSpec` 命令（含 `dws dev app …`）；
- 全部 `+shortcut` 与 smart 编排；
- 任何需要 `devAppRequireWriteGuard`、cursor 工具注入、或响应投影的命令。

## 6. 非目标

- ~~不把 canonical identity / 导航塞进 Contract（仍归 reviewed `CommandRegistry`）~~ —— 该非目标已被后续演进取代：reviewed `CommandRegistry` 已退役，identity 现由 collector 收集 `ContractFinal.Identity` 声明（声明即 identity，不再是独立评审文件）。selection 文案已由 `ContractDecl.Selection` / `ProductDecl` 声明（非 hints）。
- 不要求删除 LeafSpec 门面。
- 不把「生成 catalog 字节一致」当作运行时同源的充分条件（仍需 `HOM-*` 与差分门禁）。
