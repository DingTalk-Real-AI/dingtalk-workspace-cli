# 声明式命令框架迁移指南

- **受众**：把 helpers 叶子 / Shortcut 从 Tier3 / 遗留 annotate 迁到声明式框架的工程师
- **规范权威**（本文不重复政策全文，只给可执行路径）：
  - [`rfc-command-framework-convergence.md`](rfc-command-framework-convergence.md) **§5.0**（声明定义、三档路径、Schema 字段权威）
  - [`flag-help-schema-homology.md`](flag-help-schema-homology.md)（路径 A：Contract 嵌入 Schema）
  - 仓库根 [`AGENTS.md`](../AGENTS.md)（Authoring tiers、Agent Schema contract、curation）
- **架构速览**：[`command-framework-architecture.md`](command-framework-architecture.md)
- **实现基线**：PR #830 起 `helpers.LeafSpec` / `shortcut.Shortcut` → `corecmd.Spec` → `corecmd.New`；Schema 经 `RegisterSchemaSourceRoot` → `ResolveSchemaBuild` 运行时组装（**声明即 Catalog**）

---

## 1. Why / 终态

目标不是「再写一份 Catalog JSON」，而是 **一份声明同时驱动**：

1. Cobra 可执行表面（flags / required / constraints / help）
2. 运行时确认门（`confirmation=user_required` → `ConfirmSafety`）
3. Agent Schema（`ToolSpec` / `ResolveMeta`）

**单向数据流（摘要）**：

```text
leaf Safety + Contract / ParamDecl / ProductDecl
  → ContractFinal（corecmd.New 或 AttachContract / DeclareLeafMetadata）
  → CollectIdentitySpecs（活 Cobra 叶上的 Identity）
  → ResolveSchemaBuild → SchemaRegistry
  → dws schema / ResolveMeta（同一组装结果）
```

硬约束：

| 要做 | 不要做 |
|---|---|
| 在叶子旁声明 `Safety` / `Contract` / `ParamDecl` | 提交 `schema_catalog/`、`schema_meta_index.*`、`schema_agent_metadata/`、`schema_hints/`、`schema_mcp_metadata.json` |
| 用 `make generate-schema` 刷新 param aliases + 证明组装确定性 | 把 Catalog dump 当交付权威或手工改 wire |
| help/Schema 事实 **declare OR annotate** | 纯推断、或钩子闭包里「顺带」发明表面 |
| `Confirmation` 单独驱动运行时门 | 从 `effect`/`risk` 机械推导 `confirmation` |

`cmd_schema_catalog` 仅供 CI/local dump；生产路径没有 committed Catalog pin。

---

## 2. Authoring tiers（何时用哪一档）

同一 `ContractFinal` 语义；三档不是互相否定。详见 RFC §5.0.2a。

| 档 | 入口 | 声明什么 | 执行面 | 何时用 |
|---|---|---|---|---|
| **Tier1** | `corecmd.New` / `helpers.NewLeafCommand(LeafSpec)` | `Flags` / `Constraints` / `Safety` / `ConstParams` / `Contract` 全进 Spec | 框架注册 flag、投影参数、`ConfirmSafety`、派发 | **新命令默认**；执行面可迁入 LeafSpec/`Call` |
| **Tier2** | `helpers.DeclareLeafMetadata(cmd, LeafSpec)` | 仅 `Safety` + `Contract`（可选 `Validate`） | **不**注册 flag；确认挂 RunE 包装器 | helpers / Shortcut 迁移态：先补 Agent Schema，执行面暂时手写 `RunE`/`Execute` |
| **Tier3** | 裸 `*cobra.Command` | 无框架声明（或仅 annotate / 精确排除） | 调用方自管 | 应收窄；新增裸叶须迁元数据或进 `schema_command_exclusions.go` |

选用规则（今日）：

1. 新命令 → **Tier1**。
2. 既有叶子要进 Schema、但 `RunE`/`Execute` 还不想重写 → **Tier2**（声明写在命令字面量旁）。
3. **Shortcut + `DeclareLeafMetadata` 合法**；多数 Shortcut 已走 `FromShortcut` → `corecmd.New`（Tier1 表面 + 自有 `Execute`）。
4. Tier2 → Tier1 与 Shortcut → mcpbind 是后续里程碑，**不是**本阶段硬门槛；不得用 Tier2 绕开「业务 flag 必须声明」的纪律（半接管字段会 panic）。

