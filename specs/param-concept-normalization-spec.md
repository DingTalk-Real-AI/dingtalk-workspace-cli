# Spec：参数幻觉治理 —— 概念字典 + 逐命令归约

- 状态：Draft（待评审）
- 分支：`fix/param-hallucination`
- 关联草稿：[`docs/param-concept-dictionary-draft.json`](../docs/param-concept-dictionary-draft.json)、[`docs/param-concept-dictionary-draft.md`](../docs/param-concept-dictionary-draft.md)
- 评测来源：`param_alias_map_merged_20260720`（440 条 badcase）+ 参数幻觉与 dws 参数设计分析报告

---

## 1. 背景与目标

### 1.1 问题
LLM/Agent 调用 dws CLI 时高频出现"参数幻觉"：对同一实体在不同命令用不同拼写（`--group`/`--conversation-ids`/`--id`）、用业界习惯别名（`--keyword` 代 `--query`、`--size` 代 `--limit`）、或用形态变体（`--pageSize` 代 `--page-size`）。评测显示去重后 144 组幻觉 / 388 次，其中约 70% 属"同实体异名"，可通过归一化消除。

### 1.2 现状（已核实）
dws 已有一套**相当完整、但机制分散、无单一源**的运行时参数处理。本 spec 是"统一/收编"这些机制，而非从零造：
- **PreParse 改写流水线（已实现，`root.go` 注册）**：`AliasHandler`（camel/snake→kebab，如 `--startTime`→`--start-time`）、`StickyHandler`（拆粘连值 `--limit100`→`--limit 100`）、`ParamNameHandler`（近似拼写自动纠正）。
- **全局语义别名 `CrossProductAliases`（已实现，`cross_product_aliases.go`）**：12 组等价名 → 批量注册隐藏 flag。**本 spec 的概念字典（15 概念）即其上位替代。**
- **各命令手写隐藏别名 flag**：`calendar event list`（一次手写 8 个时间拼写变体）、`todo`(subject/content→title)、`sheet find`(find→query) 等，与 CrossProductAliases 叠加。
- **取值回落逻辑 5 套并存**：`flagOrFallback`、`mustFlagOrFallback`、手写 if/else 链、`shortcut.RuntimeContext.IntFirst`、`validateRequiredFlagWithAliases`。
- **flag 级 did-you-mean 已实现**：`SuggestFlagFix`（`pkg/cmdutil/flagfix.go`）在 unknown flag 时给 `Did you mean --xxx?`，用**动态阈值**（≤3→1 / ≤8→2 / 否则 3）+ `CommonFlagAliases` 语义快路 + 粘连检测；错误载荷含**结构化 `available_flags`**（非仅文本）。子命令级另有 sheet 家族的 `attachUnknownSubcommandGuard`。
- **零 `SetNormalizeFunc`**：形态归一由上面的 PreParse 流水线完成，dws 未用 pflag 库级钩子。
- 覆盖不一致：`calendar` 接受 `count`，`report` 不接受；概念字典未覆盖的命令仍散落手写。

### 1.3 目标
用"概念字典（唯一 reviewed 源）+ build 时逐命令归约生成器"取代散落手写；分层交付（形态/同义/模糊各归其位）；把 440 条 badcase 降级为回归夹具；新增命令自动被覆盖。

### 1.4 Non-goals
- 不解决"红类能力缺口"（命令本身缺参数/接口）——那是补功能，不是归一。
- 不改 `schema_catalog.json` 的 wire 格式（保持 #602 兼容基线）。
- 不引入运行时单体中间件；不新建第二身份源。
- 不做语义 NLU；归约全靠"概念成员表 ∩ 命令真实 flag"的机械规则。

---

## 2. 架构与数据流

