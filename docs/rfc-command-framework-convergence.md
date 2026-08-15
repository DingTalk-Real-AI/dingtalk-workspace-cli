# RFC：将 DWS 命令执行收敛到一套类型化契约

- **状态**：draft v5.1，征求设计评审
- **范围**：`internal/corecmd`、`internal/helpers`（`LeafSpec`）、`internal/shortcut`
- **实现基线**：PR #830 的 `6fb08c9e`
- **#830 过渡派发 API（仍生产使用）**：`Invoke` / `Orchestrate` / `Ctx`（见 §3 / §5.0.3；目标 mcpbind + Handler，**本里程碑不删**）
- **同源决策**：[`flag-help-schema-homology.md`](flag-help-schema-homology.md)（路径 A：Contract 权威）
- **框架声明定义**：本文 **§5.0**（今日 `corecmd.Spec`/`LeafSpec` 与目标 `Contract`）

本文档取代收敛方案的前三版草案。它保留了 PR #830 中有价值的部分——一份声明同时驱动运行时校验、Runtime Schema 和帮助信息，外加严格的漂移门禁。`Invoke` / `Orchestrate` / `Ctx` 是 **#830 过渡派发 API**：今日仍在生产路径上，终态目标是 mcpbind 绑定 + Handler；不得在文档里写成「已丢弃」或要求本里程碑完整删派发 API。

v5.1 的变更：

- **flag / help / schema 同源**已拍板为路径 A：Contract / LeafSpec 为 CLI 表面权威，且 **必须经 cobra 注解嵌入 Schema 生成物**（`dws.schema.contract` / property / type / required / constraints / 显式 `dws.schema.risk`）。interface 事实由 leaf `Contract.Interface` / `ParamDecl` 声明（`schema_mcp_metadata` 已退役），不得从 MCP meta 生成 flag。身份/selection 继续评审源。见 [`flag-help-schema-homology.md`](flag-help-schema-homology.md)。

v5 的变更，全部来自第二轮独立评审并经过代码级复核。它们修的是 v4 自己引入的规范矛盾，因此逐条列出而不是折进正文：

- §5.11.1 新增：dry-run 计划的**内部表示与渲染形态分开上线**。v4 声称 M1「无上线输出变化」，但 28 条 Leaf 命令今天的 `--dry-run` 输出是 `executor.Result` 信封，与 `DryRunPlan{Summary, Steps[]}` 按构造不可能字节等价，与 M1 的零增量门禁直接冲突。M1 现在交付一个逐字复刻遗留信封的兼容渲染器；切换到原生计划渲染是后续 approved delta。M0 新增第 12 项交付物固定该信封 golden。
- §M4 明确 `Args` 等函数值字段比较的是**规范化后的有效元数标签，而不是字段身份**。遗留 `mount()` 不设 `Args`（nil），声明侧映射为非 nil 的 `ArgsArbitrary`；二者运行时等价，但反射式比较不等，会让第一个分批卡在假阳性上。§M5 的「三表面字节相等」按同一定义收窄。
- §5.7 取消「`Definition.Effects` 必须与 `CallSpec.Effect` 一致」这条自相矛盾的规则。派发器需要 `{CallSpec.Effect, EffectOutput}` 两个许可，而 §5.5 规定未声明的 effect 是内部安全错误，于是旧规则同时要求作者写两项、又要求它与单数字段「一致」。形态 1/2 现在完全不手写 `Effects`，由 `Bind` 推导；手写只属于形态 3。
- §5.4 显式记录 `Get` **保留零占位返回、以毒化取代静默**这次原则调整，并在 §4.3 指明那里批评的是静默而非零值本身。v4 静默放松了早期草案的「绝不降级」原则，却让 §4.3 读起来仍持旧立场。
- §5.7 定义 `ResponseProjector` 返回 error 的**归类规则**（`apperrors` 原样透传，裸 error 落入现状同一类别，错误发生在 `ResultSink.Write` 之前）。此前未定义，而 M5b 阶段 A 要求错误类别/消息/退出码逐条对等，该门禁按构造无法满足。
- §3.4 的修复**脱离本 RFC 成为独立 hotfix H0**（§8），不再排在架构序列的第四个 PR，§11 决策 5 也不再阻塞本 RFC 的任何里程碑。一个已上线的数据完整性缺陷不应由一套架构改造持有排期。
- §M4 新增**绝对性能预算**。此前只有逐 PR 相对阈值，10 个 PR 各 9.9% 可累积到约 2.6 倍而永不触发门禁。相对阈值退为附加门禁。
- §5.11 的预览文件计数由「3 个包中的 10 个文件」更正为「2 个包中的 9 个文件」；旧数字把框架自身的 `runner.go` 算作了命令声明。这是本 RFC 第二次出现同类计数偏差，因此 M0 交付物 1/10 落地前，所有章节引用的计数一律标注暂定。

v4 的变更，全部来自第一轮独立评审并经过代码级验证：

- §3.4 记录了一个**当前已上线的缺陷**：在非交互进程中，write/high-risk 类 Shortcut 会静默丢弃写操作并以 0 退出。bug-for-bug 兼容**明确不覆盖**这一条。
- §5.9 把「拒绝（decline）兼容」的适用范围收窄为真实的交互式拒绝，使差分门禁无法把该缺陷固化为必需行为。
- §5.3 新增 flag 级的 `Input`（`@file` / stdin），§5.11 新增声明式 dry-run 计划。两者都是契约层面的能力，而非逐个 handler 的口头约定；两者都借鉴自一个同类 CLI，该 CLI 已在全量命令上提供这两项能力。
- M0 新增**减法普查**：在 M7 假定所有手写命令都必须迁移之前，先把它们分类为 删除 / 迁移 / 能力受阻。
- M1 新增**入口门禁**：新命令必须走新核心，使该里程碑独立于 Shortcut 分批迁移就能产生价值。
- M2 在本就要重写该调用点的接缝工作中，顺带修复非交互确认。
- §3.5 与 §5.1 记录了声明与执行之间真正的分界：对全部 376 个 Shortcut `Execute` 函数体的 AST 普查表明，大多数命令的参数装配本可由声明表达。`Definition` 去掉强制 Handler 字段，「可执行」成为一个独立类型；mcpbind 新增响应投影钩子。验收标准不是删光执行体，而是**没有 `Execute`/`Call` 函数体只为装配参数而存在**；执行体仍是一等扩展点（与 LeafSpec 的 `Call`、同类 CLI 一致）。

核心决策是：

> 收敛到一套可执行的 CLI **契约**，而不是一个万能结构体，也不是「一次后端调用」与「多次后端调用」之间的区分。

声明与执行在**框架边界**处分离，而不是靠强迫每条命令各自持有一个 Handler 来分离（定义见 **§5.0**）。一条受管命令声明的是 CLI 表面、校验规则，以及——当它只有一次后端调用时——调用绑定；执行钩子不得充当第二套表面权威。固定流水线由框架执行。只有在控制流无法声明时才存在命令式 `Handler`。MCP 载荷绑定与响应投影都不放在 `command` 内。裸 Cobra 命令被明确排除在受管命令保证之外。现存裸命令通过清单方式豁免；只有新增的裸注册才需要迁移元数据。

---

## 1. 决策摘要

1. 把命令定义拆分为纯 CLI `Contract` 和一个**由类型安装、而非以可空字段声明**的执行所有者。`Definition` 本身不可编译；Handler 是控制流无法声明时的逃生口。
2. 每个 flag 恰好解析一次，得到不可变的类型化 `ResolvedValues` 快照，且发生在任何校验或执行之前。
3. 让 required 检查、enum 检查、关系约束、自定义校验器、MCP 绑定与 handler 全部消费同一份快照。
4. 在把全部 LeafSpec 命令迁移到 `mcpbind` 的同一个合并边界内，从核心契约中移除 `BuildArgs` 与 MCP 形状的字段。
5. 不要把单步与多步执行建模成两种不同的核心派发器。改用三种声明形态：仅调用绑定；调用绑定加一个纯响应投影器；命令式 Handler。
6. 不允许受管命令通过 `RunE` 绕过 required/约束/Risk 的强制执行。裸 Cobra 命令走一条独立且命名诚实的路径。
7. 把取消表示为类型化的结果/哨兵值。迁移期间保持当前退出行为；收敛方向另行决策。
8. 逐字段定义 Runtime Schema 的归属权。不要用源码声明取代经过评审的身份/导航注册表。
9. 只有在全部 376 个内置命令都通过完整的结构对等与差分运行时对等之后，才把 Shortcut 接入生产。
10. 保留 `LeafSpec` 作为面向 MCP 的门面，直到有证据表明它不再产生价值。语义收敛并不要求删除每一个适配器类型。
11. 迁移期间对遗留 Shortcut 的默认值强制转换保持 bug-for-bug 兼容。修正畸形的 int/bool/list 默认值属于独立的行为变更。
12. 从规范表面生成 Catalog 与接口产物，并显式采用 `auto(checked-in manifest)` 执行配置，同时独立证明 `旧遗留表面 == Contract 表面 == 规范表面`。
13. 每一次受管的后端/输出副作用都必须由调用级许可（permit）把关；不要把裸输入流或裸后端 helper 暴露给受管 Handler。
14. 保持遗留行为 bug-for-bug 兼容，**除非**该行为本身是正确性缺陷。§3.4 的非交互确认要在差分门禁能够要求它之前修好，因为兼容门禁会保护它所度量的一切。
15. 把「没有可用的交互输入」当作一种独立的确认结果，与接受和拒绝都不同。
16. **flag / help / schema 参数面同源取路径 A，且 Contract 必须嵌入 Schema**：Contract（LeafSpec 门面）经 `corecmd.New` 写入 cobra Schema 注解并注册类型化 Final，供 catalog/agent metadata 组装消费；MCP meta 不得创建 CLI flag；1:1 MCP 透传仅作可选子集。**硬规则**：每份 help/Schema 事实须 **声明 OR 人工标注**，禁止纯推断。**声明** = `LeafSpec`/`corecmd.Spec` 数据字段（`Flags`/`Constraints`/`Safety`/…），钩子不算声明；未设完整 SafetySpec 的旧写命令须 `runtime_gate` 标注。见 [`flag-help-schema-homology.md`](flag-help-schema-homology.md) §1.1–§1.3。
17. 让 dry-run 输出与 `@file`/stdin 输入成为声明式的契约能力，而不是逐 handler 的代码，这样覆盖率就是框架的属性，而不依赖作者自觉。
18. 从 M1 起要求新命令使用新核心，使框架在 Shortcut 分批迁移之前就已改善代码库。
19. 把 LeafSpec 已上线的 `Bind → Call → callMCPTool` 路径视为「声明可以完成执行」的存在性证明：27 个源声明 / 28 条上线命令，生产环境 `RunE` 用量为零。Shortcut 应当获得同样的绑定词汇，而不是把每个 `Execute` 都包成 Handler。
20. 用**残留的手写执行体数量**衡量迁移是否成功（当前为 376 中的约 31 条：多步加纯本地），而不是看是否每条命令都有非 nil 的 Handler。

---

## 2. 目标与非目标

### 2.1 目标

- 为 flag、类型化默认值、required 语义、enum、关系约束、位置参数、帮助信息和运行时 Risk 提供一份权威的可执行契约。
- 校验、约束、绑定与 handler 观察到的是完全相同的已解析值。
- 单次调用与多步命令都走同一套受管预检。单次调用命令优先使用声明驱动的派发（形态 1/2）；多步与纯本地命令保留命令式 Handler（形态 3）。
- 静态内置命令在测试/启动阶段快速失败；动态或用户自定义命令返回可操作的构造错误，而不是 panic。
- LeafSpec 与 Shortcut 可以通过新核心编译，且不改变各自当前的对外行为。
- 每一次迁移都由结构快照和带后端记录的差分测试背书，而不仅仅是生成物的 catalog 漂移。
- 上线过程具备仅作用于执行的新旧选择器、一份签入的精确键清单，以及经过测试的同二进制回滚。

### 2.2 非目标

- 本 RFC 不改变用户拒绝时的退出码。
- 不在同一个项目内迁移所有手写命令。
- 不要求用一个 Go 结构体同时承载 CLI、后端、发现和经评审的语义元数据。
- 不移除经评审的 CommandRegistry 或 schema 评审层。（后续演进已超越该非目标：reviewed `schema_command_registry/` 已退役，稳定 identity 切换为 identity collector 收集 `ContractFinal.Identity` 声明；schema 评审层以 reviewed exclusions、参数映射 ledger、MCP pin 等形式保留。）
- 不主张「生成的 catalog 字节一致」就能证明运行时等价。
- 不在 `LeafSpec` 或 `PostMount` 的真实能力获得具名替代之前删除它们。

---

## 3. 现状证据与修正后的范围

### 3.1 命令体系

| 体系 | 当前数量 | 说明 |
|---|---:|---|
| 手写 Cobra 命令 | 1195（暂定普查值） | 先前的 AST 计数尚无签入的复现脚本；M0 使其可审计 |
| Shortcut | 376 | 208 read、142 write、26 high-risk-write |
| 公开 Shortcut 列表 | 265 | 默认 `shortcut list`；`shortcut list --all` 覆盖全部 376 |
| LeafSpec | 27 个源声明 / 28 条上线命令 | 目前全部属于 devapp；其中一个生命周期定义被实例化两次 |

在分母定义清楚之前，不要给这些数字附加百分比。这三行是不同的命令表面，并不自动构成一个互斥的总体。

### 3.2 重要的兼容性事实

- 12 个生产 Shortcut 使用 `Validate`；其对外行为覆盖 14 条 `ConstraintCustom` 声明。
- 干净的内置注册表中，有 78 个带非空默认值的 flag 实例，分布在 53 条命令上。它们由 70 个字面量构造点展开而来，因为共享的 flag 模板被复用。78 个之中，有 52 个 flag（分布在 39 条命令）的默认值为非字符串类型（`int`、`bool` 或 `string-slice`）。
- 默认的公开列表输出 73 个带默认值的 flag；`--all` 输出全部 78 个。用户自定义 Shortcut 属于另一个运行时总体。
- 干净的内置命令树有 17 个隐藏兼容 flag。两种 list 模式都刻意序列化零个隐藏 flag，因此 list 输出不能作为完整的编译器基线。
- 文本搜索在 16 个 Shortcut 文件中找到 19 处 `time.Now()` 匹配，其中 1 处在注释里。18 处可执行调用由 15 个文件中的 16 处 handler 调用加 2 处用量记录调用组成。handler 取时间必须使用 Runtime 注入的 Clock，而记录器接收自己独立的 Clock 依赖。
- 只有 168 个 write/high-risk Shortcut 能走到确认-拒绝分支。改变拒绝策略并不影响全部 376 条命令。
- 265 条公开 Shortcut 列表项与 Runtime Schema 的 Shortcut 叶子不是同一总体：经评审的注册表当前有 210 个 `+shortcut` 路径。余下 55 条需要精确的、经评审的排除项，而不是隐式缺席；两个数字不得互相替代。
- 27 个 LeafSpec 源声明**每一个**都使用 `PostMount`：
  - 15 个源声明直接使用 `devAppMeta` 包装器；
  - 12 个使用内联闭包；
  - 运行时变成 16 个直接包装器加 12 个闭包，因为一个生命周期定义被实例化两次；
  - 全部 28 条上线命令都调用 `devAppLeafMeta`；
  - 4 条额外添加 cursor flag；
  - 内联闭包还覆盖命令别名、兼容/隐藏 flag、事件码、scope、成员、安全设置和一个 bool flag。
- `devAppLeafMeta` 做的事不止 Schema 身份：它还设置 `NoArgs`、`DisableAutoGenTag`、遗留偏好和后端注解。
- 生产环境的 Leaf 声明使用了一个长兼容别名，且 `EnvVar` 与 `RequiredHint` 字段用量为零。全部 27 个都使用 Call；生产环境的 `RunE`、声明式 Risk/Constraints/Required 以及 MarkRequired 均未被使用。这些框架特性属于兼容测试面，不能作为 Leaf 广泛使用它们的证据。

以上「源码 / 上线」的区分是针对当前提交的审计事实，不是永久的规划常量。M0 会签入复现脚本，并使其成为后续里程碑的权威依据。

### 3.3 PR #830 证明了什么

PR #830 为项目提供了一个有用的共享基座和针对它的直接测试。其零漂移结果，是对这些产物所覆盖的生成表面的有效证据。它的包级测试覆盖了 LeafSpec 运行时流水线。

PR #830 当时**没有**证明 `FromShortcut` 可以替换上线的 `mount()`，因为该适配器尚未上线并会丢失运行时语义。后续收敛阶段已经补齐 Hidden、typed default、Changed-required、Enum、Custom/Validate 与隐藏参数 Schema 投影，并将 live `mount()` 切换为 `corecmd.New(FromShortcut(s))`；该结论由全量 Shortcut surface 差异测试、Catalog 零漂移和统一确认行为测试守护。

### 3.4 一个兼容策略绝不应保留的已上线缺陷

§3.2 中其他所有行为都是有意保留的。这一条不是兼容性事实，而是一个已上线的正确性缺陷；必须在此点名，否则 §5.4.1 与 M3 会迫使新编译器复现它。

遗留的确认提示直接读取进程 stdin，没有任何交互性检查：

```go
// internal/shortcut/runner.go
reader := bufio.NewReader(os.Stdin)
answer, _ := reader.ReadString('\n')
answer = strings.TrimSpace(strings.ToLower(answer))
return answer == "yes" || answer == "y"
```

在非交互进程中——agent、流水线、CI，或任何没有附加终端的父进程——该读取会在 EOF 处立即返回。空答案与用户键入 "no" 无法区分，于是 runner 走拒绝分支并返回 `nil`。在当前分支上用一条真实的 write Shortcut、关闭 stdin 实测：

| 调用方式 | 退出码 | stdout | 是否触达后端 |
|---|---:|---|---|
| write Shortcut，stdin 关闭 | 0 | 0 字节 | 否 |
| 同一命令加 `--yes` | 1 | 错误载荷 | 是 |

