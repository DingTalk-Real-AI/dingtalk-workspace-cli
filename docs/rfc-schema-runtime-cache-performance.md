# RFC 附件：CLI 性能提升报告

状态：Draft。日期：2026-09-06。设计见 [单入口 CLI 与 Schema Runtime Cache RFC](rfc-schema-runtime-cache.md)。

## 1. 结论

Darwin/arm64 本地完整矩阵显示，当前实现相对 PR-base main 在七个代表场景全部更快：

| 场景 | main p50 | candidate p50 | 提升 | native RSS p50 |
|---|---:|---:|---:|---:|
| Schema leaf | 1508.36 ms | 292.97 ms | **80.6%** | 341.34 → 36.18 MiB（89.4%） |
| root help | 405.24 ms | 302.23 ms | **25.4%** | 47.82 → 45.88 MiB（4.1%） |
| version | 407.07 ms | 285.67 ms | **29.8%** | 47.83 → 31.05 MiB（35.1%） |
| leaf help | 1505.69 ms | 294.64 ms | **80.4%** | 342.97 → 41.65 MiB（87.9%） |
| calendar list dry-run | 406.82 ms | 291.51 ms | **28.3%** | 48.10 → 40.03 MiB（16.8%） |
| config list | 408.96 ms | 285.62 ms | **30.2%** | 47.96 → 30.89 MiB（35.6%） |
| calendar list mock | 407.56 ms | 291.51 ms | **28.5%** | 48.45 → 40.45 MiB（16.5%） |

同轮 DWS native 的 wall p50 在 Schema/help/version/leaf-help/dry-run 上比 Lark native 快 **7.5%～12.6%**；npm public 入口的 wall p50 比 Lark public 快 **5.3%～8.2%**。完整 Lark 诊断为 **37/40** 指标通过：help RSS 三项未过，且本地 30 样本未达到该诊断器要求的 100 样本，因此不能表述为整体追平 Lark。DWS 也尚未追平 GWS：native wall p50 慢约 **2.5～2.7 倍**，public 慢约 **50%～54%**。

这份本地矩阵使用实现完成但尚未提交的工作树候选，因此是方向和回归判断证据，不是最终 release proof。Ready 结论以推送后 clean head 的 Darwin/arm64、Linux/amd64 CI artifact 为准。

## 2. 方法与可比边界

- CPU：Apple M3 Pro；OS：Darwin/arm64；Go：1.25.9。
- 固定 main：`6f71222b9b07c760cdb5f376b24dab9155e62094`。
- candidate、main、Lark CLI 1.0.85、GWS 0.22.5 每场景 30 次随机交错延迟样本，另做 30 次独立内存样本，共 2,640 次正式 invocation。
- native RSS 来自小型 C parent 在 `wait4` 读取的进程峰值；public RSS 是 1 ms 采样的同时进程树峰值，包含 Node wrapper 与 native child。
- 每个场景先锁定 stdout/stderr oracle，正式样本必须逐字节一致；错误退出不能算快速成功。
- Schema cache/live、default telemetry/DO_NOT_TRACK 分开。竞品命令只保证意图相近，不保证字段、鉴权和输出合同相同，因此竞品是诊断，不是 release gate。
- 本机本轮存在约 280 ms 的 DWS 进程启动/安全扫描共同地板；它同时影响 candidate 与 main，配对提升有效，但绝对毫秒不能与先前约 10 ms 的 run 混用。

五维报告完成状态为 `complete: true`，`failures: {}`。本地原始报告 SHA-256：`60ab19f6d5c34b6436e0615be9827cbc93653283fb494390ec10ec4048e10374`；该文件将在 clean-head CI 中重新生成并作为 workflow artifact 上传。

`check-cli-lark-performance.py` 对该报告给出 37/40 observed metrics pass、`accepted: false`。原因是 native help RSS p50、public help RSS p50/p95 三项高于 Lark，并且每场景只有 30 个样本；该脚本要求 100。表中的 wall 延迟结论成立，但不能替代完整竞品诊断结论。