```
[源] internal/cli/param_concepts.json      ← 唯一 reviewed 事实源（概念 + command_overrides）
       │  (go generate ./internal/cli)
       ▼
[生成器] internal/generator/cmd_param_aliases
       │  读 Cobra 树真实 flag + 概念，morph() 后求 ∩，∩=1 产出，∩≥2 护栏
       ├─► internal/cli/param_aliases_generated.go   （逐命令别名表，generated 禁手改）
       └─► 灌入 schema_agent_metadata（别名对 schema --all / CI 可见）
       ▼
[运行时] 复用现有 PreParse 流水线 + 错误路径（非新建中间件）
       ├─ ③ 扩展 AliasHandler/新增 PreParse handler：morph 后查②表 → --size 解析期即归一到 --limit
       ├─ ④ 统一取值：值已落 canonical，读 canonical；require-one-of 收口单一 helper
       └─ ⑤ 扩展现有 SuggestFlagFix：命中②表 ambiguous/block → 引导 --help；其余仍最近邻，均不静默改写
       ▼
[门禁] ⑥ validation_fixture 回归（走 embedded 交付路径）+ 共现 gate（scripts/policy）
```

**硬不变量**：`morph()` 归一逻辑在 build 期（求 ∩）与运行期（PreParse 流水线查表）**必须是同一份代码**。⚠️ 现状 build 用 `cmdutil.Morph`、运行期 `AliasHandler` 用 `toKebabCase` 是**两份实现**，③ 落地时必须统一成同一函数（`Morph`），否则生成期认为能配、运行期配不上 = 契约漂移。

---

## 3. 组件详设

### ① 中央概念字典事实源

**是什么 / 干什么**：唯一 reviewed 源，声明概念（成员/排除）与逐命令覆盖，取代散落手写别名。

**文件改动**：
| 文件 | 类型 | 说明 |
|---|---|---|
| `internal/cli/param_concepts.json` | 新增（源，**非生成物**） | 由 `docs/param-concept-dictionary-draft.json` 正式化 |
| `internal/cli/param_concepts.schema.json` | 新增 | 闭合校验契约，参照 `schema_command_registry.schema.json` |
| `internal/cli/param_concepts.go` | 新增 | loader + `go:embed` + 结构体 + 语义校验 |
| `internal/cli/param_concepts_contract_test.go` | 新增 | 结构/约束契约测试 |

**数据结构（Go）**：
```go
type ParamConcepts struct {
    Version    string                     `json:"version"`
    Morph      MorphRules                 `json:"morphological_rules"`
    Concepts   map[string]Concept         `json:"concepts"`
    Overrides  map[string]CommandOverride `json:"command_overrides"` // key = 命令主路径
    Fixture    ValidationFixture          `json:"validation_fixture"`
}
type Concept struct {
    Denotes      string   `json:"denotes"`
    CanonicalHint string  `json:"canonical_hint"` // 仅治理建议，运行时不用
    Members      []string `json:"members"`
    Excludes     []string `json:"excludes"`
    Risk         string   `json:"risk"` // green|yellow
}
type CommandOverride struct {
    Bind          map[string]string `json:"bind,omitempty"`          // realFlag -> conceptID
    ScopedAliases map[string]string `json:"scoped_aliases,omitempty"`// emitted -> realFlag（仅本命令）
    Block         []string          `json:"block,omitempty"`         // 拒绝归约，走 did-you-mean
    Ambiguous     []string          `json:"ambiguous,omitempty"`     // 共现白名单（reviewed）
    Confirm       bool              `json:"confirm,omitempty"`
    ScopeStrict   bool              `json:"scope_strict,omitempty"`
    Note          string            `json:"note,omitempty"`
}
```

**loader 期校验（否则 CI 失败）**：
1. 概念成员两两不重叠（一个拼写不能同属两概念）。
2. `members` 与 `excludes` 无交集。
3. `bind` 的 value 必须是已声明的 conceptID。
4. `scoped_aliases`/`bind`/`block` 引用的命令路径需存在（对齐 Cobra 树，见 §7 时序）。
5. 未知字段拒绝（closed schema）。

---

### ② build 时归约生成器

**是什么 / 干什么**：`go generate ./internal/cli` 的一步；把字典 + Cobra 真实 flag 编译成逐命令别名表与 schema 别名清单。

