# DWS 工具检索排序实测报告

> 独立审计勘误（2026-08-12）：本报告的 Intent query 与索引中的
> `agent_summary` 来自同一批 selection 作者，属于内部 proxy set，不是独立
> test qrels；case-level bootstrap 支持 Hybrid 在当前微平均样本上的增益，
> 但 product-cluster bootstrap 区间约为 -0.40～+7.03 pp，不能表述为跨产品
> 稳定。文中的 “BM25F” 实际实现是各字段独立 BM25 后加权求和，正式名称
> 应为“字段 BM25 分数集成”。最终选型和结论强度以
> [`rfc-tool-search-progressive-discovery.md`](rfc-tool-search-progressive-discovery.md)
> 为准。

> 评测日期：2026-08-12；主线合并后复测：2026-08-13
>
> 分支：`test/tool-search-ranking-eval`
>
> 当前 Catalog：1,098 个工具，`source_hash=sha256:02c633c075f4af915ea097d383bdcbc25fe549c172e2d3336152e30cf2906e80`，`surface_hash=sha256:7f9839f1ff428b52f1d032d2fa6493815c3755650b7d9fa653839e7d589482e5`
>
> 评测代码：[`scripts/dev/eval_tool_search_ranking.py`](../scripts/dev/eval_tool_search_ranking.py)
>
> 2026-08-14 最新审计与优化路线：[`tool-search-optimization-plan-20260814.md`](tool-search-optimization-plan-20260814.md)

## 1. 结论

### 1.1 主线合并后的当前 Go 结果（规范现状）

当前结果由 `cmd_tool_search_comparison` 直接消费 Go 运行时声明装配结果，不读取仓库中的 Catalog JSON。评测/CI 如需检查分片，只能先运行 `cmd_schema_catalog`，在 `.worktrees/policy-tmp/tool-search-schema-catalog` 动态生成；该目录被忽略且不是生产输入。

| 指标 | `fielded_bm25_ensemble` 当前默认 | `fielded_bm25_action_v1` shadow |
|---|---:|---:|
| Intent cases | 1,123（另有 10 条超预算并显式排除） | 1,123 |
| R@1 / R@5 / MRR@5 | **65.27% / 88.16% / 0.7441** | 62.51% / 84.24% / 0.7131 |
| 纯中文（402）R@1 / R@5 | **51.74% / 83.83%** | 50.00% / 79.85% |
| 中文混 ASCII（721）R@1 / R@5 | **72.82% / 90.57%** | 69.49% / 86.69% |
| 整句 workflow Complete@5 / Recall@5 | 40% / 61.67% | 50% / **76.67%** |
| reviewed 拆解 Complete@5 / Recall@5 | 80% / 90% | **90% / 95%** |
| Forbidden@1 / @5（越低越好） | **0% / 0.0719%**（1/1,390） | **0% / 0.0719%**（1/1,390） |

身份与完整性门禁：1,098 个 canonical、1,098 个 primary CLI、19 个 reviewed alias、1,098 个 NFKC identity 及 1,098 个 `exact_filtered` 全部 100%；5,801 个响应的 Catalog 绑定失败、unknown candidate、ineligible candidate、response budget violation 均为 0。上下文方面，平均 Search + gold Inspect 为 4,507.03 bytes，相对 17,876,084 bytes 的 compact 全量 Schema 减少 **99.9748%**，相对假设 oracle 已选对产品的理想导航仍减少 **96.3169%**。两条评测路径都直接 Inspect gold leaf，即使 Search miss 也不计额外尝试，因此是容量上界，不是实际 Agent 成本。

**2026-08-13 排序加固后的变化**（相对上一版基线 86.64%/37.55%）：中文分词改为 unigram+bigram 并存、结构化词表补英文同义词、rerank 门从"任意 ASCII 词禁用"收窄为"仅技术标识符禁用"、新增跨算法的 avoid_when 软降权层、删除 OA task_id 专属乘子。默认 ensemble 全指标提升（R@5 +1.52pp，拆解 workflow Complete@5 50%→80%）；Forbidden@1 清零、Forbidden@5 降至 1/1,390 来自 avoid_when 层对负向 proxy（query 即 avoid_when 原句）的惩罚——该口径与匹配机制对口，真实 query 的收益取决于是否包含 avoid_when 短语，应视为安全下界而非日常改善。action_v1 shadow 同源 R@5 由 86.38% 降至 84.24%（OA 乘子删除 + 混合 query 乘子介入的代价），仅在拆解 workflow（90%/95%）上保留优势。

