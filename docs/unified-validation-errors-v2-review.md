# 统一校验框架 v2：复审记录

状态：Draft。日期：2026-09-06。复审与验证按提交轮次记录；最新的延迟节点修复和原生边界优化见文末。保留 Draft，不代表合并准入已通过。

本轮以 PR #1292 的 `a2a7d779` 和当前工作区为起点。原有验证记录代表原有用例通过，不构成所有 Cobra 执行路径均已覆盖的证明。

## 发现与处理

| 优先级 | 发现 | 证据与处理 |
| --- | --- | --- |
| P1，已修复并验证 | `TraverseChildren` 的父级参数解析绕过 `FlagErrorFunc` | 用户确认最小依赖补丁后，父级解析失败调用当前节点的有效 handler，nil 回退保留原始错误。真实 `ExecuteC` 的 Find/Traverse、多层 local/persistent flags、未知/值错误、处理器选择与错误身份回归通过。 |
| P1，已修复并验证 | wiki 代理手动解析未接回框架 | 原二进制 `dws --format json wiki list --unknown-review-flag` 返回 internal/退出码 5。工作区改为调用目标 `FlagErrorFunc`，新增真实代理执行回归，验证 nil fallback、API、取消和超时，失败时业务调用数为 0、目标 handler 恰好调用 1 次。重新 `make build` 后，同一命令已返回 validation/退出码 3，stdout 为空，stderr 保留目标 `dws wiki space list --help` 提示。 |
| P2，已优化 | 整树快照和每个闭包保存了无关钩子 | 仅快照具有继承关系的 flag handler；在第二遍读取节点自己的 Args/PreRun，闭包捕获最小函数集合。已有快照、失败原子性、顺序与身份测试通过。 |
| P2，已加固 | 专项门禁没有输出覆盖数量，且未包含 Traverse 路径 | 现在报告真实树节点与通过的扩展场景数量，并要求代理解析和 ValidationTraversal 回归 run/pass；增加本地 Cobra 源码完整性与全量依赖测试。补丁后门禁通过。 |

