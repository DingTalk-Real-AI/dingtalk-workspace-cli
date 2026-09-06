# PR #1296 性能专项实施记录

状态：实施中，**尚未超过 Lark CLI**。用户已决定保持统计 SDK 异步发送和最多 300 ms 的退出等待，不改变字段、投递方式或业务结果生命周期。

本批从 `3dc56c48ad6c84be9f628ff56fd9328d1e41d61e` 开始。以下是 Darwin/arm64 Apple M3 Pro、Go 1.25.9 的本机开发诊断；候选构建如实标记 `source_dirty=true`，不能替代最终提交的原生 CI、Linux、public 或发布证据。[生产变更指纹与失败记录](benchmarks/schema-cache/optimization-3dc56c48/evidence.json)。

## 已实施

1. `schemaruntime/cache_codec.go`：JSON 校验成功时不再为每个 provenance winner/candidate 格式化错误位置；仅失败时构造相同的诊断。仍校验所有候选，包括不属于已知展示字段的 provenance。
2. `schemaruntime/model.go`：将 provenance 值与刚由 `json.Marshal` 成功生成的最终值比较；字节一致时已经证明其 JSON 有效，省去重复扫描。不同编码仍走原语义比较；未选中候选、被覆盖候选和唯一 winner 规则保留。
3. `corecmd/contract/types.go`：已经完整缓冲的 Result JSON 使用 `json.Unmarshal`，省去流式 Decoder 的读缓冲与第二次 Decode。RawMessage 保留嵌套数字和字段顺序，仍拒绝多对象、null 和尾随垃圾。
4. `shortcut/userdef/loader.go`：没有自定义 YAML 时直接返回，避免复制整个内置 Shortcut 注册表和构建冲突索引。有自定义命令时仍走原框架。
5. 新增真实 core 哈希 benchmark 和独立 Lark 指标检查器。没有改制品认证、SDK、输出格式、业务执行路由或 launcher capability allowlist。

## 组件结果

改前/改后测试二进制在相同独立 HOME 中交替执行三轮，正式样本不启用 profile。下表取三轮中位数；不是独立进程的端到端耗时。时间单位 ms，分配为十进制 MB/op。

| 场景 | 时间：前 → 后 | 时间下降 | 分配：前 → 后 | 分配下降 |
|---|---:|---:|---:|---:|
| Meta 解码 | 2.053 → 2.069 | 无改善 | 2.599 → 2.599 | 无改善 |
| calendar 分片校验/解码/索引 | 4.390 → 3.140 | 28.5% | 3.760 → 3.257 | 13.4% |
| calendar 文件打开/认证/解码/索引（已认证 Meta） | 5.374 → 3.921 | 27.1% | 4.321 → 3.819 | 11.6% |
| 完整 Schema 校验/解码/索引 | 129.160 → 96.053 | 25.6% | 102.546 → 89.448 | 12.8% |
| 完整命令树装配 | 14.426 → 14.347 | 未见明显改善 | 17.751 → 16.586 | 6.6% |

[汇总](benchmarks/schema-cache/optimization-3dc56c48/component-summary.json)及[逐轮命令/顺序](benchmarks/schema-cache/optimization-3dc56c48/component-order.json)。目录内保存每轮原始 benchmark 输出。B/op 是累计分配，不是整体进程峰值 RSS。

真实约 41.5 MB finalized core 的 Go SHA-256（含打开/关闭，warm 文件）约 16–18 ms。32 KiB、128 KiB、1 MiB 缓冲没有稳定加速，增大缓冲增加分配；因此没有修改认证读法。[原始结果](benchmarks/schema-cache/optimization-3dc56c48/core-hash.txt)。

## 安装包诊断与未完成项

两个候选包的 Schema identity 完全一致，包括 Source/Surface、Meta/shard 摘要与 BuildID。逐次输出比较也没有发现漂移。测量使用真实 launcher 和 finalized core，而非裸 `go build`。

本轮第二次尝试收齐 17 场景 × 改前/改后 × 30 次的 **1,020 个耗时样本**：opt-out 完整导出 p50 从约 1,105 ms 降到 1,008 ms（8.8%）；opt-out 单叶从 75.62 ms 到 73.86 ms（2.3%）；默认单叶从 488.61 ms 到 482.88 ms（1.2%）。普通命令和 help/version 暂无稳定改善，个别场景变慢，不能用组件 28% 代替默认体验收益。

这些本机进程时间明显高于历史原生 CI 数量级。还出现两次 SIGKILL：第一次在初次 Schema 预热，第二次在新编译的内存采样器首次执行；没有退出错误文本。签名校验和随后直接 core/launcher 探测通过，固定路径的内存采样器探测也通过，但尚未确定被杀原因。**没有删除失败并标绿；整份配对报告保持 incomplete，内存仍缺测。** [失败与原始耗时样本](benchmarks/schema-cache/optimization-3dc56c48/native-paired-incomplete.json)。后续内存测量独立归档，不改写这次失败记录。

## 超过 Lark 的判定

新工具 `scripts/dev/check-cli-lark-performance.py` 消费原五维报告，重新从 raw samples 计算 native/public 的 Schema、root help、version、leaf help、dry-run 的 wall 与 RSS p50/p95，共 40 项/平台。每项必须严格小于 Lark；缺样本、默认环境缺失/opt-out、制品或依赖摘要漂移、内存方法错配、public 未同时观测 wrapper 与子进程、NaN/Inf 都不能通过。

单平台至少 100 次/场景/阶段。该检查器只判断一个报告；不能替代计划要求的两平台、三轮、最终安装包以及业务正确性验收。现有 CI 仍按原 RFC 门槛运行；运行成功不表示 Lark 目标通过。

历史 `34018840739` 的 30 次报告仅可展示观测值：[Darwin](benchmarks/schema-cache/optimization-3dc56c48/lark-darwin-baseline.json) 11/40 项、[Linux](benchmarks/schema-cache/optimization-3dc56c48/lark-linux-baseline.json) 8/40 项更低；样本数也不足，两个报告都明确未通过。GWS 继续作为对照，不参与用户本轮“超过 Lark”的完成判定。

## 验证与下一步

- 已通过：Schema reader/fastpath/runtime 与 CLI 测试；corecmd 全包与 userdef 测试；Schema 合同门禁（31 产品、1,370 工具）；全量 Go suite；Lark 检查器反例测试。
- 正在完成：独立内存测量、最终提交的两平台原生 CI。
- 仍未完成：默认入口超过 Lark、普通命令回退消除、public 对比、两平台三轮完整矩阵及发布验证。
- 下一步按 profile 继续减少普通命令初始化与元数据分配。保持当前上报行为；若等待构成默认场景的下界，明确保留未通过指标，不改比较口径。
