# DWS 渐进式工具发现与可信 Inspect：综合设计文档

本文档合并了以下六份独立文档，按 RFC → 调研 → 评测 → 优化 → 审查 的逻辑组织：

1. RFC：DWS 渐进式工具发现与可信 Inspect
2. DWS 工具检索与排序源码实现调研
3. DWS Tool Search：GitHub 源码架构调研
4. DWS 工具检索排序实测报告
5. Schema Search 调研、实测与优化方案（2026-08-14）
6. Tool Search 十轮 Review 与收敛记录

---


# Part 1: RFC — DWS 渐进式工具发现与可信 Inspect


- 状态：Proposed / Spike 已验证，尚未达到发布门禁
- 日期：2026-08-12
- 目标版本：分阶段交付，不绑定单一版本
- 影响范围：`internal/cli`、DWS Skills、Schema Search/Inspect；Agent Host 仅为可选增强
- 关联实现：[`internal/cli/tool_search.go`](../internal/cli/tool_search.go)
- 关联测试：[`internal/cli/tool_search_test.go`](../internal/cli/tool_search_test.go)
- 评测实现：[`scripts/dev/eval_tool_search_ranking.py`](../scripts/dev/eval_tool_search_ranking.py)
- 评测报告：本文档 Part 4
- 最新审计与优化方案：本文档 Part 5
- 背景调研：本文档 Part 2
- GitHub 源码架构调研：本文档 Part 3

## 1. 摘要

DWS 当前由 Go 声明在运行时装配出 1,098 个命令契约的 typed Schema Catalog。对 Agent 而言，**DWS 本身是一个稳定元工具**；Catalog 中的条目是 DWS 内部命令空间，不是要向 Host 动态注册的 1,098 个独立 Tool。Agent 已可按产品、分组、leaf 渐进查询，但对自然语言意图仍缺少一个稳定的子命令检索入口；复合任务还会遇到只找回部分步骤、相关命令挤占 Top-K，以及结果无法证明来自哪个 Catalog 版本等问题。

本 RFC 决定采用以下架构：

```text
Agent 通过同一 DWS 元工具拆解任务
  → DWS identity resolve
      ├─ exact hit → eligibility check → exact reference / typed exact_filtered
      └─ no hit → Catalog availability / namespace / effect Hard Filter
  → 可替换的轻量词法召回
  → contradiction / policy gate
  → 最多 5 个 ToolReference
  → Inspect 完整 ToolSpec
  → 真实 Cobra leaf Execute（沿用既有执行链）
```

关键选型是：

1. **DWS CLI 不内嵌 Dense/Embedding 模型。** 当前 `dws` 二进制约 44 MiB，实验模型 `BAAI/bge-small-zh-v1.5` 约 90 MB，且需要 ONNX runtime、多平台适配和模型生命周期管理。当前 proxy 评测中的收益不足以支持将这些成本变成 CLI 的强依赖。
2. **BM25 不是架构前提。** 词法层定义为可替换接口。Alpha 以 Okapi BM25 作为稳定 control，同时 shadow TF-IDF cosine 和 BM25+；独立 holdout 通过前不宣布任何算法胜出。
3. **不实现 CandidateProvider、外部 ranking 或 RRF fallback。** Dense/Hybrid 只保留为历史评测对照，不进入 CLI、公开协议或 Host 接入面。当前规模下，本地确定性词法检索足以支持 Alpha 验证；额外远端链路的认证、版本、超时、融合和失败状态属于过度设计。
4. **Exact、Search、Inspect、Execute 必须分层。** 搜索结果不是可执行 Schema；Agent 必须精确 Inspect 后才能组装参数和执行。
5. **多动作任务先拆解后检索。** 每个动作保留候选预算，再做集合合并和完整性校验，不能把整个工作流只当作一个 query 排一次 Top-5。
6. **相关性不等于安全。** `avoid_when`、effect、确认、权限和幂等约束属于独立 gate；排序分数不得表述为“可信概率”。
7. **显式 Search 是规范，但只服务未知命令。** Skill/reference 已给出精确 CLI path 时直接调用 DWS；只有无法定位子命令时才走 Search → 双 hash Inspect → 同一 DWS Execute。Search query、Catalog、候选和失败必须可复现；Host 原生适配只是可选的交互优化。

### 1.1 2026-08-13 主线合并后的范围与实现状态

本分支已合并 `upstream/main@346444ea`。主线移除了旧 Recovery 实现，并把 Catalog 迁移为“Go 声明即 Catalog”的运行时装配；因此本 RFC 的交付范围同步收敛为 Tool Search 与双 hash Inspect，旧 Sandbox/Recovery 工作仅保存在归档分支 `archive/sandbox-recovery-pre-main-20260813`，不会被恢复到当前分支。安全恢复仍是后续独立 RFC，不是本次 Search 发布能力。

Catalog 的规范数据是 Go `ProductDecl` / `ContractDecl` 和 `ResolveSchemaBuild` 的结构化结果。仓库不提交 Catalog JSON，也不从 JSON 加载生产索引；只有评测/CI 会调用 `cmd_schema_catalog`，把分片临时生成到被忽略的 `.worktrees/policy-tmp/tool-search-schema-catalog`。生成器、运行时 Search、评测聚合与可信度校验的核心框架均为 Go。

当前 Implemented (unreleased)：

- `dws schema search` 的 exact / `exact_filtered`、hard filter、Go BM25/TF-IDF、动作重排和资源预算；
- `ToolReference → schema Inspect` 的 source/surface 双 hash 闭环与 typed `catalog_changed`；
- 由运行时声明直接生成的 Go 诊断对比、中文切片、workflow、负例暴露、身份与完整性指标。

当前仍未通过：独立 qrels、真实模型配对 Agent A/B、英文切片、线上 contradiction gate 与 cold subprocess SLA。因此状态仍是 Proposed，不得把同源诊断指标表述成线上任务成功率。

### 1.2 执行模型：DWS 是单一元工具

规范业务路径不需要 Codex/OpenAI 式的动态 Tool Registry：

```text
已知能力：Agent → dws <known cli path> → Cobra Execute
未知能力：Agent → dws schema search
                    → dws schema <canonical> --expected-*-hash
                    → dws <primary_cli_path>
```

Search/Inspect/Execute 都是同一 DWS Tool 的子命令。因此，会话内“加载新工具”、Schema LRU 和 Host registry 改造不是 local-only 发布前置条件。如果未来 Host 愿意将该链路包装成原生 `tool_search` 调用，必须复用同一 versioned wire 和 SearchTrace，不得发布第二套身份、排序或授权语义。

### 1.3 规范用语与能力标签

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
| Go 字段 BM25 默认 | R@1 65.27%；R@5 88.16%；MRR@5 0.7441 | 独立 qrels 缺失期间采用的保守默认 |
| Go action ranker shadow | R@1 62.51%；R@5 84.24%；MRR@5 0.7131 | 同源 proxy 上 R@5 低 3.92 pp；仅拆解 workflow 更好，等待独立门禁 |
| 精确身份 | Exact Guard 后 canonical / CLI Top-1 100% | Exact Guard 是穷举契约门禁 |
| 多工具完整性 | 默认整句 Complete@5 40% / required recall 61.67%；reviewed 拆解后 80% / 90% | 拆解结果是人工上界，不是端到端成功率；action shadow 为 90% / 95% |
| 中文召回 | 默认算法纯中文 R@5 83.83%，中英混合 90.57% | 中文 unigram+bigram + 标识符保留可用，但纯中文仍是重点优化项 |
| 负例暴露 | 默认 Forbidden@1 0%，Forbidden@5 0.0719%（1/1,390） | 同源 avoid_when proxy 已大幅下降，但仍必须建设独立 alternative gold / contradiction gate |

相对最简单的 IDF keyword overlap，TF-IDF 在当前 proxy 集上的 R@5 从 76.91% 提升到 84.55%，提高 7.64 个百分点。这个数字可以说明“需要一个正式词法 ranker”，但因为 query 与索引元数据来自同一批作者，不能外推为线上业务收益。

