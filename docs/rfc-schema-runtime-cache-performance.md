# RFC 附件：CLI 性能提升报告

状态：Draft，2026-09-07 单树架构重置后的本地阶段报告。设计见[主 RFC](rfc-schema-runtime-cache.md)。

## 1. 当前结论

PR 已撤回 selective tree、Schema 前置执行和 root help projection。旧 head 中 calendar 0.85 ms、config 0.36 ms、root help 比 Lark 慢 2.5% 等数字描述的是已删除结构，全部标记为历史结果，不能用于当前 Ready 结论。

当前可确认的是完整构树优化：在同机、同 toolchain、同一父子提交对照下，DWS 始终构造约 1,825 个 command，B/op 降低 **22.8%**，allocs/op 降低 **9.8%**，ns/op 变化为 **+0.3%**。这说明不需要为 root help 建第二份 projection，也能显著压低完整树的内存分配，同时没有可辨认的构树延迟回退。

端到端 Schema、help、业务命令、RSS 和 Lark/GWS 表必须在 clean commit 的 Darwin/arm64、Linux/amd64 CI 上重跑。本附件在 CI 前不宣称已解决 root help RSS。

## 2. 方法与版本

本地完整树 microbenchmark：

- CPU/OS：Apple M3 Pro，Darwin/arm64；
- Go：1.26.1，本地工具链；正式 CI 固定 Go 1.25.9；
- benchmark：`BenchmarkNewRootCommand`，每轮 10 次，共 3 轮，取三轮中位数；
- 父提交：`c0131d35`；candidate implementation commit：`34ce7493`；
- 两边均构造完整公开 Cobra tree，不执行 handler；
- 端到端 macOS wall time 受本机安全扫描影响，本轮不采用。

Lark 参考固定在 v1.0.85 源码 SHA `13305ae51b62833c6cd07368be48ae523bb491df`：

- [生产 Build 挂载完整 utility/service/shortcut tree](https://github.com/larksuite/cli/blob/13305ae51b62833c6cd07368be48ae523bb491df/cmd/build.go#L273-L294)
- [completion 只按 invocation 开启 callback 注册](https://github.com/larksuite/cli/blob/13305ae51b62833c6cd07368be48ae523bb491df/internal/cmdutil/completion.go#L12-L37)
- [v1.0.85 release 使用 Go 1.23](https://github.com/larksuite/cli/blob/13305ae51b62833c6cd07368be48ae523bb491df/.github/workflows/release.yml#L98-L105)

## 3. 完整命令树

| 实现 | command 数 | ns/op | B/op | allocs/op |
|---|---:|---:|---:|---:|
| DWS 父提交 `c0131d35` | ≈1,825 | 13,503,654 | 16,431,941 | 164,227 |
| DWS implementation commit `34ce7493` | ≈1,825 | 13,544,096 | 12,681,568 | 148,166 |
| DWS 变化 | — | **+0.3%** | **-22.8%** | **-9.8%** |
| Lark v1.0.85 本机参考 | ≈905 | ≈7,000,000 | ≈10,300,000 | ≈84,600 |

DWS 当前完整树总 B/op 约为 Lark 的 1.23 倍、allocs/op 约 1.75 倍，而 command 数约 2.02 倍。粗略每节点为：

| 实现 | KiB / command | alloc / command |
|---|---:|---:|
| DWS 当前 | ≈6.95 | ≈81.2 |
| Lark 参考 | ≈11.4 | ≈93.5 |

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

当前实现的验收将分开报告：

| 路径 | 必须包含 | 待补指标 |
|---|---|---|
| cache hit | 完整 Cobra tree + normal schema handler + authenticated shard read | wall/CPU/RSS p50/p95 |
| cache miss/disabled | 完整 Cobra tree + declarations assembly | wall/CPU/RSS p50/p95 |
| repair | 完整 Cobra tree + corrupt cache rejection + rebuild + atomic publish | wire parity、wall/RSS |

门槛仍为 warm hit 相对 live assembly 的 user CPU p50 至少降低 80%，peak RSS ≤100 MiB。

## 5. Help 与 version

root help 与 version 现在都构造完整 tree。帮助直接从同一 runtime tree 渲染，不读取 Schema cache、不执行业务 auth/RPC，也不创建中间 projection。

需要在新 head 重测：

- candidate 相对固定 main 的 default/opt-out p50/p95；
- candidate 相对专项父提交的 native RSS；
- DWS 与 Lark native root help 的 p50/p95 RSS；
- help stdout 的逐字节 oracle；
- telemetry default 与 opt-out 的差值。

旧 single/selective head 的 DWS 46.37 MiB、Lark 45.20 MiB 只作为问题来源，不能作为当前结果。RFC 要求新 head RSS 相对父提交不回退，并将超过 Lark 5% 作为性能专项阻断。

## 6. 业务命令

calendar list/get、dry-run、mock、config 和 event utility 均承担同一完整构树成本。专项不再用“无关产品 factory 调用为零”证明收益，而关注：

- 完整构树的统一 B/op、allocs/op 和 retained heap；
- PreParse、validation、auth、Safety、handler 与 cleanup 分段；
- default telemetry 不等待网络；
- dry-run 不新增真实写 RPC；
- 输出、错误分类和 side-effect count 与固定 main 一致。

当前只有构树 microbenchmark；新 head 的端到端业务表待两平台 CI 回填。

## 7. 整体运行内存

预计 root help、version 和短业务命令都会受益于完整树每次约 3.75 MB 的临时分配下降，但 B/op 不能直接等同 peak RSS。正式报告需要 wait4/native RSS 和 public process-tree RSS，各场景独立采样。

当前本地新 binary 的少量非正式样本约 44.7～45.1 MiB，接近先前 Lark 的 45.2 MiB；本机安全软件会延迟或终止新 binary，因此该观察不进入验收。

## 8. Lark 与 GWS

Lark 证明了“每次完整构树”本身不是 root help projection 的理由。它的优势来自较小的命令面、紧凑 typed metadata、统一 service builder，以及执行期工作不进入 Build。

GWS 0.22.5 的历史同机 native 启动约 3～4 ms、RSS 约 8～11 MiB。其优势主要在更小的 executable/init/runtime 与命令面。本 RFC 不通过减少 DWS 产品面、增加第二 runtime 或 daemon 追这个区间。

clean-head 竞品表将包含 Schema、root help、version、leaf help、dry-run，并同时列 wall、CPU、RSS、node count、B/op 和 allocs/op。Lark/GWS 是诊断；固定 main 回归仍是产品 release gate。

## 9. 当前验收状态

- [x] 单二进制、单进程、完整 runtime tree 结构落地。
- [x] Schema 前置执行与 root help projection 删除。
- [x] 本地完整树 B/op -22.8%、allocs/op -9.8%，达到 RFC 门槛。
- [x] 定向 corecmd/app/help/Schema dependency tests 通过。
- [ ] clean commit Go 1.25.9 microbenchmark。
- [ ] Darwin/arm64 native full/race/performance artifact。
- [ ] Linux/amd64 native full/race/performance artifact。
- [ ] root help RSS 与 Lark 的新 head 对比。
- [ ] 正式 release 最终签名制品与安装验证。
