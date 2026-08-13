# DWS Tool Search：GitHub 源码架构调研

- 状态：Research / 固定 commit 源码核验
- 日期：2026-08-12
- 关联 RFC：[`rfc-tool-search-progressive-discovery.md`](rfc-tool-search-progressive-discovery.md)
- 排序评测：[`tool-search-ranking-evaluation.md`](tool-search-ranking-evaluation.md)
- 调研范围：工具目录、渐进披露、检索、融合、索引生命周期、Agent 编排、安全边界与失败语义

## 1. 结论先行

GitHub 上没有一个项目可以被 DWS 整体照搬。可复用的是不同项目各自已经验证过的边界：

1. **协议形态采用 Anthropic / OpenAI 的 deferred tool 思路。** 模型先看到搜索入口，命中后才加载完整工具定义；但两个 SDK 都没有开源托管排序实现，不能把其 BM25 名称当作 DWS 算法代码。
2. **Agent 循环参考 LangGraph BigTool。** 检索返回稳定工具 ID，下一轮只把已选工具绑定给模型；DWS 还必须补上候选 ID 校验、Catalog 版本和执行安全。
3. **检索内核重点参考 Ratel。** 最有价值的不是固定 `k1/b`，而是不可变索引快照、全量重建 BM25 统计、稳定 tie-break、两路深召回、RRF、模型指纹和原子向量缓存。
4. **上下文和状态预算参考 NVIDIA NemoClaw。** query、返回文本、已发现工具数、checkpoint 状态和可见 Schema 都应有确定性上限；同时必须明确“披露不是授权”。
5. **中文与字段策略必须由 DWS 自己评测。** ToolUniverse 甚至因模板噪声而只索引参数名、不索引参数描述，说明“字段越全越好”不成立。
6. **失败降级必须由 DWS 明确定义。** Ratel 和 Haystack 都会把某些 Dense/子检索器错误向上抛出，并不自动回退；DWS 的“provider 失败仍返回本地结果”是自己的可靠性契约，不能归因于这些项目。
7. **搜索可信不等于执行可信。** GitHub 参考实现基本不处理 DingTalk 权限、确认、幂等、未知受理状态和补偿；这些必须继续由 DWS 的 Inspect、Cobra、Runner 和 recovery 分层承担。

建议目标架构保持：

```text
reviewed CommandRegistry + Cobra + metadata
                    ↓ one-way resolution
             immutable SchemaRegistry snapshot
              ├─ identity resolve / Inspect index
              └─ local lexical index
                        ↑ untrusted versioned external ranking
                    deterministic validation + fusion
                        ↓ bounded ToolReference
                    exact Inspect + hash check
                        ↓
                    real Cobra Execute
                        ↓
               Verify / Retry / Compensate
```

## 2. 调研方法与证据等级

本报告只使用固定 commit 的仓库源码、测试、ADR 和官方 SDK 类型作为证据。README 只用于导航，不用于证明内部实现。

证据按用途分为四类：

| 等级 | 含义 | 本报告中的项目 |
|---|---|---|
| 协议证据 | 源码中存在 deferred/reference/namespace 等公开形态 | Anthropic SDK、OpenAI Agents SDK |
| 编排证据 | 源码展示搜索结果如何进入 Agent 下一轮工具列表 | LangGraph BigTool、Semantic Kernel、NemoClaw |
| 检索工程证据 | 源码实现索引、缓存、并发、融合和失败边界 | Ratel、ToolUniverse、Haystack |
| 评测证据 | 代码/论文定义指标和多相关工具评测方法 | ToolRet |

“仓库有实现”不等于“适合 DWS 生产默认”。本报告同时检查：

- 是否有固定身份和唯一 Catalog；
- Search 是否与 Inspect / Execute 分开；
- 是否有 exact、hard filter 和稳定 tie-break；
- 索引更新是否原子、可版本化；
- 一路失败时是整体失败还是可降级；
- 是否有权限、确认、幂等和恢复语义；
- 是否有与核心行为直接对应的测试。

## 3. 源码级横向对照