这些数据说明当前实现达到了“显著缩小上下文、身份不退化、中文自然语言 Top-5 约 88%”的工程目标，但不等价于线上任务成功率。独立 qrels、英文 ≥100、workflow ≥80、真实同模型 Agent A/B 仍为空，且封存门禁已按仓库决策移除——独立评测决策需人工执行 `make generate-tool-search-comparison` 并比对；不能据此把 action 重排提升为默认。

### 1.2 合并前算法研究记录（2026-08-12，历史基线）

以下 572 工具 / 602 intent 的结果用于记录算法选型过程，已被主线 1,098 工具 Catalog 的 Go 诊断结果取代，不能作为当前发布数值。其价值是说明为何不锁死 BM25、为何不内嵌 Dense，以及为什么要独立 qrels。

在原报告之后继续补了字段投影消融、更多轻量算法、公平中文切片、纯词法 RRF、Dense depth/k sweep、单查询延迟、Go 冷启动成本和跨进程确定性。新增结果进一步收紧选型：

1. **轻量默认仍不能锁定。** 同一 `proxy_v1` 投影下，TF-IDF R@5 为 84.55%，BM25 为 83.06%；TF-IDF 相对 BM25 的 +1.50 pp case bootstrap CI 仍跨 0。BM25L、BM25+、weighted Jaccard 均没有胜出。
2. **两路同质词法 RRF 不值得。** TF-IDF+BM25 RRF R@5 为 83.55%，低于 TF-IDF 单路；相对 BM25 仅 +0.50 pp，product-cluster macro 差值为 -4.08 pp 且区间跨 0。
3. **参数描述是 shadow 字段，不是已证明噪声。** 在相同 tokenizer/ranker 下加入参数描述，TF-IDF R@5 84.55%→85.05%，BM25 83.06%→83.72%，两个 paired CI 均跨 0；10 条工作流 Complete@5 分别 30%→40%、20%→30%，样本太小不能定默认。参数类型/alias 没有带来额外 R@5 增益。
4. **`use_when` 是明确的数据泄漏上界。** 历史实验显式开启 `use_when` 投影，而 602 条 proxy query 正来自 `use_when`；含该字段后 BM25/TF-IDF R@5 都达到 100%。当前 Go 默认已经关闭该字段。这不是生产效果，而是“拿答案原句搜答案”的上界；只有独立 qrels 可以重新评估是否值得开启。
5. **当前数据无法执行英文门禁。** 602 条 intent 中纯中文 283 条、中文混 ASCII 319 条、英文 0 条；RFC 要求的英文 ≥100 条只能由独立数据集补齐，不能从当前 proxy 声称通过。
6. **TF-IDF+Dense 已否决为产品候选。** 默认 `depth=100,k=60` 时 R@5=87.87%，dev sweep 最好 `depth=100,k=10` 为 88.70%；但默认候选相对 BM25 的 product-cluster macro 差值为 -2.98 pp、CI 跨 0，参数又在同一 proxy 上挑选，不足以覆盖 Provider/RRF 的实现与运维复杂度。
7. **历史 572 工具 Spike 中，冷启动而不是 warm search 是 subprocess 主要成本。** Go warm query 为约 0.59～0.63 ms、约 116 KB/op；当时的 engine build 为约 51～53 ms、31 MB/op。当前声明装配版必须重新测“进程启动 + runtime Catalog assembly + index build + Search + JSON”，不能沿用历史 warm p95。
8. **修复了一个真实的跨进程确定性缺陷。** BM25 原先遍历 Go map 累加浮点分数；现已改为每请求排序 query term 后固定顺序累加，并以独立子进程逐字节 golden 回归。

续测后的产品顺序是：`Exact → hard filter → TF-IDF/BM25 可替换单路 control`。Alpha 前不启用双词法 RRF，不默认索引 `use_when`；Dense/RRF 仅保留为历史评测对照，Provider/fallback 已从实现和协议删除。

这轮实测曾验证“Exact + BM25 + Dense RRF”的研究上限，但后续产品决策因独立证据不足和链路复杂度否决 Provider/RRF。以下数字只解释为什么做过实验，不再代表当前架构方向：