**文件改动**：
| 文件 | 类型 | 说明 |
|---|---|---|
| `internal/generator/cmd_param_aliases/main.go` | 新增 | 生成器入口，对齐 `cmd_schema_catalog` 等 |
| `internal/cli/param_aliases_generated.go` | 新增（**generated**） | 逐命令别名表，纳入 `check-generated-drift.sh` |
| `internal/cli/schema_agent_metadata/*` | 修改（生成） | 追加各命令 `accepted_aliases` |
| `internal/cli/generate.go`（或既有 `//go:generate` 聚合处） | 修改 | 注册新生成步骤 |

**核心算法（伪代码）**：
```
for cmd in cobraTree.runnableLeaves():
    F = { morph(f) : f for f in cmd.realFlags() }        # 归一形态 -> 原名
    ov = concepts.Overrides[cmd.path]
    aliasMap = {}                                        # morph(emitted) -> canonicalRealFlag

    # (a) 概念自动归约
    for concept in concepts.Concepts:
        eff = { morph(m) for m in concept.Members }
        eff |= { morph(rf) for rf,cid in ov.Bind if cid==concept.id }   # bind 并入
        hit = eff ∩ set(F.keys())
        if len(hit) == 1:
            canon = F[ the one in hit ]
            for m in eff \ {canon}: aliasMap[m] = canon
        elif len(hit) >= 2:
            assert cmd.path in ov.Ambiguous, FAIL("co-occurrence 未登记: %s %s" % (cmd, concept))
            # 共现：不产出别名

    # (b) 命令级 scoped_aliases（仅本命令；scope_strict 不外泄）
    for emitted, realFlag in ov.ScopedAliases:
        assert realFlag in F.values(), FAIL("scoped alias 目标非真实 flag")
        aliasMap[morph(emitted)] = realFlag

    # (c) block：从 aliasMap 移除并记入 blockList
    blockList[cmd.path] = [morph(b) for b in ov.Block]

    emit(cmd.path, aliasMap, blockList)
    emitSchemaAliases(cmd.path, aliasMap.keys())         # schema 可见
```

**生成器保证**：
- 全量重算（非增量补丁），对齐 `make generate-schema` 的确定性快照语义。
- 任一命令 `∩≥2` 且不在 `Ambiguous` → **生成失败**。
- `scoped_aliases`/`bind` 目标不是真实 flag → 失败。
- 生成物 byte 稳定，drift gate 守。

---

### ③ 形态/语义层：接入现有 PreParse 流水线（非新建 `SetNormalizeFunc`）

**是什么 / 干什么**：形态归一（camel/snake→kebab）**已由现有 `AliasHandler` 实现**；本步不引入平行的 `SetNormalizeFunc`，而是**把②的语义别名表接进同一条 PreParse 流水线**——在形态归一后查②表，把 `keyword→query`、`min-time→start` 这类语义别名改写成命令真名；命中 `Ambiguous`/`Blocked` 则不改写、留给⑤。落地后即可删掉各命令的语义/形态隐藏 flag。

**文件改动**：
| 文件 | 类型 | 说明 |
|---|---|---|
| `pkg/cmdutil/flagnorm.go` | 已存在 | `Morph(string) string` 已就位；运行期查表须**复用它**（勿再用 `toKebabCase` 另一份） |
| `internal/pipeline/handlers/alias.go`（或新增 PreParse handler） | 修改/新增 | 形态归一后，查 `generatedParamAliases[cliPath]`：命中别名→改写为真名；命中 ambiguous/block→不改写 |
| `internal/pipeline/cobra.go` / `Context` | 修改 | 让 PreParse handler 拿到当前命令的 CLIPath，以索引②表 |
| `internal/helpers/calendar.go` 等 + `cross_product_aliases.go` | 修改（清理） | ②表覆盖后删对应手写隐藏 flag；`CrossProductAliases` 12 组并入概念字典后下线 |

