# Tool Search 十轮 Review 与收敛记录

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
