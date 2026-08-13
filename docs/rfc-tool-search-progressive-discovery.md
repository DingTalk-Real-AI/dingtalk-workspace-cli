# RFC：DWS 渐进式工具发现与可信 Inspect

- 状态：Proposed / Spike 已验证，尚未达到发布门禁
- 日期：2026-08-12
- 目标版本：分阶段交付，不绑定单一版本
- 影响范围：`internal/cli`、Agent Host、Schema Search/Inspect
- 关联实现：[`internal/cli/tool_search.go`](../internal/cli/tool_search.go)
- 关联测试：[`internal/cli/tool_search_test.go`](../internal/cli/tool_search_test.go)
- 评测实现：[`scripts/dev/eval_tool_search_ranking.py`](../scripts/dev/eval_tool_search_ranking.py)
- 评测报告：[`tool-search-ranking-evaluation.md`](tool-search-ranking-evaluation.md)
- 背景调研：[`tool-search-ranking-research.md`](tool-search-ranking-research.md)
- GitHub 源码架构调研：[`tool-search-github-architecture-research.md`](tool-search-github-architecture-research.md)

## 1. 摘要

DWS 当前由 Go 声明在运行时装配出 1,098 个工具的 typed Schema Catalog。Agent 可以按产品、分组、leaf 渐进查询，但对自然语言意图仍缺少一个稳定的工具检索入口；复合任务还会遇到只找回部分步骤、相关工具挤占 Top-K，以及结果无法证明来自哪个 Catalog 版本等问题。

本 RFC 决定采用以下架构：

```text
Agent 任务拆解
  → DWS identity resolve
      ├─ exact hit → eligibility check → exact reference / typed exact_filtered
      └─ no hit → Catalog availability / namespace / effect Hard Filter
  → 可替换的轻量词法召回
  → 可选的宿主外部候选（带 Catalog 版本）
  → RRF / 确定性融合
  → contradiction / policy gate
  → 最多 5 个 ToolReference
  → Inspect 完整 ToolSpec
  → 真实 Cobra leaf Execute（沿用既有执行链）
```

关键选型是：

1. **DWS CLI 不内嵌 Dense/Embedding 模型。** 当前 `dws` 二进制约 44 MiB，实验模型 `BAAI/bge-small-zh-v1.5` 约 90 MB，且需要 ONNX runtime、多平台适配和模型生命周期管理。当前 proxy 评测中的收益不足以支持将这些成本变成 CLI 的强依赖。
2. **BM25 不是架构前提。** 词法层定义为可替换接口。Alpha 以 Okapi BM25 作为稳定 control，同时 shadow TF-IDF cosine 和 BM25+；独立 holdout 通过前不宣布任何算法胜出。
3. **Dense 仅作为可选 CandidateProvider。** Agent Host 或已有服务可以提供独立候选集；DWS 验证 canonical、过滤非法候选并与本地词法结果融合。Provider 失败时确定性降级，本地 CLI 仍可离线工作。
4. **Exact、Search、Inspect、Execute 必须分层。** 搜索结果不是可执行 Schema；Agent 必须精确 Inspect 后才能组装参数和执行。
5. **多动作任务先拆解后检索。** 每个动作保留候选预算，再做集合合并和完整性校验，不能把整个工作流只当作一个 query 排一次 Top-5。
6. **相关性不等于安全。** `avoid_when`、effect、确认、权限和幂等约束属于独立 gate；排序分数不得表述为“可信概率”。
7. **显式 Search 是规范，自动触发只是 Host 优化。** Search query、Catalog、候选和失败必须形成可复现 transcript；Host 可基于近期可信用户消息自动调用同一个 API，但不能维护第二套隐式检索语义。

### 1.1 2026-08-13 主线合并后的范围与实现状态

本分支已合并 `upstream/main@5fed80fc`。主线移除了旧 Recovery 实现，并把 Catalog 迁移为“Go 声明即 Catalog”的运行时装配；因此本 RFC 的交付范围同步收敛为 Tool Search 与双 hash Inspect，旧 Sandbox/Recovery 工作仅保存在归档分支 `archive/sandbox-recovery-pre-main-20260813`，不会被恢复到当前分支。安全恢复仍是后续独立 RFC，不是本次 Search 发布能力。

Catalog 的规范数据是 Go `ProductDecl` / `ContractDecl` 和 `ResolveSchemaBuild` 的结构化结果。仓库不提交 Catalog JSON，也不从 JSON 加载生产索引；只有评测/CI 会调用 `cmd_schema_catalog`，把分片临时生成到被忽略的 `.worktrees/policy-tmp/tool-search-schema-catalog`。生成器、运行时 Search、评测聚合与可信度校验的核心框架均为 Go。

当前 Implemented (unreleased)：

- `dws schema search` 的 exact / `exact_filtered`、hard filter、Go BM25/TF-IDF、动作重排、稳定 RRF 和资源预算；
- `ToolReference → schema Inspect` 的 source/surface 双 hash 闭环与 typed `catalog_changed`；
- 可选 CandidateProvider 的版本握手、超时、非法候选校验、脱敏 warning 与本地确定性降级；
- 由运行时声明直接生成的 Go 诊断对比、中文切片、workflow、负例暴露、身份与完整性指标。

当前仍未通过：独立 qrels、真实模型配对 Agent A/B、英文切片、线上 contradiction gate 与 cold subprocess SLA。因此状态仍是 Proposed，不得把同源诊断指标表述成线上任务成功率。

### 1.2 规范用语与能力标签

- **MUST / 必须**：进入对应阶段的发布门禁；未满足不得声称具备该能力。
- **SHOULD / 应**：推荐实现；偏离必须记录理由和替代风险控制。
- **Current Spike**：当前分支已有实验实现，不代表公开兼容承诺。
- **Target**：本 RFC 选择的目标契约，可能尚待实现或门禁验证。
- **Implemented (unreleased)**：当前分支已有代码和定向测试，但尚未通过独立数据与发布门禁。
- **Non-goal**：本 RFC 明确不承担的能力。

## 2. 背景与问题

### 2.1 当前已有能力

DWS 已经具备：

- reviewed `CommandRegistry` 作为稳定 identity/navigation 的唯一事实源；
- 从同一 `ToolSpec` 派生的 `SchemaRegistry`、`SchemaIndex`、Catalog 和 `schema --all`；
- canonical path、primary CLI path 和 reviewed alias 的精确解析；
- 参数、约束、effect、risk、confirmation、idempotency、interface availability 和 Agent selection metadata；
- 真实 Cobra leaf 的业务校验、确认、dry-run、审计和 transport 执行链路。

这些能力意味着工具检索不应该创建第二份 Catalog，也不应该通过运行时 `tools/list` 重新发现命令。搜索必须消费发布后二进制中已经验证过的 typed Registry。

### 2.2 需要解决的三个目标

1. **让 Agent 找得到。** 自然语言、中文混合参数名、canonical/CLI path 和复合工作流都能找到正确且完整的工具集合。
2. **让返回结果可信。** 能证明候选来自哪个 Catalog，经过什么过滤和排序；选中后使用完整 Schema，而不是根据摘要猜参数。
3. **保持执行边界安全。** Search 不授权、不执行、不自动重放；执行失败恢复由后续独立 RFC 与主线执行架构负责。

### 2.3 非目标

- 不在本 RFC 中实现一个新的生成式 LLM。
- 不让搜索命令直接执行工具。
- 不用检索分数替代权限鉴定、用户确认或业务校验。
- 不承诺跨进程事务或任意工具的自动补偿。
- 不把当前同源 proxy 数据集当作正式生产发布门禁。
- 不在本分支恢复或重建已被主线删除的旧 Recovery/Sandbox 框架。

## 3. 整体收益

### 3.1 Agent 效果收益

合并主线后的 Go shipped-runtime proxy 集包含 1,123 条预算内意图、1,390 条负例、2,196 条 identity 和 10 条工作流；另有 10 条 selection 文案因超过生产 query 预算被显式计数并排除：

| 能力 | 当前证据 | 可支持的结论 |
|---|---:|---|
| Go 默认动作重排 BM25 | R@1 65.63%；R@5 86.38%；MRR@5 0.7417 | 当前本地默认候选，尚非独立发布结论 |
| Go 原始字段 BM25 control | R@1 65.09%；R@5 86.64%；MRR@5 0.7370 | 默认动作重排的 R@5 低 0.27 pp，但 R@1/MRR@5、workflow 与负例暴露更好 |
| 精确身份 | Exact Guard 后 canonical / CLI Top-1 100% | Exact Guard 是穷举契约门禁 |
| 多工具完整性 | 整句 Complete@5 40% / required recall 66.67%；reviewed 拆解后 90% / 95% | 动作级检索有效；拆解结果是人工上界，不是端到端成功率 |
| 中文召回 | 默认算法纯中文 R@5 81.59%，中英混合 89.04% | 中文 bigram + 标识符保留可用，但纯中文仍是重点优化项 |
| 负例暴露 | Forbidden@1 5.83%，Forbidden@5 36.83% | Top-1 已较低，Top-5 仍高，必须继续建设 alternative gold / contradiction gate |