## 3. Schema

| 路径 | wall p50 / p95 | user CPU p50 | RSS p50 / p95 |
|---|---:|---:|---:|
| candidate cache hit | 292.97 / 300.58 ms | 26.12 ms | 36.18 / 37.33 MiB |
| candidate live assembly | 1188.15 / 1223.82 ms | 1527.00 ms | 308.13 / 316.23 MiB |
| fixed main | 1508.36 / 1576.56 ms | 1770.93 ms | 341.34 / 364.80 MiB |

缓存命中相对同候选 live assembly 的 user CPU 降低 **98.3%**，RSS p50 降低 **88.3%**。它证明 verified protobuf cache 解决了 Schema 构建成本。缓存仍不是业务 handler；普通命令不会因为 cache 存在而绕过框架。

## 4. Help 与 version

独立 default/opt-out 入口矩阵的 12 个 gate 全部通过：candidate 的 help/version p50、p95 均不超过固定 main 的 105%。

| 场景 | candidate default p50 / p95 | candidate opt-out p50 / p95 | main default p50 / p95 |
|---|---:|---:|---:|
| root help | 302.22 / 324.39 ms | 301.52 / 323.34 ms | 403.49 / 435.35 ms |
| version | 283.50 / 299.37 ms | 284.30 / 301.20 ms | 410.90 / 435.83 ms |

candidate 的 default 与 opt-out 基本重合，支持“事件入队后不等待最后一次发送”的因果判断。main 的 default 比 opt-out 多约 112～122 ms；具体差值受当时网络响应影响，不能把 300 ms timeout 当固定 sleep。

version 的 user CPU p50 从约 49.95 ms 降到 17.38 ms，RSS 从 47.83 MiB 降到 31.05 MiB，说明收益也来自 utility 路径不构建产品树，不只是 telemetry。

## 5. 业务命令

| 场景 | main wall p50 / p95 | candidate wall p50 / p95 | p50 提升 | candidate RSS p50 |
|---|---:|---:|---:|---:|
| calendar leaf help | 1505.69 / 1561.17 ms | 294.64 / 310.65 ms | **80.4%** | 41.65 MiB |
| calendar list dry-run | 406.82 / 433.73 ms | 291.51 / 306.65 ms | **28.3%** | 40.03 MiB |
| calendar list mock | 407.56 / 442.56 ms | 291.51 / 308.26 ms | **28.5%** | 40.45 MiB |
| config list | 408.96 / 462.30 ms | 285.62 / 297.63 ms | **30.2%** | 30.89 MiB |

这些场景覆盖真实命令树、flag、PreParse、validation、Safety/dry-run 或 mock 输出，但不包含有真实凭据的远端 RPC。网络服务时延和 authenticated throughput 不在这份启动报告中。

core 构树微基准进一步分离了装配贡献：

| 构造路径 | ns/op | B/op | allocs/op | 相对完整树 |
|---|---:|---:|---:|---:|
| 完整 root | 14.0～14.4 ms | 17.1 MB | 169k | 100% |
| process calendar list | 0.84～0.86 ms | 1.17 MB | 10.1k | 时间约 6.0%，alloc 约 5.9% |
| process config get | 0.35～0.38 ms | 0.46 MB | 3.7k | 时间约 2.6%，alloc 约 2.2% |

因此业务收益有两个来源：单入口和 telemetry no-wait 消除固定税；统一框架按需装配减少 core 内无关产品构建。早期约 7.1 ms 的选择性数据仍先执行了全量 helper 工厂；修复后 calendar 降至约 0.85 ms。Prepare/config/profile 未删除：调用计数显示单 profile 一次、dry-run 零次，未发现值得承担生命周期风险的重复点。

## 6. 整体运行内存

