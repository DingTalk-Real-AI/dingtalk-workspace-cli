# DWS 完整命令树性能专项 Plan

状态：Draft，P0～P3 已完成本地实现，待两平台 clean-head 验收。日期：2026-09-07。

本计划执行 [单入口、完整命令树与 Verified Schema Cache RFC](rfc-schema-runtime-cache.md)。目标是在始终构造完整公开 Cobra 树的前提下，降低 root help、version、Schema 和业务命令共同承担的构树内存与分配。

## 1. 架构基线

```mermaid
flowchart LR
    A[argv] --> P[制品与 metadata 预检]
    P --> B[一次完整 runtime tree build]
    B --> C[Cobra parse / Find]
    C --> D[统一 PreParse / validation / auth / Safety]
    D --> E[Schema handler 或业务 handler]
    E --> F[统一 output / cleanup]
    F --> G[telemetry enqueue; no flush wait]
```

所有公开调用共享 B～G。Schema cache 只改变 Schema handler 的 catalog 数据源。公开 help 直接从 B 得到的树渲染。不存在 launcher、argv 产品路由、utility-only tree、Schema 前置 handler 或 help projection。

Lark CLI 1.0.85 的可参考结构：生产 Build 每次挂载 utilities、service catalog 和 shortcuts；completion 只控制 callback 注册，不改变命令树来源。固定 SHA `13305ae51b62833c6cd07368be48ae523bb491df` 在本机约 905 个 command，暖 Build 约 7 ms / 10.3 MB / 84.6k alloc。

DWS 当前约 1,825 个 command。绝对总量不能只和 Lark wall time 比较；同时比较每节点 B/op、allocs/op、进程 RSS 和输出合同。

## 2. 实施阶段

### P0：删除双模式与第二投影

- [x] 删除 process argv route、产品/shortcut route index 和 selective-tree 构造器。
- [x] process、public、help、version、config、业务与 completion 均构造完整产品面。
- [x] 删除 Cobra 前的 Schema argv fast path；verified cache 留在正常 Schema command 内。
- [x] 删除 root help model/snapshot package；renderer 直接遍历 runtime tree。
- [x] 删除对应资格探测、回退矩阵、route benchmark 与 drift tests。
- [x] 新增跨平台测试，对 help、Schema、config、业务 argv 检查完整产品面。

交付标准：代码中不存在“选择性树与完整树等价”问题，因为公开运行时只有完整树。

### P1：压缩 typed metadata 的所有权

- [x] ContractFinal payload 在 builder 完成规范化和必要深拷贝后，转移给 command-owned weak store，避免第二次完整 clone。
- [x] RuntimeContractFinal 的读取仍返回 defensive deep copy。
- [x] RunE closure 不再捕获已经写入 Cobra/ContractFinal 的 Use、help prose、Contract 和 PostMount 等 build-only 字段。
- [x] 框架直接提取私有 `executionSpec`，替代捕获已清零的大型 `Spec`；所有 `corecmd.New` 调用自动受益，产品声明与 adapter API 无需迁移。该结构只承载已有执行流水线，不参与命令注册、Schema 或 help 声明。
- [x] 对 `ParamDecl.Enum`、`Required` 等嵌套引用增加调用者 mutation 隔离测试。

交付标准：Safety、identity、selection、result schema 和参数合同与父提交一致；废弃树仍可由 weak store 回收。

### P2：执行期对象延迟初始化

- [x] Safety content scanner 在首次真实 payload 检查时才编译规则，不在完整构树时初始化。
- [x] disabled scanner 保持 nil，enabled scanner 的第一次 ScanPayload 前不得初始化。
- [ ] 用 CPU/heap profile 继续审计 auth、transport、config 和 formatter 是否在 factory/Mount 阶段创建只供 RunE 使用的对象。
- [ ] 每个后续 lazy 变化必须有首次执行与并发测试，不能把失败延迟到不可诊断的位置。

交付标准：help/version 构树不创建网络连接、读取 token 或编译业务响应规则。

### P3：低分配通用 builder

