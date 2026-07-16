# DWS Shortcut 能力 — 总体规划

> 目标：为 dws 引入一套 **声明式高保真命令（Shortcut）** 能力，对齐 larksuite/cli 的 `+command`
> 体验（如 `lark-cli contact +search-user`），并在此之上做 dws 差异化：**基于用户高频使用场景，
> 主动把常用操作沉淀为自定义 shortcut**。

## 1. 背景与动机

dws 当前的命令有三类来源：

1. **MCP 运行时动态发现** —— `dws mcp <service> <tool> --json '{...}'`，通用但裸、参数需手拼 JSON。
2. **`internal/helpers/` 产品命令** —— 手写 cobra 命令，体验好但每个都从零写、缺统一框架。
3. **`internal/registry/recipes.yaml`** —— 多步工作流的静态描述。

痛点：想新增一个「精选、参数友好、带 dry-run/format/身份」的单命令，只能手写 helper，
没有统一的声明式框架，重复劳动多、一致性差。larksuite 的 shortcut 框架正好解决这一层。

## 2. 与 larksuite/cli 的架构差异（关键）

| 维度 | larksuite/cli | dws-cli |
|------|---------------|---------|
| 命令来源 | 静态硬编码 Go shortcut | MCP 运行时动态发现 + helpers |
| 调用底座 | Lark SDK 直连 API | MCP JSON-RPC（`executor.Runner`） |
| 精选命令层 | `shortcuts/`（200+ 声明式） | `internal/helpers/`（手写 cobra） |
| 全局 flag | 框架注入 | root 已内建 `--format/--dry-run/--jq/--yes/--fields/--profile` |

**结论**：不能直接搬代码。移植的是 shortcut 的**声明式设计**，执行底座换成 dws 的
`executor.Runner`，全局能力复用 dws 已有的 output/safety/auth。

## 3. 分期目标

### P1 — 静态声明式框架（本期，正在做）
- 新建独立模块 `internal/shortcut/`（零侵入现有 helpers）。
- `types.go`：`Shortcut` / `Flag` / `RuntimeContext` 声明层。
- `runner.go`：把 `Shortcut` 编译成 `*cobra.Command`，串起 flag 注册 → 校验 → dry-run →
  `executor.Runner.Run` → `output.WriteCommandPayload`。
- `register.go`：按 service 分组产出命令，在 `internal/app/legacy.go` 装配点 merge 进命令树。
- 样板命令 `contact +search-user`：打通 MCP 执行 / format / dry-run / 身份，作为后续命令模板。
- 交付判据：`dws contact +search-user --help`、`--dry-run` 正常；`go build` / `go test` 通过。

### P2 — 高频场景自动沉淀（后续，先设计再实现）
- **使用埋点**：命令执行入口记录 `~/.dws/usage.jsonl`（只记参数形状，不记敏感值）。
- **模式挖掘**：`dws shortcut suggest` 聚合高频 `(service, tool, 固定参数组合)`。
- **主动沉淀**：命中候选时提示用户，一键写入 `~/.dws/shortcuts/*.yaml`（声明式，与 P1 结构对应）。
- **运行时加载**：`register.go` 额外扫描 `~/.dws/shortcuts/*.yaml` 动态注册，复用
  `internal/plugin/loader.go` 已验证的「从 `~/.dws/` 加载并挂 cobra 命令」模式。

## 4. 落地方式（P1）

采用**独立模块 + 装配点 merge**，不改 helpers 内部：

```
internal/shortcut/
  types.go            # Shortcut / Flag / RuntimeContext
  runner.go           # 声明式→cobra 编译 + 执行管道
  register.go         # Commands(runner) []*cobra.Command，按 service 分组
  contact/
    search_user.go    # 样板：var SearchUser = shortcut.Shortcut{...}
    shortcuts.go      # Shortcuts() []shortcut.Shortcut
```