1. **Dense 单独没有优于 BM25**：Dense Recall@5 为 82.06%，普通 BM25 为 83.06%。不能因为语义模型更先进就替换词法检索。
2. **Hybrid RRF 在当前微平均 proxy case 上有配对增益**：字段 BM25 + Dense 的 Recall@5 为 85.88%，比字段 BM25 高 3.65 个百分点；case-level 配对 bootstrap 95% 区间为 +1.33～+6.31 个百分点。product-cluster 区间跨 0，因此不能扩展为跨产品稳定结论。
3. **字段权重还没有调好**：当前试设字段 BM25 分数集成权重的单意图 Recall@5 为 82.23%，反而略低于未分字段 BM25 的 83.06%；但它在 10 条工作流诊断集上完整覆盖 40%，高于普通 BM25 的 20%。字段权重必须在独立验证集上调，不能直接把建议数字写死。
4. **RRF 会破坏精确标识符**：不加保护时，字段 Hybrid 的 canonical Top-1 从 100% 降到 91.43%。融合前或融合后必须做 Exact Guard；加 Guard 后 canonical 和 CLI path 都恢复 100%，自然语言指标不变。
5. **单次整句检索仍不擅长多工具任务**：字段 Hybrid 的工作流 Comprehensiveness@5 只有 50%。人工正确拆解子任务后，同一检索器可达到 80%，说明瓶颈已经从“相关性”部分转移到“任务分解与集合选择”。
6. **`avoid_when` 必须成为门禁**：最佳自然语言 Hybrid 仍会把当前 query 明确不该使用的工具放入 Top-5，暴露率为 55.61%。相关性排序不能理解业务否定关系，不能替代 contradiction/policy gate。

因此，当前推荐继续验证的产品候选是：

```text
Hard Filter
  → Exact Guard
  → 可替换本地轻量词法召回
  → avoid_when / policy contradiction gate
  → 多动作 query 分解与集合补全
  → Top-5 ToolReference
```

## 2. 评测协议

### 2.1 数据集

| 数据集 | 数量 | Gold / 目标 | 用途 |
|---|---:|---|---|
| Intent | 602 | 每条人工 `use_when` 对应的 canonical tool | 自然语言工具召回 |
| Forbidden | 820 | 每条人工 `avoid_when` 所属工具为 forbidden | 错误暴露诊断，越低越好 |
| Canonical | 572 | canonical path 精确对应工具 | 精确身份回归 |
| CLI path | 572 | CLI path 精确对应工具 | 命令身份回归 |
| Workflow | 10 | 每条 query 有 2～3 个 required tools | Top-5 集合完整度诊断 |

工作流 fixture 位于 [`tool_search_workflows.json`](../scripts/testdata/tool_search_workflows.json)。它是小规模人工诊断集，不是正式发布门禁。

### 2.2 防止数据泄漏

Intent query 直接来自人工 `use_when`。如果索引也包含同一条 `use_when`，检索器只是在“拿原句搜原句”，结果会虚高。本轮索引明确排除：

- 全部 `use_when`；
- 全部 `avoid_when`；
- 全部 examples。

索引只使用当前生产 Catalog 中可以稳定获得的：

- canonical path、CLI path、name、product/group；
- `agent_summary`、title、display；
- description；
- 参数名、interface property 和 enum。

因此本报告衡量的是这些字段对未索引意图句的泛化能力，不是把人工答案直接放进索引后的理论上限。

### 2.3 被测方法

| 方法 | 实现 |
|---|---|
| Keyword overlap | 中文二元字串 + 英文/CLI 标识符 token；按 query/document 交集 IDF 求和 |
| BM25 unfielded | 全字段拼接；`k1=0.9, b=0.4` |
| Fielded BM25 score ensemble | identity=8、summary=5、description=2、parameters=2，各字段独立 BM25 后分数加权；不是严格 BM25F |
| Dense | `BAAI/bge-small-zh-v1.5`，FastEmbed 0.7.3 ONNX，归一化 cosine |
| Hybrid RRF | Sparse 和 Dense 各取 Top-100，等权 RRF，`k=60` |
| Exact Guard | query 精确等于 canonical 或 CLI path 时绕过 Hybrid 排序 |
| Manual decomposition upper bound | 人工把工作流拆成 2～3 个子意图，各自 Hybrid 检索后 round-robin 合并 Top-5 |

中文词法实现保留完整 canonical/CLI 标识符，同时拆 snake_case、kebab-case、dot path 和 camelCase；中文连续文本使用二元字串。这只是可复现基线，不代表最终 tokenizer 已定型。

### 2.4 运行环境

- macOS / Apple M3 Pro / 18 GiB；
- Python 3.9.6；
- uv 0.9.9；
- FastEmbed 0.7.3；
- Dense 模型下载后从本地 cache 加载。