当前分支的纯 Go shipped-runtime 对比器直接消费运行时装配的 typed Catalog，不读取仓库中的生成 JSON。action shadow `fielded_bm25_action_v1` 相对当前默认 `fielded_bm25_ensemble`：R@1 **-2.76 pp**、R@5 **-3.92 pp**、MRR@5 **-0.031**；raw workflow required recall **+15 pp**，reviewed 拆解的 Complete@5 / required recall **+10 / +5 pp**；两算法的 Forbidden@1 同为 **0**、Forbidden@5 同为 **0.0719%**。由于这些结果来自同源 proxy，且主召回指标全面落后，action 只能继续 shadow；若未来重评，切换默认需独立数据的非劣效与最小增益人工裁决（sealed 门禁已按仓库决策移除）。

合并主线前曾用固定 `gpt-5.6-sol` 做过一轮 answer-free、无业务执行的规划 smoke A/B：同一批 10 条 workflow，各 arm 一个 batch/一次 trial。该 run 绑定旧 572 工具 Catalog，已因当前 1,098 工具 surface 变化判为过期，只保留方法记录，不能作为现行效果证明。旧结果中两臂 required-tool complete/recall 都是 **100% / 100%**；精确最小计划率分别为 **90% / 70%**，plan precision 为 **95.45% / 87.50%**，额外步骤为 **1 / 3**。单 trial 无区间，不能据此通过默认开启门禁。

### 3.2 上下文与 Token 收益

本机对当前 Catalog 的实测：

| 载荷 | 大小 |
|---|---:|
| compact 全量 Schema | 17,876,084 bytes |
| 平均 Search + gold Inspect | 4,507.03 bytes |
| 理想化 overview + 正确 product + Inspect | 122,368.95 bytes |

渐进发现把“预加载全部 Schema”变成“约 3 KB 候选 + 选中 leaf 的数 KB Schema”。实际模型 token 下降比例取决于 tokenizer 和对话编排，因此 RFC 只以字节数作为已验证证据，不给出未经测量的 token 节省承诺。

用 Go 对比器逐条渲染当前 compact JSON envelope 后，Search + gold leaf Inspect 平均 **4,507.03 bytes**；相对 17,876,084 bytes 的全量 Schema 减少 **99.9748%**，相对已经假设 oracle 知道正确 product 的理想化导航也减少 **96.3169%**。评测器直接 Inspect gold，即使 Search miss 也不计额外尝试；因此这是 JSON byte 容量上界，不是 tokenizer token、真实 Agent 成本或任务成功率承诺。

### 3.3 CLI 与工程收益

- 本地轻量检索不增加第三方 Go 依赖，不要求网络、模型下载、GPU 或个人凭据。
- 继续复用同一个 `SchemaRegistry`，避免 Catalog、CLI 和 Agent metadata 出现双事实源。
- ToolReference 携带 Catalog source/surface hash，便于审计搜索和 Inspect 是否基于同一版本。
- 检索器、policy gate 和 executor 分层后，可以独立评测与灰度。

### 3.4 性能成本与当前缺口

纯 Go Spike 在 Apple M3 Pro、572 个工具、中文复合 query 上的 warm benchmark：

| 版本 | 单 query | 分配 |
|---|---:|---:|
| 初版 | 2.04～2.82 ms | 约 1.42 MB / 9,625 allocs |
| 复用 query 词频后 | 1.31～1.55 ms | 约 115 KB / 1,048 allocs |

2026-08-13 在历史 572 工具 Spike 上运行三轮 benchmark，warm Search 为 **0.556～0.574 ms**、约 **86 KB / 1,123 allocs**；当时的 engine build 为 **48.70～48.82 ms**、约 **28.5 MB / 244k allocs**。后者仍提示短生命周期 subprocess 的主要本地成本不能被 warm p95 掩盖；它不是当前声明装配版的 SLA 数值。

这组历史数据证明全量扫描在当时 572 工具规模下可用，也暴露了初始化和分配仍需优化；当前 1,098 工具声明装配版必须重新建立 SLA。正式门禁必须以 release binary 独立进程测量 cold start，并以长生命周期进程测量多次 warm 请求。当前每次发现只需要一次本地 Search；只有真实端到端数据超预算后才评估 daemon/sidecar。

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

因此 Dense/Hybrid 只保留为历史实验对照，否决“纯 Dense 替换词法检索”和“外部 Provider 进入当前产品链路”。

### 4.4 阶段四：CLI 体积约束推翻内嵌模型

用户指出 DWS 是 CLI，90 MB 左右的模型过重。进一步检查确认：当前二进制约 44 MiB；内嵌模型还会引入 runtime、多平台构建和模型升级成本。当前 proxy 收益无法覆盖这些成本。

决策从“CLI 内建 Hybrid”修正为：

```text
DWS 本地 Exact + 轻量词法
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

本轮进一步核验 Anthropic SDK、OpenAI Agents SDK、NVIDIA NemoClaw、LangGraph BigTool、Semantic Kernel、Ratel、ToolUniverse、Haystack 和 ToolRet 的固定 commit。完整证据见 本文档 Part 3。

源码调研改变或强化了以下决策：

1. **Progressive disclosure 不是授权。** NemoClaw 明确保留完整 executor registry；隐藏工具名如果被猜中仍可能进入执行器。因此搜索/披露只优化上下文，ACL、确认、凭据和 sandbox 必须在执行层再次生效。
2. **外部 Provider 不进入当前方案。** 版本、认证、超时、融合与降级会显著扩大协议和故障面；历史原型已删除。
3. **Exact 被过滤时不得 fuzzy fallback。** 明确 canonical/CLI identity 命中但因 product/effect/exclude/availability 不可用，应返回 typed `exact_filtered`，不能推荐一个相似 sibling。
4. **索引更新必须按完整 snapshot 原子切换。** Ratel 在固定 commit 中实现了 BM25 corpus 变化后重建统计、Dense batch 先校验 model fingerprint/维度再提交；DWS 静态 Catalog 可在构造时一次完成，未来热更新选择更强的 build-then-swap 契约。
5. **资源预算不能只有 Top-K。** 公开接口还要限制 query、subquery 数、引用/响应 bytes、单 leaf Schema bytes 和 Host 累积已发现工具状态。
6. **外部项目的多路检索失败语义不能平移到 DWS。** Ratel Dense/Hybrid 和 Haystack 子检索失败都会抛错；当前 DWS 通过删除外部检索臂直接消除这类故障面。
7. **参数描述应按 arm 消融。** ToolUniverse keyword 的 docstring 直接证明作者因英文生物医学模板噪声只索引参数名，但其 embedding arm 又编码完整 JSON；仓库无消融，ASCII tokenizer 对中文零召回。它证明“字段策略可按通道不同”和作者设计理由，不证明中文 DWS 应排除参数描述。DWS 续测中加入参数描述仅 +0.50～0.66 pp 且 CI 跨 0，因此保持 shadow。
8. **生产投影若包含 `use_when` 会在当前 proxy 上泄漏。** 602 条 intent 正来自 `use_when`；续测含该字段时 TF-IDF/BM25 R@5 均为 100%。因此当前分支已把 `IncludeUseWhen` 默认翻转为 `false`，含该字段的结果只能标记为 leakage upper bound，不能进入选型。
9. **TF-IDF+Dense 只是历史实验。** 默认 RRF 的 proxy R@5 为 87.87%，dev sweep 最好 88.70%，但 product-cluster CI 跨 0，参数又在同一 proxy 上选择，不足以承担外部 Provider 的产品复杂度。
10. **ToolRet 只提供指标/数据方法论。** `Comprehensiveness@K = mean(Recall@K == 1)`、分级 qrels、instruction/no-instruction 和固定一阶段候选值得采用；固定 commit 的 BM25/ColBERT 路径存在直接崩溃/错误 qrels 等缺陷、无 tests、数据集未锁 revision 且全英文，不能复制 harness 或用于中文结论。

### 4.9 阶段九：五个独立 Reviewer 复核

按本轮要求另外启动了 5 个只读 reviewer，职责不重叠；RFC author 只在汇总阶段改文档：

| Reviewer | 审查边界 | 纳入 RFC 的关键修正 |
|---|---|---|
| GitHub Architecture | 固定 commit 与 DWS API 映射 | `exact_filtered` 终止；删除外部 Provider 边界；补齐 Invocation identity/evidence |
| Retrieval | Ratel、ToolUniverse、Haystack、ToolRet 源码 | 纠正“自动降级”“中文支持”“缓存新鲜度”和 benchmark 可复用性过度表述 |
| Recovery | Runner、SafetySpec、journal、probe | 发现成功状态过度推断、311/322 写工具幂等未知、journal 非原子与 Verify 缺口 |
| Evaluation v2 | 泄漏、统计、中文/算法公平性 | 要求 ProjectionVersion、Go/Python parity、cluster CI 和预注册门禁 |
| RFC Delivery | 协议可实施性与文档一致性 | 明确 Host/CLI stdin/stdout JSON、Search→Inspect hash closure、Contract owner/transport、唯一 normative DTO |

五方共同否决了“当前 Spike 已可直接公开”的结论：定向单测通过只证明实验内核可运行，不代表 Inspect 版本闭环或安全恢复已经实现。因此 RFC 状态保持 Proposed，实施顺序收敛为“本地内核 → local-only Search/Inspect → contradiction gate → 后续独立 execution/recovery RFC”。

### 4.10 阶段十：两种 Agent 编排范式复核

补充源码核验区分了两类形态：Anthropic/OpenAI/NemoClaw/BigTool 的模型显式 search，以及 Semantic Kernel 在模型调用前用近期消息自动注入。这些实现面向“多个外层函数工具”；DWS 不同，它在 Agent 侧本就是一个元工具。因此 DWS 选择显式 `tool-search.v1 → schema-inspect.v1 → DWS CLI Execute` 作为唯一规范路径，理由是 version handshake、失败语义和 SearchTrace 可直接审计。Skill/Agent 直接调用这条路径就能完成闭环；Host MAY 再用最近可信用户消息自动触发以省掉模型的一次 search decision，但：

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
                    └─ SearchIndex: local lexical retrieval
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

本地 Catalog availability 不等价于用户真实业务权限。Search v1 不接收“已授权”布尔值，也不输出 authorized。用户级 ACL/policy 必须由执行端基于当前 principal/tenant 和 policy revision 重新验证。

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

### 5.6 纯本地传输与排名边界

公开边界固定为 stdin/stdout JSON，不要求 Skill 或 Host import `internal/cli`：

```text
Agent/Host ── dws schema search ──→ local lexical ranking + CatalogVersionRef
                                      ↓
                                ToolReference
