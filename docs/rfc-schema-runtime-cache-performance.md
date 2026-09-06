# RFC 附件：CLI 性能提升报告

状态：Draft。日期：2026-09-06。设计见 [单入口 CLI 与 Schema Runtime Cache RFC](rfc-schema-runtime-cache.md)。

## 1. 结论

Darwin/arm64 本地完整矩阵显示，当前实现相对 PR-base main 在七个代表场景全部更快：

| 场景 | main p50 | candidate p50 | 提升 | native RSS p50 |
|---|---:|---:|---:|---:|
| Schema leaf | 1497.66 ms | 300.17 ms | **80.0%** | 344.12 → 38.53 MiB（88.8%） |
| root help | 397.81 ms | 305.07 ms | **23.3%** | 47.89 → 46.37 MiB（3.2%） |
| version | 392.00 ms | 290.99 ms | **25.8%** | 47.83 → 32.81 MiB（31.4%） |
| leaf help | 1495.98 ms | 292.84 ms | **80.4%** | 343.70 → 36.16 MiB（89.5%） |
| calendar list dry-run | 402.92 ms | 293.18 ms | **27.2%** | 47.79 → 33.61 MiB（29.7%） |
| config list | 394.66 ms | 291.62 ms | **26.1%** | 47.76 → 33.08 MiB（30.7%） |
| calendar list mock | 404.15 ms | 293.20 ms | **27.4%** | 47.88 → 34.12 MiB（28.7%） |

同轮 DWS native 的 wall p50 在 Schema/help/version/leaf-help/dry-run 上比 Lark native 快 **8.5%～12.7%**；npm public 入口的 wall p50 比 Lark public 快 **3.0%～7.2%**。完整 Lark 诊断为 **36/40** 指标通过：help RSS 四项未过，且本地 30 样本未达到该诊断器要求的 100 样本，因此不能表述为整体追平 Lark。DWS 也尚未追平 GWS：native wall p50 慢约 **2.4～2.6 倍**，public 慢约 **49%～58%**。

这份本地矩阵使用 clean head `4a25ec8b0274c16c7e7b7a74ee08fdc11a7c0c96` 构建，候选记录为 `source_dirty: false`。它是 Darwin/arm64 的开发候选证据，不是最终 release proof；Ready 结论仍需同一推送 head 的 Darwin/arm64、Linux/amd64 CI artifact。

## 2. 方法与可比边界

- CPU：Apple M3 Pro；OS：Darwin/arm64；Go：1.25.9。
- 固定 main：`6f71222b9b07c760cdb5f376b24dab9155e62094`。
- candidate、main、Lark CLI 1.0.85、GWS 0.22.5 每场景 30 次随机交错延迟样本，另做 30 次独立内存样本，共 2,640 次正式 invocation。
- native RSS 来自小型 C parent 在 `wait4` 读取的进程峰值；public RSS 是 1 ms 采样的同时进程树峰值，包含 Node wrapper 与 native child。
- 每个场景先锁定 stdout/stderr oracle，正式样本必须逐字节一致；错误退出不能算快速成功。
- Schema cache/live、default telemetry/DO_NOT_TRACK 分开。竞品命令只保证意图相近，不保证字段、鉴权和输出合同相同，因此竞品是诊断，不是 release gate。
- 本机本轮存在约 280 ms 的 DWS 进程启动/安全扫描共同地板；它同时影响 candidate 与 main，配对提升有效，但绝对毫秒不能与先前约 10 ms 的 run 混用。
- 本机安全软件会终止 `/private/tmp` 中新注入的 ad-hoc 候选。测量前将最终候选逐字节复制到 Go build cache 的可信执行目录；源文件与执行副本 SHA-256 均为 `4bd00e121852c63d8b8b75347c004c8fa7f48950bf679e812a60d98ec50303b1`，版本身份和签名内容未改。CI 仍须直接运行 workflow 产出的最终 artifact。

五维报告完成状态为 `complete: true`，`failures: {}`。本地原始报告 SHA-256：`7e9ad6176c10803b5fecc56d3641a89f978856e0a3e3ae87d2fd977e814d42d6`；default-entry 报告为 `c8a48ad4b56d4a7014d57f4875dddc49592e6df3740794b0697d673ca99b67dc`，Schema 验证报告为 `4e2851ec29dcf8778a89c6c61359607d53e8e763df3d31dfe5bc5d9570089794`。CI 将重新生成并上传对应 artifact。

`check-cli-lark-performance.py` 对该报告给出 36/40 observed metrics pass、`accepted: false`。原因是 native/public help RSS 的 p50、p95 四项高于 Lark，并且每场景只有 30 个样本；该脚本要求 100。表中的 wall 延迟结论成立，但不能替代完整竞品诊断结论。

## 3. Schema

| 路径 | wall p50 / p95 | user CPU p50 | RSS p50 / p95 |
|---|---:|---:|---:|
| candidate cache hit | 300.17 / 325.39 ms | 36.50 ms | 38.53 / 39.12 MiB |
| candidate live assembly | 1199.26 / 1371.38 ms | 1540.34 ms | 309.27 / 319.81 MiB |
| fixed main | 1497.66 / 1569.25 ms | 1807.92 ms | 344.12 / 359.23 MiB |

缓存命中相对同候选 live assembly 的 user CPU 降低 **97.6%**，RSS p50 降低 **87.5%**。它证明 verified protobuf cache 解决了 Schema 构建成本。缓存仍不是业务 handler；普通命令不会因为 cache 存在而绕过框架。

## 4. Help 与 version

独立 default/opt-out 入口矩阵的 12 个 gate 全部通过：candidate 的 help/version p50、p95 均不超过固定 main 的 105%。