**流水线内的查表逻辑（复用 build 同一 `Morph`）**：
```go
// 在 AliasHandler 形态归一之后追加
m := cmdutil.Morph(bare)                          // 与 build 期求 ∩ 用同一函数
entry := generatedParamAliases[ctx.CLIPath]
switch {
case entry.isBlocked(m) || entry.isAmbiguous(m):  // 不改写，交⑤引导 --help
    // 原样放回，unknown flag 由⑤装饰
case entry.Aliases[m] != "":                      // 语义别名 → 真名
    rewrite("--" + entry.Aliases[m])
default:
    // 形态归一结果若命中真实 flag，AliasHandler 既有逻辑已处理
}
```
> 关键一致性：现状 `AliasHandler` 用 `toKebabCase`、build 用 `cmdutil.Morph`，二者必须统一为**同一函数**（`Morph`），否则生成期与运行期对同一拼写的归一结果可能分叉 = 契约漂移。

---

### ④ 统一取值回落入口

**是什么 / 干什么**：③②落地后别名在解析期已归一到 canonical，**纯别名 fallback 变冗余**（直接读 canonical）；真正的跨 flag 约束（require-one-of）收口成单一 helper。

**文件改动**：
| 文件 | 类型 | 说明 |
|---|---|---|
| `internal/helpers/helpers.go`（或 `pkg/cmdutil`） | 修改 | 保留唯一 `RequireOneOf(cmd, flags...)`；标注旧 helper deprecated |
| `internal/helpers/calendar.go` | 修改 | 删 `flagOrFallback(...,"page-size","size","count")`、if/else 链，改读 canonical |
| `internal/shortcut/runner.go` | 修改 | `IntFirst` 逻辑并入统一 helper 或改读 canonical |
| `internal/helpers/todo.go`、`minutes.go` | 修改 | `validateRequiredFlagWithAliases` → `RequireOneOf`（约束保留，别名清单删除） |

**迁移判定（逐调用点）**：
- 纯别名取值（同一实体多拼写）→ **删 fallback，读 canonical**（别名已由②③处理）。
- require-one-of（title/subject/content 至少一个）→ **收口 `RequireOneOf`**，别名清单从代码移入字典。
- 跨真正不同 flag 的读取（非别名）→ 保留，但用统一 helper 表达。

> ⚠️ 本项是**行为迁移**，风险最高：必须在 ③②稳定、⑥夹具铺好后逐命令迁，每命令迁移配夹具断言。

---

### ⑤ flag 级 did-you-mean（扩展现有 `SuggestFlagFix`）

**是什么 / 干什么**：flag 级 did-you-mean **已由 `SuggestFlagFix`（`flagfix.go`）实现**（动态阈值最近邻 + `CommonFlagAliases` + 粘连检测）。本步是**在其上增加②表路由**：未知 flag 先查本命令的 `Ambiguous`/`Blocked`，命中则**不猜单个候选、直接引导 `--help`**（共现无法二选一、block 的最近邻往往就是它正要拦的错映射）；未命中再走现有最近邻。全程**只提示不改写**（尤其写命令）。

**文件改动**：
| 文件 | 类型 | 说明 |
|---|---|---|
| `pkg/cmdutil/flagfix.go`（`SuggestFlagFix`） | 修改 | 入口先查 `entry.Ambiguous`/`entry.Blocked` → 引导 `--help`；否则保留现有最近邻/`CommonFlagAliases`/粘连逻辑 |
| flag 解析错误处理处（`root.go` 已接 `SuggestFlagFix`） | 保持 | 结构化 `available_flags`/`hint`/`actions` 已在，无需新建 |