下面的批处理耗时用于比较数量级，不是线上单 query p95。生产延迟仍需在最终 Go/服务实现上单独压测。

## 3. 总体结果

### 3.1 质量指标

| 方法 | Intent R@1 | Intent R@5 | MRR | Forbidden@5 ↓ | Canonical Top-1 | CLI Top-1 | Workflow Complete@5 | Required Recall@5 |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| Keyword overlap | 47.51% | 76.91% | 0.6017 | **44.27%** | 100.00% | 70.98% | 20% | 51.67% |
| BM25 unfielded | 56.64% | 83.06% | 0.6821 | 54.02% | 99.83% | 88.99% | 20% | 46.67% |
| Fielded BM25 score ensemble | 54.49% | 82.23% | 0.6690 | 55.00% | 100.00% | 100.00% | 40% | 61.67% |
| Dense | 54.15% | 82.06% | 0.6651 | 49.15% | 75.87% | 73.43% | 30% | 56.67% |
| Hybrid RRF, unfielded | **60.63%** | **86.05%** | **0.7230** | 56.46% | 91.08% | 86.01% | 30% | 61.67% |
| Hybrid RRF, fielded | 60.13% | 85.88% | 0.7204 | 55.61% | 91.43% | 90.21% | **50%** | **70.00%** |
| Hybrid fielded + Exact Guard | 60.13% | 85.88% | 0.7204 | 55.61% | **100.00%** | **100.00%** | **50%** | **70.00%** |

这里没有把 Forbidden@5 最低的 Keyword overlap 判成“最安全”。它的自然语言召回也最低。正确处理方式是相关性检索与负向门禁分层，而不是牺牲召回换取偶然较低的暴露率。

### 3.2 相对字段 BM25 的配对结果

以每条 Intent case 是否命中 Top-5 为配对样本，做 2,000 次固定随机种子的 bootstrap：

| 候选方法 | Recall@5 差值 | 95% bootstrap 区间 | 解释 |
|---|---:|---:|---|
| Keyword overlap | -5.32 pp | [-8.47, -2.33] pp | 明显退化 |
| BM25 unfielded | +0.83 pp | [-1.50, +3.16] pp | 无法证明有差异 |
| Dense | -0.17 pp | [-3.16, +2.99] pp | 无法证明有差异 |
| Hybrid fielded | +3.65 pp | **[+1.33, +6.31] pp** | 当前微平均 proxy case 有配对增益；不代表跨产品稳定 |
| Hybrid unfielded | +3.82 pp | **[+1.16, +6.48] pp** | 当前微平均 proxy case 有配对增益；不代表跨产品稳定 |

两种 Hybrid 的单意图差只有 0.17 pp，本轮不能据此判定 unfielded 更好。字段版本在小工作流集上更完整，但样本只有 10 条；下一步需要扩大 workflow qrels，再选择字段策略和权重。

### 3.3 构建与离线批处理耗时

| 方法 | 索引/模型初始化 | 全部 query 离线批处理 |
|---|---:|---:|
| Keyword overlap | 0.037 s | 0.66 s |
| BM25 unfielded | 0.039 s | 1.16 s |
| Fielded BM25 score ensemble | 0.040 s | 4.15 s |
| Dense | 22.19 s | 5.46 s |

Dense 初始化包含从本地 cache 加载 ONNX 模型并为 572 个工具生成向量。生产设计应离线构建和缓存工具向量；请求路径只编码 query。当前 Python BM25 是清晰优先的评测实现，没有使用倒排表优化，不能把 4.15 秒批处理外推为 Go 生产性能。

### 3.4 轻量算法公平对照

以下方法使用完全相同的 `proxy_v1` 字段投影、tokenizer、zero-score 过滤和 canonical stable tie-break：

| 方法 | R@1 | R@5 | MRR | Forbidden@5 ↓ | Python warmed p95 |
|---|---:|---:|---:|---:|---:|
| Keyword overlap | 47.51% | 76.91% | 60.17% | 44.27% | 0.57 ms |
| Weighted Jaccard | 44.85% | 74.58% | 58.67% | 39.63% | 11.48 ms |
| BM25L | 52.16% | 80.90% | 64.71% | 50.24% | 1.59 ms |
| BM25+ | 53.49% | 81.23% | 65.78% | 51.46% | 1.54 ms |
| BM25 | 56.64% | 83.06% | 68.21% | 54.02% | 1.51 ms |
| TF-IDF cosine | **57.14%** | **84.55%** | **69.56%** | 54.02% | 1.68 ms |
| TF-IDF + BM25 RRF | 58.97% | 83.55% | 70.27% | 54.63% | 未单测 |