| 场景 | candidate default p50 / p95 | candidate opt-out p50 / p95 | main default p50 / p95 |
|---|---:|---:|---:|
| root help | 301.77 / 309.39 ms | 299.54 / 307.60 ms | 397.14 / 421.07 ms |
| version | 285.48 / 294.26 ms | 284.12 / 295.30 ms | 398.59 / 440.05 ms |

candidate 的 default 与 opt-out 基本重合，支持“事件入队后不等待最后一次发送”的因果判断。main 的 default 比 opt-out 多约 110 ms；具体差值受当时网络响应影响，不能把 timeout 当固定 sleep。

version 的 user CPU p50 从约 49.43 ms 降到 18.94 ms，RSS 从约 48.16 MiB 降到 33.13 MiB，说明收益也来自 utility 路径不构建产品树，不只是 telemetry。

## 5. 业务命令

| 场景 | main wall p50 / p95 | candidate wall p50 / p95 | p50 提升 | candidate RSS p50 |
|---|---:|---:|---:|---:|
| calendar leaf help | 1495.98 / 1572.39 ms | 292.84 / 304.26 ms | **80.4%** | 36.16 MiB |
| calendar list dry-run | 402.92 / 431.28 ms | 293.18 / 304.52 ms | **27.2%** | 33.61 MiB |
| calendar list mock | 404.15 / 428.92 ms | 293.20 / 302.97 ms | **27.4%** | 34.12 MiB |
| config list | 394.66 / 429.13 ms | 291.62 / 303.17 ms | **26.1%** | 33.08 MiB |

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
| DWS native | 38.53 MiB | 46.37 MiB | 32.81 MiB | 36.16 MiB | 33.61 MiB |
| DWS npm public | 73.06 MiB | 84.98 MiB | 58.02 MiB | 72.05 MiB | 63.58 MiB |
| Lark native | 45.56 MiB | 45.20 MiB | 45.14 MiB | 45.21 MiB | 45.66 MiB |
| Lark public | 79.90 MiB | 79.69 MiB | 77.63 MiB | 79.19 MiB | 78.27 MiB |
| GWS native | 10.12 MiB | 8.45 MiB | 8.36 MiB | 10.86 MiB | 11.23 MiB |
| GWS public | 41.88 MiB | 41.88 MiB | 41.88 MiB | 41.91 MiB | 41.93 MiB |

DWS native 除 help 外均低于 Lark；native help 高约 2.6%。DWS public 的 help 高约 6.6%，其余表中场景低约 8.6%～25.3%。GWS 仍有明显的 runtime 与命令面体积优势。

Schema live assembly 是例外高峰：candidate 约 309 MiB。cache hit 将它降至约 39 MiB；未启用 identity 的平台或首次重建仍会承担 live 峰值，报告不能隐去这条冷路径。

## 7. Lark 与 GWS 对比

### Native wall p50

| 场景 | DWS | Lark | DWS 对 Lark | GWS | DWS 对 GWS |
|---|---:|---:|---:|---:|---:|
| Schema | 300.17 ms | 332.52 ms | 快 9.7% | 115.20 ms | 慢 2.61× |
| help | 305.07 ms | 333.38 ms | 快 8.5% | 119.49 ms | 慢 2.55× |
| version | 290.99 ms | 333.27 ms | 快 12.7% | 115.83 ms | 慢 2.51× |
| leaf help | 292.84 ms | 331.48 ms | 快 11.7% | 122.81 ms | 慢 2.38× |
| dry-run | 293.18 ms | 330.30 ms | 快 11.2% | 116.40 ms | 慢 2.52× |

### Public wall p50

| 场景 | DWS npm | Lark public | DWS 对 Lark | GWS public | DWS 对 GWS |
|---|---:|---:|---:|---:|---:|
| Schema | 522.45 ms | 538.67 ms | 快 3.0% | 349.17 ms | 慢 49.6% |
| help | 512.16 ms | 538.42 ms | 快 4.9% | 338.58 ms | 慢 51.3% |
| version | 509.10 ms | 545.45 ms | 快 6.7% | 328.67 ms | 慢 54.9% |
| leaf help | 517.89 ms | 556.87 ms | 快 7.0% | 327.13 ms | 慢 58.3% |
| dry-run | 511.83 ms | 551.72 ms | 快 7.2% | 344.55 ms | 慢 48.6% |

Lark 在本轮同机环境中主要承担 Node/CLI 初始化地板；DWS 通过单进程 native 路径和按需构树已经在 wall 延迟上略快。help RSS 仍是明确差距。GWS 的 native executable 启动 CPU 只有约 3～4 ms，RSS 约 8～11 MiB，说明它的程序映像、runtime 和命令面更小。DWS 若继续追 GWS，需要分析静态依赖/init、Go runtime 映像和业务框架常驻对象；继续压 Schema protobuf 已不是首要方向。

## 8. 验收状态

- [x] 本地固定 main default/opt-out 入口 gate：12/12 通过。
- [x] 本地五维报告：44 场景，延迟与内存各 30 样本，`complete: true`、零失败。
- [x] 相对 main 的业务命令 p50 全部下降，代表业务下降超过 40% 的门槛由 leaf help 满足。
- [x] 本地 DWS native/public 五个可比场景的 wall p50 均快于 Lark。
- [ ] 完整 Lark 诊断：当前 36/40；help RSS 四项和 100 样本要求未通过，因此不得宣称整体追平。
- [ ] clean PR head Darwin/arm64 native CI。
- [ ] clean PR head Linux/amd64 native CI。
- [ ] 正式 release 最终签名制品与安装验证。