| 项目 | 实际拥有的层 | 已核验实现 | 不应照搬或误读 |
|---|---|---|---|
| [Anthropic Python SDK @ `009b035`](https://github.com/anthropics/anthropic-sdk-python/tree/009b035305e0724ce108ebd796935f91711fc6e1) | BM25/Regex 托管搜索的协议类型；普通 tool 也支持 defer | [`defer_loading`](https://github.com/anthropics/anthropic-sdk-python/blob/009b035305e0724ce108ebd796935f91711fc6e1/src/anthropic/types/beta/beta_tool_search_tool_bm25_20251119_param.py)、`tool_search_tool_result`、仅含 `tool_name` 的 [`tool_reference`](https://github.com/anthropics/anthropic-sdk-python/blob/009b035305e0724ce108ebd796935f91711fc6e1/src/anthropic/types/beta/beta_tool_reference_block.py)；官方 client 示例用 substring 自实现搜索 | SDK 没有 BM25/Regex 检索代码；“加载完整定义”是服务端行为，本地只有 registry/available-name 簿记；confirmation/idempotency 均未提供 |
| [OpenAI Agents SDK @ `5250cb8`](https://github.com/openai/openai-agents-python/tree/5250cb86053f50abea9d30e7d06b8fc4b5b6adb1) | deferred function、namespace、hosted MCP 和 Responses 配置/校验 | [`ToolSearchTool`](https://github.com/openai/openai-agents-python/blob/5250cb86053f50abea9d30e7d06b8fc4b5b6adb1/src/agents/tool.py#L1511)；qualified identity、冲突检测、deferred lookup/approval key；文档建议 namespace 少于 10 个函数 | `execution="client"` 不被标准 Runner 自动执行；非 Responses 后端 fail-fast；SDK 不是本地 ranker，也不实现定义注入运行时 |
| [NVIDIA NemoClaw @ `ac6adac`](https://github.com/NVIDIA/NemoClaw/tree/ac6adacd5a343243d470520a66f76b4ff595ad4a) | 预算完整的渐进披露 middleware | [`progressive_tool_disclosure.py`](https://github.com/NVIDIA/NemoClaw/blob/ac6adacd5a343243d470520a66f76b4ff595ad4a/agents/langchain-deepagents-code/progressive_tool_disclosure.py)限制 query、输出、state、名称、工具数和单项/总 Schema bytes；拒绝重名；checkpoint reducer 确定性 | 搜索只是 case-insensitive substring；完整 executor registry 仍在且猜名可执行；专项验证和接线脚本代表工程证据，不等于已核验生产 SLA |
| [LangGraph BigTool @ `0bb7f92`](https://github.com/langchain-ai/langgraph-bigtool/tree/0bb7f9227d349afa4d4207c6630e800658c80894) | Search → bind selected tools → execute 的 Agent 图 | [`graph.py`](https://github.com/langchain-ai/langgraph-bigtool/blob/0bb7f9227d349afa4d4207c6630e800658c80894/langgraph_bigtool/graph.py#L45)累积 ID，并在下一轮 `bind_tools`；Store semantic search 默认每次 2 个 | unknown ID 裸字典访问导致 KeyError，坏 ID 又因只增 reducer 持续污染线程；状态无预算、无 Catalog hash/权限/恢复 |
| [Semantic Kernel @ `c028a0c`](https://github.com/microsoft/semantic-kernel/tree/c028a0c7dc4f0814cdcbaba9d998f187a41197bf) | 框架自动用近期对话做 function vector search | [`ContextualFunctionProvider`](https://github.com/microsoft/semantic-kernel/blob/c028a0c7dc4f0814cdcbaba9d998f187a41197bf/dotnet/src/SemanticKernel.Core/Functions/ContextualSelection/ContextualFunctionProvider.cs#L94)首次懒向量化，并默认拼最近 2 条消息；FunctionStore 默认只向量化 name + description | 标记 `SKEXP0130`；collection/embedding/unknown name 均 fail-fast；外部 store 同步与生命周期由调用方负责，不存参数 Schema |
| [Ratel @ `dcb657f`](https://github.com/ratel-ai/ratel/tree/dcb657f8f695bad48585e4b0a7aa83e4a07cf3ac) | BM25 / Dense / Hybrid 检索内核与缓存 | [`search.rs`](https://github.com/ratel-ai/ratel/blob/dcb657f8f695bad48585e4b0a7aa83e4a07cf3ac/src/core/src/search.rs)稳定排序和缓存；[`dense_cache.rs`](https://github.com/ratel-ai/ratel/blob/dcb657f8f695bad48585e4b0a7aa83e4a07cf3ac/src/core/src/dense_cache.rs)原子构建；[`fusion.rs`](https://github.com/ratel-ai/ratel/blob/dcb657f8f695bad48585e4b0a7aa83e4a07cf3ac/src/core/src/fusion.rs)两路深召回 RRF | 默认英文 tokenizer/model；Dense/Hybrid 错误会返回 `Err`，不是自动 BM25 fallback；不拥有 Inspect/Execute 安全 |
| [ToolUniverse @ `cf5566c`](https://github.com/mims-harvard/ToolUniverse/tree/cf5566c3d6c361d19fd826eaae7e791c83451a4d) | 实用 keyword/BM25、Embedding、LLM finder | [`tool_finder_keyword.py`](https://github.com/mims-harvard/ToolUniverse/blob/cf5566c3d6c361d19fd826eaae7e791c83451a4d/src/tooluniverse/tool_finder_keyword.py#L269)做 n-gram BM25 和 exact bonus；[`tool_finder_embedding.py`](https://github.com/mims-harvard/ToolUniverse/blob/cf5566c3d6c361d19fd826eaae7e791c83451a4d/src/tooluniverse/tool_finder_embedding.py#L323)把工具文本和模型配置纳入 cache key | 三套 finder 没有统一 Hybrid；参数描述排除只是实现选择/注释，需由 DWS 消融验证；ASCII tokenizer 不支持中文；index/cache freshness 和稳定 tie-break 不够严格 |
| [Haystack @ `e7d2643`](https://github.com/deepset-ai/haystack/tree/e7d26438e9f7eb2c64381546bbe5bb528c54cc4b) | 通用并发多路检索与 RRF | [`MultiRetriever`](https://github.com/deepset-ai/haystack/blob/e7d26438e9f7eb2c64381546bbe5bb528c54cc4b/haystack/components/retrievers/multi_retriever.py#L231)并发执行；[`_reciprocal_rank_fusion`](https://github.com/deepset-ai/haystack/blob/e7d26438e9f7eb2c64381546bbe5bb528c54cc4b/haystack/utils/misc.py#L156)融合异构排名 | 组件标记 experimental；任一路失败会整体失败；MultiRetriever 未暴露 RRF weight；无显式 doc-id tie-break |
| [ToolRet @ `c4181d9`](https://github.com/mangopy/tool-retrieval-benchmark/tree/c4181d914a227134705ecb6bab13fbd92ccd2938) | 大规模工具检索 benchmark | [`eval.py`](https://github.com/mangopy/tool-retrieval-benchmark/blob/c4181d914a227134705ecb6bab13fbd92ccd2938/toolret/eval.py#L78)计算 NDCG、MAP、Recall、Precision、Comprehensiveness | 只负责评测，不是服务架构；固定 commit 无 tests，部分 BM25/rerank/ColBERT 路径存在明显代码缺陷，指标思想可借、harness 不可直接复用 |

## 4. 七个关键架构发现

### 4.1 Progressive disclosure 不是授权

NemoClaw 的源码把这一点写得最明确：middleware 只过滤每次发给模型的工具，完整 executor registry 仍然存在；如果模型猜到隐藏工具名，仍可能进入执行器。因此：

- Search/披露只能减少上下文，不能授予权限；
- `ToolReference.effect/risk` 只是已审核元数据，不是执行许可；
- 用户 ACL、确认、凭据、sandbox 和业务校验必须在执行端再次生效；
- “搜索结果里没有”也不能作为 deny policy，真正 deny 必须在 Runner/transport 边界执行。

这与 DWS 当前分层一致：搜索只消费 typed Catalog，最终仍调用真实 Cobra leaf。

### 4.2 Catalog view 和 executor registry 必须同源但不能混为一谈

BigTool 维护 `tool_registry`，NemoClaw 又专门拒绝同名的不同 owner。原因是：模型看到的 Schema 和 executor 按名字查到的实现如果来自两套去重规则，可能“绑定 A、执行 B”。

DWS 应维持单向发布图：

```text
Cobra + reviewed CommandRegistry + metadata
              → one resolved ToolSpec
              → Search view / Inspect view / execution identity
```

Search 不得重新调用 `tools/list` 建第二份目录，也不得自己生成 executable identity。CandidateProvider 只能返回当前 Catalog 的 canonical path，由 DWS 再解析。

### 4.3 索引一致性比选择哪种 ranker 更基础

Ratel 提供了三个值得直接吸收的规则：

1. BM25 corpus 变化后完整重建统计，不做会冻结旧 `avgdl` 的伪增量 upsert；
2. 先对完整候选做 `(score desc, id asc)`，再截 Top-K，避免 HashSet/浮点同分导致 Top-K 成员漂移；
3. Dense batch 先完整校验 model fingerprint、维度和数量，再一次提交；全量 rebuild 失败保留旧完整向量。注意增量 replace 会先使旧 ID 向量失效，随后 embedding 失败时 Dense 暂不可用；这不是任意更新都保留 last-good snapshot。

DWS 当前 Catalog 是随二进制发布的不可变快照，Alpha 可以在构造时一次建索引。若未来支持远端热更新，应采用比 Ratel whole-catalog reload 更强的 build-then-swap，并增加 `IndexGeneration`：

```go
type CatalogVersionRef struct {
    SourceHash  string
    SurfaceHash string
}

type SearchIndexSnapshot struct {
    Catalog CatalogVersionRef
    Lexical LexicalIndex
    BuiltAt time.Time
}
```

新 snapshot 完整构建和验证成功后再原子替换；不能让 Search、Inspect 和 CandidateProvider 分别看见不同代。

### 4.4 Hybrid 必须是独立深召回，且 fallback 需要单独设计

Ratel 的 Hybrid 是 BM25 和 Dense 各自召回到 depth 100，再用 RRF 融合。Haystack 也先并发取回各路列表，再融合。共同结论是：Dense 只能重排 sparse Top-N 时，无法补回 sparse miss，不是真正的 Hybrid。

Ratel 固定 commit 的 `k1=.9/b=.4`、RRF `k=60`（0-based rank）与 depth=100 都有源码/注释证据；Haystack 私有函数用 `k=61` 表达 1-based 论文公式，数学等价，但 MultiRetriever 没暴露 weights，最终排序也没有 doc-id tie-break。它们证明实现方式，不证明 DWS 中文参数最优；DWS 已测到 sparse arm/depth/k 会改变 R@K 和 workflow，必须自行锁 dev/test。

但两者的错误语义都不能照搬：

- Ratel `Semantic/Hybrid` 直接返回可捕获的 `EmbedderError`；
- Haystack 任一路 retriever 出错会让整个 `MultiRetriever` 失败。

DWS CLI 的离线约束要求更强的本地兜底：

```text
local lexical success + provider success → validate + RRF
local lexical success + provider timeout/error/invalid → local result + stable warning code
runtime Catalog assembly corrupt → fail closed, search unavailable
caller context canceled → return cancellation, not stale fallback
```

这项 fallback 是 DWS 的产品契约，需有自己的测试和 telemetry。

### 4.5 必须给 query、引用、状态和 Schema 设预算

NemoClaw 不只限制 Top-K，还限制：

- model 提供的 query 长度；
- 单次搜索返回的 UTF-8 bytes；
- 每项描述长度；
- checkpoint 中累计工具名数量和 bytes；
- 单工具 Schema bytes；
- 一次模型请求可见的 Schema 总 bytes。

固定 commit 的具体常量是：query 256 chars、search output 8 KiB、单线程 discovered tools 64、discovered state 8 KiB、单工具名 120 bytes、单工具 Schema 16 KiB、已发现 Schema 总量 128 KiB、单次 search results 20、单项 description 256 chars。无匹配返回可行动文本；超预算返回解释并按 UTF-8 边界截断；malformed schema fail-closed。这些是可引用的实现常量，不是 DWS 已验证阈值。

DWS 当前实现已进一步加入 256 rune / 2 KiB query、8 个 subquery、64 KiB request、8 KiB response、候选和摘要边界；超限返回 typed validation，不静默截断。后续 Host 会话层还应增加：

- Inspect 的单 leaf bytes 观测与异常保护；
- Host 累积已发现工具和已 Inspect Schema 的上下文预算。

预算溢出应返回“被截断、请细化 query”的机器可读状态，不能静默截断到一个看似完整的列表。

### 4.6 显式搜索与自动注入是两种不同编排范式

Anthropic/OpenAI/NemoClaw/BigTool 都让模型显式调用 search/retrieve tool；Semantic Kernel 则在每次模型调用前，默认用最近 2 条消息自动构造 query 并注入候选。二者不是协议细节，而是可观测性和成本取舍：

- 显式 Search 多一次 Agent step，但 query、Catalog version、候选与失败语义天然进入 transcript，容易审计、复现和恢复；
- 自动注入减少一次模型 tool call，却会在每轮模型调用前产生隐式检索，必须处理近期工具输出污染、额外延迟、重复检索和“为何出现这个工具”的解释；
- OpenAI `execution="client"` 明确要求手工 orchestration，证明本地 Host 不能假设 SDK 会自动完成 Search→Load；
- Anthropic 本地 substring 示例证明“协议与 ranker 解耦”，不证明 substring 是 DWS 的质量默认。

DWS 的规范选择应是 **显式、versioned Search→Inspect**。Host 可以为了交互体验基于近期用户消息自动触发同一个 Search API，但必须生成等价 SearchTrace，禁止拼入不可信工具输出，并且不得绕过 exact、hard filter、Catalog hash 或预算。这样自动触发只是 orchestration optimization，不成为第二套检索语义。

### 4.7 Namespace 适合作为 hint，不适合作为唯一第一层路由

OpenAI 对 namespace 的 identity、冲突校验和“每组尽量少于 10 个函数”建议很有价值；DWS 已有 product/group/canonical，可以直接生成 reviewed namespace view。但当前 1,098 工具 Catalog 中的跨产品任务和 `product_mismatch` 风险决定了：

- namespace 可以作为已知产品时的 hard filter，或未知产品时的可解释召回特征；
- 不应强制“先选唯一 namespace，再只做组内搜索”，否则 router miss 会造成不可恢复的 leaf recall loss；
- 多动作任务应允许每个 subquery 命中不同 namespace；
- namespace 描述、route Top-K 和组内函数数必须进入独立消融，不能直接套用厂商 `<10` 建议。

## 5. 对 DWS 当前 Spike 的源码审查

当前 [`internal/cli/tool_search.go`](../internal/cli/tool_search.go) 已经具备：

- 从同一 `SchemaRegistry` 构建不可变本地索引；
- Exact Guard；
- product/effect/exclude/executable hard filter；
- canonical 稳定 tie-break；
- 独立 CandidateProvider 召回、候选验证和 RRF；
- provider 错误/非法候选时的本地 fallback；
- source/surface hash；
- 默认 Top-5 轻量 `ToolReference`；
- 多动作 round-robin 合并。

与 GitHub 参考实现对照后发现的差距及当前分支状态如下。

### 5.1 已修复：Provider 版本握手

旧 Provider 接口只返回 `[]string`：

```go
Retrieve(context.Context, string, int) ([]string, error)
```

它可能基于旧 Catalog 返回“当前仍存在但语义已经改变”的 canonical。当前分支已改为与 RFC 的 `ExternalCandidateRanking` 同构，并校验 source/surface hash、provider identity、version、重复/unknown/超限；同进程接口仍不是外部 Host 的公开 Go API：

```go
type ExternalCandidateRanking struct {
    Catalog          CatalogVersionRef
    Provider         string
    ProviderVersion  string
    CanonicalRanking []string
}
```

公开边界是 versioned stdin/stdout JSON：DWS 不访问远端服务，Host 自己处理认证、deadline、egress 与脱敏，再把 ranking 作为不可信输入交给 DWS。hash 不一致、重复、unknown 或超限时整路丢弃并 fallback，不做跨版本融合。

### 5.2 已修复：Exact 命中但被过滤时终止

旧逻辑只有“exact 且 eligible”才直接返回。当前分支已让 exact identity 在 product/effect/exclude/availability 不满足时返回以下 typed 结果，且候选为空，不再进入 fuzzy：

建议返回：

```text
strategy=exact_filtered
candidates=[]
reason=excluded | product_mismatch | effect_mismatch | unavailable
```

若需要替代工具，由上层使用新的自然语言 intent 明确发起第二次搜索。

### 5.3 P0：搜索 hard filter 不是用户 ACL

当前只过滤 Catalog availability、product、effect 和显式 exclude。公开输出必须继续声明：

- `available_in_catalog=true` 不等于当前用户有 DingTalk 权限；
- Host 可以用当前 principal 的 policy 结果缩小 Provider 输入，但 Search 不接收或输出 `authorized`，执行端仍需重新鉴权；
- 不得把搜索结果或 Agent identity 用作授权依据。

### 5.4 已修复：错误 warning 结构化和脱敏

旧 fallback warning 会拼接 `providerErr.Error()`。当前分支只返回稳定 code，原始错误不进入 Agent-visible JSON：

```text
provider_timeout
provider_unavailable
provider_catalog_mismatch
provider_invalid_ranking
provider_internal
```

详细错误进入本地脱敏 trace，不进入 Agent-visible `ToolReference` 响应。

### 5.5 部分修复：资源预算与可观测状态

当前分支已增加 query 256 scalars/2 KiB、最多 8 个 subquery、summary 256 scalars、response 8 KiB、provider deadline，以及 `truncated/abstained/degraded` 和稳定原因码。仍缺少 Host 侧累计 discovered/schema bytes 与只进入 trace 的每路 latency/count：

- query/subquery/response bytes 上限；
- `truncated`、`abstained`、`degraded` 和 `degraded_reason_code`；
- 每路 `retrieved_count`、`accepted_count`、`latency_ms`；
- 仅在 trace 中记录原始分数，Agent-visible score 明确标为 ordering-only，或直接只返回 rank。

### 5.6 已修复：词法实现可替换

当前分支已把词法边界抽成 Go `LexicalRetriever`，并实现 fielded BM25 control 与 fielded TF-IDF cosine shadow；两者共享 tokenizer、hard-filter-before-score、zero-score abstain 和 canonical stable tie-break。默认仍锁 BM25，只有独立 qrels 门禁通过后才能切换。

### 5.7 P1：评测投影与 Go 发布投影尚未完全对齐

当前 Python proxy 为降低同源泄漏而排除了 `use_when`；Go 默认也已翻转为不索引 `use_when`，但 Python 仍没有完整覆盖 Go 的 alias、参数描述、interface description、类型和字段权重。现有 TF-IDF/BM25 R@K 因此仍不能直接代表 Go 发布配置。

必须把以下内容组成显式 `ProjectionVersion`：

- 字段集合和字段权重；
- 是否包含 `use_when`、参数描述和类型；
- tokenizer/normalization 版本；
- zero-score、filter 和 stable tie-break 规则。

Go 与 Python 对同一 Catalog/query/config 的 exact、eligible 集合和 Top-K 应逐 case golden parity；最终算法结论只能来自独立 qrels 上的生产投影。

### 5.8 已修复：BM25 浮点累加的跨进程不确定性

Spike 原先直接遍历 `queryFrequency map[string]int` 累加 BM25 分数。Go map 迭代顺序跨进程不稳定，浮点加法又不满足结合律，近似同分候选可能因 ulp 级差异绕过 canonical tie-break。现已改为每请求只排序一次 query term，再让所有 field/document 按固定顺序计分；测试会启动 3 个独立子进程，对完整 JSON response 做逐字节比较。

这项修复只关闭了本地词法 arm 的已知不确定性。外部 Provider 仍必须自己保证稳定排序、model revision 和 canonical tie-break，DWS 融合后再执行最终稳定排序。

## 6. 推荐的稳定接口

规范 DTO 只保留一套，以 RFC [5.6 CandidateProvider 与融合](rfc-tool-search-progressive-discovery.md#56-candidateprovider-与融合)、[5.7 ToolReference](rfc-tool-search-progressive-discovery.md#57-toolreference) 和 [5.8 Inspect](rfc-tool-search-progressive-discovery.md#58-inspect) 为准。进程内的 `ToolSearchRequest/ToolSearchCandidateProvider` 仍是 non-public SPI；跨进程只允许依赖 `tool-search.v1` JSON。`dws schema search --request-json -` 和双 hash Inspect 已在当前分支实现，是否默认由 Host 使用仍受独立门禁约束。

跨进程稳定链路是：

```text
tool-search.v1 request
  → tool-search.v1 response {CatalogVersionRef, bounded ToolReference[]}
  → schema-inspect.v1 request {canonical, expected source/surface hash}
  → schema-inspect.v1 response {same CatalogVersionRef, ToolSpec}
```

接口约束：

- Search 只返回引用，不返回完整参数 Schema；
- Inspect 必须按 canonical exact 解析，并校验同一 Catalog hash；
- Execute 不接受检索 score，只接受 Inspect 得到的 typed contract；
- Provider 是 Host 管理的独立召回器，不得只能重排本地 Top-N；
- Provider 失败不影响本地路径，但 Catalog/本地索引损坏必须 fail closed；
- 用户权限和 destructive confirmation 不能由 Search 决定。

## 7. 可信执行与安全恢复：GitHub 实现没有替 DWS 解决的部分

开源项目主要优化“模型看到哪些工具”和“如何排序”，没有证明 DingTalk 写操作能安全恢复。规范 `InvocationContractV1`、hash canonicalization、跨进程 evidence 和数据治理以 RFC [6.1 Invocation Contract](rfc-tool-search-progressive-discovery.md#61-invocation-contract) 为准；本报告不再定义第二套简化 DTO。

执行与恢复规则：

1. **执行前再验证。** Inspect hash、当前登录身份、权限、参数约束、confirmation 和 real Cobra leaf 必须同时有效。
2. **错误分类不等于未执行。** timeout/connection reset 可能发生在服务端已受理之后；未知受理状态的非幂等写禁止自动重放。
3. **幂等必须有真实后端能力。** Catalog 标注 `idempotent` 不能凭空制造服务端幂等；只有后端接受且回显 idempotency key 时才自动重试写。
4. **先 Verify 再 Retry。** 创建/发送后优先用 receipt、资源 ID 或查询接口验证；确认未发生才重试。
5. **Compensate 必须逐工具 reviewed。** 只有有明确反向操作、资源 ID、权限和确认契约时才能补偿；否则转人工。
6. **恢复使用原始 Catalog contract。** 若版本已经变化，旧 Invocation 只能按审计/验证处理，不能静默套用新 Schema 重放。

## 8. 推荐实施顺序

### Slice A：搜索内核契约

- `exact_filtered`、Provider Catalog 版本、warning code 脱敏和 query/response/subquery 预算已完成；
- Go `LexicalRetriever`、BM25 control 和 TF-IDF shadow 已完成；
- 公开 transport 已提前实现，但默认启用仍遵循 Slice C/D 门禁。

### Slice B：索引与融合门禁

- TF-IDF、BM25 共享 tokenizer/filters/tie-break；
- provider 独立深召回；
- RRF 参数只在 dev 调整；
- 固定 ProjectionVersion 和 Go/Python parity；
- provider timeout/invalid/stale/duplicate/cancel 测试。

### Slice C：公开 local-only `dws schema search`

- 已实现 Search 的薄 CLI 包装和严格 stdin JSON；
- 已同步 `schema` leaf/exclusion；skills 和 release command-surface 门禁仍需复核；
- 已增加 expected source/surface hash 校验并在 compact Inspect 回传双 hash；
- 不新增顶层 `dws discover`。

### Slice D：Provider 跨进程 shadow/fusion

- Host 负责远端认证、deadline、egress 和脱敏；
- 只提交带 CatalogVersionRef 的 canonical ranking；
- 先 shadow，指标通过后再开放 fusion feature flag；
- provider 失败逐字段退回 local-only response。

### Future Slice E：Invocation 与 recovery（后续独立 RFC，当前不实现）

- 在 `internal/executor.Invocation` 传播 versioned Contract，并由 `runtimeRunner` 在 transport 前验证；
- 建立 receipt/postcondition；
- unknown write 禁止自动重放；
- journal 原子状态迁移、锁、幂等 finalize 和数据保留门禁；
- 为第一批写工具逐个审核 compensation。

## 9. 必须新增的测试

| 边界 | 验收测试 |
|---|---|
| Catalog 同源 | Search/Inspect/Execute canonical 全量 round-trip；source/surface hash 一致 |
| Exact | canonical、primary CLI、unique alias、NFKC、空格；exact filtered 不返回 sibling |
| Determinism | 不同进程/hash seed 下 Top-K 成员和顺序一致 |
| Provider | timeout/error/cancel/duplicate/unknown/stale hash/filtered candidate |
| Budget | query、subquery 数、单 reference、总 response、Inspect Schema 超限 |
| Hybrid | 两路各自深召回；构造 sparse miss 被 provider 补回 |
| Fallback | Provider 故障返回逐字段相同的本地结果和稳定 warning code |
| Security | hidden/disclosed 状态不改变 Runner ACL/confirmation；猜工具名仍被执行层拒绝 |
| Index lifecycle | 新 snapshot 构建失败保留旧 snapshot；版本切换原子化 |
| Future Recovery（独立 RFC） | read、幂等写、非幂等写、unknown acceptance、verify-success、manual handoff |

## 10. 最终选型

DWS 不选择某一个 GitHub 项目作为基础框架，而是组合经过源码验证的模式：

| 能力 | 采用的模式 | DWS 的增强 |
|---|---|---|
| Deferred disclosure 协议 | Anthropic/OpenAI 的 defer → reference → load 形态 | 自建本地加载运行时、typed Catalog、双 hash 和 Inspect；不依赖 SDK 内不存在的本地 ranker |
| Agent 编排 | BigTool 的显式 Search → bind → execute | 显式、versioned Search → Inspect 为规范路径；校验 unknown/stale ID，状态有界，权限仍在执行层 |
| 自动触发（可选） | Semantic Kernel 的 recent-context retrieval | 只作为 Host optimization，复用同一 Search API/Trace；不拼工具输出，失败不拖垮模型调用 |
| Namespace | OpenAI 的 qualified identity / namespace | 作为 reviewed hint 或已知产品 filter；未知、复合任务保留 global/multi-namespace recall |
| 索引生命周期 | Ratel 的完整 BM25 统计、原子 Dense batch/rebuild | Catalog generation 绑定、build-then-swap；不照搬英文 tokenizer/model 或固定 `k1/b/depth` |
| 多路召回与融合 | Ratel/Haystack 的独立深召回 + RRF | 稳定 `(score desc, canonical asc)`；Provider 失败逐字段回到本地结果，这是 DWS 自有契约 |
| 预算/状态 | NemoClaw 的 query/output/state/Schema 多层预算 | 用 DWS 实测冻结阈值，并覆盖 ToolReference、Inspect 与 Host 累积上下文 |
| 字段经验 | ToolUniverse 的按通道字段选择、exact bonus、完整 cache key | 做中文/中英混合字段消融；参数描述保持 shadow，不复用 ASCII tokenizer 与弱 freshness |
| 评测定义 | ToolRet 的 graded qrels 与 Comprehensiveness | 使用 DWS 自有、带测试且锁数据版本的 harness；加入 Forbidden、Identity、Workflow、Agent/recovery 指标 |

最终仍是：

```text
唯一 typed Catalog
  → identity resolve
      ├─ exact → eligibility → exact / exact_filtered（终止）
      └─ not found → Hard Filter
  → 可替换本地轻量召回
  → 可选、带 Catalog 版本的外部补召回
  → 确定性融合与预算
  → ToolReference
  → schema-inspect.v1 hash check
  ── 当前 Tool Search RFC 边界 ──
  ⇢ Future RFC: Cobra Execute → reviewed Verify / Retry / Compensate
```

这套架构的收益不是单纯提高 R@5，而是同时缩小模型上下文、保持 CLI 离线可用、让搜索结果可追溯，并把误重试写操作的风险隔离在明确的执行/恢复契约之外。
