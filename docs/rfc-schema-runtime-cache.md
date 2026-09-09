# RFC：单入口、完整命令树与 Verified Schema Cache

| 字段 | 值 |
|---|---|
| 状态 | Draft；设计已冻结，代码与两平台验收进行中 |
| 日期 | 2026-09-07 |
| PR | #1296 |
| 固定基线 | `main` at `6f71222b9b07c760cdb5f376b24dab9155e62094` |
| 范围 | CLI 入口、完整命令树、Schema cache、telemetry 退出策略、发布与性能验收 |

性能数字和竞品结果放在[性能附件](rfc-schema-runtime-cache-performance.md)。命令框架的实施顺序见[专项计划](plan-command-framework-performance.md)。

## 1. 设计决策

### 1.1 一个二进制、一个进程、一棵完整树

正式产品只有一个 `dws`。所有通过制品完整性预检的公开调用，包括 root help、version、Schema、utility、业务命令和 completion，都构造同一棵完整 Cobra 树，再由 Cobra 解析和分派。

明确禁止：

- `dws-launcher → dws-core` 双二进制、逐次 core SHA-256 和第二套运行时；
- 按 argv 选择产品 factory、utility-only tree 或“无法证明时回退完整树”的双模式；
- root help snapshot、RootHelpModel、独立 help projection 或由另一棵声明树渲染公开 help；
- 在 Cobra 前识别 Schema argv 并直接输出缓存结果；
- 独立的 `argv → handler`、Safety、auth 或 transport 路由。

root help 直接遍历刚构造完成的公开树。Schema cache 命中改变的是 `schema` handler 读取 typed catalog 的来源，不改变命令解析和执行路径。

`NewSchemaSourceRootCommand` 仍可作为声明审计与离线 catalog assembly 的完整 distribution tree。它不是进程入口，不处理用户 argv，也不构成第二套公开运行时。

### 1.2 参考 Lxxx 软件的完整树优化方式

Lxxx CLI v1.0.85 在生产 Build 中每次挂载 utility、service catalog 和 shortcuts，并由同一命令树处理 help 与业务命令。`Lxxx` 源码（匿名化，不挂公开链接）的本机暖构树约为 905 个 command、7 ms、10.3 MB/op、84.6k allocs/op。

DWS 采用相同的结构选择，并针对约 1,825 个 command 优化完整树：

- declaration 使用紧凑 typed metadata；相同形状的命令由通用 builder 构造；
- ContractFinal 在 builder 已完成规范化和深拷贝后转移所有权，避免重复复制完整合同；
- help/build-only 字段挂到 Cobra/ContractFinal 后不再被 RunE closure 捕获；
- Safety scanner、鉴权、网络 caller 和其他执行期对象在真正执行时初始化；
- 通用 flag builder 避免 pflag 在空默认值上的大块临时分配，同时保持 pflag 类型与解析行为；
- 插件、shortcut、aliases、validation、Safety 和输出仍一次性进入完整树。

任何优化若需要第二棵树、独立 projection、跨调用“不存在”缓存或按 argv 削减功能，必须先修改本 RFC；不得先合代码再补设计。

### 1.3 Schema 与业务执行分层

Schema 的唯一语义源是 declarations，经 `ResolveSchemaBuild` 生成 typed Meta/Registry。缓存是经过身份认证的构建衍生物，不拥有 handler、auth、Safety 或业务 transport。

- `dws schema ...` 先经过完整 Cobra 树，再由正常 schema handler 读取缓存；
- cache miss、禁用或用户缓存损坏时，从同一 declarations path 同步重建；
- 普通业务命令不读取 Schema cache，也不为 Schema 查询组装 catalog；
- root help/version 不创建 Schema cache 文件；
- 缓存命中和 live assembly 的 wire output 必须逐字节等价。

### 1.4 Telemetry 不阻塞退出

命令完成事件仍进入官方 SDK 的异步队列。`NoFlushWait` 使主进程不等待最后一次网络发送；后台 `Close` 不改变命令结果。

这是产品合同：接受进程退出时最后一条分析事件可能丢失。本 RFC 不承诺 at-least-once。统一结果提交、output sink 关闭、stdio child 停止、audit drain、signal handler 卸载和 timing report 仍同步完成。

### 1.5 Prepare 只凭证据去重

根级 metadata validation、profile 参数规范化、PreParse、leaf validation、auth、Safety 和 cleanup 均保留。只有 profile/trace 证明同一 invocation 重复执行同一工作，且错误分类、输出和副作用测试等价时，才删除具体重复点。

## 2. Schema cache 合同

### 2.1 数据与身份

Meta 和按产品分片的 Registry 使用 deterministic protobuf。embedded identity 至少绑定 edition、declaration source digest、normalized surface digest、build ID、payload 长度及 SHA-256、固定 Go toolchain 和生成环境。

依赖边界：

- `internal/cli/schemaruntime`：typed decode，不依赖 Cobra/app/auth/network；
- `internal/schemacache`：有界认证 I/O 与原子发布；
- `internal/schemareader`：只读 embedded identity；
- `internal/cli`：repair、process memoization 和 handler delivery；
- `internal/app`：注册完整声明源和公开 Schema command。

### 2.2 状态语义

