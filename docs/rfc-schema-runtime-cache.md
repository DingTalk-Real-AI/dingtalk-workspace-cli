# RFC：单入口、完整命令树与 Verified Schema Cache

| 字段 | 值 |
|---|---|
| 状态 | Draft；设计已冻结，代码与两平台验收进行中 |
| 日期 | 2026-09-09 |
| PR | #1296 |
| 固定基线 | `main` at `6f71222b9b07c760cdb5f376b24dab9155e62094` |
| 范围 | CLI 入口、完整命令树、Schema cache、telemetry 退出策略、发布与性能验收 |

本文是 PR #1296 的**唯一规范性文档**。设计、实施阶段、性能数字、编译期交付讨论与进展均收束于此。下列路径仅为指向本文的短指针，不再维护独立正文：

- [性能附件指针](rfc-schema-runtime-cache-performance.md)
- [编译期 Schema 交付指针](rfc-compiled-schema-delivery.md)
- [CLI 性能专项计划指针](plan-cli-performance-parity.md)
- [CLI 性能专项技术方案指针](design-cli-performance-parity.md)
- [完整命令树专项计划指针](plan-command-framework-performance.md)
- [性能优化进展指针](performance-optimization-progress.md)

**当前产品事实：**

- 编译期 / 发布期**不**生产、不嵌入 Schema identity（无 compile-time identity seal）。
- 受支持端在安装或首次 `dws schema` 从本机 live declarations **生成本机 identity**，写入认证磁盘 cache；miss/损坏则 live assembly 并修复发布。
- CLI 默认 `FlushTimeout=50ms`，不设置 `NoFlushWait`。

## 1. 设计决策

### 1.1 一个二进制、一个进程、一棵完整树

正式产品只有一个 `dws`。所有通过制品完整性预检的公开调用，包括 root help、version、Schema、utility、业务命令和 completion，都构造同一棵完整 Cobra 树，再由 Cobra 解析和分派。

```mermaid
flowchart LR
    W[可选 npm wrapper] --> D[单一 dws 进程]
    D --> I[metadata preflight]
    I --> T[构造完整 Cobra runtime tree]
    T --> C[Cobra parse / Find]
    C --> P[统一 PreParse / validation / auth / Safety]
    P --> H{normal handler}
    H -->|schema| S[本机 identity + 认证 cache，miss 则 live assembly]
    H -->|utility / business| B[既有 handler / transport]
    S --> O[统一 output / cleanup]
    B --> O
    O --> Q[telemetry enqueue，FlushTimeout 50ms]
```

root help 直接遍历刚构造完成的公开树 `T`。Schema cache 命中改变的是 `schema` handler 读取 typed catalog 的来源，不改变命令解析和执行路径。

明确禁止：

- `dws-launcher → dws-core` 双二进制、逐次 core SHA-256 和第二套运行时；
- 按 argv 选择产品 factory、utility-only tree 或“无法证明时回退完整树”的双模式；
- root help snapshot、RootHelpModel、独立 help projection 或由另一棵声明树渲染公开 help；
- 在 Cobra 前识别 Schema argv 并直接输出缓存结果；
- 独立的 `argv → handler`、Safety、auth 或 transport 路由。

`NewSchemaSourceRootCommand` 仍可作为声明审计与离线 catalog assembly 的完整 distribution tree。它不是进程入口，不处理用户 argv，也不构成第二套公开运行时。

### 1.2 参考 Lxxx 软件的完整树优化方式

Lxxx CLI v1.0.85 在生产 Build 中每次挂载 utility、service catalog 和 shortcuts，并由同一命令树处理 help 与业务命令。`Lxxx` 源码（匿名化，不挂公开链接）的本机暖构树约为 905 个 command、7 ms、10.3 MB/op、84.6k allocs/op。completion 只按 invocation 开启 callback 注册，不改变命令树来源。

DWS 采用相同的结构选择，并针对约 1,825 个 command 优化完整树：

- declaration 使用紧凑 typed metadata；相同形状的命令由通用 builder 构造；
- ContractFinal 在 builder 已完成规范化和深拷贝后转移所有权，避免重复复制完整合同；读取侧继续 defensive clone；
- help/build-only 字段挂到 Cobra/ContractFinal 后不再被 RunE closure 捕获；框架直接提取私有 `executionSpec`，替代捕获已清零的大型 `Spec`；
- Safety scanner、鉴权、网络 caller 和其他执行期对象在真正执行时初始化；
- 通用 flag builder 避免 pflag 在空默认值上的大块临时分配，同时保持 pflag 类型与解析行为；
- shortcut 装配通过 declaration pointer 构造，避免构树期间复制大型声明；
- 插件、shortcut、aliases、validation、Safety 和输出仍一次性进入完整树。

任何优化若需要第二棵树、独立 projection、跨调用“不存在”缓存或按 argv 削减功能，必须先修改本 RFC；不得先合代码再补设计。

### 1.3 Schema 与业务执行分层

Schema 的唯一语义源是 declarations，经 `ResolveSchemaBuild` 生成 typed Meta/Registry。缓存是经过身份认证的构建衍生物，不拥有 handler、auth、Safety 或业务 transport。