| 产品/入口 | Schema | help | version | leaf help | dry-run |
|---|---:|---:|---:|---:|---:|
| DWS native | 36.18 MiB | 45.88 MiB | 31.05 MiB | 41.65 MiB | 40.03 MiB |
| DWS npm public | 73.08 MiB | 85.19 MiB | 56.73 MiB | 76.70 MiB | 74.42 MiB |
| Lark native | 45.40 MiB | 45.65 MiB | 44.91 MiB | 45.16 MiB | 45.57 MiB |
| Lark public | 79.40 MiB | 79.38 MiB | 77.62 MiB | 77.71 MiB | 80.03 MiB |
| GWS native | 10.12 MiB | 8.44 MiB | 8.33 MiB | 10.88 MiB | 11.23 MiB |
| GWS public | 42.62 MiB | 41.92 MiB | 41.89 MiB | 42.14 MiB | 42.21 MiB |

DWS native 已低于或接近 Lark；DWS public 的 help 高约 7.3%，其余表中场景低约 1.3%～26.9%。GWS 仍有明显的 runtime 与命令面体积优势。

Schema live assembly 是例外高峰：candidate 约 308 MiB。cache hit 将它降至约 36 MiB；未启用 identity 的平台或首次重建仍会承担 live 峰值，报告不能隐去这条冷路径。

## 7. Lark 与 GWS 对比

### Native wall p50

| 场景 | DWS | Lark | DWS 对 Lark | GWS | DWS 对 GWS |
|---|---:|---:|---:|---:|---:|
| Schema | 292.97 ms | 324.37 ms | 快 9.7% | 114.45 ms | 慢 2.56× |
| help | 302.23 ms | 326.84 ms | 快 7.5% | 113.45 ms | 慢 2.66× |
| version | 285.67 ms | 327.01 ms | 快 12.6% | 113.21 ms | 慢 2.52× |
| leaf help | 294.64 ms | 325.43 ms | 快 9.5% | 114.09 ms | 慢 2.58× |
| dry-run | 291.51 ms | 329.66 ms | 快 11.6% | 112.68 ms | 慢 2.59× |

### Public wall p50

| 场景 | DWS npm | Lark public | DWS 对 Lark | GWS public | DWS 对 GWS |
|---|---:|---:|---:|---:|---:|
| Schema | 503.89 ms | 541.43 ms | 快 6.9% | 331.31 ms | 慢 52.1% |
| help | 511.19 ms | 539.72 ms | 快 5.3% | 331.86 ms | 慢 54.0% |
| version | 495.71 ms | 540.06 ms | 快 8.2% | 330.58 ms | 慢 50.0% |
| leaf help | 506.77 ms | 539.87 ms | 快 6.1% | 332.07 ms | 慢 52.6% |
| dry-run | 503.48 ms | 539.41 ms | 快 6.7% | 331.78 ms | 慢 51.8% |

Lark 在本轮同机环境中主要承担 Node/CLI 初始化地板；DWS 通过单进程 native 路径和按需构树已经在 wall 延迟上略快。help RSS 仍是明确差距。GWS 的 native executable 启动 CPU 只有约 3～4 ms，RSS 约 8～11 MiB，说明它的程序映像、runtime 和命令面更小。DWS 若继续追 GWS，需要分析静态依赖/init、Go runtime 映像和业务框架常驻对象；继续压 Schema protobuf 已不是首要方向。

## 8. 验收状态

- [x] 本地固定 main default/opt-out 入口 gate：12/12 通过。
- [x] 本地五维报告：44 场景，延迟与内存各 30 样本，`complete: true`、零失败。
- [x] 相对 main 的业务命令 p50 全部下降，代表业务下降超过 40% 的门槛由 leaf help 满足。
- [x] 本地 DWS native/public 五个可比场景的 wall p50 均快于 Lark。
- [ ] 完整 Lark 诊断：当前 37/40；help RSS 三项和 100 样本要求未通过，因此不得宣称整体追平。
- [ ] clean PR head Darwin/arm64 native CI。
- [ ] clean PR head Linux/amd64 native CI。
- [ ] 正式 release 最终签名制品与安装验证。