- [x] string-slice flag 对空默认值不再为每个 flag 创建 encoding/csv 的 4 KiB writer。
- [x] 保持 pflag 的 `stringSlice` 类型、CSV quoting、重复 flag append、SliceValue Append/Replace/GetSlice 和 DefValue 行为。
- [x] shortcut builder 在注册表到 Cobra mount 之间传指针，避免复制大型 Shortcut declaration；执行时再取得 invocation-local value。
- [x] helper registry 只保留 name + factory，并在构建时验证实际 command name，避免另建 route map。
- [ ] 根据新 profile 逐项处理 annotations、result-schema normalization 和 pflag map；单项收益低于噪声或需要缓存第二份真相时不做。

交付标准：所有产品仍走相同 generic builder；不为 root help 特制 leaf 或简化 flag。

### P4：向紧凑 service catalog 收敛

先在框架处理闭包、参数适配和规范化的重复分配。代表产品用于验证执行行为；只有 profile 证明剩余成本来自产品自身声明，且框架不能统一消除时，才启动下述产品迁移。

- [x] 统计手写 helper/shortcut builder 中重复的 Cobra 对象、flag spec 和 annotation 形状；当前 Linux root help RSS 47.68 MiB，Lark 42.85 MiB，差 11.3%。
- [ ] 后续迁移一个代表产品到只读 typed service descriptor + 通用 builder，要求 descriptor 直接成为 Cobra/ContractFinal 的共同构造输入，禁止运行时再合并两套参数声明。
- [ ] 代表产品必须同时降低 `B/op`、allocs/op 和 Linux wait4 RSS，并保持完整树节点数、help/Schema digest、Safety 与 handler 行为。
- [ ] 迁移收益可复现后再按产品分批替换手写装配，继续降低完整树固定成本。
- [ ] catalog 只负责构造；Schema、Safety 和 handler 仍使用现有权威类型，不引入生成的第二份命令真相。

P4 是后续专项，不阻挡 #1296 Ready。clean-head Linux root help 为 47.68/49.88 MiB，低于 RFC 的 50/55 MiB 产品预算且相对 main 降低；Lark 约 905 个节点的 42.85 MiB 仍作为压缩方向参考。实现仍必须是一棵完整 runtime tree；不能用 root-help projection、lazy leaf tree 或 argv 路由替代对象压缩。

### P5：两平台证据与发布

- [x] Darwin/arm64、Linux/amd64 使用 Go 1.25.9 跑完整测试、受影响 race、generate/drift/schema gates。
- [ ] 每个平台对专项父提交与 candidate 各跑完整树 microbenchmark。
- [x] 端到端随机交错测 root help/version/Schema/dry-run/mock/config，记录 wall、CPU、RSS 和输出 digest。
- [x] Lark 1.0.85 同机 root help/完整 Build 作为诊断，写清 commit、node count 和入口。
- [x] 将 clean-head artifact 和结论回填 RFC 附件；保持 Draft 直到 Linux RSS 与发布门禁通过。

## 3. 当前本地微基准

Apple M3 Pro、Darwin/arm64、同一 Go 1.26.1、本地每组 10 次，取三轮中位数：

| 完整 `NewRootCommand` | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| 专项父提交 `c0131d35` | 13,503,654 | 16,431,941 | 164,227 |
| implementation commit `34ce7493` | 13,544,096 | 12,681,568 | 148,166 |
| 变化 | **+0.3%** | **-22.8%** | **-9.8%** |

本地构树门槛已满足。该数据只证明完整树构造；正式结论必须使用 clean commit 和 Go 1.25.9 CI。macOS 本机的进程 wall time 受安全扫描干扰，不用于端到端 gate。

### 3.1 框架执行闭包压缩（2026-09-07，本地）

父提交 `b56ee380a` 与本次 `corecmd.New` 修改各编译独立 app 测试二进制，在 Apple M3 Pro / Darwin arm64 / Go 1.26.1 上交错运行六轮，每轮每个版本构树 100 次，奇偶轮交换先后顺序。下面是六轮中位数；测量时没有同时运行本次测试任务。

| 完整 `NewRootCommand` | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| 父提交 | 14,009,676 | 12,678,155.5 | 148,132.5 |
| 私有 `executionSpec` | 13,669,121 | 12,249,058 | 148,131 |
| 变化 | 噪声内 | -3.4% | 基本不变 |