- `dws schema ...` 先经过完整 Cobra 树，再由正常 schema handler 读取缓存；
- cache miss、禁用或用户缓存损坏时，从同一 declarations path 同步重建；
- 普通业务命令不读取 Schema cache，也不为 Schema 查询组装 catalog；
- root help/version 不创建 Schema cache 文件；
- 缓存命中和 live assembly 的 wire output 必须逐字节等价。

### 1.4 Telemetry 退出等待：`FlushTimeout=50ms`

命令完成事件仍进入官方 SDK 的异步队列。CLI 默认设置 `FlushTimeout=50ms`，且不设置 `NoFlushWait`，因此主进程在退出前最多等待 50ms 做 best-effort flush。这是相对 SDK 默认约 300ms 与 `NoFlushWait` 的时延/投递折中。超时即放弃，不会无限挂起。`NoFlushWait` 仍保留在 SDK `Config` 上，供明确接受末条事件丢失的可选接入使用；本 CLI 默认路径不启用。

本 RFC 不承诺 at-least-once。统一结果提交、output sink 关闭、stdio child 停止、audit drain、signal handler 卸载和 timing report 仍同步完成。

### 1.5 Prepare 只凭证据去重

根级 metadata validation、profile 参数规范化、PreParse、leaf validation、auth、Safety 和 cleanup 均保留。只有 profile/trace 证明同一 invocation 重复执行同一工作，且错误分类、输出和副作用测试等价时，才删除具体重复点。

## 2. Schema cache 合同

### 2.1 数据与身份

Meta 和按产品分片的 Registry 使用 deterministic protobuf。**编译期 / 发布期不生产、不嵌入 Schema identity**；发运二进制不钉 ldflags digest。每个受支持端（darwin/linux/windows 的 amd64/arm64）在安装或首次 `dws schema` 时，从本机二进制的 live declarations 生成 identity，写入认证磁盘 cache；后续命中先校验摘要再读 protobuf shards。空/缺失本地 identity 表示 generate then use，不是永久 live-only。测试仍可注入完整 identity。

依赖边界：

- `internal/cli/schemaruntime`：typed decode，不依赖 Cobra/app/auth/network；
- `internal/schemacache`：有界认证 I/O 与原子发布；
- `internal/schemareader`：identity 解析，供本地 sidecar 与测试注入；
- `internal/cli`：本地 identity 生成、repair、process memoization 和 handler delivery；
- `internal/app`：注册完整声明源和公开 Schema command；生产注册 **local generate** cache，不注册 compile-time cache identity。

当前磁盘形态还包括（DTO v5）：

- payload 分片携带按 canonical 路径寻址的预渲染 compact 叶子；`schema <leaf> --compact -f json` 快路径只做小 range 读，不打开 registry 分片；
- Meta 的 `command_entries` 按产品拆为 `command_entry_shards`；`CommandMeta(path)` 经 locator 定位产品后只解码该产品分片；
- root 构树时可 `PrewarmSchemaCache`（`WithNoCreate` 只读探测），与 Cobra 建树重叠；进程级共用单一 payload 句柄。

别名、分组、产品、非 compact 查询仍走 registry 路径（别名渲染会改 `cli_path`/`is_alias`，不能用 canonical 字节）。

### 2.2 状态语义

| 状态 | 行为 |
|---|---|
| 生产，本地 identity 缺失 | 首次 schema 路径从 live declarations 生成 identity，原子发布 shards 与 sidecar |
| 生产，本地 identity 命中且认证通过 | handler 读取所需 Meta 或产品 shard |
| 生产，cache 缺失/损坏 | 同步重建并原子发布（与本机 identity 对齐） |
| 测试注入完整 identity，cache 缺失 | 同步重建并原子发布 |
| 测试注入完整 identity，cache 认证通过 | handler 读取所需 Meta 或产品 shard |
| 用户 cache 截断、摘要不符或 protobuf 非法 | 丢弃结果并从 declarations 自愈，不输出部分结果 |
| live build 失败 | 返回原有分类错误，不发布新 cache |
| 插件等改变命令面 | 本进程禁用持久 cache I/O |

不发明新的未认证加密方案。每个 edition cache 目录只写一份稳定 sidecar `identity.json`；**不以二进制 fingerprint 作为缓存维度**（不再使用 `identity.<fingerprint>.json` 作为主键或查找键）。Sidecar 内的 source/surface/build_id 与 shard 摘要即代际身份。升级失效靠：安装器在预热前清除旧 sidecar、Publish 的 ExpectedIdentity digest/auth，以及 live declarations 与 sidecar 哈希不一致时重新生成。遗留的 `identity.*.json` 只忽略或清理，不参与查找。macOS `/Library/Caches` 的 sticky 祖先目录被接受；装不上共享 cache 时回退到用户 cache，安装器不得在未写出文件时宣称共享 cache 成功。运行时不得创建系统共享 cache 目录。

