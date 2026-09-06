# RFC：单入口 CLI、Schema Runtime Cache 与按需命令装配

| 字段 | 值 |
|---|---|
| 状态 | Draft；设计已冻结，代码与两平台性能验收进行中 |
| 日期 | 2026-09-06 |
| PR | #1296 |
| 基准 | `main` at `6f71222b9b07c760cdb5f376b24dab9155e62094` |
| 范围 | CLI 入口、Schema cache、命令启动装配、telemetry 退出策略、发布注入与性能验收 |

性能数字、原始样本和竞品结果放在[性能附件](rfc-schema-runtime-cache-performance.md)。本文只规定设计选择、行为合同和验收。

## 1. 决策

### 1.1 保持单二进制产品形态

正式入口继续是一个 `dws` 可执行文件。PR #1296 新增的 `dws-launcher → dws-core` 双二进制、逐次 core SHA-256、canonical package manifest 和 launcher 专用 help snapshot 全部撤回，不进入默认产品形态。

所有命令最终仍由现有 Cobra、pipeline、corecmd validation、Safety、auth、handler、统一输出和清理生命周期执行。Schema 快速读取和按 argv 缩小装配范围属于同一进程入口的准备阶段，不是第二套命令执行器。

发布归档保持原有扁平结构：每个平台包含一个 `dws`（Windows 为 `dws.exe`）及既有元数据。npm、Homebrew、安装、升级和回滚继续操作这一个文件。

### 1.2 Schema 与业务命令分开

Schema 的唯一语义源仍是 declarations，经 `ResolveSchemaBuild` 生成 typed Meta/Registry。缓存只是经过身份认证的构建衍生物，不拥有 handler、auth、Safety 或业务 transport。

- `dws schema ...` 在支持且命中的情况下，于构造业务命令树前读取缓存。
- 缓存 miss、用户缓存损坏或禁用时，从 declarations 同步重建并返回同一 wire output。
- 普通 `list/get/create/...` 不为查询 Schema 而构建无关产品树，也不把 Schema cache 当 handler registry。
- root help、version 和普通业务命令不创建 Schema cache 文件。

### 1.3 统一框架按需装配产品

真实进程入口先读取根级 flag，再选择需要的顶层产品 factory。Cobra 仍执行最终解析与校验。

| 输入状态 | 装配行为 |
|---|---|
| 已知 utility，例如 `config`、`version` | 只装配 utility |
| 已知产品或顶层 alias，例如 `calendar`、`im` | 只构建对应产品及其 shortcut |
| exact root help、空 argv | 构建完整树，保持导航面 |
| 未知 flag、`--`、未知命令 | 构建完整树，保持 Cobra 错误、建议和兼容行为 |
| edition 增加命令，或存在安装/开发插件 | 构建完整树，让扩展和 PreParse 保持原语义 |

按需路径只从现有 factory registry 选择构造范围。不得维护独立的 `argv → handler` 表，不得提前解析 leaf flags，不得绕过全局 flag、aliases、PreParse、required/group validation、auth 或 Safety。以后若加入影响顶层路由的能力，必须同时更新 registry、保守回退和等价性测试。

可复用 API `NewRootCommand` / `NewRootCommandWithEngine` 始终返回完整树；按需装配只用于真正的进程入口，避免测试、嵌入方和多次构造调用观察到依赖 `os.Args` 的不完整树。

### 1.4 Telemetry 不阻塞退出

命令完成事件仍使用现有官方 SDK、现有字段和异步队列。DWS 配置 `NoFlushWait` 后，事件入队即返回，后台启动 `Close`，主进程不等待最后一次网络发送。

这是显式的可靠性取舍：**接受进程退出时最后一条事件可能丢失**。本 RFC 不宣称 at-least-once 投递，也不把发送成功当命令成功条件。若未来要求可靠投递，应另行设计持久 outbox；不得重新在交互命令退出路径等待网络。

