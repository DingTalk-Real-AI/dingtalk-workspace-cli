# RFC 附件：CLI 性能提升报告

状态：Draft，2026-09-07 单树架构 clean-head 报告。设计见[主 RFC](rfc-schema-runtime-cache.md)。中间测量 dump 与过程化复跑脚本不入库；当时 native 记录见 [Actions run 34080469082](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/actions/runs/34080469082)。

## 1. 当前结论

PR 已撤回 selective tree、Schema 前置执行和 root help projection。旧 head 中 calendar 0.85 ms、config 0.36 ms、root help 比 Lxxx 软件慢 2.5% 等数字描述的是已删除结构，全部标记为历史结果，不能用于当前 Ready 结论。

完整构树本地父子对照中，DWS 始终构造约 1,825 个 command，B/op 降低 **22.8%**，allocs/op 降低 **9.8%**，ns/op 变化为 **+0.3%**。clean head `a8376f92` 的 [native workflow](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/actions/runs/34080469082) 已在 Darwin/arm64 与 Linux/amd64 全绿，完整 Go suite、race 与五维测量均完成。Compile-time Schema identity 已从发运模型中移除；下列 cache-hit 数字仅作历史/测试参考。

root help 延迟已在两平台快于 Lxxx 软件。RSS 相对固定 main 在 Linux 降低 **7.7%**、Darwin 降低 **8.3%**，p50/p95 均低于 RFC 的 50/55 MiB 产品预算。Darwin 的绝对 RSS 也低于 Lxxx 软件；Linux 为 47.68 MiB，对比 Lxxx 42.85 MiB，仍高 **11.3%**。该差值继续公开，但 Lxxx 软件只有约一半节点，不能作为绝对 release gate，否则会激励 projection/裁树。后续通过更紧凑的通用 typed service builder 继续压缩；不允许恢复 projection、selective tree 或 launcher。

## 2. 方法与版本

本地完整树 microbenchmark：

- CPU/OS：Apple M3 Pro，Darwin/arm64；
- Go：1.26.1，本地工具链；正式 CI 固定 Go 1.25.9；
- benchmark：`BenchmarkNewRootCommand`，每轮 10 次，共 3 轮，取三轮中位数；
- 父提交：`c0131d35`；candidate implementation commit：`34ce7493`；
- 两边均构造完整公开 Cobra tree，不执行 handler；
- 端到端 macOS wall time 受本机安全扫描影响，本轮不采用。

Lxxx 参考固定在 v1.0.85，`Lxxx` 源码（匿名化，不挂公开链接）：

- 生产 Build 挂载完整 utility/service/shortcut tree
- completion 只按 invocation 开启 callback 注册
- v1.0.85 release 使用 Go 1.23

## 3. 完整命令树

| 实现 | command 数 | ns/op | B/op | allocs/op |
|---|---:|---:|---:|---:|
| DWS 父提交 `c0131d35` | ≈1,825 | 13,503,654 | 16,431,941 | 164,227 |
| DWS implementation commit `34ce7493` | ≈1,825 | 13,544,096 | 12,681,568 | 148,166 |
| DWS 变化 | — | **+0.3%** | **-22.8%** | **-9.8%** |
| Lxxx v1.0.85 本机参考 | ≈905 | ≈7,000,000 | ≈10,300,000 | ≈84,600 |

DWS 当前完整树总 B/op 约为 Lxxx 的 1.23 倍、allocs/op 约 1.75 倍，而 command 数约 2.02 倍。粗略每节点为：

| 实现 | KiB / command | alloc / command |
|---|---:|---:|
| DWS 当前 | ≈6.95 | ≈81.2 |
| Lxxx 参考 | ≈11.4 | ≈93.5 |

每节点数据只能说明 DWS 的主要总量差距来自更大的公开命令面，不能证明单个节点复杂度等价。DWS 仍需继续 profile annotations、result-schema normalization 和 pflag maps，但不应通过减少 runtime tree 或复制 help surface 降低总量。

本轮已落地的归因：

- ContractFinal ownership transfer，避免 builder 与 store 各保留/复制一份完整 payload；
- RunE closure 不再捕获 build-only help/contract 字段；
- Safety scanner 首次执行时初始化；
- empty string-slice flag 不再创建 4 KiB CSV writer；
- shortcut builder 减少 declaration value copy；
- root help 直接遍历完整 tree，不创建中间 help model。

## 4. Schema

Schema cache 的算法目标不变：verified protobuf hit 避免约 1,825-command declaration catalog 的 live assembly。上一单二进制候选曾测得 cache-hit 相对 live assembly 的 user CPU 降低 97.6%、RSS 降低 87.5%；该数据仍可证明 cache 方向，但因当时 Schema 在 Cobra 前短路，不能作为当前端到端数字。

当前实现的验收结果：

| 平台 | warm cache wall p50 | live assembly wall p50 | warm RSS p50 | live RSS p50 |
|---|---:|---:|---:|---:|
| Linux/amd64 | 59.45 ms | 1,878.07 ms | 56.16 MiB | 323.85 MiB |
| Darwin/arm64 | 65.68 ms | 1,696.10 ms | 49.27 MiB | 326.27 MiB |

门槛仍为 warm hit 相对 live assembly 的 user CPU p50 至少降低 80%，peak RSS ≤100 MiB。

## 5. Help 与 version

root help 与 version 现在都构造完整 tree。帮助直接从同一 runtime tree 渲染，不读取 Schema cache、不执行业务 auth/RPC，也不创建中间 projection。

30 次交错 native 样本：