首轮曾记录 -2.4% 耗时，但 3.2 的安静复测显示父提交 ns/op 本身就横跨 13.67–16.11 ms，该差值落在测量噪声内，不构成加速证据。只有 B/op 与 allocs/op 是确定性的，可作为结论。

编译器 `go build -gcflags='-m=2' ./internal/corecmd` 的逃逸分析显示，被执行闭包保留的结构从 848 字节降到 184 字节。收益来自缩小已有堆对象；产品无需迁移，四种派发方式继续共用原 preflight、Safety 与结果发布流水线。执行结构不包含 help/Contract，也没有新增注册表。

复测入口（两个版本分别 `go test -c -o <binary> ./internal/app`）：

```sh
DWS_PACKAGE_VERSION=0.0.0-test <binary> -test.run '^$' \
  -test.bench '^BenchmarkNewRootCommand$' -test.benchtime=100x \
  -test.count=1 -test.benchmem
```

验证通过：`corecmd/...` 单测与 race；Shortcut 适配及 builtin 整包 race；helpers 的 Leaf/FromLeaf/DeclareLeaf/ContractRuntime/ContractRunE/Contract confirmation 定向 race；app 完整树、help、Schema 参数一致性与废弃树回收检查；generated drift 与 Schema catalog policy（31 产品、1370 工具）。helpers 整包 race 中的 19 产品参数组合遍历在约 470 秒时主动中止，未计为通过。policy 临时生成工具在仓库临时路径遭 SIGKILL，使用独立 `DWS_POLICY_TMPDIR` 后两个门禁正常完成。

这组数据仅证明框架构树分配下降与本地耗时观测。尚无本次修改的 Linux wait4 RSS、双平台 native CI 或端到端延迟证据；不得复用上一轮 PR head 的绿灯宣称本次修改已通过发布验收。

按节点粗算，当前 DWS 约 6.95 KiB 和 81 alloc/command；Lark 约 11.4 KiB 和 93.5 alloc/command。DWS 的总量仍更高，主要因为公开节点约为 Lark 两倍。这个比例只能帮助归因，不能证明两边 command 复杂度相同。

### 3.2 命令路径分配收敛（2026-09-07，本地）

两处都只改变框架内部的临时分配，不改变任何对外输出：

- `annotatePreferredShortcutOwners` 的深度优先遍历原先为每个节点复制一份父路径切片。只有 `strings.Join` 的结果逃逸，遍历是同步且深度优先的，因此子节点可以直接复用父切片的剩余容量，兄弟节点之间不会互相保留数据。
- `schemaruntime.NormalizeCLIPath` 原先对每个节点都做 `strings.Fields` + `strings.Join`。Cobra 与生成声明已经使用单个 ASCII 空格，规范化结果就是输入的子串，因此直接返回子串即可。非 ASCII 字节、单空格以外的空白和重复空格仍走原 `Fields` 路径，语义不变。

同口径交错复测（父提交 `b56ee380a` 对当前工作树，六轮、每轮构树 100 次、奇偶轮换序、安静机器）：

| 完整 `NewRootCommand` | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| 父提交 | 15,392,720 | 12,677,545 | 148,129.5 |
| 3.1 + 3.2 累计 | 15,394,490 | 12,048,942 | 142,687 |
| 变化 | 噪声内 | -4.96% | -3.67% |

3.2 单独贡献约 -200 KB/op（-1.6%）与 -5,444 allocs/op；其余来自 3.1 的执行闭包压缩。两次父提交测量分别落在 13.67–16.11 ms 和 14.01 ms，说明本地构树耗时的轮间方差大于这几项改动的总收益，因此本节只声明分配下降，不声明加速。

验证通过：`internal/cli/...` 全量单测（含 `homology`、`schemaruntime`）；app 的 root/leaf help、Shortcut 归属、进程始终构造完整树、Schema 参数与 help flag 一致性、反向完整性检查；`NormalizeCLIPath` 新增对 `strings.Fields` 参考实现的等价表驱动测试，覆盖空串、`dws` 前缀、重复空格、制表符、换行、NBSP、非 ASCII 命令名与前后空白。

尚未证明：本次改动的 Linux wait4 RSS、双平台 native CI 与端到端延迟。

### 3.3 已测量并回退：Result Schema 按需解码（2026-09-07）