这是 Python 全量扫描实现的相对成本，不是 Go SLA。Jaccard 的低 Forbidden 主要来自低召回。纯词法 RRF 提高 R@1/MRR、却降低 R@5，无法满足“第二路补回 lexical miss”的架构目的。

### 3.5 字段投影消融

| 累积投影 | TF-IDF R@5 | BM25 R@5 | TF-IDF Workflow Complete@5 | BM25 Workflow Complete@5 |
|---|---:|---:|---:|---:|
| identity | 13.95% | 11.79% | 0% | 0% |
| + summary | 78.24% | 76.58% | 50% | 30% |
| + tool description | 84.55% | 82.56% | 30% | 20% |
| + parameter name/enum（`proxy_v1`） | 84.55% | 83.06% | 30% | 20% |
| + parameter/interface description | **85.05%** | **83.72%** | 40% | 30% |
| + parameter type/alias | 85.05% | 83.72% | 40% | 30% |
| + `use_when` 泄漏上界 | 100% | 100% | 40% | 40% |

summary 是最大的单次增益；tool description 对单意图有明显贡献，但在 10 条 workflow 上出现退化，说明“单工具相关性”和“集合完整性”必须分开优化。参数描述相对 `proxy_v1` 的 TF-IDF/BM25 R@5 增益只有 +0.50/+0.66 pp，case 与 product-cluster CI 都跨 0。

### 3.6 中文与产品分布

`proxy_v1` 下：

| 方法 | 纯中文（283）R@5 | 中文混 ASCII（319）R@5 | 英文样本 |
|---|---:|---:|---:|
| BM25 | 81.98% | 84.01% | 0 |
| TF-IDF | **83.04%** | **85.89%** | 0 |
| TF-IDF + 参数描述 | 83.04% | **86.83%** | 0 |

TF-IDF 的 product R@5 从 `devdoc=0%`、`event=57.14%`、`dev=61.76%`、`mail=67.74%` 到多个小产品 100%，分布很不均衡。micro 平均不能替代 product/tool-family cluster 门禁。`devdoc=0%` 已核实只有 1 条 binary-gold case：gold 是 `devdoc.search_open_platform_docs_rag`，实际排第 7；同一 backing operation 还暴露为 `dev.search_open_platform_docs_rag`，两者 `interface_ref.product_id=devdoc`、`rpc_name=search_open_platform_docs` 相同。该 0% 首先是单样本 canonical/equivalent-entrypoint 标注问题，不是调全局权重的证据；独立 graded qrels 应把专用入口标 relevance=3、兼容入口标 relevance=2。

### 3.7 Dense depth / RRF 参数敏感性

| Sparse arm | 参数 | R@1 | R@5 | MRR | Workflow Complete@5 |
|---|---|---:|---:|---:|---:|
| TF-IDF + Dense | depth=20,k=10 | 61.79% | 87.87% | 73.41% | 40% |
| TF-IDF + Dense | depth=100,k=60 | 61.30% | 87.87% | 73.14% | 40% |
| TF-IDF + Dense | depth=100,k=10（dev best） | **61.96%** | **88.70%** | **73.76%** | 40% |
| BM25 + Dense | depth=100,k=20（dev best） | 60.63% | 87.87% | 72.47% | 30% |
| 字段 BM25 + Dense | depth=100,k=10（dev best） | 60.63% | 86.88% | 72.48% | **50%** |

depth 20 到 100 对 TF-IDF+Dense 的 R@5 最多相差 0.83 pp，但对不同 sparse arm、MRR 和 workflow 的影响不同。`CandidateLimit=20` 不是明显错误，却不能在独立 dev/test 前被写死；dev-best 参数不进入 test 反复挑选。

### 3.8 Go 冷启动与确定性

Apple M3 Pro、darwin/arm64、5 次 Go benchmark：

| 路径 | 结果 |
|---|---:|
| warm embedded Search | 0.59～0.63 ms/op，约 116 KB、1,052 allocs/op |
| embedded engine build | 51.36～53.12 ms/op，约 31.2 MB、265k allocs/op |

engine build 尚不包含外部 Host 启动进程、解析 stdout 和第二次 fusion 调用。公开 `schema search` 后必须补真实 release binary 的 cold/warm subprocess p50/p95/p99。当前修复后的跨进程测试会运行 3 个独立 test process，并比较完整 JSON bytes；这只证明排序确定性，不是延迟测试。

