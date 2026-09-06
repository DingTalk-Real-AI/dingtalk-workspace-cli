# RFC 附件：Schema 缓存与 CLI 入口性能报告

日期：2026-09-06。状态：开发候选测量，发布验收未完成。

下一轮测量已实现于 [五维进程测量脚本](../scripts/dev/measure-cli-five-dimensions.py)，并接入两平台原生 CI。每个平台 51 个场景，每场景分别进行 30 次独立耗时试验与 30 次进程树内存试验。包含 DWS 原生/npm/core/固定基线、Lark 1.0.85 与 GWS 0.22.5 原生/npm 入口的 Schema、根 help、version、日历列表叶子 help、日历列表 dry-run，以及 DWS 配置查询、mock 和强制实时装配对照。当前尚未取得这一轮的完整结果，下文保留已完成轮次的结论。

所有业务预演和 mock 都不执行真实业务请求。Lark dry-run 使用独立进程环境中的占位账号信息；没有真实凭据或登录配置。进程树内存用独立试验按请求的 1 ms 间隔观测同时驻留 RSS，统计包含 wrapper 与子进程；它是采样下界，包含共享页重复计数，不能当作 PSS 或精确的内核峰值。耗时仍采用无内存轮询干扰的阻塞 `wait4` 测量。当前矩阵不包含长期事件监听或真实服务端延迟。