第二行是因为测量沙箱中一个无关的环境原因而失败的，而这恰恰使它更有说明力：真正执行了的命令报告了自己的失败，而什么都没做的命令报告了成功。提示语走 stderr，因此一个读取 stdout 和退出码的调用方——这正是非交互消费者的常规契约——看到的是一次干净的成功，而该操作从未发生。这种沉默恰好发生在无法挽回的那个方向上：真实的后端失败始终可见，唯独「什么都没发生」不可见。

影响范围与归类：

- 全部 168 个 write/high-risk Shortcut 受影响；read 命令从不提示。
- `corecmd.ConfirmSafety` 读取 `cmd.InOrStdin()` 而非进程 stdin，因此它是可测试的；EOF 会显式返回 `confirmation_required`，不会把未执行误报为成功。
- 这**不是** §5.9 的拒绝策略问题。那个问题是「真实拒绝应当产生哪个退出码」。这里没有任何人拒绝过：拒绝分支在 CLI 最常被使用的那种环境里默认就可达。
- 由于这是用户可见的行为变更，它走 §9 的 approved delta 流程。该流程不依赖 Contract、解析器或 mcpbind，因此这项修复不受新核心的进度约束。
- **因此它作为独立 hotfix（§8 中的 H0）先于本 RFC 的实现序列发布，而不是排在架构工作的第四个 PR。** 这是一个已上线的数据完整性级缺陷：168 条写命令在 agent/CI 环境下静默丢弃写操作并报告成功。把它排在一套架构改造之后，等于让缺陷的存续时间取决于一个与它无关的项目的进度。本 RFC 记录它、给出修复形态与门禁，但**不持有它的排期**；即使本 RFC 被否决或推迟，H0 也应照发。

M2 在确认提示的接缝工作中修复它；§5.9 记录了为什么差分对等不得要求旧行为；§11 决策 5 记录产品对信号形式的选择。

### 3.5 376 个 Shortcut `Execute` 函数体实际在做什么

LeafSpec 已经在生产中证明了声明驱动的执行：27 个源定义每一个都使用 `Call`，生产环境 `RunE` 用量为零。框架路径是 `BuildArgs → Call → callMCPTool`。Shortcut 的 API 恰好相反：`Execute` 是必填的，而 `Flag` 没有 `Bind`/`Transform`/`OmitEmpty` 字段，于是每条内置命令无论是否需要，都要付出一个手写函数体的代价。

对全部 376 个内置 `Execute` 函数字面量做 AST 普查，划出一条架构分界线：函数体是否消费后端响应？

| 形态 | 数量 | 占比 | 声明需要什么 |
|---|---:|---:|---|
| 单次调用，直接 `return` 调用结果 | 237 | 63% | 仅需调用绑定；零代码 |
| 单次调用，消费响应后再整形/输出 | 108 | 29% | 调用绑定 + 一个纯投影器 |
| 两次及以上后端调用 | 21 | 5.6% | 命令式 Handler |
| 零后端调用（纯本地） | 10 | 2.7% | 命令式 Handler，或一项具名的本地能力 |

这些数字在 **P0 签入普查脚本之前均为暂定值**，与 1195 这个命令数一样（见 M0 交付物 10）。此处引用它们，是因为它们改变了目标 API 的形状；但 §10 的残留标准是对照签入的 golden 来判定的，而不是对照本表。

**376 中有 345 条（92%）只发起一次后端调用。** 21 条多步命令需要框架无法声明的控制流。10 条纯本地命令属于另一种情况：它们完全不需要后端调用，因此当下归为形态 3，但一旦 M7 为它们所用的本地能力命名，就可能变为可声明。这两组都没有被断言为永久命令式。

在 237 个直接返回的函数体中，有 108 个整体就是一条语句，形如 `return rt.CallMCP(tool, map[…]{…})`。余下 119 个包含条件判断；主流形态是 `if rt.Changed("limit") { params["limit"] = … }`，这正好对应 `OmitEmpty` / `PresenceExplicit` / `IncludeMode`——LeafSpec 和 §5.7 的绑定规则早已具备的词汇。

那 108 个消费响应者，正是「声明等于执行」并非免费的原因。外层包装是高度重复的——65 处输出 `{count, list}`——但字段映射不是：存在 82 个各自独立的 `*Project` helper，且出现频次最高者连同自身定义也不过 5 次，实际复用更低。因此形态 2 的诚实目标不是一套通用的投影 DSL，而是一个纯 `func(response) any`，不触碰调用、确认、输出写入器或错误归类。`ResultSink` 负责写出，不负责整形。

对目标 API 的推论：如果要求每个受管定义都有非 nil 的 `Handler`，就会把 376 个 `Execute` 函数体变成 376 个 Handler，却未必消掉「只为装配参数而存在」的函数体。那样只统一了表面与安全检查，却丢掉了本 RFC 所声称的分离。因此 `Definition` 不强制 Handler（见 §5.1）：可声明的装配走绑定；真正的业务控制流留在执行体。验收标准是**没有函数体只为装配参数而存在**，而不是把执行体压到某个残留数量。§3.5 的三形态表保留为声明覆盖度参考，不再作为「删除目标」。

---

## 4. 为什么最初的 S1 模型不是目标架构

#830 过渡派发 API（**仍生产使用**，见文首）形态：

```go
RunE        func(*cobra.Command, []string) error
Invoke      func(*Ctx, map[string]any) error  // 过渡：单步
Orchestrate func(*Ctx) error                   // 过渡：多步
```

它不是终态架构，有四个原因。完整替换为 mcpbind + Handler 是后续里程碑，**不是**本 RFC 落地 PR 的删除义务。

### 4.1 `Invoke` 是由下一步要收敛掉的 MCP 泄漏所定义的

`Invoke` 与 `Orchestrate` 之间唯一本质的区别，是 `corecmd` 会在 `Invoke` 之前构建 `map[string]any`。一旦载荷绑定移出核心，两种形态都变成 `func(*Runtime) error`。

因此旧的 S4 并不是一条独立的清理链。载荷绑定的分离与 handler 模型必须一起设计；在那之前，过渡 API 继续服务 Leaf/Shortcut。

### 4.2 `RunE` 会让已声明的安全契约变成假话

当前的 `RunE` 逃生路径会注册并投影 flag、约束与 Risk，然后绕过它们的运行时强制执行。于是一条命令可以对外宣称「需要确认」，却从不确认。

裸命令有时是必要的，但不能把它们表示成好像也获得了受管保证。

### 4.3 `Ctx` 没有提供唯一的已解析真值

原型对未知 flag 和整数解析错误静默返回零值。`Orchestrate` 还跳过了 `BuildArgs`，而后者目前正是某些解析/转换错误浮现的地方。因此非法的环境变量/默认值数据会以 `0` 的形式抵达 handler。

此处的缺陷是**静默**，而不是零返回本身。§5.4 的 `Get` 仍返回零占位，但那次读取会毒化整个调用并在四个强制检查点被拦下；两者的区别不在返回值形状，而在于坏读取能否走到一次 effect 前面。该原则调整在 §5.4 中显式记录。

正确的原语是一份已校验的快照，而不是一组建立在可变 Cobra flag set 之上的尽力而为的 getter。

### 4.4 「恰好一个可空函数」是校验，不是类型保证

构造期的基数检查有用，但三个可空函数仍然允许非法取值。`FromShortcut` 还可能围绕一个为 nil 的 `Shortcut.Execute` 生成非 nil 的包装器，于是基数检查通过，执行却在之后失败。

因此稳定 API 不靠「更好的基数检查」来解决这个问题，而是**按类型**分离声明与可执行性：`Definition` 不可编译，而获得可编译的 `*Executable` 的每一条途径都恰好安装一个执行所有者（见 §3.5、§5.1）。框架派发器覆盖了常见情形，因此多数命令完全不需要 Handler。

---

## 5. 目标架构

```text
Definition（仅声明；不可编译）
  ├── Contract ── compile ──> Cobra 表面 + Runtime Schema 注解
  ├── Validators
  ├── CompileFacts
  └── Effects
          │
          │  恰好一个构造函数安装执行所有者
          ├── mcpbind.Bind(def, CallSpec[, WithProjector]) ──> *Executable
          └── command.WithHandler(def, Handler[, WithPlan]) ──> *Executable
          │
          v
解析 argv
  → 一次性解析全部取值
  → required / enum / 关系校验
  → 自定义校验器
  → Risk 确认
  → 执行（三者恰好其一）：
       ├── 形态 1：mcpbind 派发器 ── 编码 → 一次后端调用 → ResultSink
       ├── 形态 2：同上，再经 ResponseProjector → ResultSink
       └── 形态 3：Handler.Execute(Runtime) ── 零/一/多次后端调用

裸 Cobra 命令 ──> 独立的遗留/裸构造器，无受管保证
```

三种声明形态取代「每条命令都有一个 Handler」：

| 形态 | 声明内容 | 命令式代码 | 当前普查数 |
|---|---|---|---:|
| 1. 绑定调用 | Contract + mcpbind `CallSpec` | 无 | 237 |
| 2. 绑定调用 + 投影 | 形态 1 + 纯 `ResponseProjector` | 仅 data→data | 108 |
| 3. 工作流 | Contract + `Handler` | 完整 Execute 函数体 | 31 |

形态 1 和 2 正是 LeafSpec 已经在全部 28 条上线命令上交付的东西（形态 1；生产环境尚无响应投影器）。形态 3 保留给 21 条多步与 10 条纯本地 Shortcut，以及未来任何提示内容或调用扇出无法声明的命令。

### 5.0 框架上的「声明」定义（今日实现 + 目标 Contract）

本节把「声明」钉在**命令框架**上，而不仅在同源文档里。**Schema 叶子（`cli.ToolSpec`）的每一个字段组都必须在本节有权威归属**——声明、标注、评审源或组装派生物；不得出现「Schema 有、框架未定义谁写」的空洞。字段级细节见 [`flag-help-schema-homology.md`](flag-help-schema-homology.md) §1.2–§1.4。

#### 5.0.1 分工：声明 = 最终源，Schema = 透传

| 层 | 含义 | 今日落点 |
|---|---|---|
| **声明（declare）** | `corecmd.Spec` / `LeafSpec` / `ContractDecl` **数据字段**（声明证据；交付见下） | `Flags`/`Constraints`/`Risk`/`ConstParams`/`Contract`；类型真身在 `corecmd/contract`（DTO：`SafetySpec`/`ParamDecl`/`ProductDecl`/`ContractFinalPayload`；**无** Cobra store） |
| **框架转换** | 类型转换并注册（**禁止** JSON 注解桥） | `embedContractDecl` → `corecmd/contractfinal.RegisterRuntimeContractFinal`（annotate + store；全部调用方直调，`corecmd.New` 内部注册） |
| **注解 seam** | Cobra `dws.schema.*` 写入 | `internal/corecmd/runtimeannotate.AnnotateRuntime*`（框架侧；`cli` 根经 `runtime_schema_seam.go` 包内别名访问；`cli/runtimeannotate` 垫片包已删，一律直引 corecmd） |
| **Schema 透传** / 交付 | 组装读取注册表，原样投影为 `ToolSpec`；`RegisterSchemaSourceRoot` → `ResolveSchemaBuild`（`ResolveMeta` 自同一组装投影）；go:embed 仅限 reviewed 输入（MCP meta / `param_concepts` 等；reviewed `schema_command_registry/` 已退役，identity 由 collector 收集），映射排除走 Go ledger（`schema_parameter_mapping_ledger.go`），不得 embed Catalog | `internal/cli` 根（交付边界）；ContractFinal store 在 `corecmd/contractfinal`（`cli` 根经 `runtime_schema_seam.go` 包内别名访问；`cli/contractfinal` 垫片包已删） |
| **执行（execute）** | 钩子不发明表面 | `Validate` / `Call` / `RunE` / `PostMount` |

依赖方向硬规则：`internal/corecmd`（含子包）**不得** import 任何 `internal/cli` 包；annotate 与 ContractFinal store 归属框架侧。

硬规则：

1. 声明体系**不含评审并行字段**；hooks 不算声明。声明载荷携带 reviewed 字段（如 `Selection.Reviewed`）组装直接报错——`Reviewed` 是旧路径（hints/registry）专用标记。
2. 不得把声明序列化成 JSON 再解析；框架自己做 `ContractDecl` → `ContractFinalPayload` → `ToolSpec`。
3. 已声明字段不得被 hints/registry 盖写。迁移期未声明叶可走旧路径。
4. 受管（已绑定）叶上声明的 `Identity` 必须与绑定 entry 一致（`product_id`/`name`/`canonical_path`/`cli_path`/`aliases` 等）；不一致组装报错（`validateContractFinalIdentity`）。后续演进中声明 identity 已成为唯一 identity 来源（identity collector 直接收集 `ContractFinal.Identity`，reviewed `schema_command_registry/` 已退役），不再是平行钉扎通道。

#### 5.0.2 今日契约字段（`corecmd.Spec` / `LeafSpec`）

下列字段**是**框架声明面（经 `corecmd.New` 生效并嵌入 `dws.schema.*`）：

- `Flags`（含 Name/Kind/Default/Required/MarkRequired/Usage 等注册面；`Input` 是取值来源声明，经 `corecmd.New` 生效但**不**嵌入 `dws.schema.*`，能力靠 `Usage` 文案声明，见 §5.3）
- `Constraints`
- **非空** `Risk`（空值 = 运行时当只读确认，且**不**嵌入 `dws.schema.risk`）
- `ConstParams`（载荷声明；不上用户 flag 表）
- `Use` / `Short` / `Long` / `Example`（cobra help；**不是** Schema canonical identity）

下列字段**不是**声明面（编排 / 执行）：

- `Validate`、`PostMount`、`Call` / `Invoke` / `Orchestrate`、`RunE`
- Leaf 的 `Server` / `Tool`（路由；Schema interface 另由 MCP meta / ParamDecl 表达）

验收（框架门禁，而非口头约定）：

1. 业务 flag 只出现在 `Flags`（或领域工具注入 + 同源允许的 annotate），不在 `PostMount`/`Validate` 里 `Flags().String`；
2. 业务参数只由 `Flags`/`ConstParams` 装配，不在 `Call`/`Execute` 里 `params[k]=…`；
3. 写副作用：新 Leaf 声明完整 `SafetySpec`（框架 `ConfirmSafety` + Schema Final）；未迁移旧路径显式标注 `runtime_gate`；二者皆无则不合格；
4. Schema `ToolSpec` 全字段组均落在 §5.0.4 表中某一权威格，禁止无主字段。

#### 5.0.2a 三档声明路径（Tier1 / Tier2 / Tier3）

当前生产允许的三档路径（同一 `ContractFinal` 语义；不是互相否定）：

| 档 | 入口 | 声明面 | 执行面 | 适用 |
|---|---|---|---|---|
| **Tier1 完全托管** | `corecmd.New` / `NewLeafCommand(spec)` | Flags/Constraints/Safety/Contract 全进 command | 框架接管 flag 注册、参数投影、`ConfirmSafety`、派发 | 新命令；可自由设计执行面 |
| **Tier2 声明元数据** | `DeclareLeafMetadata(cmd, spec)` | 仅 `Safety` + `Contract`（经 `AttachContract`） | **不**注册 flag、**不**接管参数投影；可选 `Validate` 与 `ConfirmSafety` **同挂 RunE 包装器**（Validate 在前）；无 Validate 时确认推迟到首次 `CallTool` | helpers 迁移态；**Shortcut 也可采用此路径，可接受** |
| **Tier3 裸 Cobra** | 手写 `*cobra.Command` | 无框架声明（或仅 annotate / 评审排除） | 调用方自管 | 应逐步收；新增裸叶需迁移元数据或精确排除 |

选用规则：

1. 新命令默认走 Tier1 完全托管。
2. 既有 helpers / Shortcut 要补 Agent Schema、但执行面暂时不能迁入 LeafSpec → Tier2；声明必须写在命令字面量旁，禁止旁路 generated map。**Shortcut + `DeclareLeafMetadata` 是合法当前路径**，不得在文档或评审里否定。
3. Tier2 是**迁移态**：具备条件后应升级为 Tier1；不得用它绕开「业务 flag 必须声明」的框架纪律。大规模 Tier2→Tier1 与 Shortcut→mcpbind 属于后续里程碑，**不是**本阶段硬门槛。
4. `DeclareLeafMetadata` 传入 Flags/Call/RunE/PostMount 等执行面字段直接 panic，防止半接管；**唯一允许的执行钩子是 `Validate`**，与 `ConfirmSafety` 同在 RunE 层（禁止只挂 PreRunE——直接调 `RunE` / `proxySubCmd` 会跳过 PreRunE）。无独立 Caller 的本地/PAT 命令必须提供 `Validate`，否则会回退成确认抢先。
5. **长期展望**（非当前硬要求）：更多 Shortcut 可收敛到 mcpbind 形态 1/2，并减少仅为参数装配而存在的 `Execute` 函数体。不得把「必须删除 `Shortcut.Execute` / 必须 mcpbind」写成当前门禁。

#### 5.0.3 与目标 `Contract` 的对应

```text
今日 LeafSpec / corecmd.Spec.Flags|Constraints|Safety|…
        ≈  目标 Definition.Contract
今日 Call / Invoke / Orchestrate / RunE（#830 过渡派发仍生产使用）
        ≈  目标 mcpbind 派发 或 Handler（形态 1/2/3；完整删派发 API 属后续里程碑）
今日 Validate / PostMount
        ≈  目标 Validators / 收窄后的挂载钩子（不得再充当第二套 flag 权威）
```

收敛不得把「声明」偷换成「每个命令一个 Handler」：声明覆盖度以**数据字段**衡量；执行体是扩展点，不是声明的替代品（§3.5）。

#### 5.0.4 Schema 全覆盖：`ToolSpec` 字段权威（命令框架视角）

以下对照 `internal/cli.ToolSpec`（及嵌套类型）。**每一行必须有且仅有一个写入权威类**；组装器只投影，不发明。