Windows（amd64/arm64）使用同一套 envelope / Publish / OpenRegistry / OpenPayloads / ReadMeta 合同与本机 identity，不引入 compile-time seal：

- 用户 cache：`os.UserCacheDir()`，即 `%LOCALAPPDATA%\dws\schema\<edition-sha256>\v1`。
- 可选共享 cache：`%ProgramData%\dws`（再拼 `dws\schema\<edition-sha256>\v1`），仅安装器创建；运行时只读探测，缺失或不安全（不可信 owner / 普通用户可写 DACL）则回退用户 cache。
- 安装器/测试可用 `DWS_SCHEMA_CACHE_DIR` 覆盖基目录（按共享 cache 语义打开）。
- 安全近似：拒绝意外 reparse point；在打开的句柄上核验 owner+DACL（共享根、edition 目录、sidecar/shards/lock）。共享 ACL 允许 Builtin Users 读+遍历，Admins/SYSTEM（及安装者）保留写；个人 cache 仍为 owner+SYSTEM 的保护 DACL（`restrictOwnerWrite`）。安装器不得对不安全的 `%ProgramData%\dws` 盲目 `New-Item -Force`，应硬化或拒绝并回退 per-user cache。temp+rename 原子发布；读取仍用 ExpectedIdentity 与 SHA-256 pin 认证，篡改即 digest 失败。

其他 os/arch 仍按 build tag 编译掉，保持 live-only。

## 3. 构建与发布

官方 prerelease/stable **不得**在 runner 上生成 Schema identity，也不得用 ldflags / `DWS_SCHEMA_IDENTITY_PROOF` 把 identity 封进二进制：

1. 发布工作流不再运行 `schema-release-native-proof` / `compare-schema-release-proofs`。
2. GoReleaser 生成原有单二进制归档；`post-goreleaser.sh` 只做 runtime payload、签名和重打包，不再按 identity 重建 `dws`。
3. `go build ./cmd` 与正式包均不生成或嵌入 Schema identity。
4. 最终归档继续经过 checksum、签名、安装器、npm、Homebrew 和 smoke 验证。
5. 安装器在受支持端尝试预热 cache，且**未写出产物不得宣称成功**：
   - darwin/linux amd64/arm64：`install.sh` 写共享 cache（Linux `/var/cache/dws`，macOS `/Library/Caches/dws`）；预热前删除 `identity.json` 与遗留 `identity.*.json`，仅当 Meta/Registry/Payloads 与 `identity.json` 均已写出才宣称成功。遗留 fingerprint 文件不得算成功。
   - windows amd64/arm64：`install.ps1` 在二进制安装后调用 `Build-SharedSchemaCache`。经 `Initialize-SharedSchemaCacheRoot` / `Protect-SharedSchemaCacheTree` 硬化 `%ProgramData%\dws`（可用 `DWS_SCHEMA_CACHE_SHARED_DIR` 覆盖；Admins/SYSTEM 可写、Builtin Users 只读）；根目录不可信或不可用则预热 `%LOCALAPPDATA%` 下的用户 cache。同样在预热前清除旧 sidecar，只在 `identity.json` + 三个 shard 均存在且共享树 ACL 保护成功时宣称共享成功，否则警告首次 schema 命令会建 per-user cache。

所有公开 target 都发布单个 `dws`。Schema handler 优先走本机认证 cache，否则 live declaration assembly。

## 4. 完整树优化与实施阶段

目标是在始终构造完整公开 Cobra 树的前提下，降低 root help、version、Schema 和业务命令共同承担的构树内存与分配。P4 是后续专项，不阻挡 #1296 Ready。

### P0：删除双模式与第二投影（已完成）

- 删除 process argv route、产品/shortcut route index 和 selective-tree 构造器。
- process、public、help、version、config、业务与 completion 均构造完整产品面。
- 删除 Cobra 前的 Schema argv fast path；verified cache 留在正常 Schema command 内。
- 删除 root help model/snapshot package；renderer 直接遍历 runtime tree。
- 删除对应资格探测、回退矩阵、route benchmark 与 drift tests。
- 新增跨平台测试，对 help、Schema、config、业务 argv 检查完整产品面。

交付标准：代码中不存在“选择性树与完整树等价”问题，因为公开运行时只有完整树。

### P1：压缩 typed metadata 的所有权（已完成）

- ContractFinal payload 在 builder 完成规范化和必要深拷贝后，转移给 command-owned weak store，避免第二次完整 clone。
- RuntimeContractFinal 的读取仍返回 defensive deep copy。
- RunE closure 不再捕获已经写入 Cobra/ContractFinal 的 Use、help prose、Contract 和 PostMount 等 build-only 字段。
- 框架直接提取私有 `executionSpec`；所有 `corecmd.New` 调用自动受益。该结构只承载已有执行流水线，不参与命令注册、Schema 或 help 声明。
- 对 `ParamDecl.Enum`、`Required` 等嵌套引用增加调用者 mutation 隔离测试。

### P2：执行期对象延迟初始化（核心已完成）