**行为**：
```
unknown flag --xxx
  # 1) 已登记的共现存疑：不猜单个候选，直接引导 --help
  if Morph(xxx) in entry.Ambiguous:
      stderr "unknown flag --xxx; it is ambiguous on 'dws <path>'; run 'dws <path> --help' to pick the right flag"
  # 2) 已登记的 block（假同义词/不同实体）：绝不给最近邻
  #    （否则会把 block 正要拦的那个错映射又推荐回去），直接引导 --help
  elif Morph(xxx) in entry.Blocked:
      stderr "unknown flag --xxx; it is a different entity than it looks here; run 'dws <path> --help' for valid flags"
  # 3) 其余未知 flag（普通拼错）：最近邻提示，仍附带 --help 兜底
  else:
      cands = nearest(Morph(xxx), realFlags(cmd) ∪ conceptMembersOf(cmd), dist<=2)
      if cands: stderr "unknown flag --xxx; did you mean --{cands[0]}? (or run 'dws <path> --help')"
      else:     stderr "unknown flag --xxx; see 'dws <path> --help'"
  # 永不自动改写；退出非零
```
> 与 ①-block 协同：`block` 名单命中时**绝不自动改写、也不给最近邻建议**（blocked 名的最近邻往往就是它正要拦的错映射，如 `node`→`job-id`），统一引导 `--help`。
> 引导 `--help` 无需新增数据：`entry.Ambiguous` / `entry.Blocked` 已足以判定命中；若日后想在提示里直接列出候选真实 flag（如 `--user`/`--users`），需在生成表额外存该概念的真实交集，属可选增强。

---

### ⑥ validation_fixture 回归 + 共现 gate

**是什么 / 干什么**：两道门禁，防退化 + 防将来静默误归约。

**文件改动**：
| 文件 | 类型 | 说明 |
|---|---|---|
| `internal/app/param_alias_fixture_test.go` | 新增 | 读字典 `validation_fixture`，**走 embedded 交付路径**断言归约结果（含 did-you-mean/blocked） |
| `scripts/policy/check-param-concepts.sh` | 新增 | 校验字典 schema + loader 语义 |
| `scripts/policy/check-param-alias-cooccurrence.sh` | 新增 | 扫全量命令，∩≥2 且不在 Ambiguous → 红 |
| `Makefile` | 修改 | `generate-schema` 加生成步；`policy` 加两 gate |

**夹具断言语义**：
- `expect=<realFlag>`：模型给 `emitted`，经交付路径归约后命中 canonical。
- `expect=did-you-mean:ambiguous`：命中共现护栏，产出提示、不改写。
- `expect=did-you-mean:blocked`：命中 block，产出提示、不改写。

**为什么走 embedded 交付路径**：对齐 AGENTS.md——语义回归必须经最终 embedded loader/query 交付路径验证，生成器单测或 JSON count 不充分。

---

## 4. 关键不变量（评审必查）

1. **单一身份源**：别名只来自 `param_concepts.json`；代码里不得再手写别名清单（②后 grep 兜底）。
2. **morph 一致**：build 与运行时共用 `pkg/cmdutil.Morph`。
3. **概念纯度**：不同实体分属不同概念（`app-id`≠`app-key`、`folder`≠`space`）。
4. **∩ 三态**：`=1` 自动、`=0` 不适用、`≥2` 护栏（须 reviewed 白名单）。
5. **泛型名逐命令**：`id/name/type` 不入全局概念，仅 `bind`/`scoped_aliases` 处理。
6. **不静默改写写命令**：模糊层只提示。
7. **`required` 硬底线**：归一/别名不得降低 Cobra `MarkFlagRequired`。
8. **生成物只读**：`param_aliases_generated.go` 禁手改，drift gate 守。

---

## 5. 现有隐藏 flag 迁移 triage

对每个现存隐藏别名 flag 分三类处理（**逐个 reviewed，非盲替**）：

| 类别 | 例 | 处理 |
|---|---|---|
| (a) 纯形态变体 | `startTime`/`start-time` | 删，交③（现有 `AliasHandler` 已做形态归一） |
| (b) 真同义词 | `size↔limit`、`find→query` | 进概念字典，②生成；草稿缺的成员 reviewed 补 |
| (c) 非别名/独立/兼容 flag | `remind-at`(内部兼容)、`modified-start` | **保留**，字典不碰 |

**行为收敛提示**：统一字典后 `report` 也会接受 `count/limit` 等（现仅 `calendar` 接受）。属有意一致化，但为**行为变更**，需评审 + 夹具兜底，不得静默。