## 4. 多工具任务效果

### 4.1 整句检索

10 条人工工作流中：

- Keyword / 普通 BM25：完整覆盖 2/10；
- Dense：完整覆盖 3/10；
- 字段 BM25：完整覆盖 4/10；
- 字段 Hybrid：完整覆盖 5/10。

典型失败是“给群里发文件，并确认群成员是否已读”。字段 Hybrid 的 Top-5 是：

```text
chat.query_custom_user_roles
chat.query_msg_read_status
chat.add_group_member
chat.update_group_settings
chat.set_group_member_mute_list
```

它找到了“查询已读”，却没有保留 `chat.send_personal_message`。原因不是发送工具完全不相关，而是一个整句在全局排序中被多个“群成员/群设置”能力挤占。仅仅把 Top-5 调到 Top-10 会增加上下文和误调用面，不能从结构上解决问题。

### 4.2 正确拆解后的检索上限

把每条工作流人工拆成动作级 subquery，再用同一个“字段 Hybrid + Exact Guard”检索并 round-robin 合并：

- Comprehensiveness@5：从 50% 提升到 **80%**；
- required tool 平均 Recall@5：从 70% 提升到 **90%**；
- “发群文件并查已读”完整找回两个工具：

```text
chat.send_personal_message
chat.query_msg_read_status
chat.reply_personal_message
chat.unset_pin_message
chat.query_message_send_status
```

这组 80% 是**人工正确分解后的上限诊断**，不包括模型自动拆解错误，不能当作端到端任务成功率。它证明应该增加 Planner / Workflow Completer 的评测，而不是继续只调单 query ranker。

仍失败的两条是：

- “上传文件后授权”：`drive.add_permission` 被 `drive.permission_update` 等相邻能力挤出；
- “搜索文档后读取”：`doc.get_document_content` 被 create/list 类工具挤出。

这两类需要更清晰的动词/状态差异、`use_when` 正向重排和 `avoid_when` 冲突门禁；由于本轮为防泄漏没有索引 `use_when`，后续应在独立人工 query 集上验证这些字段的真实增益。

## 5. Hybrid 救回了什么，又损失了什么

### 5.1 典型救回

字段 BM25 未进 Top-5、Hybrid 进 Top-5 的例子包括：

| Query | Gold | Hybrid 的作用 |
|---|---|---|
| 需要新建一份 AI 多维表（able）时 | `aitable.base_create` | 把多维表语义与 Base 创建关联起来 |
| 需要重命名 AI 表格 Base 时 | `aitable.base_update` | 从大量 Base list/search 中提升 update |
| 用户明确要求删除仪表盘中的图表时 | `aitable.chart_delete` | 从 dashboard delete 扩展到 chart delete |
| 查看仪表盘配置或拿到 chartId 时 | `aitable.dashboard_get` | 找到“查看配置”的读取能力 |
| 查看、筛选、全文搜索或遍历记录时的主入口 | `aitable.query_records` | 从弱词面重合恢复到记录查询入口 |

### 5.2 典型退化

Hybrid 也会把 BM25 原本命中的工具挤出 Top-5：

| Query | Gold | 退化原因 |
|---|---|---|
| 已知 roleId，需要看字段/行级规则时 | `aitable.advperm_role_get` | Dense 被“字段/规则/已知 ID”泛化到考勤和视图设置 |
| 已知 baseId，需要看 Base 元数据与下属 tableId 时 | `aitable.base_get` | Dense 偏向各种 `get_*_info` 通用读取能力 |
| 创建/更新仪表盘前需要参考配置 JSON 模板时 | `aitable.dashboard_config_example` | Dense 更偏实际 create/update/get，弱化 template/example |
| 查开放平台 OpenAPI、错误码、OAuth2 | `devdoc.search_open_platform_docs_rag` | `dev.*` 与 `devdoc.*` alias/兼容能力竞争 |
| 三步上传最后一步提交入库 | `drive.commit_upload` | Dense 提升了语义相近的 upload，而忽略流程阶段“最后一步” |

这些错误说明后续 reranker 需要显式使用：

- 资源 ID/前置参数是否已知；
- get/create/update/delete 等 effect/verb；
- workflow stage 和 `requires / produces`；
- alias/迁移/兼容入口优先级；
- template/example 与真实执行工具的区别。