`contract.NormalizeResultSpec` 占构树 alloc_space 约 17%（196.82 MB / 1.16 GB），是单点最大热点。尝试把 `validateResultSchemaDescriptions` 的整树 `map[string]any` 通用解码换成在 `json.RawMessage` 上按需解码，只解析 `properties` / `items` / `allOf|anyOf|oneOf` / `description`，并复用 `canonicalJSONObject` 已解析的顶层对象以省掉第二次解析。

同口径交错复测结果是回归，已回退：

| 完整 `NewRootCommand` | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| 父提交 | 15,107,688 | 12,677,545 | 148,129.5 |
| 按需解码 | 19,451,605 | 13,227,972 | 151,994.5 |
| 变化 | 明显变慢 | +4.3% | +2.6% |

原因是假设错了：一次完整的单遍 `json.Unmarshal` 比 N 次小的按需解码更便宜。每个 `json.Unmarshal` 调用都要建立自己的 decoder 状态，schema 属性越多、小调用越多，固定开销累积反而超过省下的通用解码。`canonicalJSONObject` 保留嵌套数字原文与字段顺序的约束也不是瓶颈。

等价性本身是成立的：同一张 24 用例表（含 JSON null、非字符串 description、类型不匹配、嵌套 items 与组合分支）在父提交与新实现的生产路径 `NormalizeResultSpec` 上两侧全部通过。唯一差异在已删除的独立入口 `validateResultSchemaDescriptions` 的根级 `null`——旧实现接受它，因为 `json.Unmarshal` 解码 null 到 `map[string]any` 不报错；生产路径上 `canonicalJSONObject` 先拒绝 null，因此该差异不可观测。回退后该独立入口恢复，测试改为直接驱动 `NormalizeResultSpec`，覆盖反而更贴近真实调用链。

结论：不要再以「按需解码替代单遍通用解码」的方式优化这条路径。若要继续压缩这 17%，方向应是减少解析次数本身（例如复用一次解析结果同时服务规范化与校验），而不是把一次解析拆成多次。

### 3.4 darwin-arm64 本机对 Lark 的五维实测（2026-09-07）

在本机用 CI 完全相同的口径复现了 `measure-cli-five-dimensions.py`：候选由 `build-schema-cache-candidate.py` 从本次框架优化提交构建（go1.25.9、darwin-arm64、工作树干净），基线用 CI 指定的固定提交 `6f71222b`（go1.25.9），竞品固定 `@larksuite/cli@1.0.85` 与 `@googleworkspace/cli@0.22.5`，venv + psutil 7.2.2，harness 自检 7 项通过。报告 `complete=true`、0 失败、44 用例 × 30 样本 × 延迟与内存两阶段全部齐备。

wall p50 / p95（ms），DWS 对 Lark：

| 工作负载 | DWS p50/p95 | Lark p50/p95 | p50 比值 |
|---|---|---|---|
| help | 353.09 / 650.78 | 412.95 / 765.52 | 0.855x |
| version | 335.70 / 588.36 | 376.89 / 656.41 | 0.891x |
| leaf-help | 357.14 / 711.56 | 418.89 / 818.26 | 0.853x |
| schema | 413.37 / 1346.92 | 454.18 / 864.57 | 0.910x |
| dry-run | 334.93 / 612.33 | 387.14 / 2783.71 | 0.865x |

五个负载的 wall p50 全部 DWS 更快。

两条必须随数据一起读的限制：

- 绝对值约 350 ms 远高于 CI 记录的 37–44 ms，因为这台 Mac 的进程 wall time 受安全扫描干扰（§3 开头已说明该干扰，不用于端到端 gate）。可用证据是**同机比值**，不是绝对值。
- CPU `user_ms` p50 上 DWS 在 leaf-help（45.87 vs 43.26）与 schema（64.57 vs 47.63）反而高于 Lark。按延迟口径这一项通过；若把「快」定义为 CPU 时间，这两个负载仍需优化。schema 的差距与 §3.3 未压缩的那 17% Result Schema 规范化成本方向一致。

本次数据只覆盖 darwin-arm64。linux-amd64 必须由 CI native job 给出，本机无法复现。

### 3.5 CI 双平台对 Lark 的实测差距与根因归因（2026-09-07，head `9e52edcc`）