| Schema 字段组 | 子字段 / 内容 | 权威类 | 今日写入面 | 框架声明？ |
|---|---|---|---|---|
| **Identity** | `product_id`, `name`, `cli_name`, `canonical_path` / `cli_path` / `primary_cli_path`, `group`, `aliases`, `source`, `source_product_id` | 绑定树 entry（identity collector 收集结果） | `ContractDecl.Identity` 声明（identity collector 收集；reviewed `schema_command_registry/` 已退役）；组装仍经 `validateContractFinalIdentity` 校验声明与绑定一致 | **是**（identity 即声明） |
| **Display / Title / Description** | 产品展示名；工具 title/description | **title**：ContractDecl 声明优先，否则 Cobra Short，再 MCP；**description**：**构造期** `ContractDecl.Description` **必填**（声明证据）；**Catalog 交付** Cobra Long 优先（provenance `cobra_help` / `cobra_help_preferred`），无 Long 用声明（`contract_final`） | 命令树产品名（registry 产品 shard 已退役）；`ContractDecl` + Cobra Short/Long；组装 stamp 真实 winner | 非「declare = wire 最终值」、非双权威：声明必填 + 交付 Long 可赢；**canonical 文案不以 Contract 胜 identity** |
| **Parameters** | `name`, `type`, `required` / `cli_required`, `default` | **声明**（或手写 annotate 同形） | `Flags` → `embedContractIntoSchema` / cobra | **是** |
| | `description`（usage 文案） | 声明 usage | `FlagSpec.Usage` / ParamDecl；`schema_hints/` 已退役 | usage **是**；不得用 hint overlay 改 type/required/default |
| | `property`（载荷键） | 声明 | `FlagSpec.Bind`（空则 Name） | **是**（载荷映射） |
| | `enum`, `format`, `example`, `required_when` | 声明或评审 annotate | 今日部分仍手工 annotate；目标进 Contract / reviewed 约束（`schema_hints/` 已退役） | 有则须 declare 或 reviewed annotate |
| | `interface_description`, `interface_type`, `interface_default` | 评审源（interface） | ParamDecl / mapping ledger exclusions（`schema_mcp_metadata` 已退役） | 否；**不得创建 CLI flag**（`HOM-I1`） |
| **Constraints** | `require_one_of`, `mutually_exclusive`, `require_together` | **声明** | `Constraints` → `AnnotateConstraints` | **是** |
| **Positionals** | 位置参数名/必填/说明 | **声明** 或显式 annotate | 目标 `Args`/`PositionalSpec`；今日少量 cobra Args + 注解 | 受管命令应声明，禁止推断 |
| **Safety** | `effect`, `risk`, `confirmation`, `idempotency` | **声明**（完整 `contract.SafetySpec`）**或标注**（`runtime_gate`） | `Safety` / `AnnotateRuntimeGate`（metadata 壳 `tools: {}`，不再承载 reviewed Safety） | 四字段独立；confirmation 单独驱动运行时 |
| | `idempotency` | 评审源（或未来 Contract） | reviewed metadata | 今日非框架声明；不得推断 |
| | `effect_source` / provenance | 组装派生物 | resolver 写入 `FieldProvenance` | 派生，不手写 |
| **DryRun** | `preview_kind`, `remote_reads` | 评审源 | `schema_dry_run_capabilities`（正能力声明） | 否；无条目 ≠ 推断「不支持」之外的假能力 |
| **Interface** | `interface_mode`, `interface_ref`, `availability`, `reason` | 评审源 | MCP meta + agent metadata 解析 | 否；与 CLI Identity 分离 |
| **Selection** | `agent_summary`, `use_when`, `avoid_when`, `examples`, `prerequisites`, `tips`, `workflow_refs`, … | 声明（`ContractDecl.Selection` / `ProductDecl`） | `ContractDecl` / `ProductDecl`（`schema_hints/` 已退役） | 可声明；声明载荷**不得携带** `Reviewed`（旧路径专用），携带即组装报错 |
| **FieldProvenance** | 各字段 winner / candidates | 组装派生物 | Schema 组装器 | 派生；须与 delivered value 一致 |
| **Extensions / MetadataSource** | 扩展袋；元数据来源标记 | 评审源或组装标记 | embedded MCP / resolver（禁止 hints 回潮） | 不构成 CLI 表面 |
| **ConstParams**（框架有、Schema parameters 无） | 固定 toolArgs | **声明**（载荷） | `ConstParams` | **是**（故意不上 parameter 表） |

产品级 Schema（`ProductSpec` 的 description / selection）权威在 `ProductDecl`（reviewed registry 的产品维度已随 `schema_command_registry/` 退役），**不在**单命令 Contract。

冲突规则（路径 A）：

- parameters 的 type / required / default / name 集合：**Contract/cobra 胜** hints 与 MCP；
- Safety 的 effect/risk/confirmation/idempotency：完整 Contract `Safety`（Final 声明）胜；否则 `runtime_gate` 可把旧路径的 confirmation 升为 user_required；最末迁移期 reviewed Safety。Safety 各字段之间不得互推（声明 > 标注，§5.10）；
- Identity：**绑定 entry 胜**——Contract 声明必须与之一致，不一致组装报错，不得改写 canonical path；Selection 已声明则声明胜；Interface 不得发明 RPC。

### 5.1 Definition、Contract 与 Handler

下面的 API 草图固定包边界；具体命名可在实现阶段调整。`Contract` 即 §5.0 声明面的目标形态。

```go
// Definition 仅是声明。它没有 Handler 字段，因此「已声明但没有执行所有者」
// 是不可表示的，而不仅仅是被拒绝。
type Definition struct {
    Contract   Contract
    Validators []ValidatorSpec
    Facts      CompileFacts
    // Effects 由形态 3 作者声明。形态 1/2 留零值：mcpbind.Bind 从 CallSpec
    // 推导 {CallSpec.Effect, EffectOutput}，非零值在那里是构造错误（§5.7）。
    Effects []EffectKind
}

// Executable 把一份冻结的 Definition 与恰好一个执行所有者配对。所有字段均不导出，
// 唯一构造函数是 mcpbind.Bind（形态 1/2）与 command.WithHandler（形态 3）。
// 「无所有者」与「双所有者」都无法表达，因此 §4.4 对基数检查的批评不会在此重现。
type Executable struct {
    definition Definition
    owner      executionOwner
    plan       Planner
}

type HandlerOption func(*handlerConfig) error

// WithPlan 提供 §5.11 的 dry-run 计划；write/high-risk 的形态 3 handler 必须声明。
// 形态 1/2 的计划从 CallSpec 推导。
func WithPlan(Planner) HandlerOption

func WithHandler(
    def Definition,
    h Handler,
    opts ...HandlerOption,
) (*Executable, error)

type Contract struct {
    Use     string
    Short   string
    Long    string
    Example string

    Hidden            bool
    Aliases           []string
    DisableAutoGenTag bool

    Args        ArgsSpec
    Flags       []FlagSpec
    Constraints []Constraint
    Risk        Risk
}

type RuleID string

type Risk uint8

const (
    RiskInvalid Risk = iota
    RiskRead
    RiskWrite
    RiskHighRiskWrite
)

type ConstraintKind uint8

const (
    ConstraintInvalid ConstraintKind = iota
    ConstraintAtLeastOne
    ConstraintExactlyOne
    ConstraintMutuallyExclusive
    ConstraintRequireTogether
    ConstraintCustom
)

type ArgsPolicy uint8

const (
    ArgsInvalid ArgsPolicy = iota
    ArgsArbitrary
    ArgsNoArgs
    ArgsDeclarative
)

type ArgsSpec struct {
    Policy      ArgsPolicy
    Positionals []PositionalSpec
}

type PositionalSpec struct {
    Name        string
    Description string
    Required    bool
    Variadic    bool
}

type Constraint struct {
    ID          RuleID
    Kind        ConstraintKind
    Flags       []string
    Presence    PresenceMode
    Description string
}

type Handler interface {
    Execute(*Runtime) error
}

type ContractBoundHandler interface {
    Handler
    ValidateContract(Contract) error
}

type CompileFacts struct {
    Annotations []AnnotationFact
}

type AnnotationFact struct {
    Key   string
    Value string
}

type HandlerFunc func(*Runtime) error

func (f HandlerFunc) Execute(rt *Runtime) error { return f(rt) }

type ValidatorSpec struct {
    // ValidatorID 标识可执行的校验钩子。它只运行一次。
    ValidatorID string
    Covers      []RuleID
    Validate    func(ValidationContext) error
}
```

规则：

- 执行所有权由**类型表达，而不是可空性检查**。`Definition` 不可编译；只有 `*Executable` 可以。两个构造函数各自恰好安装一个所有者，且都不接受 `*Executable`，因此双重绑定与无所有者编译都不可表示。
- 唯一残留的非法值是 nil 或零值 `*Executable`，Compile 会像拒绝 `RiskInvalid` 与 `PresenceInvalid` 一样拒绝它。这是一次平凡检查，而不是函数基数组合矩阵——这正是 §4.4 所划的区别。
- 因为 `Executable` 持有调用方无法触及的冻结深拷贝，§5.7 的 Contract digest 不再是防止 Bind 后篡改的机制，而仅作为针对内部 bug 的纵深防御。
- 优先形态 1，然后形态 2，再形态 3。不得仅为获得 `*Executable` 而把单次调用函数体包成 Handler。237 个直接返回的 Shortcut 完成迁移的标志是删除其 `Execute` 字段，而不是改名。
- 当 Handler 受契约绑定时，`Compile` 调用 `ValidateContract`。不匹配是构造错误，不是延后的运行时不变量。
- `CompileFacts` 是封闭的、纯数据贡献通道。Compile 校验注解命名空间、重复键以及与 Contract/注册表事实的冲突。它不能注册 flag、替换 `RunE` 或修改 Cobra。
- `Contract` 不包含 MCP server/tool/载荷字段。
- `Definition.Effects` 是封闭的执行能力声明，独立于纯 CLI Contract。Compile 拒绝重复、非法 kind 以及 Risk/effect 冲突。它由**形态 3** 的作者手写；形态 1/2 留零值，由 `mcpbind.Bind` 从 `CallSpec` 推导（§5.7）。因此这个字段永远只有一个写入者，绝不出现「作者声明一份、绑定推导一份、再检查两者是否一致」的三方结构。
- `RiskInvalid` 被拒绝。遗留适配器把空遗留值显式映射为 RiskRead；新 Contract 中的省略绝不能静默关闭确认。
- 每条自定义 `Constraint` 都有稳定且非空的 `RuleID`。每条 `RequiredWhen` 也有自己的 `RuleID`。
- 四种关系类 ConstraintKind 是声明式的，不得被 `ValidatorSpec.Covers` 命名。`ConstraintCustom` 与 RequiredWhen 是仅有的命令式规则类；它们需要 ID，且恰好由一个 Validator 覆盖。未知/非法 kind 导致构造失败。
- `ValidatorSpec.Covers` 命名规则 ID，而不是复制规则元数据。命令式规则未被覆盖、被多个校验器覆盖、或被无处引用时，Compile 失败。即便覆盖多条规则，校验器也只运行一次。这表示今天 12 个钩子覆盖 14 条自定义约束且无重复执行。
- 在一个 Definition 内，自定义 Constraint 与 RequiredWhen 的 RuleID 唯一。`Covers` 只能命名这些命令式规则 ID；每个目标必须存在，每条命令式规则必须恰好被覆盖一次，每个 ValidatorSpec 必须有非空 `ValidatorID`、非空 `Covers` 和非 nil 函数。现存 14 条自定义声明获得签入的语义 ID；编译器不得从切片位置合成不稳定的序号 ID。遗留迁移期间这些 ID 是适配器侧元数据或非序列化字段，且不得改变 `shortcut list` JSON。跨命令审计身份是 `(CommandKey, RuleID)`；RuleID 不必全局唯一。
- `RequiredWhen` 规则只作为 RequiredWhen 元数据投影。覆盖其 ID 并不同时发布自定义约束，因此 Schema/帮助不会把同一语义规则渲染两次。
- 校验器接收只读上下文，无输出或后端能力。因此它们不能在 Risk 确认之前有意执行副作用。
- 规则的 ID/描述及其覆盖 ValidatorID 对帮助、审计与覆盖率夹具可用。仅当 Schema 线缆有诚实表示时才投影到 Runtime Schema；任意自定义规则不得被误标为受支持的关系约束。
- 位置参数与元数是声明式的。真正需要自定义 Cobra 解析的命令在其行为被建模之前保持为裸命令。
- 命令式 `Handler` 不编码执行发起多少次后端调用；形态 1/2 在构造上始终是一次调用。
- 所有可从本地 CLI/配置状态判定的解析、规范化、校验与可失败转换，必须在 Risk 确认前完成。形态 3 Handler 可在确认后执行依赖后端的校验，当该事实无法本地获知时。它必须在第一次写之前完成必要的读与检查，然后可在不写的情况下返回类型化的校验/业务错误。例如，devapp delete 可先确认 Risk，再拉取应用名，比对 `--confirm-name`，然后才发起 delete。
- **提示内容**必须由后端读派生的命令不适合这条单次确认流水线，在单独评审的分阶段确认模型出现之前保持为裸命令。

`ArgsSpec.Policy` 有非法零值，因此省略不能静默改变行为。当前 Shortcut 命令映射为 `ArgsArbitrary`，因为遗留 `mount` 让 Cobra `Args` 为 nil；全部 28 条当前 Leaf 挂载命令映射为 `ArgsNoArgs`。对 `ArgsNoArgs` 与 `ArgsArbitrary`，Compile 要求声明字段 `Positionals` 为空；这并不意味着 Arbitrary 拒绝运行时 argv。`ArgsDeclarative` 要求至少一个位置参数，只允许在必填项之后出现可选项，且至多一个可变参数位于最后一槽。必填可变参数接受一或多个值；可选可变参数接受零或多个。

### 5.2 构造 API

```go
type Clock interface {
    Now() time.Time
}

type CompileDeps struct {
    Env       EnvReader
    Clock     Clock
    Confirmer Confirmer
}

type BuildIssue struct {
    Path    string
    Code    string
    Message string
}

type BuildError struct {
    Issues []BuildIssue
}

func (e *BuildError) Error() string

func Compile(
    exe *Executable,
    deps CompileDeps,
) (*cobra.Command, error)

func MustCompile(exe *Executable, deps CompileDeps) *cobra.Command
```

- `Compile` 校验名称、重复 flag、别名、类型化默认值、约束、校验器 ID/覆盖、规则描述、位置参数、Risk 与 required 依赖。它拒绝 nil/零值 `*Executable`；所有者存在性在其余情况下由构造保证，typed-nil 所有者在两个构造函数内部被拒绝。
- `MustCompile` 仅用于发行版拥有的静态定义，并委托给 `Compile`。
- 用户自定义/运行时加载的 Shortcut 定义必须使用返回错误的路径。非法用户输入不得使 CLI 崩溃。
- 构造错误被确定性排序并聚合进 `BuildError`，以便作者一次修完所有声明问题。
- Compile 在入口处深拷贝并冻结全部 Contract 切片/映射/默认值；Compile 之后的调用方篡改不能改变已挂载表面或受契约绑定的 Handler digest。
- 环境与时间是注入依赖。解析不直接调用 `os.Getenv`，测试可提供封闭、确定的来源。

### 5.3 带类型键的纯 CLI flag

```go
type ValueType interface {
    ~string | ~int | ~bool | ~[]string
}

// Key 字段是私有的。作者创建一次键，在 Contract 中声明，
// 并在校验器/绑定/handler 中复用同一类型键。
type Key[T ValueType] struct {
    name string
    kind FlagKind
}

func StringKey(name string) Key[string]
func IntKey(name string) Key[int]
func BoolKey(name string) Key[bool]
func StringsKey(name string) Key[[]string]

type FlagSpec struct {
    Name    string
    Usage   string
    Kind    FlagKind
    Default DefaultValue // 类型化；由 StringDefault/IntDefault/... 创建

    Aliases []AliasSpec
    EnvVar  string
    Hidden  bool
    Trim    bool
    Empty   EmptyPolicy
    // 显式空字符串/列表是停止解析还是回落到别名/环境/默认值。
    // Leaf 与 Shortcut 今天行为不同，因此不能继续隐式处理。
    EmptyInput EmptyInputPolicy

    Required Requirement
    Enum     []string

    // Input 声明 flag 本身之外的额外取值来源：`@path` 读文件，`-` 读 stdin。
    // 为空表示仅使用 flag 值。
    Input []InputSource

    Format       string
    Example      string
    RequiredWhen *ConditionalRequirement
}

type InputSource uint8

const (
    InputSourceInvalid InputSource = iota
    InputFile  // @path
    InputStdin // -
)

type Requirement struct {
    Enabled  bool
    Presence PresenceMode
}

type ConditionalRequirement struct {
    ID          RuleID
    Description string
}

type PresenceMode uint8

const (
    PresenceInvalid PresenceMode = iota
    // 显式主/别名输入；bool/int 的 false/0 算存在，
    // 而 string/list 仍要求有意义的内容。
    PresenceExplicit
    // 显式主/别名输入或环境变量；排除注册默认值。
    PresenceSupplied
    // 已解析的有效值也可来自类型化默认值。
    PresenceEffective
)

func (k Key[T]) Declare(opts ...FlagOption[T]) FlagSpec
```

`DefaultValue` 有不导出的内部与类型化构造函数。`Compile` 拒绝 kind 与 flag 不匹配的默认值。非法静态默认值永不到达运行时。

类型键避免在每个 handler 内再次书写 flag 名与期望的 Go 类型。异构的 Contract 仍存储具体的 `FlagSpec` 值；`Key[T].Declare` 产生一个，而 handler 保留类型键。

`EmptyPolicy` 按 kind 定义何为有意义内容。`EmptyInputPolicy` 单独定义回落行为。这保留了显式 `false`/`0` 算存在、空白字符串或空列表不算存在等情况，而无需在解析器中硬编码框架特定行为。

只要启用了 Required 或关系约束，就拒绝 `PresenceInvalid`。适配器必须显式选择模式；省略的零值不能静默选择遗留语义。