- Safety content scanner 在首次真实 payload 检查时才编译规则，不在完整构树时初始化。
- disabled scanner 保持 nil。
- 后续可用 CPU/heap profile 继续审计 auth、transport、config 和 formatter；每个 lazy 变化必须有首次执行与并发测试。

交付标准：help/version 构树不创建网络连接、读取 token 或编译业务响应规则。

### P3：低分配通用 builder（核心已完成）

- string-slice flag 对空默认值不再为每个 flag 创建 encoding/csv 的 4 KiB writer；保持 pflag `stringSlice` 行为。
- shortcut builder 在注册表到 Cobra mount 之间传指针。
- helper registry 只保留 name + factory。
- 根据新 profile 逐项处理 annotations、result-schema normalization 和 pflag map；单项收益低于噪声或需要缓存第二份真相时不做。

### P4：向紧凑 service catalog 收敛（后续专项）

先在框架处理闭包、参数适配和规范化的重复分配。代表产品用于验证执行行为；只有 profile 证明剩余成本来自产品自身声明，且框架不能统一消除时，才启动产品迁移。

- 统计手写 helper/shortcut builder 中重复的 Cobra 对象、flag spec 和 annotation 形状。
- 后续迁移一个代表产品到只读 typed service descriptor + 通用 builder；descriptor 必须直接成为 Cobra/ContractFinal 的共同构造输入。
- catalog 只负责构造；Schema、Safety 和 handler 仍使用现有权威类型。
- 不能用 root-help projection、lazy leaf tree 或 argv 路由替代对象压缩。

### P5：两平台证据与发布

- Darwin/arm64、Linux/amd64 使用 Go 1.25.9 跑完整测试、受影响 race、generate/drift/schema gates。Windows amd64/arm64 编译并跑 persistent schema-cache 单测与 coverage-windows 门禁。
- 端到端随机交错测 root help/version/Schema/dry-run/mock/config。
- Lxxx 同机 root help/完整 Build 作为诊断，写清 commit、node count 和入口；绝对 RSS 不是 release gate。
- 首次正式 release 的签名、最终制品与安装验证仍阻挡正式发布。

### 4.1 验收矩阵

| 场景 | 树 | 必须保持的行为 |
|---|---|---|
| root help / version | 完整 runtime tree | 同一 public flags、服务/utility 列表、locale、startup diagnostics |
| Schema hit/miss/repair | 完整 runtime tree | Cobra parsing、shortcut/plugin diagnostics、wire parity；生产走本机 identity+cache，miss 时 live assembly |
| leaf help / dry-run / mock | 完整 runtime tree | aliases、required/groups、Safety、无多余 RPC、统一输出 |
| config / event utility | 完整 runtime tree | shared caller、profile、PreParse 和 cleanup 不缺失 |
| completion | 完整 runtime tree | 候选、描述、directive、alias 与 shell script contract |
| public/embedded constructor | 完整 runtime tree | 不读取宿主 argv 决定 command surface |

阻断条件：

- 出现另一棵公开 tree、help projection、Schema 前置执行或 argv factory selection；
- help bytes、Schema wire、flags、aliases、validation、Safety、错误分类或 cleanup 改变；
- 完整树相对父提交未达到 `B/op -20%`、`allocs/op -8%`，或 ns/op 超出允许回归；
- root help RSS 相对 main 回退，或任一平台超过 RFC 的 50/55 MiB p50/p95 预算；
- clean-head 两平台测试、race 或发布 proof 不完整。

## 5. 性能验收

### 5.1 测量规则

候选、固定 main 和专项父提交使用同机、同 target、同 Go 1.25.9 构建。报告绑定源码 SHA、binary SHA-256、toolchain、argv、环境、stdout/stderr digest 和原始样本。每个端到端场景至少 30 次随机交错；接近门槛时扩到 100 次。中间测量 dump 与过程化复跑脚本不入库。

首次调用、warm cache、default telemetry、`DO_NOT_TRACK` 分开。stdout/stderr 必须匹配 oracle，失败样本不能计作快速成功。代表场景：完整树 microbenchmark、Schema leaf/overview/`--all`、root help、version、calendar leaf help、calendar dry-run/mock、config 和常用 get。public npm wrapper 的 RSS 包含 Node 与 native child。macOS 本机进程 wall time 受安全扫描干扰，不用于端到端 gate。

### 5.2 必过项

| 维度 | 门槛 |
|---|---|
| 单树结构 | help/version/schema/config/业务/completion 的进程 root 均包含完整公开产品；不存在 argv 产品路由、Schema 前置执行或 help projection |
| 完整构树 | 相对本专项父提交，warm `B/op` 至少降低 20%，`allocs/op` 至少降低 8%；ns/op 不得超过 `max(parent ×105%, parent + 1 ms)` |
| help/version | candidate p50 ≤ `max(main ×105%, main + 3 ms)`；p95 ≤ `max(main ×110%, main + 3 ms)` |
| root help RSS | native p50 ≤50 MiB、p95 ≤55 MiB，且相对固定 main 的 p50/p95 不回退；同机 Lxxx 的绝对值和按 command 归一化结果必须进入报告，但因公开节点数不同不作为 release gate |
| Schema | 发运不嵌入 compile-time identity；安装/首次 schema 在本机生成 identity 并写 cache；后续命中走认证 shards。cache-hit 数字可作本机参考，不是发布门禁 |
| 业务命令 | dry-run、mock/get、config 的 p50/p95 相对固定 main 不回退 |
| 正确性 | help bytes、flags、aliases、validation、Safety、Schema wire、输出和错误分类不变 |
| 清理 | telemetry 退出前最多等待 CLI `FlushTimeout` 50ms；业务 cleanup、signal 和退出码测试通过 |

