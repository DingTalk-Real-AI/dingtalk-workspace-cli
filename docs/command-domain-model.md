# command 领域模型

本文档描述 `internal/corecmd` 包的领域模型——类型、概念及其关系。

## 核心模型图

```
┌─────────────────────────────────────────────────────────────────────┐
│                         corecmd.Spec                                  │
│                    (一个叶子命令的完整契约)                             │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  ┌─── CLI 表面 ───┐   ┌─── 参数声明 ───────────────────────────┐   │
│  │ Use            │   │ FlagSpec[]                              │   │
│  │ Short          │   │  ├─ Name / Kind / Default              │   │
│  │ Long           │   │  ├─ Required / MarkRequired            │   │
│  │ Example        │   │  ├─ Aliases[] / EnvVar (回退链)         │   │
│  └────────────────┘   │  ├─ Bind / Transform / OmitEmpty       │   │
│                       │  └─ Enum / Format / SchemaDescription  │   │
│                       │                                        │   │
│                       │ Constraint[]                            │   │
│                       │  ├─ at_least_one                       │   │
│                       │  ├─ exactly_one                        │   │
│                       │  └─ mutually_exclusive                 │   │
│                       │                                        │   │
│                       │ ConstParams map[string]any             │   │
│                       └────────────────────────────────────────┘   │
│                                                                     │
│  ┌─── 安全模型 ──────────────────────────────────────────────────┐  │
│  │  Safety contract.SafetySpec                                       │  │
│  │  ├─ Effect       (read / write / destructive)                │  │
│  │  ├─ Risk         (low / medium / high)                       │  │
│  │  ├─ Confirmation (not_required / user_required) ──▶ 运行时门 │  │
│  │  └─ Idempotency  (idempotent / retryable / …)                │  │
│  │                                                               │  │
│  │  四字段彼此独立；同一值同时供运行时与 Schema 使用              │  │
│  │  ConfirmFirst: bool (只控制确认门顺序)                         │  │
│  └───────────────────────────────────────────────────────────────┘  │
│                                                                     │
│  ┌─── Contract 声明 (Agent 可见的元数据) ────────────────────────┐  │
│  │  ContractDecl                                                   │  │
│  │  ├─ Title / Description                                      │  │
│  │  ├─ contract.DryRunSpec     {PreviewKind, RemoteReads}       │  │
│  │  ├─ contract.InterfaceSpec  {Mode, Availability, Reason, Ref}│  │
│  │  ├─ contract.SelectionSpec  {AgentSummary, UseWhen, AvoidWhen│  │
│  │  │                          Prerequisites, Tips, Examples}    │  │
│  │  ├─ contract.ToolIdentitySpec {ProductID, CanonicalPath, …}  │  │
│  │  └─ Positionals[]  {Name, Type, Required, Variadic}          │  │
│  └───────────────────────────────────────────────────────────────┘  │
│                                                                     │
│  ┌─── 执行体 (恰好一个) ─────────────────────────────────────────┐  │
│  │  Invoke(Ctx, toolArgs)     ← #830 过渡：单步派发（目标 mcpbind）│  │
│  │  Orchestrate(Ctx)          ← #830 过渡：多步编排（目标 Handler）│  │
│  │  RunE(cmd, args)           ← 逃生舱：完全自定义               │  │
│  └───────────────────────────────────────────────────────────────┘  │
│                                                                     │
│  ┌─── 钩子 ─────────────────────────────────────────────────────┐  │
│  │  Validate(cmd, args)   ← 条件式业务校验（约束表达不了的）      │  │
│  │  PostMount(cmd)        ← 挂载收尾（设置 Args 等 cobra 属性）   │  │
│  └───────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────┘
```

## 构建与执行流

```
corecmd.Spec ──── corecmd.New() ────▶ cobra.Command ──── 用户执行 ────▶ Ctx
                     │                    │                            │
              构建时检查:              注册产物:                    执行上下文:
              • validateDispatchDecl   • Flags + Aliases            • Str(name)
              • validateSafetySpec     • Annotations (Schema)       • Int(name)
              • validateContractDecl     • Long (约束 help)           • Bool(name)
              • RegisterFlags          • RunE (管线)                • StrSlice(name)
              • ValidateConstraintDecls                            • Changed(name)
              • embedContractIntoSchema                            • DryRun() / Yes()
              • AnnotateConstraints
              • PostMount
```

## 领域概念

| 概念 | 类型 | 职责 |
|------|------|------|
| **corecmd.Spec** | struct | 一个命令的完整契约（声明 + 执行） |
| **FlagSpec** | struct | 一个参数的注册、回退链、绑定规则 |
| **Constraint** | struct | 参数间的关系约束 |
| **Safety** | contract.SafetySpec | 运行时与 Schema 共用的安全契约 |
| **ContractDecl** | struct | Agent 可见的完整工具规格声明 |
| **contract.SelectionSpec** | struct | Agent 选择该工具的语义指引 |
| **contract.InterfaceSpec** | struct | 工具的接口模式与可用性 |
| **contract.DryRunSpec** | struct | dry-run 能力声明 |
| **contract.ToolIdentitySpec** | struct | 工具在注册表中的身份标识 |
| **contract.RuntimeSchemaPositional** | struct | 有序位置参数声明 |
| **Ctx** | struct | 执行上下文（类型安全的 flag 读取） |
| **New** | func | 统一构建器（`corecmd.Spec` → `*cobra.Command`） |