| 平台 | DWS help wall p50/p95 | Lxxx help wall p50/p95 | DWS RSS p50/p95 | Lxxx RSS p50/p95 | 结论 |
|---|---:|---:|---:|---:|---|
| Linux/amd64 | 44.05 / 45.99 ms | 47.11 / 48.88 ms | 47.68 / 49.88 MiB | 42.85 / 43.35 MiB | wall 快 6.5%；RSS 高 11.3%，产品预算通过 |
| Darwin/arm64 | 37.26 / 51.45 ms | 45.06 / 53.49 ms | 41.91 / 42.17 MiB | 44.43 / 44.91 MiB | wall 快 17.3%；RSS 低 5.7%，通过 |

default tracker 相对固定 main 的 help/version p50/p95 gate 在两平台全部通过；`DO_NOT_TRACK` 与 default 的结果也证明退出不再等待约 300 ms 的网络 flush。help stdout 为 4,760 bytes，SHA-256 `590ebfc7d090cdfa81e63cfcf6f47727ad1f4ac5f0fc5451445e855ebcf497d0`，与父实现逐字节一致。

## 6. 业务命令

calendar list/get、dry-run、mock、config 和 event utility 均承担同一完整构树成本。专项不再用“无关产品 factory 调用为零”证明收益，而关注：

- 完整构树的统一 B/op、allocs/op 和 retained heap；
- PreParse、validation、auth、Safety、handler 与 cleanup 分段；
- default telemetry 不等待网络；
- dry-run 不新增真实写 RPC；
- 输出、错误分类和 side-effect count 与固定 main 一致。

端到端业务/utility 的 native p50：

| 平台 | leaf help | dry-run | config | mock |
|---|---:|---:|---:|---:|
| Linux/amd64 | 51.02 ms / 52.13 MiB | 44.10 ms / 47.70 MiB | 44.02 ms / 47.83 MiB | 44.21 ms / 48.05 MiB |
| Darwin/arm64 | 42.87 ms / 45.85 MiB | 38.47 ms / 41.93 MiB | 38.55 ms / 42.05 MiB | 36.97 ms / 42.52 MiB |

每格为 wall p50 / RSS p50。dry-run、config 与 mock 都承担同一完整树成本；结果表中没有 launcher 或第二进程。

## 7. 整体运行内存

wait4 native 数据确认，完整树优化相对固定 main 将 root help RSS p50 从 51.66 降到 47.68 MiB（Linux，-7.7%），从 45.70 降到 41.91 MiB（Darwin，-8.3%）。B/op 的下降确实传导到了进程 RSS。两平台 p50/p95 都通过 50/55 MiB 预算；Linux 仍未压过节点数约为一半的 Lxxx 软件。

下一阶段只优化同一完整树：把分散的 helper 构造逐步收敛到紧凑 typed service descriptors 与通用 builder，减少每节点 Cobra/pflag/annotation 常驻对象和构造期触达的代码页。GC 参数实验没有采用：本机 `GOGC/GOMEMLIMIT` 只降低约 0.5 MiB，且增加延迟，不能解决结构性差距。

## 8. Lxxx 与 Gxx

Lxxx 软件证明了“每次完整构树”本身不是 root help projection 的理由。它的优势来自较小的命令面、紧凑 typed metadata、统一 service builder，以及执行期工作不进入 Build。

Gxx 软件 0.22.5 的历史同机 native 启动约 3～4 ms、RSS 约 8～11 MiB。其优势主要在更小的 executable/init/runtime 与命令面。本 RFC 不通过减少 DWS 产品面、增加第二 runtime 或 daemon 追这个区间。

clean-head native p50 对比：

| 平台/场景 | DWS | Lxxx 1.0.85 | Gxx 0.22.5 |
|---|---:|---:|---:|
| Linux Schema | 58.32 ms / 54.42 MiB | 47.08 ms / 43.11 MiB | 4.08 ms / 9.05 MiB |
| Linux root help | 44.05 ms / 47.68 MiB | 47.11 ms / 42.85 MiB | 3.03 ms / 6.90 MiB |
| Linux dry-run | 44.10 ms / 47.70 MiB | 47.25 ms / 42.62 MiB | 4.45 ms / 9.92 MiB |
| Darwin Schema | 46.92 ms / 48.94 MiB | 45.02 ms / 44.34 MiB | 8.70 ms / 9.67 MiB |
| Darwin root help | 37.26 ms / 41.91 MiB | 45.06 ms / 44.43 MiB | 8.08 ms / 8.00 MiB |
| Darwin dry-run | 38.47 ms / 41.93 MiB | 45.07 ms / 44.41 MiB | 9.07 ms / 10.81 MiB |

每格为 wall p50 / RSS p50。Lxxx/Gxx 是诊断；固定 main 回归仍是产品 release gate。DWS Schema 还包含完整树与认证 cache read；竞品 Schema/业务输出合同不等价。

## 9. 当前验收状态

- [x] 单二进制、单进程、完整 runtime tree 结构落地。
- [x] Schema 前置执行与 root help projection 删除。
- [x] 本地完整树 B/op -22.8%、allocs/op -9.8%，达到 RFC 门槛。
- [x] 定向 corecmd/app/help/Schema dependency tests 通过。
- [x] clean head Go 1.25.9 双平台完整 suite、race、cache 与性能 workflow。
- [x] Darwin/arm64 native full/race/performance artifact。
- [x] Linux/amd64 native full/race/performance artifact。
- [x] root help RSS 与 Lxxx 的新 head 对比已记录。
- [x] root help RSS 两平台 p50 ≤50 MiB、p95 ≤55 MiB，且相对固定 main 降低。
- [x] Lxxx 差值与节点数差异已记录；Linux +11.3% 作为后续压缩诊断，不替代产品预算。
- [ ] 正式 release 最终签名制品与安装验证。