## 6. 对实现方案的修正

基于本轮数据，推荐把原方案修正为：

1. **MVP 保留 Exact + BM25**，但不要宣称当前字段权重已优于 unfielded；先作为可配置实验参数。
2. **Dense + RRF 的历史 proxy 增益不足以支持产品复杂度**，不进入当前实现；如未来重启必须另立 RFC 并使用独立数据证明。
3. **Exact Guard 是强制约束**，不是调权重选项。canonical/CLI path 的 100% Top-1 必须独立于语义融合。
4. **`avoid_when` 进入独立 contradiction gate**。Forbidden@5 不能作为普通相关性权重优化。
5. **多工具 query 先分解再检索**，每个动作保留候选预算，再做最小完整集合选择；不要直接对整句只取全局 Top-5。
6. **参数和关系元数据需要结构化**：`requires`、`produces`、`verifies`、effect、resource type 和 workflow stage 应进入 deterministic rerank。
7. **建立真正的独立测试集**：本轮 use/avoid 元数据适合 Phase 0，但 release gate 需要业务团队编写的自然 query、替代工具、forbidden 和多工具依赖，训练/调参/测试严格隔离。

## 7. 复现

只运行标准库词法方法：

```bash
python3 scripts/dev/eval_tool_search_ranking.py \
  --output /tmp/dws-tool-search-lexical-eval.json
```

运行 Dense 与 Hybrid：

```bash
uv run --with fastembed==0.7.3 \
  scripts/dev/eval_tool_search_ranking.py \
  --dense-backend fastembed \
  --dense-model BAAI/bge-small-zh-v1.5 \
  --output /tmp/dws-tool-search-full-eval.json
```

运行 evaluator 单元测试：

```bash
python3 -m unittest scripts/dev/eval_tool_search_ranking_test.py
```

评测输出包含：协议和 Catalog hash、每种方法的完整指标、按产品最差分片、Intent miss、Forbidden Top-1、工作流缺失工具、Hybrid 相对字段 BM25 分数集成的救回/退化案例，以及固定随机种子的 paired bootstrap 区间。

两次独立 Dense 运行的质量指标 JSON 已做逐字段比较，结果完全一致；耗时字段因运行状态不同不要求相同。

## 8. Go 运行时对比与 Agent A/B

Python evaluator 用于离线算法研究；发布路径的检索、workflow 诊断和 Agent A/B 聚合全部已有 Go 实现。执行：

```bash
make generate-tool-search-comparison
```

会从当前 Go 声明运行时装配的 typed Catalog 重新生成 `.worktrees/policy-tmp/tool-search-comparison.json`，仓库不提交该产物。当前默认 Go ranker 的诊断结果是：

| 项目 | 结果 |
|---|---:|
| 1,123 条预算内同源 intent R@1 / R@5 / MRR@5 | 65.27% / 88.16% / 0.7441 |
| Go action ranker shadow R@1 / R@5 / MRR@5 | 62.51% / 84.24% / 0.7131 |
| Go TF-IDF shadow R@1 / R@5 / MRR@5 | 48.09% / 77.83% / 0.5936 |
| 负向 proxy Forbidden@1 / Forbidden@5 | 0% / 0.0719%（1/1,390） |
| 10 条 workflow 整句 Complete@5 / required recall | 40% / 61.67% |
| reviewed subquery 后 Complete@5 / required recall | 80% / 90% |
| Search + gold Inspect 平均 compact JSON | 4,507.03 bytes（oracle-assisted capacity upper bound） |
| 相对 oracle 导航 `overview → product → inspect` 的 byte reduction | 96.3311% |
| 相对全量紧凑 Schema 的 byte reduction | 99.9749% |

这解决了“Python 离线结果和 Go 生产实现混在一起”的问题：Python 表格用于记录候选算法研究，Go 表格锁定 shipped runtime。由于 proxy 同源且 action ranker 的整体与纯中文 R@5 分别低 3.92 / 3.98 pp，当前默认回到更简单的字段 BM25；action/product/entity 重排仅作为 shadow，由 sealed 独立 qrels 上的配对 product-cluster CI 决定是否具备切换资格。Go TF-IDF 使用 fielded/raw-TF，而 Python 头条 TF-IDF 使用 unfielded/log-TF；当前 Go shadow 明显更差，不能因为历史 Python 的 84.55% 就切换生产默认。两边由于 Catalog、投影和 TF 定义不同，不声称逐 case parity。

### 8.1 当前 release binary 冷进程与包体（Apple M3 Pro）