旧的 `launcher ≤ core +5%`、calendar ≤ full-tree 70%、config ≤ full-tree 10% 和 full/selective equivalence gate 全部废止，不得与本口径并存。旧 head 中 calendar 0.85 ms、config 0.36 ms、root help 比 Lxxx 软件慢 2.5% 等数字描述的是已删除的 selective tree / projection 结构，全部标记为历史结果，不能用于当前 Ready 结论。

### 5.3 竞品边界

Lxxx 软件的完整构树只用于结构和单位节点资源参考；Gxx 软件用于观察更小程序映像/init 的上限。DWS 约 1,825 个公开节点，Lxxx 约 905 个，命令面与输出合同也不同，因此竞品绝对 RSS 不替代 DWS 的固定预算和 main 回归门禁。报告必须同时列出节点数、B/op、allocs/op 和端到端 RSS，不能只比较 wall time。

本 RFC 不通过减少 DWS 产品面、增加第二 runtime 或 daemon 追 Gxx 软件 3～4 ms / 8～11 MiB 区间。

## 6. 实施记录

中间测量 dump 与过程化复跑脚本不入库。当时 native 记录见 [Actions run 34080469082](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/actions/runs/34080469082)（head `a8376f92`）。Compile-time Schema identity 已从发运模型中移除。

### 6.1 完整命令树 microbenchmark

本地完整树：Apple M3 Pro，Darwin/arm64；Go 1.26.1（正式 CI 固定 Go 1.25.9）；`BenchmarkNewRootCommand`，每轮 10 次，共 3 轮，取三轮中位数。父提交 `c0131d35`；candidate `34ce7493`。两边均构造完整公开 Cobra tree，不执行 handler。

| 实现 | command 数 | ns/op | B/op | allocs/op |
|---|---:|---:|---:|---:|
| DWS 父提交 `c0131d35` | ≈1,825 | 13,503,654 | 16,431,941 | 164,227 |
| DWS implementation commit `34ce7493` | ≈1,825 | 13,544,096 | 12,681,568 | 148,166 |
| DWS 变化 | — | **+0.3%** | **-22.8%** | **-9.8%** |
| Lxxx v1.0.85 本机参考 | ≈905 | ≈7,000,000 | ≈10,300,000 | ≈84,600 |

门槛已满足（B/op −20%、allocs/op −8%；ns/op 为门槛内噪声）。DWS 当前完整树总 B/op 约为 Lxxx 的 1.23 倍、allocs/op 约 1.75 倍，而 command 数约 2.02 倍。粗略每节点：DWS ≈6.95 KiB / ≈81.2 alloc；Lxxx ≈11.4 KiB / ≈93.5 alloc。每节点数据只能说明主要总量差距来自更大的公开命令面，不能证明单个节点复杂度等价。

本轮已落地的归因：ContractFinal ownership transfer；RunE 不再捕获 build-only 字段；Safety scanner 首次执行时初始化；empty string-slice flag 不再创建 4 KiB CSV writer；shortcut builder 减少 declaration value copy；root help 直接遍历完整 tree。

### 6.2 框架执行闭包与路径分配（本地）

父提交 `b56ee380a` 与 `corecmd.New` 的私有 `executionSpec`：B/op −3.4%，allocs 基本不变；ns/op 落在测量噪声内。逃逸分析显示被执行闭包保留的结构从 848 字节降到 184 字节。产品无需迁移。

随后 `annotatePreferredShortcutOwners` 复用父路径切片剩余容量，`schemaruntime.NormalizeCLIPath` 在已规范化单空格输入上直接返回子串：累计相对该父提交 B/op −4.96%、allocs/op −3.67%。本地构树耗时的轮间方差大于这几项改动的总收益，因此只声明分配下降，不声明加速。

### 6.3 已测量并回退：Result Schema 按需解码

`contract.NormalizeResultSpec` 曾占构树 alloc_space 约 17%。把整树 `map[string]any` 换成在 `json.RawMessage` 上按需解码后，同口径复测 **回归**（ns/op 明显变慢，B/op +4.3%，allocs/op +2.6%），已回退。一次完整的单遍 `json.Unmarshal` 比 N 次小的按需解码更便宜。不要再以「按需解码替代单遍通用解码」优化这条路径；若继续压缩，应减少解析次数本身。

### 6.4 Schema cache-hit 与 help/version/业务

