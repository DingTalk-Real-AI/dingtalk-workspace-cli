# 命令框架对比：DWS command vs lark-cli vs GWS

本文档对比 DWS（钉钉工作区 CLI）、lark-cli（飞书 CLI）和 GWS（Google Workspace CLI / gcloud）三套命令框架的设计差异。

## 总览对比

| 维度 | DWS (command) | lark-cli | GWS (gcloud) |
|------|---------------|----------|--------------|
| 语言 | Go | Go | Python (gcloud) / Go (部分) |
| CLI 框架 | cobra | cobra | argparse + calliope |
| 调用底座 | MCP JSON-RPC | Lark REST SDK (`CallAPITyped`) | Google API Client |
| 命令层次 | 2 层：LeafSpec + Shortcut | 3 层：Shortcuts + API Commands + Raw API | 2 层：surface commands + raw |
| Schema 来源 | 代码声明投影 | 代码声明 + 运行时 introspection | API Discovery 文档自动生成 |
| Agent 适配 | 内建 (dws.schema.*) | 内建 (--print-schema) | 外挂 (MCP adapter) |

## 架构对比

### DWS command

```
corecmd.Spec (声明) → corecmd.New() → cobra.Command
     │
     ├── contract.SafetySpec (运行时 + Schema 单一安全来源)
     ├── FlagSpec[] (参数 + 回退链 + 绑定)
     ├── Constraint[] (互斥/至少一个)
     ├── ContractDecl (Agent Selection/DryRun/Interface)
     │
     └── Invoke / Orchestrate / RunE (执行)
```

**核心特点**：
- 声明与执行严格分离
- SafetySpec 四个独立字段直接对齐 Agent Runtime Schema
- 有效值回退链：flag → alias → env → default
- 框架统一校验、装配、确认、投影
- Schema 从代码声明直接投影，无外部 hint 文件

### lark-cli

```
Shortcut (声明) → runner.Mount() → cobra.Command
     │
     ├── Risk string (确认行为)
     ├── Scopes / ConditionalScopes (OAuth 权限)
     ├── Flag[] (参数 + Enum + Input sources)
     ├── AuthTypes (user/bot)
     │
     ├── DryRun hook → DryRunAPI
     ├── Validate hook
     └── Execute hook → RuntimeContext → CallAPITyped
```

**核心特点**：
- Execute 内直接调 REST API (`CallAPITyped`)
- DryRun 是独立 hook（返回结构化 API 计划）
- 内建 OAuth scope 声明与预检
- `--print-schema --flag-name` 运行时 introspection
- 无 Schema 投影层，Agent 通过 introspection 动态发现

### GWS (gcloud 风格)

```
API Discovery → 代码生成 → surface command
     │
     ├── arguments (从 JSON Schema 自动生成)
     ├── request/response 映射
     └── 自定义 action hook (少量)
```

**核心特点**：
- Schema-first：从 API Discovery 文档自动生成命令
- 参数直接映射 API 字段（flat schema）
- 人工 surface command 是 thin wrapper
- Agent 适配通过 MCP 外部 adapter

## 核心设计差异

### 1. 声明粒度

| 能力 | DWS command | lark-cli | GWS |
|------|-------------|----------|-----|
| 参数别名 + 环境变量回退 | ✅ FlagSpec.Aliases + EnvVar | ❌ 无 | ❌ 无 |
| 声明式约束 (互斥/至少一个) | ✅ Constraint[] | ❌ 只有 Validate hook | ✅ argparse group |
| 安全契约 | ✅ SafetySpec（effect/risk/confirmation/idempotency） | Risk | 无 |
| Schema 投影 (Agent metadata) | ✅ ContractDecl 内建 | ⚠️ 运行时 introspection | ❌ 外挂 |
| 参数绑定 (flag name → API key) | ✅ FlagSpec.Bind | ❌ 手写 | ✅ 自动映射 |
| ConstParams (固定载荷) | ✅ | ❌ 手写在 Execute | ✅ 隐式 |
| 确认门顺序可配 (ConfirmFirst) | ✅ | ❌ 固定顺序 | ❌ 无确认机制 |