只有 Contract 声明了该关系时，别名才贡献到逻辑键。现存 Shortcut 隐藏兼容 flag 保持为独立键，除非其上线校验与绑定语义已把它们当作别名。

`RequiredWhen` 不是纯文案。Compile 要求非空 ID/描述以及恰好一个 `ValidatorSpec.Covers` 引用；Runtime Schema 接收描述，而该校验器在运行时强制条件。

`Input` 是声明式的，因为替代方案是每个 handler 各自重实现 `@file`/stdin 解析与路径校验。它在 Resolve 内、任何校验之前解析，因此不可读路径、路径穿越尝试或同一命令行中两个 flag 争抢 stdin，都作为普通输入校验失败，从而发生在 Risk 确认之前。规则：

- 多个 flag 可声明 `InputStdin`，但每次调用最多消费一次；同一命令行中第二个 `-` 是校验错误，而不是静默空读。
- 文件内容受与字面量值相同的 `Trim`/`Empty` 策略约束，因此 `Input` 只改变值的来源，不改变其他语义。
- 路径解析复用现有的本地文件 effect 边界（§5.5.2）；它不是新的临时文件 API。
- 构造时拒绝 `InputSourceInvalid`。
- 当前没有任何 Shortcut 或 Leaf 声明 `Input`，因此 M1 增加能力且零上线表面变化。让现有命令采用它属于 §9 下的用户可见变更。

`Input` 的框架能力今日已在 `corecmd` 落地（声明即执行的过渡形态，语义与上文目标一致），使用指南：

**今日声明形态**：`FlagSpec.Input []string`，源常量 `corecmd.InputFile`（`"file"`）/ `corecmd.InputStdin`（`"stdin"`）。`helpers.LeafFlag` 是 `corecmd.FlagSpec` 别名，直接可用；`shortcut.Flag.Input` 同形声明，经 `FromShortcut` 映射到 `FlagSpec`。

```go
// LeafSpec / helpers
Flags: []helpers.LeafFlag{
    {
        Name:  "content",
        Usage: "文档内容（支持 @文件路径 或 - 读 stdin）",
        Bind:  "content",
        Input: []string{corecmd.InputFile, corecmd.InputStdin},
    },
}

// shortcut
Flags: []shortcut.Flag{
    {Name: "markdown", Desc: "Markdown 内容（支持 @文件路径 或 -）",
     Input: []string{"file", "stdin"}},
}
```

**运行时语义**（`resolveInputFlags`，在 `runDeclaredPreflight` 内、required/enum/约束/Validate 之前执行，原地改写 cobra flag 值）：

- `--flag @path`：文件内容替换取值；`--flag -`：stdin 内容替换取值。
- `--flag @@value`：转义为字面 `@value`，不做来源解析。
- 只解析显式 CLI token（主名或别名）；EnvVar 回落与注册默认值透传不解析。
- 内容前置剥离 UTF-8 BOM；`Trim` 等既有语义照常作用于解析后的值。
- 读取失败、源不支持、`@` 后空路径都是类型化校验错误（退出码 3）；同时声明两种源而文件读取失败时附 stdin 引导 hint。

**作者守则**：

- 声明即全部能力：required/enum/约束/Validate 校验的已是解析后的真实内容，`Execute`/`Invoke` 无需任何额外代码。
- `Usage`/`Desc` 必须写明支持 `@路径`/`-`；框架不自动改写 help 文案，今日也不向 Schema 投影（新增投影字段须先过 homology 评审，避免 catalog drift）。
- `user_required` 确认的写命令若声明 `InputStdin`：stdin 在校验阶段被消费，交互确认将 fail-closed 为 `confirmation_required`，此类调用必须显式 `--yes`（或 `--dry-run`）。
- **声明前先确认取值空间不会被前缀吃掉**：声明 `InputFile` 后，任何以 `@` 开头的合法值都会被当成文件路径（本产品尤其常见的是 at 提及类取值，如 `--at-user @zhangsan` 会报读取文件失败），用户只能改用 `@@` 转义；声明 `InputStdin` 后字面值 `-` 不可达（与 curl 等约定一致）。若该 flag 的正常取值可能命中这两种形态，就不要声明对应来源。
- 声明在构造期校验（fail-closed panic）：仅限 `KindString`；源值必须是 `file`/`stdin` 且不重复。

**今日实现与目标形态的差异**（迁移到本节目标 `FlagSpec` 时收敛）：

| 维度 | 今日 | 目标 |
|---|---|---|
| 源类型 | `[]string` 常量 | 类型化 `InputSource` |
| 路径边界 | 直接本地文件 IO | 复用 §5.5.2 本地文件 effect 边界 |
| Schema 投影 | 无（靠作者在 Usage 声明） | 声明即最终源，随 Catalog 透传 |

核心 FlagSpec 故意没有：

- `Bind`；
- `ArgDefault`；
- `Transform`；
- `OmitEmpty`。

那些是载荷绑定的关切。

### 5.4 一次性解析

```go
type ValueSource uint8

const (
    SourceUnset ValueSource = iota
    SourcePrimary
    SourceAlias
    SourceEnv
    SourceDefault
)

type ResolvedValues struct {
    // 私有异构存储；构造后不可变
}

type ValueView interface {
    // 封闭；仅由 ValidationContext 与 Runtime 实现
    valueView()
}

type ValueMeta struct {
    Source     ValueSource
    SourceName string
    Explicit   bool
    Supplied   bool
    Present    bool
}

func Get[T ValueType](v ValueView, key Key[T]) (T, ValueMeta)
```

性质：

- 没有任何公开 API 把异构存储暴露为 `any`，或返回原始 ResolvedValues 容器。校验器、handler 与 mcpbind 都通过 `Get[T]` 读取值；`ValueMeta` 单独携带来源/存在性事实。
- `any` 仅保留在天生异构的后端载荷/结果与输出边界；它从不作为已解析 CLI 值的 API。
- 解析顺序被声明并测试：显式主 → 显式别名 → 环境 → 类型化默认值。
- 整数环境值在解析期间解析。非法值在校验器或 `Handler.Execute` 之前返回校验错误。
- 未声明键与错误 kind 访问会用类型化的编程错误毒化本次调用。`Get` 可为控制流返回零占位，但流水线在每个 Validator 之后、提示之前、每个受管 effect 之前以及 Handler 返回之后检查毒化。它绝不能静默授权已批准的后端/输出适配器。
- **这是相对本 RFC 早期草案的一次有意原则调整，在此明确记录，而不是静默放松。** 早期草案要求「绝不降级为 `""`/`0`/`false`/`nil`」并让 `Get` 返回三值 `(T, ValueMeta, error)`。本版保留双值签名与零占位返回，同时把 §4.3 对原型的批评精确化：那里的缺陷是**静默**，不是零返回本身——原型的零值一路抵达 handler 且无人知情，也没有任何东西阻止它授权一次后端写。因此代价从「每个读取点一个 error 分支」换成「毒化标记 + §5.6 的四个强制检查点 + 许可即时铸造与原子消费」。这个取舍用调用点工效换取「坏读取绝不能授权 effect」由流水线而非作者纪律来保证。若评审认为四个检查点不足以覆盖每条 effect 路径，退路是给 `Get` 加回 error 返回值，而不是放宽毒化检查；这一点列入 §12 检查清单。
- 切片规范化与字符串 trim 只发生一次。
- 引用值在快照入口与出口被拷贝；`Get` 对切片返回拷贝，因此调用方不能篡改后续阶段观察到的数据。
- Required 检查与约束显式选择 `PresenceExplicit`、`PresenceSupplied` 或 `PresenceEffective`。仅有 Required 语义不够；约束需要同样的选择。现存 Leaf 解析器能力接受环境值但对约束排除注册默认值，这就是 `PresenceSupplied`；生产 Leaf 声明当前 `EnvVar` 字段为零。因此该模式是由直接测试覆盖的兼容保证，不是广泛使用环境回落的证据。
- 快照不可变，因此校验与执行不能观察到不同的 flag 状态。

#### 5.4.1 遗留 Shortcut 默认值编解码

类型化默认值并不授权清理现存强制转换。Shortcut 编译器使用显式兼容编解码，复现 `registerFlags`：

| 遗留 flag kind | 兼容规则 |
|---|---|
| string | 精确保留原始默认值 |
| int | 空或畸形文本经 `atoiDefault` 变为 `0` |
| bool | 仅精确字符串 `"true"` 变为 true；`"True"` 与 `"1"` 变为 false |
| string slice | 对整串两端 trim 空白；空变为 nil，否则在该 trim 后的字符串上按逗号拆分，不做逐元素 trim 或 CSV 解码 |

声明 golden 同时记录原始默认值与挂载后的 Cobra `DefValue`/有效类型值。迁移期间，差分不匹配是阻塞项；不能因为新行为看起来更干净就自动接受。

当前全部 78 个内置默认值都能按其声明 kind 成功解析。畸形值兼容风险主要来自用户自定义 v1 YAML，但遗留解码器仍保持显式，并为畸形 int、非精确 bool 文本以及空白/带引号的切片值提供夹具。

新的 Contract 原生定义使用严格的类型化构造函数。改变遗留内置或用户自定义 Shortcut 的强制转换，需要独立的、用户可见的变更，并附兼容分析、CHANGELOG 与更新后的夹具。

### 5.5 Runtime

```go
type InvocationIO struct {
    In  io.Reader
    Out io.Writer
    Err io.Writer
}

type InvocationOptions struct {
    DryRun bool
    Yes    bool
    Format string
    JQ     string
    Fields []string
}

type Confirmer interface {
    Confirm(
        context.Context,
        ConfirmationRequest,
        InvocationIO,
    ) (ConfirmDecision, error)
}

type Runtime struct {
    // 字段不导出
}

type ValidationContext interface {
    ValueView
    Context() context.Context
    Args() []string
    Clock() Clock
}

type EffectKind uint8

const (
    EffectInvalid EffectKind = iota
    EffectBackendRead
    EffectBackendWrite
    EffectNetworkRead
    EffectLocalRead
    EffectLocalWrite
    EffectOutput
)

type EffectPermit struct {
    // 字段不导出；携带调用上下文与 effect 作用域数据
}

type ContextPolicy uint8

const (
    ContextInvalid ContextPolicy = iota
    ContextPropagate
    ShortcutLegacyDetached // 仅临时迁移策略
)

func (r *Runtime) Context() context.Context
func (r *Runtime) Args() []string
func (r *Runtime) DryRun() bool
func (r *Runtime) Yes() bool
func (r *Runtime) Options() InvocationOptions
func (r *Runtime) Clock() Clock
func (r *Runtime) Permit(EffectKind) (EffectPermit, error)
```

`Runtime` 包含 CLI 执行能力，不包含 MCP 能力。

Runtime IO 按每次调用从已编译的 Cobra 命令经由 `cmd.InOrStdin()`、`cmd.OutOrStdout()` 与 `cmd.ErrOrStderr()` 解析。`CompileDeps` 不缓存 reader 或 writer，因此后续 `SetIn`/`SetOut`/`SetErr` 调用与测试隔离继续有效。

编译器安装的 `RunE` 是唯一保留 Cobra 命令的受管层。解析后它创建 `InvocationIO`，并把继承的 `dry-run`、`yes`、`format`、`jq` 与 `fields` 快照进 `InvocationOptions`。CompileDeps、Confirmer、Handler、Caller 与 ResultSink 不得捕获 Cobra 流或稍后重读这些 flag。尤其是，遗留 Shortcut 提示对 `os.Stdin` 的直接读取必须在差分测试前迁到 InvocationIO，且 `Output` 必须消费快照后的输出选项，而不是调用 `output.WriteCommandPayload(rt.cmd, ...)`。

Runtime 上不暴露裸输入/输出流。InvocationIO 仅传递给框架 Confirmer 与框架拥有的 sink 工厂。这阻止受管 Handler 实现未经评审的「后端读 → 自定义提示 → 输入循环」；此类命令在分阶段确认被建模之前保持为裸命令。

每一个获批的后端、网络、本地文件或输出 effect 都把 `EffectPermit` 传给其具名适配器。`Runtime.Permit` 拒绝非法或未声明的 kind、被毒化的调用、`ContextPropagate` 下已取消的上下文，以及 dry-run 下的后端写。适配器在交接时重新检查携带的上下文与毒化状态，并用该上下文执行操作。许可是一次性的，且在交接前即时铸造；原子消费拒绝零、过期、复用、错误调用或错误 kind 的令牌。因此后续的坏 Get 不能躲在更早铸造的许可之后。

仅当 M2 选择 bug-for-bug 兼容时，才允许临时且显式的 `ShortcutLegacyDetached` 策略，并随遗留执行器一并移除。ResultSink 通过许可接收输出作用域的 writer 与快照后的输出选项，从不通过 Cobra。

Dry-run 从不授予真正的 EffectBackendWrite 许可。兼容适配器必须保留更具体的已上线行为：直通 CallMCP/mcpbind 写通过 EffectOutput 发出精确的 tool/args 预览且不调用 Caller；CallMCPWriteData 保持失败闭合；已声明的后端读仍可获得 EffectBackendRead。本地/网络 dry-run 行为是能力特定的，必须匹配其差分夹具。

RiskRead 禁止 EffectBackendWrite。RiskWrite 与 RiskHighRiskWrite 可在确认后允许后端读，再跟写/输出。网络读与本地读/写需要各自显式的 Definition.Effects 与具名适配器；它们从不从 Risk 推断。未声明的 effect 是内部安全错误。

受管命令包不得导入裸后端 helper，也不得直接读取 `os.Stdin`/全局 writer；策略分析器强制该依赖边界。Go Handler 仍是受信任代码，但受支持的受管 effect 路径由机制把关，而不是仅依赖文案。

`ContextInvalid` 被拒绝。Contract 原生与 Leaf 定义始终使用 `ContextPropagate`；只有 Shortcut 迁移适配器可选择 M2 记录的临时分离策略。

注入的 Clock 有意保持最小（`Now`），并使当前基于时间的工作流可确定。仅当迁移后的轮询工作流需要时，才把感知上下文的 Sleeper 作为单独具名能力加入；handler 不直接调用 `time.Now`/`time.Sleep`。

在适配当前 helper 期间，可存在临时的 `Command() *cobra.Command` 兼容访问器，但每一次使用都必须登记。它不是首选 API，因为它在解析之后重新打开可变的 Cobra 状态。

#### 5.5.1 Shortcut 兼容 Runtime

后端中立性适用于 `command.Runtime`，而非每一个领域适配器。今天的 376 个 `Shortcut.Execute` 函数体依赖 Shortcut 拥有的、带 MCP、输出与便利方法的 `RuntimeContext`。契约编译器在不可变核心 Runtime 之上重建该门面，**作为迁移桥接，而非终态**：

```go
// package shortcut，不是 package corecmd
type RuntimeDeps struct {
    Caller Caller
    Sink   ResultSink
    Tagger AIMessageTagger
}

func adaptHandler(
    s Shortcut,
    deps RuntimeDeps,
) command.Handler {
    return command.HandlerFunc(func(core *command.Runtime) error {
        rt := newRuntimeContext(core, s, deps)
        return s.Execute(rt)
    })
}
```

该闭包是形态 3 Handler 包装器。它不把 Caller 或 MCP 载荷形状加入 Contract 或核心 Runtime。在双执行器窗口期间，376 个 Execute 函数体可保持不变，以便差分对等可度量。那是脚手架。按 §3.5 / §5.1，形态 1 与形态 2 的 Shortcut 离开该适配器：删除其 `Execute` 字段，执行迁到 `mcpbind.Bind`（需要时加 `WithProjector`）。只有残留的多步与纯本地集合保留 `adaptHandler`。以仍调用 `s.Execute` 的 376 个包装器结束迁移，不满足 §10 的残留标准。

其 `RuntimeContext` 门面委托给内部运行时值源。v2 源使用 ResolvedValues，且绝不能读 Cobra；临时、隔离的遗留源可继续读取规范命令，使同二进制回滚是真实的而非名义的。

方法迁移是显式的：

| 现有 `shortcut.RuntimeContext` 表面 | v2 来源 |
|---|---|
| `Str`、`Int`、`Bool`、`StrSlice` | 来自 `ResolvedValues` 的类型化 `command.Get` |
| `StrFirst` | 按已声明名称顺序取第一个非空值 |
| `IntFirst` | 显式兼容键的 `ValueMeta` 优先，然后是主有效值 |
| `Changed`、约束 helper 与 `RangeInt` | `ValueMeta` 与同一份不可变值 |
| `DryRun`、`Yes`、参数与 Clock | 核心 Runtime |
| `CallMCP`、`CallMCPData`、`CallMCPWriteData` | 注入的 Shortcut `Caller`，带读/写 EffectPermit，保留 dry-run 策略 |
| `Output` | 注入的 `ResultSink`，带输出 EffectPermit 与快照选项 |
| `AddAIMessageTag` | 注入/打标适配器，保留当前版本行为 |
| `Command` | 仅临时、已登记的逃生口；任何 flag/值读取都不得依赖它 |

M3 按此表审计全部 23 个当前方法/helper。两个直接消费 `Command().Context()` 的站点迁到 `Runtime.Context()`，并移除公开的 Command 逃生口。在回滚窗口期间，只有具名的 `legacyRuntimeSource` 可保留 Cobra 指针；v2 Runtime、Validate/Execute 函数体、Caller 与 ResultSink 不得保留。M6 删除这最后一个来源。

Shortcut Caller 方法接受显式上下文。目标策略传递 `Runtime.Context()`。迁移期间，§M2 要么在单独获批的变更中更新遗留行为，要么在两侧观测中安装显式临时的 `ShortcutLegacyDetached` 策略；不得在编译器重构内不可见地改变取消语义。

#### 5.5.2 具名的网络与本地文件 effect

RuntimeContext 方法清单不是完整的 effect 清单。例如 `chat +messages-resource-download` 在 `CallMCP*` 之外调用 `http.Client.Do` 并创建/重命名/删除本地文件。M0 审计每个 Handler 上的直接网络、文件系统、子进程与全局 IO effect。