Schema cache 的算法目标：verified protobuf hit 避免约 1,825-command declaration catalog 的 live assembly。上一单二进制候选曾测得 cache-hit 相对 live assembly 的 user CPU 降低 97.6%、RSS 降低 87.5%；该数据仍可证明 cache 方向，但因当时 Schema 在 Cobra 前短路，不能作为当前端到端数字。门槛仍为 warm hit 相对 live assembly 的 user CPU p50 至少降低 80%，peak RSS ≤100 MiB（本机参考，不是发布门禁）。

当时单树 clean-head 验收（生产随后改为本机 identity，下列 cache-hit 数字作历史参考）：

| 平台 | warm cache wall p50 | live assembly wall p50 | warm RSS p50 | live RSS p50 |
|---|---:|---:|---:|---:|
| Linux/amd64 | 59.45 ms | 1,878.07 ms | 56.16 MiB | 323.85 MiB |
| Darwin/arm64 | 65.68 ms | 1,696.10 ms | 49.27 MiB | 326.27 MiB |

root help 与 version 现在都构造完整 tree。帮助直接从同一 runtime tree 渲染，不读取 Schema cache、不执行业务 auth/RPC，也不创建中间 projection。30 次交错 native 样本：

| 平台 | DWS help wall p50/p95 | Lxxx help wall p50/p95 | DWS RSS p50/p95 | Lxxx RSS p50/p95 | 结论 |
|---|---:|---:|---:|---:|---|
| Linux/amd64 | 44.05 / 45.99 ms | 47.11 / 48.88 ms | 47.68 / 49.88 MiB | 42.85 / 43.35 MiB | wall 快 6.5%；RSS 高 11.3%，产品预算通过 |
| Darwin/arm64 | 37.26 / 51.45 ms | 45.06 / 53.49 ms | 41.91 / 42.17 MiB | 44.43 / 44.91 MiB | wall 快 17.3%；RSS 低 5.7%，通过 |

default tracker 相对固定 main 的 help/version p50/p95 gate 在两平台全部通过；`DO_NOT_TRACK` 与 default 的结果也证明退出不再等待约 300 ms 的网络 flush。help stdout 为 4,760 bytes，SHA-256 `590ebfc7d090cdfa81e63cfcf6f47727ad1f4ac5f0fc5451445e855ebcf497d0`，与父实现逐字节一致。

端到端业务/utility 的 native p50（wall / RSS）：

| 平台 | leaf help | dry-run | config | mock |
|---|---:|---:|---:|---:|
| Linux/amd64 | 51.02 ms / 52.13 MiB | 44.10 ms / 47.70 MiB | 44.02 ms / 47.83 MiB | 44.21 ms / 48.05 MiB |
| Darwin/arm64 | 42.87 ms / 45.85 MiB | 38.47 ms / 41.93 MiB | 38.55 ms / 42.05 MiB | 36.97 ms / 42.52 MiB |

wait4 native：完整树优化相对固定 main 将 root help RSS p50 从 51.66 降到 47.68 MiB（Linux，−7.7%），从 45.70 降到 41.91 MiB（Darwin，−8.3%）。两平台 p50/p95 都通过 50/55 MiB 预算。GC 参数实验没有采用：本机 `GOGC/GOMEMLIMIT` 只降低约 0.5 MiB，且增加延迟。

clean-head native p50 对照（wall / RSS）：

| 平台/场景 | DWS | Lxxx 1.0.85 | Gxx 0.22.5 |
|---|---:|---:|---:|
| Linux Schema | 58.32 ms / 54.42 MiB | 47.08 ms / 43.11 MiB | 4.08 ms / 9.05 MiB |
| Linux root help | 44.05 ms / 47.68 MiB | 47.11 ms / 42.85 MiB | 3.03 ms / 6.90 MiB |
| Linux dry-run | 44.10 ms / 47.70 MiB | 47.25 ms / 42.62 MiB | 4.45 ms / 9.92 MiB |
| Darwin Schema | 46.92 ms / 48.94 MiB | 45.02 ms / 44.34 MiB | 8.70 ms / 9.67 MiB |
| Darwin root help | 37.26 ms / 41.91 MiB | 45.06 ms / 44.43 MiB | 8.08 ms / 8.00 MiB |
| Darwin dry-run | 38.47 ms / 41.93 MiB | 45.07 ms / 44.41 MiB | 9.07 ms / 10.81 MiB |

Lxxx/Gxx 是诊断；固定 main 回归仍是产品 release gate。DWS Schema 还包含完整树与认证 cache read；竞品 Schema/业务输出合同不等价。

darwin-arm64 本机五维对照（当时 CI 同口径、go1.25.9）五个负载 wall p50 全部快于 Lxxx，但绝对值约 350 ms 远高于 CI 的 37–44 ms（本机安全扫描干扰）。可用证据是同机比值。CPU `user_ms` 上 leaf-help 与 schema 当时仍高于 Lxxx。

### 6.5 Schema / leaf-help 差距归因与否决路径

CI native run `34097698630`（head `9e52edcc`）wall p50（ms）：