```

规范 v1 DTO：

```go
type CatalogVersionRef struct {
    SourceHash  string `json:"source_hash"`
    SurfaceHash string `json:"surface_hash"`
}

type ToolSearchV1Request struct {
    Version          string   `json:"version"`
    Query            string   `json:"query"`
    Subqueries       []string `json:"subqueries,omitempty"`
    Limit            int      `json:"limit"`
    CandidateLimit   int      `json:"candidate_limit"`
    ProductIDs       []string `json:"product_ids,omitempty"`
    Effects          []string `json:"effects,omitempty"`
    ExcludeCanonical []string `json:"exclude_canonical,omitempty"`
}
```

CLI transport 固定为 `dws schema search --request-json -`：stdin 只接受一个有总 bytes 上限的 `ToolSearchV1Request`，stdout 只写一个 `tool-search.v1` JSON envelope，诊断写 stderr。所有请求都只在当前进程的 immutable typed Catalog 上执行本地排名；相同 Catalog/query/config 必须逐字段一致。

当前方案明确不接受 `external_ranking`，不提供 `ToolSearchCandidateProvider`，不实现 RRF 融合，也不产生 `_fallback`、`degraded` 或 Provider warning code。这样删除远端认证、租户隔离、egress、deadline、版本对账、熔断和双路一致性等非必要故障面。未来若独立数据证明词法召回无法满足业务目标，应重新立 RFC，而不是在 v1 中保留未启用扩展点。

`CandidateLimit` 只控制本地词法候选深度。它必须作为 dev 调参后锁定的 SearchConfig；test 只运行一次，公开默认不得凭参考项目常量或历史 proxy best 决定。

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
  "rank_sources": ["fielded_bm25_ensemble"],
  "requires_inspect": true
}
```

Response 同时返回：

- `version: "tool-search.v1"`；
- `catalog: {source_hash, surface_hash}`；
- `strategy`；
- `abstained / truncated`；
- 多动作时的 `subqueries`。

这是规范 v1 ToolReference。完整 `use_when/avoid_when` 和参数 Schema 只在 Inspect 返回，避免轻量引用膨胀；contradiction gate 仍可在 DWS 内部消费它们。公开模型接口只返回 `rank`、`matched_fields` 和 `rank_sources`，不返回 raw `score/sparse_score`。当前分支已经从 JSON DTO 移除 raw score、自由文本 warning 和完整 use/avoid，只保留进程内未导出的排序分数。

当前分支已限制 query、subquery 数、summary 和总响应 bytes，并返回 `truncated/abstained`。普通 DWS 元工具路径每次 Inspect 后直接执行，不维护“已加载工具集”，因此不要把 Host 累积 Schema 状态当成本地发布前置条件。只有可选 Host adapter 把 Inspect 结果长期注入模型工具集时，才必须实现累积已发现工具、单个 compact Inspect 和任务内可见 Schema 总量预算，且按任务/步骤失效。

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

路径形态边界：`schema` 接受多个位置参数并按空格拼接成 **CLI path**，因此 `dws schema chat message read-status` 与加引号的 `dws schema "chat message read-status"`、`--cli-path "chat message read-status"` 完全等价。canonical path 含点号且必须作为单个参数整体传入（`dws schema chat.query_msg_read_status`）；把 canonical 按点号拆成多个 token 会走 CLI path 归一化而解析失败。Agent 应直接使用 Search 返回的 `canonical_path`（单 token）Inspect，用 `primary_cli_path` 执行，不要自行重排身份字符串。`ResolveQuery` 只做既有 canonical / CLI path / reviewed alias 的精确解析，不为拆分后的 token 组合新增身份形态。

### 5.9 多动作检索

Skill 或 Agent planner 负责把：

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
dev：算法、权重、k1/b/delta 调参
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

### 8.5 Alpha 门禁

以下是进入 Alpha 的最低条件，不是当前已达成声明：

| 门禁 | 要求 |
|---|---|
| Exact identity | 所有 canonical、primary CLI path、reviewed unique alias Top-1 100%；filtered exact 返回 typed refusal 且不返回 sibling |
| Determinism | 相同 Catalog/query/config 的非耗时输出逐字段相同 |
| Projection parity | 固定 ProjectionVersion 下 Go/Python exact、filter、Top-K 逐 case 一致 |
| Lightweight quality | 独立 test 上按预注册非劣效 margin 比较锁定 BM25；默认切换还需预注册最小增益；报告 product/tool-family cluster CI |
| 中文质量 | 成对中文口语、混 ASCII、英文和错别字 slice 分别达到预注册阈值与最小样本量 |
| Safety | excluded/unavailable 候选 0 泄漏 |
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
- Catalog source/surface hash；
- 多动作 round-robin；
- Python 多算法与中文评测。
- BM25 query term 固定顺序累加与 8 子进程 JSON golden；
- Go shipped-runtime 诊断对比与配对 Agent A/B 聚合器；
- 字段投影、轻量算法、纯词法/Dense RRF depth/k、cold build/warm query 续测。

当前测试覆盖搜索内核、公开 transport、双 hash Inspect 和跨进程确定性；benchmark 已记录。Go `LexicalRetriever`、BM25 control 与 TF-IDF shadow 已实现。它仍不是默认发布实现，因为倒排/分配优化、Go/Python parity 和正式独立 qrels 尚未完成。

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
- DWS Skill 按“已知命令直接执行；未知命令 Search → Inspect → Execute”进行 shadow；不要为已知高频路径增加 Search 往返。