不等待 telemetry 只发生在 `app.ExecuteWithTelemetry` 返回之后。以下业务清理仍同步完成：统一结果提交和输出 sink 关闭、stdio 子进程停止、audit sink drain、signal handler 卸载及 timing report 写入。

### 1.5 Prepare/config/profile 只凭证据去重

根级 metadata validation、profile 参数规范化、pipeline PreParse、leaf validation、auth 和 Safety 均保留。只在 profile 或 trace 证明同一 invocation 重复读取或重复 Prepare 后，才删除具体重复点，并附带输出、错误分类和副作用等价测试。本 RFC 不以推测为由合并这些生命周期。

## 2. Schema 缓存合同

### 2.1 数据与身份

Meta 和按产品分片的 Registry 使用 deterministic protobuf。embedded identity 至少绑定：

- edition；
- declaration source digest；
- normalized surface digest；
- build ID；
- Meta/Registry payload 长度及 SHA-256；
- 固定 Go toolchain 和生成环境。

`internal/schemareader` 解析 embedded identity，`internal/schemacache` 负责有界读取、认证和原子发布，`internal/cli/schemaruntime` 负责纯 typed decode。依赖方向不得从 reader/cache 反向引入 Cobra、app、auth 或网络。

### 2.2 命中、失效和错误

| 状态 | 行为 |
|---|---|
| embedded identity 全空 | 该 build 未启用快路径，走 declarations live path |
| embedded identity 部分存在、格式非法或相互矛盾 | fail-closed，退出 125，不读取缓存 |
| identity 完整，缓存缺失 | 同步从 declarations 重建，原子发布后返回 |
| identity 完整，缓存认证通过 | 读取需要的 Meta 或产品 shard |
| 用户缓存截断、摘要不符、protobuf 非法或版本不兼容 | 丢弃该缓存结果，从 declarations 自愈；不得输出部分结果 |
| declaration/live 构建失败 | 返回原有分类错误，不发布新缓存 |

用户可写 cache 的损坏属于可恢复状态；embedded identity 的损坏属于制品错误。两者不能共享“静默回退”语义。

## 3. 构建与发布

候选和正式包调用同一份 identity 验证与 linker flag 生成逻辑。首次包含本 RFC 的官方 prerelease/stable 必须：

1. 在 Darwin/arm64 与 Linux/amd64 原生 runner 上，用 Go 1.25.9、CGO 关闭、空 GOFLAGS/GOEXPERIMENT、GOWORK=off 各生成两次 identity；同机重复结果必须一致。
2. 比较两个原生 target 的 identity bytes；不一致则停止发布。
3. GoReleaser 先生成扁平单二进制归档；post-processing 对已支持的 Darwin/arm64、Linux/amd64 目标使用 verified identity 重建同一个 `dws`，再注入 runtime payload、签名并重打包。
4. proof 缺失、release commit 非完整精确 SHA、linker contract 非法或最终二进制不含 build ID 时 fail-closed。
5. 对最终归档继续执行既有 checksum、签名、安装器、npm、Homebrew 和 smoke 验证。

当前快路径支持矩阵是 `open × {darwin/arm64, linux/amd64}`。其他 target 仍发布单二进制，但 embedded identity 为空并走 live path。扩大矩阵必须先补对应 native reproducibility proof；不得用交叉构建结果冒充原生一致性证明。

本设计不把最终可执行文件 SHA 嵌入自身，也不增加运行时自哈希。发布 checksum、可信安装源和平台签名继续负责交付完整性；embedded Schema identity 只证明该二进制接受的 cache 内容。

## 4. 性能验收

### 4.1 固定基线

所有 PR 净收益以同机、同 target、同 Go 1.25.9 构建的固定 `main` commit `6f71222b9b07c760cdb5f376b24dab9155e62094` 为基线。候选和基线都使用最终 runtime payload；报告绑定源码 commit、binary SHA-256、toolchain、argv、环境、stdout/stderr digest 和原始样本。