`DeclareLeafMetadata` 禁止传入：`Flags` / `Constraints` / `ConstParams` / `Call` / `RunE` / `PostMount` / `ConfirmFirst` / `Server`/`Tool`；**唯一允许的执行钩子是 `Validate`**。

---

## 3. What to declare（声明清单）

### 3.1 框架声明面（数据字段才算声明）

| 字段 | 作用 | Schema / 运行时 |
|---|---|---|
| `Flags`（Tier1） | kebab-case flag、Kind、Default、Required、`Bind`、Aliases… | parameters；`Bind` → property |
| `Constraints`（Tier1） | `at_least_one` / `exactly_one` / `mutually_exclusive` / custom | constraints 段 + 运行时校验 |
| `Safety`（`contract.SafetySpec`） | `effect` / `risk` / `confirmation` / `idempotency` **四字段齐全** | ToolSpec Safety；仅 `confirmation` 驱动确认门 |
| `ConstParams`（Tier1） | 固定 toolArgs，不上用户 flag 表 | 载荷；非 parameters |
| `Contract`（`corecmd.ContractDecl`） | Identity / Description / Selection / Parameters / Interface / DryRun | ContractFinal → Catalog |
| `ProductDecl` | 产品级 routing（`contract.RegisterProductDecl`） | 产品 overview selection |

钩子（`Validate` / `Call` / `Invoke` / `Orchestrate` / `RunE` / `Execute` / `PostMount`）**不算**声明——行为正确也不能替代 Schema 事实。

### 3.2 Contract 必填要点

- `Identity`：`ProductID` / `Name` / `CanonicalPath` / `CLIPath` / `PrimaryCLIPath`（与活 Cobra 路径一致；collector 以此为唯一 identity 源）
- `Description`：构造期必填（声明证据）；Catalog **交付**可优先 Cobra Long（provenance `cobra_help`）
- `Selection`：决策向 `AgentSummary` / `UseWhen` / `AvoidWhen` / `Examples`（勿复述 Short；**禁止** `Reviewed` 字段）
- `Parameters`（`[]contract.ParamDecl`）：CLI flag 名 → RPC `Property`；`interface_type` 等接口事实写这里，不写 MCP pin
- `Interface`：`mcp` / `composite` / `local` + `Ref` 或 `Reason`

### 3.3 ProductDecl

在产品根命令构造处注册一次，例如 `internal/helpers/drive.go` 的 `newDriveCommand`：

```go
contract.RegisterProductDecl(contract.ProductDecl{
	ID: "drive",
	Selection: contract.ProductSelectionDecl{
		AgentSummary: "…",
		UseWhen:      []string{"…"},
		AvoidWhen:    []string{"…"},
	},
})
```

---

## 4. 分步迁移清单

### 4.1 典型 helpers 叶子（裸 Cobra → Tier2）

1. **确认 CLI 路径**：`dws <path> --help`，记下 flag 名与 required 组。
2. **选定 Identity**：`product_id` / `name` / `canonical_path` / `cli_path` 与兄弟命令不冲突；aliases 若有则写进 Identity（与 collected registry 一致）。
3. **声明 Safety**：写操作填完整四字段；删除/破坏性用 `confirmation=user_required`（见 §5）。
4. **声明 Contract**：`Description` + `Selection` + `Interface`；flag→RPC 用 `Parameters: []ParamDecl{{Name, Property}}`。
5. **挂元数据**：在 flag 注册与 `RunE` 之后调用：

```go
DeclareLeafMetadata(cmd, LeafSpec{
	Safety:   contract.SafetySpec{ /* 四字段 */ },
	Contract: LeafContract{ /* Identity / Selection / … */ },
	// 本地副作用且 user_required：补 Validate，避免确认抢先或 fail-closed
})
```

6. **Property 缺口**：CLI 有、RPC 无的 selector → `schema_parameter_mapping_ledger.go` 的 `mapping_exclusions`（精确键 + 非空 reason），不要造 binding JSON。
7. **产品路由**：若产品尚无 `ProductDecl`，在产品根补注册。
8. **验证**（§7）后再开 PR。