### Phase 3：Contradiction gate Alpha

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
| 只按产品/分组手工导航 | 保留兼容 | 稳定但自然语言召回和复合任务不足 |
| 纯 Keyword/Jaccard | 否决为主方案 | R@5 明显落后；低 Forbidden 是低召回副作用 |
| BM25 固定为唯一算法 | 否决 | TF-IDF 当前略高且差异未定；接口应允许替换 |
| TF-IDF 立即替代 BM25 | 否决 | +1.50 pp CI 跨 0，且 qrels 同源 |
| CLI 内嵌 BGE/ONNX | 否决 | 模型比当前 CLI 大，跨平台和生命周期成本过高 |
| 远端 Dense 作为强依赖 | 否决 | 破坏离线可用性，引入延迟、隐私和可用性耦合 |
| Host 可选 CandidateProvider | 否决 | 增加认证、版本、超时、融合和失败语义，当前独立证据不足以覆盖复杂度 |
| LLM 直接在 1,098 工具中选择 | 否决 | 上下文大、不可确定、难以审计和回归 |
| 顶层 `dws discover` | 否决 | 与 endpoint/业务发现概念冲突，兼容面更大 |
| `dws schema search` | 接受为 Phase 2 | 与现有 Inspect 语义一致，但需单独处理 runnable parent/exclusion 兼容 |

## 11. 风险与开放问题

### 11.1 已知风险

- tool metadata 文案相似会造成 sibling confusion；
- `avoid_when` gate 可能误杀，需要 alternative gold；
- score 不同算法不可直接比较或设统一置信阈值；
- 复合任务拆解错误会让后续 ranker 无法补救；
- Catalog 当前缺少完整的 requires/produces/verifies 关系；
- 失败恢复不在当前分支范围内，不能把 Search 完成误写为安全恢复完成；
- `+shortcut` 已通过 reviewed exact exclusion group 纳入主线合并后的 Catalog 完整性处理；新增或删除 shortcut 仍必须同步复核该组，禁止通配排除。

### 11.2 待决策

1. 独立 test qrels 的实际人员签署与样本采集；owner 角色、最小规模、hash 和双签冻结流程已写入 `scripts/testdata/tool_search_eval_manifest.json`；
2. TF-IDF 与 BM25 shadow 的线上采样比例；
3. contradiction gate 先用规则、Host LLM 还是两阶段组合；
4. `requires/produces/verifies` 放入哪个 reviewed Schema input；
5. Host SDK 如何把已实现的 `catalog_changed` discovery error 映射为跨语言错误类型；
6. Contradiction gate 采用规则、Host LLM 还是两阶段，以及 alternative gold/误杀率 owner；
7. 多动作响应预算固定 Top-5，还是按 action 数受控扩到最多 10；
8. 公开 CLI 的 cold subprocess p95 目标。

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

本 RFC **不锁定** BM25、TF-IDF 或字段权重。当前建议是 BM25 作为 Alpha control，TF-IDF/BM25+ shadow；独立 holdout 和真实 Agent 灰度通过后再确定默认。DWS 基础 CLI 始终保持零模型依赖和离线可用。

---

# Part 2: DWS 工具检索与排序源码实现调研


> 选型更新（2026-08-12）：本文记录前期源码调研和候选参数，不代表最终
> DWS CLI 方案。后续多算法、中文切片、CLI 体积和多 Agent 独立审计否决了
> “内嵌 Dense”与“锁死 BM25”的假设。2026-08-13 又进一步删除了外部
> CandidateProvider、RRF 与 fallback；Dense/Hybrid 只保留为历史实验。
> 正式决策、整体收益和决策过程见
> 本文档 Part 1；
> 固定 commit 的源码架构复核见
> 本文档 Part 3。

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
  → 可替换本地轻量词法召回
  → 确定性特征重排 → Top-K ToolReference
                      └→ 多工具集合补全
```

推荐分阶段落地：

1. **第一版先做“硬过滤 + 精确匹配 + 字段化 BM25 + 稳定 Top-5”**。本地、无模型、容易回归，也能直接利用现有人工评审元数据。
2. **Dense/RRF 与 Cross-encoder 只保留离线研究**。当前产品协议不预留 Provider/fallback 扩展点；如未来重启必须另立 RFC。
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

### 3.3 OpenAI Codex CLI

上述“没有本地 ranker”只适用于 Agents SDK/托管 API，不适用于当前 Codex CLI。[Codex @ `b1373b7`](https://github.com/openai/codex/tree/b1373b74a27d1d9b65074a873202683355cae772) 已实现完整 Rust 本地 Tool Search：[handler](https://github.com/openai/codex/blob/b1373b74a27d1d9b65074a873202683355cae772/codex-rs/core/src/tools/handlers/tool_search.rs)使用 BM25，[projection](https://github.com/openai/codex/blob/b1373b74a27d1d9b65074a873202683355cae772/codex-rs/tools/src/tool_search.rs)索引 namespace、工具名/描述和递归参数 Schema，命中后把完整 deferred Schema 写入 `tool_search_output`。

这证明 Host 原生 Search → load 闭环可以工程化，但不证明其 ranker 适合 DWS：Codex 使用 `Language::English`，没有 DWS 的 Exact Guard/`exact_filtered`、Catalog 双 hash、查询级 hard filter 和显式 stable tie-break。更重要的是执行模型不同：Codex 搜索多个外层工具，而 DWS 本身是一个元工具。DWS 的 Search 结果应双 hash Inspect 后映射回同一 CLI 命令空间，不需要复制 Codex 的动态 Registry。

### 3.4 MCP

[MCP Tools 规范](https://modelcontextprotocol.io/specification/draft/server/tools)中的 `tools/list` 负责枚举、分页、缓存和变更通知，不定义自然语言检索。客户端要自行建设 Catalog 搜索。

[MCP 客户端最佳实践](https://modelcontextprotocol.io/docs/2026-07-28/develop/clients/client-best-practices)推荐 Search → Inspect → Execute，并列出 Regex/BM25、Embedding、小模型和混合检索。这意味着 DWS 不应修改 MCP 的 `tools/list` 语义来塞入模糊排序，而应提供独立 `search_tools` 或本地 Schema 搜索能力。

### 3.5 AWS AgentCore Registry

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

建议保留 `SchemaIndex.Resolve` 的精确、无推断契约，新建独立检索层。本节是方案演进记录，不再定义第二套公开 DTO；规范 `tool-search.v1`、`ToolReference` 与 `schema-inspect.v1` 以 RFC 5.6～5.8（本文档 Part 1，56-纯本地传输与排名边界） 为准。

建议分成以下组件，避免以后更换模型时重写 CLI：

```text
CatalogSnapshot
  ├─ PolicyFilter
  ├─ ExactRetriever
  ├─ BM25Retriever
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
4. 排序输出必须确定：相同语料、配置和 query 跨进程返回相同 Top-K 成员和顺序。

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

4. Constraint Rerank
   - `use_when` 一致性加分
   - `avoid_when` 冲突降分/淘汰
   - 明确 product、effect、对象类型约束
   - 同分以 canonical path 升序

5. Workflow Completion
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

本仓库已完成第一轮无泄漏实测，完整协议、代码、指标、置信区间与失败案例见 《DWS 工具检索排序实测报告》（本文档 Part 4）。主要结果是：字段 BM25 + Dense RRF 的 Intent Recall@5 为 85.88%，比字段 BM25 高 3.65 个百分点；Exact Guard 可把融合后下降的 canonical/CLI Top-1 恢复到 100%；人工正确拆解工作流后 Comprehensiveness@5 从 50% 提升到 80%。同时，Dense 单独没有优于 BM25，当前字段权重也没有提高单意图召回，因此二者都不能未经独立验证就直接设为生产默认。

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
- 本地轻量算法的默认切换必须在固定独立测试集上达到预注册门槛；
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