已知的资源下载行为移到 Shortcut 拥有的 `ResourceDownloader` 之后。其适配器：

- 接收 Runtime.Context 与类型化输入；
- 在每一类操作前立即请求已声明的 EffectLocalRead、EffectNetworkRead 与 EffectLocalWrite 许可；
- 保留路径/禁止覆盖校验、HTTPS/重定向限制、临时文件清理与原子发布行为；
- 在 dry-run 下不执行任何网络或文件系统操作，并通过 EffectOutput 发出精确的既有计划。

Execute 函数体使用该具名适配器，而不是直接使用 `net/http` 或 `os`。M0 发现的任何额外直接 effect 要么获得类似的窄能力，要么保持遗留/裸；全部 376 条的验收标准不能靠把此类调用藏出清单来达成。

### 5.6 受管执行流水线

对每条受管命令：

1. Cobra 解析 flag 并检查位置参数元数。
2. `Resolve` 构建不可变的类型化快照。
3. Required 与 Enum 规则校验快照。
4. 声明式约束校验快照。
5. 被覆盖的命令式校验器按声明顺序各运行一次。
6. 检查值访问毒化与执行上下文。
7. 运行 Risk 确认。
8. 再次检查值访问毒化与执行上下文。
9. 执行所有者运行：形态 1/2 派发器或形态 3 Handler，经由许可把关的 effect。
10. 返回值与任何已记录的编程/effect 错误合并。

任何受管定义都不能退出步骤 2–9。形态 1/2 没有作者 Handler；派发器仍受同一流水线约束。

核心不构建后端载荷。它不关心步骤 9 发起零次、一次还是多次后端调用。

步骤 6/8 的上下文部分遵循显式 ContextPolicy。`ContextPropagate` 在 `ctx.Err` 时失败；临时的 `ShortcutLegacyDetached` 在双执行器窗口期间复现遗留取消行为，同时仍对值毒化失败。M6 移除分离分支。

RiskRead 无需提示。Dry-run 跳过 write/high-risk 提示，但保持写许可禁用，使执行所有者只能渲染兼容预览；Yes 跳过提示并许可已确认的执行路径。

步骤 9 中依赖后端的状态守卫不是框架 Validator，即便为兼容性其不匹配以校验错误类别呈现。它可在确认后读，但必须在第一次变更前完成。形态 1/2 在构造上没有此类守卫；形态 3 可以有。

### 5.7 MCP 绑定适配器

`internal/corecmd/mcpbind`（或具有相同依赖方向的兄弟包）拥有载荷塑形：

```go
type Binding struct {
    // 私有声明
}

type plan struct {
    // 私有；包含已校验 Contract 的规范 digest
}

type IncludeMode uint8

const (
    IncludeInvalid IncludeMode = iota
    WhenExplicit
    WhenSupplied
    WhenEffective
    Always
)

func Param[T command.ValueType](
    backendName string,
    key command.Key[T],
    include IncludeMode,
    opts ...BindingOption[T],
) Binding

type CallSpec struct {
    Product  string
    Server   string
    Tool     string
    Effect   command.EffectKind
    Bindings []Binding
}

type Caller interface {
    Call(
        command.EffectPermit,
        CallRequest,
    ) (CallResult, error)
}

type ResultSink interface {
    Write(command.EffectPermit, any) error
}

// ResponseProjector 在 ResultSink.Write 之前整形单个 CallResult。
// 它是纯的：无 IO、无后端调用、无确认、无格式猜测。
type ResponseProjector func(CallResult) (any, error)

func IdentityProjector() ResponseProjector

func Uppercase() BindingOption[string]
func PayloadDefault[T command.ValueType](T) BindingOption[T]

func Bind(
    base command.Definition,
    call CallSpec,
    caller Caller,
    sink ResultSink,
    opts ...BindOption,
) (*command.Executable, error)

func WithProjector(ResponseProjector) BindOption
```

规则：

- `Bind` 是形态 1/2 唯一的公开装配 API。它相对 `base.Contract` 校验每个绑定键，合并后端事实，安装框架派发器作为执行所有者，并返回 `*Executable`。调用方不能把从 Contract A 编译的 Plan 与 Definition B 分开组合。
- `Bind` 接受 `Definition`，从不接受 `*Executable`；`command.WithHandler` 亦然。声明驱动派发与命令式 Handler 之间的互斥因此是结构性的；无需经评审的覆盖或运行时检查。
- **形态 1/2 不手写 `Definition.Effects`。** `Bind` 从 `CallSpec` 推导出封闭集合 `{CallSpec.Effect, EffectOutput}` 并把它安装进 `*Executable`。派发器随后申请的正是这两个许可（见下文），因此 §5.5 的「未声明的 effect 是内部安全错误」不会被派发器自己的路径触发。作者在形态 1/2 定义上把 `Effects` 设为非零是 `Bind` 错误，不是需要与 `CallSpec.Effect` 比对的「一致性」检查——同一事实只有一个来源，因为只有一处写它。要求作者写 `[后端 effect, EffectOutput]` 再检查它「与单数 `CallSpec.Effect` 一致」是自相矛盾的规则，会稳定产出 376 份错误声明。
- 手写 `Effects` 只属于形态 3（`command.WithHandler`），那里没有 `CallSpec` 可推导，且多步/本地 effect 集合本就无法从任何单一事实导出。§5.5.2 的具名网络/本地文件 effect 因此始终是形态 3 的显式声明。
- `Bind` 仍按 §5.5 校验推导结果：`CallSpec.Effect` 必须与 Contract 的 `Risk` 相容——`RiskRead` 不得推导出 `EffectBackendWrite`——否则返回构造错误。推导免除的是作者的重复劳动，不是校验。
- 派发器保留其校验过的深拷贝 Contract 的规范 digest，并实现 `ContractBoundHandler`。由于 `Executable` 持有冻结副本且调用方无法触及，该 digest 是针对内部 bug 的纵深防御，而不是防止调用方篡改的保证。
- 编码通过 `Param[T]` 捕获的类型键经 `command.Get[T]` 读取；它从不接收原始字符串或公开 `any` 值。
- 包含/省略通过 `IncludeMode` 显式表达，并与编码正交。flag kind 从不静默决定是否发送 `0`、`false` 或空值。Shortcut 模式 `if rt.Changed(name) { params[…] = … }` 映射为 `WhenExplicit` / `WhenSupplied`；它不得继续以手写形式留在形态 1 候选内。
- 仅载荷默认值留在绑定中，且从不满足 CLI required 规则。
- `Bind` 拒绝 nil/typed-nil 的 Caller 或 ResultSink、未知键、重复后端键、类型不匹配、非法 include/effect 模式、Contract Safety.Effect/后端 effect 冲突以及不兼容的默认值。
- 绑定选项有不透明内部，且对内置全函数编码器与类型化载荷默认值仅有封闭、具名的构造函数。没有公开的 `func(string) (any, error)` 转换钩子。可失败的用户输入转换属于 Resolve 或 Validator，因此在 Risk 之前完成。
- 「全函数」指编码器对任意 T 值都不返回用户错误、也不 panic；内置编码器对其输入域有穷尽测试。
- 在成功的 Bind、command Compile 与 Resolve 之后，在派发器内物化载荷不能产生用户校验错误。它仍可能浮现内部不变量失败、后端失败或输出错误。
- 私有派发器在 Caller 与 ResultSink 之前立即请求 `CallSpec.Effect` 与 EffectOutput 许可。两个接口都不接受裸上下文或 writer，因此上下文、dry-run 与毒化调用检查不会被意外跳过。
- dry-run 下派发器根本不被调用。§5.11 拥有该路径：框架从 `CallSpec` 推导计划，交由当前渲染器渲染后返回——M1 是复刻遗留信封的兼容渲染器，原生计划渲染是后续 approved delta（§5.11.1）。没有第二条、派发器本地的预览分支。预览载荷/文本仍是每条形态 1/2 命令差分夹具的一部分。
- 输出委托给显式的 `ResultSink`；适配器不写全局 stdout，也不猜测格式。
- **响应投影是第三条声明腿。** `ResultSink` 写出一个值；`ResponseProjector` 从 `CallResult` 产生该值。投影器：
  - 仅接收调用结果并返回 `(any, error)`；
  - 不得调用 Caller、Confirmer、依赖 Clock 的副作用，或写 Output；
  - 可选：省略即形态 1（`IdentityProjector`）；
  - 是今天调用 `CallMCPData` 再调用本地 `*Project` helper 的 108 个单次调用 Shortcut 的迁移目标；
  - 返回的 `error` 由框架**按既有分类透传，不重新包装**：`apperrors` 类型化错误原样保留其类别、消息与退出码；非 `apperrors` 的裸 error 映射到今天同一批 `*Project` helper 在 `Execute` 内返回同类 error 时所落入的那个类别。这条映射不是新政策，而是对现状的固定；M0 交付物 8 把这些边缘错误的当前类别记录为 golden，M5b 阶段 A 的逐条对等对照它比较。没有这条规则，M5b 阶段 A 那句「错误类别、消息与退出码逐条对等」按构造无法满足，因为框架侧的归类未定义；
  - 其 error 发生在 `ResultSink.Write` 之前，因此可观测顺序是「后端调用已发生、输出未写」。这与今天 `CallMCPData` 成功后 `*Project` 失败的行为相同，并在差分夹具中显式覆盖，而不是留给实现推断；
  - **不是**通用投影 DSL。外层 `{count, list}` 信封以后可获得封闭 helper；在复用证据足够之前，82 个独立字段映射保持为具名 Go 函数。
- `Bind` 把后端身份注解合并进 `Definition.Facts`；command 投影这些事实而不导入 mcpbind。`CallSpec` 本身永不进入 command，也不涉及任意 `PostMount`。

当前生产 Leaf 总体只有两个 transform；二者都是大写字符串且不会失败。它们映射为全函数内置编码器。可失败的 transform 夹具仍是遗留路径的测试，不带入新的 mcpbind API。

LeafSpec 可保留为构造 Contract 加 mcpbind 派发器的小门面（今天是形态 1）。当第一个带响应投影器的 Shortcut 迁移时加入形态 2；Leaf 不需要投影器来证明路径。仅当门面没有工效或评审价值时才应移除。

### 5.8 裸 Cobra 逃生路径

裸命令使用独立 API，例如：

```go
type RawMeta struct {
    Owner         string
    Reason        string
    TrackingIssue string
}

func RegisterRaw(
    build func() *cobra.Command,
    meta RawMeta,
) (*cobra.Command, error)
```

裸命令：

- 不宣称 command 强制执行 Required/Constraint/Risk；
- 在迁移前保留各自经评审的 Schema 元数据；
- 不计入已收敛的受管定义。

该 API 仅对基线之后的**新增**静态裸叶子强制。现存手写叶子从挂载树清单（如 `test/fixtures/raw-command-baseline.json`）豁免；它们不被机械改写，也不各自需要新 issue。

策略是基于集合的，而不是基于计数的：

```text
currentRawKeys ⊆ grandfatheredRawKeys ∪ reviewedExceptions
```

- 基线记录可执行的非受管叶子，而非纯分组节点。
- 新的裸键需要带非空 owner/reason/tracking issue 的 `RegisterRaw` 以及经评审的例外。
- 重命名是删除加新增；新键需要获批的重映射。
- 用户自定义/动态命令有单独清单。
- 迁移/移除豁免键时在同一 PR 中从基线删除它，使其不能在未经新裸评审的情况下再次出现。不允许静默替换。

这比在受管 Contract 内放一个 `RunE` 字段更诚实。

基线允许有永久非零下界。子集规则防止未经评审的裸增长；它**不**承诺每个豁免命令最终都会变成受管。像 devapp delete 的先 get 再 compare 这类依赖后端的检查，可在 §5.1 下受管，当它们在通用 Risk 确认之后、第一次写之前运行时。实际提示必须由后端状态构造的工作流仍保持为裸命令。

未来设计可增加经审计的只读预检并返回 `ConfirmPlan`，但须先定义后端读分类、提示构造、TOCTOU 行为与重放/取消语义。那超出本 RFC。

### 5.9 取消与拒绝

确认返回类型化的哨兵/结果：

```go
type ConfirmDecision uint8

const (
    ConfirmInvalid ConfirmDecision = iota
    ConfirmAccepted
    ConfirmDeclined
    // ConfirmUnavailable 表示没有可用的交互输入，这不是拒绝。见 §3.4。
    ConfirmUnavailable
)

type cancellationKind uint8

const (
    cancellationInvalid cancellationKind = iota
    cancellationUserDeclined
)

type cancellationError struct {
    kind cancellationKind
}

func (e cancellationError) Error() string

var errUserDeclined error = cancellationError{kind: cancellationUserDeclined}

func IsUserDeclined(err error) bool
```

两个枚举都有非法零值，哨兵是不导出的不可变值。Confirmer 返回 `ConfirmDecision`；受管核心把 `ConfirmDeclined` 转为哨兵。调用方只能通过以 `errors.Is` 实现的 `IsUserDeclined` 归类它。在目标状态，根应用把它映射为渲染与退出码。

Confirmer 结果处理是确定的：非 nil 错误优先；nil 错误加 `ConfirmInvalid` 或未知值变为类型化内部错误；仅 `ConfirmAccepted` 继续；`ConfirmDeclined` 变为哨兵；`ConfirmUnavailable` 变为携带命令路径与 `--yes` 补救提示的类型化 `confirmation_required` 错误。不需要确认的 Risk 级别不调用 Confirmer。

确认消费 `context.Context` 与 Runtime 输入/输出；不得直接读取 `os.Stdin`。`context.Canceled` 与 `context.DeadlineExceeded` 仍与用户回答 "no" 不同。

关闭的输入流也与用户回答 "no" 不同。Confirmer 分类三种情况且从不合并它们：

| 情形 | Decision | 受管结果 |
|---|---|---|
| 交互式拒绝 | `ConfirmDeclined` | 用户拒绝哨兵 |
| 交互式接受，或 `--yes` | `ConfirmAccepted` | Handler 运行 |
| 无可用交互输入（EOF / 非终端） | `ConfirmUnavailable` | 类型化 `confirmation_required` 错误，绝不是拒绝哨兵 |

`ConfirmUnavailable` 之所以存在，是因为当前实现把 EOF 当作拒绝，从而对从未运行的写报告成功（§3.4）。下面的遗留拒绝映射器仅适用于真实拒绝；任何映射器都不得把 `ConfirmUnavailable` 变成 `nil`。这是有意的 approved delta，而不是留给门禁去发现的兼容性破口。
拒绝保证 Handler 不被调用且后端调用计数保持为零。在 Handler 交接前观察到的取消有同样保证。交接后竞态的取消经每个许可与后端请求传递；它不做「已开始的远程操作总能被召回」这种不可能的声称。

迁移行为：

- 临时结果映射器在编译器边界包装命令：在受管执行返回之后、根看到结果之前。
- Shortcut 编译器的映射器仅把匹配 `IsUserDeclined` 的错误变为 `nil`，仅为真实交互拒绝保留今天的「拒绝即成功」行为，别无其他。
- Leaf 编译器的映射器仅把该哨兵变为今天的类型化校验错误，保留退出码 3。
- 映射器按编译器族选择一次。它不存储在 Contract、mcpbind 或单条命令声明中，并在产品决策后删除。
- 在产品负责人选择目标行为并记录精确退出码之前，不做收敛。返回校验错误当前意味着退出码 3，而不仅仅是「非零」。

策略是封闭枚举，不是任意回调：

```go
type LegacyDeclinePolicy uint8

const (
    LegacyDeclineInvalid LegacyDeclinePolicy = iota
    PropagateDecline
    ShortcutDeclineAsSuccess
    LeafDeclineAsValidation
)

cmd.RunE = func(cmd *cobra.Command, args []string) error {
    err := executeManaged(cmd, args, definition)
    return mapManagedOutcome(err, familyPolicy)
}
```

构造时拒绝 `LegacyDeclineInvalid`；任何编译器族都不会意外得到零值策略。

这在技术上位于根退出映射之前：

```text
受管流水线返回用户拒绝哨兵
→ 编译器安装的 RunE 把它映射为 nil 或校验错误
→ 根接收映射后的结果
→ 根把任何剩余错误映射为进程退出码
```

编译器不分配整数退出码；它只在现有根映射器运行前保留遗留族结果。

### 5.10 Runtime Schema 字段权威

终态是**按字段组**一份权威，而不是一个全局注册表：

| 字段组 | 权威 | 必需的一致性检查 |
|---|---|---|
| 稳定规范身份、导航与精确排除 | identity collector（`ContractFinal.Identity` 声明；reviewed `schema_command_registry/` 已退役） | 每条收集的 identity 绑定一次；每个公开 Shortcut 有一个绑定或一个经评审的排除 |
| 活 CLI 名称、kind、默认值、帮助、required、enum、约束与位置参数 | 可执行 Contract | 挂载的 Cobra 表面与 Runtime Schema 调用事实匹配 |
| Flag 级 Hidden | 可执行 Contract | 挂载的 flag 可见性与感知隐藏的 Schema 投影匹配 |
| Shortcut 定义的 Cobra 命令 Hidden | 经评审的 Shortcut 可见性解析器：语义 catalog 优先，签入的公开 catalog 回落 | 解析器在 Contract 编译前应用可见性；Contract 携带解析值但不能覆盖它 |
| 非 Shortcut 受管定义的 Cobra 命令 Hidden | 可执行 Contract | 挂载的命令可见性与声明匹配 |
| Shortcut 列表成员资格与语义 disposition | 经评审的 Shortcut 可见性解析器 | public/all 列表成员资格与经评审决策匹配 |
| Runtime Schema / Agent 暴露 | identity collector 收集结果加精确排除 | 每个暴露叶子解析到活 Contract；排除显式且不重叠 |
| Safety 与运行时确认 | 可执行 Contract 的完整 `contract.SafetySpec`，或迁移期显式 annotate（如 `runtime_gate`）；见 §5.0 | `confirmation` 单独驱动运行时门，`effect` / `risk` / `idempotency` 原样发布，禁止跨字段机械推导；任一 Safety 字段非空时四字段必须齐全，否则构造期 panic；`ConfirmFirst` 只在 `confirmation=user_required` 时合法 |
| 后端 product/tool/载荷绑定 | mcpbind + 后端元数据 | 每个绑定引用真实的 flag/属性 |
| Agent 选择文案（`use_when`、`avoid_when`、摘要） | 声明：`ContractDecl.Selection` / `ProductDecl`（交付 provenance `contract_final`）；`schema_hints/` 已退役，禁止回潮 | 身份解析到活契约；选择文案不得创建 CLI 表面 |