CI native feedback run `34097698630` 全绿（Schema policy、两平台 Full Go suite、identity comparison 均 success），两平台 `five-dimensions-report.json` 均 `complete=true`、0 失败。wall p50（ms）：

| 负载 | linux DWS | linux Lark | 判定 | darwin DWS | darwin Lark | 判定 |
|---|---|---|---|---|---|---|
| help | 43.98 | 47.26 | 快 0.931x | 44.39 | 49.27 | 快 0.901x |
| version | 43.37 | 46.00 | 快 0.943x | 41.27 | 50.38 | 快 0.819x |
| leaf-help | 50.58 | 46.29 | **慢 1.093x** | 48.35 | 50.04 | 快 0.966x |
| schema | 57.78 | 47.07 | **慢 1.228x** | 52.35 | 50.15 | **慢 1.044x** |
| dry-run | 43.49 | 46.88 | 快 0.928x | 46.17 | 49.35 | 快 0.936x |

CI 门禁绿不代表「比 Lark 快」：Lark 对比是诊断项，不是 release gate。help / version / dry-run 三个负载在两平台都快于 Lark，说明 §3.1–§3.2 的构树分配优化确实传导到了端到端延迟；leaf-help（仅 Linux）与 schema（两平台）仍然慢。

需要关闭的差距：linux schema 10.71 ms、linux leaf-help 4.29 ms、darwin schema 2.20 ms。

根因已确认，两个负载同源——都要解析 Schema Meta，root help 不需要：

| 平台 | file-hit 阶段 | 中位成本 | 对应差距 |
|---|---|---|---|
| linux | `selected-open-locator-authenticate-decode-index` | 6.517 ms | leaf-help 差距 6.60 ms（50.58−43.98），几乎完全吻合 |
| linux | `meta-open-authenticate-decode-lookup` | 4.536 ms | schema 差距 13.80 ms 的主要部分 |
| darwin | `selected-...-decode-index` | 4.943 ms | leaf-help 差距 3.96 ms |
| darwin | `meta-open-authenticate-decode-lookup` | 3.719 ms | schema 差距 7.96 ms |

旁证：`TestResolveMetaAndLeafHelpReuseAssembledMetaCache` 证明 leaf `--help` 复用同一份 assembled Meta。本机构建（无 pinned cache identity）实测 `--help` 342 ms 而 leaf `--help` 与 `schema` 均约 1.42 s，与 CI 的 `dws-live/schema` 1369.70 ms 一致，说明 cache 未命中时两者都付 live assembly；CI 测的是 warm cache 情形。

读取路径的分配构成（本机 `-memprofile`，按只出现在读取路径的函数估算约 2.38 MB/op，占 3.48 MB/op 的大部分）：`readMeta` 约 823 KB/op 的文件读取、protobuf 字符串 UTF8 校验约 950 KB/op、`validateAndConvertMeta` 下游的 `ToolSpec.Validate` / `ToolSpec.normalized` / `ProductSpec.normalized`。单叶查询目前仍解码并规范化全部 1370 个工具。

因此关闭差距需要把 Meta 读取削减 65%（linux leaf-help）到 78%（linux schema）。

改动范围已数清。`CommandMetaByPath` 的生产消费方：`command_meta.go:161,170`（`ResolveMeta` 单路径查找）、`cache_codec.go:395`（分片校验 `commandMetaSubsetEqual`）、`cache_codec.go:542`（`validMetaAliasExpansion`）、`cache_codec.go:547-557`（遍历全部条目做 product locator 一致性交叉校验并构建 `commandCountByProduct`）、`schema_cache_delivery.go:603`（与 `BuildCommandMetaLookup(registry)` 做 `DeepEqual` 交付等价校验）。后四项需要全量数据，因此「单叶只解码一个工具」不是纯性能改动。

曾设想一条「只解码 Identity」的路径，理由是 `cache_codec.go:542` 与 `547-557` 看起来只读 `Identity`。**这个前提是错的，已否决**：`meta.go:163` 的 `validMetaAliasExpansion` 对别名行调用 `equalCommandMeta(primary, meta)`，比较的是完整 `CommandMeta`，用于验证别名行与主行的 `Safety` 与 `Selection` 逐字段一致。因此只解码 Identity 会削弱这项校验，不是纯性能改动。相应的 `commandIdentityFromProto` 已回退，未留在仓库里。

