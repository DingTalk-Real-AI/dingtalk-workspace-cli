# Plan：DWS 默认入口性能超过 Lark CLI

状态：Draft / 实施中。日期：2026-09-06。文档与组件优化完成不表示默认入口性能已经超过 Lark CLI。

范围更新：用户明确本次针对 PR #1296 做性能专项。当前执行顺序以[技术方案 §1.1](design-cli-performance-parity.md#11-本次专项的直接工作包)的 O0–O5 为准；下文单二进制/telemetry 新合同是结构成本无法满足目标后的后续备选，不是本次专项的默认前置改造。

最新用户决定：本轮目标是**超过 Lark CLI**；GWS 继续测量并展示，作为进一步优化的对照。本轮不修改统计行为，保留 SDK 异步发送、退出前最多 300 ms 等待及现有投递语义。旧的双竞品 ×1.05 竞争性目标由本轮 Lark 严格比较取代；当前 RFC 的历史发布回归 gate 不变。

关联：[当前 RFC](rfc-schema-runtime-cache.md)、[五维报告](rfc-schema-runtime-cache-performance.md)、[原生证据](benchmarks/schema-cache/native-7cbf7f52/evidence.json)、[PR #1296](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pull/1296)。

## 1. 完成定义

追齐覆盖 **Schema、root/leaf help、version、普通命令与整体运行内存**，同时覆盖 Darwin/arm64、Linux/amd64 和 native/public 入口。必须测量用户安装后的默认行为，不能要求用户关闭 telemetry、预启后台进程或直接调用内部 core 才达标。

当前 PR #1296 完成了自身历史回归门槛。它没有证明竞品追齐；本计划是独立的下一阶段，不能把旧 RFC 的“竞品仅诊断”悄悄改成已经通过的竞品门禁。

本轮先对 PR #1296 的既有实现做性能专项。具体优化工作包见[技术方案](design-cli-performance-parity.md)；先取得真实路径的启动/RSS、SDK 完成时间线、生产哈希和 Schema 分配证据，再逐点优化。单二进制、outbox 与大范围按需 runtime 迁移仅作为后续备选，需要结构成本证据和独立合同评审。当前双二进制发布合同继续有效。

“确保追齐”由自动化阻挡条件落实，不承诺未经测量的收益；任何维度未达标都保持未完成，不以另一个维度的收益抵消。

## 2. 已知差距与尚未证明的原因

事实绑定实现 `7cbf7f529d347bd0e9750687138dd2be009c5bcb`、源码树 `9dfbbca99267c7eaca3e6fe81fc1748888d3d682`、[成功 run 34018840739](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/actions/runs/34018840739)。PR head `35d7613f` 在该实现后只增加文档/证据。固定竞品为 Lark CLI 1.0.85、GWS 0.22.5。

下表为 native 默认配置 wall p50，单位 ms。目标列仅展示这批证据的数量级，不是后续 CI 可直接复用的机器无关常数。

| 平台/场景 | DWS | Lark | GWS | 本轮比较对象：Lark（必须更低） |
|---|---:|---:|---:|---:|
| Linux Schema | 375.45 | 46.06 | 4.34 | 46.06 |
| Linux root help | 305.99 | 46.42 | 3.27 | 46.42 |
| Linux version | 305.08 | 45.25 | 3.14 | 45.25 |
| Linux leaf help | 394.70 | 45.52 | 4.67 | 45.52 |
| Linux dry-run | 389.67 | 46.61 | 4.84 | 46.61 |
| Darwin Schema | 368.49 | 41.97 | 8.38 | 41.97 |
| Darwin root help | 320.22 | 42.15 | 6.80 | 42.15 |
| Darwin version | 319.54 | 41.73 | 6.83 | 41.73 |
| Darwin leaf help | 382.30 | 41.07 | 8.81 | 41.07 |
| Darwin dry-run | 377.57 | 41.73 | 8.02 | 41.73 |

内存同样是目标：当前 native root help 约 12–14 MiB、Schema 38–43 MiB；GWS 对应约 7–8 MiB、9–10 MiB。不能只移除 300 ms 等待就宣称五维完成。

证据支持三处改造重点，但还不支持精确的可加总耗时归因：

1. `third_party/aem-go-sdk/clitrack/clitrack.go` 的 `Tracker.Run → close` 在命令结束后等待 inner Close，默认上限 300 ms；`aem.Tracker.Close` 等待发送 worker。**300 ms 是上限，不是写死的 sleep。** default/opt-out help/version 的实测差值接近此数，仍需本地可控 collector 和网络故障实验区分正常投递、网络阻塞与 SDK 关闭行为。
2. `internal/launcher/launcher.go` 每次委派都打开并 SHA-256 校验完整 core；`delegate_unix.go` 随后调用 `syscall.Exec`。Unix 上这是替换当前进程，不是常驻父子双进程。已有 Python 全量 SHA 诊断 p50 为 Darwin 22.38 ms、Linux 35.41 ms，只能证明成本量级，不能当作生产 Go 校验分段耗时。
3. `internal/app/root.go` 仍承担命令树、utility/product 命令挂载及生命周期。默认 Schema 由 core 执行，launcher 只允许 opt-out Schema。Go package init、默认配置读取、命令装配、typed decode、输出缓冲各占多少需要 profile；不能全部归因于 Cobra，也不能预先声称换语言就能解决。

## 3. 不允许用来“达标”的替代行为

- 不关闭默认 telemetry，不只上报更快的 `DO_NOT_TRACK` 数据；不把 flush 改短后忽略投递丢失。
- 不把安全检查换成 size/mtime 缓存或“上次校验通过”；不把用户可写 manifest 当信任根。
- 不删除 Schema/flags/safety/result 字段、缩小返回范围或跳过 validation/confirmation 来压低耗时。
- 不为 config/dry-run/mock 在 launcher 复制业务执行器；按统一命令框架装配和执行。
- 不把常驻进程、异步 helper、outbox 的内存与启动成本移出统计；不把返回码失败或空输出当快样本。
- 不把 p50 单项、某一 OS、某个入口或平均值达标外推为全矩阵完成。

## 4. 候选设计及必须先解决的冲突

| 决策 | 本计划方向 | 旧合同冲突 / 实现前交付 |
|---|---|---|
| 产品形态 | 保留当前双制品，先优化认证消费与初始化；单二进制只在另行决定并通过真实依赖/RSS 与信任实验后进入后续版本 | 取代当前 RFC §1 的双二进制选择、§6 的绑定与发布流程；提交单独设计补丁和迁移矩阵，不能直接绕过旧 gate |
| 制品信任 | 新包在下载/安装/升级时验证可信发布摘要和签名，安装后使用明确的 OS/权限保护；保留 Schema payload 的二进制绑定与校验 | 逐次 launcher→core 哈希移除是信任边界变化；须逐平台写清本地篡改威胁、签名验证时机及限制。旧“双产物任一损坏在执行前拒绝”与新单产物承诺必须逐项映射。无法接受的威胁退化阻挡实施，不能宣称等价 |
| telemetry | 保留 SDK 异步发送和最多 300 ms 的退出等待；本轮只做成本与输出时序诊断 | 用户已明确不改统计行为；不引入 outbox、缩短等待、关闭默认统计或改变字段/投递方式 |
| delivery 单源 | 按需目录由现有 declarations 派生，presentation/Schema 和业务执行继续共用 `corecmd` 合同 | 不新增手工路由清单、Catalog 权威或一份只为性能维护的 help；依赖 gate 与全树/按需路径一致性测试先行 |
| 竞品门槛 | 新增独立 performance-parity gate；保留 v1 历史回归和正确性 gate | 不把同包 core +5% 门槛恢复成发布目标；竞争性完成由下述新矩阵决定 |

在保留当前上报行为的条件下，默认短命令可能持续慢于 Lark。此时仍记录“目标未完成”，不能删除默认模式、改用只看输出首字节或用 opt-out 代替。技术方案中的 outbox 是未采纳的备选分析，不再作为本轮里程碑。

## 5. 执行顺序与交付物

以下每项由对应模块负责人承担，具体人员在开工时指派；不用固定日期冒充未知工作量。每个里程碑单独 PR，依赖未完成不进入下一项验收。

| 阶段 | 责任模块与代码落点 | 必须交付 | 完成/继续条件 |
|---|---|---|---|
| P0：基线与分段测量 | 性能 harness；`scripts/dev/measure-cli-five-dimensions.py`、`measure-schema-default-entry.py`、内存 sampler | 固定三产品实际安装包；首次调用/预热、default/opt-out 分开；SDK close、core hash、exec/startup、config、树装配、decode/output 的独立诊断；CPU/alloc/heap/init profile；完整 argv/输出 oracle | 三产品所有计时调用成功、返回所需字段；分段解释和总耗时相符且标明重叠；找出前两大 wall 和 RSS 来源。先建立红色竞品 gate，不能只产图 |
| P1：SDK 生命周期诊断 | telemetry/SDK；`internal/clitelemetry`、`third_party/aem-go-sdk/clitrack` 与 `aem` | 核对业务完成、输出提交、清理、事件入队、Close 和退出时序；默认与 opt-out 数据分开 | 保留用户选定的原行为，记录等待成本；不以改变投递方式消除红色指标 |
| P2：单原生制品与信任迁移 | runtime/release；`cmd`、`cmd/dws-launcher`、`internal/launcher`、packagemanifest、post-goreleaser、安装器 | 明确选择单二进制的 v2 RFC；新 manifest/install/upgrade/rollback；移除内部委派自校验链；裁剪 package init 和静态依赖；两个 OS 的真实签名/安装证明 | 不启动/哈希旁路 core，制品验证和旧版本升级回滚通过；root help/version 默认及内存具备达到 GWS 的实测可能。未通过可信制品边界评审不能以性能名义合入 |
| P3：统一框架按需装配 | 命令框架；`internal/app/root.go`、`internal/corecmd`、`helpers.LeafSpec`、product declarations | 一条 owning declaration→路由→按需产品/叶子的路径；Schema/presentation 不初始化业务 transport/auth；业务调用仍经统一 validation/safety/lifecycle；full export 保留全量装配 | root/leaf help、Schema 与真实执行的 alias/flags/required/safety/result/locale 一致；config/dry-run/mock 相对旧入口回退消除；错误路径与扩展委派不丢合同 |
| P4：Schema 与 RSS 尾差 | Schema/runtime；schemareader、schemacache、schemaruntime、roothelp、profilemetadata | 依据 P0 profile 减少冗余 decode/复制/索引和静态初始化；选定 product/leaf 精确消费；解码前完整认证；必要时另审版本化格式变更 | Schema native p50/p95 与 RSS 都通过竞品 gate；不得用 mmap 后的低 RSS 隐藏缺页/映射成本，需另报 page faults/mapped bytes；缓存 miss 仍权威自愈 |
| P5：public 与所有命令 | npm/install 与统一 runtime；`build/npm/bin/dws.js`、installers、性能 harness | npm wrapper 依赖和同步检查 profile；减少重复解析/进程工作；验证 public 启动真实最终二进制；普通命令独立 CPU/RSS 改善 | public 对 public 五场景以及普通命令合同通过；native/public 不能混比较；单个 help 结果不能覆盖实际命令 |
| P6：稳定性与发布 | CI/release；新增 parity workflow/report，沿用全量 suite/声明/制品验证 | 三轮独立完整同机交错测量；稳定版本 final-artifact proof；最终五维报告、完整失败/样本归档；旧包→新包→旧包回滚 | §6 全部指标逐格通过，telemetry/安全合同完成，无未归因回退，正式安装后复核同样通过才标记“追齐” |

本次先执行技术方案的 O0–O5，优先定位并优化现有实现。上表是完整追齐的条件路线图；涉及产品形态和上报语义的 P2 及相关迁移，必须有结构下界证据后另行决定。不得通过在旧 launcher 添加业务快路径替代统一框架优化。若单二进制的 Go 启动或 RSS 下界仍高于目标，必须在 P2 输出最小真实合同实验及失败数据，另提 runtime/backend 设计评审；不得预先假定换语言必然成功，也不得直接降低目标宣布完成。

## 6. 追齐门槛：逐格判定

### 6.1 竞争性矩阵

矩阵固定为 **2 个 native OS × 2 种入口（native/public）× 5 个场景（Schema/root help/version/leaf help/dry-run）= 20 格**。每格对 wall p50、wall p95、独立内存 peak RSS p50/p95 四项分别判定，总计 80 个判定。每一项都满足：

`DWS < Lark CLI`

这是按用户最新目标“超过 Lark”做逐项严格比较，GWS 列不参与本轮完成判定。不使用 5% 容差将更慢认定为超过；不得在失败后加常数毫秒、跨平台平均或只看 p50。public RSS 是同时进程树峰值采样，同方法对比；native RSS 是低开销 C 父进程的内核峰值。采样缺失或测量不确定时结果为 incomplete，绝不能通过。

每轮每格至少 100 次有效耗时 + 100 次独立内存试验，预先固定随机顺序种子。连续三轮独立分配的 runner 会话全部通过，每轮都在同一机器交错比较三产品；不能取三轮最好值。失败样本、超时、无输出、错字段/退出码均失败，不通过删除 outlier 补齐。发布复核按同样矩阵测最终安装包。

CPU user/system、minor/major page faults、输入输出字节、样本顺序、OS/CPU/工具链/签名状态、负载与二进制/依赖 SHA 一并保存。网络以可复现 collector 下的独立因果实验和真实默认用户入口两套结果呈现；真实默认模式决定用户体验是否追齐，实验环境不得替代它。明确记录各产品的上报策略；默认行为差异属于结果的一部分。

固定竞品版本用于实现期间稳定归因；P6 再确认准备发布时可获得的版本。若版本变化，开一个新锁定批次完整重测并报告差异，不能在运行中自动 latest 漂移，也不能只选旧的慢版本。所有输出合同先人工核对，再由 oracle 测试保护；Schema 必须是相近能力的完整所需信息，不能只验证非空 JSON。

### 6.2 普通命令、缓存状态与整体内存

- config/mock 没有可靠的一一对应竞品合同，不伪造跨产品比较：在现有 `config list --json`、`calendar book list --dry-run/--mock -f json` 上，默认入口 wall p50/p95 与 native/public RSS p50/p95 均 ≤固定 pre-PR 基线 1.05 倍。dry-run 还须通过竞争性矩阵。这只证明这些普通命令，其他业务另建匹配 fixture。
- 从真实的缓存机制区分：首次安装首次调用、cache 冷/损坏修复、warm 命中、warm 命中但未预启服务。当前 20 格主要覆盖 warm；首次调用完整报告且相对冻结 DWS 基线不得回退超过 5%。竞品无对等修复语义时不得硬拼比例。Schema full export、product、overview、leaf 分别做正确性和既有资源预算，不能用 leaf 结果外推 `--all`。
- 长期事件、真实 RPC、分页请求是独立负载：建立本地可控服务的短响应/分页大响应/流式事件 fixture，保留 auth、validation、安全与输出路径，禁止绕过框架。记录稳态 RSS、峰值、累计分配与队列长度；相对冻结 DWS 基线回退 ≤5%，持续 30 分钟不得随事件/页数线性驻留增长。没有对应 Lark/GWS 负载时只报告自身变化，不发表跨产品吞吐结论。
- 若出现 helper/worker/共享 daemon，必须测冷启动、整个生命周期 CPU、同时进程树峰值、后台空闲 RSS 和持久队列大小。worker 成本无法可靠归属的场景标为 incomplete；不得沿用“单进程 kernel peak”宣称总体内存追齐。
- 成功、业务失败、validation 失败、broken pipe、SIGINT/SIGTERM、第二次信号都检查退出码、清理顺序、单次上报和零部分输出；二进制/缓存损坏检查拒绝/自愈合同不退化。

## 7. 代码评审与验收清单

以下为完整验收证据，当前尚未全部完成；本计划的存在、#1296 全绿或一个原型结果都不能替代勾选所需证据。

- [ ] C0：P0 的当前制品、竞品锁、红色 parity report、真实生产路径分段 profile 已归档。
- [ ] C1：既有 telemetry 合同回归通过；默认等待数据和业务输出时序明确，未修改投递方式/等待预算；默认模式差距保留。
- [ ] C2：当前双制品信任、安装/upgrade/rollback 合同通过；只有另行决定迁移形态时，才追加 v2 RFC 与两平台信任差异验收。
- [ ] C3：装配优化保持统一框架的 Schema/help/flags/safety/error 合同；若另行实施按需装配，追加全树与按需 route 的双向一致性验收。
- [ ] C4：native 10 格全部四指标通过；config/mock/真实 fixture 自身回归合同通过。
- [ ] C5：public 10 格全部四指标通过；测量覆盖所有同时驻留和后台资源。
- [ ] C6：三轮完整 80 判定通过、没有缺样本/未归因数据；当前发布竞品版本有独立复核。
- [ ] C7：最终正式安装包复验、签名公证、升级/整包回滚通过；五维最终报告与 raw evidence 固定 commit/run/artifact SHA。

每个失败项必须落到代码位置、负责人模块、下一实验与可验证结果；没有完成 C0–C7，就保持追齐工作未完成。计划交付后第一项工作是 P0，不是继续扩 launcher 快路径，也不是把历史性能附件改成“已追齐”。

实施记录：[PR #1296 性能专项实施记录](performance-optimization-progress.md)。