---

## 6. 分阶段落地（rollout）

| 阶段 | 内容 | 验收 |
|---|---|---|
| P0 | ①源 + schema + loader + contract_test | `check-param-concepts.sh` 绿；无运行时行为变化 |
| P1 | ②生成器 + `param_aliases_generated.go` + schema 别名可见 | drift gate 绿；`schema --all` 出现 aliases |
| P2 | ③ 扩展 PreParse 流水线（AliasHandler 查②表），试点 **calendar** 删手写隐藏 flag | 夹具 calendar 子集绿；`--help`/baseline 更新 |
| P3 | ⑥ 夹具全量 + 共现 gate 接入 `make policy` | 63 条夹具全绿；共现 gate 生效 |
| P4 | ⑤ 扩展 `SuggestFlagFix` 加 ambiguous/block 路由 | 命中②表引导 --help；写命令不改写 |
| P5 | ④ 逐命令迁移 fallback（风险最高，最后做） | 每命令迁移配夹具；`RequireOneOf` 收口 |

**最小可用切片 = P0+P1+P2(calendar)+P3**：先证明"删手写别名、行为不变、夹具全绿、契约可见"。

---

## 7. 时序与依赖

- 生成器需 `app.NewRootCommand()` 建好的 Cobra 树来枚举真实 flag；loader 的"命令路径存在性"校验同样依赖它。
- 依赖链：①→②→(③,⑤)；②产物→④；全程被⑥守。
- CI 顺序：`make generate-schema`（含②）→ `check-generated-drift.sh` → `check-param-concepts.sh` → `check-param-alias-cooccurrence.sh` → 夹具测试 → 既有 schema/catalog gate。

---

## 8. 测试计划

| 层 | 测试 | 位置 |
|---|---|---|
| 源校验 | 成员不重叠/excludes/bind 目标/未知字段 | `param_concepts_contract_test.go` |
| 生成器 | ∩ 三态、scoped/block、byte 稳定 | `internal/generator/cmd_param_aliases` 单测 |
| 交付回归 | 63 条 fixture 走 embedded 路径 | `internal/app/param_alias_fixture_test.go` |
| 共现 | 全量命令扫描 | `check-param-alias-cooccurrence.sh` |
| 形态 | `Morph` 幂等/边界 | `pkg/cmdutil/flagnorm_test.go` |
| did-you-mean | 最近邻/block 协同/不改写 | `pkg/cmdutil/flagsuggest_test.go` |
| 回归 | 既有 CLI smoke / interface-baseline | 既有 gate |

---

## 9. 风险与回退

**风险**：
- ④行为迁移可能改变取值优先级/空值语义（5 套写法语义本就不一致）→ 逐命令迁 + 夹具。
- 行为收敛（report 开始收 count）可能影响既有脚本 → 评审 + CHANGELOG 记录。
- 概念圈错（不同实体混一概念）→ 共现 gate + 概念纯度评审拦截。

**回退**（分支 `fix/param-hallucination`，小步提交）：
- 未提交：`git restore <file>`；新文件 `git clean -n` 确认后 `git clean -fd <path>`。
- 已提交未合并：`git revert <sha>`（按组件粒度）。
- 生成物需与源**一起回退或重生成**：`make generate-schema && ./scripts/policy/check-generated-drift.sh`，避免 drift 红。
- 每阶段独立 commit，回退粒度 = 阶段。

---

## 10. 待评审开放项

1. 概念字典 15 个概念的 `excludes` 边界是否正确（错放 = 误归约）。
2. `command_overrides` 中 `confirm/investigate` 项逐条定夺（date→start、query→name、chat conversation-info:user 等）。
3. 行为收敛范围（哪些命令允许新接受 count/limit 等）是否需要灰度。
4. 生成物形态：`.go`（embed 常量）vs `.json`+embed，与既有 `schema_*` 保持一致的选择。
5. `④` 是否一次性迁移还是长期渐进（保留旧 helper 一段时间）。