### Phase 3：矛盾门禁与工作流覆盖

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
3. **不做融合**：Dense/RRF/Provider/fallback 不进入当前产品方案。
4. **确定性**：所有路径统一 `(score desc, canonical asc)`，先稳定排序全候选再截断。
5. **可用性**：本地词法索引更新原子化，不引入远端检索依赖。
6. **负向语义**：`avoid_when` 进入冲突门禁和评测，不进入正向全文。
7. **多工具任务**：把 Comprehensiveness@5 和 plan completeness 设为一等指标。
8. **可信与恢复**：搜索结果只说明“候选相关”，Inspect 确认契约，Execute 返回结构化回执，Verify/Recovery 使用 effect、risk、confirmation、idempotency 单独治理。

这样可以同时支撑三个目标：

- **让 Agent 找得到**：Exact + 本地轻量词法 + 多工具补全；
- **让返回结果可信**：唯一 Catalog、来源 hash、权限硬过滤、match reasons、Inspect 和执行后验证；
- **让失败后安全恢复**：索引原子更新保证检索一致性；业务幂等/回执/状态机属于后续独立执行 RFC。

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

---

# Part 3: GitHub 源码架构调研


- 状态：Research / 固定 commit 源码核验
- 日期：2026-08-12
- 关联 RFC：本文档 Part 1
- 排序评测：本文档 Part 4
- 调研范围：工具目录、渐进披露、检索、融合、索引生命周期、Agent 编排、安全边界与失败语义

## 1. 结论先行

GitHub 上没有一个项目可以被 DWS 整体照搬。首先要区分执行模型：Codex/Anthropic/OpenAI 的 deferred search 面向多个外层函数工具，而 **DWS 本身就是 Agent 的一个元工具**，Catalog 条目是 DWS 子命令。因此 DWS 不需要动态将搜索结果注册成新 Host Tool；可复用的是不同项目已经验证过的检索和契约边界：

1. **协议形态参考 Anthropic / OpenAI 的 reference → inspect。** DWS 搜索只返回轻量引用，命中后才读完整命令契约；但 Inspect 后仍调用同一 DWS，不动态注册新外层工具。
2. **Codex 证明本地 Tool Search 可以做到 Host 级闭环，但不应照搬其 ranker。** 当前 Codex 已有 Rust 本地 BM25、namespace、MCP/Apps 和 deferred Schema 回填；但它是英文管线，没有 DWS 的 Exact Guard、`exact_filtered`、Catalog 双 hash 与稳定 canonical tie-break。
3. **检索内核重点参考 Ratel 的本地部分。** 最有价值的不是固定 `k1/b`，而是不可变索引快照、全量重建 BM25 统计和稳定 tie-break；Dense/RRF 仅作为研究对照，不进入当前 DWS 产品链路。
4. **上下文和状态预算参考 NVIDIA NemoClaw。** query、返回文本、已发现工具数、checkpoint 状态和可见 Schema 都应有确定性上限；同时必须明确“披露不是授权”。
5. **中文与字段策略必须由 DWS 自己评测。** ToolUniverse 甚至因模板噪声而只索引参数名、不索引参数描述，说明“字段越全越好”不成立。
6. **删除外部 Provider 故障面。** Ratel 和 Haystack 都会把某些 Dense/子检索器错误向上抛出；DWS 当前不需要为可选增益引入认证、超时、融合和降级协议，因此已删除 Provider/fallback 原型。
7. **搜索可信不等于执行可信。** GitHub 参考实现基本不处理 DingTalk 权限、确认、幂等、未知受理状态和补偿；这些必须继续由 DWS 的 Inspect、Cobra、Runner 和 recovery 分层承担。

建议目标架构保持：