相对最简单的 IDF keyword overlap，TF-IDF 在当前 proxy 集上的 R@5 从 76.91% 提升到 84.55%，提高 7.64 个百分点。这个数字可以说明“需要一个正式词法 ranker”，但因为 query 与索引元数据来自同一批作者，不能外推为线上业务收益。

当前分支的纯 Go shipped-runtime 对比器直接消费运行时装配的 typed Catalog，不读取仓库中的生成 JSON。当前默认 `fielded_bm25_action_v1` 相对原始 `fielded_bm25_ensemble`：R@1 **+0.53 pp**、R@5 **-0.27 pp**、MRR@5 **+0.47 pp**；raw workflow required recall **+5 pp**，reviewed 拆解的 Complete@5 / required recall **+40 / +20 pp**；Forbidden@5 **下降 0.72 pp**。这支持继续 shadow 当前动作重排，但仍受同源 query 和人工拆解限制，不能替代独立 test 或真实 Agent A/B。

合并主线前曾用固定 `gpt-5.6-sol` 做过一轮 answer-free、无业务执行的规划 smoke A/B：同一批 10 条 workflow，各 arm 一个 batch/一次 trial。该 run 绑定旧 572 工具 Catalog，已因当前 1,098 工具 surface 变化判为过期，只保留方法记录，不能作为现行效果证明。旧结果中两臂 required-tool complete/recall 都是 **100% / 100%**；精确最小计划率分别为 **90% / 70%**，plan precision 为 **95.45% / 87.50%**，额外步骤为 **1 / 3**。单 trial 无区间，不能据此通过默认开启门禁。

### 3.2 上下文与 Token 收益

本机对当前 Catalog 的实测：

| 载荷 | 大小 |
|---|---:|
| compact 全量 Schema | 17,851,126 bytes |
| 平均 Search + gold Inspect | 4,486.87 bytes |
| 理想化 overview + 正确 product + Inspect | 122,193 bytes |

渐进发现把“预加载全部 Schema”变成“约 3 KB 候选 + 选中 leaf 的数 KB Schema”。实际模型 token 下降比例取决于 tokenizer 和对话编排，因此 RFC 只以字节数作为已验证证据，不给出未经测量的 token 节省承诺。

用 Go 对比器逐条渲染当前 compact JSON envelope 后，Search + gold leaf Inspect 平均 **4,486.87 bytes**；相对 17,851,126 bytes 的全量 Schema 减少 **99.9749%**，相对已经假设 oracle 知道正确 product 的理想化导航也减少 **96.3297%**。评测器直接 Inspect gold，即使 Search miss 也不计额外尝试；因此这是 JSON byte 容量上界，不是 tokenizer token、真实 Agent 成本或任务成功率承诺。

### 3.3 CLI 与工程收益

- 本地轻量检索不增加第三方 Go 依赖，不要求网络、模型下载、GPU 或个人凭据。
- 继续复用同一个 `SchemaRegistry`，避免 Catalog、CLI 和 Agent metadata 出现双事实源。
- CandidateProvider 是可选项；外部服务超时、离线或返回非法 canonical 时，本地结果仍然可用。
- ToolReference 携带 Catalog source/surface hash，便于审计搜索和 Inspect 是否基于同一版本。
- 检索器、provider、policy gate 和 executor 分层后，可以独立评测、灰度和回退。

### 3.4 性能成本与当前缺口

纯 Go Spike 在 Apple M3 Pro、572 个工具、中文复合 query 上的 warm benchmark：

| 版本 | 单 query | 分配 |
|---|---:|---:|
| 初版 | 2.04～2.82 ms | 约 1.42 MB / 9,625 allocs |
| 复用 query 词频后 | 1.31～1.55 ms | 约 115 KB / 1,048 allocs |

2026-08-13 在历史 572 工具 Spike 上运行三轮 benchmark，warm Search 为 **0.556～0.574 ms**、约 **86 KB / 1,123 allocs**；当时的 engine build 为 **48.70～48.82 ms**、约 **28.5 MB / 244k allocs**。后者仍提示短生命周期 subprocess 的主要本地成本不能被 warm p95 掩盖；它不是当前声明装配版的 SLA 数值。

这组历史数据证明全量扫描在当时 572 工具规模下可用，也暴露了初始化和分配仍需优化；当前 1,098 工具声明装配版必须重新建立 SLA。正式门禁必须以 release binary 独立进程测量 cold start，并以长生命周期进程测量多次 warm 请求。Host 不应为了 local-only + fusion 无条件启动两个短命进程；优先支持一次 versioned fusion request，只有真实端到端数据超预算后才评估 daemon/sidecar。

### 3.5 业务闭环收益

预期收益链路是：

```text
更高的工具集合召回
  → 更少的“找错命令 / 漏步骤”
  → Inspect 后更少的参数猜测
  → effect / confirmation / idempotency 约束进入执行
  → 更少的重复写、静默失败和人工排查
```

这些是合理的因果假设，尚未由真实 Agent 任务成功率证明。上线评测必须把最终指标从 Retrieval 扩展到：工具选择成功率、参数一次通过率、任务完成率、错误写入率、人工接管率和恢复成功率。

## 4. 决策过程与证据修正

本节保留方案形成过程，避免只记录最终答案而丢失约束来源。

### 4.1 阶段一：从评测报告提炼三个目标

初始问题被归纳为：Agent 能发现、结果可信、失败可恢复。Catalog 已经提供 selection/safety/interface 元数据，因此第一版方向是 Catalog → Search → Inspect → Execute。

### 4.2 阶段二：对照业内渐进发现

