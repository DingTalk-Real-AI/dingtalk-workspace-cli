# RFC：编译期 Schema 交付与命令树 codegen

状态：草案，待评审
日期：2026-09-07
关联：`docs/rfc-schema-runtime-cache.md`、`docs/rfc-schema-runtime-cache-performance.md`、`docs/plan-command-framework-performance.md` §3.3–§3.6

## 1. 问题

CI run `34097698630`（head `9e52edcc`，两平台 `complete=true`、0 失败）实测 DWS 对固定 Lark 1.0.85 的 wall p50：

| 负载 | linux DWS / Lark | darwin DWS / Lark |
|---|---|---|
| help | 43.98 / 47.26 | 44.39 / 49.27 |
| version | 43.37 / 46.00 | 41.27 / 50.38 |
| leaf-help | **50.58 / 46.29** | 48.35 / 50.04 |
| schema | **57.78 / 47.07** | **52.35 / 50.15** |
| dry-run | 43.49 / 46.88 | 46.17 / 49.35 |

help / version / dry-run 两平台都快于 Lark，但 leaf-help（linux）与 schema（两平台）慢。需关闭的差距：linux schema 10.71 ms、linux leaf-help 4.29 ms、darwin schema 2.20 ms。

## 2. 为什么增量优化不够

差距的构成已量化到阶段级（见 plan §3.5）：

- 完整树构造基线约 44 ms（linux），两平台的 help 都已快于 Lark，说明这一层不是差距来源
- leaf-help 与 schema 的额外成本来自 Schema Meta 读取，与 `BenchmarkRealSchemaFileHit` 的两个阶段耗时精确吻合：linux `selected-...-decode-index` 6.517 ms ≈ leaf-help 差距 6.60 ms；linux `meta-...-decode-lookup` 4.536 ms 是 schema 差距 13.80 ms 的主要部分

关闭 linux leaf-help 需削减 4.29 ms，而整笔 Meta 读取只有 4.536 ms——**需要削减约 95%**。而 protobuf 的 unmarshal 本身就会解码全部字符串字段（约 950 KB/op），只跳过 Go 侧的 `CommandMeta` 转换最多省约 20% 分配。要省 95%，必须让单叶查询根本不解码 Selection 字符串，这需要改 DTO；且即便如此也只是把一条链路削薄，基线仍在。

根因是两条现行架构决定：

1. **「声明即 Catalog」运行时装配**。AGENTS.md 明确无 `cmd_schema_catalog` 的 `//go:generate` 交付步骤，生产必须走 `RegisterSchemaSourceRoot → ResolveSchemaBuild`。装配昂贵 → 需要 verified cache → cache 需要哈希校验 + protobuf 解码 + 全量 `CommandMeta` 转换。**Meta 读取链路的存在本身就是这条决定的代价。**
2. **每次进程调用构造完整 Cobra 树**。AGENTS.md 禁止 argv-selected product trees、pre-Cobra Schema execution、separate root-help projection。1825 个节点每次全建。

Lark 的 `schema`（47.07 ms）≈ 它的 `help`（47.26 ms），额外成本接近零：命令面约一半（905 vs 1370），且读取时无逐次验证负担。

## 3. 方案

### A. 编译期交付不可变 Schema 目录（主方案）

把 Schema 目录在构建期生成为不可变、可直接寻址的结构并编译进二进制，取代运行时装配 + 磁盘 protobuf cache。

- 消灭 cache 文件、`DTOVersion` 判定、哈希校验、protobuf 解码与全量 `CommandMeta` 转换。读取退化为结构体访问，Meta 读取的 4.5–11 ms 趋近于零
- verification 从「每次读取时重验」变为「编译期属性 + 生成物漂移门禁」。现有 `check-generated-drift.sh` 的模式可直接复用
- 保留 `dws schema -f json` 的线格式与 `--compact` 投影语义不变；`ResolveMeta` 的对外契约不变，只换数据来源
- 保留 declare-or-annotate、provenance 优先级与 homology 门禁——它们仍在装配期执行，只是装配发生在构建期而非运行期

预期收益：直接关闭全部三个差距，因为 Meta 读取整层消失。

### B. 命令树构建 codegen（配套）

当前每次构树分配 12.05 MB（已优化后），来自运行时解释声明。改为构建期生成直接构造 Cobra 命令的 Go 代码，可显著降低分配与 CPU。压的是 44 ms 基线，对 help / version / dry-run 也有收益。

### C. 索引化导航（可选）

用预编译的导航索引保证 help 完整性，而不必每次构造全部节点。收益最大但改动最深，建议作为独立后续 RFC。

## 4. 与现行契约的冲突

A 与 C 直接违反 AGENTS.md 的明文规定，必须先修订契约：

| 现行规定 | 冲突 | 处理 |
|---|---|---|
| 无 `cmd_schema_catalog` `//go:generate` 交付步骤 | A 需要生成步骤 | 修订为「生成物是交付输入，但仍由声明派生，漂移门禁保证一致」 |
| Meta 与分片是可丢弃的传输派生物 | A 下不再有磁盘 cache | 改为编译期常量；miss 语义消失 |
| 禁止 separate root-help projection | C 需要索引 | 若采纳 C 则修订；A 与 B 不涉及 |
| Cache enablement requires final-artifact evidence | A 下无 cache | 改为生成物证据 |

关键不变量必须保留：声明仍是唯一权威（generated 目录是声明的派生物，不是新的 authoring input）；`dws schema` 线格式不变；每个 Schema 工具仍可解析到可执行 Cobra 命令，且每个公开可执行叶子仍要么进 Schema 要么有精确的 reviewed exclusion。

## 5. 迁移路径

1. 先落地 B（命令树 codegen）——不违反现行契约，独立可验证，压基线
2. 再落地 A——需先修订 AGENTS.md 与 `rfc-schema-runtime-cache.md`，并新增生成物漂移门禁
3. C 作为独立 RFC 评估

每一步都必须重跑：`BenchmarkRealSchemaFileHit`（或其后继基准）、`check-generated-drift.sh`、`check-schema-catalog.sh`、两平台 CI 五维测量，并确认 help stdout 的 SHA-256 一致性门禁不变。

## 6. 验收

目标是两平台全部五个负载的 wall p50 都低于固定 Lark 1.0.85。当前 3/5 达标，A 落地后预期 5/5。

不通过减少产品面、增加第二 runtime 或 daemon 达成——这条约束继续保留。