Cobra 的 [Traverse 实现](https://github.com/spf13/cobra/blob/v1.10.2/command.go#L821) 直接返回 `ParseFlags` 的错误；其普通 `execute` 路径才调用 `FlagErrorFunc`。当前 app 根默认使用 Find，但配置、认证、dev 等组带有 `TraverseChildren: true`，独立执行这些组属于需要验证的路径，不能只用 app 根成功证明全部接入成立。

保持无独立执行句柄、保留遍历语义、保持 Cobra 不变，这三个要求在当前公共 API 下不能同时兑现。用户于 2026-09-06 明确确认最小依赖补丁，调整“不修改 Cobra”的约束；执行入口和遍历支持范围保持原方案。

补丁覆盖 `Traverse` 的 `ParseFlags` 失败分支及 `ExecuteC` 对应报错分支：调用 handler、返回解析失败节点，并保留根命令静默策略。`third_party/cobra` 用 Go module replace 接入，保留全部上游包与测试、许可证、原始校验和以及可逆补丁。依赖门禁反向应用补丁后检查上游文件原始哈希，再运行 Cobra 与 doc 全量测试，均已通过。DWS 回归覆盖多层父级、局部/持久 flag、未知 flag/值错误、最近 handler 恰好一次、nil fallback、API/ExitCoder/取消/超时身份；成功路径验证 alias 命令选择、flag 值和钩子顺序。依赖内部没有引入 DWS 分类策略或第二执行器。

## 逐项核对

| 方案要求 | 当前证据 | 判定 |
| --- | --- | --- |
| 一个错误保护策略 | `PreserveClassification`、NormalizeValidation、app 提示、helpers 包装共用；typed/ExitCoder/取消/超时多层包装回归 | 已证实 |
| Tier1/Tier2 共用失败后不继续的校验边界 | `WithValidation` 与 TierParity 的直接 RunE/真实 Execute 用例，原业务错误身份断言 | 已证实 |
| 仅框架拥有的阶段自动分类 | 原普通 PreRunE 错误保持身份；既有 P1 review 对该问题的指摘已由当前实现和回归解决 | 已证实 |
| 最近 handler、nil fallback、重复准备原子失败 | `TestCrossPlatformCoverageValidationAdapters` / `PrepareCommandTree` | 已证实；精简快照后复跑通过 |
| 别名处理后检查 required/group，保留确认顺序 | PreRun 归一化、required/group、ConfirmFirst 和 Tier2 deferred confirmation 回归 | 补丁后专项门禁与全量复验通过 |
| 独立根、嵌套树和扩展的真实解析 | Find、Traverse 和 extension fixtures 真实执行回归 | 补丁后专项门禁通过 |
| 代理解析接回统一边界 | 原二进制复现；新 ProxyParseValidationBoundary 和 app 代表用例通过；重新构建的真实二进制返回 validation/3 | 已证实 |
| 重复执行保留状态，独立调用创建新树 | Cobra reuse/fresh fixture，凭证和 version 生命周期回归 | 已证实 |
| I/O 不被误归类 | 招聘文件读取显式 internal/cause；drive 取消/超时保留；输入读取错误已有来源分类 | 已证实本轮涉及的路径；不宣称整个仓库业务错误已迁移 |
| Help、Schema、输出契约 | 最终全量、二进制输出回归通过；固定 PR head 的 Linux 生成漂移、确定性和 Schema 门禁通过 | 已证实 |
| 性能在可比条件下确认 | 固定编译产物、同一时段交替采样，见下表 | 原 20.5% 跨时段差异未复现；新增分配已显著收敛 |
| CI | 新增专项 CI 对 PR head 的依赖、框架、生成与 Schema 检查全部通过；既有 AI Behavior 因 Draft 拒绝准入，Draft Fast Gate 在 merge 父节点与事件 base 不一致时失败 | 本轮代码验证完成；仓库合并准入未通过，保持 Draft |

## 前轮补验状态（S1–S6 跟进前）

- `DWS_PACKAGE_VERSION=0.0.0-test go test -p 2 ./... -timeout=20m` 退出 0：app 441.286 秒、helpers 121.404 秒、test/scripts 421.270 秒。随后新增的 parser cause 身份用例随最终专项门禁复跑通过；生产代码未再改动。
- `make build` 通过。最终二进制在隔离配置下，wiki 代理、audit、OA 的参数错误返回 stderr legacy validation/3，sheet revision-get 返回 stdout unified validation/3；实际退出码均为 3，另一输出流为空（wiki stderr 保留重定向提示）。
- 加入 Traverse 回归后，门禁曾以该用例失败阻止验收；依赖补丁后已退出 0。Cobra/doc 全量测试通过，DWS 真实树扫描为 1,809 节点，5 个扩展子用例全部通过。
- 本地 `check-generated-drift.sh` 未完成：生成器在启动时收到 SIGKILL（退出 137），更换临时目录及 `GOFLAGS=-a` 全量重编译均未改变结果。单独生成器的磁盘签名校验有效，执行仍以 -9 退出；系统日志仅提供终端防护对该进程的标记，没有明确终止原因。未修改主机防护设置，未将中断记为内容漂移或检查通过。
- 固定 PR head `ee66854953d1cb0c9be4cb80c8304cab5018f6b2` 的 [Linux CI #33985390886](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/actions/runs/33985390886) 全部通过。日志明确 checkout 该提交：依赖及框架门禁覆盖 1,809 节点、5 个扩展场景；生成漂移与两次组装确定性通过；Schema 契约通过（31 产品、1,357 工具）。此结果补齐本地独立生成器无法执行的验证，不改写本地 SIGKILL 记录。
- 较早的 [CI #33984836718](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/actions/runs/33984836718) 也通过，但默认 checkout 的是合并预览 `f4f87d53`（含主线 `d39d7590` 的额外命令，1,825 节点/1,370 工具）。已修正 workflow 固定 PR head；不将合并预览的数据冒充本分支数据。
- 本地全量测试、构建及最终性能基于 `b3206646` 的生产代码；该提交至 `ee668549` 的全部 Go 源码、go.mod/go.sum 均无差异。至 `35d59111` 的交付只更新复审与方案文档；以下 S1–S6 跟进另有失败分支改动与独立验证。
- 发布说明片段门禁、Go 格式与变更空白检查通过。Cobra 模块归档与 go.sum 的 h1 校验和匹配，42 个上游文件的原始哈希已逐一对照归档确认。

## 性能复核

基线是可追溯的提交 `0758f351`，候选一是已推送的 `a2a7d779`，候选二是该提交加工作区的精简快照与代理解析修复。它们各自用 `go test -c` 编译 app 基准二进制，再在相同 cwd 和隔离 `DWS_CONFIG_DIR` 下交替运行。此处的旧提交基线不同于上一轮含未提交修改的工作区基线，两批数据不能混成一批统计。

每组 10 对，交替先后顺序，每次 `-test.benchtime=100x -test.count=1 -test.benchmem -test.run=^$ -test.bench=^BenchmarkNewRootCommand$`。正式测量期间没有本任务的其他测试/构建；桌面进程负载仍会变化。

| 对照 | 旧版 ns/op 中位数 | 候选 ns/op 中位数 | 旧版 B/op | 候选 B/op | 旧版 allocs/op | 候选 allocs/op | 配对耗时差值中位数的 bootstrap 95% 区间 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 已推送 Draft | 16,409,053.5 | 16,010,015.0 | 17,270,434.0 | 17,541,139.0 | 160,874.5 | 159,821.0 | -4.62% 至 +1.31% |
| 精简快照后 | 19,918,003.0 | 19,612,594.0 | 17,270,144.5 | 17,325,651.5 | 160,873.5 | 159,821.0 | -11.59% 至 +3.96% |

精简后相对同批旧版增加约 0.32% B/op（约 55.5 KB），比未优化 Draft 少约 215 KB/次构建；分配次数相对旧版减少约 0.65%。两轮的配对耗时区间均跨过 0，没有检出稳定的构建变慢。没有以这个结果宣称跨机器、跨负载绝对零退化；原始样本保留如下。

Bootstrap 对每对相对差值的中位数重采样 10,000 次，种子 42，仅描述本次样本。

已推送 Draft原始样本：

```csv
pair,before_ns,candidate_ns,before_bytes,candidate_bytes,before_allocs,candidate_allocs
1,16265477,15863454,17271132,17539826,160874,159821
2,15902677,16362795,17270324,17540160,160874,159820
3,15870995,15768262,17270544,17540942,160873,159822
4,16153882,16149322,17269376,17539983,160874,159821
5,16713717,15708226,17269741,17544176,160875,159821
6,16024162,16463469,17269329,17543751,160875,159822
7,16990848,15788345,17270606,17539751,160875,159821
8,16552630,16409377,17271120,17542059,160875,159821
9,17359583,17338014,17271813,17542884,160875,159822
10,16638581,15870708,17269347,17541336,160873,159821
```

精简快照后原始样本：

```csv
pair,before_ns,candidate_ns,before_bytes,candidate_bytes,before_allocs,candidate_allocs
1,24573999,20605902,17272090,17327566,160874,159821
2,20131449,20936812,17270329,17324548,160875,159819
3,21467700,20097199,17276222,17325203,160877,159822
4,19704557,20081018,17271565,17326100,160874,159821
5,20921278,19368418,17272231,17324019,160873,159819
6,19499361,17490580,17267340,17326136,160873,159823
7,23569767,19856770,17269941,17328097,160876,159821
8,17138609,18580501,17269679,17328646,160873,159822
9,16690387,16144953,17269960,17325179,160873,159821
10,18500444,19232365,17267896,17324507,160872,159822
```

## 最终补丁版本性能复核
本节测量版本包含精简快照、wiki 解析修复和当时的 Cobra 三行补丁（后续 S1 仅调整失败分支归属与静默处理）。使用相同的 `0758f351` 基线二进制，10 对交替测量，每次 100 次构建；测量时本任务的测试和构建均已结束。
| 指标 | 基线中位数 | 最终版本中位数 |
| --- | ---: | ---: |
| ns/op | 13,957,918.0 | 14,153,880.5 |
| B/op | 17,270,153.0 | 17,325,608.0 |
| allocs/op | 160,873.0 | 159,820.5 |

分配字节增加约 55.5 KB（0.32%），分配次数减少约 0.65%。耗时中位数之比为 +1.40%；每对相对变化的中位数为 -0.67%，bootstrap 95% 区间为 -1.85% 至 +1.78%。两种统计量不同，不能将配对中位数写成整体加速；本组未检出稳定变慢，不承诺所有机器和负载绝对零退化。

固定基准二进制 SHA-256：

```text
before 58e3a97527a64b97608068e1876c1aeffc7258c52ceb99de5a83704b1846b152
after c346baeeaabdad7a86050d3bd72005b552e0d2746ff2cecc89bc6cece25eff67
```

最终原始样本：

```csv
pair,before_ns,candidate_ns,before_bytes,candidate_bytes,before_allocs,candidate_allocs
1,13591664,13528940,17270894,17321829,160874,159819
2,13906478,14459369,17270515,17326618,160873,159820
3,13934432,13724226,17267682,17325460,160872,159821
4,13869914,13560148,17268166,17324237,160872,159821
5,13839632,14408748,17269196,17324142,160872,159818
6,14503364,14375215,17270357,17327216,160874,159821
7,14526377,14207342,17269575,17324917,160872,159820
8,14527854,14467563,17271629,17325756,160876,159822
9,14064339,14100419,17271959,17327045,160875,159819
10,13981404,13733755,17269949,17327460,160873,159823
```

最终小型树执行基准（每场景 5 次、每次 1 秒，复用已准备的同一树、固定 argv；仅报告绝对成本）：

| 场景 | ns/op 中位数 | B/op 中位数 | allocs/op 中位数 |
| --- | ---: | ---: | ---: |
| success | 2,158.0 | 4,299.0 | 32.0 |
| invalid_flag | 1,757.0 | 2,850.0 | 30.0 |
| missing_required | 1,086.0 | 2,529.0 | 22.0 |
| invalid_positionals | 1,255.0 | 2,586.0 | 25.0 |
| invalid_parameters | 1,744.0 | 3,274.0 | 25.0 |

## S1–S6 代码建议跟进

| 建议 | 处理 | 验证 |
| --- | --- | --- |
| S1 失败节点归属 | Traverse 的 handler 结果与 nil fallback 均返回实际解析节点；ExecuteC 报错同时遵守 root 与该节点的 SilenceErrors，避免根静默失效 | 直接 Traverse/ExecuteC 的返回节点、group 帮助路径、四种 root/group 静默组合，以及 DWS 五种 flag 作用域的节点断言通过 |
| S2 required 文案 | 保留统一文案，changelog 明确旧/新差异；原始 Cobra 文案保留为 cause | required 错误的统一文案与原始 cause 双重断言通过 |
| S3 typed constraint 边界 | AGENTS 与方案明确 PreRunE 中该步骤不可由原生检查替代；真实树门禁要求 runnable 节点存在该钩子 | 无钩子/PreRun/PreRunE × Run/RunE × required/group 共 12 例；typed/3、原始 cause、业务不执行均通过 |
| S4 handler 返回保护 | 保持 NormalizeValidation 中的共享保护规则，补注释说明，不重复调用 PreserveClassification | 既有 API/ExitCoder/取消/超时身份回归继续通过 |
| S5 手动解析清单 | 扫描发现唯一 production Cobra 手动调用为 wiki proxy；门禁仅允许该已审核表达式 | 当前清单通过；临时新增 ParseFlags、Flags().Parse、PersistentFlags().Parse 三种调用均被拒绝。使用 POSIX 搜索以适配 CI，明确只覆盖常规单行表达式 |
| S6 nil 测试入口 | ExecuteCForTest 明确报参数错误；context 变体也先检查 nil，避免 panic | 四个辅助入口 nil 回归通过 |

本轮 `go test ./internal/corecmd -count=1`、统一校验专项门禁与 `make build` 通过。专项门禁反向应用包含 `command.go` 和原始测试预期调整的可逆补丁，42 个原始文件哈希仍匹配；Cobra/doc 全量测试、1,809 节点和 5 个扩展场景通过。上述新增用例均纳入门禁必跑/必过清单。新构建二进制的 wiki/audit/OA legacy stderr 与 sheet unified stdout 检查均返回 validation/exit 3，另一输出流为空。

前轮根命令构建性能样本没有重新测量：本轮未改变 PrepareCommandTree 构造逻辑或 Cobra 成功执行路径，只调整解析失败分支和 nil 测试入口。前轮全仓库测试结果是历史证据，本轮本地复验范围为 corecmd、依赖全量与专项门禁；PR head 的生成/Schema 验证由更新后的 Linux CI 执行。

S7 仍为流程事项：PR 与方案保持 Draft，旧 CHANGES_REQUESTED、leaf/OA 截图和 Code Admission 不在本次代码改动中解除。没有将已有评论代为撤销，也不把专项检查通过表述为合并准入通过。

## 延迟生成命令复审与原生边界优化

继续检查 `0c0bf188` 时，发现静态树准备无法覆盖 Cobra 在 ExecuteC 内新建的节点。独立准备根后，`completion bash unexpected` 返回普通 `errors.errorString`；无参数的 `__complete` 及其别名同样漏过 typed validation。实际 `ExecuteC` 复现用例先失败，证明这不是静态扫描数量能够排除的问题。DWS 主应用自定义 completion 的错误原先已正确分类；该发现主要涉及框架的独立执行和延迟默认命令。

修复在 Cobra 的原生 Args/required/group **失败点**增加带阶段标识的继承回调，由 corecmd 安装同一个分类器。默认依赖行为保持原错误；最近 handler 只调用一次，返回 nil 也不能放行失败。业务 PreRun/Run/PostRun 不经过分类回调。删除 corecmd 对 Args/PreRun 的包装和提前 required/group 检查，保留原生顺序与注解。S3 原先保护的提前检查由此被替代，新的门禁要求原生分类恰好一次、失败不进入业务。

应用的调用状态清理也接到原生失败回调，并由全树共享一个闭包；先生成诊断，再清理 flag 状态。这样既覆盖后建节点，又不在成功 Args 路径增加清理包装。延迟节点继承回调，并不伪装成已经遍历或标记 prepared 的节点。

代价是 Cobra 本地补丁维护面扩大：新增 ValidationStage 和继承回调 API，补丁已改名 `command-validation.patch`，仍可反向恢复原始 42 文件哈希。直接调用裸 Args/PreRun 或 Cobra 的 ValidateArgs/ValidateRequiredFlags/ValidateFlagGroups 不经过 ExecuteC 的回调；完整 typed CLI 保证属于原生执行入口。没有引入额外执行句柄、输出文案匹配、全局树注册表或新分类源。

### 本轮验证范围

- corecmd 全量测试与最终专项门禁通过；Cobra/doc 全量、原始源码哈希、1,809 静态节点和 5 扩展场景通过。
- 新增 9 个延迟节点场景：默认 completion 位置参数/required/group、隐藏补全及别名缺参、定制 help Args、三条合法补全/help 路径。
- 12 个 PreRun/PreRunE/无钩子 × Run/RunE × required/group 场景继续通过，并断言原生分类恰好一次。所有阶段保留 typed/ExitCoder/取消/超时身份。
- 依赖测试验证最近 handler、nil fallback、业务钩子错误不被处理、成功校验不调用 handler；应用验证延迟补全 Args 失败后清理凭证状态，后续调用不复用。
- `make build` 通过；新二进制四个 validation/output 场景继续返回 exit 3 且保持输出流，completion bash 与 help completion 正常生成内容。全仓库测试与本提交 Linux CI 的最终结果在 PR 对应提交记录中补齐；不以此前全量结果代替本轮验证。

### 本轮性能：固定 0c0bf188 基线

前后分别编译固定 app/corecmd 测试二进制；测量期间本任务没有其他构建或测试。根构建交替运行 10 对，每样本 100 次；小命令交替 5 对，每场景 200 ms。后续改动只涉及测试/文档，不改变测量的生产逻辑。

第一版原生回调给每个节点各建清理闭包，分配反而增加。最终改为全树共享一个闭包，下表是共享后的版本。保留该实验过程，避免把未测量的“更简洁”当作更快。

| 根构建 | 基线 | 当前 |
| --- | ---: | ---: |
| 中位耗时（ns/op） | 16,323,650.5 | 16,736,219.0 |
| 中位 B/op | 17,326,358.5 | 17,310,604.0 |
| 中位 allocs/op | 159,820.5 | 159,044.0 |

每对耗时变化中位数 +2.91%，bootstrap 95% 区间 −2.23%～+4.11%；当前数据未证实稳定变慢，也不证明绝对零回归。每次构建减少约 15.8 KB 与 776 次分配。

| 对次 | 基线 ns/op | 当前 ns/op | 基线 B/op | 当前 B/op | 基线 allocs/op | 当前 allocs/op |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 1 | 16176048 | 16826010 | 17321960 | 17311491 | 159819 | 159045 |
| 2 | 16418510 | 15881305 | 17325325 | 17310761 | 159819 | 159045 |
| 3 | 16408797 | 16477557 | 17327854 | 17310678 | 159822 | 159043 |
| 4 | 16159067 | 15798557 | 17323233 | 17309844 | 159820 | 159044 |
| 5 | 16199961 | 17183718 | 17325634 | 17310316 | 159820 | 159044 |
| 6 | 16505941 | 17230460 | 17327083 | 17311542 | 159821 | 159044 |
| 7 | 15849015 | 16456409 | 17323820 | 17308791 | 159818 | 159044 |
| 8 | 17243319 | 17814568 | 17328437 | 17310491 | 159823 | 159043 |
| 9 | 17313201 | 16877034 | 17328795 | 17311359 | 159823 | 159042 |
| 10 | 16238504 | 16646428 | 17328242 | 17310530 | 159822 | 159044 |

基准二进制 SHA-256：

- before: `aec859fc2d318501533c9a3cd608901dfc6c72d3cf42452be965fc9452f2e3ed`
- after: `eb562952409f72c9ccaea6d3f7f2ec83211439516ef34e724edded75fe9a3fb2`

| 小命令场景 | 基线 ns/op | 当前 ns/op | 基线 B/op | 当前 B/op | 基线 allocs/op | 当前 allocs/op |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| success | 2191 | 1942 | 4299 | 4251 | 32 | 30 |
| invalid_flag | 1807 | 1783 | 2850 | 2850 | 30 | 30 |
| missing_required | 1087 | 1162 | 2529 | 2529 | 22 | 23 |
| invalid_positionals | 1233 | 1252 | 2585 | 2585 | 25 | 25 |
| invalid_parameters | 1774 | 1552 | 3274 | 3226 | 25 | 23 |

成功路径中位耗时下降约 11.4%，分配 32→30；required 失败路径增加约 75 ns 和 1 次分配。分类保留检查与失败回调有成本，不能把成功路径收益表述为所有错误路径都更快。上述小场景是同机样本中位数，不是跨平台性能保证。

### Find 的 legacyArgs 补充

收尾时继续枚举 Cobra 的校验返回点，发现 `Find` 会在 Args 未声明时提前执行 `legacyArgs`。独立准备的裸根传入未知子命令仍返回 untyped 错误；补充复现先失败。现在 ExecuteC 的 Find 失败分支也交给同一 Args 回调，Traverse 的 flag 错误继续走 FlagErrorFunc。新增依赖回归覆盖 owner、回调一次、nil fallback 和失败不执行，corecmd 门禁覆盖实际 typed/3。

前一版 `aace4ff6` 的全仓库测试已通过（app 416.722 s、cli 154.437 s、helpers 127.707 s、test/scripts 352.444 s），[对应 Linux CI](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/actions/runs/34005743046) 也已全部通过（31 产品、1,357 工具）。legacyArgs 补充仅改 Find 失败分支，性能样本中的构造与五个执行场景代码不受影响；最终增量与全量验证结果以 PR 最终 head 记录为准。
