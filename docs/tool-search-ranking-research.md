# DWS 工具检索与排序源码实现调研

> 选型更新（2026-08-12）：本文记录前期源码调研和候选参数，不代表最终
> DWS CLI 方案。后续多算法、中文切片、CLI 体积和多 Agent 独立审计否决了
> “内嵌 Dense”与“锁死 BM25”的假设。正式决策、整体收益和决策过程见
> [`rfc-tool-search-progressive-discovery.md`](rfc-tool-search-progressive-discovery.md)；
> 固定 commit 的源码架构复核见
> [`tool-search-github-architecture-research.md`](tool-search-github-architecture-research.md)。

> 调研日期：2026-08-12
>
> 调研对象：官方 Tool Search 协议、GitHub 可核验实现、工具检索评测与 DWS 当前 Schema Catalog
>
> 结论范围：本文讨论“让 Agent 找得到工具”的检索和排序层；Schema 检查、工具执行、结果验证与失败恢复是后续独立层，不能由相关性分数替代。

## 1. 结论先行

DWS 不适合只实现一个 `search 关键词` 的字符串过滤，也不应直接把当前 1,098 个完整 Tool Schema 交给模型选择。最合适的目标形态是：

```text
Catalog 搜索（返回轻量 ToolReference）
  → Inspect（加载完整 Tool Schema）
  → Execute（执行并返回结构化回执）
  → Verify / Recover（验证结果或安全恢复）
```

检索排序应采用分层管线，而不是一个不可解释的总分：

```text
查询 + 最近任务上下文
  → 权限、可用性、接口和风险策略硬过滤
  → 精确路径 / 名称 / alias 匹配
  → BM25 词法召回 ─┐
                    ├→ RRF 名次融合 → 确定性特征重排 → Top-K ToolReference
  → Dense 语义召回 ─┘                            └→ 多工具集合补全
```

推荐分阶段落地：

1. **第一版先做“硬过滤 + 精确匹配 + 字段化 BM25 + 稳定 Top-5”**。本地、无模型、容易回归，也能直接利用现有人工评审元数据。
2. **独立离线评测支持语义召回有增益后，再加 Dense + RRF**。不要把 BM25 原始分和 cosine 原始分线性相加。
3. **只在低置信度或歧义查询上做 Cross-encoder/小模型重排**，候选池控制在 20～50 个，不能让 LLM 扫全量工具。
4. **在线使用反馈只能来自真实执行行为和最终结果**，不能学习检索器自己的曝光结果，否则会自我强化错误。
5. 对“给群里发文件并确认送达”这类任务，评测目标不是 Top-1，而是 Top-K 内同时覆盖“发送”和“查询状态”两个能力；这需要集合完整度指标和工作流依赖信息。

`dws discover --intent "..."` 在当前仓库里并不存在，它只是前期讨论中的概念命令。若实施，建议保留现有精确查询语义，并新增明确的搜索入口，例如：

```bash
# 提议接口，不是当前已发布命令
dws schema search --query "给群里发文件并确认送达" --top-k 5
```

Agent/MCP 侧对应一个轻量 `search_tools`，返回 canonical path、摘要、匹配原因、风险和 inspect 引用，不直接返回全部参数 Schema。

## 2. DWS 当前基线与可复用资产

### 2.1 当前只有精确解析，没有自然语言排序

当前 `dws schema [path]` 支持稳定 canonical path、CLI path 及兼容的点号/斜杠路径查询；[`SchemaIndex.Resolve`](../internal/cli/schema_contract_model.go) 明确声明“不做推断或模糊匹配”。因此：

- `dws schema chat.send_personal_message` 能精确找到工具；
- `dws schema "给群里发文件"` 不能按意图检索；
- `dws schema` 的渐进展开解决了“不要一次加载全部 Schema”，但还没有解决“Agent 如何从自然语言找到 leaf”。

### 2.2 Catalog 已经具备很好的检索语料

主线合并后，Catalog 不再是仓库内嵌 JSON。`RegisterSchemaSourceRoot → ResolveSchemaBuild → deliverySchemaCatalog` 直接从 Go `ProductDecl` / `ContractDecl` 结构化声明装配出：

- 27 个产品；
- 1,098 个运行时工具；
- 1,123 条预算内正向诊断 query，另有 10 条超生产预算被计数排除；
- 1,390 条负向诊断 query；
- canonical path、CLI path、标题、描述、参数、effect、risk、confirmation、idempotency、interface/provenance 等结构化字段。

运行时 Catalog 校验 source/surface hash、完整性和接口 provenance。这比从 Cobra help、MCP `tools/list` 或生成 JSON 临时拼文本可靠得多，应当成为唯一检索语料源。`cmd_schema_catalog` 只允许在构建/CI 时把分片生成到被忽略的 `.worktrees/policy-tmp`，不是生产输入，也不得提交产物。

现有 selection fixture 会把 `use_when` / `avoid_when` 变成可复现的正负诊断用例，并且刻意不把字符串匹配伪装成 Agent 语义评测。它适合作为同源 proxy 与回归集，不得冒充独立 qrels。

### 2.3 DWS 的关键优势不是工具数量，而是负向元数据

很多开源实现只索引 `name + description`。DWS 额外拥有人工评审的 `avoid_when`、风险、确认和幂等语义。使用方式必须区分：

- `use_when` 是正向召回和重排信号；
- `avoid_when` 是冲突检测或负向重排信号，**不能当普通正文本一起索引**；
- `availability`、调用身份、权限和产品开关应是硬过滤；
- `risk`、`effect`、`confirmation` 不代表语义不相关，通常用于硬策略、同相关度下的排序和结果展示；
- `idempotency` 主要服务执行与恢复，不应被相关性排序吞掉。

