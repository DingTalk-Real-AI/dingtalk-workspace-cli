# DWS 命令框架性能专项 Plan

状态：Draft，本地实施完成，待 clean-head 两平台验收。日期：2026-09-06。

本计划承接 [PR #1296 执行计划](plan-cli-performance-parity.md) 与 [单入口 RFC](rfc-schema-runtime-cache.md)。目标是减少 route / assemble / prepare 成本，保留统一 MCP、参数校验、鉴权、Safety、离线 Schema 和同步业务清理。

当前 #1296 的单入口、telemetry 退出策略属于前置工作；后续框架专项不再改变其保证。版式裁剪、Go 映像/init 专项、追平 GWS 的绝对延迟不属于本计划验收。

## 1. 决策与源码现状

| 项目 | 已观察到的状态 | 本计划决定 |
|---|---|---|
| 产品选择 | `mountLegacyPublicCommandsFor` 已改为全量/目标二选一 | 工厂计数证明目标调用一次、无关工厂零次 |
| 路由目录 | name/alias/factory 与 shortcut service 在注册时建立同源 map | 产品查询约 9.6～10.0 ns；shortcut service 约 7.3～7.5 ns |
| 扩展判定 | clean path 固定一次 Lstat、一次 ReadDir、一次 settings ReadFile | 证明无插件后跳过后续 loader；不确定状态完整回退 |
| utility 依赖 | `event +listen-im` 使用共享 helper caller | 选择性构树仍须调用进程级 runtime 初始化；不能凭 utility 分类省略依赖 |
| Completion | 生产代码没有 `RegisterFlagCompletionFunc`；补全请求必须完整构树 | 不存在全局 callback map 的 DWS 注册泄漏；门闩落在启动路由 |
| Prepare/config/profile | 生产代码没有额外 `PrepareCommandTree`；单 profile 业务执行解析一次，dry-run 为零次 | 未发现可安全删除的重复点，保留现有 auth/Safety 生命周期 |

Lark 的 completion、插件和装配优化仅作为待核验的参考线索。引用时记录仓库、完整 SHA、文件位置与复现方法；不以此前对话中的描述作为 DWS 的实现或收益依据。

## 2. 实施顺序与交付

### P0：修复按需装配并建立可信基线（当前 #1296）

- [x] 将 `mountLegacyPublicCommandsFor` 改为全量/选定工厂互斥调用，避免先构建后丢弃。
- [x] 用工厂调用计数验证目标工厂调用一次、无关产品工厂调用零次；utility 不进入产品挂载分支。
- [x] 保留 `initializeLegacyPublicRuntime`：静态 endpoint、runtime default 和 caller 初始化与工厂构建分离。
- [x] 完整树与选择性树比较目标命令的 flags、alias、约束、Safety 和输出元数据；覆盖 calendar 与 chat/im 路由。
- [x] 回归 `event +listen-im` 的目标解析和 dry-run 无订阅写入，保留原命令允许的解析 RPC。
- [x] 修复后重测：calendar 约 0.84～0.86 ms、1.17 MB、10.1k alloc；旧 7.1 ms 数据包含被丢弃的全量 helper 构建。

交付：正确性修复、工厂调用证明、固定 SHA 的构树基线。P0 是后续专项的前置，不因后续优化尚未实现而推迟。

### P1：分段测量、completion 与装配 I/O 审计

- [x] 诊断报告新增 `startup_surface`、`startup_route`、`runtime_init`、`product_assemble`、`plugin_discovery`、`preparse`、`command_execute`、`business_cleanup`；启动与执行子阶段标记为 `nested`，展示但不重复计入 `overhead_ms`。正常输出不变。telemetry exit 由外层 no-wait 单测证明。
- [x] 枚举 completion 注册：生产代码没有 `RegisterFlagCompletionFunc`；现有能力是 Cobra 的脚本生成与隐藏补全请求。
- [x] 因没有 DWS 注册进入 Cobra 全局 callback map，不存在可复现的该类保留链，不增加无效 heap 门闩。
- [x] 路由门闩识别 `completion`、`__complete`、`__completeNoDesc` 并完整构树；公共/嵌入构造器继续完整。
- [x] 普通业务进程没有补全 callback 注册；补全模式保留完整产品命令。
- [x] 审计构造期 I/O，并用 seam 固定 clean path 的三次资格检查；产品工厂不读取 settings/profile。
- [x] clean path 的 settings 读取用于 dev plugin 资格；证明插件不存在后不再进入会重复读取 settings/token 的 plugin loader。

交付：分段基线、completion 调用/保留链报告、需要时实施门闩、装配 I/O 次数门禁。若未发现注册或泄漏，提交审计结论，跳过无收益改动。

### P2：同源路由索引（依赖 P0/P1）

- [x] 产品 canonical name、alias、factory 与 shortcut service 来自同一运行时注册源；索引不承载 Safety、handler 或 Schema 真相。
- [x] 比较线性与 map：产品表仅约 11.3 ns → 9.6 ns；shortcut service 约 2.10 µs → 7.4 ns。保留注册时 map，不增加生成器。
- [x] 不采用生成式索引，因此没有生成文件或 drift 面；一致性测试直接校验 map 与注册权威。
- [x] 处理 utility/product 同名、chat/im alias 与 edition 条件；无法静态确定的路由走完整框架。
- [x] 覆盖全局 flag 值、附着短选项、`--`、未知 token 和 completion；Cobra 保持最终解析权。
- [x] 索引查询不调用工厂；启动仍包含 O(argv 长度) 解析，不宣称整个启动 O(1)。

交付：同源索引、完整树路由一致性测试；若有生成器则附 drift gate。若线性查找只占噪声量级，保留简单实现并记录不做生成器的决定。

### P3：插件发现延迟（依赖稳定路由）

- [x] 插件影响面列为命令、endpoint/stdio、PreParse hook、配置环境、身份上下文和 skill 同步。
- [x] 只有同次启动检查证明 user/dev plugin 均不存在时跳过 loader；不只依赖内建命令命中。
- [x] shortcut 目录存在或不可判定时完整回退；空目录也回退。
- [x] 不使用持久“无插件”缓存；同次检查结果只消除后续 loader 的重复 ReadDir/settings/token 读取。
- [x] 未知命令、completion、插件/edition/异常配置继续走完整树和原 loader；测试证明 loader 在扩展状态下仍调用。

交付：扩展影响矩阵、等价测试和扫描成本报告。如果现有插件合同允许全局修改已知内建命令，则保留发现步骤，仅消除重复工作。

### P4：Prepare 去重证明（依赖 P1）

- [x] 生产路径只有一次 framework PreParse；Cobra 保留 leaf PreRun/required/group/Run 顺序，没有额外 `PrepareCommandTree`。
- [x] 单 profile 业务执行的 resolver 调用为一次，dry-run 为零次；multi-profile 的完整 selector 探测与逐项解析语义不同，不去重。
- [x] 构造期未发现产品数相关的 profile/config 读取；扩展资格固定三次 I/O 已独立记录。
- [x] 没有建立跨调用配置/auth 缓存，避免 profile 切换、配置写入和长运行 event 使用陈旧状态。
- [x] 没有删除 auth/Safety/业务 cleanup；现有错误分类、取消、一次业务请求与输出原子发布回归继续作为验收。

交付：调用前后对照及有收益的最小改动。无显著收益时保留原链路，以测量报告完成该阶段。

## 3. 路由、诊断与短路矩阵

以下是本计划的目标合同，未实施项不能描述为现状。

| 输入/环境 | 产品装配 | 必须保留的行为 |
|---|---|---|
| 已知 calendar list/get，无扩展 | 仅目标产品 | 统一 PreParse、所需 auth/validation/Safety、业务 handler 与 cleanup |
| config 等 utility，无扩展 | 无无关产品 | utility handler、共享 runtime 依赖与输出生命周期 |
| event +listen-im | utility 自有命令及必要依赖 | caller 可用；目标解析与原有 dry-run 合同 |
| Schema 可验证命中且无扩展影响 | 不构业务树 | 完整输出、错误/信号语义；miss 使用同一声明源 |
| root help/version，无扩展 | 使用当前受支持的帮助/版本路径 | 不引入业务鉴权/RPC；保留进程诊断与输出语义 |
| leaf help，无扩展 | 目标产品 | flags、别名、Safety 帮助内容等价，不执行业务 |
| shortcuts/ 存在，包括空目录 | 完整树 | 用户 loader 与畸形文件警告；schema/config/help/version 均覆盖 |
| plugin/edition hook 或资格不确定 | 完整树 | 原扩展发现、冲突检查、PreParse 与诊断 |
| 未知 argv、无法可靠解释的全局 flag | 完整树 | 原错误、建议和退出码 |
| completion 请求 | 保守完整树并启用补全能力 | 原补全候选、描述、directive、alias 和 shell 脚本合同 |
| 公共/嵌入 root constructor | 默认完整能力 | 不从宿主 os.Args 推断或削减功能；优化须显式 opt-in |

存在性检查是调用时观察，不承诺消除并发安装的所有竞争；发现异常时回退。需要更强的一致性保证时，先定义配置版本/锁协议。

## 4. 测量与验收

维护两个固定对照：#1296 的 PR-base main `6f71222b9b07c760cdb5f376b24dab9155e62094` 用于累计收益；后续若拆分独立 PR，则在开始时固定父提交用于专项增量。每份报告记录完整 SHA、binary digest、Go 版本、平台与工作树状态。

代表集必须包含 calendar list、一个真实存在的 get 路由、两者 dry-run/mock、config、Schema hit/miss、root/leaf help、version、event、空/畸形 shortcut、插件与 completion。测试采用隔离配置和本地 mock，不把鉴权失败或业务失败作为性能成功样本。

| 验收项 | 阻断条件 |
|---|---|
| 按需工厂 | 无扩展业务命令调用无关产品工厂，或目标工厂被重复调用，即失败 |
| 接口与执行合同 | flags/alias/约束/Safety/错误码/输出不同，启动诊断丢失，业务执行或清理次数改变，即失败 |
| 构树性能 | P0 沿用 RFC 的构树门槛；后续同机交错比较专项父提交，assemble 中位数改善至少 10% 才宣称性能收益 |
| 回归 | default 及 opt-out 分开测；代表场景 p50 超过 `max(父提交 × 105%, 父提交 + 3 ms)`，或 p95 超过 `max(父提交 × 110%, 父提交 + 3 ms)` 时阻挡专项合并 |
| completion 保留 | 普通模式不新增仅供补全的 callback；重复 Build/丢弃/GC 后不得保留随批次线性增长的命令树，诊断须指向对象保留链 |
| 路由一致性 | 索引与权威完整树不一致即失败；生成式实现必须零 drift |
| I/O | 产品/leaf 构建增加不能增加配置读取次数；扩展发现例外单列，不能隐藏进 assemble 收益 |
| Prepare | 删除后的 auth/Safety/业务调用与 cleanup 合同不等价即失败 |

每场景至少 30 次随机交错延迟样本；RSS 独立采样，同时报告 CPU、allocs、B/op、峰值 RSS 与 GC 后存活 heap。关键分段至少 5 轮微基准；测量不并发跑全仓测试。噪声或结果接近门槛时补样本，不能挑单次最好结果。

completion 泄漏测试使用预热后多个等规模 Build/GC 批次，结合 heap 保留链判断；一次高 RSS 不等于泄漏。Darwin/arm64、Linux/amd64 的 clean-head 正确性、受影响 race 测试及性能结果共同验收。

Lark/GWS 只进入诊断附表；版本、native/public 入口和输出范围写清，不能使用竞品门槛代替本计划回归门禁。

## 5. 收尾清单

- [x] P0 正确性修复与新构树基线完成。
- [x] P1 completion/I/O 审计完成，有证据的路由门闩与次数约束落地。
- [x] P2 采用同源注册 map；微基准不支持增加生成器，索引一致性测试已落地。
- [x] P3 clean path 延迟插件发现，扩展/不确定状态保持原 loader。
- [x] P4 未发现可安全删除的重复 Prepare/profile/config；调用次数证明后明确保留。
- [ ] 两平台 clean-head 分段报告、回退矩阵和测试通过。
- [ ] 各 PR 正文只声称本次已验证收益；累计报告与专项增量分开。

当前 P0～P4 随 #1296 一并验证；后续若拆分，回滚以专项提交/PR 为单位。性能资格失败时可回退完整构树；已经开始业务执行或发布输出后不得重新执行命令。Schema/契约权威源、MCP 和 Safety 不随性能方案回滚而改变。