参考：`internal/helpers/drive.go`（`DeclareLeafMetadata` + `RegisterProductDecl`）。

### 4.2 典型 Shortcut（补 Safety + Contract）

1. Shortcut 已由 `FromShortcut` → `corecmd.New` 接管 flag/约束/确认；迁移焦点是 **显式 `Safety` + `Contract`**，不要再依赖仅 `Risk` 推断。
2. 在 `shortcut.Shortcut{…}` 字面量上增加：

```go
Safety: contract.SafetySpec{
	Effect: "read", Risk: "low",
	Confirmation: "not_required", Idempotency: "idempotent",
},
Contract: corecmd.ContractDecl{
	Identity:    contract.ToolIdentitySpec{ /* … */ },
	Description: "…",
	Interface:   &contract.InterfaceSpec{Mode: "composite", Availability: "available", Reason: "…"},
	Selection: contract.SelectionSpec{
		AgentSummary: "…",
		UseWhen:      []string{"…"},
		AvoidWhen:    []string{"…"},
		Examples:     []string{"dws chat +conversation-info --group <openConversationId>"},
	},
},
```

3. `Flags` / `Constraints` / `Validate` / `Execute` 保持 Shortcut 词汇；adapter 映射到 `corecmd.Spec`。
4. Examples 用真实可执行路径与 flag；**不要**写 `--yes`。
5. 验证（§7）。

参考：`internal/shortcut/chat/chat_conversation.go`（`ConversationInfo`）；adapter：`internal/shortcut/adapter.go`。

### 4.3 新命令 / 可重写执行面 → Tier1

优先 `NewLeafCommand(LeafSpec{ Flags, Safety, Contract, Call/… })`，例如 `internal/helpers/devapp.go`。业务 flag 只进 `Flags`，禁止在 `PostMount`/`Validate` 里 `Flags().String` 注册业务面。

---

## 5. Safety / confirmation

### 5.1 声明完整 SafetySpec

```go
Safety: contract.SafetySpec{
	Effect:       "destructive", // read | write | destructive
	Risk:         "high",        // low | medium | high
	Confirmation: "user_required", // not_required | user_required
	Idempotency:  "unknown",     // idempotent | retryable | non_idempotent | unknown
},
```

非空则四字段必须齐全（构造期校验）。字段独立：`effect=destructive` **不会**自动变成 `user_required`。

### 5.2 迁离 AnnotateRuntimeRisk / AnnotateRuntimeGate

| 旧路径 | 新路径 |
|---|---|
| `AnnotateRuntimeRisk` / 字符串 `dws.schema.risk` | 声明 `Safety.Risk`（经 ContractFinal） |
| `AnnotateRuntimeGate` / `runtime_gate`（如 `devAppRequireWriteGuard`） | 声明 `Safety.Confirmation=user_required`；框架 `ConfirmSafety` |
| 仅 Shortcut `Risk=write` 隐式确认 | 显式 `Safety`（覆盖 Risk 展开） |

**禁止新增**生产 `AnnotateRuntimeRisk` / `AnnotateRuntimeGate` 调用点；存量 annotate 可保留至迁完（`HOM-S2`）。同源门禁：`confirmation=user_required` ↔ 运行时 gate（见 `check-runtime-confirmation-truth.sh`、`internal/cli/homology`）。

### 5.3 Tier2 确认时机

- 有 `Validate`：`Validate` → `ConfirmSafety` → 原 `RunE`
- 无 `Validate`：确认推迟到首次 `CallTool`；无 Caller 的本地副作用叶必须补 `Validate`，否则 fail-closed

执行前需确认时，用 `--yes` 跳过交互；**Schema examples 永不包含 `--yes`**。

---

## 6. Parameters / mapping