使用相同 `-buildmode=pie -trimpath -ldflags='-s -w'` 构建当前分支和 `upstream/main@5fed80fc`。当前二进制为 28,471,026 bytes，上游为 28,267,138 bytes，增量 **203,888 bytes（约 199 KiB）**，低于 RFC 的 1 MiB 目标。每组 20 个独立进程、输出丢弃后的结果：

| 命令 | cold p50 / p95 / p99 | RSS p50 / p95 |
|---|---:|---:|
| 当前 `schema search` | 1.26 / 1.28 / 1.30 s | 281.8 / 287.2 MiB |
| 当前 `schema` overview | 1.39 / 1.88 / 1.94 s | 281.8 / 288.1 MiB |
| upstream `schema` overview | 3.41 / 7.81 / 10.87 s | 283.1 / 290.2 MiB |

这是单机诊断，不是跨平台 SLA：进程缓存、系统负载和上游 overview 的长尾会影响结果。它证明 Tool Search 没有带来 MiB 级包体膨胀，且 Search 冷进程没有比同分支 Schema 装配更慢；但约 1.3 秒/282 MiB 仍说明短命 subprocess 的主要成本来自 CLI 启动与完整 Catalog 装配。Linux amd64 与 warm 10k query 门禁仍需 CI/长生命周期 Host 数据。

真正的 Agent 效果必须由相同模型、prompt、权限和预算下的配对 run 提供。Go 的 `AggregateToolSearchAgentAB` 接收 `tool-search-agent-ab.v1`，要求每个 `case_id/trial` 同时存在 `direct_schema` 与 `search_inspect`，聚合 task success、correct plan、unsafe action、recovery、token、tool calls 和 latency，并给出按 case 聚类的固定种子 bootstrap 区间。当前未提供真实模型 run，因此生成报告明确标记 `agent_ab_status=not_run_requires_independent_tasks_and_model_runs`。

合并主线前曾使用 `gpt-5.6-sol` 做过一个只读规划 smoke：模型只收到 10 条业务 query，不收到 required gold；direct arm 只能用 Schema 分层导航，search arm 只能用 Search→Inspect，均禁止执行业务命令。该结果基于旧 572 工具 Catalog，已被当前 1,098 工具 surface **判为过期，不能作为现行效果证明**，仅保留方法记录：

| 规划指标 | Direct Schema | Search→Inspect |
|---|---:|---:|
| required-tool Complete / Recall | 100% / 100% | 100% / 100% |
| Exact minimal plan | **90%** | 70% |
| Plan precision | **95.45%** | 87.50% |
| 额外步骤 | **1** | 3 |

Search 的额外步骤是 wiki space search、minutes detail read，以及两臂都加入的 pending-approval list。它们可能是合理前置，也可能是非最小噪声；当前 fixture 只标 minimal required，因此按 extra 计。该实验每 arm 只有一个 batch/一次 trial，没有置信区间，也没有执行、参数、恢复和 token 的可信配对数据。它只能说明“搜索覆盖没有退化，但尚未超过现有导航的计划精度”，不能通过默认开启门禁。

## 9. 局限与下一轮

- 当前 1,123 条预算内 Intent 来自工具作者的 `use_when`，虽然该字段没有被索引，语言风格仍比真实用户 query 更规范；
- Forbidden query 是原始 `avoid_when`，目前没有结构化替代 gold，不能衡量“是否正确找到了替代工具”；
- Workflow 只有 10 条，80% 的人工分解上限置信区间很宽；
- 只测试一个中文小型 Dense 模型，不能推出所有 embedding/cross-encoder 的效果；
- 没有加入 namespace router、权限 filter、cross-encoder、LLM reranker 和真实 Agent 调用；
- 当前 Python 实现面向可读性和可复现性，不是性能基准；
- 本轮没有评测最终工具调用成功率、写后验证和失败恢复。

下一轮优先级：

1. 业务团队补 300～500 条真实 query，其中至少 80 条多工具任务、100 条 hard negative；
2. 按 query 模板和工具族切分 train/dev/test，再调 BM25/TF-IDF 字段权重；
3. 继续记录 Dense/cross-encoder 的离线研究结果，但不接入产品协议；
4. 实现 Exact Guard、permission hard filter、avoid contradiction 和 query decomposition 的端到端 shadow evaluator；
5. 把 Recall@5、Forbidden Exposure、Comprehensiveness@5、Agent task success 与 recovery success 一起作为发布门禁。