```text
reviewed CommandRegistry + Cobra + metadata
                    ↓ one-way resolution
             immutable SchemaRegistry snapshot
              ├─ identity resolve / Inspect index
              └─ local lexical index
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
| [OpenAI Codex @ `b1373b7`](https://github.com/openai/codex/tree/b1373b74a27d1d9b65074a873202683355cae772) | 生产 Host 级本地 Tool Search | [handler](https://github.com/openai/codex/blob/b1373b74a27d1d9b65074a873202683355cae772/codex-rs/core/src/tools/handlers/tool_search.rs)使用 Rust BM25；[tool projection](https://github.com/openai/codex/blob/b1373b74a27d1d9b65074a873202683355cae772/codex-rs/tools/src/tool_search.rs)索引 namespace/name/description/参数并把完整 deferred Schema 写入 `tool_search_output`；接通 MCP、Apps 和动态 registry | `Language::English`；无 Exact Guard/`exact_filtered`、Catalog 双 hash、查询级 product/effect/exclude filter 和显式 stable tie-break；该外层 registry 闭环不是 DWS 元工具路径的必需依赖 |
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

Search 不得重新调用 `tools/list` 建第二份目录，也不得自己生成 executable identity。候选必须只来自当前 typed Catalog。

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

新 snapshot 完整构建和验证成功后再原子替换；Search 与 Inspect 必须看见同一代。

### 4.4 Hybrid/RRF 调研结论：不进入当前方案

Ratel 的 Hybrid 是 BM25 和 Dense 各自召回到 depth 100，再用 RRF 融合。Haystack 也先并发取回各路列表，再融合。共同结论是：Dense 只能重排 sparse Top-N 时，无法补回 sparse miss，不是真正的 Hybrid。

Ratel 固定 commit 的 `k1=.9/b=.4`、RRF `k=60`（0-based rank）与 depth=100 都有源码/注释证据；Haystack 私有函数用 `k=61` 表达 1-based 论文公式，数学等价，但 MultiRetriever 没暴露 weights，最终排序也没有 doc-id tie-break。它们证明实现方式，不证明 DWS 中文参数最优；DWS 已测到 sparse arm/depth/k 会改变 R@K 和 workflow，必须自行锁 dev/test。

但两者的错误语义都不能照搬：

- Ratel `Semantic/Hybrid` 直接返回可捕获的 `EmbedderError`；
- Haystack 任一路 retriever 出错会让整个 `MultiRetriever` 失败。
- Codex 对空 query/`limit=0` 返回 `RespondToModel`，对不支持的 payload 返回 `Fatal`；它没有可失败 Provider arm，因此也没有“保留本地候选、只标记降级”的对等契约。
- Gemini CLI 公开文档定义了 discovery command/MCP 注册和 include/exclude，但未定义外部检索失败后返回稳定本地候选的协议；不应在没有对应源码/契约证据时简化成“Gemini 直接报错”。

DWS CLI 的离线约束通过单一本地检索链满足：

```text
local lexical success → bounded ToolReference
runtime Catalog assembly corrupt → fail closed, search unavailable
caller context canceled → return cancellation
```

历史 Provider/fallback 原型已经删除。Dense/RRF 实验仍说明多路检索应采用独立深召回，而不是只重排 sparse Top-N；但这些研究事实不再构成 DWS v1 的实现要求。若未来独立评测证明必须引入第二路召回，应通过新 RFC 重新证明收益能够覆盖新增故障面。

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
- source/surface hash；
- 默认 Top-5 轻量 `ToolReference`；
- 多动作 round-robin 合并。

与 GitHub 参考实现对照后发现的差距及当前分支状态如下。

### 5.1 已删除：Provider 与外部排名

当前公开边界只有 versioned stdin/stdout JSON 和本地排名。`external_ranking`、`ToolSearchCandidateProvider`、RRF、provider timeout/bulkhead、`_fallback`、`degraded` 与 provider warning codes 均已删除。双 hash 只用于 Search→Inspect 的 Catalog 一致性闭环。

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
- 不得把搜索结果或 Agent identity 用作授权依据。

### 5.4 已简化：无外部错误协议

本地检索没有 Provider 错误或降级状态。响应超过 8 KiB 时只按完整 reference 边界截断并设置 `truncated=true`，不再为这一单一状态维护 warning code 数组。

### 5.5 部分修复：资源预算与可观测状态

当前分支已增加 query 256 scalars/2 KiB、最多 8 个 subquery、summary 256 scalars、response 8 KiB，以及 `truncated/abstained`。仍缺少 Host 侧累计 discovered/schema bytes：

- query/subquery/response bytes 上限；
- `truncated`、`abstained`；
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

这项修复关闭了当前本地词法排名的已知不确定性。外部 Provider 已不在方案范围内。

## 6. 推荐的稳定接口

规范 DTO 只保留一套，以 RFC 5.6 纯本地传输与排名边界（本文档 Part 1，56-纯本地传输与排名边界）、5.7 ToolReference（本文档 Part 1，57-toolreference） 和 5.8 Inspect（本文档 Part 1，58-inspect） 为准。跨进程只允许依赖 `tool-search.v1` JSON。`dws schema search --request-json -` 和双 hash Inspect 已在当前分支实现，DWS Skill/Agent 可以直接调用该链路；不依赖 Host 实现动态 Schema 注入。

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
- Catalog/本地索引损坏必须 fail closed；
- 用户权限和 destructive confirmation 不能由 Search 决定。

## 7. 可信执行与安全恢复：GitHub 实现没有替 DWS 解决的部分

开源项目主要优化“模型看到哪些工具”和“如何排序”，没有证明 DingTalk 写操作能安全恢复。规范 `InvocationContractV1`、hash canonicalization、跨进程 evidence 和数据治理以 RFC 6.1 Invocation Contract（本文档 Part 1，61-invocation-contract） 为准；本报告不再定义第二套简化 DTO。

执行与恢复规则：

1. **执行前再验证。** Inspect hash、当前登录身份、权限、参数约束、confirmation 和 real Cobra leaf 必须同时有效。
2. **错误分类不等于未执行。** timeout/connection reset 可能发生在服务端已受理之后；未知受理状态的非幂等写禁止自动重放。
3. **幂等必须有真实后端能力。** Catalog 标注 `idempotent` 不能凭空制造服务端幂等；只有后端接受且回显 idempotency key 时才自动重试写。
4. **先 Verify 再 Retry。** 创建/发送后优先用 receipt、资源 ID 或查询接口验证；确认未发生才重试。
5. **Compensate 必须逐工具 reviewed。** 只有有明确反向操作、资源 ID、权限和确认契约时才能补偿；否则转人工。
6. **恢复使用原始 Catalog contract。** 若版本已经变化，旧 Invocation 只能按审计/验证处理，不能静默套用新 Schema 重放。

## 8. 推荐实施顺序

### Slice A：搜索内核契约

- `exact_filtered` 和 query/response/subquery 预算已完成；
- Go `LexicalRetriever`、BM25 control 和 TF-IDF shadow 已完成；
- 公开 transport 已提前实现，但默认启用仍遵循 Slice C/D 门禁。

### Slice B：索引门禁

- TF-IDF、BM25 共享 tokenizer/filters/tie-break；
- 固定 ProjectionVersion 和 Go/Python parity；

### Slice C：公开 local-only `dws schema search`

- 已实现 Search 的薄 CLI 包装和严格 stdin JSON；
- 已同步 `schema` leaf/exclusion；skills 和 release command-surface 门禁仍需复核；
- 已增加 expected source/surface hash 校验并在 compact Inspect 回传双 hash；
- 不新增顶层 `dws discover`。

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
| Budget | query、subquery 数、单 reference、总 response、Inspect Schema 超限 |
| Security | hidden/disclosed 状态不改变 Runner ACL/confirmation；猜工具名仍被执行层拒绝 |
| Index lifecycle | 新 snapshot 构建失败保留旧 snapshot；版本切换原子化 |
| Future Recovery（独立 RFC） | read、幂等写、非幂等写、unknown acceptance、verify-success、manual handoff |

## 10. 最终选型

DWS 不选择某一个 GitHub 项目作为基础框架，而是组合经过源码验证的模式：

| 能力 | 采用的模式 | DWS 的增强 |
|---|---|---|
| Deferred disclosure 协议 | Anthropic/OpenAI 的 defer → reference → load 形态 | DWS 不动态注册新外层工具；reference 通过双 hash Inspect 映射回同一 DWS 命令空间 |
| Agent 编排 | BigTool/Codex 的显式 Search → load/bind → execute | DWS 元工具内的 versioned Search → Inspect → CLI Execute；校验 unknown/stale ID，权限仍在执行层 |
| 自动触发（可选） | Semantic Kernel 的 recent-context retrieval | Skill 已能完成规范链路；Host adapter 只作为减少一次模型决策的可选 optimization，必须复用同一 Search API/Trace |
| Namespace | OpenAI 的 qualified identity / namespace | 作为 reviewed hint 或已知产品 filter；未知、复合任务保留 global/multi-namespace recall |
| 索引生命周期 | Ratel 的完整 BM25 统计、原子 Dense batch/rebuild | Catalog generation 绑定、build-then-swap；不照搬英文 tokenizer/model 或固定 `k1/b/depth` |
| 多路召回与融合 | Ratel/Haystack 的研究对照 | 当前否决，不进入 DWS v1；稳定 `(score desc, canonical asc)` 仅用于本地词法排名 |
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
  → 确定性排序与预算
  → ToolReference
  → schema-inspect.v1 hash check
  ── 当前 Tool Search RFC 边界 ──
  ⇢ Future RFC: Cobra Execute → reviewed Verify / Retry / Compensate
```

这套架构的收益不是单纯提高 R@5，而是同时缩小模型上下文、保持 CLI 离线可用、让搜索结果可追溯，并把误重试写操作的风险隔离在明确的执行/恢复契约之外。

---

# Part 4: 工具检索排序实测报告


> 独立审计勘误（2026-08-12）：本报告的 Intent query 与索引中的
> `agent_summary` 来自同一批 selection 作者，属于内部 proxy set，不是独立
> test qrels；case-level bootstrap 支持 Hybrid 在当前微平均样本上的增益，
> 但 product-cluster bootstrap 区间约为 -0.40～+7.03 pp，不能表述为跨产品
> 稳定。文中的 “BM25F” 实际实现是各字段独立 BM25 后加权求和，正式名称
> 应为“字段 BM25 分数集成”。最终选型和结论强度以
> 本文档 Part 1
> 为准。

> 评测日期：2026-08-12；主线合并后复测：2026-08-13
>
> 分支：`test/tool-search-ranking-eval`
>
> 当前 Catalog：1,098 个工具，`source_hash=sha256:02c633c075f4af915ea097d383bdcbc25fe549c172e2d3336152e30cf2906e80`，`surface_hash=sha256:7f9839f1ff428b52f1d032d2fa6493815c3755650b7d9fa653839e7d589482e5`
>
> 评测代码：[`scripts/dev/eval_tool_search_ranking.py`](../scripts/dev/eval_tool_search_ranking.py)
>
> 2026-08-14 最新审计与优化路线：本文档 Part 5

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

---

# Part 5: Schema Search 调研、实测与优化方案（2026-08-14）


## 1. 决策摘要

当前 `test/tool-search-ranking-eval@e9efd3e9` 已具备可发布实现的主要结构：

- 同一 typed `SchemaRegistry` 驱动 Search、Inspect 与 Schema 导出；
- canonical / primary CLI / alias 的 NFKC Exact Guard；
- product、effect、exclude、`AgentExecutable` hard filter；
- 中文 unigram+bigram、ASCII/CamelCase/identifier 保留的 fielded BM25；
- 最多 5 个轻量 `ToolReference`，再用 source/surface 双 hash Inspect；
- 2 KiB query、64 KiB request、8 KiB response、Top-K/Candidate/Subquery 上界；
- 跨进程确定性、Catalog 绑定、响应预算和 `catalog_changed` 测试；
- TF-IDF/action shadow、独立 qrels evaluator 和真实 Agent A/B 聚合器。

因此不建议更换检索内核或引入 Dense/RRF。最优路线是：保留 `Exact → hard filter → fielded BM25 → Inspect`，先补独立评测和冷启动工程，再决定 action rerank、contradiction gate 或 OpenAI Host adapter。

当前状态只能定为 **unreleased Alpha / Proposed**。同源 proxy 证明实现可用和契约可靠，但不能证明真实用户召回率、Agent 任务成功率或线上 token 收益。

## 2. 可复现实测

### 2.1 测试环境与输入