1. **主权威**：Tier1 用 `FlagSpec.Bind`（空则 Name）；Tier2 / 混合路径用 `contract.ParamDecl{Name, Property}`。
2. **mapping ledger**（`internal/cli/schema_parameter_mapping_ledger.go`）：只放 `mapping_exclusions` / removals——CLI flag **无**直接 RPC property 时的精确评审排除；非空 reason。
3. **不要**：提交 MCP pin（`schema_mcp_metadata.json` 已退役）；不要指望 live MCP 创建 CLI flag；不要用 hints overlay 改 `type`/`required`/`default`。
4. **required 地板**：Cobra `MarkFlagRequired` 不得被低优先级源降为 optional。
5. 接口事实（`interface_ref` / `interface_type`）声明在 leaf `Contract.Interface` / `ParamDecl`；CLI path ≠ MCP path 时用 `Interface.Ref`（例：`drive delete` → `doc.delete_document`）。

---

## 7. Schema 验证

迁移后至少跑：

```bash
make generate-schema
./scripts/policy/check-schema-catalog.sh
./scripts/policy/check-runtime-confirmation-truth.sh   # 若动了 confirmation / gate
# 聚焦：
go test ./internal/app -run 'TestSheetFinalSchemaConfirmationMatchesRuntimeGuards|TestFinalSchemaParametersMatchExecutableHelpFlags' -count=1
```

手工抽查：

```bash
dws schema --cli-path "drive mkdir" -f json
# 或 ResolveMeta 同源：help Safety 行与 Schema confirmation 一致
dws drive mkdir --help
```

Examples 规则（组装/门禁）：

- 每 tool 最多两条；路径与 flag 必须是活 Cobra 可接受的
- **禁止** `--yes`；禁止 shell 注释
- 缺必填 / 约束失败 = 契约 bug，不是「跳过 example」的理由

可选：`make test-schema-agent-examples`（合同 + 显式 dry-run 能力子集）。

---

## 8. Pitfalls（常见坑）

1. **Schema source root = declarationOnly**  
   `app.NewSchemaSourceRootCommand` 以 `declarationOnly=true` 建树：**跳过** `helpers.InitDeps` / `injectStaticServers`，避免组装时清掉进程里的 ToolCaller / plugin endpoint。叶子构造与 RunE 包装必须对 `deps == nil` 安全（见 `printDocDeprecationWarning` 注释，`internal/helpers/doc.go`）。

2. **DryRun `RemoteReads`**  
   声明 `Contract.DryRun` / `DryRunSpec` 时，`RemoteReads: false` 表示预览计划不发起远程读；勿把「有 dry-run 能力」与「会打后端」混为一谈。无 reviewed dry-run 能力时，example 门禁不会魔法升级为 runtime dry-run。

3. **不要复活退役目录 / pin**  
   `schema_hints/`、`schema_agent_metadata/`、`schema_command_registry/`、committed `schema_catalog/`、`schema_mcp_metadata.json` —— 出现即 policy 失败。

4. **Tier2 半接管**  
   往 `DeclareLeafMetadata` 塞 `Flags`/`Call`/`RunE` 会 panic；要框架管 flag → 升 Tier1。

5. **Identity 漂移**  
   `Contract.Identity` 必须与活路径一致；不一致组装失败。排除项用 `schema_command_exclusions.go` 精确路径 + reason，禁止前缀通配。

6. **Selection 里塞 `Reviewed`**  
   旧 hints 标记；声明载荷携带即组装报错。

7. **在钩子里发明 flag/property**  
   Schema 看不到；同源门禁（`HOM-P*` / `HOM-D1`）会打回来。

---

## 9. Worked mini-example（前后对照）

### 9.1 helpers：裸叶 → Tier2（模式摘自 `drive mkdir` / `drive delete`）

**Before（概念）**：手写 `cobra.Command` + `RunE` + `Flags().String`，无 `Safety`/`Contract` → Agent Schema 缺叶子或靠已退役 hints。

**After（Tier2，缩写）**：