旧的 “launcher ≤ 同包 core +5%” gate 废止，因为产品形态已恢复为一个入口。历史报告保留原布尔值，但不得用于当前 Ready 结论。

### 4.2 必过项

| 维度 | 门槛 |
|---|---|
| help/version 默认与 opt-out | candidate p50、p95 均 ≤ 固定 main 的 105% |
| Schema cache | warm leaf 相对 live declaration assembly 的 user CPU p50 至少降低 80%；所有有效样本 peak RSS ≤100 MiB |
| 业务命令 | `calendar ... list --dry-run`、常用 get、config 读取各自 default p50/p95 相对固定 main 不回退；至少一个代表业务命令的 p50 降低 40% |
| 装配贡献 | process `calendar list` 的构树时间与 allocations 均 ≤完整树的 70%；utility `config get` 均 ≤完整树的 10% |
| 正确性 | full/selective tree 的目标 command identity、flags、aliases、validation、Safety、输出和错误分类一致；插件/未知输入回退完整树 |
| 清理 | telemetry 不等待网络；业务清理、信号和退出码测试全部通过 |

### 4.3 归因与竞品

报告必须分别给出：

- 入口贡献：单二进制与 telemetry no-wait；
- core 贡献：Schema 命中、按需产品装配、已证实的重复 I/O 删除；
- 整体：wall p50/p95、user/system CPU、peak RSS、allocations；
- Lark CLI 1.0.85 与 GWS 0.22.5 的同机相近场景结果。

Lark/GWS 对比用于诊断产品差距，不作为本 RFC 的 release gate，因为命令面、输出和鉴权合同不完全相同。不得把 opt-out 结果当默认产品表现，也不得把 Node wrapper 或父子进程排除在 public RSS 外。

## 5. 非目标

- 不将业务 leaf、auth、transport 或 Safety 复制到独立 launcher。
- 不为 root help 维护构建期 snapshot；帮助来自同一 Cobra/declaration tree。
- 不承诺所有 Schema 查询都无 live fallback，也不提交生成 Catalog。
- 不以竞品延迟替代本项目的固定 main 回归门禁。
- 不在本 RFC 中引入常驻 daemon、持久 telemetry outbox 或语言重写。
- 不凭代码阅读删除 Prepare、config/profile 读取或业务清理。

## 6. 代码对齐清单

- [x] 默认二进制名恢复为 `dws`，launcher/core/package manifest 实现撤回。
- [x] npm、Homebrew、安装、升级、回滚恢复单文件布局。
- [x] Schema candidate builder、verifier和测量脚本改为单二进制绑定。
- [x] 正式 release workflow 要求两原生 target 的 byte-identical identity proof。
- [x] process entry 可按 utility/产品选择 factory；公开 root constructor 保持完整。
- [x] 插件、edition hook、未知输入和 root help 回退完整树。
- [x] telemetry no-wait 明确接受末条事件丢失，并有阻塞 collector 测试。
- [ ] 两平台完整测试与 race 通过；阻挡 Ready。
- [ ] 两平台固定 main 性能矩阵通过；阻挡 Ready。
- [ ] Lark/GWS 新 head 诊断附件生成；不阻挡 Ready，但阻挡“已经追齐竞品”的表述。
- [ ] 首次正式 release 的签名、最终制品与安装验证；阻挡正式发布，不阻挡 PR Ready。

## 7. 回滚

代码回滚恢复上一版单 `dws`。新 cache 以 identity/build ID 隔离；旧二进制命中不了就按自身规则重建，无需手工迁移。若 telemetry 丢失率不可接受，回滚 `NoFlushWait` 会恢复最多 300 ms 的退出等待；可靠且无前台等待需要后续持久 outbox 设计，不能在本 PR 中静默改变保证。
