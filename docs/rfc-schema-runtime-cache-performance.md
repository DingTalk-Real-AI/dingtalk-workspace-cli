# RFC 附件：Schema、help、命令与进程内存性能报告

日期：2026-09-06。候选：`7cbf7f529d347bd0e9750687138dd2be009c5bcb`，源码树：`9dfbbca99267c7eaca3e6fe81fc1748888d3d682`。状态：当前 RFC 口径下的双平台开发候选验收通过；候选本身 `release_eligible:false`，正式签名、公证、安装升级与整包回滚仍由主 RFC R8 阻挡首次官方发布。

关联：[主 RFC](rfc-schema-runtime-cache.md) · [PR #1296](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pull/1296) · [原生 CI 34018840739](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/actions/runs/34018840739) · [本轮证据索引](benchmarks/schema-cache/native-7cbf7f52/evidence.json)。

## 1. 性能提升多少

| 维度 | 双平台实测结论 |
|---|---|
| Schema | 相对固定 pre-PR 基线，native wall p50 **下降 78.19%–84.57%**；相对同一候选的实时装配，用户态 CPU p50 **下降 97.39%–97.88%** |
| 根 help | 相对固定 pre-PR 基线，默认上报五维样本的 wall p50 **下降 7.92%–11.61%**；独立入口 gate 的 default/opt-out p50、p95 八项全部通过 |
| 命令 | version p50 **下降 7.75%–11.77%**，日历列表 leaf help p50 **下降 77.40%–83.79%**；config、dry-run、mock p50 **增加 8.39%–12.74%**，不能宣称所有命令加速 |
| 整体运行内存 | 相对固定基线，native Schema 峰值 RSS p50 **下降 87.69%–88.99%**，root help **下降 69.42%–76.21%**；config、dry-run、mock 增加约 **2.51%–4.47%**，不存在统一的“全 CLI 内存下降百分比” |
| Lark CLI / GWS | DWS 在已测五类 native/public wall p50、p95 中均更慢；root help/version 的 native RSS 低于 Lark、高于 GWS。Schema 优化没有证明竞品领先 |

这些结论只覆盖预热元数据、无真实凭据的短命令。它们不外推到远端 RPC、认证、冷缓存首次修复或长期事件进程，也不代表尚未发布的正式安装包。

## 2. 验收结果与测量合同

原生 workflow `34018840739` 的六个 job 全部成功：Darwin/arm64 与 Linux/amd64 native candidate、两平台完整 Go suite、Schema declaration policy、跨平台 identity metadata coordinator。两个五维报告均满足：

- `complete:true`，`failures:{}`；
- 51 个 latency case 与 51 个 memory case，每个 case 恰好 30 个样本；
- 默认入口报告 `passed:true`，12 个正式 gate 全部为 true；
- package version、identity environment、en/zh finalized core help proof 全部通过；
- candidate source commit/tree 精确匹配，`source_dirty:false`；
- 候选 package、public package 和固定竞品安装在测量前后的摘要不变。

| 项目 | 固定口径 |
|---|---|
| pre-PR 基线 | `5243e5ca19b55a3e785e5cc09273b653ad5381dc`，在各 native host 以相同 Go 1.25.9、CGO=0、PIE 与 runtime payload 构建 |
| 平台 | Linux amd64，4 CPU；macOS 26.6.2 arm64，3 CPU |
| 工具 | Go 1.25.9、Node 22.16.0、Lark CLI 1.0.85、GWS 0.22.5 |
| 矩阵 | 每平台 51 场景；每场景 30 次独立耗时与 30 次独立内存试验；随机交错，预热不计样本 |
| 默认环境 | 独立 HOME、预热缓存、无真实凭据；五维耗时保留各产品默认上报，网络未统一隔离 |
| 耗时 | 原生父进程阻塞 `wait4`，从子进程启动到回收；p50 为中位数，p95 为排序后第 `ceil(n×0.95)` 项 |
| native 内存 | 小型 C 父进程 `exec` 后读取内核 `wait4` peak RSS，避免 Python 自身 pre-exec RSS；native case 均为单进程负载 |
| public 内存 | 1 ms 请求间隔采样同时驻留的 wrapper + child 进程树 RSS；共享页可能重复，是观测下界，不是 PSS |
| 业务范围 | Schema 发现、root/leaf help、version、配置查询、日历列表 dry-run；DWS 另测本地 mock，不执行真实业务 RPC |

## 3. 相对固定 pre-PR 入口的净变化

耗时为默认上报条件下的 native p50；内存为独立 `wait4` 峰值 RSS p50。

| 平台 | 场景 | 基线 p50 ms | 候选 p50 ms | 耗时变化 | 基线 RSS MiB | 候选 RSS MiB | RSS 变化 |
|---|---|---:|---:|---:|---:|---:|---:|
| linux | schema | 2432.95 | 375.45 | 下降 84.57% | 350.70 | 43.19 | 下降 87.69% |
| linux | help | 346.19 | 305.99 | 下降 11.61% | 51.29 | 12.20 | 下降 76.21% |
| linux | version | 345.78 | 305.08 | 下降 11.77% | 51.34 | 12.05 | 下降 76.52% |
| linux | leaf-help | 2435.48 | 394.70 | 下降 83.79% | 349.42 | 55.35 | 下降 84.16% |
| linux | config | 346.38 | 390.51 | 增加 12.74% | 50.83 | 53.10 | 增加 4.47% |
| linux | dry-run | 346.19 | 389.67 | 增加 12.56% | 50.98 | 52.95 | 增加 3.85% |
| linux | mock | 346.51 | 390.33 | 增加 12.65% | 51.44 | 53.16 | 增加 3.35% |
| darwin | schema | 1689.57 | 368.49 | 下降 78.19% | 346.77 | 38.19 | 下降 88.99% |
| darwin | help | 347.76 | 320.22 | 下降 7.92% | 45.65 | 13.96 | 下降 69.42% |
| darwin | version | 346.37 | 319.54 | 下降 7.75% | 45.58 | 13.73 | 下降 69.87% |
| darwin | leaf-help | 1691.71 | 382.30 | 下降 77.40% | 345.76 | 48.96 | 下降 85.84% |
| darwin | config | 346.17 | 377.23 | 增加 8.97% | 45.76 | 46.91 | 增加 2.51% |
| darwin | dry-run | 347.15 | 377.57 | 增加 8.76% | 45.54 | 46.97 | 增加 3.14% |
| darwin | mock | 348.76 | 378.02 | 增加 8.39% | 46.04 | 47.23 | 增加 2.58% |

Schema 缓存命中与同一候选实时装配的对照如下。该对照只改变 Schema delivery 路径，更适合描述缓存本身收益。

| 平台 | 指标 p50 | 实时装配 | 缓存命中 | 变化 |
|---|---|---:|---:|---:|
| linux | wall | 2626.17 ms | 375.45 ms | 下降 85.70% |
| linux | user CPU | 3344.09 ms | 70.82 ms | 下降 97.88% |
| linux | peak RSS | 333.67 MiB | 43.19 MiB | 下降 87.06% |
| darwin | wall | 1731.59 ms | 368.49 ms | 下降 78.72% |
| darwin | user CPU | 1838.08 ms | 47.96 ms | 下降 97.39% |
| darwin | peak RSS | 335.80 MiB | 38.19 MiB | 下降 88.63% |

## 4. 生效的 help/version/Schema gate

本表来自独立默认入口报告。default 与显式 `DO_NOT_TRACK` opt-out 分开测量；每个候选 p50、p95 都必须不超过固定 pre-PR 对应值的 1.05 倍。旧的 `launcher ≤ 同包 core +5%` 只保留 diagnostics，本轮 diagnostics 也全部为 true，但不参与 `passed`。

| 平台 | 入口/模式 | pre-PR p50/p95 ms | 候选 p50/p95 ms | 变化 p50/p95 |
|---|---|---:|---:|---:|
| linux | help default | 345.95/347.19 | 305.95/306.40 | -11.56%/-11.75% |
| linux | help opt-out | 45.31/47.29 | 4.83/5.10 | -89.35%/-89.21% |
| linux | version default | 345.44/346.96 | 304.97/305.54 | -11.72%/-11.94% |
| linux | version opt-out | 44.39/45.70 | 3.55/3.73 | -91.99%/-91.84% |
| darwin | help default | 349.61/375.60 | 318.49/327.96 | -8.90%/-12.68% |
| darwin | help opt-out | 39.37/69.75 | 10.28/12.94 | -73.89%/-81.45% |
| darwin | version default | 350.53/391.93 | 317.86/325.68 | -9.32%/-16.90% |
| darwin | version opt-out | 40.20/66.90 | 9.05/15.00 | -77.49%/-77.58% |

Schema 的正式 gate 同样通过：default 命中的 user CPU p50 相对实时装配下降 97.38%–97.82%，opt-out 下降 99.12%–99.35%；四个平台/模式的 peak RSS p95 为 19.69–43.82 MiB，低于 100 MiB 上限。

## 5. 与 Lark CLI、GWS 的同机对比

Schema 选择相近的日历列表发现接口：DWS `calendar.list_calendars`、Lark `calendar.calendars.list`、GWS `calendar.calendarList.list`。输出及产品合同不完全等价，结果只用于诊断。耗时单位 ms，格式为 p50/p95；RSS 为独立内存试验 p50。

### 5.1 Native 对 native

| 平台 | 场景 | DWS ms | Lark ms | GWS ms | DWS RSS | Lark RSS | GWS RSS |
|---|---|---:|---:|---:|---:|---:|---:|
| linux | schema | 375.45/377.22 | 46.06/47.89 | 4.34/4.55 | 43.19 | 42.68 | 9.10 |
| linux | help | 305.99/306.46 | 46.42/47.72 | 3.27/3.37 | 12.20 | 42.70 | 6.91 |
| linux | version | 305.08/305.53 | 45.25/46.98 | 3.14/3.26 | 12.05 | 40.93 | 6.84 |
| linux | leaf-help | 394.70/396.32 | 45.52/47.66 | 4.67/4.82 | 55.35 | 40.79 | 9.42 |
| linux | dry-run | 389.67/391.47 | 46.61/47.89 | 4.84/5.02 | 52.95 | 42.46 | 9.97 |
| darwin | schema | 368.49/376.09 | 41.97/45.91 | 8.38/9.84 | 38.19 | 43.98 | 9.67 |
| darwin | help | 320.22/327.34 | 42.15/46.24 | 6.80/8.88 | 13.96 | 44.14 | 8.01 |
| darwin | version | 319.54/327.01 | 41.73/45.73 | 6.83/8.68 | 13.73 | 43.84 | 7.92 |
| darwin | leaf-help | 382.30/392.60 | 41.07/43.94 | 8.81/10.40 | 48.96 | 43.78 | 10.45 |
| darwin | dry-run | 377.57/392.50 | 41.73/48.40 | 8.02/10.87 | 46.97 | 43.95 | 10.81 |

### 5.2 Public wrapper 对 public wrapper

public RSS 采样包含同时驻留的 Node wrapper 与子进程；DWS public 使用候选 canonical package 的相同字节，不代表 npm 当前正式版已启用这些快路径。

| 平台 | 场景 | DWS ms | Lark ms | GWS ms | DWS RSS | Lark RSS | GWS RSS |
|---|---|---:|---:|---:|---:|---:|---:|
| linux | schema | 402.45/404.62 | 72.00/73.65 | 31.88/32.49 | 88.69 | 85.18 | 44.81 |
| linux | help | 332.85/333.52 | 72.17/74.71 | 30.68/31.73 | 57.54 | 85.20 | 44.80 |
| linux | version | 331.88/332.43 | 70.66/73.14 | 30.56/31.82 | 57.57 | 85.17 | 44.81 |
| linux | leaf-help | 422.14/423.89 | 70.87/72.91 | 32.20/33.03 | 100.88 | 84.17 | 44.95 |
| linux | dry-run | 417.52/419.77 | 71.85/73.57 | 32.72/33.41 | 98.63 | 85.29 | 44.81 |
| darwin | schema | 397.91/415.47 | 68.48/76.56 | 36.63/41.99 | 76.24 | 75.57 | 38.62 |
| darwin | help | 355.11/363.43 | 68.99/79.70 | 35.69/38.17 | 53.02 | 74.41 | 38.69 |
| darwin | version | 354.38/360.05 | 67.86/73.82 | 35.96/40.99 | 52.80 | 74.82 | 38.65 |
| darwin | leaf-help | 413.76/436.63 | 68.21/86.36 | 35.73/43.77 | 88.78 | 73.70 | 38.79 |
| darwin | dry-run | 412.54/419.36 | 68.85/87.81 | 35.39/40.87 | 85.89 | 74.13 | 38.85 |

DWS 没有在这些竞争性 wall 指标上领先。内存随入口变化：native root help/version 明显小于 Lark，但大于 GWS；Schema 在 Darwin 小于 Lark，在 Linux略大于 Lark；leaf help、dry-run 和多数 public 入口没有统一优势。

## 6. 架构与性能含义

结果支持 RFC 选定的受限热路径 runtime：Schema、exact root help、exact version 获得显著收益，且新的固定 pre-PR gate 双平台通过。结果也验证了委派路径不应以“launcher 相对同包 core ≤5%”作为 release gate：leaf help 因避免全量 Schema 组装而大幅改善，但 config、dry-run、mock 仍承担 launcher 启动、完整 core 校验和委派成本，出现 8.39%–12.74% 回退。

因此本 RFC 的性能承诺只覆盖 allowlist 内热路径和明确的 Schema CPU/RSS 预算。委派路径保留墙钟、CPU、RSS 和同包 core 诊断，不用热路径收益抵消普通命令回退，也不以竞品对比作为 Ready/release gate。

## 7. 原始证据与剩余发布边界

[证据索引](benchmarks/schema-cache/native-7cbf7f52/evidence.json)记录 run、artifact ID、源码 commit/tree、原始字节数，以及归档前后 SHA-256。两个较大的 JSON 使用确定性 gzip 无损保存：

- 五维逐次样本：[Linux](benchmarks/schema-cache/native-7cbf7f52/linux/five-dimensions-report.json.gz) · [Darwin](benchmarks/schema-cache/native-7cbf7f52/darwin/five-dimensions-report.json.gz)；
- 默认入口 gate：[Linux](benchmarks/schema-cache/native-7cbf7f52/linux/default-entry-report.json.gz) · [Darwin](benchmarks/schema-cache/native-7cbf7f52/darwin/default-entry-report.json.gz)；
- 最终 help proof、package version、identity environment、candidate build 和固定基线信息在同目录按平台保存；
- 迁移前历史数据继续保留在 [`native-c0f3aaca`](benchmarks/schema-cache/native-c0f3aaca/evidence.json)，其旧 gate 语义不再用于当前验收。

R9 和 D1 可由本轮证据勾选。R8 仍未完成：开发候选使用 ad-hoc/无签名，`release_eligible:false` 是预期状态；第一次包含本实现的官方 prerelease/stable 仍须完成 Developer ID、公证、最终发布制品的 native help/Schema 验证，以及不可变整包安装、升级和回滚证明。