```go
driveMkdirCmd := &cobra.Command{
	Use:   "mkdir",
	Short: "创建文件夹",
	RunE:  /* 既有 callMCPTool("create_folder", …) */,
}
driveMkdirCmd.Flags().String("name", "", "…")
driveMkdirCmd.Flags().String("folder", "", "…")

DeclareLeafMetadata(driveMkdirCmd, LeafSpec{
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium",
		Confirmation: "not_required", Idempotency: "unknown",
	},
	Contract: LeafContract{
		Identity: contract.ToolIdentitySpec{
			ProductID: "drive", Name: "create_folder",
			CanonicalPath: "drive.create_folder",
			CLIPath: "drive mkdir", PrimaryCLIPath: "drive mkdir",
		},
		Description: "创建文件夹",
		Interface: &contract.InterfaceSpec{
			Mode: "mcp", Availability: "available",
			Ref: &contract.InterfaceRefSpec{ProductID: "drive", RPCName: "create_folder"},
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "创建文件夹",
			UseWhen:      []string{"用户要在钉盘下新建普通文件夹时"},
			AvoidWhen:    []string{"知识库内建文件夹改用 dws wiki node create …"},
			Examples:     []string{`dws drive mkdir --name "项目资料" --format json`},
		},
		Parameters: []contract.ParamDecl{
			{Name: "folder", Property: "parentId"},
		},
	},
})
```

破坏性写示例（`drive delete`）：`Confirmation: "user_required"`，`Effect: "destructive"`，`Interface.Ref` 可指向 `doc`/`delete_document`。

路径：`internal/helpers/drive.go`。

### 9.2 Tier1 LeafSpec（`dev app event list` 缩写）

```go
return NewLeafCommand(LeafSpec{
	Use: "list", Short: "查询应用已订阅的事件列表",
	Tool:   devAppEventListTool,
	Safety: /* read / low / not_required / idempotent */,
	Flags: []LeafFlag{
		{Name: "unified-app-id", Bind: "unifiedAppId", Required: true, Trim: true},
		{Name: "keyword", Bind: "keyword", OmitEmpty: true, Trim: true},
	},
	Contract: LeafContract{ /* Identity + Selection + Interface */ },
	Call:     devAppCallCursor(runner),
})
```

路径：`internal/helpers/devapp.go`。

### 9.3 Shortcut（`chat +conversation-info`）

见 §4.2；完整字面量：`internal/shortcut/chat/chat_conversation.go`。`FromShortcut` 把 `Flags`/`Constraints`/`Safety`/`Contract` 编进 `corecmd.Spec`（`internal/shortcut/adapter.go`）。

---

## 10. Done criteria（完成标准）

迁移 PR 合并前，下列应满足：

| 检查项 | 证据 |
|---|---|
| 叶子有完整 `Safety`（或存量 `runtime_gate` 未扩大） | `dws schema --cli-path "…" -f json` 四字段与声明一致 |
| `user_required` ↔ 运行时确认 | `./scripts/policy/check-runtime-confirmation-truth.sh`；相关 `HOM-S1`/`HOM-S2` |
| Identity / 路径可执行且进 Schema（或精确 exclusion） | `CollectIdentitySpecs` / reverse-completeness；无前缀排除 |
| parameters ⊆ help flags；property 有声明或 mapping exclusion | `check-schema-catalog.sh`；`HOM-P1`/`HOM-D1` |
| 无退役 pin / hints / agent_metadata / committed catalog | policy 脚本；`git status` 无这些路径 |
| `make generate-schema` 通过（aliases + 组装确定性） | CI / 本地 |
| Examples 可执行、无 `--yes` | 组装 example 门禁；必要时 `make test-schema-agent-examples` |
| gofmt；未无关改动 | PR diff |

长期可选（非本阶段硬门槛）：Tier2 → Tier1；Shortcut `Execute` 中「只为装配参数」的函数体收敛到 mcpbind 形态 1/2（RFC §3.5 / §5.0.2a.5）。

---

## 附录：相关文件速查

| 主题 | 路径 |
|---|---|
| Leaf Tier1/Tier2 API | `internal/helpers/leaf.go` |
| Leaf → corecmd | `internal/helpers` `FromLeafSpec`；`internal/corecmd` |
| Shortcut → corecmd | `internal/shortcut/adapter.go` |
| Safety / Contract DTO | `internal/corecmd/contract` |
| ContractFinal 注册 | `internal/corecmd/contractfinal` |
| Schema 组装入口 | `internal/cli`：`RegisterSchemaSourceRoot`、`ResolveSchemaBuild`、`ResolveMeta` |
| 精确排除 | `internal/cli/schema_command_exclusions.go` |
| mapping exclusions | `internal/cli/schema_parameter_mapping_ledger.go` |
| declaration-only 根 | `internal/app.NewSchemaSourceRootCommand` |