关联：[主 RFC](rfc-schema-runtime-cache.md) · [PR #1296](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pull/1296)

## 1. 已证实的收益

最近一轮已完成、包含两平台全部样本的原生测量来自 `decb45a7`：

- **Schema 缓存命中**相对同一候选的强制实时装配，进程耗时 p50 下降 **79.53%–85.76%**，用户态 CPU 时间下降 **97.31%–97.84%**，每进程峰值 RSS 的中位数下降 **86.88%–88.35%**。
- **默认 `--version`** 相对不可变 PR 前基线，进程耗时 p50 下降 **9.94%–11.87%**；同包 core 与 PR 前基线的四项 5% 门槛，在两个平台均通过。
- **默认 `--help`** 在该提交仍委派 core，耗时 p50 相对 PR 前基线**增加 9.68%–11.29%**，四项门槛均失败。因此本轮整体性能验收仍未通过。

新 head `f2a8b9d7` 已包含 sealed help 快路径，但[新一轮原生 CI](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/actions/runs/34009073470)的两平台 candidate 均在封装前失败：`en help projection differs from finalized core`。未进入当前 head 的性能采样，不能宣称新 help 已加速。原始失败证明见文末。

### 按五项需求阅读

| 维度 | 已有结论 | 尚缺证据 |
|---|---|---|
| 1. Schema | 缓存命中耗时 p50 降低 79.53%–85.76%，用户态 CPU 降低 97.31%–97.84% | 冷缓存修复、更多路由和最终发布制品 |
| 2. help | 已完成测量的旧 help 慢 9.68%–11.29%；新 help 因输出不一致未开始采样 | 修复输出一致性，再验证两平台四项延迟门槛 |
| 3. 命令 | 默认 version 耗时 p50 降低 9.94%–11.87%；普通业务命令没有同条件前后实测 | 本地命令、需 ResolveMeta 的业务命令、远端调用分别测量 |
| 4. 整体运行内存 | Schema 命中进程峰值 RSS 中位数约 330 MiB → 38–43 MiB；version 约 46–51 MiB → 12–14 MiB | 各类业务命令、冷修复、子进程树峰值与持续事件进程 |
| 5. Lark CLI / GWS | 有历史版本的本机参考值；当前候选尚无同条件竞品对照 | 固定版本，public/native 分开，统一上报与缓存条件后重测 |

## 2. 测量范围与口径

| 项目 | 口径 |
|---|---|
| 已完成测量 | [Native CI 34007681787](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/actions/runs/34007681787) |
| 候选提交 | `decb45a741de47204aa0f7cf4ea98ea1a07e21f5` |
| PR 前基线 | `5243e5ca19b55a3e785e5cc09273b653ad5381dc`，从固定 Git 树构建，包含其 runtime payload |
| 平台与工具链 | Linux amd64、macOS arm64；Go 1.25.9，CGO=0，`-trimpath -buildmode=pie` |
| 样本 | 每平台 8 种模式 × 30 次独立进程，共 240 次；两平台共 480 次 |
| 顺序 | 固定随机种子 `20260906` 打乱交错运行；预热和输出 oracle 获取不计入样本 |
| 运行环境 | 独立 HOME，无认证 profile；所有计时子进程均未设置 `DO_NOT_TRACK`，保留默认 tracker 与 flush 预算 |
| 网络 | 本轮计时未隔离或控制网络；不能据此证明上报送达或网络条件无影响 |
| wall | 进程启动至回收的耗时，包含默认 tracker；不包含外层 Python sampler 的启动耗时 |
| CPU | `wait4` 返回的子进程用户态 CPU 时间；不是 CPU 利用率，也不是端到端耗时 |
| RSS | `wait4` 返回的子进程峰值驻留内存，转换为 MiB；表中 p50 是 30 个峰值的中位数 |
| 分位数 | p50 为中位数；p95 为排序后第 `ceil(30×0.95)=29` 个样本，与测量脚本一致 |

表中变化率为 `(候选 / 对照 − 1) × 100%`，基于未舍入数据计算；展示值保留两位小数。30 次样本支持本次门槛判断，不能代表所有机器、网络或真实用户分布。

## 3. Schema：缓存命中与实时装配

测量命令为 `schema calendar.create_calendar_event --compact -f json`。对照是**同一候选程序**的强制实时装配路径；缓存路径使用预热并通过长度、摘要验证的文件。两条路径保留默认上报，并要求输出与权威装配 oracle 一致。

这组结果衡量缓存命中的收益，不是冷缓存首次修复，也不是全部业务命令、全部 Schema 路由或 PR 前版本的整体加速。


| 平台 | 指标 | 实时装配 | 缓存命中 | 变化 |
| --- | --- | --- | --- | --- |
| Linux amd64 | 耗时 p50 | 2608.39 ms | 371.44 ms | 下降 85.76% |
| Linux amd64 | 耗时 p95 | 2657.49 ms | 372.76 ms | 下降 85.97% |
| Linux amd64 | 用户态 CPU p50 | 3307.12 ms | 71.32 ms | 下降 97.84% |
| Linux amd64 | 峰值 RSS p50 | 331.06 MiB | 43.42 MiB | 下降 86.88% |
| macOS arm64 | 耗时 p50 | 1819.41 ms | 372.44 ms | 下降 79.53% |
| macOS arm64 | 耗时 p95 | 2077.41 ms | 383.56 ms | 下降 81.54% |
| macOS arm64 | 用户态 CPU p50 | 1942.15 ms | 52.17 ms | 下降 97.31% |
| macOS arm64 | 峰值 RSS p50 | 330.22 MiB | 38.47 MiB | 下降 88.35% |


缓存命中 30 次样本中的最大峰值 RSS：Linux amd64 **45.45 MiB**；macOS arm64 **38.80 MiB**，均低于 100 MiB 门槛。

收益来自避免每个进程重新装配完整声明，再通过认证的 Meta/产品分片读取所需投影。仍保留完整性校验、同源解码与默认上报；缓存缺失或损坏时的修复成本不包含在上述命中收益内。


## 4. help

同包 core 对照衡量 launcher 入口开销；PR 前固定基线对照衡量整个 PR 的净变化。以下是 `decb45a7` 的完整结果。

| 平台 | 耗时分位 | PR 前基线 ms | 同包 core ms | launcher ms | 相对 PR 前 | 相对同包 core |
| --- | --- | --- | --- | --- | --- | --- |
| Linux amd64 | p50 | 346.11 | 349.41 | 385.18 | 增加 11.29% | 增加 10.24% |
| Linux amd64 | p95 | 347.45 | 350.95 | 386.91 | 增加 11.35% | 增加 10.25% |
| macOS arm64 | p50 | 352.78 | 357.08 | 386.92 | 增加 9.68% | 增加 8.36% |
| macOS arm64 | p95 | 361.65 | 365.94 | 401.46 | 增加 11.01% | 增加 9.71% |

该提交的 help 仍需委派 core，保留 core 摘要校验和完整入口开销。两平台相对同包 core、PR 前基线的 p50/p95 共四项检查均失败。

`4822c10c` 接入帮助快照，`f2a8b9d7` 加入采样超时控制。然而两平台最新 candidate 都因英文帮助与最终 core 不一致而中止，尚无新 help 耗时数字。应先修复正确性，再使用相同基线和门槛采样，不能以代码减少了调用链推断实测收益。

## 5. 命令

### 5.1 已测量：默认 version

| 平台 | 耗时分位 | PR 前基线 ms | 同包 core ms | launcher ms | 相对 PR 前 | 相对同包 core |
| --- | --- | --- | --- | --- | --- | --- |
| Linux amd64 | p50 | 345.68 | 348.51 | 304.66 | 下降 11.87% | 下降 12.58% |
| Linux amd64 | p95 | 347.81 | 350.72 | 305.17 | 下降 12.26% | 下降 12.99% |
| macOS arm64 | p50 | 354.56 | 355.65 | 319.31 | 下降 9.94% | 下降 10.22% |
| macOS arm64 | p95 | 366.91 | 363.86 | 329.75 | 下降 10.13% | 下降 9.38% |

version 在已证明的普通 open-edition 调用中由 launcher 完成，共用 core 的版本格式、profile 元数据读取、官方 tracker 与信号语义。默认入口仍有约 305–319 ms 的 p50 耗时；用户态 CPU 下降不等于整个命令同比加速。

### 5.2 普通业务命令：暂不能给出统一提升比例

Schema 查询和 version 不能代表所有业务命令。架构上，首次 `ResolveMeta` 可从完整装配转为认证的 Meta 缓存读取，但当前成对进程测量没有证明某条普通业务命令的端到端净收益。远端 RPC、认证、输出大小以及是否命中元数据缓存都会影响实际结果。

后续测量矩阵应至少覆盖：

| 命令类别 | 要测的成本 | 对照要求 |
|---|---|---|
| 本地命令与 leaf help | CLI 构造、参数校验、首次 ResolveMeta | 固定 argv、缓存状态、默认上报，PR 前基线与候选交错 |
| 只读业务命令 | CLI 开销与远端请求分别归因 | 固定响应 fixture 的离线对照，加明确网络条件的真实只读样本 |
| 写命令的既有 dry-run | 参数/确认/元数据准备，不实际写入 | 仅使用声明支持 dry-run 的命令，检查输出等价 |
| 持续事件命令 | 启动、稳态内存和运行时增长 | 固定时长、事件量与子进程计量，不能套用短命令峰值 |

未做上述实测前，报告只能说“降低元数据准备成本”，不能写“所有命令提升 80%”。

## 6. 整体运行内存

这里统计的是被测 CLI 子进程**全生命周期峰值 RSS**，包含该进程的启动、依赖加载、命令与 tracker；并非仅 protobuf 解码器的堆内存。它也不是整机内存、多个独立进程的总和或全部业务命令的平均值。

| 平台 | 入口 | 对照口径 | 原峰值 RSS p50 MiB | 候选峰值 RSS p50 MiB | 变化 |
| --- | --- | --- | --- | --- | --- |
| Linux amd64 | Schema | 同版本实时装配 | 331.06 | 43.42 | 下降 86.88% |
| Linux amd64 | version | PR 前基线 | 51.27 | 12.12 | 下降 76.37% |
| Linux amd64 | help | PR 前基线 | 51.39 | 52.98 | 增加 3.08% |
| macOS arm64 | Schema | 同版本实时装配 | 330.22 | 38.47 | 下降 88.35% |
| macOS arm64 | version | PR 前基线 | 45.59 | 13.84 | 下降 69.65% |
| macOS arm64 | help | PR 前基线 | 45.81 | 46.78 | 增加 2.11% |

Schema 和 version 的进程级内存收益已证实；旧 help 略有增加。不能据此宣称“DWS 整体运行内存下降 88%”。若命令会并发启动 MCP/其他子进程，应测量进程树同时驻留峰值；简单相加各进程在不同时刻的最高值不代表整体峰值。测试 suite 的存活对象回收改善也不能替代真实命令 RSS。

## 7. 与 Lark CLI、GWS 对比

### 7.1 已有历史参考

主 RFC §5.11 保留了 2026-09-04 的本机数据。下面只引用历史记录，不将其与当前 DWS 的原生 CI 数值组成速度排名：机器负载、版本、上报、缓存和输出合同未统一，且不是同一批交错样本。

| 历史对象 | 路径 | Schema leaf wall p50 | 样本数 |
|---|---|---:|---:|
| Lark CLI v1.0.85 | npm public shim | 122 ms | 30 |
| Lark CLI v1.0.85 | native | 78 ms | 30 |
| GWS v0.22.5 | npm public shim | 149 ms | 30 |
| GWS v0.22.5 | native | 9.8 ms | 30 |

历史场景是语义相近的 recurring-event instances Schema，Lark 输出约 19.5 KB、GWS 约 7.0 KB；当前 DWS 默认入口样本是另一条 calendar compact leaf，不能当作相同工作量。历史机器还存在明显负载波动，wrapper 的 rusage 不覆盖 native child，故不能做整体 RSS 排名。

主 RFC §5.13 另有不含 Schema 的 version-only 原型：DWS p50/p95 为 4.22/6.51 ms，GWS native 为 8.23/10.75 ms。该原型不是当前默认 tracker 入口，也不是最终签名制品，**不能用于宣称当前 DWS 比 GWS 更快**。

### 7.2 当前结论与对比要求

当前候选对 Lark CLI/GWS 的性能领先尚未证实，不能给出可信的“快多少倍”或“省多少整体内存”。新的对比应记录三者准确版本、二进制摘要、OS/CPU、完整 argv、默认上报和认证状态，并在同一空闲机器交错采样至少 30 次：

- public 对 public：保留各自官方启动入口，计入 wrapper 开销；这是用户实际入口对比。
- native 对 native：作为启动开销归因，单列结果，不能代替 public 对比。
- Schema：选择语义相近的查询，报告输出大小，分冷缓存与命中；不假设三个产品返回完全相同的合同。
- help 与命令：分别测根帮助和代表性命令；先验证行为正确，再比较 p50/p95、CPU 和进程树 RSS。
- 默认配置与 opt-out 分开报告；不能用 DWS 关闭上报的数字对比竞品默认入口。

## 8. 门槛判定

下表严格复述 `decb45a7` 原始报告的布尔结果，不以某一项收益抵消另一项失败。


| 门槛 | Linux amd64 | macOS arm64 |
| --- | --- | --- |
| Schema 用户态 CPU p50 至少下降 80% | 通过 | 通过 |
| Schema 全部样本峰值 RSS ≤100 MiB | 通过 | 通过 |
| version 相对同包 core 耗时 p50 回退 ≤5% | 通过 | 通过 |
| version 相对同包 core 耗时 p95 回退 ≤5% | 通过 | 通过 |
| version 相对 PR 前基线耗时 p50 回退 ≤5% | 通过 | 通过 |
| version 相对 PR 前基线耗时 p95 回退 ≤5% | 通过 | 通过 |
| help 相对同包 core 耗时 p50 回退 ≤5% | **失败** | **失败** |
| help 相对同包 core 耗时 p95 回退 ≤5% | **失败** | **失败** |
| help 相对 PR 前基线耗时 p50 回退 ≤5% | **失败** | **失败** |
| help 相对 PR 前基线耗时 p95 回退 ≤5% | **失败** | **失败** |


两平台各有 6 项通过、4 项失败；CI 整体为 failure，identity coordinator 跳过。两平台全量 Go suite 和声明 policy 通过，不改变性能门槛的失败结论。

## 9. 适用边界与下一轮验收

1. 本报告只描述开发候选。macOS 为 ad-hoc 签名，普通发布尚未注入 Schema cache identity 或 help snapshot，不能说已发布版本已经获得这些收益。
2. 默认入口的计时保留上报，但环境为无 profile 的独立 HOME；真实配置、扩展、认证、网络以及低性能机器的表现未由这批样本覆盖。不确定调用仍回退 core。
3. 不将 Schema 命中与实时装配的对照包装成相对 Lark/GWS 的领先结论，也不将组件微基准替代用户入口耗时。
4. `f2a8b9d7` 需要完成两平台 finalized-core en/zh help 字节一致性、实际无 core 的默认 help 执行，以及同样八项 help/version 延迟检查。新结果应另加按 SHA 标识的章节，保留本轮失败记录。
5. 发布仍需最终制品身份、时钟独立性及尝试访问审计、Developer ID/公证安装、升级回滚和 Code Admission 证据。性能局部通过不授权启用或发布。

## 10. 原始证据与复核

- [Linux 默认入口原始样本、汇总和门槛](benchmarks/schema-cache/native-decb45a7/linux/default-entry-report.json)
- [macOS 默认入口原始样本、汇总和门槛](benchmarks/schema-cache/native-decb45a7/darwin/default-entry-report.json)
- [构建、artifact 和日志证据索引](benchmarks/schema-cache/native-decb45a7/evidence.json)
- [Linux 候选构建记录](benchmarks/schema-cache/native-decb45a7/linux/candidate-build.json) · [macOS 候选构建记录](benchmarks/schema-cache/native-decb45a7/darwin/candidate-build.json)
- [Linux 固定基线构建记录](benchmarks/schema-cache/native-decb45a7/linux/baseline-build.json) · [macOS 固定基线构建记录](benchmarks/schema-cache/native-decb45a7/darwin/baseline-build.json)
- [默认入口测量脚本](../scripts/dev/measure-schema-default-entry.py) · [采样与汇总实现](../scripts/dev/verify-schema-cache-binary.py)

本文已从每种模式的 30 条原始样本重新计算 p50/p95，并与原始汇总逐项比较一致；所有百分比从未舍入汇总计算。原始报告文件 SHA-256：


| 报告 | SHA-256 |
| --- | --- |
| linux | `7467ab0305075da21cc59673fa046fa1a65c0ce3e5d15fcfc42885d30f832e14` |
| darwin | `9927231632f11171cc931655a84166eb3491647bedf2b2354253285c5eba8f19` |

最新 help 封装失败证据（`f2a8b9d7`，不含性能样本）：[来源与摘要](benchmarks/schema-cache/native-f2a8b9d7-help-failure/evidence.json) · [Linux](benchmarks/schema-cache/native-f2a8b9d7-help-failure/linux-root-help-proof.json) · [macOS](benchmarks/schema-cache/native-f2a8b9d7-help-failure/darwin-root-help-proof.json)。竞品历史引用来源为[主 RFC §5.11 / §5.13](rfc-schema-runtime-cache.md)，不作当前版本性能证明。