## SafetySpec

`corecmd.Spec.Safety` 使用 `corecmd/contract.SafetySpec`（无 cli 类型别名），没有 command 自定义 Risk/Safety 枚举，也没有 `SafetyDecl` 覆盖层：

```go
Safety: contract.SafetySpec{
    Effect:       "write",
    Risk:         "high",
    Confirmation: "user_required",
    Idempotency:  "unknown",
}
```

- `Confirmation == "user_required"` 时执行确认；`--yes` 和 `--dry-run` 可跳过交互。
- `Effect`、`Risk`、`Idempotency` 不参与确认决策，也不会改写 `Confirmation`。
- 任意一个字段非空时，四个字段必须全部显式声明；构建时拒绝部分声明。
- 完全空值仅作为历史只读默认，最终发布为 `read/low/not_required/idempotent`。

## FlagSpec 有效值回退链

框架统一的 flag 值解析顺序：

```
显式主 flag (Changed)
    │ 空?
    ▼
隐藏别名 (Changed, 按声明序)
    │ 空?
    ▼
环境变量 (EnvVar)
    │ 空?
    ▼
注册默认值 (Default)
    │ 空?
    ▼
ArgDefault (兜底)
```

各 Kind 的特殊行为：

| Kind | 入参条件 | 回退链 |
|------|----------|--------|
| KindString | 有效值非空（或 !OmitEmpty） | 完整参与 |
| KindInt | 值 ≠ 0（putInt 语义） | 完整参与 |
| KindBool | Changed 时入参（显式 false 也下发） | 不参与别名/env 回退 |
| KindStringSlice | 存在非空元素 | 仅 Changed 的主 flag/alias |

## Constraint 约束

声明式跨 flag 关系，构建时校验合法性，运行时统一执行：

| Kind | 语义 | 错误文案示例 |
|------|------|-------------|
| `at_least_one` | 至少提供一个 | "请至少指定 --a、--b 之一" |
| `exactly_one` | 恰好提供一个 | "请指定 --a、--b 之一" / "只能指定其一" |
| `mutually_exclusive` | 最多提供一个 | "参数 --a、--b 互斥，只能指定其一" |

"是否提供"的判定复用有效值回退链（显式主 flag → 别名 → env），注册默认值不算作已提供。

## ContractDecl 子结构

### contract.SelectionSpec（Agent 选择指引）

```go
contract.SelectionSpec{
    AgentSummary: "一句话描述工具做什么",
    UseWhen:      []string{"在什么场景下应该选择这个工具"},
    AvoidWhen:    []string{"什么场景不应该用，应该用什么替代"},
    Prerequisites: []string{"使用前提条件"},
    Tips:          []string{"使用技巧"},
    Examples:      []string{"dws dev app create --name Bot --dry-run"},
}
```

### contract.InterfaceSpec（接口模式）

```go
contract.InterfaceSpec{
    Mode:         "composite",  // local / mcp / composite
    Availability: "available",  // available / unavailable
    Reason:       "...",        // composite/unavailable 时的原因
    Ref: &contract.InterfaceRefSpec{ // mcp 时的 ref
        ProductID: "...",
        RPCName:   "...",
    },
}
```

### contract.DryRunSpec（dry-run 能力）

```go
contract.DryRunSpec{
    PreviewKind: "invocation",  // invocation / request / plan
    RemoteReads: false,         // dry-run 时是否发起远端读
}
```

## 执行体三选一

| 执行体 | 适用场景 | 框架做了什么 |
|--------|----------|-------------|
| **Invoke** | #830 过渡单步派发（生产仍用；目标 mcpbind） | 框架完成 required→constraint→validate→buildArgs→confirm，传入装配好的 toolArgs |
| **Orchestrate** | #830 过渡多步编排（生产仍用；目标 Handler） | 框架完成 required→constraint→validate→confirm，传入 Ctx 自行组装调用 |
| **RunE** | 逃生舱 | 框架仍执行 Safety 确认，具体业务执行完全自定义 |

## 设计不变量

1. **一个 corecmd.Spec = 一个叶子命令的全部事实**
2. **声明面绝不调用后端**——command 是 dispatch-agnostic
3. **执行面绝不发明 CLI 表面**——业务 flag 必须在 Flags 声明
4. **构建时拦截 > 运行时报错**——声明错误 panic 在注册阶段
5. **SafetySpec 是单一事实源**——Confirmation 驱动运行时，其余字段原样进入 Schema
6. **声明即 review**——代码中的 Schema 经 code review 后直接投影，不依赖外部 hint 文件