| 负载 | linux DWS | linux Lxxx | 判定 | darwin DWS | darwin Lxxx | 判定 |
|---|---|---|---|---|---|---|
| help | 43.98 | 47.26 | 快 | 44.39 | 49.27 | 快 |
| version | 43.37 | 46.00 | 快 | 41.27 | 50.38 | 快 |
| leaf-help | 50.58 | 46.29 | **慢** | 48.35 | 50.04 | 快 |
| schema | 57.78 | 47.07 | **慢** | 52.35 | 50.15 | **慢** |
| dry-run | 43.49 | 46.88 | 快 | 46.17 | 49.35 | 快 |

需要关闭的差距当时为 linux schema 10.71 ms、linux leaf-help 4.29 ms、darwin schema 2.20 ms。两个负载同源——都要解析 Schema Meta，root help 不需要。`BenchmarkRealSchemaFileHit` 阶段耗时与差距吻合：linux `selected-...-decode-index` 6.517 ms ≈ leaf-help 额外成本；linux `meta-...-decode-lookup` 4.536 ms 是 schema 差距的主要部分。

否决路径：

- **只解码 Identity**：`validMetaAliasExpansion` 对别名行调用完整 `equalCommandMeta`（含 Safety/Selection）。只解码 Identity 会削弱校验。
- **leaf-help 改读 ContractFinal**：`ToolSpec.Selection` 的 provenance 按字段条件裁决，不恒为 `contract_final`；直读会漂移 help 文本。`RenderHelpAffordances` 一次 `ResolveMeta` 同时取 Selection / Safety / Identity，只改 Safety 仍触发整笔 Meta 读取。
- **给 `DecodedSchemaMeta` 加锁做惰性记忆化**：该类型多处按值传递并用 `reflect.DeepEqual`；加 `sync.Mutex`/`sync.Map` 会触发 copylocks 并破坏 DeepEqual。正确方向是按需解码但不做记忆化（不可变 `commandEntries` + 纯函数 `CommandMeta(path)`）。
- **identity-only 的不完整 `CommandMetaByPath`**：`RenderHelpAffordances` 需要 Selection；值不完整会缺 help 文本。

信任模型：`equalCommandMeta` 的别名/主行一致性是写入方正确性检查。cache 完整性已由哈希保证；读取时重跑对损坏是冗余的。写入方自校验已落地（`BuildSchemaCache` 在 `DeepEqual` 后执行 `validMetaAliasExpansion`）。protobuf unmarshal 仍会解码全部字符串字段（约 950 KB/op）；只跳过 Go 侧 `CommandMeta` 转换最多省约 20% 分配，不足以单独关闭 95% 的 Meta 读取预算。

### 6.6 DTO v5：预渲染 compact 叶子与投机预热

v4（payload 文件）落地后 CI（head `ddc84f1c`）仅剩 schema 落后。浪费是「为输出一个工具的字节而解码整个产品分片」。

DTO v5：payload 分片 = 4 字节头长 + 头 proto + 原始叶子 blob 区。`schema <leaf> --compact -f json` 快路径只做小 range 读，完全不打开 registry 分片；leaf-help 的 ResolveMeta 只读头。写入方要求 rendered 集合精确覆盖全部非空 canonical 路径且每条是换行结尾的合法 JSON；读取方逐级 SHA-256 认证。`TestPersistentSchemaCacheRenderedLeafFastPath` 断言快路径零 Registry I/O，并与同二进制禁缓存的活体渲染逐字节一致。

本地实测（M3 Pro，冷进程模拟）：单叶 schema 从 5.74 ms / 6.07 MB / 79.7k allocs 降到 **3.31 ms / 1.98 MB / 19.7k allocs**。当时曾用链接期钉住 payload 身份字段缩短认证链；**发运模型已改为不在编译期生产 Schema identity**。

投机预热（纯运行时，无格式变化、无新 pin）：`PrewarmSchemaCache` 用 `WithNoCreate` 只读探测，与 Cobra 建树重叠；三处 range 读共用进程级单一 payload 句柄。缺失缓存时探测零 mkdir/零写。repair 入口在锁内丢弃共享句柄。

收官 CI（run 34272576441，head `7bb3e64c`，固定 Lxxx 1.0.85，wall p50）两平台五负载相对 Lxxx 诊断全胜：linux-amd64 help −3.51 / version −2.51 / leaf-help −1.54 / schema −2.17 / dry-run −3.02 ms；darwin-arm64 help −5.33 / version −5.22 / leaf-help −3.12 / schema −11.66 / dry-run −7.41 ms。过程中对 CI 测量做了稳健化（不改变门禁语义）：default-entry 采样 30 → 60、native 作业超时 75 → 120 分钟、`-race cli` 子集超时 15 → 30 分钟。

## 7. 编译期 Schema 交付（未采纳）

曾单独起草「编译期 Schema 交付与命令树 codegen」方案，用于关闭 §6.5 中 leaf-help / schema 相对 Lxxx 的剩余 wall 差距。**本 PR 不采纳该方案作为发运模型。** 当前产品事实仍是：声明即 Catalog 的运行时装配 + 本机生成 identity + 认证磁盘 cache；无 compile-time identity seal，也无 `cmd_schema_catalog` `//go:generate` 交付步骤。