| 状态 | 行为 |
|---|---|
| embedded identity 全空 | 该 build 未启用 cache，schema handler 走 live declarations |
| identity 部分存在、非法或矛盾 | fail-closed，退出 125，不读取用户 cache |
| identity 完整，cache 缺失 | 同步重建并原子发布 |
| identity 完整，cache 认证通过 | handler 读取所需 Meta 或产品 shard |
| 用户 cache 截断、摘要不符或 protobuf 非法 | 丢弃结果并从 declarations 自愈，不输出部分结果 |
| live build 失败 | 返回原有分类错误，不发布新 cache |

用户可写 cache 损坏是可恢复状态；embedded identity 损坏是制品错误。

## 3. 构建与发布

候选和正式包调用同一 identity 验证与 linker flag 生成逻辑。首次包含本 RFC 的官方 prerelease/stable 必须：

1. 在 Darwin/arm64 与 Linux/amd64 原生 runner 上，用 Go 1.25.9、CGO 关闭、空 GOFLAGS/GOEXPERIMENT、GOWORK=off 各生成两次 identity；同机重复必须一致。
2. 两个 native target 的 identity bytes 不一致时停止发布。
3. GoReleaser 生成原有单二进制归档；支持 target 用 verified identity 重建同一个 `dws`，注入 payload、签名并重打包。
4. proof 缺失、release commit 不是完整精确 SHA、linker contract 非法或最终 binary 不含 build ID 时 fail-closed。
5. 最终归档继续经过 checksum、签名、安装器、npm、Homebrew 和 smoke 验证。

当前 cache payload 支持矩阵为 `open × {darwin/arm64, linux/amd64}`。其他 target 仍发布单个 `dws`，identity 为空并由正常 schema handler 走 live path。

## 4. 性能验收

### 4.1 测量规则

候选、固定 main 和专项父提交使用同机、同 target、同 Go 1.25.9 构建。报告绑定源码 SHA、binary SHA-256、toolchain、argv、环境、stdout/stderr digest 和原始样本。每个端到端场景至少 30 次随机交错；接近门槛时扩到 100 次。

### 4.2 必过项

| 维度 | 门槛 |
|---|---|
| 单树结构 | help/version/schema/config/业务/completion 的进程 root 均包含完整公开产品；不存在 argv 产品路由、Schema 前置执行或 help projection |
| 完整构树 | 相对本专项父提交，warm `B/op` 至少降低 20%，`allocs/op` 至少降低 8%；ns/op 不得超过 `max(parent ×105%, parent + 1 ms)` |
| help/version | candidate p50 ≤ `max(main ×105%, main + 3 ms)`；p95 ≤ `max(main ×110%, main + 3 ms)` |
| root help RSS | native p50 ≤50 MiB、p95 ≤55 MiB，且相对固定 main 的 p50/p95 不回退；同机 Lxxx 的绝对值和按 command 归一化结果必须进入报告，但因公开节点数不同不作为 release gate |
| Schema cache | warm leaf 相对 live assembly 的 user CPU p50 至少降低 80%；有效样本 peak RSS ≤100 MiB |
| 业务命令 | dry-run、mock/get、config 的 p50/p95 相对固定 main 不回退 |
| 正确性 | help bytes、flags、aliases、validation、Safety、Schema wire、输出和错误分类不变 |
| 清理 | telemetry 不等待网络；业务 cleanup、signal 和退出码测试通过 |

旧的 `launcher ≤ core +5%`、calendar ≤ full-tree 70%、config ≤ full-tree 10% 和 full/selective equivalence gate 全部废止，不得与本口径并存。

### 4.3 竞品边界

Lxxx 软件的完整构树只用于结构和单位节点资源参考；Gxx 软件用于观察更小程序映像/init 的上限。DWS 约 1,825 个公开节点，Lxxx 约 905 个，命令面与输出合同也不同，因此竞品绝对 RSS 不替代 DWS 的固定预算和 main 回归门禁。报告必须同时列出节点数、B/op、allocs/op 和端到端 RSS，不能只比较 wall time。

## 5. 非目标

- 不在本 RFC 内裁剪 open edition 的产品面。
- 不以 daemon、常驻进程或语言重写追求 Gxx 软件 3～4 ms。
- 不提交生成 Catalog，也不让 cache 成为声明源。
- 不为 completion 增加尚不存在的 callback 门闩。
- 不凭代码阅读删除 auth、Safety、Prepare 或业务 cleanup。

## 6. 代码对齐清单

- [x] launcher/core/package manifest 撤回，发布恢复单 `dws`。
- [x] argv 产品选择、route index、插件资格分流和 selective-tree tests 删除。
- [x] Schema 的 Cobra 前置 fast path 删除；cache 由正常 schema handler 使用。
- [x] root help snapshot/model package 删除；公开 help 直接遍历完整 runtime tree。
- [x] ContractFinal ownership transfer、build-only closure 清理、lazy Safety scanner 和低分配 string-slice builder 落地。
- [x] 测试钉住 process invocation 总是包含完整产品面。
- [x] telemetry no-wait 明确接受末条事件丢失，并有阻塞 collector 测试。
- [x] 两平台完整测试与 race 通过（head `a8376f92`，run `34080469082`）。
- [x] 两平台固定 main 性能矩阵通过；root help p50/p95 均低于 50/55 MiB，且相对 main 降低。
- [ ] 首次正式 release 的签名、最终制品与安装验证；阻挡正式发布。

## 7. 回滚

完整树优化按独立提交回滚，不改变 Schema/Safety 的权威源。cache 用 identity/build ID 隔离，旧二进制无法命中时按自身规则重建。若 telemetry 丢失率不可接受，可回滚 `NoFlushWait` 并恢复退出等待；可靠且不阻塞的投递需要另立持久 outbox RFC。