## 3. 官方产品告诉了我们什么

### 3.1 Anthropic Tool Search

[官方文档](https://platform.claude.com/docs/en/agents-and-tools/tool-use/tool-search-tool)提供 BM25 和 Regex 搜索，搜索名称、描述、参数名称与参数描述；工具以 `defer_loading: true` 延迟加载，默认最多返回 5 个 `tool_reference`，声明最多可放入 10,000 个延迟工具。

其协议流程与 DWS 很接近：

```text
模型先看到 tool_search
  → 搜索
  → 返回 tool_reference
  → 加载完整 Schema
  → 调用工具
```

但要注意：开源 [Anthropic Python SDK 类型定义](https://github.com/anthropics/anthropic-sdk-python/tree/009b035305e0724ce108ebd796935f91711fc6e1/src/anthropic/types/beta) 只包含 BM25/Regex、`tool_search_tool_result` 和仅含 `tool_name` 的 `tool_reference` 等请求/响应类型；普通 ToolParam 也只有 `defer_loading` 声明。**BM25/Regex 的索引、分词、权重、排序和“引用后注入定义”都是托管能力，没有开源**。官方 client-side 示例反而只是扫描 tool JSON 的 substring search，说明协议允许替换 ranker，不说明 substring/BM25 对 DWS 最优。本地 dispatch 只维护 available tool name，不是加载运行时。

### 3.2 OpenAI Agents SDK

[官方 Tool Search 文档](https://openai.github.io/openai-agents-python/tools/)支持延迟函数、namespace 和 MCP server。真实 SDK 代码中：

- [`ToolSearchTool` 数据结构](https://github.com/openai/openai-agents-python/blob/5250cb86053f50abea9d30e7d06b8fc4b5b6adb1/src/agents/tool.py#L1511)声明 hosted/client search 配置；[独立 validator](https://github.com/openai/openai-agents-python/blob/5250cb86053f50abea9d30e7d06b8fc4b5b6adb1/src/agents/tool.py#L1653)限制一个 Agent 只能配置一个搜索工具，并检查 deferred surface；
- [Responses 请求转换](https://github.com/openai/openai-agents-python/blob/5250cb86053f50abea9d30e7d06b8fc4b5b6adb1/src/agents/models/openai_responses.py#L1970)把 namespace 和 tool-search 配置序列化给服务端；
- [转换测试](https://github.com/openai/openai-agents-python/blob/5250cb86053f50abea9d30e7d06b8fc4b5b6adb1/tests/models/test_openai_responses_converter.py#L760)覆盖延迟 namespace 和搜索声明。

`_tool_identity.py` 还实现 qualified name、bare/namespaced/deferred lookup key、合成 namespace、名字冲突诊断与 approval key；这比排序算法更值得复用。SDK 文档建议 namespace 尽量少于 10 个函数，但 `execution="client"` 明确不会由标准 Runner 执行，deferred/namespace 又是 Responses-only 特性。同样，SDK 开源的是声明、校验和 wire orchestration，**托管检索器和本地定义注入运行时均未公开**。DWS 可用 namespace 作 reviewed hint/hard filter，不应强制唯一 namespace 路由。

### 3.3 MCP

[MCP Tools 规范](https://modelcontextprotocol.io/specification/draft/server/tools)中的 `tools/list` 负责枚举、分页、缓存和变更通知，不定义自然语言检索。客户端要自行建设 Catalog 搜索。

[MCP 客户端最佳实践](https://modelcontextprotocol.io/docs/2026-07-28/develop/clients/client-best-practices)推荐 Search → Inspect → Execute，并列出 Regex/BM25、Embedding、小模型和混合检索。这意味着 DWS 不应修改 MCP 的 `tools/list` 语义来塞入模糊排序，而应提供独立 `search_tools` 或本地 Schema 搜索能力。

### 3.4 AWS AgentCore Registry

[AWS 官方 Registry Search 文档](https://docs.aws.amazon.com/bedrock-agentcore/latest/devguide/registry-search-records.html)披露了更完整的生产行为：关键词和语义检索并行后合并；name 对关键词排序影响最大；description 同时参与关键词和语义匹配；结构化 filter 在两路打分前应用；只有 Approved 记录可被搜索；索引采用最终一致性，刚批准的记录可能需要退避重试并用强一致读取确认状态。

这直接支持 DWS 的两个设计：已知属性必须走 filter 而不是混入自然语言 query，治理状态必须先于相关性排序。AWS 没有公开融合公式，因此它是产品行为参考，不是可复用的排序源码。

## 4. GitHub 源码实现与证据等级比较

以下结论均来自固定 commit，避免只引用 README 的营销描述。

| 实现 | 排序方式 | 真实代码提供了什么 | 对 DWS 的价值 | 局限 |
|---|---|---|---|---|
| [LangGraph BigTool](https://github.com/langchain-ai/langgraph-bigtool/tree/0bb7f9227d349afa4d4207c6630e800658c80894) | 默认调用 Store semantic search；可插拔检索器 | [`tools.py`](https://github.com/langchain-ai/langgraph-bigtool/blob/0bb7f9227d349afa4d4207c6630e800658c80894/langgraph_bigtool/tools.py#L14)默认 `limit=2`；[`graph.py`](https://github.com/langchain-ai/langgraph-bigtool/blob/0bb7f9227d349afa4d4207c6630e800658c80894/langgraph_bigtool/graph.py#L83)把已选工具逐轮绑定给模型并去重累积 | 展示显式 Search→bind→execute 的 Agent 循环 | unknown ID 裸索引直接 KeyError，且只增不减 reducer 会让坏 ID 持续污染线程；状态无预算 |
| [Semantic Kernel](https://github.com/microsoft/semantic-kernel/tree/c028a0c7dc4f0814cdcbaba9d998f187a41197bf) | 框架自动上下文向量检索 | [`FunctionStore.cs`](https://github.com/microsoft/semantic-kernel/blob/c028a0c7dc4f0814cdcbaba9d998f187a41197bf/dotnet/src/SemanticKernel.Core/Functions/ContextualSelection/FunctionStore.cs#L105)向量搜索 `Function name + description`；[`ContextualFunctionProvider`](https://github.com/microsoft/semantic-kernel/blob/c028a0c7dc4f0814cdcbaba9d998f187a41197bf/dotnet/src/SemanticKernel.Core/Functions/ContextualSelection/ContextualFunctionProvider.cs#L97)首次懒向量化，并默认用最近 2 条消息构造 query | 提供“模型无感自动注入”的对照范式 | `SKEXP0130`；collection/embedding/unknown name fail-fast；外部 store 生命周期由调用方负责，不存参数 Schema |
| [ToolUniverse](https://github.com/mims-harvard/ToolUniverse/tree/cf5566c3d6c361d19fd826eaae7e791c83451a4d) | BM25、Embedding、LLM 三套 finder | 有完整可读的分词、BM25、exact bonus、embedding cache、Top-K 和 LLM 预过滤代码 | 最具体的开源工具排序参考，可观察不同排序类型的真实权衡 | 三套 finder 尚未统一成强混合管线；部分 filter 行为不一致 |
| [Ratel](https://github.com/ratel-ai/ratel/tree/dcb657f8f695bad48585e4b0a7aa83e4a07cf3ac) | BM25 默认；Dense/Hybrid 可选；加权 RRF；可选 usage arm | 提供稳定索引投影、缓存、事务化向量更新、确定性排序、可捕获错误边界、并发保护和在线反馈设计 | 与 DWS 目标最接近，工程约束比单纯“搜到工具”完整 | Dense/Hybrid 错误会向上返回，并不自动回退；Rust/英文默认模型不能原样搬入 DWS |
| [ToolRet](https://github.com/mangopy/tool-retrieval-benchmark/tree/c4181d914a227134705ecb6bab13fbd92ccd2938) | BM25、Dense、ColBERT、Cross-encoder/LLM rerank | [`eval.py`](https://github.com/mangopy/tool-retrieval-benchmark/blob/c4181d914a227134705ecb6bab13fbd92ccd2938/toolret/eval.py#L78)实现 nDCG、MAP、Recall、Precision 和 Comprehensiveness；统一 Top-100 召回再重排 | 给 DWS 提供多相关工具的评测方法 | 是 benchmark，不是生产 Catalog 服务 |
| [Haystack](https://github.com/deepset-ai/haystack/tree/e7d26438e9f7eb2c64381546bbe5bb528c54cc4b) | 多路检索 + 等权 RRF | [`_reciprocal_rank_fusion`](https://github.com/deepset-ai/haystack/blob/e7d26438e9f7eb2c64381546bbe5bb528c54cc4b/haystack/utils/misc.py#L156)按名次融合并去重；[`MultiRetriever`](https://github.com/deepset-ai/haystack/blob/e7d26438e9f7eb2c64381546bbe5bb528c54cc4b/haystack/components/retrievers/multi_retriever.py#L120)并发多路召回、全局排序后再截 Top-K | 提供通用异构排名融合参考 | 组件仍标记 experimental，任一路失败会整体抛错；不理解工具风险、权限和工作流依赖 |

### 4.1 ToolUniverse：BM25 不是“关键词包含”

[`tool_finder_keyword.py`](https://github.com/mims-harvard/ToolUniverse/blob/cf5566c3d6c361d19fd826eaae7e791c83451a4d/src/tooluniverse/tool_finder_keyword.py#L258)的具体做法包括：

- 索引 name、description、type、category 和参数名；
- stop words、简单 stemming，并扩展 bigram/trigram；
- BM25 使用 `k1=1.5, b=0.75`；
- 对多词短语用 `phrase_discount=0.3` 逐级折扣，且文档长度只用原始 token 数，避免 n-gram 扩展同时扭曲长度归一化；
- exact whole-name、name 子串、名称 token、描述整句和类别命中有独立 bonus；
- 特意不索引参数描述，因为重复模板文本制造了错误命中。

最后一点对 DWS 非常重要，但证据强度需要分层：源码 docstring 是“作者确实因英文生物医学模板噪声而排除参数描述”的直接、高可信证据；仓库却没有公开消融，因此它不是“排除参数描述普遍提升召回”的效果证据。其 tokenizer 正则只接受 ASCII，纯中文 query 会得到空 token 和零召回。Anthropic 会搜索参数描述，而 ToolUniverse keyword 选择排除、embedding 又编码完整 JSON；DWS 不应照抄任一方，应把“参数名”和“参数描述”按召回通道分别消融。

[`tool_finder_embedding.py`](https://github.com/mims-harvard/ToolUniverse/blob/cf5566c3d6c361d19fd826eaae7e791c83451a4d/src/tooluniverse/tool_finder_embedding.py#L323)把工具完整 prompt 编码并做归一化 cosine，磁盘 cache key 包含工具文本、backend、模型和 prompt，能够避免这些配置变化时复用错误向量。但运行时 refresh 只比较工具名集合，同名工具描述/Schema 变化可能漏刷；缓存写入也没有展示 DWS 所需的原子 rename/进程锁。因此只借鉴 cache key 维度，不照搬 freshness 和持久化实现。

[`tool_finder_llm.py`](https://github.com/mims-harvard/ToolUniverse/blob/cf5566c3d6c361d19fd826eaae7e791c83451a4d/src/tooluniverse/tool_finder_llm.py#L213)不是让 LLM 直接扫描全库，而是先用 name/description 关键词把候选压到 50 个，再截短描述交给模型。零分候选仍可能按原始顺序填满 50 个，词面无重合的正确工具会在 LLM 前丢失；它适合做候选内重排，不是可靠的第一阶段召回。

ToolUniverse 没有统一 Hybrid：`auto` 是 keyword > embedding > LLM 的单选/异常回退链，不是多路深召回与融合。keyword freshness 只比较工具数，embedding refresh 只比较工具名集合；同数量/同名但描述或 Schema 变化都会漏刷。可直接借鉴的是 exact bonus 层级、完整 cache-key 维度和 over-fetch 后过滤，不是 tokenizer、freshness 或排序实现。

### 4.2 Ratel：最值得复用的是失败语义和确定性

Ratel 的 [`searchable_text`](https://github.com/ratel-ai/ratel/blob/dcb657f8f695bad48585e4b0a7aa83e4a07cf3ac/src/core/src/indexing.rs)只抽取工具名称、描述、参数/返回字段名、描述与 enum，跳过 `type`、`required`、括号等 JSON 结构噪声；snake_case 和 camelCase 同时保留完整标识符与拆词形式。

[`Bm25Index`](https://github.com/ratel-ai/ratel/blob/dcb657f8f695bad48585e4b0a7aa83e4a07cf3ac/src/core/src/search.rs)使用 `k1=0.9, b=0.4`，针对短工具描述降低词频和长度归一化强度。更重要的是，它不直接拿底层引擎的 Top-K：底层候选通过 HashSet 时，同分项可能跨进程变化。实现会先排完整候选，再以 `(score desc, id asc)` 确定性排序，最后截断，保证同分落在 Top-K 边界时成员也稳定。

[`fusion.rs`](https://github.com/ratel-ai/ratel/blob/dcb657f8f695bad48585e4b0a7aa83e4a07cf3ac/src/core/src/fusion.rs)提供加权 RRF：

```text
rrf_score(tool) = Σ arm_weight / (60 + rank_in_arm(tool))
```

BM25 与 Dense 各召回到固定深度 100，再融合。RRF 只使用每路名次，避免把无上界的 BM25 分数与 `[-1, 1]` cosine 分数强行归一化。融合后仍以 tool ID 作为同分 tie-breaker。

其 [检索 ADR](https://github.com/ratel-ai/ratel/blob/dcb657f8f695bad48585e4b0a7aa83e4a07cf3ac/docs/adr/0011-selectable-retrieval-methods.md)还定义了值得 DWS 对照的失败语义：

- BM25 是默认、无模型、不可失败路径；
- Dense/Hybrid 是显式 opt-in；
- 搜索请求绝不临时重建全量向量；
- embedding 增量 `extend` 先校验整批条数/维度/fingerprint，再一次性提交；失败时未完成的 ID 继续 missing，其他已存在向量不受影响；若调用方先 `invalidate(id)`，失败不会神奇恢复该旧向量；
- 全量 `rebuild` 失败时不替换旧完整 state；
- Catalog metadata reload 后即使 Dense embedding 构建失败，新的 BM25 corpus 仍可用；Semantic/Hybrid 返回 `EmbeddingsNotBuilt`，由调用方显式选择 BM25，而不是库内静默 fallback；
- score 只在当前列表内可解释，调用方应使用稳定 `rank`，不能伪造跨 BM25/cosine/RRF 的统一“置信度”。

这些行为应当进入 DWS 的验收标准，而不是实现后再补。

`k1=.9/b=.4`、RRF `k=60`（0-based rank）和 depth=100 在固定 commit 中都是真实常量并有注释理由，但不是 DWS 参数最优性的评测证据。Ratel 默认 `Language::English` 与 `bge-small-en-v1.5`；DWS 的中文 proxy sweep 已显示 sparse arm、depth 和 k 会改变 R@K/Workflow。因此三者只进入 dev control/sweep，不直接成为发布默认。

### 4.3 ToolRet：复用指标定义，不复用评测脚手架

固定 commit 的 `trec_eval` 定义了 NDCG/MAP/Recall/Precision@5/10/20，并把 `Comprehensiveness@K` 定义为 `Recall@K == 1` 的 query 占比，即“所有相关工具零遗漏进入 Top-K”。这与 DWS 的 workflow Complete@K 同义，且分级 relevance qrels 可以支持 NDCG。HF 当前可见数据约为 7,961 queries、44,453 tools、35 个英文任务；论文口径的 7.6k/43k 是近似值，数据集又没有随 Git commit 锁 revision，因此引用时必须同时注明源码 commit 与 dataset revision/snapshot。

不能直接使用仓库 harness：`eval_bm25` 含未定义 `runs/args`、错误 category/字段、qrels 只保留最后 query 且未调用指标；`eval_colbert_v2` 未初始化 `results[task]`；rerank 的 instruction 布尔语义与 retrieval 相反且无测试。仓库没有 tests，语料/stopwords 全英文，也没有字段消融。DWS 应用自己有单测的 evaluator 或直接调用成熟 `pytrec_eval`，复用指标数学定义、分级 qrels、instruction/no-instruction 对照和“固定一阶段候选再公平评 reranker”的协议，不复制这些执行路径。

### 4.4 在线反馈：不能从曝光学习曝光

Ratel 的 [Adaptive Usage Ranking ADR](https://github.com/ratel-ai/ratel/blob/dcb657f8f695bad48585e4b0a7aa83e4a07cf3ac/docs/adr/0014-adaptive-usage-ranking.md)把使用历史作为第三个、低于基础检索权重的 RRF arm。设计中最关键的约束是：

- 只从实际 `InvokeStart` / 最终调用学习，不从 SearchHit 学习；
- 无历史命中时 usage arm 完全缺席，基础排序 bit-identical；
- 使用证据权重低于词法/语义证据，不能凭历史凭空召回基础检索完全无关的工具；
- embedding 模型变化时，如果向量空间不兼容就暂停 usage boost；
- “误合并意图”比“漏合并意图”更危险，因此聚类阈值应保守。

DWS 后续若引入团队/用户个性化，也应遵循相同原则，并额外做租户隔离、时间衰减和隐私控制。

## 5. 不同排序类型该放在哪一层

| 类型 | 擅长 | 不擅长 | 在 DWS 中的位置 |
|---|---|---|---|
| Regex / substring | canonical path、CLI path、固定编号、前缀搜索 | 同义词、口语意图、跨语言 | 精确命中/调试入口，不作为主自然语言排序 |
| Exact + alias | 已知工具名、命令名、产品名 | 未知词汇表达 | 最高优先的确定性召回 arm |
| BM25 / BM25F | 稀有关键词、字段名、产品名、CLI 术语；可解释、便宜 | “送达/回执/确认”等没有词面重合的同义表达 | 第一版主召回；长期保留为 Hybrid 一路 |
| Dense embedding | 同义词、自然语言、跨表达方式和上下文语义 | 精确 ID、否定、权限；模型/缓存可能失败 | 第二阶段召回；不得替代硬过滤和 BM25 |
| RRF | 融合不同量纲的有序结果；不需校准原始分 | 不能创造候选，也不能判断权限 | BM25、Dense、Exact、Usage 等 arm 的主融合器 |
| Cross-encoder | Query 与候选联合判断，精排质量高 | 延迟和算力高；只能重排已有候选 | 低 margin/歧义请求的可选 Top-20 精排 |
| LLM / 子 Agent | 理解复杂业务约束、组合多个工具 | 成本、不确定性、提示注入、难回归 | 可选的计划/重排层，不做全库召回 |
| Usage / personalization | 弥补组织黑话和高频路径 | 反馈环、自强化、冷启动、隐私 | 真实成功调用生成的低权重可选 arm |
| Policy / risk | 权限、可用性、身份、危险动作治理 | 不能判断语义是否相关 | 排序前硬过滤 + 排序后展示/门禁，不是普通相关性项 |
| Workflow coverage | 一次意图需要多个工具 | 单文档式 Top-1 指标无法衡量 | RRF 后的集合补全与 plan completeness |

### 5.1 为什么建议 BM25F，而不是把所有文本拼成一段

DWS 字段质量和含义差异很大。把全部字段拼成单文档会让长 description、参数模板和 provenance 稀释 canonical/name/use_when。建议维护字段级倒排统计或等价的权重投影：

| 字段 | 初始建议权重 | 处理方式 |
|---|---:|---|
| canonical path、CLI path、tool name、人工 alias | 8 | 同时保留完整标识符与拆词；完整精确命中走独立 override |
| `agent_summary`、`use_when` | 5 | 正向主语义字段；每条作为独立短文本更容易解释 |
| title、description | 2～3 | 去掉格式和重复模板；长文本限制贡献 |
| 参数名、enum | 2 | kebab/snake/camel 拆词并保留原词 |
| 参数描述 | 0～1 | 默认低权重；先做模板重复率和消融实验 |
| examples | 1～2 | 只索引自然语言意图和稳定命令词；不能把示例里的 ID 当能力词 |
| `avoid_when` | 不进入正向 BM25 | 单独做 Query↔avoid 的冲突特征或 hard contradiction gate |
| provenance、JSON 结构词 | 0 | 不索引 |

以上数字是**待评测的初始化建议，不是行业标准参数**。DWS 中文语料需要中文/英文/标识符混合分词：中文至少使用连续字串或可靠 tokenizer；英文标识符做大小写、下划线、短横线和 camelCase 拆分；canonical 原词必须保留。

### 5.2 为什么不线性相加原始分

以下写法不可取：

```text
final = 0.4 * bm25_score + 0.6 * cosine_score
```

BM25 分数无固定上界，且随 Catalog 文档频率变化；cosine 的分布取决于模型；Cross-encoder 又是第三种量纲。离线 min-max 也会受候选集合和异常值影响。Haystack、Ratel 等成熟实现均选择 RRF，DWS 第一版 Hybrid 也应优先采用稳定名次融合：

```go
// 方案伪代码：不是当前仓库已有实现。
func weightedRRF(arms []RankedArm, k float64) []Hit {
    scores := map[string]float64{}
    for _, arm := range arms {
        for rank, id := range arm.IDs {
            scores[id] += arm.Weight / (k + float64(rank))
        }
    }
    // 必须按 score 降序、canonical path 升序稳定排序，再截 Top-K。
    return stableSort(scores)
}
```

### 5.3 Hard filter 必须先于排序

工业系统里“找不到无权使用的工具”比“把它排到后面”更安全。建议先过滤：

- 当前二进制和 Catalog 中不可用的工具；
- 当前调用身份不支持的 interface mode；
- 租户、组织和用户无权访问的能力；
- 明确禁用的产品/namespace；
- 当前策略禁止暴露的高风险工具；
- 输入上下文已知无法满足前置条件的工具。

过滤后再做检索，但 Search v1 只表达 Catalog availability/product/effect/exclude，不返回 `authorized` 或用户 ACL 结论。权限未知时不能伪装成“允许”；当前 principal/tenant 的鉴权与 destructive confirmation 必须在执行端重新完成。

## 6. DWS 推荐架构

### 6.1 API 分层

建议保留 `SchemaIndex.Resolve` 的精确、无推断契约，新建独立检索层。本节是方案演进记录，不再定义第二套公开 DTO；规范 `tool-search.v1`、`ToolReference`、ExternalRanking 与 `schema-inspect.v1` 以 [RFC 5.6～5.8](rfc-tool-search-progressive-discovery.md#56-candidateprovider-与融合) 为准。当前 `internal/cli` 类型均为 non-public Spike。

建议分成以下组件，避免以后更换模型时重写 CLI：

```text
CatalogSnapshot
  ├─ PolicyFilter
  ├─ ExactRetriever
  ├─ BM25Retriever
  ├─ DenseRetriever（可选）
  ├─ RRFFuser
  ├─ ConstraintReranker
  ├─ WorkflowCompleter
  └─ SearchTrace / EvalRecorder
```

CLI、Agent tool 和 MCP capability 都调用同一个 library service，不能各自维护第二套搜索逻辑。

### 6.2 Catalog 与索引生命周期

建议用现有 `source_hash` / `surface_hash` 作为索引世代标识：

1. 启动时验证由 Go 声明经 `ResolveSchemaBuild` 装配的 Catalog；验证失败继续遵守当前 fail-closed 语义，不能临时从 Cobra 树拼第二份目录。
2. BM25 索引按 Catalog hash 构建一次并复用；Catalog 变更时在新实例中完整构建。
3. 新索引完成全部校验后原子替换；并发查询继续持有旧快照，不观察半更新状态。
4. Dense cache key 至少包含 Catalog hash、投影版本、模型 ID、模型 revision、维度和 query/document prompt。
5. Dense 构建或加载失败时，搜索自动降级为 Exact + BM25，并在 trace 中标记 `degraded_reason`；不能返回空列表。
6. 排序输出必须确定：相同语料、配置和 query 跨进程返回相同 Top-K 成员和顺序。

### 6.3 建议的排序步骤

```text
0. Normalize
   - 仅在请求内存中保留原始 query；默认日志记录 salted digest/脱敏片段
   - 标准化空白、大小写、CLI 标识符
   - 结合最近 1～2 条任务消息，但不能拼入不可信工具输出

1. Identity Resolve + Hard Filter
   - identity 命中后只检查 eligibility：eligible 返回 exact，ineligible 返回 `exact_filtered` 并终止
   - 只有 identity 未命中才进入 availability / interface / product / effect / exclude filter 和模糊检索
   - 用户 ACL 不由 Search 判断，Execute 前重验

2. Exact Arm
   - canonical、CLI path、tool name、alias 完整匹配优先
   - 前缀/子串不能冒充完整精确命中

3. Sparse Arm
   - 字段化 BM25，召回至少 min(100, candidate_count)

4. Dense Arm（可选、由 Agent Host/远端 Provider 承担）
   - 同一稳定文本投影；召回相同深度
   - 失败则记录并跳过，不影响 Sparse

5. Fusion
   - weighted RRF，初始 k=60
   - Exact、BM25、Dense 的权重由 benchmark 决定

6. Constraint Rerank
   - `use_when` 一致性加分
   - `avoid_when` 冲突降分/淘汰
   - 明确 product、effect、对象类型约束
   - 同分以 canonical path 升序

7. Workflow Completion
   - 对多动作 query 识别必要 capability 集合
   - 优先选择能覆盖全部动作、且依赖可衔接的最小工具集

8. Return
   - 默认最多 5 个轻量 ToolReference
   - 返回 rank 与各 arm 名次/匹配字段，不发布伪统一 confidence
```

### 6.4 多工具意图不能只做普通 Top-K

“给群里发文件并确认送达”至少包含：

1. 把文件发到指定群；
2. 获得可查询的 message/conversation 标识；
3. 查询发送/已读状态；
4. 根据业务定义判断“送达”。

DWS Catalog 中相关能力包括：

- `chat.send_personal_message`：以当前用户身份发送群聊或单聊文本/媒体消息；
- `chat.query_msg_read_status`：在已知 conversation ID 和 message ID 时查询已读状态及人员。

搜索时 BM25 容易因“群、发、文件”找到发送工具；Dense 更可能把“确认送达”关联到回执/状态；Workflow Completer 应确保二者都留在 Top-K，而不是让五个相似的发送工具占满结果。

还必须区分业务语义：API 接受消息、消息发送成功、服务端投递、群成员已读并不是同一件事。`query_msg_read_status` 能证明的是其 Schema 定义的“已读状态”，不能自动声称“所有人已收到”。Inspect 层应把该能力边界暴露给模型，执行层再按业务规则验证。

可以给 Catalog 增加不改变工具执行 Schema 的检索关系元数据：

```json
{
  "canonical_path": "chat.query_msg_read_status",
  "requires": ["conversation_id", "message_id"],
  "verifies": ["message_read_status"],
  "common_predecessors": ["chat.send_personal_message"]
}
```

这些关系应由人工评审或执行 trace 证实，不能让模型一次生成后直接进入生产目录。

## 7. 评测：排序好不好必须用 DWS 自己的数据证明

ToolRet 的 [ACL 2025 论文](https://aclanthology.org/2025.findings-acl.1258/)在 7,600 个任务、43,000 个工具上比较多种检索模型，结论是常规 IR 方法在大规模工具检索上仍有明显不足；其代码把“所有相关工具是否都被召回”单列为 Comprehensiveness。DWS 应避免只报告 Top-1 accuracy。

本仓库已完成第一轮无泄漏实测，完整协议、代码、指标、置信区间与失败案例见 [《DWS 工具检索排序实测报告》](tool-search-ranking-evaluation.md)。主要结果是：字段 BM25 + Dense RRF 的 Intent Recall@5 为 85.88%，比字段 BM25 高 3.65 个百分点；Exact Guard 可把融合后下降的 canonical/CLI Top-1 恢复到 100%；人工正确拆解工作流后 Comprehensiveness@5 从 50% 提升到 80%。同时，Dense 单独没有优于 BM25，当前字段权重也没有提高单意图召回，因此二者都不能未经独立验证就直接设为生产默认。

续测补充了更公平的轻量/字段对照：同投影 TF-IDF R@5=84.55%、BM25=83.06%，差异不显著；TF-IDF+BM25 RRF=83.55%，说明同质词法融合没有补召回价值。加入参数描述后 TF-IDF/BM25 分别为 85.05%/83.72%，paired CI 仍跨 0，应保持 shadow。TF-IDF+Dense 默认 RRF=87.87%，dev sweep best=88.70%，但 product-cluster CI 仍跨 0。当前 602 条 query 没有英文；`IncludeUseWhen=true` 会在同源 proxy 上产生 100% R@5 的答案泄漏上界，因此 Go 当前默认已翻转为 false。这些数字仍不能替代独立中/英 test。

### 7.1 数据集构建

从现有评审资产生成至少四类 case：

1. 每条 `use_when` 是正例 query，gold 为对应 canonical tool；
2. 每条 `avoid_when` 是负例/对比 query，gold 为文案指向的替代工具或“不得选当前工具”；
3. examples 去掉具体资源 ID 后生成口语变体；
4. 人工补充跨工具工作流、组织黑话、中文同义词、拼写错误和精确 CLI path。

每个 query 可有多个 gold tool，并标记：

- `required`：完成任务必需；
- `acceptable`：可替代；
- `forbidden`：不应暴露或不应选择；
- `ordered_dependencies`：工具间前后依赖。

训练、调参、测试按 query 模板和工具族隔离，避免同一句 `use_when` 改写泄漏到测试集。

### 7.2 离线检索指标

| 指标 | 目的 |
|---|---|
| Recall@1 / Recall@5 | 必需工具是否进入模型可见范围 |
| MRR、nDCG@5、MAP@5 | 正确工具是否靠前，以及多相关工具排序质量 |
| Comprehensiveness@5 | 多工具任务的全部 required tool 是否都在 Top-5 |
| Forbidden Exposure Rate@5 | `avoid_when`、无权限或策略禁止工具是否被暴露 |
| Exact Top-1 | canonical/CLI path 查询是否 100% 精确置顶 |
| Determinism | 相同 snapshot/query 重复和跨进程结果是否一致 |
| p50/p95 latency | 检索是否满足 Agent 交互延迟 |
| Context/token reduction | 相对全量 Schema 减少多少上下文 |

### 7.3 端到端指标

- Tool selection success：模型最终选到正确工具；
- Task success：真实任务完成，而非只有工具名正确；
- Wrong-tool / forbidden-tool call rate；
- Plan completeness：多工具任务是否形成完整调用链；
- Verify rate：写操作后是否执行可验证读回；
- Recovery success：超时、部分成功和重试后是否安全收敛；
- 额外模型调用数、token 与总体 p95 延迟。

### 7.4 建议发布门槛

以下是 DWS 的**建议目标，需要用实测校准**：

- 无权限、不可用、策略禁止工具暴露率为 0；
- canonical/CLI path 精确查询 Top-1 为 100%；
- 同 snapshot/query 的 Top-K 成员和顺序确定性为 100%；
- 人工评审单工具集 Recall@5 ≥ 98%；
- 多工具集 Comprehensiveness@5 ≥ 95%；
- Hybrid 相比 BM25 在固定测试集上有显著增益后才默认启用；
- Dense/重排失败时 BM25 结果可用且有明确 degraded trace；
- 搜索返回轻量引用相对 `schema --all` 的上下文体积降低至少 80%。

不能只选总体平均分最高的模型。必须按以下 slice 分开看：中文口语、英文 CLI、精确标识符、跨产品、单工具、多工具、写操作、高风险、否定用例、长参数描述、低频工具。

## 8. 实施路线

### Phase 0：先建可复现 benchmark

- 复用 602 条 `use_when`、820 条 `avoid_when`、724 个 examples；
- 增加多工具 gold set 与 forbidden set；
- 固定 Catalog hash、query 集、qrels 和评测脚本；
- 建立 Exact / substring / BM25 基线。

### Phase 1：本地可替换词法内核

- 新增独立 `ToolSearchIndex`，不改变 `SchemaIndex.Resolve`；
- 只从验证后的 Catalog snapshot 建索引；
- 实现混合中文/英文/标识符 tokenizer，以及同投影 TF-IDF/BM25 control；
- Catalog 可用性/product/effect/exclude 硬过滤；
- 默认返回 Top-5 `ToolReference` 与可解释 match reasons；
- 全量排序后以 canonical path 稳定 tie-break，再截 Top-K；
- Go/Python projection parity 与独立 holdout 通过后，再提供 local-only `dws schema search`。

### Phase 2：公开 local-only Search/Inspect

- 增加 `tool-search.v1` 和带 expected Catalog hashes 的 `schema-inspect.v1`；
- 同步 `schema` runnable parent、reviewed disposition、skills 和 command surface 门禁；
- Host shadow 新发现路径，但不改变实际执行。

### Phase 3：Host Provider + RRF

- 选择支持中文和工具语料的 embedding 模型；
- 固定模型 revision 和 projection version；
- Provider 在 Host 外置，DWS CLI 不内嵌模型/runtime；
- Provider 认证、deadline、egress 和脱敏由 Host 管理；只提交带 CatalogVersionRef 的 canonical ranking；
- 本地词法/Provider 各深召回后由 DWS 验证并 RRF；Provider 失败逐字段退回 local-only；
- 通过消融实验决定字段、模型、RRF 权重和是否默认启用。

### Phase 4：歧义重排与工作流覆盖

- 仅对低 margin、跨产品或复杂 query 启用 Cross-encoder/小模型；
- 候选池不超过 20～50；
- 增加 `requires / produces / verifies` 关系；
- 用 set coverage 选最小完整工具集；
- 检索到的文本按不可信数据处理，防止工具描述提示注入影响重排模型。

### Phase 5：受控在线学习

- 只记录真实调用和最终成功/失败；
- 以低权重 usage arm 参与 RRF；
- 租户/用户隔离，设置最小 support、时间衰减和可清除机制；
- 新排序器 shadow 运行，观察错误暴露和回滚指标后再放量。

## 9. 明确不建议的方案

- 不建议只做 `strings.Contains` 或 Regex 并称为自然语言搜索；
- 不建议纯 Dense 替代 exact/BM25，精确 CLI 与新工具冷启动会退化；
- 不建议把 BM25/cosine/LLM raw score 直接线性相加；
- 不建议把完整原始 JSON Schema、provenance 和结构词全部索引；
- 不建议把 `avoid_when` 当正向文本；
- 不建议 LLM 扫当前全量 1,098 个工具后自由排序；
- 不建议检索器从自己的 Top-K 曝光学习；
- 不建议在请求路径临时重建全量 embedding；
- 不建议 Dense 失败时返回空目录或静默切换而不留 trace；
- 不建议把相关性分数当权限、风险、安全确认或执行成功的证明。

## 10. 建议决策

如果现在要为 DWS 立项，建议批准的最小技术决策是：

1. **接口**：Catalog → Search → ToolReference → Inspect → Execute，`search_tools` 与 CLI 共享实现。
2. **第一排序器**：字段化 BM25，Exact 独立优先，硬策略先过滤，Top-K 默认 5。
3. **融合**：Dense 只在 benchmark 证明增益后加入，使用 RRF，不融合原始分。
4. **确定性**：所有路径统一 `(score desc, canonical asc)`，先稳定排序全候选再截断。
5. **可用性**：BM25 永远可用；Dense/重排可选且失败可降级；索引更新原子化。
6. **负向语义**：`avoid_when` 进入冲突门禁和评测，不进入正向全文。
7. **多工具任务**：把 Comprehensiveness@5 和 plan completeness 设为一等指标。
8. **可信与恢复**：搜索结果只说明“候选相关”，Inspect 确认契约，Execute 返回结构化回执，Verify/Recovery 使用 effect、risk、confirmation、idempotency 单独治理。

这样可以同时支撑三个目标：

- **让 Agent 找得到**：Exact + BM25 + 可选 Dense/RRF + 多工具补全；
- **让返回结果可信**：唯一 Catalog、来源 hash、权限硬过滤、match reasons、Inspect 和执行后验证；
- **让失败后安全恢复**：索引原子更新和检索降级保证“还能找到”，幂等/回执/状态机保证“不会因重试制造第二次业务副作用”。

## 11. 主要来源

### 官方协议与产品文档

- [Anthropic Tool Search](https://platform.claude.com/docs/en/agents-and-tools/tool-use/tool-search-tool)
- [Anthropic Advanced Tool Use](https://www.anthropic.com/engineering/advanced-tool-use)
- [OpenAI Agents SDK Tools](https://openai.github.io/openai-agents-python/tools/)
- [MCP Tools Specification](https://modelcontextprotocol.io/specification/draft/server/tools)
- [MCP Client Best Practices: Progressive Tool Discovery](https://modelcontextprotocol.io/docs/2026-07-28/develop/clients/client-best-practices)
- [AWS AgentCore Registry Search](https://docs.aws.amazon.com/bedrock-agentcore/latest/devguide/registry-search-records.html)

### 固定版本源码与论文

- [Anthropic Python SDK @ `009b035`](https://github.com/anthropics/anthropic-sdk-python/tree/009b035305e0724ce108ebd796935f91711fc6e1)
- [OpenAI Agents Python @ `5250cb8`](https://github.com/openai/openai-agents-python/tree/5250cb86053f50abea9d30e7d06b8fc4b5b6adb1)
- [LangGraph BigTool @ `0bb7f92`](https://github.com/langchain-ai/langgraph-bigtool/tree/0bb7f9227d349afa4d4207c6630e800658c80894)
- [Microsoft Semantic Kernel @ `c028a0c`](https://github.com/microsoft/semantic-kernel/tree/c028a0c7dc4f0814cdcbaba9d998f187a41197bf)
- [ToolUniverse @ `cf5566c`](https://github.com/mims-harvard/ToolUniverse/tree/cf5566c3d6c361d19fd826eaae7e791c83451a4d)
- [Ratel @ `dcb657f`](https://github.com/ratel-ai/ratel/tree/dcb657f8f695bad48585e4b0a7aa83e4a07cf3ac)
- [Haystack @ `e7d2643`](https://github.com/deepset-ai/haystack/tree/e7d26438e9f7eb2c64381546bbe5bb528c54cc4b)
- [ToolRet benchmark @ `c4181d9`](https://github.com/mangopy/tool-retrieval-benchmark/tree/c4181d914a227134705ecb6bab13fbd92ccd2938)
- [ToolRet: A Benchmark for Tool Retrieval for Large Language Models, ACL Findings 2025](https://aclanthology.org/2025.findings-acl.1258/)