### 7.1 问题陈述（历史）

当时 CI 显示 help / version / dry-run 两平台已快于 Lxxx，但 linux leaf-help 与两平台 schema 仍慢。完整树构造基线约 44 ms（linux）已不是差距来源；额外成本来自 Schema Meta 读取。Lxxx 的 `schema` ≈ 它的 `help`，额外成本接近零：命令面约一半，且读取时无逐次验证负担。

### 7.2 为什么增量优化不够（历史分析）

根因是两条现行架构决定：

1. **「声明即 Catalog」运行时装配。** 装配昂贵 → 需要 verified cache → cache 需要哈希校验 + protobuf 解码 + `CommandMeta` 转换。Meta 读取链路的存在本身就是这条决定的代价。
2. **每次进程调用构造完整 Cobra 树。** 约 1,825 个节点每次全建。

关闭 linux leaf-help 当时需削减约 4.29 ms，而整笔 Meta 读取只有约 4.536 ms——需要削减约 95%。protobuf unmarshal 会解码全部字符串字段；只跳过 Go 侧转换不够。

### 7.3 曾考虑的三条路

| 方案 | 内容 | 与现行契约 |
|---|---|---|
| A. 编译期交付不可变 Schema 目录 | 构建期生成可直接寻址结构并编译进二进制，取代运行时装配 + 磁盘 protobuf cache | **直接违反** AGENTS.md：无 `//go:generate` Catalog 交付；Meta/分片是可丢弃传输派生物 |
| B. 命令树构建 codegen | 构建期生成直接构造 Cobra 命令的 Go 代码，压 44 ms 基线 | 不必然违反现行契约，可独立评估 |
| C. 索引化导航 | 预编译导航索引，不必每次构造全部节点 | 接近禁止的 separate root-help projection，需独立 RFC |

A 若将来采纳，必须先修订 AGENTS.md 与本 RFC，并新增生成物漂移门禁。关键不变量仍须保留：声明是唯一权威；`dws schema` 线格式不变；每个 Schema 工具仍可解析到可执行 Cobra 命令。

### 7.4 本 PR 的结论

不通过 A 或 C 关闭剩余差距。已落地的是磁盘 cache 上的 DTO v5 预渲染叶子、写入方自校验、投机预热，以及本机 identity generate-then-use。编译期封印 identity、把 Catalog 编进二进制、或第二套公开 command surface，均不在本 RFC 的发运范围内。B/C 若要推进，另立 RFC。

## 8. 非目标

- 不在本 RFC 内裁剪 open edition 的产品面。
- 不以 daemon、常驻进程或语言重写追求 Gxx 软件 3～4 ms。
- 不提交生成 Catalog，也不让 cache 成为声明源。
- 不为 completion 增加尚不存在的 callback 门闩。
- 不凭代码阅读删除 auth、Safety、Prepare 或业务 cleanup。
- 不把编译期 Schema 目录或 compile-time identity 重新引入发运路径。

## 9. 代码对齐清单

- [x] launcher/core/package manifest 撤回，发布恢复单 `dws`。
- [x] argv 产品选择、route index、插件资格分流和 selective-tree tests 删除。
- [x] Schema 的 Cobra 前置 fast path 删除；cache 由正常 schema handler 使用。
- [x] root help snapshot/model package 删除；公开 help 直接遍历完整 runtime tree。
- [x] ContractFinal ownership transfer、build-only closure 清理、lazy Safety scanner 和低分配 string-slice builder 落地。
- [x] 测试钉住 process invocation 总是包含完整产品面。
- [x] 撤回 compile-time Schema identity 生产与发布封印；各端在安装/首次 schema 生成本机 identity 并写 cache。
- [x] telemetry 默认 `FlushTimeout=50ms` 且不设置 `NoFlushWait`；`NoFlushWait` 仅作为 SDK 可选字段保留。
- [x] 完整树 B/op 和 allocs/op 达到本地父提交门槛（−22.8% / −9.8%）。
- [x] 两平台完整测试与 race 通过（head `a8376f92`，run `34080469082`）。
- [x] 两平台固定 main 性能矩阵通过；root help p50/p95 均低于 50/55 MiB，且相对 main 降低。
- [x] DTO v5 预渲染 compact 叶子与投机预热落地；诊断性五负载相对 Lxxx 的收官 CI 通过。
- [ ] 完成 app/helpers/shortcut/corecmd/cli、packaging 与全仓测试在当前 head 上的持续绿灯。
- [ ] 首次正式 release 的签名、最终制品与安装验证；阻挡正式发布。

## 10. 回滚

完整树优化按独立提交回滚，不改变 Schema/Safety 的权威源。生产不依赖 compile-time identity；本机生成的 identity 只命中同一二进制指纹下的 cache。telemetry 默认 `FlushTimeout=50ms` 且不启用 `NoFlushWait`；若需牺牲末条事件完整性以进一步降低退出延迟，可显式启用 SDK `NoFlushWait`。可靠且不阻塞的投递需要另立持久 outbox RFC。