`commandMetaFromProto`（`cache_conversion.go:75`）每条拷贝 6 个字符串列表，其中 5 个属于 `Selection`（UseWhen / AvoidWhen / Prerequisites / Tips / Examples），只有 Aliases 属于 Identity；`commandMetaProtoLists` 只返回引用 proto 字段的切片数组，不产生堆分配，所以每条的堆分配就是那 6 次 `make` + `copy`。`CommandEntries.Items` 与 `Locators.Items` 都已由 `cache_codec.go:506-507` 与 `520-521` 强制按 `LookupPath` 严格递增排序，二分查找可行。

因此真正的决策点是信任模型：`equalCommandMeta` 的别名/主行一致性校验是**写入方正确性**检查，却在每次读取时重跑。cache 内容已由哈希保证完整性，哈希并不能发现写入方产生了不一致的别名行——这项检查抓的是写入方 bug，不是损坏。要压缩读取成本，必须决定把它移到写入时（写入方自校验），读取时不再重验；否则每次读取都得付这笔全量解码成本。这个决定不该由性能优化单方面做出。

但 leaf-help 存在一条**不需要上述决策**的路径。生产 leaf help 走 `help_affordance.go:29` 的 `RenderHelpAffordances`，它通过 `ResolveMeta(cliPath)` 取三样东西：`meta.Selection`（渲染 selection guidance）、`meta.Safety`（渲染 Safety 行）、`meta.Identity.ProductID`（服务 Help 页引用）。这三样 `ContractFinal` 全都携带——`AttachContract` 在构树时已把 Identity / Safety / Selection 挂到 Cobra 命令上，`CollectIdentitySpecs` 正是从 `ContractFinal.Identity` 读取的。

因此 `RenderHelpAffordances` 可以直接从已挂载的 `ContractFinal` 读取，完全不触发 Schema cache Meta 读取。这条路不改 cache 读取路径，因此不涉及信任模型决策；而且 `ContractFinal` 是上游声明权威、Schema Meta 是它的派生物，读的是更权威的来源而非更弱的来源。预期收益是省掉 leaf-help 的整笔 Meta 读取（linux 约 4.5 ms），足以关闭 linux leaf-help 的 4.29 ms 差距。

落地前必须验证的前提：Schema Meta 的 `CommandMeta` 由 `ContractFinal` 派生，但派生过程可能对 `Selection` 施加了 `Normalized()` 等规范化。切换读取来源前必须证明渲染输出逐字节一致，否则 help 文本会漂移。相关门禁是 help stdout 的 SHA-256 一致性检查。

## 4. 验收矩阵

| 场景 | 树 | 必须保持的行为 |
|---|---|---|
| root help / version | 完整 runtime tree | 同一 public flags、服务/utility 列表、locale、startup diagnostics |
| Schema hit/miss/repair | 完整 runtime tree | Cobra parsing、shortcut/plugin diagnostics、wire parity、identity fail-closed |
| leaf help / dry-run / mock | 完整 runtime tree | aliases、required/groups、Safety、无多余 RPC、统一输出 |
| config / event utility | 完整 runtime tree | shared caller、profile、PreParse 和 cleanup 不缺失 |
| completion | 完整 runtime tree | 候选、描述、directive、alias 与 shell script contract |
| public/embedded constructor | 完整 runtime tree | 不读取宿主 argv 决定 command surface |

阻断条件：

- 出现另一棵公开 tree、help projection、Schema 前置执行或 argv factory selection；
- help bytes、Schema wire、flags、aliases、validation、Safety、错误分类或 cleanup 改变；
- 完整树相对父提交未达到 `B/op -20%`、`allocs/op -8%`，或 ns/op 超出允许回归；
- root help RSS 相对 main 回退，或任一平台超过 RFC 的 50/55 MiB p50/p95 预算；
- clean-head 两平台测试、race 或发布 proof 不完整。

## 5. 暂不绑定

- edition 产品面裁剪；
- 追平 GWS 3～4 ms；
- daemon/常驻进程；
- 无证据的 Prepare/auth/config 去重；
- 为不存在的 completion callback 泄漏增加基础设施。