- macOS arm64，Apple M3 Pro；
- Catalog：27 个产品、1,098 个工具；
- `source_hash=sha256:02c633c075f4af915ea097d383bdcbc25fe549c172e2d3336152e30cf2906e80`；
- `surface_hash=sha256:7f9839f1ff428b52f1d032d2fa6493815c3755650b7d9fa653839e7d589482e5`；
- 1,123 条预算内同源 intent，另有 10 条超预算；
- 1,390 条同源 `avoid_when` negative；
- 10 条人工 workflow；
- 独立 qrels：`state=collecting`，当前 0 条；
- 真实配对 Agent A/B：未运行。

复现命令：

```bash
make tool-search-evaluation-harness
go test ./internal/app -run '^$' \
  -bench '^BenchmarkToolSearchDelivery(ChineseQuery|DecomposedWorkflow|IndexBuild)$' \
  -benchmem -count=3
```

### 2.2 质量与契约结果

| 指标 | 当前默认 `fielded_bm25_ensemble` | action shadow |
|---|---:|---:|
| R@1 / R@5 / MRR@5 | 65.27% / 88.16% / 0.7441 | 62.51% / 84.24% / 0.7131 |
| 纯中文 R@5（402） | 83.83% | 79.85% |
| 中文混 ASCII R@5（721） | 90.57% | 86.69% |
| workflow 整句 Complete@5 / required recall | 40% / 61.67% | 50% / 76.67% |
| reviewed 拆解 Complete@5 / required recall | 80% / 90% | 90% / 95% |
| Forbidden@1 / @5 | 0% / 0.0719%（1/1,390） | 0% / 0.0719%（1/1,390） |

身份与完整性结果：

- 1,098 canonical、1,098 primary CLI、19 alias、1,098 NFKC、1,098 `exact_filtered`：全部通过；
- 5,801 个响应：Catalog 绑定失败、unknown candidate、ineligible candidate、response budget violation 均为 0；
- Search + gold Inspect 平均 4,507.03 bytes；相对 17,876,084-byte 全量 compact Schema 减少 99.9748%；
- 上述 byte 比较直接 Inspect gold，即使搜索 miss 也不计恢复成本，是容量上界，不是实际 Agent token 或任务收益。

默认 BM25 在同源 proxy 上比 action shadow 的 R@5 高 3.92 pp，纯中文高 3.98 pp。action 只在 10 条人工 workflow 上更完整，样本和拆解都不足以支持切换默认。

### 2.3 性能结果

完整 1,098 工具 Catalog 的 Go benchmark：

| 路径 | 时间 | 分配 |
|---|---:|---:|
| warm 单意图 Search | 2.12～2.19 ms/op | 270.7 KB，4,606 allocs |
| warm 两 subquery workflow | 5.03～5.10 ms/op | 573.5 KB，9,956 allocs |
| 已缓存 Catalog 后重建 Search index | 89.5～93.0 ms/op | 62.0 MB，约 575k allocs |

单意图 warm search 已低于 RFC 的 10 ms 目标。主要成本是 Catalog/索引构建和短命 CLI 进程，不是排序本身。

同 flags 的 stripped PIE binary：

- 当前分支：28,537,074 bytes；
- `upstream/main@0a063e3e`：28,300,322 bytes；
- 增量：236,752 bytes（约 231 KiB），低于 1 MiB 目标。

30 次交错 cold subprocess 诊断中，当前 Search p50 为 1.27 s；当前与 upstream overview p50 均约 1.20 s。三组 p95 都出现 7 秒以上的共同系统长尾，因此本轮不能把 p95 当作稳定 SLA。应在隔离 runner 上预热后串行测量至少 100 次，并单列 CPU、RSS、Catalog assembly、index build 和 JSON encode。

## 3. 本轮发现并修复的问题

### 3.1 指标漂移

旧文档把 Forbidden@5 写成 0%，实际生成报告为 `1/1390`；RFC 还混用了旧 572/602 基线的中文、负例和 context 数字，并把 action R@5 差值写成 0.27 pp。已按当前 Go 报告修正为：

- Forbidden@5：0.0719%；
- 默认与 action R@5 差：3.92 pp；
- 纯中文差：3.98 pp；
- 当前中文切片和导航 byte 数以 1,098 工具报告为准。

后续应让文档表格由 comparison JSON 生成或校验，避免手工数字再次漂移。

### 3.2 非法实验配置静默毒化检索

复现：

```bash
DWS_TOOL_SEARCH_K1=NaN dws schema search --query '查询群消息已读状态'
DWS_TOOL_SEARCH_B=Inf dws schema search --query '查询群消息已读状态'
```

修复前两者均退出 0，并返回空候选 `abstained=true`；负字段权重也会直接参与排序。现在 engine build 会拒绝：

- 非有限或负 `k1`（0 仍表示沿用默认值，解析后的值必须为正）；
- 非有限或不在 `[0,1]` 的 `b`；
- 非有限或负字段权重；

同时新增了相应回归测试。实验开关仍可保留，但必须 fail closed，不能把配置错误伪装成“没有合适工具”。

### 3.3 缺少 release-scale benchmark

旧 `internal/cli` benchmark 只覆盖 4 个 fixture 工具，历史文档又引用 572 工具 Spike。现已新增完整 declaration-assembled 1,098 工具 Catalog 的单意图、workflow 和 index-build benchmark，后续性能变化可直接在真实规模上回归。

## 4. 优化路线

### P0：发布证据与契约（Alpha 前）

1. **独立 qrels**
   - 由非 Catalog selection 作者采集和签署；
   - train/dev/test 隔离；test 解封前预注册算法、参数、阈值和统计方法；
   - 纯中文、中文混 ASCII、英文各至少 100 条；
   - 至少 80 条 2～4 步 workflow、100 条 hard negative；
   - 覆盖全部产品、read/write/destructive、graded equivalent、sibling confusion、forbidden + alternative gold。

2. **恢复机器可执行的 sealed gate**
   - 当前 manifest 允许 `state=collecting` 和 0 case 通过，只能说明结构合法；
   - 增加独立 `release` 命令/target：要求 qrels 非空、签署/hash/coverage 完整，并执行预注册 threshold；
   - proxy harness 保持 diagnostic，不能以绿色 CI 暗示发布质量已通过。

3. **协议回归保持强制**
   - Exact/filtered exact 100%；
   - hard-filter 泄漏 0；
   - Search response 不含完整参数 Schema，`requires_inspect=true`；
   - 双 hash mismatch 必须 `catalog_changed` 且不得执行；
   - query/request/response/subquery/Top-K 预算与跨进程确定性持续为 blocker。

4. **指标自动同步**
   - 从 `.worktrees/policy-tmp/tool-search-comparison.json` 生成 Markdown 数据块，或提供检查脚本；
   - 文档引用必须携带 Catalog hashes、算法名和 case count；
   - 禁止把历史 Python、当前 Go、同源 proxy、独立 test 混在同一“当前结果”表中。

### P1：性能和中文召回（Alpha 期间）

1. **索引生命周期优先于微调 scorer**
   - 在长生命周期 Host 内每个 Catalog hash 只构建一次 index；
   - CLI cold path 若仍是主要入口，测量并优化 declaration assembly 与 index projection；
   - 先减少 index-build 的 62 MB/575k alloc，再优化 2 ms query 的 map/string 分配；
   - 不以 daemon/sidecar 为默认，除非隔离 cold benchmark 与真实调用频率证明收益覆盖运维成本。

2. **结构化倒排候选召回**
   - 当前 1,098 文档全扫描已达 2 ms，不必立即引入复杂依赖；
   - 当 Catalog 或并发扩大时，再把每字段 token → sorted canonical posting 建成 immutable index；
   - exact、eligible-domain filter、score-desc/canonical-asc tie-break 和逐 case结果必须与当前实现 parity；
   - 目标是减少 query allocations，不改变排名语义。