冲突在 CI 中失败闭合。Selection / Safety / 参数事实须声明在 ProductDecl 或 owning leaf 的 ContractFinal 上；不得用 hints overlay 静默覆盖可执行 CLI 行为，也不得把 hints 写成 selection 权威。

该权威表不取代现有的多源 Schema 解析器。它使每个字段组的优先级显式：Contract / ProductDecl 供给可执行与 Agent 选择层，而 identity collector（原经评审 registry 的继任者）、版本化绑定、排除与后端元数据保留其声明职责。

### 5.11 声明式 dry-run 计划

Dry-run 安全性已由框架强制，不是缺口。遗留 runner 在 `--dry-run` 下拒绝非读工具，`CallMCPWriteData` 无条件拒绝，§5.6 保持写许可禁用。今天没有任何 handler 能在 dry run 期间写。

缺口是有用性，且按构造就不均匀：

| Handler 形态 | 当前 `--dry-run` 结果 |
|---|---|
| 单次直通调用 | 框架自动渲染结构化计划（工具加已解析参数） |
| 无手写预览的多步 handler | 校验错误，指示作者渲染预览 |

在当前注册表上测量（下列计数**暂定**，与 §3.5 一样以 M0 交付物 1/10 签入的脚本与 golden 为准）：168 个 write/high-risk 命令分布在 12 个服务，79 个源文件在 17 个包中使用多步 helper，而只有 9 个源文件显式分支 `DryRun()` 来渲染预览——分布在 2 个包：`internal/shortcut/chat` 5 个、`internal/shortcut/smart` 4 个。本 RFC 早期版本报的是「3 个包中的 10 个文件」，那次计数错误地把 `internal/shortcut/runner.go` 算作了命令声明；它是框架自身，`DryRun()` 在其中被定义并用于失败闭合，而不是某条命令在渲染预览。评审复核了这个数字，此处按复核结果更正。这类偏差本身就是 M0 交付物 1/10 必须先落地的理由。

因此对多数多步写命令，`--dry-run` 在用户请求计划时返回错误。那是安全且无用的，且对差分门禁不可见，因为两侧编译器会复现同一错误。

因此计划以数据形式归属执行所有者：

```go
// 计划不是 Definition 字段：它属于执行所有者。形态 1/2 在 mcpbind.Bind
// 内从 CallSpec 推导；形态 3 通过 command.WithHandler(def, h, command.WithPlan(p))
// 提供。对 write/high-risk 形态 3 handler 必需，其余可选。

type Planner interface {
    // Plan 在与 Validator 相同的只读预检能力下运行：
    // 已解析值、参数与 Clock，无写许可，无输出 writer。
    Plan(ValidationContext) (DryRunPlan, error)
}

type DryRunPlan struct {
    Summary string
    Steps   []PlannedStep
}

type PlannedStep struct {
    Product string
    Server  string
    Tool    string
    Params  map[string]any
    // Conditional 标记依赖先前步骤响应、因此无法在执行前完全解析的步骤。
    Conditional bool
    Note        string
}
```

规则：

- 由框架而非 handler 渲染计划，并遵守 `--format`/`--jq`/`--fields`。Handler 停止手写打印预览。`DryRunPlan` 是数据，渲染器是可替换策略：框架同时提供一个复刻遗留信封的兼容渲染器和一个原生计划渲染器，切换渲染器是 approved delta 而不是里程碑内的隐式变更（§5.11.1）。
- 形态 1 与 2 自动从已编译的 `CallSpec` 推导计划。这些命令的作者不声明 `Plan`；dry-run 是绑定的属性，这正是 Leaf 已有有用 dry-run 输出且零逐命令预览代码的原因。
- 形态 3（命令式 Handler）在命令为 write/high-risk 时需要显式 `Planner`：框架不能从单个 CallSpec 发明多步参数。
- dry run 执行 `Plan` 并返回；它从不进入 Handler 或形态 1/2 派发器调用路径。这比禁用写许可更强，因为它也移除了预览曾经做的读调用。
- `Conditional` 存在是为了让计划对尚无法知道的步骤保持诚实，而不是发明参数。隐瞒扇出的计划比错误更糟。
- 因为计划构造是预检，它可以在 Risk 确认前作为普通输入校验失败，从而保持 §5.1 的顺序规则完整。

#### 5.11.1 内部表示与渲染形态分开上线

计划的**内部表示**和它的**渲染形态**必须分两步上线，因为二者的兼容性含义不同。把二者绑在 M1 会同时违反 M1 的零增量门禁。

今天 28 条 Leaf 命令的 `--dry-run` 输出并不是这里的 `DryRunPlan`，而是遗留执行器的信封 `executor.Result`：

```text
{"invocation": {kind, stage, implemented, dry_run, canonical_product,
                tool, canonical_path, legacy_path, params},
 "response":   {dry_run, note, request /* 完整 JSON-RPC 信封 */}}
```

它与 `DryRunPlan{Summary, Steps[]}` 结构不同，按构造不可能字节等价。因此：

- **M1 交付机制、自动 mcpbind 计划推导，以及一个逐字复刻上述遗留信封的渲染器。** 28 条 Leaf 命令是首批用户，上线输出零变化：变的是计划的内部来源，不是用户看到的字节。渲染器可替换是这里的设计要点——`DryRunPlan` 是数据，渲染是策略。M1 的零增量定位与回滚窗口因此完整保留，不为一个展示格式让路。M1 门禁比较该渲染器输出与 M0 记录的 28 条命令 dry-run golden，要求字节相等。
- **把 Leaf 的 dry-run 输出切到 `DryRunPlan` 的原生渲染形态是 §9 下的 approved delta，不属于 M1。** 它与下一条捆绑在同一里程碑交付，届时一次性给出前后夹具、CHANGELOG 与更新后的差分期望。在此之前，原生渲染器只经直接测试覆盖，不挂到任何上线命令上。
- 策略门禁——每个 write/high-risk **形态 3** 受管定义声明 `Plan`——随命令迁移逐条可强制。形态 1/2 在构造上已覆盖。该门禁在遗留 Shortcut 分批上线前无法覆盖全部 168 条，因此覆盖率按里程碑报告，而不是事先声称。
- 用声明计划替换遗留手写预览，属于 §9 下的用户可见输出变更；上述 9 个预览文件各自以 approved delta 迁移。

---

## 6. 能力对等矩阵

在每一行都被建模或在构造时被显式拒绝之前，Shortcut 不能接入生产。

| 能力 | 上线 Shortcut/Leaf 行为 | 所需 v2 状态 |
|---|---|---|
| 命令 Use/Short/Long | 上线 | 精确快照对等 |
| 命令 Example/Tips | Shortcut 把 Tips 渲染为 Example | 已建模 |
| Shortcut 命令 Hidden | 经评审的可见性解析器是有效权威；Contract 携带解析值 | 编译器在 Contract 校验前应用语义 catalog/公开回落可见性 |
| 其他受管命令 Hidden | 声明拥有 | 在 Contract 中建模 |
| 命令别名 | 由 devapp PostMount 使用 | 已建模 |
| 位置参数元数/schema | Cobra Args + Runtime Schema 位置参数；当前全部 Leaf 命令为 `NoArgs` | 已建模；工作流 handler 可读 `Runtime.Args`，但本 RFC 中 mcpbind 仍仅限 flag 键 |
| String/int/bool/list flag | 上线 | 已建模 |
| 类型化默认值 | 使用 bool/int/list 默认值 | 类型化，精确 DefValue 对等 |
| Flag Hidden | 兼容别名依赖它 | 已建模 + 感知隐藏的 schema |
| Flag 别名/环境回落 | 框架能力；生产 Leaf 使用一个别名且 `EnvVar`/`RequiredHint` 声明为零 | 保留解析器能力与直接兼容测试 |
| Shorthand/deprecation/NoOpt | 当前 Shortcut/Leaf 总体无自定义 shorthand 或 deprecation；bool `NoOptDefVal` 由 Cobra 派生 | 快照断言精确空/派生值；受影响手写命令迁移前 M7 必须建模这些字段 |
| Required | 框架语义当前不同 | 显式 PresenceMode |
| Enum | Shortcut 强制 string/list enum | 运行时 + 帮助 + schema |
| AtLeastOne/ExactlyOne/MutuallyExclusive | 上线 | 运行时 + 帮助 + schema |
| RequireTogether | Schema/helper 能力存在 | 声明式支持 |
| RequiredWhen | Runtime Schema 能力存在 | 已建模元数据 + 校验器链接 |
| Range 与条件校验 | 当前命令式 | 已发布 ValidatorSpec |
| ConstraintCustom | 14 条声明 | 已发布 ValidatorSpec；永不丢弃 |
| 隐藏 flag 约束折叠 | Shortcut 仅投影公开事实 | 精确投影对等 |
| Risk/yes/dry-run | 上线 | 公共流水线 |
| Dry-run 预览内容 | 单次调用 handler 自动（遗留 `executor.Result` 信封）；79 个多步文件中仅 9 个手写渲染，其余报错 | 声明式 `Plan`，框架渲染；M1 用复刻遗留信封的兼容渲染器，原生渲染是后续 approved delta（§5.11、§5.11.1） |
| 来自 `@file`/stdin 的 flag 值 | 两套框架均不可用 | 声明式 `Input`；M1 无上线命令采用（§5.3） |
| 拒绝结果 | 框架不同 | 类型化哨兵 + 兼容策略 |
| 非交互确认 | 缺陷：EOF 被当作拒绝，退出 0，写被丢弃 | `ConfirmUnavailable` → `confirmation_required`；由独立 hotfix H0 先发，approved delta 而非对等（§3.4、§8） |
| Validate 顺序/错误文案 | 已上线行为 | 差分对等 |
| 后端调用参数/顺序 | 已上线行为 | 记录器对等 |
| 用户自定义 Shortcut | 运行时加载 | 返回错误的编译器，对等前保持遗留 |
| PostMount | 27 个源赋值 / 28 条上线命令 | 移除前先具名能力 |
| 声明 vs 执行 | Leaf：Bind+Call 执行且零 RunE；Shortcut：全部 376 条要求 Execute | 三种形态（§3.5、§5.1）；仅形态 3 需要 Handler |
| 响应投影 | 108 个 Shortcut 在一次调用后整形；82 个独立 helper；ResultSink 只写 | mcpbind.Bind 上的 `ResponseProjector`（§5.7）；本 RFC 无投影 DSL |

---

## 7. 迁移计划

旧的 S1–S9 序列由 M0–M7 取代。关键路径是：证据 → 原子 Leaf 采纳 → 确定的遗留接缝 → Shortcut 桥接 → 差分门禁 → 仅执行上线 → 声明形态转换。

最后一步不是可选润色。M0–M5 结束时每条 Shortcut 仍通过兼容包装器执行手写函数体；M5b 才把统一契约变成删除执行代码，而 §10 是对照其结果写的。

### M0 — 可复现证据与基线

交付物：

1. 签入 AST 普查脚本及其机器可读输出，并绑定到源提交。
2. 发布完整分类列表，而不只是聚合计数。
3. 捕获结构快照：
   - 全部 376 个内置（`shortcut list --all`，不是默认的 265）；
   - 全部已挂载的活 Cobra 叶子；
   - 全部 27 个 LeafSpec 源定义与 28 条上线命令；
   - Runtime Schema 输出。
4. 增加独立声明 golden，例如 `test/fixtures/shortcut-v1-contract.json`。它记录每个 Shortcut 的每个字段，包括隐藏 flag、Tips、`HasValidate` 与 `HasExecute`。若两侧编译器意外同向漂移，这是历史锚点。
5. 分别记录源码与上线总体：70 个非空默认值字面量构造点对 78 个挂载的带默认值 flag、17 个生产隐藏 flag，以及 §3.2 中精确的 Clock 清单。
6. 给全部 14 条当前自定义规则稳定语义 ID，并签入规则到校验器的覆盖 golden。
7. 签入完整 PostMount 能力清单与豁免的裸叶子键集。
8. 记录边缘情况的当前错误类别/消息与后端调用行为，包括畸形遗留默认值强制转换。
9. 把手写叶子分类为四种 disposition 之一，并与普查一并签入：**delete**（死亡、被取代或从未调用）、**fold**（可由 flag 表达的兄弟薄变体）、**migrate**（普通受管叶子），或 **capability-blocked**（按 M7 命名缺失的 Runtime 能力）。普查计数手写命令有多少；该分类决定项目打算保留多少。迁移成本与幸存者成正比，因此减法是 M7 范围的规划输入，而不是事后清理。此处不断言任何数字：分类由同一可复现脚本产出，暂定数会重犯下文指出的错误。
10. 签入 §3.5 Execute 形态普查的**脚本与** golden：对每个内置 Shortcut，记录其 `Execute` 是形态 1 候选（直接返回的单次调用）、形态 2 候选（单次调用 + 消费响应）、形态 3 多步，还是形态 3 纯本地。§3.5 的表在此项落入 P0 前为暂定；此后 golden 是 M5b 候选列表与 §10 残留标准的权威。分类器必须像每个其他 M0 产物一样，可由另一开发者从干净检出重跑。
11. 在 `MustCompile` 进入根路径前建立性能基线，**并在同一处签入由这些基线导出的绝对预算数字**（§M4），而不只是噪声地板；缺少绝对预算时逐 PR 相对阈值可被累积绕过：
   - `dws version` 与 `dws --help` 的冷子进程启动；
   - 每个门禁平台上的发行二进制大小与冷进程峰值 RSS；
   - 发行版与空配置的正常根构造；
   - 全部 376 个 Shortcut 的遗留构造；
   - 28 条上线 Leaf 命令的既有构建/执行路径。
12. 签入 28 条上线 Leaf 命令的 `--dry-run` 输出 golden，逐字保留当前 `executor.Result` 信封（§5.11.1）。M1 的兼容渲染器对照它比较；没有这份 golden，「dry-run 零增量」无法被证明，只能被声称。

M0 不能对 Contract 编译或受管 Resolve 做基准，因为这些组件尚不存在。M1 记录首个 command/mcpbind 组件基线；M3 记录首个 376 定义 Contract 编译器/桥接基线。在此之前，冷启动/根/遗留端到端测量是跨实现回归门禁。

在本步骤可复现之前，暂定的 1195/141/92/368/594 普查不用作硬规划输入。

现有接口快照覆盖全部 376 个 Shortcut 节点，但省略了 `Short`、`Long`、`Example`、flag usage 与 Schema 注解。M0 扩展而非静默重解释该夹具。

现有完整 Shortcut 假后端测试是有用种子：今天 375 个定义到达已记录的后端调用，1 个被校验有意阻塞。更丰富的基线必须保留该区分，并仍拒绝静默成功。

**门禁**：另一开发者可从干净检出重跑普查与快照并得到相同结果。在评判实现变更前记录基准噪声地板与预算。

### M1 — 原子核心、mcpbind 与 Leaf 采纳

这是一个合并边界，即便拆成可评审提交也是如此。不要合并一个休眠的第二核心，再等后续 PR 才在活命令上证明它。

一起实现：

- Contract、Definition 与可选的 Handler 接口；
- 三种声明形态：mcpbind 派发器（形态 1）、派发器加 `ResponseProjector`（形态 2），以及命令式 Handler（形态 3）；
- ArgsSpec、类型化默认值、ResolvedValues、ValueSource 与 PresenceMode；
- 受管执行流水线、RuleID/ValidatorSpec 与类型化取消；
- EffectPermit 加受管包导入/IO 策略分析器；
- 返回错误的 Compile、静态 MustCompile 与豁免的裸路径；
- 带全函数编码器与可选投影器的 mcpbind；
- 全部 27 个 LeafSpec 源定义，产出 28 条形态 1 上线命令。

在移除 PostMount 之前，建模其实际能力：

- `ArgsNoArgs`、`DisableAutoGenTag` 与命令别名；
- 后端身份注解与遗留偏好；
- 额外未绑定 flag 与隐藏兼容别名；
- 四条带 cursor 的命令。

LeafSpec 可保留为 Contract + mcpbind 之上的领域门面。在同一合并边界内，移除旧核心流水线及其 MCP 泄漏：

- `corecmd.Spec` 与原型 `Ctx`/`Invoke`/`Orchestrate`；
- `BuildArgs`；
- 核心的 `Bind`、`ArgDefault`、`Transform` 与 `OmitEmpty` 字段。

M1 有意采用穷尽对等加发布/回退流程，而不是同二进制 Leaf 选择器。总体固定且可枚举（一个文件中 27 个静态声明、28 条挂载命令，全部经 Call），而真正的选择器必须保留或复制完整的过时解析器、校验与 BuildArgs 路径。那会与上述原子删除矛盾，并加倍活安全面。合并必须有文档化的一步发布回滚流程。

此处「回滚」指重新部署保留的上一 GA 二进制/制品，而不是在 P2/P3 依赖其 API 之后选择性 revert P1。P1 在依赖里程碑之前接受隔离的候选发布/dogfood 浸泡，记录发布负责人与接受的回滚 SLO，保留先前制品，并执行回滚演练。Git revert 仅在依赖变更合并前有效。

若产品策略反而要求同二进制 Leaf 回滚，唯一诚实的替代是冻结的 `leaflegacy` 实现，且 BuildArgs 在其稳定窗口结束前只是被移动而非删除。这两种声称不能共存。

M1 还关闭入口，这使该里程碑本身就有价值，而不是仅作为 M3–M5 的脚手架。每个**新的**受管叶子必须通过新核心声明：