接线：`internal/app/legacy.go: newLegacyPublicCommands` 里，
`helpers.NewPublicCommands(runner)` 之后追加 `shortcut.Commands(runner)`，
一起走 `mergeTopLevelCommands`（同名 service 命令自动合并，`+xxx` 作为其子命令）。

复用点：
- 执行：`executor.NewHelperInvocation` + `runner.Run`（与 helper 完全一致的调用路径）。
- 输出：`output.WriteCommandPayload(cmd, resp, output.FormatJSON)`（自动吃 root 的 `--format/--jq/--fields`）。
- dry-run：读 root `--dry-run`，置 `Invocation.DryRun`，由 runner 返回请求预览。
- 身份/安全：复用 `--profile`、`internal/safety`（高风险 `--yes` 确认）。

## 5. Shortcut 声明模型（草案）

```go
type Shortcut struct {
    Service     string   // "contact"       → 顶层命令
    Command     string   // "+search-user"  → 子命令（保留 + 前缀，对齐 larksuite）
    Description string
    Risk        string   // read | write | high-risk-write
    Flags       []Flag
    Validate    func(*RuntimeContext) error
    Execute     func(*RuntimeContext) error   // 必填；内部调 rt.CallMCP(...)
}

type Flag struct {
    Name, Type, Default, Desc string
    Required bool
    Enum     []string
}
```

`RuntimeContext` 给 Execute 提供：flag 读取（`Str/Bool/Int/StrSlice/Changed`）、
`CallMCP(product, tool, params)`（内部 `runner.Run`）、`Output(payload)`、`DryRun()`。

## 6. 风险与边界

- **与 `dws mcp` 通道的边界**：shortcut 是「人工精选的薄封装」，不替代通用 MCP 通道；
  一个 tool 可以既能 `dws mcp` 直调，也能有 shortcut。
- **自定义 shortcut 安全（P2）**：YAML 的 `execute` 若允许任意 MCP 调用，需过
  `internal/security` 的 endpoint 白名单，且沉淀的参数值要脱敏。
- **命名冲突**：`+` 前缀天然与现有 leaf 命令区分，降低与 helper 命令的冲突面。
- **edition 差异**：oss / enterprise 的可用 service 不同，注册时按 edition 过滤（后续接入）。

## 7. 进度看板

- [x] P1-1 types.go — `Shortcut` / `Flag` / `Risk` 声明层
- [x] P1-2 runner.go — `RuntimeContext` + `mount` 编译 + 校验/确认/dry-run；`CallMCP` 委托 `helpers.CallMCPToolOnServer`（复用错误分类/输出/dry-run）
- [x] P1-3 register.go + `internal/shortcut/contact/search_user.go` + `builtin` 聚合包
- [x] P1-4 接线 `legacy.go`（append 到 `mergeTopLevelCommands`）+ build/test 全绿
- [ ] P2 设计文档（进行中）

### P1 落地实证（已验证）

- `dws contact +search-user --help`：命令挂载，继承全局 `--format/--dry-run/--jq/...`。
- 必填校验：不传 `--query` → 结构化 validation 错误。
- `--dry-run`：走 helpers 路径输出 `[DRY-RUN]` 预览（tool + 参数）。
- 命令树 merge：`+search-user` 与现有 `user/dept/label/relation` 共存，`contact user search` 未受影响。
- 测试：`internal/shortcut` 单测通过；`internal/app` 全量回归通过。

### P1 关键决策记录

- **执行底座复用 helpers 而非裸 `runner.Run`**：`CallMCP` 委托 `helpers.CallMCPToolOnServer(product, tool, params)`，
  一步获得错误分类（auth/PAT/业务）+ 格式化输出 + dry-run，避免重造劣质输出层。代价是
  `internal/shortcut → internal/helpers` 的单向依赖（无环）。后续若要 shortcut 做多调用编排/输出重塑，
  再补一个返回原始 payload 的 `CallMCPRaw`。
- **避免 import 环**：service 包（contact）import 核心 `shortcut` 包并在 `init()` 注册；
  `builtin` 聚合包 blank-import 各 service 包；`app` 只依赖 `builtin`。