3. **只在独立 dev 集调 tokenizer/权重**
   - 优先分析纯中文 miss、同产品 sibling 和错别字；
   - 保持 `use_when` 默认不入索引，避免答案句泄漏；
   - action rerank 继续 shadow。只有独立 test 的 product-cluster CI 同时满足非劣效和最小增益，才允许切换默认；
   - 不新增 Dense/RRF，除非独立数据证明净收益且覆盖包体、跨平台、下载、缓存、失败降级成本。

4. **多动作分解由真实 Agent 评测**
   - 当前 reviewed subquery 是人工上界；
   - 测量自动分解准确率、Complete@5、依赖顺序、跨步参数传递和额外工具；
   - 每个动作保留候选预算，最终集合默认最多 5，受控扩大最多 10；
   - 不把 round-robin recall 等同于可执行计划正确。

### P2：Host 集成与真实收益（默认开启前）

1. **OpenAI native adapter（可选）**
   - OpenAI Tool Search 支持 hosted 与 client-executed 形态、deferred functions/namespaces/MCP，并把新工具定义注入上下文末尾以保留 cache；
   - DWS 可将 `Search → 双 hash Inspect` 得到的完整 ToolSpec 转为标准 `tool_search_output`；
   - Host 支持 Tool Search 时走 native adapter，不支持时继续命令式 Search/Inspect；
   - adapter 必须复用 DWS canonical identity、Catalog hashes、hard filter 和执行期 policy，不得产生第二检索或授权事实源；
   - capability namespace 按 `product.capability` 分组并控制在约 10 个函数以内，避免按整个产品制造超大 namespace。

2. **真实配对 Agent A/B**
   - 相同模型、prompt、Catalog、权限和调用预算；
   - 每个 case/trial 同时跑 `direct_schema` 与 `search_inspect`；
   - 报告任务完成、参数一次通过、minimal plan、错误/forbidden tool、错误写入、人工接管、模型 token、调用数、延迟和成本；
   - 按 case/product/tool-family 聚类 bootstrap；
   - 只有任务质量不退化且 token 或成本达到预注册改善，才默认开启。

3. **contradiction 与执行安全保持分层**
   - `avoid_when` soft penalty 不是权限或安全 gate；
   - 有 independent alternative gold 后再实现 typed contradiction decision，报告 false removal；
   - Search 永不授权、确认或执行。ACL/availability/confirmation/idempotency/业务校验仍由执行路径负责。

## 5. 明确不做

- 不把同源 88.16% R@5 写成线上召回或任务成功率；
- 不因 10 条人工 workflow 的 action shadow 优势切换默认；
- 不将 `avoid_when` 原句匹配得到的低 Forbidden 暴露解释为安全保证；
- 不直接从 Search 引用猜参数或执行；
- 不新增 Catalog JSON、Provider、Dense、RRF 或第二套 identity/authorization source；
- 不用 warm 4-tool benchmark 替代 release-scale 或 cold subprocess 数据；
- 不把 full Schema byte reduction 直接换算成模型 token/cost reduction。

## 6. 完成定义

Schema Search 可从 Proposed 升为 Alpha，至少需要：

- P0 独立 qrels 与 sealed release gate 全部通过；
- 当前 contract/trust/integrity blocker 保持绿色；
- 完整 Catalog warm p95 <10 ms，隔离 cold subprocess 阈值已预注册并通过；
- binary 增量 <1 MiB；
- workflow、英文和 hard-negative 最小样本量达到 RFC；
- 文档指标与生成报告自动一致。

可默认开启还必须额外通过真实同模型配对 Agent A/B。Host adapter、action rerank、倒排优化和 contradiction gate 都是可独立灰度的增量，不能替代上述发布证据。

---

# Part 6: Tool Search 十轮 Review 与收敛记录


日期：2026-08-13
基线：`upstream/main@5fed80fc0f1aaa4ee9123b0acf3041d0d9b45bc9`
范围：Go Tool Search、`tool-search.v1`、`schema-inspect.v1`、评测门禁、主线迁移与文档；旧 Sandbox/Recovery 不在当前实现范围。

## 结论

十轮 review 已完成。发现的实现 blocker 均已修复并增加回归覆盖；独立 qrels 仍处于 `collecting`，所以公开 Alpha/默认开启门禁会按设计 fail closed。当前完成的是可运行、可评测、可审计的 Go 框架，不把同源 proxy 或旧模型 smoke 写成线上收益。

## 十轮清单

| 轮次 | 审查面 | 主要发现 | 收敛结果 |
|---|---|---|---|
| 1 | 召回与排序 | exact filtered 可能落回 fuzzy；复合 query 结果合并缺少 fail-closed | exact identity 先 resolve；filtered 立即终止；多动作任一 exact filtered 全局 abstain |
| 2 | 中文与算法 | BM25 不是已证明最优；TF-IDF/BM25+ 差异区间跨 0；中文短 query 是残余盲区 | Go 保留 BM25 control、TF-IDF shadow；Dense/Provider 否决；短 query 必须进入独立 slice，不在同源长句上盲调 |
| 3 | 确定性与性能 | map 浮点累加可漂移；旧 572 warm 数据不能代表当前 1,098 Catalog | BM25/TF-IDF 固定累加顺序并跨进程 golden；补当前 release cold/RSS/包体诊断 |
| 4 | CLI/DTO | 全局 fields/jq/format 被静默忽略；公开 validation 缺稳定 reason | Search 固定完整 JSON envelope；所有 transport/version/budget 输入错误均返回 typed reason |
| 5 | Search→Inspect/Provider | Inspect 与 RFC envelope 不一致；Provider 显著扩大版本、timeout/panic 和失败语义 | 实现双 hash `schema-inspect.v1`；随后删除 Provider/RRF/fallback 全链路 |
| 6 | 安全与预算 | Bidi 可回显；8 KiB 未计 newline；摘要静默截断 | RejectControlChars；按最终 wire bytes 计预算；`truncated_fields` 可观察 |
| 7 | 测试/race | 手工恢复全局 seam 违反 policy；alias/NFKC 未穷举 | 使用 `testseam.Swap`；增加 alias/NFKC trust；删除 Provider 并发故障面 |
| 8 | RFC/调研一致性 | 旧 572、旧模型 smoke、当前 1,098 混写；Recovery 被画成当前能力 | 历史/当前结果分栏；Recovery 明确为 Future RFC；Catalog 统一为 Go 声明运行时装配 |
| 9 | 主线迁移与发布门禁 | merge/revert ancestry 带入 5 个旧 Harness commit；评测 freeze 未绑定 Go 源码 | 从 upstream/main 重建干净同名分支；旧历史、Sandbox/Recovery 分支归档；Go ranker/gate source hash 冻结 |
| 10 | 最终交付审计 | 独立 qrels gate 仅格式/自声明 coverage；缺 graded/forbidden/sibling/workflow/相对 control 判据 | 增加 Go graded qrels evaluator、产品聚类 paired bootstrap、BM25 非劣效、语言/安全/workflow 阈值；coverage 从 Catalog/qrels 派生 |

## 关键回归矩阵

- Exact：canonical、primary CLI、reviewed alias、NFKC、`exact_filtered`。
- Budget：query、aggregate subquery、request、summary、完整 response 与 encoder newline 边界。
- Security：危险 Unicode、ToolReference 不携带参数 Schema/执行载荷。
- Evaluation：graded Recall/MRR@5/NDCG@5、Forbidden/alternative、sibling exposure、2～4 步 workflow、BM25 paired product-cluster CI。
- Delivery：构建时动态 Catalog、strict command surface、normal/race、全仓 tests、policy、build/vet。

## 未被伪装成完成的事项

- 独立 qrels 还未由业务与独立 Evaluation owner 封存；英文 ≥100、workflow ≥80 等数据门禁尚未满足。
- 当前 Forbidden@5 同源诊断仍高，不能默认开启。
- 旧 `gpt-5.6-sol` 规划 smoke 绑定 572 工具 Catalog，已判过期。
- 恢复/补偿/自动重放属于后续独立 RFC；当前 Search 不授权、不确认、不执行。
- “发群文件”等中文短 query 是已知研究切片，必须通过独立标注与消融决定 unigram/词典/expansion，不能在当前 proxy 上直接调参。