- `make policy` 在新增的静态 `&cobra.Command{}` 可执行叶子上失败，除非它是带经评审例外的 `RegisterRaw` 项，或纯分组节点；
- 门禁对照 M0 豁免键集比较，因此只约束新代码，从不要求现有命令迁移；
- 新的 write/high-risk 定义从一开始就获得 `ConfirmUnavailable` 处理与 §5.11 `Plan`，因此在遗留分批仍在迁移时，§3.4 类缺陷不能被重新引入。

没有该门禁，框架收益要到 M5 之后才出现，而手写总体在 M0–M4 期间继续增长并放大 M7。

**门禁**：

- command 与 mcpbind 直接测试，变更代码覆盖率 100%；
- 非法静态/动态定义在命令可执行前失败；
- 未知/错误 kind 访问返回编程错误；mcpbind 与 Shortcut 兼容桥在后端/输出副作用前失败闭合；
- 任何执行所有者（派发器或 Handler）都不得在 required/enum/constraint/validator/Risk 检查之前运行；
- `Definition` 不可编译，且没有不经恰好一个执行所有者就能得到 `*Executable` 的公开路径；Compile 拒绝 nil/零值；
- 受管包不能调用裸后端/stdio 路径，且获批的 Caller/ResultSink 实现拒绝非法许可；
- 全部 28 条命令的结构对等，包括帮助、Schema、注解、别名、隐藏 flag、cursor flag 与 enable/disable 实例化；
- 精确的 product/server/tool/类型化载荷、错误类别/消息/顺序、确认、拒绝、dry-run、输出与零调用对等；
- 两个大写编码器与含版本发布的写路径有显式夹具；
- Resolve、受管 no-op 与 mcpbind 的首个组件基线，同时 M0 端到端启动/根预算仍适用；
- 入口门禁对故意添加的裸可执行叶子夹具失败，对等价受管声明通过；
- `Input` 与 `Plan` 有直接测试——不可读路径、重复 stdin 声明、条件步骤渲染——即便尚无上线命令声明 `Input`；
- 28 条 Leaf 命令的 `--dry-run` 输出经兼容渲染器后与 M0 记录的遗留 `executor.Result` 信封字节相等（§5.11.1）；原生计划渲染器只有直接测试，不挂到任何上线命令；
- Catalog/Schema/接口零增量，**dry-run 输出亦零增量**。M1 不接受有意的表面增量。

### M2 — 确定的遗留 Shortcut 接缝

在不改变其生产语义的前提下，使遗留实现可测试：

- 向 15 个文件中的 16 个 handler 调用点注入 Runtime Clock；
- 向两处用量记录调用注入单独 Clock；
- 用按调用的 Cobra 输入替换遗留提示对 `os.Stdin` 的直接读取，并保留其当前 stderr 提示；
- 以 §3.4 已由独立 hotfix H0 修复为前提（§8）：M2 以**修复后**的行为作为差分基线，并且在重写该确认调用点时不得回退到「EOF 等于拒绝」。若 H0 因故未能先落地，M2 必须先落地它，且它仍是单独 PR 加单独 CHANGELOG，不与接缝重构混在同一提交；
- 通过调用接缝捕获输出/全局选项，供 M3 稍后以 `InvocationOptions` 支撑；
- 生产使用真实 Clock 与正常 Cobra 流，测试使用固定依赖。

不要把上下文传播静默折进这次重构。当前 Shortcut 后端 helper 以 `context.Background` 分离，而目标使用 Runtime.Context。在 M3 之前，选择并记录下列两策略之一：

1. 首选：单独的、用户可见的 P2b 把遗留实现改为传播命令上下文，带产品批准、CHANGELOG 与取消测试；然后差分两侧都使用纠正后的行为；
2. 兼容：两侧在上线期间使用显式 `ShortcutLegacyDetached` 策略，上下文传播作为独立变更稍后交付。

这是单独里程碑，因为把用作差分基线的代码变更与引入新编译器放在同一 PR，会使失败含义不清。

§3.4 修复不属于 M2，而是独立 hotfix H0，理由见 §3.4 末段。它仍必须早于 M3：一旦差分门禁存在，旧行为会成为两侧的必需输出，缺陷会被 CI 锁死；在门禁建成前修复，比事后给 168 条命令批例外更便宜。M2 与它的关系只有一条——M2 以修复后的行为为基线，并且不得在重写调用点时把它改回去。

**门禁**：在真实依赖下，遗留结构输出、错误、提示与后端观测保持不变（H0 已落地的 §3.4 行为即为基线，M2 自身不引入任何有意增量）；固定 Clock/IO 夹具确定，且 `SetIn`/`SetOut`/`SetErr` 隔离有效。取消观测匹配所选且显式记录的策略。关闭 stdin 的写调用为非零且零后端调用，交互式 "no" 保持当前结果，`--yes` 不受影响。

### M3 — Shortcut Contract 编译器与兼容桥

把表面编译与执行选择分开：

```go
type SurfaceCompiler interface {
    CompileSurface(Shortcut) (*cobra.Command, error)
}

type ExecutorMode string

const (
    ModeLegacy ExecutorMode = "legacy"
    ModeV2     ExecutorMode = "v2"
    ModeAuto   ExecutorMode = "auto"
)

type ExecutorKind uint8

const (
    ExecutorInvalid ExecutorKind = iota
    ExecutorLegacy
    ExecutorV2
)

type ExecutorSelector interface {
    For(CommandKey) (ExecutorKind, error)
}
```

在 M3 期间，`legacySurfaceCompiler` 仍是生产路径与独立对等预言机。P4 切换到规范表面后它仅用于测试。`contractSurfaceCompiler` 构建候选规范 Cobra 表面。运行时选择单独设计，且不得决定命令字段。

在 `command.Runtime + ResolvedValues + RuntimeDeps` 之上实现 §5.5.1 Shortcut 兼容 Runtime。其签入的方法清单覆盖全部 23 个当前方法/helper：

- 类型化访问器与 `Changed` 读取不可变快照；
- `StrFirst` 保留第一个非空顺序，而 `IntFirst` 在主有效值之前保留显式兼容键优先；
- 声明式关系 helper 迁入核心；其余兼容校验器读取同一快照；
- DryRun、Yes、参数、Context、Clock 与输出选项来自核心 Runtime；
- CallMCP/Data/WriteData、JSON/错误行为、AI 打标与 Output 位于注入的 Shortcut 适配器，不在 command；
- 当前两处 `Command().Context()` 调用迁到 `Runtime.Context()`；Validate、Execute 与共享适配器不得读 Cobra flag 或流；
- 隔离的 `legacyRuntimeSource` 仅为广告的遗留执行器保留旧的 Cobra 背书访问器，并随其删除。

兼容门面为动态 v1 定义保留字符串名 getter，其名称不能全部由静态 Go 键表示。因其旧签名不能返回错误，未知/错误 kind 访问记录粘性内部错误。流水线在每个兼容 Validator 之后、Risk 提示之前检查它（§5.6 步骤 6）；桥接也在每个后端/输出 effect 之前与 Handler 返回之后检查它。静默零值永不能授权工作。AST 夹具验证每个内置字面量名称与 kind。

12 个遗留 `Validate(*RuntimeContext)` 钩子与 Execute 分开包装。其校验门面由 ValidationContext 与能力拒绝的 Caller/ResultSink 实现支撑；任何尝试的 MCP/输出方法在调用或写之前返回类型化能力错误。这保留旧函数体而不授予 Risk 前的 effect。每个钩子有零调用/零输出测试。它们以后可直接迁到 ValidatorSpec，但该源码改写不得藏在编译器管道内。

契约编译器还必须携带类型化/遗留默认值、Args 策略、PresenceMode、Enum、Tips、解析后的命令可见性、感知隐藏的 Schema 投影、全部自定义校验器、精确校验顺序/错误文案以及族级拒绝兼容。

影子模式下生产继续挂载并执行遗留命令。测试编译两侧表面；生产从不执行两侧 handler，尤其是写。

**门禁**：全部 376 个内置在无丢弃能力的情况下编译；23/23 Runtime 方法有 v2 非 Cobra 映射；直接 `Command()` 使用归零；全部剩余 Cobra 值读取限制在已检查的遗留源白名单内；全部 12 个校验门面证明零 effect 能力；用户自定义非法声明返回 BuildError 而非 panic。本里程碑记录首个 Contract 编译/桥接组件基准基线；M0 端到端预算仍适用。

### M4 — 结构、差分运行时与性能门禁

#### 结构快照

至少比较：

- Use、Short、Long、Example；
- Hidden、Aliases、Args/元数、DisableAutoGenTag；
- 每个 flag 的名称、类型、DefValue、NoOptDefVal、usage、Hidden、注解、deprecation 与补全元数据；
- 命令注解与 Runtime Schema 约束；
- 可见帮助输出。

`Args` 比较的是**规范化后的有效元数行为，而不是字段身份**。遗留 `mount()` 构造 `&cobra.Command{}` 时只设 `Use`/`Short`/`Long`/`Hidden`，从不设 `Args`，因此 `cmd.Args == nil`（Cobra 对无子命令的叶子按 `ArbitraryArgs` 处理）；而 §5.1 把同一条命令映射为显式的 `ArgsArbitrary`，即一个非 nil 函数。两侧运行时行为相同，但任何反射式结构比较都不相等。快照因此把 `Args` 记录为封闭的规范化标签（如 `arbitrary` / `none` / `exact:N` / `minimum:N`），遗留侧由「`Args` 是否为 nil + 是否有子命令」推导，声明侧由 ArgsSpec 推导；比较发生在标签上。直接对函数值用 `reflect.DeepEqual` 会让第一个分批卡在一个假阳性上。同理，§M5 的三表面「字节相等」指序列化后的帮助/Schema/Catalog/结构快照字节，其中结构快照使用这里的规范化标签；它不是对 `cobra.Command` 做字段身份比较。

#### 差分运行时

对这些组合分别对记录假后端运行；从不调用真实写 API：

1. 遗留表面 + 遗留执行器（历史基线）；
2. 规范 Contract 表面 + 遗留执行器（实际回滚路径）；
3. 规范 Contract 表面 + v2 执行器；
4. 规范表面 + 使用精确清单的 `auto`。

组合 2 是强制的：它证明遗留执行器仍 bug-for-bug 观察 Contract 注册的 pflag/默认值状态，而不是仅证明两个孤立编译器彼此一致。

比较：

- 错误 nil/非 nil、结构化类别、精确消息与顺序；
- 确认提示与拒绝结果；
- dry-run / yes 行为；
- 有序后端调用轨迹；
- product、server、tool 与载荷；
- 确定性规范化后的输出载荷；
- 校验/拒绝时无调用。

记录器保留类型化载荷，并在任何 JSON 渲染前使用 `reflect.DeepEqual`，因此 `int`→`float64` 或 `bool`→`string` 的变化不能躲在等价 JSON 文本之后。

```go
type Observation struct {
    Panic  string
    Error  ErrorObservation // 具体类型/类别/原因/退出/消息
    Stdout string
    Stderr string
    Calls  []CallObservation // 有序 product/server/tool/类型化参数
}
```

生成用例覆盖：

- 缺失与仅空白的 required 值；
- 类型化默认值对显式值；
- 显式 false 与零；
- 隐藏别名与环境回落；
- 非法整数环境/默认值；
- 每个 enum 值加非法值；
- 每个关系约束的零/一/多成员；
- 自定义校验器；
- read/write/high-risk 确认路径；
- 预取消上下文与等待确认时的取消，对照显式 M2 取消策略；
- 畸形遗留默认值与每个全函数 mcpbind 编码器；
- 位置参数与 NoArgs。

扩展而非替换现有的 `TestAllShortcutsAssemble`：它已有假后端与全注册表迭代。

12 个 Validate 钩子各有策展失败夹具，14 条自定义约束每一条都由稳定 RuleID 与至少一个夹具命名。依赖时间的 handler 使用注入 Clock；门禁不得因规范化掉整段时间字段而掩盖真实增量。

对两侧修订都存在的基准，性能运行在同一 worker 上交错跑 merge-base 与 head：

- 预构建冷二进制，至少 30 个样本，比较启动 p50/p95；
- 比较发行二进制字节与冷进程峰值 RSS；
- Go 基准使用 `-benchmem -count=10` 与成对 `benchstat`；
- 根构造、全定义编译、Resolve 与受管 no-op 执行报告时间与分配。

**逐 PR 相对阈值。** M0 记录环境噪声地板后，time/op、B/op 或 allocs/op 上统计显著且超过 10% 的回归阻塞 PR，除非绝对变化低于该记录地板。冷启动 p95 超过 10% 也阻塞；二进制大小或平台特定峰值 RSS 超过 10% 需要显式性能批准。生产 `cmd_init` 遥测是 dogfood 信号，不是 CI 的替代。新引入的组件基准在 M1 或 M3 建立基线，并在后续 PR 成为增量门禁；它不假装有 merge-base 对应物。

**绝对预算，作为真正的上限。** 只有相对阈值会产生棘轮效应：约 400 条静态定义在 init 期 `MustCompile`，加上 M1 的入口门禁持续新增受管命令，10 个 PR 各 9.9% 可以累积到约 2.6 倍而永不触发逐 PR 门禁。因此签入一组固定的绝对上限，以 M0 交付物 11 的基线为锚：

- `dws version` 与 `dws --help` 冷子进程 p95 不超过 M0 基线 p95 的 1.25 倍；
- 每个门禁平台的发行二进制大小与冷进程峰值 RSS 各有绝对字节上限。

任何 PR 越过绝对上限即阻塞，**即便其自身增量低于 10%**；相对阈值退为附加门禁，用于抓住单次跳变。提高绝对预算本身是一次显式决策，需要性能批准、记录在案的理由，以及与基线一并更新的签入数字——预算可以调，但只能被看见地调。

**门禁**：全部 376 个内置加代表性用户自定义 Shortcut 夹具零未解释结构或运行时增量，且全部性能预算通过。

### M5 — 基于风险的上线与回滚

在第一批之前，把内置生产构造切换到一个规范的、由 Contract 构建的 Cobra 表面。其 `RunE` 包含两条兼容执行路径，并咨询**仅执行**选择器。选择器从不选择构造哪些 flag、帮助、注解或 Schema 事实。

这解决了 Catalog/选择器冲突。建议名 `DWS_SHORTCUT_COMPILER` 被拒绝，因为它暗示构造变更。使用发布作用域的 `DWS_SHORTCUT_EXECUTOR=legacy|v2|auto`：

- `legacy`：每个内置执行遗留流水线；
- `v2`：每个内置为 CI、dogfood 与复现执行受管流水线；
- `auto`：使用签入的精确 `(service, command)` 执行清单。

规范 `RunE` 在 Cobra 解析固定表面之后执行选择：

```go
cmd.RunE = func(c *cobra.Command, args []string) error {
    executor, err := selector.For(key)
    if err != nil {
        return err
    }
    switch executor {
    case ExecutorLegacy:
        return executeLegacy(c, args, shortcut, legacyDeps)
    case ExecutorV2:
        return executeManaged(c, args, definition, managedDeps)
    default:
        return errInvalidExecutorMode
    }
}
```

只有 `executeLegacy` 可实例化 `legacyRuntimeSource`；受管分支立即捕获 InvocationIO/选项并解析类型化值。

生产根始终挂载同一规范表面。Schema 生成器（`NewSchemaSourceRootCommand`）、Catalog 生成器与接口快照任务接收显式 `auto(checked-in manifest)` 选项，且从不读取环境变量。上线前，CI 独立构建旧遗留表面、Contract 表面与生产规范表面，并要求完整的 Cobra/帮助/Schema/Catalog 字节相等：

```text
遗留表面 == v2 Contract 表面 == 规范 auto 表面
```

这里的相等按 §M4 的定义执行：序列化后的帮助、Schema、Catalog 与结构快照字节相等，其中 `Args` 等函数值字段以规范化的有效元数标签参与比较。不存在对 `cobra.Command` 的反射式字段身份比较。

因此生成漂移仍是实际发布命令表面的证明。它有意**不是**所选执行器行为相同的证明；差分门禁提供该证据。尽管表面字节相同，选择器仍必要，因为运行时校验、确认、后端调用与输出可能不同。

建议分批：

1. 至多十个精确键的有界读密集金丝雀；
2. 其余隐藏/只读内置，按有界键批次；
3. 公开只读内置；
4. 拒绝策略签字后的 write/high-risk 内置；
5. 用户自定义 Shortcut 最后，经其单独的运行时加载编译路径。

选择器解析返回错误。非法值绝不能静默变成 `auto`。若当前根构造器不能返回错误，它挂载规范树以使帮助仍可用，然后在受管执行、确认或任何后端调用之前返回类型化配置错误。

`auto` 在调用前解析为封闭的 `ExecutorKind`（`legacy|v2`）；`ExecutorAuto` 不是可执行 kind。签入清单是内置上的全量精确映射：

```text
keys(manifest) == BuiltInCommandKeys
value(manifest[key]) ∈ {legacy, v2}
```

缺失、未知或重复键与非法值是构造/配置错误。新内置在没有显式初始清单项时不能合并。全部内置 v2 编译已在 RunE 能选择它之前成功，且没有从失败的 v2 执行到遗留的按调用回退。

运行时加载的用户自定义 Shortcut 有意不在该静态映射内。它们使用单独校验的 `DWS_USER_SHORTCUT_EXECUTOR=legacy|v2` 策略：在 P6b 前遗留保持默认；CI/dogfood 可强制 v2；仅在代表性与畸形 YAML 夹具通过两侧编译器后，P6b 才改变默认。稳定窗口内同二进制 `legacy` 覆盖仍保留。缺失的内置清单项绝不能与此动态回退混淆。

其回滚路径在第一批之前测试。

每个分批 PR 包含：

- 执行清单变更；
- 该批与全注册表的结构与差分结果；
- 零漂移证据；
- 回滚测试。

禁止执行清单变更改变 Catalog 或任何 Cobra 表面字节。有意的用户可见表面变更是单独 PR，使用正常兼容/CHANGELOG/产品决策流程，并在广告回滚期间必须被两侧执行器支持。