- Anthropic Tool Search 协议支持 Regex/BM25 变体和 `tool_reference`；普通工具也可 `defer_loading`。固定 commit 的 SDK 只有 OpenAPI 类型、dispatch 簿记和一个客户端 substring 示例，托管 BM25/Regex 及定义注入均无开源实现。[Anthropic Tool Search](https://platform.claude.com/docs/en/agents-and-tools/tool-use/tool-search-tool)
- OpenAI Agents SDK 的 `ToolSearchTool()` 支持 deferred functions、namespace 和 hosted MCP tools，并实现 Responses-only 配置校验、qualified identity 与 approval key；`execution="client"` 明确不会由标准 Runner 自动执行。[OpenAI Agents SDK Tools](https://openai.github.io/openai-agents-python/tools/)
- MCP 客户端最佳实践把 progressive discovery 描述为 `search_tools` 返回轻量名称/描述，再按需加载完整工具。[MCP Client Best Practices](https://modelcontextprotocol.io/docs/develop/clients/client-best-practices)

由此确定 DWS 不需要发明新的协议形态，但检索实现、ToolReference→Inspect 定义加载、会话可见工具管理和 client orchestration 都必须自研，不能把任一 SDK 当作本地运行时依赖。

### 4.3 阶段三：Dense/Hybrid 实测

FastEmbed + `BAAI/bge-small-zh-v1.5` 的 proxy 结果显示：

- Dense 单独 R@5 82.06%，没有超过 BM25 的 83.06%；
- 字段 BM25 集成 + Dense RRF 为 85.88%；
- 相对字段 BM25 集成的 case-level paired bootstrap 为 +3.65 pp，95% CI `[+1.33, +6.31]`；
- product-cluster bootstrap 约为 `[-0.40, +7.03]`，跨 0，增益不能表述为跨产品稳定；
- Hybrid 会损伤精确 identity，必须由 Exact Guard 隔离；
- 20/572 个工具文本超过 Dense 的单段 512 token 范围，Dense 在这些工具上的召回更差，需要分字段/分块。

因此保留“外部可选语义补召回”，否决“纯 Dense 替换词法检索”。

### 4.4 阶段四：CLI 体积约束推翻内嵌模型

用户指出 DWS 是 CLI，90 MB 左右的模型过重。进一步检查确认：当前二进制约 44 MiB；内嵌模型还会引入 runtime、多平台构建和模型升级成本。当前 proxy 收益无法覆盖这些成本。

决策从“CLI 内建 Hybrid”修正为：

```text
DWS 本地 Exact + 轻量词法
  + Agent Host 可选 CandidateProvider
```

### 4.5 阶段五：BM25 假设被重新打开

BM25 是强 baseline，但并非协议要求。三路复核使用相同 Catalog、query、tokenizer 和 stable tie-break 对比多种零模型算法。

本分支可复现评测使用统一 `k1=0.9, b=0.4, delta=1` 时：

| 方法 | R@1 | R@5 | MRR | Forbidden@5 ↓ | Workflow Complete@5 |
|---|---:|---:|---:|---:|---:|
| IDF keyword overlap | 47.51% | 76.91% | 0.6017 | 44.27% | 20% |
| Weighted Jaccard | 44.85% | 74.58% | 0.5867 | 39.63% | 30% |
| BM25L | 52.16% | 80.90% | 0.6471 | 50.24% | 20% |
| BM25+ | 53.49% | 81.23% | 0.6578 | 51.46% | 20% |
| Okapi BM25 | 56.64% | 83.06% | 0.6821 | 54.02% | 20% |
| 字段 BM25 分数集成 | 54.49% | 82.23% | 0.6690 | 55.00% | 40% |
| TF-IDF cosine | **57.14%** | **84.55%** | **0.6956** | 54.02% | 30% |

独立复测用 BM25+/BM25L 的文献常见默认参数也显示参数敏感；BM25+ 可到 R@5 83.55%，BM25L 仍明显落后。TF-IDF 相对 BM25 的 +1.50 pp case-level paired bootstrap 95% CI 为 `[-0.50, +3.65]`，不能判定胜出。按 22 个产品等权聚类后，TF-IDF 相对 BM25 的 macro-product 差值反而为 -3.55 pp，95% CI `[-13.42, +1.82]`；说明微平均增益集中于大产品，不能直接设成全局默认。

因此最终架构不锁定 BM25。Alpha 先以 BM25 作为 control，TF-IDF cosine 和 BM25+ 同时 shadow；独立 holdout 之前不得删除任何候选。

### 4.6 阶段六：中文专项

602 条自然语言意图全部包含中文，其中：

- 纯中文 283 条；
- 中文混合 ASCII、产品词、ID 或参数名 319 条；
- identity 另有 1,144 条 canonical / CLI path 用例。

| 方法 | 纯中文 R@1 / R@5 / MRR | 中文混 ASCII R@1 / R@5 / MRR |
|---|---|---|
| BM25 | 54.06% / 81.98% / 0.6630 | 58.93% / 84.01% / 0.6991 |
| 字段 BM25 分数集成 | 53.36% / 82.33% / 0.6628 | 55.49% / 82.13% / 0.6746 |
| TF-IDF | 54.06% / **83.04%** / 0.6715 | **59.87%** / **85.89%** / **0.7170** |
| BM25+ | 54.06% / 81.98% / 0.6621 | 58.62% / 84.01% / 0.6975 |

Tokenizer 对照：

| CJK tokenizer | 总体 R@1 / R@5 / MRR | 纯中文 R@1 / R@5 / MRR | Workflow required recall |
|---|---|---|---:|
| unigram | 50.33% / 75.25% / 0.6199 | 50.18% / 73.14% / 0.6052 | 46.67% |
| bigram | **56.64% / 83.06% / 0.6821** | 54.06% / **81.98%** / 0.6630 | 46.67% |
| unigram + bigram | 54.82% / 81.23% / 0.6703 | **55.83%** / 80.21% / **0.6739** | **51.67%** |

当前 Spike 使用 CJK bigram，同时保留完整 canonical/CLI identifier，并拆分 dot、snake_case、kebab-case 和 camelCase。它是当前 proxy 上的可复现候选，不是已确定的生产最优 tokenizer；`unigram + bigram` 可作为多动作纯中文的实验第二路，最终默认待独立配对中文 test 决定。

### 4.7 阶段七：多 Agent 独立审计

本轮使用三个独立 Agent：

| Agent | 职责 | 改变方案的发现 |
|---|---|---|
| Architecture Audit | 定位接入层、执行与恢复边界 | 搜索应与 `SchemaIndex` 并列；先内核后 `schema search`；不新增 `discover` |
| Evaluation Audit | 审指标、泄漏、统计与结论强度 | qrels 同源；product-cluster CI 跨 0；当前不是严格 BM25F；门禁尚未验证 |
| Benchmark Retest | 两次复跑、参数、算法和中文切片 | TF-IDF 微平均较高但产品分布不稳；bigram 最稳；Exact Guard 不可移除；结果可复现 |

Architecture Audit 还发现并推动修复了 Spike 中的四个问题：

1. 参数索引遗漏 `Description` 和 `InterfaceDescription`；
2. 旧 reranker 只能重排 BM25 候选，不能召回 sparse miss；
3. 多动作搜索将 CandidateLimit 错赋给最大 20 的 Limit；
4. response 只带 SourceHash，缺少 SurfaceHash。

### 4.8 阶段八：GitHub 固定 commit 源码调研

本轮进一步核验 Anthropic SDK、OpenAI Agents SDK、NVIDIA NemoClaw、LangGraph BigTool、Semantic Kernel、Ratel、ToolUniverse、Haystack 和 ToolRet 的固定 commit。完整证据见 [`tool-search-github-architecture-research.md`](tool-search-github-architecture-research.md)。

源码调研改变或强化了以下决策：

1. **Progressive disclosure 不是授权。** NemoClaw 明确保留完整 executor registry；隐藏工具名如果被猜中仍可能进入执行器。因此搜索/披露只优化上下文，ACL、确认、凭据和 sandbox 必须在执行层再次生效。
2. **Provider 必须带 Catalog 版本。** 只返回 canonical 无法识别“名称仍存在但语义已变化”的旧索引；Provider request/response 都必须绑定 source/surface hash，版本不一致时整路丢弃并走本地 fallback。
3. **Exact 被过滤时不得 fuzzy fallback。** 明确 canonical/CLI identity 命中但因 product/effect/exclude/availability 不可用，应返回 typed `exact_filtered`，不能推荐一个相似 sibling。
4. **索引更新必须按完整 snapshot 原子切换。** Ratel 在固定 commit 中实现了 BM25 corpus 变化后重建统计、Dense batch 先校验 model fingerprint/维度再提交；DWS 静态 Catalog 可在构造时一次完成，未来热更新选择更强的 build-then-swap 契约。
5. **资源预算不能只有 Top-K。** 公开接口还要限制 query、subquery 数、引用/响应 bytes、单 leaf Schema bytes 和 Host 累积已发现工具状态。
6. **外部项目没有替 DWS 实现自动降级。** Ratel Dense/Hybrid 和 Haystack 子检索失败都会抛错；DWS 的 provider 失败返回本地确定性结果是自己的契约，必须独立测试。
7. **参数描述应按 arm 消融。** ToolUniverse keyword 的 docstring 直接证明作者因英文生物医学模板噪声只索引参数名，但其 embedding arm 又编码完整 JSON；仓库无消融，ASCII tokenizer 对中文零召回。它证明“字段策略可按通道不同”和作者设计理由，不证明中文 DWS 应排除参数描述。DWS 续测中加入参数描述仅 +0.50～0.66 pp 且 CI 跨 0，因此保持 shadow。
8. **生产投影若包含 `use_when` 会在当前 proxy 上泄漏。** 602 条 intent 正来自 `use_when`；续测含该字段时 TF-IDF/BM25 R@5 均为 100%。因此当前分支已把 `IncludeUseWhen` 默认翻转为 `false`，含该字段的结果只能标记为 leakage upper bound，不能进入选型。
9. **TF-IDF+Dense 是 shadow 候选。** 默认 RRF 的 proxy R@5 为 87.87%，dev sweep 最好 88.70%，高于字段 BM25+Dense；但 product-cluster CI 仍跨 0，且参数在同一 proxy 上选择，不改变“Dense 外置、独立 test 后定默认”的结论。
10. **ToolRet 只提供指标/数据方法论。** `Comprehensiveness@K = mean(Recall@K == 1)`、分级 qrels、instruction/no-instruction 和固定一阶段候选值得采用；固定 commit 的 BM25/ColBERT 路径存在直接崩溃/错误 qrels 等缺陷、无 tests、数据集未锁 revision 且全英文，不能复制 harness 或用于中文结论。

### 4.9 阶段九：五个独立 Reviewer 复核

按本轮要求另外启动了 5 个只读 reviewer，职责不重叠；RFC author 只在汇总阶段改文档：

| Reviewer | 审查边界 | 纳入 RFC 的关键修正 |
|---|---|---|
| GitHub Architecture | 固定 commit 与 DWS API 映射 | `exact_filtered` 终止；Provider 版本/身份/timeout 边界；补齐 Invocation identity/evidence |
| Retrieval | Ratel、ToolUniverse、Haystack、ToolRet 源码 | 纠正“自动降级”“中文支持”“缓存新鲜度”和 benchmark 可复用性过度表述 |
| Recovery | Runner、SafetySpec、journal、probe | 发现成功状态过度推断、311/322 写工具幂等未知、journal 非原子与 Verify 缺口 |
| Evaluation v2 | 泄漏、统计、中文/算法公平性 | 要求 ProjectionVersion、Go/Python parity、cluster CI 和预注册门禁 |
| RFC Delivery | 协议可实施性与文档一致性 | 明确 Host/CLI stdin/stdout JSON、Search→Inspect hash closure、Contract owner/transport、唯一 normative DTO |

五方共同否决了“当前 Spike 已可直接公开”的结论：定向单测通过只证明实验内核可运行，不代表 Host 边界、Inspect 版本闭环或安全恢复已经实现。因此 RFC 状态保持 Proposed，实施顺序改为“内核 → local-only Search/Inspect → Provider shadow/fusion → managed execution/recovery”。

### 4.10 阶段十：两种 Agent 编排范式复核

补充源码核验区分了两类形态：Anthropic/OpenAI/NemoClaw/BigTool 的模型显式 search，以及 Semantic Kernel 在模型调用前用近期消息自动注入。DWS 选择显式 `tool-search.v1 → schema-inspect.v1` 作为唯一规范路径，理由是 version handshake、失败语义和 SearchTrace 可直接审计。Host MAY 用最近可信用户消息自动触发该路径以省掉模型的一次 search decision，但：

- 自动触发必须生成与显式调用相同的 request/response/trace；
- 默认只使用本轮与最近 1～2 条用户/assistant planning 消息，不拼接工具输出；
- 自动触发失败必须回到显式 Search 或 local-only 结果，不能让模型调用整体 fail-fast；
- 是否降低 tool-call/token/latency 必须在 A/B 中验证，不能因 Semantic Kernel 有实现就默认开启。

OpenAI namespace 被采纳为 reviewed routing hint 和 identity 组织方式，不采纳“必须先选唯一 namespace”的两阶段硬路由。已知 product 可作为 hard filter；未知或复合任务仍做 global/multi-namespace recall，避免 namespace router miss 变成不可恢复的 leaf miss。

## 5. 方案设计

### 5.1 数据源与分层

```text
reviewed CommandRegistry + Cobra + reviewed metadata
                         ↓ generation
                  typed SchemaRegistry
                    ├─ SchemaIndex: Exact / Inspect
                    └─ SearchIndex: Lexical retrieval
                                      ↑ optional
                         untrusted ExternalRanking
```

搜索只能读取最终、已验证的 `SchemaRegistry`。不得：

- 读取旧 Catalog 反向补齐 reviewed inputs；
- 修改 `SchemaIndex.Resolve` 的 exact 语义；
- 运行时调用 MCP `tools/list` 生成第二份工具目录；
- 直接根据 `interface_ref.rpc_name` 执行工具。

最后一条很重要：ToolSpec 支持 MCP、local 和 composite。直接调用 RPC 会绕过 Cobra 参数转换、helper 编排、确认、本地文件步骤和业务校验。

### 5.2 Exact Guard

Exact Guard 使用现有 `SchemaIndex.ResolveQuery`：

- canonical path；
- primary CLI path；
- reviewed、唯一的 CLI alias；
- NFKC、首尾空格和既有 CLI path 归一化。

只有唯一 identity 命中才能绕过排序。规范顺序是：

```text
resolve identity
  ├─ not found → 对自然语言候选做 Hard Filter + Search
  ├─ found + eligible → 返回唯一 exact reference
  └─ found + ineligible → 返回 typed exact_filtered，候选为空，禁止 fuzzy fallback
```

不得把 `name`、`group`、`product_id` 等非唯一文本作为 exact：当前 Catalog 中存在大量冲突，例如同一产品名可命中几十到上百个工具。`exact_filtered` 必须带稳定 reason code：`excluded`、`product_mismatch`、`effect_mismatch` 或 `unavailable`。

### 5.3 Hard Filter

本地强过滤顺序：

1. `Interface.AgentExecutable()`；
2. 请求指定的 product/namespace；
3. 请求指定的 effect；
4. 调用方明确排除的 canonical；

本地 Catalog availability 不等价于用户真实业务权限。Search v1 不接收“已授权”布尔值，也不输出 authorized。Host 可以基于自己的同版本 Catalog view 与 policy 结果缩小外部 Provider 的检索域，但该集合不能扩大 DWS 本地 eligible 集合，也不能作为执行授权。用户级 ACL/policy 必须由执行端基于当前 principal/tenant 和 policy revision 重新验证。

### 5.4 LexicalRetriever

当前 Go 内核已抽象为：

```go
type LexicalRetriever interface {
    Name() string
    Retrieve(context.Context, ToolSearchLexicalRequest) ([]LexicalHit, error)
}
```

当前实现：

- `fielded_tfidf_cosine` shadow；
- `fielded_bm25_ensemble` control/default；
- 可选 `bm25_plus` shadow。

所有方法共用：

- 完全相同的文档字段；
- 完全相同的 tokenizer；
- zero-score abstain；
- canonical 升序 stable tie-break；
- Exact Guard 和 hard filter。

Go BM25 的 query terms MUST 每请求排序一次，再以固定顺序累加各文档分数。Current Spike 原先遍历 map 做浮点加法，会因跨进程迭代顺序不同产生 ulp 级漂移；该缺陷已修复并用 3 个独立子进程逐字节 golden 回归。任何新 ranker 也必须证明同等确定性。

字段实验必须与算法实验正交。当前所谓 `bm25_field_weighted` 是“各字段独立 BM25 后加权求和”，RFC 正式名称为 **fielded BM25 score ensemble**，不能称为严格 BM25F。

### 5.5 中文 tokenizer

索引和 query 使用同一规则：

1. Unicode NFKC；
2. 保留完整 canonical/CLI identifier；
3. dot、snake_case、kebab-case、camelCase 拆分；
4. ASCII lower-case；
5. 连续 CJK 默认生成 bigram；
6. 不依赖系统词典或在线分词服务。

这个方案对 `baseId`、`openConversationId`、`chat.send_personal_message` 和中文意图同时保留信号，并维持 CLI 零模型、跨平台约束。

`ProjectionVersion` 至少预注册两套互斥字段策略：`production_without_use_when` 与 `production_with_use_when`。前者作为无泄漏 control；后者只能在 query 由独立作者、且不来自 Catalog selection metadata 的 dev/test 上比较。当前 proxy 中 `with_use_when` 的 100% R@5 永远不得成为发布证据。

### 5.6 CandidateProvider 与融合

**Target public boundary** 使用 stdin/stdout JSON，而不是要求外部 Host import `internal/cli`。DWS 搜索命令本身不访问远端 Provider；Agent Host 负责 provider 认证、租户隔离、egress policy、脱敏和 deadline，再把结果作为不可信输入提交给 DWS 校验/融合：

```text
Host ── dws schema search(local-only) ──→ local lexical ranking + CatalogVersionRef
  └── optional remote provider ─────────→ versioned canonical ranking
                    external ranking ──→ dws schema search(fusion)
                                           ↓ validate + RRF + ToolReference
```

规范 v1 DTO：

```go
type CatalogVersionRef struct {
    SourceHash  string `json:"source_hash"`
    SurfaceHash string `json:"surface_hash"`
}

type ExternalCandidateRanking struct {
    Catalog          CatalogVersionRef `json:"catalog"`
    Provider         string            `json:"provider"`
    ProviderVersion  string            `json:"provider_version"`
    CanonicalRanking []string          `json:"canonical_ranking"`
}

type ToolSearchV1Request struct {
    Version          string                    `json:"version"`
    Query            string                    `json:"query"`
    Subqueries       []string                  `json:"subqueries,omitempty"`
    Limit            int                       `json:"limit"`
    CandidateLimit   int                       `json:"candidate_limit"`
    ProductIDs       []string                  `json:"product_ids,omitempty"`
    Effects          []string                  `json:"effects,omitempty"`
    ExcludeCanonical []string                  `json:"exclude_canonical,omitempty"`
    ExternalRanking  *ExternalCandidateRanking `json:"external_ranking,omitempty"`
}
```

CLI transport 固定为 `dws schema search --request-json -`：stdin 只接受一个有总 bytes 上限的 `ToolSearchV1Request`，stdout 只写一个 `tool-search.v1` JSON envelope，诊断写 stderr。面向人的 `--query/--limit` flags 只允许 local-only 搜索，不能用 flags 或环境变量注入 ExternalRanking。第一次 local-only 与第二次 fusion 请求都重新计算本地排名；相同 Catalog/query/config 必须逐字段一致。

当前分支的 `ToolSearchCandidateProvider` 已复用带双 hash、provider identity 和版本的 `ExternalCandidateRanking`，但它仍只是同进程测试/SPI，不是公开 Host 协议。公开 Host 边界是 `dws schema search --request-json -` 的 stdin/stdout JSON；远端调用仍由 Host 管理。

Provider 可以由 Agent Host 的 LLM、Embedding 服务或其他召回器实现。它必须在同一 Catalog snapshot 上独立深召回，而不是只能重排本地 Top-N；否则无法补回 lexical miss。Host 应在送往 Provider 前应用 product/effect/availability 过滤，并绑定第一次 local-only Search 返回的 Catalog version；DWS 融合前仍重新解析 canonical 并执行相同 hard filter，不能信任 Host 已过滤。用户 ACL 只在 Execute 前校验，不进入 Provider 排名契约。

DWS 必须验证：

- canonical 存在于当前 Catalog；
- 没有空值和重复值；
- 候选通过相同 hard filter；
- 数量不超过 CandidateLimit；
- Provider 返回的 Catalog hash 与本地完全一致；
- ExternalRanking 的 provider/version 非空且 canonical 数量在预算内。

远端 deadline、认证、租户隔离、query 脱敏和 egress policy 由 Host 承担；DWS 不发起远端请求。Host 在 timeout/cancel/unavailable 时省略 `external_ranking`，因此 DWS 无需理解 Provider 的私有错误。

有效候选与本地排名用 RRF 融合。`k`、depth 和权重都是实验参数，不写入稳定协议。异常语义固定为：Host 未提供 ranking、provider timeout/unavailable 时提交无 external ranking；DWS 返回逐字段相同的本地结果；external ranking 只要出现重复、unknown canonical、版本不一致或超限，整路丢弃，不做“保留部分合法项”，并返回稳定 warning code。被 product/effect/exclude/availability 过滤的合法 canonical 可以单项丢弃并计数，不视为协议损坏。原始 Provider 错误只进入 Host 脱敏 trace，不进入 Agent 可见文本。

Current Spike 默认 `CandidateLimit=20`，而续测在 572 工具上比较了 depth 20/50/100：TF-IDF+Dense 的 R@5 在被测组合中相差最多约 0.83 pp，最佳 proxy 配置出现在 depth=100,k=10；字段 BM25 的 workflow 指标却对 depth 不敏感。结论不是“必须改成 100”，而是 CandidateLimit 必须作为 dev 调参后锁定的 Projection/SearchConfig，test 只运行一次；公开默认不得凭参考项目常量或当前 proxy best 决定。

### 5.7 ToolReference

搜索默认返回最多 5 个轻量引用：

```json
{
  "canonical_path": "chat.query_msg_read_status",
  "primary_cli_path": "chat query-msg-read-status",
  "product_id": "chat",
  "title": "查询群消息已读状态",
  "agent_summary": "...",
  "effect": "read",
  "risk": "low",
  "confirmation": "none",
  "idempotency": "idempotent",
  "rank": 1,
  "matched_fields": ["summary", "parameters"],
  "rank_sources": ["tfidf_cosine", "provider"],
  "requires_inspect": true
}
```

Response 同时返回：

- `version: "tool-search.v1"`；
- `catalog: {source_hash, surface_hash}`；
- `strategy`；
- `abstained / truncated / degraded`；
- `warning_codes`；
- 多动作时的 `subqueries`。

这是规范 v1 ToolReference。完整 `use_when/avoid_when` 和参数 Schema 只在 Inspect 返回，避免轻量引用膨胀；contradiction gate 仍可在 DWS 内部消费它们。公开模型接口只返回 `rank`、`matched_fields` 和 `rank_sources`，不返回 raw `score/sparse_score`。当前分支已经从 JSON DTO 移除 raw score、自由文本 warning 和完整 use/avoid，只保留进程内未导出的排序分数。

当前分支已限制 query、subquery 数、summary 和总响应 bytes，并返回 `truncated/abstained/degraded` 与稳定原因码。尚未实现的预算位于 Host：累积已发现工具、单个 compact Inspect 和任务内可见 Schema 总量；这些状态必须按任务/步骤设置上限和失效，不能像 BigTool 一样在长对话中无界累积。

NemoClaw 固定 commit 提供了 query 256 chars、search output/state 各 8 KiB、64 discovered tools、120-byte name、16 KiB single schema、128 KiB visible schemas、20 results、256-char description 等工程参考。DWS 不直接复制阈值；结合当前 5 references 约 3.2 KB、full Catalog 单工具对象 median 约 16.9 KB / max 约 69.6 KB，预注册以下 `SearchBudgetV1` shadow defaults：

| 预算 | Proposed default | 超限语义 |
|---|---:|---|
| query | 256 Unicode scalars 且 UTF-8 ≤ 2 KiB | `QUERY_TOO_LARGE`，不截断后搜索 |
| subqueries | 最多 8；每条同 query 限制 | `TOO_MANY_SUBQUERIES` |
| public references | 默认 5；多动作显式请求最多 10 | `truncated=true` + 可继续分页/细化 |
| internal/external candidates | 每 arm 最多 100 | external 超限整路拒绝；local 稳定截断 |
| 单个 summary | 256 Unicode scalars | UTF-8 边界截断并标记字段 truncated |
| search response | UTF-8 ≤ 8 KiB | 保留完整 reference 边界，`RESPONSE_BUDGET_EXCEEDED` |
| task discovered tools | 最多 64，状态 JSON ≤ 8 KiB | 停止累积并要求新 task/细化 |
| 单个 compact Inspect | 16 KiB soft warning、64 KiB hard limit | soft telemetry；hard `SCHEMA_TOO_LARGE`，禁止半 Schema |
| task visible Inspect schemas | 总计 ≤ 128 KiB | Host 必须逐出或开始新 task，不能静默超限 |

这些数值进入 dev/shadow 后才可冻结。尤其 single Inspect 必须先对真实 `--compact` 全量审计；full provenance 对象不能拿来误判模型可见 Schema。任何截断都不能产生可被当作完整参数契约的半个 ToolSpec。

### 5.8 Inspect

兼容路径继续复用：

```bash
dws schema chat.query_msg_read_status --compact
```

当前实现已经支持 expected Catalog version ref；若 search 和 inspect hash 不一致，返回 discovery error `reason=catalog_changed`（exit 6）并要求重新搜索，不能静默使用新版本 Schema。

Target CLI contract：

```bash
dws schema chat.query_msg_read_status --compact -f json \
  --expected-source-hash H1 \
  --expected-surface-hash H2
```

成功响应为 `schema-inspect.v1` envelope，包含 `catalog` 和 `tool_spec`。任一 expected hash 不一致时，在解析参数和调用业务后端之前返回机器可读错误 `reason=catalog_changed`、exit code 6，且不返回 ToolSpec。flags 省略时保留当前人类/兼容 flat 查询行为；Search → Inspect 的 Agent 路径 MUST 同时传两个 expected hash，并使用 `--compact`。

### 5.9 多动作检索

Agent Host 负责把：

```text
给群里发文件并确认送达
```

拆为至少：

```text
给群里发送文件
查询消息发送状态
查询群成员已读状态
```

DWS 对每个 action-sized subquery 独立检索，再 round-robin 或 constrained set-cover 合并，避免第一个动作耗尽全部 Top-K。

后续完整性校验应使用结构化关系：

- `requires`；
- `produces`；
- `verifies`；
- resource type；
- effect；
- workflow stage。

这些字段当前尚未完整进入 Catalog，属于后续 reviewed metadata 变更。

### 5.10 Contradiction 与 Policy Gate

当前最佳相关性方法的 Forbidden@5 仍约 54%～56%。这不是简单调低分数能解决的问题，因为 `avoid_when` 描述的工具往往与 query 语义高度相似。

Gate 至少检查：

- query 是否命中候选 `avoid_when`；
- effect 是否符合用户意图；
- 已知资源 ID 和前置参数是否匹配；
- read/create/update/delete 是否一致；
- 当前 workflow stage；
- destructive confirmation；
- Host/执行端权限。

当前评测只证明“需要 gate”，没有证明某个 gate 已有效。正式门禁需要 alternative gold、误杀率和 gate 后任务成功率。

## 6. 后续独立 RFC 草案：可信执行与安全恢复（非当前能力）

本章只保留未来方案输入，不是本分支的规范实现或发布承诺。`upstream/main` 已删除旧 `internal/recovery`；所有包名、状态机和字段都必须在后续独立 RFC 中重新审计后才能成为规范。

### 6.1 Invocation Contract

执行前把选择时的契约传播到 Invocation：

```go
type InvocationContract struct {
    Version            string
    OperationID        string
    CatalogSourceHash  string
    CatalogSurfaceHash string
    CanonicalPath      string
    ToolSchemaHash     string
    PrincipalID        string
    TenantID           string
    OrganizationID     string
    NormalizedArgsHash string
    Effect             string
    Confirmation       string
    Idempotency        string
    IdempotencyKey     string
    PolicyRevision     string
    PolicyDecisionID   string
    ApprovalEvidenceID string
    VerificationSpecID string
}
```

执行仍通过真实 Cobra leaf。Invocation Contract 用于审计、漂移检测和 recovery，不替代命令自身验证。`IdempotencyKey` 只有在后端真实支持、接受并能以同一 key 去重时才允许设置；工具级 `idempotency` 标签本身不能证明某次重放安全。

每次网络尝试另记不可变 `AttemptRecord`，至少包含 `OperationID`、递增 `Attempt`、backend request ID、发送/受理状态和时间。重试不得修改原始主体、参数摘要、Catalog、policy 或 approval evidence；任一变化都必须创建新的 operation。

#### 6.1.1 进入真实执行链路的实现边界

Target owner 和接入顺序固定如下，避免 InvocationContract 停留在文档结构体：

1. `internal/executor` 定义 `InvocationContractV1`、`AttemptRecord` 和 typed validation error；现有 `executor.Invocation` 增加可选 `Contract`，普通人类 CLI 不传时保持兼容。
2. Agent managed mode 通过继承的只读 pipe/file descriptor 传递 versioned JSON envelope；环境变量只携带 descriptor 编号，不携带凭据或完整 payload。直接在命令行参数中传 principal、approval 或 policy 证据不受信任。
3. `internal/app/runtimeRunner.Run` 是统一适配点：在任何 endpoint、PAT 自动授权或 transport retry 之前，把 Cobra 已解析的 typed params 与当前 ToolSpec 绑定，校验 Catalog、canonical、schema hash、principal/tenant、policy 与 approval evidence。managed mode 缺少或校验失败时后端调用必须为 0；普通 CLI 继续走现有交互确认。
4. 未来 recovery owner 在发送前原子写 `PREPARED`，发送边界写 `SENT`，并按 `OperationID + Attempt` 记录 immutable attempt；具体包边界不得引用已被主线删除的旧实现。
5. 首批只启用 read，以及“后端真实接受幂等键 + reviewed Verify”同时成立的少量写工具。其余 mutation/destructive 保持 `handoff-only`；不会因为 Catalog 标签为 idempotent 就自动放行。

跨进程 approval/policy evidence 必须由执行端可验证的签名或 MAC 保护，并绑定 `operation_id`、actor、tenant、canonical、normalized args、Catalog、policy revision 和 expiry；Host 自报的布尔值无效。迁移期未携带 Contract 的旧调用可以执行，但不得获得 managed retry/recovery 能力，并在审计中标记 `legacy_unmanaged`。

#### 6.1.2 摘要的规范化

- `ToolSchemaHash = SHA-256("tool-schema.v1" || canonical JSON(ToolSpec execution projection))`；projection 明确列出参数、约束、interface、safety 和版本，不包含展示顺序、示例和本地绝对路径。
- `NormalizedArgsHash = SHA-256("typed-args.v1" || RFC 8785/JCS canonical JSON(parsed typed args))`；所有业务参数都纳入，文件参数使用内容 SHA-256、size 和媒体类型，不用临时路径作为主体身份。
- `ApprovalEvidenceID` 只引用受保护证据；日志不得保存 bearer token、cookie、文件正文或 approval secret。
- projection/canonicalization 版本是 hash domain 的组成部分；规则变化必须生成新 Operation，而不是把旧 journal 静默迁移成新摘要。

### 6.2 结果可信度

每次执行记录：

- query/subquery 的 salted digest、语言/意图类别和可选脱敏片段；默认不记录原文；
- search strategy 和候选 canonical；
- Catalog hashes；
- 最终 inspect canonical；
- effect / confirmation / idempotency；
- execution ID、后端 request ID；
- 输入摘要和脱敏输出摘要；
- postcondition verification 结果。

审计和 recovery 的数据治理同样是发布契约：默认最小化采集；principal/tenant 与 query digest 仅允许当前用户和审计角色读取；本地 recovery journal 默认保留 7 天、脱敏 audit 默认 30 天，均支持更短的组织策略和显式清理；导出、诊断和 crash dump 继续执行同一 redaction。持久化 envelope 必须包含 schema version，迁移采用“读旧、写新、保留原摘要”的显式 migrator，无法识别的版本 fail closed 并 handoff。

结果状态必须拆分为：

```text
transport_status
tool_status
business_status
verification_status
```

HTTP/MCP 成功只证明 transport/tool envelope 可解析，不能自动证明业务完成。当前 Runner 对缺少业务 `success` 字段的响应会补 `success=true`；Phase 4 必须移除这项过度推断，改为 `accepted_unverified` 或 `unknown`，直到 postcondition 被验证。

Recovery journal 也必须从“追加诊断记录”升级为有合法迁移的状态机：

```text
PREPARED → SENT → ACKED | REJECTED | UNKNOWN
                     ↓
                  VERIFIED
                     ↓
       COMMITTED | COMPENSATED | HANDOFF
```

journal 需要原子写、进程锁、fsync/rename、幂等 finalize 和迁移校验。当前 JSON/JSONL store 与单一 AITable probe 只能支持保守分析和 handoff，不能作为自动恢复已完成的证据。

### 6.3 重试矩阵

| 操作状态 | 自动重试 |
|---|---|
| read + transient error | 可以，受次数和退避限制 |
| reviewed idempotent write + 后端认可的同一幂等键 | 可以 |
| write 且已证明服务端未受理 | 可以 |
| non-idempotent write | 不可以 |
| unknown write / timeout 后受理状态未知 | 不可以，先 Verify |
| destructive operation | 不因重试跳过用户确认 |

归档的旧 recovery 实验曾部分依赖工具名前缀猜 read/write。未来独立方案必须只使用 reviewed SafetySpec；不得把名称推断恢复为发布能力。

当前 322 个 mutation/destructive 工具中有 311 个 `idempotency=unknown`。因此 Phase 4 的默认必须是：unknown write 零次自动重放，先 Verify；无法验证则 handoff。

### 6.4 Verify 与 Compensate

- 每个可恢复写工具需要 reviewed postcondition 查询；
- Verify 成功则把超时视为“业务已完成”，禁止重放；
- Verify 证明未生效且满足幂等条件才重试；
- 补偿必须逐工具 reviewed，不能根据 create/delete 名称自动生成；
- 没有补偿契约时交给 Agent/human handoff，不宣称事务回滚。

## 7. 对外接口选择

### 7.1 分两次交付

第一步只交付 `internal/cli` 搜索内核和评测，不改 Cobra surface。

第二步增加薄封装：

```bash
dws schema search --query "给群里发文件并确认送达" --limit 5
```

不新增顶层 `dws discover`，原因是：

- `schema search` 清楚表达它搜索的是命令契约，不是业务数据；
- 避免与 endpoint discovery、隐藏 catalog 和业务 search 混淆；
- 复用 `dws schema <path>` 作为 Inspect，认知链路更短。

### 7.2 兼容性成本

当前 `schema` 是一个 runnable leaf，并在 reviewed exclusion 中记录。如果增加 `schema search`：

- `schema` 会成为仍可 runnable 的 parent，完整性扫描只把无子命令的 runnable command 当 leaf；
- 原 exact exclusion 会 stale；
- 必须删除旧 `schema` leaf exclusion，并给新的 runnable `schema search` leaf 增加 reviewed utility disposition；
- 同步更新 docs、skills、help、完整性测试和生成门禁。

因此不能在搜索内核 Spike 中顺手增加公开命令。

## 8. 评测设计与上线门禁

### 8.1 当前证据的限制

1. 602 条 `use_when` query 与被索引的 `agent_summary` 来自相同 selection 文件，存在同作者/同措辞偏差。
2. 10 条 workflow 太小，人工 subquery 与 required gold 在同一 fixture。
3. case-level bootstrap 把 case 当独立样本；product-cluster 结果没有证明 Hybrid 跨产品稳定。
4. Forbidden 数据只标出不能用的工具，没有标出正确替代工具。
5. 当前 Python 是全量扫描评测器，不是生产倒排索引延迟。
6. 还没有真实 Agent planner、参数生成、执行和 recovery 的端到端结果。
7. Python 评测与 Go 实现还不是逐 case parity：Go 已默认排除 `use_when`，但 alias、参数描述/类型和字段权重仍未与 Python proxy 锁成一个 `ProjectionVersion`；现有 R@K 不能直接当作 Go 发布配置结果。
8. 602 条 intent 由 283 条纯中文和 319 条中文混 ASCII 组成，英文为 0；现有数据无法执行英文 Alpha 门禁。
9. 当前 Forbidden 只有“不能选这个工具”，没有正确替代工具；在 alternative gold 完成前，contradiction gate 无法同时衡量暴露率和误杀。

### 8.2 数据集分层

建立三个严格隔离的数据集：

```text
train：同义词、字段和规则开发
dev：算法、权重、k1/b/delta、RRF k/depth 调参
test：锁定后只运行一次的发布门禁
```

每个 test 版本必须有 owner、冻结 commit、解封记录和预注册的候选算法/参数。test 失败后不得在原 test 上继续调参；需要修订时生成新版本并记录变化原因。

业务团队独立编写 test query，至少覆盖：

- 22 个产品；
- 纯中文与中文混合参数/ID；
- canonical/CLI/alias；
- read/write/destructive；
- sibling confusion；
- alternative gold；
- forbidden + 正确替代；
- 2～4 步跨产品工作流；
- 权限不可用和执行失败恢复。

### 8.3 公平算法对照

第一阶段只改变算法，不改变 corpus/tokenizer/filter：

- TF-IDF cosine；
- Okapi BM25；
- BM25L；
- BM25+；
- Weighted Jaccard 作为低复杂度对照。

第二阶段再正交改变：

- unfielded；
- fielded score ensemble；
- 标准 BM25F；
- optional CandidateProvider；
- contradiction gate。

Exact Guard 对所有方法共同前置，identity 单独按 pass/fail 报告。

生产评测还必须固定一个 `ProjectionVersion`，明确字段、字段权重、tokenizer 和归一化。Go/Python 对同一 Catalog/query/config 的 exact、filter、Top-K 必须逐 case parity；独立 qrels 上再按真实生产投影评测，不能用缩减投影数字替代。

### 8.4 指标

Retrieval：

- R@1/R@5/R@10、MRR、NDCG；
- micro、macro-product、worst-product、leave-one-product-out；
- paired product/tool-family cluster bootstrap；
- zero-result/abstain rate；
- alternative recall；
- Forbidden exposure 和 gate false removal。

Workflow：

- Complete@5；
- required tool recall；
- 自动拆解准确率；
- 顺序/依赖/参数传递正确率；
- 最小完整集合大小。

End-to-end：

- 工具选择成功率；
- 参数一次通过率；
- 任务完成率；
- 错误写入率；
- 人工接管率；
- recovery verify/retry/compensate 成功率。

Performance：

- warm p50/p95/p99；
- index build time；
- heap/index bytes；
- allocations/query；
- 二进制增量；
- CandidateProvider timeout 和 fallback latency。

### 8.5 Alpha 门禁

以下是进入 Alpha 的最低条件，不是当前已达成声明：

| 门禁 | 要求 |
|---|---|
| Exact identity | 所有 canonical、primary CLI path、reviewed unique alias Top-1 100%；filtered exact 返回 typed refusal 且不返回 sibling |
| Determinism | 相同 Catalog/query/config 的非耗时输出逐字段相同 |
| Projection parity | 固定 ProjectionVersion 下 Go/Python exact、filter、Top-K 逐 case 一致 |
| Lightweight quality | 独立 test 上按预注册非劣效 margin 比较锁定 BM25；默认切换还需预注册最小增益；报告 product/tool-family cluster CI |
| 中文质量 | 成对中文口语、混 ASCII、英文和错别字 slice 分别达到预注册阈值与最小样本量 |
| Safety | excluded/unavailable 候选 0 泄漏；invalid provider candidate 0 泄漏 |
| Fallback | provider timeout/error/invalid/duplicate 全部返回确定性本地结果 |
| Search output | 默认最多 5 个引用；不包含完整参数 Schema；必须要求 Inspect |
| Latency | 当前完整 Catalog 的目标平台 warm p95 < 10 ms；release binary cold subprocess p95 单列并在 manifest 预注册，不以 warm 数字替代 |
| Footprint | 不增加模型/runtime；Go 二进制增量目标 < 1 MiB |
| Workflow | 不少于 80 条冻结的 2～4 步任务；自动 Complete@K、依赖顺序和参数传递达到预注册阈值，不用人工拆解上界代替；K=5 与按步骤受控扩大到最多 10 同时报告 |

预注册 manifest 的 Proposed 默认数值是：轻量算法相对 BM25 control 的 Recall@5 非劣效 margin `2 pp`，切换默认的最小增益 `1 pp`；纯中文、中文混 ASCII、英文各不少于 100 条且 Recall@5 不低于 80%；workflow `Complete@5 ≥ 80%`、required-tool recall `≥ 90%`、依赖顺序与跨步参数传递各 `≥ 95%`。这些阈值由 Retrieval owner 与独立 Evaluation owner 在 test 解封前共同签署；任何修改都要先改 RFC/manifest，不能在看到 test 后补写。

质量门禁使用 product/tool-family cluster paired bootstrap 95% CI；“不劣于”要求 CI 下界不低于 `-margin`，“切换默认”要求 CI 下界高于 `minimum_effect`。性能协议固定为 Apple M3 Pro 本机与 Linux amd64 CI release build，100 次 warm-up 后至少 10,000 个单 query 请求报告 p50/p95/p99；footprint 比较同 flags 的 stripped release artifact。若独立数据表明 Proposed 数值不现实，只能在 test 仍封存时通过 RFC amendment 调整；当前状态保持 Proposed，不因 proxy 集过门禁而批准上线。

进入默认开启还需要 shadow/Alpha 真实 Agent 任务成功率和错误写入率通过业务评审。

### 8.6 Phase 4 可信执行与恢复门禁

以下门禁不阻塞只读 Search Alpha，但任何“安全自动恢复”表述或能力必须全部通过：

| 场景 | 要求 |
|---|---|
| Catalog 漂移 | Search 为 H1、Inspect/Execute 为 H2 时返回 `CATALOG_CHANGED`，后端调用 0 次 |
| Policy/ACL | deny 或 unknown 不标记 authorized；执行前拒绝时后端调用 0 次 |
| Confirmation | 缺失、过期、actor/参数/Catalog 不匹配时调用 0 次 |
| 业务状态 | HTTP 200、tool envelope 成功但无业务状态时不得生成 `success=true` |
| 未知受理 | 写请求发出后 timeout/断连，自动重放 0 次，先执行 reviewed Verify |
| 幂等并发 | 同 key/同参数并发只产生一次业务写；同 key/不同参数摘要必须拒绝 |
| 非幂等/unknown | timeout、5xx、进程重启均为 0 次自动重放 |
| 崩溃窗口 | 在 PREPARED、SENT、业务成功、journal commit 各点终止，恢复后不产生第二次副作用 |
| 状态机 | 不存在 operation、非法迁移、矛盾 finalize 全部拒绝；同结果 finalize 幂等 |
| 补偿 | 只执行 reviewed CompensationSpec；部分补偿记录完整并 handoff |
| 审计 | Search、policy、attempt、verify、compensate 通过 OperationID 全链关联 |
| 并发 | journal、幂等状态和 audit 的 race test 通过 |

### 8.7 默认开启前的整体收益 A/B

Schema bytes 下降只是容量证据，不等于线上 token 节省；如果现有 Agent 没有加载完整 `schema --all`，18.6 MB 也不能作为真实对照。默认开启前必须使用同一模型、prompt、任务、工具权限和最大调用预算，对“现有导航路径”和“Search → Inspect”做配对 A/B，并在解封 test 前预注册非劣效/改善阈值：

- 任务完成率与参数一次通过率不退化；
- 实际模型输入/输出 token、调用次数或端到端成本至少一项达到预注册改善幅度，其他项不显著恶化；
- forbidden/wrong-tool call、错误写入率和人工接管率不退化；
- 按 product/tool-family cluster 报告配对区间，不只报告 micro 平均；
- recovery 仅在 8.6 全部门禁通过后进入 A/B，否则保持 handoff-only。

Proposed A/B 判据为：任务完成率和参数一次通过率相对现有导航路径的 95% CI 下界均不低于 `-2 pp`；模型总 token 或端到端成本至少一项下降 `≥ 20%` 且另一项不恶化超过 `5%`；wrong/forbidden-tool call、错误写入率与人工接管率的 95% CI 上界不高于 control `+0.5 pp`。这些是默认开启门槛，不是当前 proxy 结果。

本分支提供 Go 聚合器 `AggregateToolSearchAgentAB`。Agent runner 必须为同一个 `case_id/trial` 同时写入 `direct_schema` 与 `search_inspect` 两臂，并记录 task completed、correct plan、unsafe action、recovery、模型 tokenizer token、tool calls 和 latency。聚合按 case 聚类，先在 case 内平均模型 seed，再使用固定种子的 10,000 次 percentile bootstrap；缺臂、Catalog hash 不一致、重复 run 或非法 recovery 状态全部拒绝。没有真实模型 run 时报告必须明确返回 `not_run_requires_independent_tasks_and_model_runs`，不得用 retrieval proxy 冒充 Agent 收益。

另外提供 `ScoreToolSearchAgentPlanningAB` 对只读规划 smoke 做严格评分。它只衡量 required-tool coverage、顺序完全一致的 minimal plan、plan precision 和额外步骤；不会把“规划正确”写成“任务已完成”。本轮规划 smoke 已写入 build-temp 报告，但 end-to-end `agent_ab_status` 仍保持 `not_run`。

诊断对比只在构建临时目录生成：

```bash
make generate-tool-search-comparison
# .worktrees/policy-tmp/tool-search-comparison.json

go run ./internal/generator/cmd_tool_search_comparison \
  -agent-plans-direct .worktrees/policy-tmp/agent-direct-schema.json \
  -agent-plans-search .worktrees/policy-tmp/agent-search-inspect.json \
  -agent-model gpt-5.6-sol \
  -output .worktrees/policy-tmp/tool-search-comparison.json

go run ./internal/generator/cmd_tool_search_comparison \
  -agent-results /path/to/paired-agent-runs.json \
  -output .worktrees/policy-tmp/tool-search-comparison.json
```

仓库不提交这份生成 JSON；结构化事实源是 Go DTO、typed Catalog、reviewed workflow fixture 和外部 Agent runner 的配对结果。

## 9. 灰度与实施计划

### Phase 0：Spike（已完成，未发布）

- 纯 Go Exact + 字段 BM25 分数集成；
- 中文/identifier tokenizer；
- Product/effect/exclude/executable hard filter；
- CandidateProvider 独立召回和 RRF；
- provider failure deterministic fallback；
- Catalog source/surface hash；
- 多动作 round-robin；
- Python 多算法与中文评测。
- BM25 query term 固定顺序累加与 8 子进程 JSON golden；
- Go shipped-runtime 诊断对比与配对 Agent A/B 聚合器；
- 字段投影、轻量算法、纯词法/Dense RRF depth/k、cold build/warm query 续测。

当前测试覆盖搜索内核、公开 transport、双 hash Inspect、provider 异常和跨进程确定性；benchmark 已记录。Go `LexicalRetriever`、BM25 control 与 TF-IDF shadow 已实现。它仍不是默认发布实现，因为倒排/分配优化、Go/Python parity 和正式独立 qrels 尚未完成。

### Phase 1：检索内核

- 已抽象 Go `LexicalRetriever`；
- 已实现 Go TF-IDF 与 BM25；
- 增加倒排索引/预计算 norm；
- Go/Python parity fixture；
- 真实 holdout 和 cluster bootstrap；
- 由独立 Evaluation owner 建设至少 100 条英文、80 条 workflow、带 alternative gold 的 hard negative；在数据未冻结前不进入公开 Alpha；
- 索引内存、binary delta、p95 门禁。

### Phase 2：本地公开 Alpha（transport 已实现，门禁未通过）

- 已增加 local-only `dws schema search`、版本化 stdin JSON 与 expected source/surface hash Inspect；
- 已把 reviewed exclusion 从旧 runnable leaf `schema` 迁移到新 leaf `schema search`；docs/skills/generated gates 仍需发布前复核；
- external ranking transport 已实现为不可信输入校验，但在独立门禁与 feature rollout 前不得由默认 Host 注入；
- Host 可在 shadow 中记录新结果，但继续用现有发现路径执行。

### Phase 3：Provider shadow 与融合 Alpha

- Host 使用第一次 local-only Search 返回的 CatalogVersionRef 调用可选 Provider；
- 先只记录 TF-IDF、BM25、provider 候选和最终实际工具，按产品/query/effect 分析救回与退化；
- 指标通过后才用 feature flag 提交 `external_ranking` 进入 DWS 校验和融合；
- provider unavailable 时逐字段退回 local-only 结果；
- contradiction gate 进入实验。

### Phase 4：后续独立 RFC（不在当前分支）

可信执行、InvocationContract、reviewed postcondition、Verify/Retry/Compensate 与恢复 journal 不再属于本 RFC 的实现阶段。`upstream/main` 已删除旧 Recovery，相关实验代码仅在归档分支 `archive/sandbox-recovery-pre-main-20260813` 留存，不能作为当前能力或发布承诺引用。

当前 Search 的强制边界是：只返回版本绑定的 ToolReference；Inspect 后仍通过真实 Cobra leaf；Search 结果不授权、不确认、不执行、不自动重放。未来恢复方案必须另行完成后端幂等键摸底、状态机持久化、approval/policy evidence、隐私保留策略和故障注入门禁。

### Phase 5：默认开启

- 独立 test 门禁通过；
- shadow/Alpha 端到端收益通过；
- 错误写入和人工接管不退化；
- 回滚开关和旧发现路径保留至少一个稳定版本周期。

## 10. 备选方案与否决原因

| 方案 | 结论 | 原因 |
|---|---|---|
| 把 1,098 个完整 Schema 全塞上下文 | 否决 | 17.85 MB compact 载荷，工具干扰和上下文成本不可接受 |
| 只按产品/分组手工导航 | 保留 fallback | 稳定但自然语言召回和复合任务不足 |
| 纯 Keyword/Jaccard | 否决为主方案 | R@5 明显落后；低 Forbidden 是低召回副作用 |
| BM25 固定为唯一算法 | 否决 | TF-IDF 当前略高且差异未定；接口应允许替换 |
| TF-IDF 立即替代 BM25 | 否决 | +1.50 pp CI 跨 0，且 qrels 同源 |
| CLI 内嵌 BGE/ONNX | 否决 | 模型比当前 CLI 大，跨平台和生命周期成本过高 |
| 远端 Dense 作为强依赖 | 否决 | 破坏离线可用性，引入延迟、隐私和可用性耦合 |
| Host 可选 CandidateProvider | 接受 | 可补召回且能验证/降级，不扩大 CLI 基础依赖 |
| LLM 直接在 1,098 工具中选择 | 否决 | 上下文大、不可确定、难以审计和回归 |
| 顶层 `dws discover` | 否决 | 与 endpoint/业务发现概念冲突，兼容面更大 |
| `dws schema search` | 接受为 Phase 2 | 与现有 Inspect 语义一致，但需单独处理 runnable parent/exclusion 兼容 |

## 11. 风险与开放问题

### 11.1 已知风险

- tool metadata 文案相似会造成 sibling confusion；
- `avoid_when` gate 可能误杀，需要 alternative gold；
- Host provider 可能返回旧 Catalog canonical；
- score 不同算法不可直接比较或设统一置信阈值；
- 复合任务拆解错误会让后续 ranker 无法补救；
- Catalog 当前缺少完整的 requires/produces/verifies 关系；
- 失败恢复不在当前分支范围内，不能把 Search 完成误写为安全恢复完成；
- `+shortcut` 已通过 reviewed exact exclusion group 纳入主线合并后的 Catalog 完整性处理；新增或删除 shortcut 仍必须同步复核该组，禁止通配排除。

### 11.2 待决策

1. 独立 test qrels 的实际人员签署与样本采集；owner 角色、最小规模、hash 和双签冻结流程已写入 `scripts/testdata/tool_search_eval_manifest.json`；
2. TF-IDF 与 BM25 shadow 的线上采样比例；
3. CandidateProvider 由 Agent Host 还是独立服务提供；
4. contradiction gate 先用规则、Host LLM 还是两阶段组合；
5. `requires/produces/verifies` 放入哪个 reviewed Schema input；
6. Host SDK 如何把已实现的 `catalog_changed` discovery error 映射为跨语言错误类型；
7. Contradiction gate 采用规则、Host LLM 还是两阶段，以及 alternative gold/误杀率 owner；
8. 多动作响应预算固定 Top-5，还是按 action 数受控扩到最多 10；
9. 公开 CLI 的 cold subprocess p95 目标，以及一次 fusion request 是否已足够避免双进程初始化。

## 12. 最终决策

本 RFC 确定的是稳定架构和安全边界：

```text
唯一 typed Catalog
  → identity resolve
      ├─ exact → eligibility → exact / exact_filtered（终止）
      └─ not found → Hard Filter
  → 可替换轻量召回
  → 可选外部补召回与确定性降级
  → ToolReference
  → schema-inspect.v1
  ── 当前 RFC 边界 ──
  ⇢ Future RFC: Cobra Execute → reviewed Verify / Retry / Compensate
```

本 RFC **不锁定** BM25、TF-IDF、字段权重、RRF 参数或 Dense 模型。当前建议是 BM25 作为 Alpha control，TF-IDF/BM25+ shadow；独立 holdout 和真实 Agent 灰度通过后再确定默认。DWS 基础 CLI 始终保持零模型依赖和离线可用。