### 2. 执行模型

| 维度 | DWS command | lark-cli | GWS |
|------|-------------|----------|-----|
| 参数装配 | 框架自动 (BuildArgs) | 手写 (`runtime.Str()/Bool()`) | 自动映射 |
| 派发方式 | Invoke(ctx, toolArgs) | Execute(ctx, runtime) | 自动调用 |
| 多步编排 | Orchestrate(ctx) | Execute 内链式 CallAPITyped | 不支持 |
| DryRun | 框架统一 (--dry-run flag) | 独立 DryRun hook 返回 API 计划 | 部分命令支持 |
| 错误分类 | apperrors 类型化 | errs.Problem 类型化 | HTTP status 映射 |

### 3. Agent 适配

| 维度 | DWS command | lark-cli | GWS |
|------|-------------|----------|-----|
| 工具发现 | `dws schema --all` (静态 catalog) | `--print-schema` (运行时) | API Discovery |
| 选择指引 | contract.SelectionSpec (UseWhen/AvoidWhen) | Description + Tips | 无 |
| 安全声明 | contract.SafetySpec 直接声明 | Risk string | 无 |
| dry-run 能力声明 | contract.DryRunSpec (reviewed) | DryRun hook 存在性 | 无 |
| 接口模式 | contract.InterfaceSpec (local/mcp/composite) | 隐式 (全部 REST) | 隐式 (全部 REST) |

### 4. Schema 生命周期

```
DWS:      代码声明 → code review → cobra annotation → catalog/metadata JSON
          (单一数据源，构建时验证完整性)

lark-cli: 代码声明 → 运行时 introspection → Agent 动态发现
          (无离线 catalog，Agent 必须执行命令才能发现)

GWS:      API Discovery JSON → 代码生成 → surface command
          (Schema-first，但命令行体验受限于 API 形状)
```

## 设计哲学对比

### DWS command 的选择

| 选择 | 理由 | 对比 |
|------|------|------|
| 框架装配参数 | 消除 N 个命令各写一份 toolArgs 装配 | lark-cli 每个 Execute 手动取 flag 值 |
| SafetySpec 单一来源 | confirmation 驱动运行时，其余字段原样发布且互不推导 | lark-cli 只有 Risk 一个维度 |
| 声明式约束 | 构建时校验合法性 + 投影到 Schema + 渲染帮助 | lark-cli 约束隐藏在 Validate 逻辑里 |
| Schema 构建时投影 | 离线 catalog 支持 Agent 批量发现 | lark-cli 需要逐个命令 introspection |
| 有效值回退链 | flag → alias → env 统一语义 | lark-cli 别名是独立 Flag 手动关联 |
| ConfirmFirst | 精确建模遗留语义 | lark-cli 确认始终在 Execute 内 |

### lark-cli 的选择

| 选择 | 理由 | 对比 |
|------|------|------|
| 直连 REST API | 精确控制请求/响应，可处理分页/重试 | DWS 通过 MCP 间接调用 |
| DryRun 返回 API 计划 | Agent 可预览将要发出的真实 HTTP 请求 | DWS dry-run 只展示参数 |
| OAuth scope 声明 | 框架预检权限，失败提前 | DWS 依赖 MCP 层鉴权 |
| `--print-schema` introspection | 运行时发现，无需维护离线 catalog | DWS 需要 re-generate |
| Input sources (file/@path/stdin) | 丰富的输入方式声明 | DWS 无此抽象 |
| PrintFlagSchema | 单 flag 级别的 JSON Schema 暴露 | DWS 只在 catalog 级别 |

### GWS (gcloud) 的选择

| 选择 | 理由 | 对比 |
|------|------|------|
| API Discovery 驱动 | 一份 Schema 生成所有：SDK/CLI/文档 | DWS/lark 手写 |
| Flat parameter 映射 | API 字段 = CLI flag，零转换 | DWS 需要 Bind 映射 |
| 无 shortcut 层 | API 粒度即用户粒度 | DWS/lark 有精选层 |

