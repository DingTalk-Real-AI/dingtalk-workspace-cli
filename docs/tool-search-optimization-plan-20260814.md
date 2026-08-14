# Schema Search 调研、实测与优化方案（2026-08-14）

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