M5 期间不改写任何声明；仅执行器切换是本里程碑的迁移。声明形态转换是 M5b 的单独工作，发生在一批在 v2 上稳定之后。晋升要求全注册表强制 v2 CI 对等，以及无未解释观测增量的 dogfood 浸泡。上线按版本/分批；本仓库没有远程百分比上线机制。

在全部 376 个内置使用 v2 之后，至少保留遗留执行器与 kill switch 一个完整稳定发布（建议最少 30 天）。先切换默认；在稍后的清理发布中删除遗留。

在该窗口期间，每条 Shortcut 声明仍必须在两侧执行器下编译并通过差分测试。CI 拒绝会使广告的 `legacy` 回滚路径不可用的 v2-only 声明或特性。

### M5b — 声明形态转换（形态 3 → 形态 1/2）

M0–M5 统一了表面、预检与执行器，但让每条 Shortcut 仍经 §5.5.1 `adaptHandler` 包装器执行——即全部 376 条都是形态 3。停在那里不能满足任何 §10 残留标准，并恰好交付 §3.5 所拒绝的结果：376 个手写函数体被改名而非移除。本里程碑才是声明/执行分离真正兑现的地方。

转换有意**不**与 M5 的执行器切换捆在一起。改变哪个执行器运行与改变声明说什么是不同风险；把它们放在同一 PR 会使差分失败含义不清，理由与 M2 独立于 M3 相同。

它分两阶段运行，因为广告的 `legacy` 回滚需要 `Shortcut.Execute` 在 M6 之前保持可调用：

**阶段 A — 声明调用，保留函数体。** 对每个形态 1/2 候选，添加 `CallSpec`（形态 2 加 `WithProjector`），并把 v2 执行器经 `mcpbind.Bind` 而不是 `adaptHandler` 路由。`Execute` 留在声明中并继续服务遗留执行器，因此上述双执行器约束仍成立，且没有任何声明变成 v2-only。

本阶段有异常强的预言机：差分门禁现在把同一命令的遗留 `Execute` 函数体与新的声明式派发比较。等价性是度量出来的，而不是断言的。每条命令必需的对等：

- 每种支持的 `--format`/`--jq`/`--fields` 下的 stdout/stderr 字节；
- 后端 product/server/tool、载荷键/值与调用顺序；
- 校验与后端失败的错误类别、消息与退出码；
- dry-run 计划输出（§5.11），对这些命令从遗留单次调用预览迁到从 `CallSpec` 推导的计划；
- 拒绝与 `ConfirmUnavailable` 时零后端调用。

转换在定义上是行为保持的。任何增量都是要修的转换缺陷，而不是要记录的 approved delta——与 §3.4 不同，此处无意改变可观测行为。

**阶段 B — 删除函数体。** 在 M6 移除遗留执行器之后，已转换命令的 `Execute` 字段没有剩余调用者并被删除，连同阶段 A 移入 `ResponseProjector` 的投影 helper。在此之前它们在 v2 路径上已死但对回滚仍活，CI 必须断言二者仍可达。

排序与记账：

- 先转换形态 1 再形态 2：237 条命令只需要 `CallSpec`，而形态 2 还需要其投影器被评审为纯函数。
- 按服务分批，顺序与 M5 所用分批相同，使回归被熟悉的爆炸半径约束。
- 候选列表是 M0 交付物 10 的 golden。转换期间被证明误分类的命令在评审下更新 golden；§10 对照的是 golden，而不是 §3.5 的暂定表。
- 抵抗转换的候选保持形态 3，并带记录原因与具名缺失能力。因此残留可能超过暂定的 31；仅当每个例外被单独解释时这才可接受，而不是静默容忍。

**门禁**：对每条已转换命令，相对遗留函数体的差分对等在上述列表上字节精确；每 PR 报告已转换计数、残留形态 3 计数以及每个残留的原因；在遗留执行器仍能选择它时，不删除任何 `Execute` 字段。

### M6 — 移除遗留路径并收窄钩子

在每批已上线且回滚窗口结束后：

- 移除遗留表面预言机与执行流水线；
- 运行 M5b 阶段 B：删除每条已转换命令的 `Execute` 字段与被取代的投影 helper，这仅在没有任何执行器能选择遗留函数体时才安全；
- 移除 `legacyRuntimeSource`，使 Shortcut 不再有 Cobra 值读取；
- 若选择了兼容策略，移除 `ShortcutLegacyDetached`；
- 移除执行器环境开关与签入的上线清单；
- 在产品决策后移除临时拒绝兼容模式；
- 用具名字段替换剩余 PostMount 清单；
- 决定 LeafSpec 是否仍是有用的 MCP 门面；
- 仅在适配器零调用者且零独特语义时删除它们。

删除 LeafSpec 是要评估的结果，不是成功指标。

### M7 — 手写命令与 Schema 权威清理

对照稳定 Contract API 重跑可复现普查。

- 先经 mcpbind 迁移简单命令。
- 经同一 Handler 接口迁移多步命令；不要创建第二个「编排器」框架。
- 把纯分组节点保持为 Cobra 父节点。
- 按缺失的 Runtime 能力分类特殊 IO/上传/轮询命令，然后只增加普遍有用的能力。
- 不要求豁免的裸清单归零。有状态的、由后端派生的提示与真正自定义的 Cobra/IO 工作流可保持为经评审的永久下界。
- 在迁移任何受影响的手写命令之前，显式建模 Shorthand、Deprecated、ShorthandDeprecated 与类型化 NoOptDefault。在 mcpbind 消费位置参数值之前，增加类型化位置键；本 RFC 的 mcpbind 仅限 flag 键。

Schema 清理是单独项目：

- 使用 §5.10 按字段分配权威；
- 仅在冲突检查表明冗余来源不增加评审价值后移除它们；
- 保留经评审的身份/导航注册表。

---

## 8. PR 切片

实现由能力驱动，而不是由服务计数驱动。

H0 不属于本 RFC 的实现序列，列在此处只为记录其与 M2/M3 的时序约束。它不依赖 P0，也不依赖任何新核心代码。

| PR | 范围 | 上线行为变更 |
|---|---|---|
| **H0**（独立 hotfix，先发） | §3.4 非交互确认修复。唯一前置是 §11 决策 5；不依赖 P0–P1，必须早于 P3 | write/high-risk 命令在非交互进程中大声失败而不是退出 0；单独 CHANGELOG 变更 |
| P0 | RFC v5、可复现普查、减法分类、声明/裸/PostMount 快照、28 条 Leaf 的 dry-run golden 与性能基线加绝对预算 | 无 |
| P1 | 原子 Contract/解析器/mcpbind + 27 个 Leaf 定义 / 28 条上线命令；删除 BuildArgs/原型；入口门禁 | 无 |
| P2a | 遗留 Shortcut Clock、InvocationIO 与确定基线接缝 | 无 |
| P2b | 可选的首选上下文传播纠正（若获批） | 仅取消语义；单独 CHANGELOG 变更 |
| P3 | Contract 表面编译器 + 影子模式下的 23 方法 RuntimeContext 桥 | 无 |
| P4a | 完整结构/运行时/性能门禁，仍在遗留生产构造上 | 无 |
| P4b | 规范表面 + 执行选择器，全量清单初始全为遗留 | 无 |
| P5 | 隐藏/只读与公开/只读执行分批 | 仅执行器选择 |
| P6a | 拒绝决策后的 write/high-risk 执行分批 | 仅执行器选择 |
| P6b | 用户自定义上线；开始强制双执行器稳定窗口 | 仅执行器选择 |
| P6d | M5b 阶段 A：按服务声明形态转换，v2 侧从 `adaptHandler` 切换到 mcpbind 派发 | 无；相对遗留函数体的差分对等字节精确 |
| P6c | 一个稳定发布 / 至少 30 天后：移除遗留，运行 M5b 阶段 B（删除已转换 `Execute` 函数体），收窄钩子 | 清理 |
| P7 | 手写迁移与 Schema 权威工作 | 单独项目 |

PR #830 可携带 RFC 供评审。`Invoke`/`Orchestrate`/`Ctx` 是已合并的**过渡派发 API**（生产仍用），不得误标为「已接受的终态 P1 API」，也不得在未完成 mcpbind+Handler 里程碑前要求完整删除。

---

## 9. 每 PR 验证

每个行为保持的实现 PR 必须通过通用门禁：

1. `go test -count=1 ./...`
2. 相关竞态套件与跨平台编译测试；
3. 变更代码覆盖率 100%，且每个新包有直接测试；
4. `make policy`；
5. `make skill-command-integrity`；
6. 生成漂移检查；
7. `dws schema --all` 字节比较；
8. `dws shortcut list --all` 字节比较；
9. 相对 merge-base 与最新稳定 GA 的接口兼容；
10. 该阶段可用的每个基准：M0 端到端指标立即对照 merge-base，而新的 M1/M3 组件基准仅在其首次记录基线后成为增量门禁；
11. 干净、最新的合并修订，且每个必需远程检查为绿。

当其基础设施存在时，附加门禁变为强制：

| 阶段 | 额外必需证明 |
|---|---|
| H0 | 全部 168 个 write/high-risk 命令的关闭 stdin 写夹具，交互拒绝与 `--yes` 夹具不变，以及已记录的 §11 决策 5 退出信号 |
| P0 | 普查/声明/裸 golden、28 条 Leaf 的 dry-run 信封 golden、绝对性能预算数字，均可从干净检出复现 |
| P1 | 直接解析器/流水线/typed-nil/effect 测试、穷尽 28 命令对等、隔离浸泡与先前制品回滚演练 |
| P2a | 确定的 Clock/InvocationIO 测试与遗留观测对等 |
| P2b | 若选择，获批的上下文传播决策与精确取消夹具；否则显式分离策略对等 |
| P3 | 376 个定义编译、23/23 v2 RuntimeContext 映射、无直接 Command 使用，且 Cobra 读取限制在遗留源白名单内 |
| P4a 及以后 | 全部 376 个内置加代表性用户自定义夹具的完整 Cobra/帮助/Schema 结构与运行时观测 |
| P4b–P6b | `legacy`、`v2` 与 `auto` 执行器测试、全量精确清单审计、三表面字节相等与同二进制 Shortcut 回滚 |
| P6b | 单独用户自定义执行器策略、动态/畸形夹具与同二进制用户自定义回滚 |
| P6d | 每命令遗留函数体 vs 声明式派发对等（输出字节、载荷/顺序、错误类别、dry-run 计划、零调用路径）；已转换与残留形态 3 计数及每残留原因；`Execute` 仍可被遗留执行器到达 |
| P6c | 文档化的稳定发布/30 天窗口、双执行器证据、无剩余回滚依赖，以及对每个已删除 `Execute` 函数体零剩余调用者 |

有意的用户可见变更——**approved delta**——不折进迁移分批。它使用单独 PR，带：

- 精确的、经评审的前后夹具；
- CHANGELOG 条目；
- 产品决策；
- 更新后的差分期望。

最后一项使 approved delta 不同于对等失败：差分期望在评审下被有意编辑，而不是压制门禁。§3.4 是第一个此类增量。

任何测试都不得仅因 Schema/catalog 漂移为零就声称运行时等价。

---

## 10. 验收标准

收敛项目仅在以下条件全部满足时完成：

- 全部 376 个内置 Shortcut 与受支持的用户自定义形态经受管 Contract 路径编译；
- 全部 27 个 LeafSpec 定义（产出 28 条上线命令）经 Contract 路径（或保留的 LeafSpec 门面）编译且无行为增量；LeafSpec 门面在有证据前不必删除；
- 结构与差分运行时对等零未解释增量；
- 非法 Contract 或 mcpbind 输入从不产生可执行命令；
- 任何受管定义都不能绕过 Required/Enum/Constraint/Validator/Risk；
- 每条命令式规则由稳定 RuleID 恰好认领一次并被测试；
- 每个 PostMount 能力有具名表示，或留在显式裸清单中；`PostMount` 不得注册业务 flag；
- 新旧 Shortcut 执行器回滚经测试，且遗留执行器可移除；
- Runtime Schema 权威按字段文档化，冲突失败闭合；受管命令满足同源文档中的 `HOM-P*` / `HOM-S*` / `HOM-I1` / `HOM-D1`（或等价门禁）；
- 启动、构造、编译与按调用性能预算通过，**包括 §M4 的绝对冷启动/二进制大小/峰值 RSS 上限**，而不只是逐 PR 相对阈值；
- 全部必需本地与远程门禁为绿；
- **没有 `Execute`/`Call` 函数体只为装配参数而存在**：可声明的 flag→载荷、常量、包含策略、表面元数据一律走声明；门禁可证明 Call/Execute 字面量不再做 `params[k]=…` 类装配（具名工具注入如 cursor 除外）。执行体本身是一等扩展点，允许存在，不计入「残留」，不要求逐条审批理由。把 376 个 Execute 改名为 376 个 Handler 却仍只做参数装配，**不**满足本标准；反过来，保留真正的业务/投影/多步执行体**不**扣分；
- **flag / help / schema 参数面同源（路径 A）**：不存在以 MCP meta 为 flag 权威的主通道；若启用 1:1 透传子集，必须满足同源文档 §5 的准入条件并与 Leaf/Shortcut 路径互斥。

1195 暂定手写命令普查的迁移是后续项目，不是 Shortcut/Leaf 收敛的条件。

---

## 11. 仍需的产品决策

前三个决策不阻塞 M0–M4，但阻塞受影响的上线。决策 4 必须在 M3 差分运行时工作前做出。决策 5 是独立 hotfix H0 的唯一前置，与本 RFC 的里程碑无依赖关系，且应当被当作最紧急的一项来处理，因为缺陷正在生产环境静默丢弃写操作。

1. **拒绝退出行为**：保留退出 0、使用既有非零类别，还是增加专用取消码？建议：保留类型化、不导出的用户拒绝哨兵，在重构期间保持当前行为，并在单独的用户可见变更中决定目标。
2. **长期存在性语义**：兼容迁移后，Required/约束应收敛到显式还是有效存在？建议：二者都可声明表达；不要把来源语义藏在框架特定代码中。
3. **LeafSpec 门面**：保留面向领域的 MCP 构建器，还是要求直接使用 Contract + mcpbind？建议：保留到可与真实迁移代码比较调用点工效与维护成本为止。
4. **遗留 Shortcut 取消**：在编译器比较前纠正当前分离的 `context.Background` 行为，还是在上线期间保留它？建议：在单独 P2b 中纠正，带产品批准与 CHANGELOG；永不把该变更藏在 P3 内。
5. **非交互确认信号**（§3.4）：`ConfirmUnavailable` 应采取哪种可观测形式——专用退出码，还是带机器可读 `confirmation_required` 原因的既有非零类别？建议：为「需要确认」保留专用退出码，因为调用方必须区分「拒绝在没有 `--yes` 时运行」与「运行了但失败」，复用校验类别会使二者无法区分。该决策只阻塞 hotfix H0，不阻塞本 RFC 的任何里程碑；反过来，H0 也不应等待本 RFC 的任何进展。不能仅因这个决策未定就让当前行为继续原样发布——若决策在一周内未定，按建议方案先发 H0，退出码语义作为后续可调整项记录在 CHANGELOG 中，因为「大声失败但退出码待定」严格优于「静默报成功」。

---

## 12. 评审者检查清单

评审者应质疑：

1. Contract 字段是否真正后端中立。
2. 每个当前值来源与类型化默认值是否由 ResolvedValues 表示。
3. PresenceMode 对 Required 与约束是否足够。
4. 裸命令是否可能意外声称受管安全保证。
5. 自定义校验器在帮助/Schema 中是否仍可发现。
6. 差分门禁比较的是行为，还是仅命令元数据。
7. 用户自定义命令是否会在非法声明上 panic。
8. 每个 Runtime Schema 字段是否恰好有一个声明权威。
9. 上线与回滚是否经实际的规范表面与执行选择器拓扑运作。
10. 任何提议的删除是否移除了有用的领域门面，而不仅仅是重复。
11. 任何被 bug-for-bug 保留的行为是否其实是缺陷，以及兼容门禁是否会使其永久化（§3.4）。
12. 确认结果是否仍可在没有人类的情况下到达，以及调用方在那时观察到什么。
13. 声明的 dry-run 计划是否对无法预先解析的步骤保持诚实，而不是打印发明的参数。
14. 该计划是否在 M5 之前就交付任何价值，还是只在最后一批之后。
15. 是否仍存在只为装配参数而写的 `Execute`/`Call` 函数体；要求每个受管定义都有非 nil Handler 是否只会改名这些装配体；形态 1/2 表是否被误当成「必须删光执行体」的指标（§3.5、§5.7、§10）。
16. `ResponseProjector` 是否保持为纯 data→data 钩子，还是静默长成带 IO 与后端调用的第二个 Handler。
17. `Get` 的零占位返回加毒化，是否真的等价于「坏读取绝不能授权 effect」；§5.6 的四个检查点是否覆盖了每一条 effect 路径，还是需要给 `Get` 加回 error 返回（§4.3、§5.4）。
18. 「M1 无上线输出变化」是否对每一类输出都成立，包括 dry-run 信封；兼容渲染器是否真的逐字复刻而不是「结构等价」（§5.11.1）。
19. 差分门禁比较的每一个字段，是否都在「有效行为」而不是「字段身份」的层面定义；有多少字段是函数值（§M4）。
20. 每条门禁是否既有相对阈值又有绝对上限，还是可以被逐 PR 的小增量累积绕过（§M4）。
21. 每一条要求「逐条对等」的门禁，其比较对象的语义是否已在别处定义；有没有哪条门禁按构造无法满足（§5.7 投影器错误归类、M5b 阶段 A）。
22. 本 RFC 引用的每个计数，是否已由签入脚本产出，还是仍是暂定值（§3.5、§5.11）。
23. flag / help / schema 是否仍存在第二写入者（hints 改写 type/required、MCP meta 创建 flag、Safety 与 Risk 各说各话）；同源是否被误读成「用平台 meta 生成全部 CLI」（决策 16、[`flag-help-schema-homology.md`](flag-help-schema-homology.md)）。
