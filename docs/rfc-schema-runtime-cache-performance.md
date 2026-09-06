# RFC 附件：Schema、help、命令与进程内存性能报告

日期：2026-09-06。候选：`c0f3aaca53c502cab780d67d8eccca4a55e5107f`。状态：两平台全部耗时样本完成；Linux 内存完成，macOS 的 GWS 原生内存采样未齐；发布验收未完成。

关联：[主 RFC](rfc-schema-runtime-cache.md) · [PR #1296](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pull/1296) · [原生 CI 34011126327](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/actions/runs/34011126327)。

主 RFC 于 2026-09-06 选定受限热路径 runtime，§8.4 已废止同包 core 的 5% release gate。本次 CI 仍使用迁移前脚本，原始 `passed/gates` 保留历史含义，不能作为新设计验收。代码差距由主 RFC §10 逐项管理；本附件只报告实测结果。

## 1. 性能提升多少

| 维度 | 本次两平台实测结论 |
|---|---|
| Schema | 相对固定 pre-PR 基线，耗时 p50 **下降 78.18%–80.04%**；同一候选缓存命中对实时装配，用户态 CPU p50 **下降 97.28%–97.68%** |
| 根 help | 已修复投影一致性；相对固定基线，耗时 p50 **下降 8.69%–9.91%**，进程树采样峰值 RSS p50 **下降 69.10%–76.07%** |
| 命令 | version 快 8.67%–10.13%，日历列表叶子 help 快 77.26%–79.12%；配置查询、日历列表 dry-run/mock **慢 8.07%–9.85%**，不能宣称所有命令加速 |
| 整体运行内存 | Schema 与 help/version 明显降低；本次配置查询、dry-run/mock 增加约 3%。没有统一的“全 CLI 内存降低百分比” |
| Lark CLI / GWS | 同机、同默认配置原则下，DWS 在已测五类 public/native 耗时中均更慢；Schema 优化不等于竞品领先。macOS GWS native 内存仍缺完整样本 |

这些是开发候选的结果。不能据此宣称已发布正式包获得相同收益，也不能外推到远端 RPC、认证、持续事件监听或冷缓存修复。

## 2. 测量合同与完成度

| 项目 | 固定口径 |
|---|---|
| pre-PR 基线 | `5243e5ca19b55a3e785e5cc09273b653ad5381dc`，固定 Git 树及对应 runtime payload；与候选比较的是整个区间净变化，不单独归因于某一提交 |
| 平台 | Linux amd64（4 CPU）；macOS arm64（3 CPU，macOS 26.6.2） |
| 工具链 | Go 1.25.9、CGO=0、PIE；Node 22.16.0；Lark CLI 1.0.85、GWS 0.22.5；psutil 7.2.2 |
| 矩阵 | 每平台 51 场景；每场景 30 次耗时与 30 次独立内存试验；随机交错，预热不计样本 |
| 入口 | DWS canonical package 原生入口 / 官方 npm wrapper / 同包 core / 固定旧基线；Lark、GWS 各自 native 与官方 npm wrapper 分开 |
| 默认配置 | 独立 HOME、预热缓存、无真实凭据，全部计时子进程均未设置 `DO_NOT_TRACK`；保留各自默认上报，不声称三产品上报实现或网络成本相同 |
| 业务执行 | 日历列表 dry-run；DWS 另测配置查询和本地 mock。未执行真实业务 RPC。Lark 预演使用显式占位 app/token 环境，仅供通过本地上下文检查 |
| 网络 | 未隔离或统一网络响应；没有用这批数据证明 tracker 送达。DWS 默认生命周期约 300 ms 的成本保留在用户入口耗时内 |
| 耗时 | 阻塞 `wait4`，从 CLI 子进程启动至回收；外部 Python sampler 启动不计，内存轮询不参与该阶段 |
| 内存 | 本轮独立 psutil 试验请求 1 ms 轮询，取同时驻留进程树 RSS 的最大观测值；含 wrapper 与孩子，共享页可能重复计算，是采样下界而非 PSS 或精确内核峰值 |
| 分位数 | p50 为中位数；p95 为排序后第 `ceil(n×0.95)` 项。变化率由未舍入值计算，表格保留两位小数 |

macOS 1 分钟 load average 从 4.15 到 2.78，Linux 从 1.61 到 1.06；本次不是严格空闲机器实验，不将小幅差异推广为跨机器保证。各场景逐次检查退出码、stdout/stderr 与预热 oracle，原始 argv、环境、输出字节数/摘要、程序摘要及逐次样本见证据归档。

| 阶段 | Linux amd64 | macOS arm64 |
|---|---|---|
| 五维耗时 | 51 × 30，全部完成 | 51 × 30，全部完成 |
| 五维进程树内存 | 51 × 30，全部完成 | 46 × 30 完成；5 项 GWS native 未齐 |
| 全量 Go suite / declaration policy | 通过 | 通过 |
| finalized core en/zh help 一致性 | 通过 | 通过 |
| 旧默认入口脚本 | 原始 10 项均通过，仅作历史结果 | 原始 10 项均通过，仅作历史结果 |
| candidate job | success | failure：短进程内存轮询漏采 |

macOS 未齐计数：GWS native Schema 23/30、help 26/30、version 25/30、leaf help 19/30、dry-run 26/30。错误均为 `memory sampler did not observe the command`；对应耗时试验全部成功。下表不展示这五项不完整内存汇总，不补零，不把原始 `complete:false` 改为 true。跨平台 identity 汇总 job 因 candidate failure 跳过。

## 3. Schema 与固定旧入口的净变化

DWS 场景固定为 `schema calendar.list_calendars --compact -f json`。表中内存为 30 次独立试验的进程树采样峰值中位数。

| 平台 | 场景 | 基线 p50 ms | 候选 p50 ms | 耗时变化 | 基线 RSS MiB | 候选 RSS MiB | RSS 变化 |
|---|---|---:|---:|---|---:|---:|---|
| linux | schema | 1795.12 | 358.23 | 下降 80.04% | 343.32 | 43.81 | 下降 87.24% |
| linux | help | 338.62 | 305.06 | 下降 9.91% | 51.89 | 12.42 | 下降 76.07% |
| linux | version | 338.80 | 304.49 | 下降 10.13% | 51.83 | 12.36 | 下降 76.16% |
| linux | leaf-help | 1794.93 | 374.82 | 下降 79.12% | 342.16 | 55.99 | 下降 83.64% |
| linux | config | 338.58 | 371.58 | 增加 9.74% | 51.98 | 53.75 | 增加 3.40% |
| linux | dry-run | 338.46 | 370.57 | 增加 9.49% | 51.99 | 53.59 | 增加 3.08% |
| linux | mock | 339.34 | 370.82 | 增加 9.28% | 52.11 | 53.86 | 增加 3.37% |
| darwin | schema | 1684.02 | 367.38 | 下降 78.18% | 348.40 | 36.98 | 下降 89.38% |
| darwin | help | 348.31 | 318.05 | 下降 8.69% | 45.23 | 13.98 | 下降 69.10% |
| darwin | version | 347.86 | 317.70 | 下降 8.67% | 45.24 | 13.70 | 下降 69.71% |
| darwin | leaf-help | 1676.12 | 381.07 | 下降 77.26% | 352.05 | 49.29 | 下降 86.00% |
| darwin | config | 347.83 | 375.91 | 增加 8.07% | 45.51 | 46.86 | 增加 2.97% |
| darwin | dry-run | 347.00 | 377.41 | 增加 8.77% | 45.28 | 46.70 | 增加 3.12% |
| darwin | mock | 346.60 | 380.73 | 增加 9.85% | 45.68 | 47.12 | 增加 3.16% |

`help` 为 exact 根 `--help`；`leaf-help` 为 `calendar book list --help`；`config` 为 `config list --json`；`dry-run` / `mock` 为 `calendar book list --dry-run/--mock -f json`。普通命令的回退必须保留，不能用 Schema 或叶子 help 收益抵消。

同一候选的实时装配对照只改变 `DWS_SCHEMA_CACHE_FORCE_LIVE`，可更直接观察 Schema 命中收益：

| 平台 | 指标 p50 | 强制实时装配 | 默认命中 | 变化 |
|---|---|---:|---:|---|
| linux | 耗时 | 1881.68 ms | 358.23 ms | 下降 80.96% |
| linux | 用户态 CPU | 2371.50 ms | 54.91 ms | 下降 97.68% |
| linux | 进程树 RSS | 330.68 MiB | 43.81 MiB | 下降 86.75% |
| darwin | 耗时 | 1729.60 ms | 367.38 ms | 下降 78.76% |
| darwin | 用户态 CPU | 1814.99 ms | 49.36 ms | 下降 97.28% |
| darwin | 进程树 RSS | 336.02 MiB | 36.98 MiB | 下降 88.99% |

配置查询和预演命令并未体现完整 Schema 重装配收益；dry-run/mock 的强制实时与默认路径耗时 p50 相差约 0%–1%。这批结果不支持“普通命令均因 Meta 缓存获得约 80% 加速”。

## 4. help、version 与委派诊断

本 head 的帮助投影包含 `contract` / `whiteboard`，en/zh 均与 finalized core 完整字节一致，原生证明显示默认根 help 无需 core。先前 `f2a8b9d7` 的封装失败保留在历史证据中，不再作为本 head 的性能结论。

以下列出基线与候选的尾部分位数，并保留同包 core 作为诊断：

| 平台 | 场景 | 基线 p50/p95 ms | 候选 p50/p95 ms | 同包 core p50/p95 ms |
|---|---|---:|---:|---:|
| linux | help | 338.62/343.10 | 305.06/305.51 | 340.97/344.84 |
| linux | version | 338.80/343.22 | 304.49/305.17 | 340.64/344.29 |
| linux | leaf-help | 1794.93/1879.27 | 374.82/384.38 | 344.69/349.64 |
| linux | config | 338.58/344.43 | 371.58/376.75 | 340.95/345.37 |
| linux | dry-run | 338.46/342.97 | 370.57/379.57 | 340.55/342.97 |
| linux | mock | 339.34/346.89 | 370.82/376.62 | 341.40/343.10 |
| darwin | help | 348.31/356.79 | 318.05/329.07 | 349.68/358.91 |
| darwin | version | 347.86/356.61 | 317.70/324.97 | 348.47/354.35 |
| darwin | leaf-help | 1676.12/1779.85 | 381.07/392.11 | 352.04/361.41 |
| darwin | config | 347.83/355.42 | 375.91/384.29 | 348.06/357.71 |
| darwin | dry-run | 347.00/353.73 | 377.41/386.74 | 349.10/357.37 |
| darwin | mock | 346.60/357.17 | 380.73/386.77 | 351.89/359.79 |

委派前的 core 校验属于当前模型的实际成本。候选入口与同包 core 的差值包含启动、校验、调度等，尚无独立分段剖析，不能把整个差值都写成 SHA-256 耗时。根据主 RFC，新验收只对允许的快路径比较固定 pre-PR 基线；委派路径与同包 core 的墙钟差异不再是 release gate。

## 5. 与 Lark CLI、GWS 的同机对比

Schema 选择日历列表接口：DWS `calendar.list_calendars`、Lark `calendar.calendars.list`、GWS `calendar.calendarList.list`。输出分别为 987、6,479、4,109 字节；这是语义相近的发现任务，不是等量输出或相同产品合同。叶子 help / dry-run 同样对应各自日历列表命令，不能把预演时间解释成真实服务端执行时间。

### 5.1 Native 对 native

耗时每项均为完整 30 次样本，单位 ms。

| 平台 | 场景 | DWS p50/p95 | Lark p50/p95 | GWS p50/p95 |
|---|---|---:|---:|---:|
| linux | schema | 358.23/364.80 | 35.82/39.80 | 3.19/3.46 |
| linux | help | 305.06/305.51 | 36.32/40.05 | 2.23/2.48 |
| linux | version | 304.49/305.17 | 35.46/38.33 | 2.17/2.37 |
| linux | leaf-help | 374.82/384.38 | 35.46/39.46 | 3.39/3.71 |
| linux | dry-run | 370.57/379.57 | 36.25/38.75 | 3.56/3.82 |
| darwin | schema | 367.38/376.84 | 42.34/47.03 | 8.50/10.11 |
| darwin | help | 318.05/329.07 | 42.66/45.23 | 7.21/8.92 |
| darwin | version | 317.70/324.97 | 41.72/43.42 | 7.00/8.69 |
| darwin | leaf-help | 381.07/392.11 | 40.18/43.83 | 8.72/9.84 |
| darwin | dry-run | 377.41/386.74 | 41.85/44.80 | 8.69/9.82 |

### 5.2 Public 对 public

均保留各自官方 npm wrapper。RSS 是同时驻留进程树采样峰值 p50，包含 Node 与 CLI 子进程；本表两平台全部 30 次内存样本齐全。DWS public 使用候选 canonical package 的同字节程序，不代表 npm 当前正式发布已启用快路径。

| 平台 | 场景 | DWS p50/p95 ms | Lark p50/p95 ms | GWS p50/p95 ms | DWS RSS MiB | Lark RSS MiB | GWS RSS MiB |
|---|---|---:|---:|---:|---:|---:|---:|
| linux | schema | 378.58/386.68 | 54.41/61.66 | 22.77/24.61 | 89.11 | 85.33 | 51.58 |
| linux | help | 324.63/326.66 | 54.15/62.00 | 21.92/23.95 | 57.63 | 85.51 | 44.85 |
| linux | version | 323.99/325.69 | 52.95/62.47 | 21.80/22.51 | 57.62 | 85.30 | 44.93 |
| linux | leaf-help | 394.88/407.90 | 53.62/58.57 | 22.72/24.09 | 101.08 | 85.32 | 52.89 |
| linux | dry-run | 391.05/397.46 | 53.91/61.86 | 23.08/26.08 | 98.71 | 85.63 | 53.22 |
| darwin | schema | 397.84/404.41 | 68.71/73.66 | 36.45/37.87 | 76.05 | 77.40 | 37.95 |
| darwin | help | 352.70/360.19 | 69.49/76.10 | 35.78/40.32 | 53.20 | 76.22 | 38.84 |
| darwin | version | 353.05/361.81 | 68.21/74.25 | 35.76/38.49 | 52.84 | 75.23 | 38.86 |
| darwin | leaf-help | 409.02/420.05 | 67.31/74.08 | 35.50/37.30 | 88.77 | 76.12 | 38.86 |
| darwin | dry-run | 409.55/417.56 | 68.80/74.07 | 35.30/38.09 | 85.96 | 78.15 | 38.83 |

### 5.3 Native 内存

单位 MiB，进程树采样峰值 p50。macOS GWS 原生入口的短生命周期暴露了轮询的漏采问题，其不完整样本不参与排名；即使样本齐全，轮询也可能遗漏真正峰值。

| 平台 | 场景 | DWS | Lark | GWS |
|---|---|---:|---:|---:|
| linux | schema | 43.81 | 41.07 | 5.78 |
| linux | help | 12.42 | 41.16 | 5.66 |
| linux | version | 12.36 | 40.98 | 5.71 |
| linux | leaf-help | 55.99 | 41.01 | 5.46 |
| linux | dry-run | 53.59 | 40.95 | 5.76 |
| darwin | schema | 36.98 | 39.03 | 未齐 |
| darwin | help | 13.98 | 39.51 | 未齐 |
| darwin | version | 13.70 | 38.85 | 未齐 |
| darwin | leaf-help | 49.29 | 38.88 | 未齐 |
| darwin | dry-run | 46.70 | 39.15 | 未齐 |

这批默认入口数据没有证明 DWS 在耗时上领先。内存也依场景变化：根 help/version 小于 Lark，Schema/普通命令与 wrapper 则不能给出统一排名。不得拿关闭 DWS 上报的历史原型与本表默认竞品交叉比较。

## 6. 未完成项与验收边界

- [ ] 补齐 macOS GWS native 五项内存采样；原生单进程改为低开销父进程的内核峰值采样后，要另建测量批次并记录方法变化，不能覆盖这批轮询原始数据。public 仍需同时进程树观测。
- [x] 按主 RFC §10 完成 allowlist/能力 gate、invalid snapshot/identity fail-closed、正式与候选同构注入，以及新延迟门槛迁移后，对最终代码重新验收；[`7cbf7f52` native run](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/actions/runs/34018840739) 已通过两平台全量、声明、生命周期、最终候选包和新性能 gate。
- [ ] 正式发布仍需最终签名/公证、安装升级与回滚、release identity 等证明。候选的 `release_eligible:false` 保持原样。

远端真实请求、持续事件内存、冷缓存首次修复和独立哈希阶段剖析不在本次五维短命令矩阵内；需要相应结论时单独设计实验，不能从上述收益外推。

## 7. 原始证据与复核

[本轮证据索引](benchmarks/schema-cache/native-c0f3aaca/evidence.json)记录原 artifact ID、文件路径、原始/归档 SHA-256。较大的 JSON 使用确定性 gzip 无损归档，解压后的字节与下载文件完全相同；未修改失败状态。

- 五维逐次样本：[Linux](benchmarks/schema-cache/native-c0f3aaca/linux/five-dimensions-report.json.gz) · [macOS](benchmarks/schema-cache/native-c0f3aaca/darwin/five-dimensions-report.json.gz)。
- 旧默认入口原始 gate：[Linux](benchmarks/schema-cache/native-c0f3aaca/linux/default-entry-report.json.gz) · [macOS](benchmarks/schema-cache/native-c0f3aaca/darwin/default-entry-report.json.gz)。
- 最终帮助一致性：[Linux](benchmarks/schema-cache/native-c0f3aaca/linux/root-help-proof.json) · [macOS](benchmarks/schema-cache/native-c0f3aaca/darwin/root-help-proof.json)。
- 先前 `decb45a7` 数据及失败门槛：[历史证据索引](benchmarks/schema-cache/native-decb45a7/evidence.json)。
- 先前 `f2a8b9d7` 帮助封装失败：[历史证据索引](benchmarks/schema-cache/native-f2a8b9d7-help-failure/evidence.json)。

本表逐项从原始样本重算 p50/p95，并验证与 JSON 汇总一致；所有已展示内存数据均具备 30 次有效试验。两平台五维报告分别保留 `complete:true` / `complete:false`。这份附件描述 `c0f3aaca`，不随工作树后续代码变化自动升级为新 head 的证明。