## 代码量对比

| 框架 | 核心框架代码 | 单命令声明开销 | 备注 |
|------|-------------|---------------|------|
| DWS command | ~1400 行 (command.go + contract_decl.go) | ~20-30 行 (纯声明) | 框架重、单命令轻 |
| lark-cli | ~800 行 (runner.go + types.go + common.go) | ~50-150 行 (声明 + Execute 逻辑) | 框架轻、单命令重 |
| GWS gcloud | ~5000+ 行 (calliope 框架) | ~10 行 (多数自动生成) | 框架最重、单命令最轻 |

## DWS 命令声明示例 vs lark-cli

### DWS (command / LeafSpec)

```go
NewLeafCommand(LeafSpec{
    Use:    "create",
    Short:  "创建应用",
    Tool:   "create_dev_app",
    Safety: contract.SafetySpec{
        Effect: "write", Risk: "high",
        Confirmation: "user_required", Idempotency: "unknown",
    },
    ConfirmFirst: true,
    Flags: []LeafFlag{
        {Name: "name", Usage: "应用名称", Bind: "name",
         Trim: true, Required: true, RequiredHint: "--name 为必填"},
    },
    Contract: ContractDecl{
        Description: "创建开放平台企业内部应用",
        DryRun:    &contract.DryRunSpec{PreviewKind: "invocation"},
        Interface: &contract.InterfaceSpec{Mode: "composite", Availability: "available", Reason: "create then configure"},
        Selection: contract.SelectionSpec{
            AgentSummary: "创建钉钉开放平台应用",
            UseWhen:      []string{"需要新建企业内部应用"},
            AvoidWhen:    []string{"应用已存在时用 update"},
            Examples:     []string{`dws dev app create --name "Bot" --dry-run`},
        },
    },
    Call: devAppCall(runner),
})
```

### lark-cli (Shortcut)

```go
var CalendarCreate = common.Shortcut{
    Service:     "calendar",
    Command:     "+create",
    Description: "Create a new calendar event",
    Risk:        "write",
    Scopes:      []string{"calendar:calendar"},
    Flags: []common.Flag{
        {Name: "summary", Desc: "Event title", Required: true},
        {Name: "start", Desc: "Start time (RFC3339)", Required: true},
        {Name: "end", Desc: "End time (RFC3339)", Required: true},
        {Name: "attendees", Type: "string_slice", Desc: "Attendee emails"},
    },
    DryRun: func(ctx context.Context, rt *common.RuntimeContext) *common.DryRunAPI {
        return &common.DryRunAPI{
            Method: "POST",
            Path:   "/open-apis/calendar/v4/calendars/{id}/events",
            Body:   buildEventBody(rt),
        }
    },
    Execute: func(ctx context.Context, rt *common.RuntimeContext) error {
        body := buildEventBody(rt)
        data, err := rt.CallAPITyped("POST",
            "/open-apis/calendar/v4/calendars/{id}/events", nil, body)
        if err != nil { return err }
        return rt.Output(data)
    },
}
```

## 适用场景总结

| 场景 | 最适合 | 原因 |
|------|--------|------|
| MCP 后端 + Agent Schema 投影 | **DWS command** | 内建 Schema 声明、SafetySpec 契约、离线 catalog |
| REST API 直连 + OAuth scope 管理 | **lark-cli** | CallAPITyped + scope 预检 + DryRun API 计划 |
| API-first 大规模 surface 生成 | **GWS gcloud** | Discovery 驱动，一份 Schema 生成一切 |
| 多步编排 (跨服务链式调用) | **lark-cli** / DWS Orchestrate | lark 的 CallAPITyped 链式 + DWS 的 Orchestrate |
| 遗留系统迁移 (保持行为等价) | **DWS command** | ConfirmFirst + 回退链 + catalog 漂移门禁 |
