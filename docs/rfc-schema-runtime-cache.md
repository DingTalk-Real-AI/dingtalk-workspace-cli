# RFC：DWS 热路径 runtime、Schema Runtime Cache 与发布合同

| 字段 | 内容 |
|---|---|
| 状态 | Draft；产品形态已定，代码对齐与发布验收未完成 |
| 范围 | Schema delivery、身份生成与 release proof、受限热路径 runtime / core 双二进制 canonical package、安装/升级/回滚及性能验证 |
| 基线 | `5243e5ca19b55a3e785e5cc09273b653ad5381dc` |
| 目标 | 降低普通业务命令首次调用 `ResolveMeta` 时的 Schema 装配成本，同时保持 Schema、安全元数据与发布合同同源 |
| 非目标 | 不恢复 deprecated `dws cache` 产品面；不把生成 Catalog 提交进仓库或嵌入二进制；不修改公开 Schema JSON 合同 |


性能数据统一放在[性能附件](rfc-schema-runtime-cache-performance.md)及其原始样本中；正文只规定选择、约束和验收。

本文的规范性决策自 **2026-09-06** 起以 §1、§6.6、§6.7.1、§6.11、§8.4 和 §9 为准；§10 逐项记录代码差距。历史实验用于说明选择，不定义第二套验收标准。逐提交实施流水不再放在本 RFC 正文，既有原始记录保持不变。

`c0f3aaca` 原生 CI 按旧脚本收集的数据保留原始门槛标识。原始报告的 `passed/gates` 只能解释为当时规则下的结果；新设计验收必须在 §10 的阻挡项清零后重新执行。不得改写旧报告布尔值、把旧 gate 通过称为新架构已验收，或用候选数据宣称正式产品已经达标。

## 1. 决策摘要

### 1.1 产品形态：选 A，保留双二进制

本 RFC **选择 A**：`bin/dws` 是受限的**热路径 runtime**，`libexec/dws-core` 是完整命令 runtime。保留现有 `launcher` 包名以避免无关重命名，产品与设计叙事不再使用 thin/极瘦启动器。它承担限定的执行职责，不能以“优化”名义扩展成通用 CLI。

不采用 B（收回单二进制 / help、version 全部回 core）。这是本 RFC 的固定设计，不以某次微基准胜负在 A/B 间切换。若未来改选 B，必须另行修改产品形态决策和验收，不在本实现中暗改。

选择 A 接受两项成本：维护绑定同一版本的两个制品；每次委派前完整验证 core 的 size/SHA-256。Unix 委派使用 `execve` 保持 PID，并非两个 runtime 同时驻留；Windows 使用 spawn/wait。SDK、只读 profile metadata、信号与错误格式通过共享实现保留语义，不复制实现来满足包体积目标。

### 1.2 可执行 allowlist：能力与依赖均封闭

以下是允许的全部 launcher 执行能力，未列出的一律交给 core。环境变量的 `plain` 判定由 `schemafastpath.PlainEnvironment` 统一拥有；会读取配置目录的 help/Schema 再由 `schemafastpath.PlainInvocation` 统一验证无扩展和配置缺席，不能在每条快路径中自行放宽。exact `--version` 的默认上报模式应用环境 allowlist；显式 telemetry opt-out 的输出不受扩展或其他 `DWS_*` 环境影响，保留 exact argv + opt-out 判定后的零文件系统路径，不为检查扩展而读取配置目录。

| 能力 | 允许 | 禁止 / 委派 |
|---|---|---|
| 版本 | exact `dws --version`；完整 version/commit/buildTime；默认模式共用 tracker，显式 opt-out 保持不读身份 | 附加 flags、别名、缺失所需元数据或不在支持范围的调用委派 |
| 根帮助 | exact `dws --help`；v1 open edition、已证明 target、plain 调用；只渲染已认证 `RootHelpModel`，en/zh 与 core 同源 | leaf help、`help <path>`、`-h`、扩展/动态发现、非 plain、非 open 全部委派，详见 §6.7.1 |
| Schema | 仅显式 telemetry opt-out、受 `schemafastpath.Prepare` 接受的 argv、有效 identity 和已认证命中；共用 typed reader/renderer | **默认上报 Schema 永远由 core 执行**；missing/corrupt cache、未知 argv、需要装配/repair 的调用委派 |
| 通用支持 | release identity/core 完整校验、共享信号/错误处理、只读 profile metadata、官方 SDK 的既有上报 | 不执行业务命令，不增加自己的认证、发送器或第二套 metadata/参数语义 |

仓库包 allowlist 必须与 [入口传递依赖门禁](../internal/launcher/dependencies_test.go) 中 `TestCrossPlatformCoverageLauncherRuntimeDependencies` 的集合完全一致：

| 允许的包（完整列举，组内不是通配符） | 限定用途 |
|---|---|
| `cmd/dws-launcher`、`internal/launcher`、`internal/buildversion` | 固定能力路由、制品校验、版本与委派 |
| `internal/roothelp`、`internal/localename` | 只读帮助模型、共享 renderer 与语言规则 |
| `internal/clitelemetry`、`internal/profilemetadata`、`internal/clisignal` | 既有官方 SDK、非凭据身份投影、信号；后两者直接依赖只能是标准库 |
| `internal/jsonutil`、`internal/errors`、`internal/tui`、`pkg/config`、`pkg/validate` | 既有 JSON/错误/非交互样式与纯配置校验；不授权交互 TUI、认证或业务执行 |
| `internal/schemacache`、`internal/schemareader`、`internal/schemafastpath` | 安全文件读取、identity 与严格快路径准备 |
| `internal/cli/schemacachepb`、`internal/cli/schemaruntime`、`internal/corecmd/contract`、`internal/skillpaths` | generated DTO、纯查询与合同、兼容路径；不包含父包 `internal/cli` |

**永久禁止项（本 RFC v1）**：Cobra/pflag、`internal/app`、`internal/cli` 根包、auth/凭据/keychain、plugin 执行、runtime payload、业务 transport、完整 Schema assembler，以及上述清单外的仓库包。禁止通过允许包的传递依赖绕过边界。

非标准库网络依赖只允许现有 gate 中的精确边：官方 SDK `internal/sender → net/http`、`aem → net`、`internal/encoder → net/url`，以及 `pkg/config`、`internal/errors`、`pkg/validate` 的 `net/url` 解析。不得新增 launcher 自有 sender。纯 Schema reader 的 [独立依赖门禁](../internal/cli/schemaruntime/dependency_test.go) `TestCrossPlatformCoverageThinSchemaDependencyClosure` 继续禁止全部网络/命令框架及反向依赖 launcher；该旧测试名不代表 launcher 产品形态。

**新增快路径或扩大现有 argv/环境适用范围的合入顺序固定为：先更新本节 RFC allowlist 与对应 dependency/能力路由 gate，经评审后才能合实现。** 不得先合代码、后补白名单。能力须有可枚举的入口合同与正负路由测试；PR policy 必须验证能力集合变化同时修改 RFC 与 gate。当前仅有包依赖检查尚不能执行这一完整规则，差距见 R1。

### 1.3 数据形态与单一声明源

本 RFC 采用一个由发布二进制内容摘要约束的、本地自愈的 Schema Runtime
Cache，并采用 Meta + product-sharded Registry 两层派生物，而不是让所有消费方读取同一个
39 MB JSON 或 14.92 MB protobuf：

| 消费路径 | 缓存内容 | 编码 | 压缩 | 当前样本 | 决策理由 |
|---|---|---|---|---:|---|
| 普通业务命令、leaf help、`ResolveMeta` | generated `SchemaMetaCache` protobuf DTO v2 (historical v1 prototype measurements) | deterministic protobuf | 不压缩 | overview/locator-inclusive Meta mirror 718,182 B | decode + complete lookup p50 约 1.44 ms、2.44 MB allocated；gob 的约 0.15 ms 差异不足以证明收益，统一 protobuf 可减少 parser/format 面 |
| `dws schema` leaf/product/group | `SchemaMetaCache` 内的 path→product locator + concatenated generated product protobuf shards | deterministic protobuf | 不压缩 | 31 shards 合计 14,927,893 B；目标 calendar shard 549,488 B | Meta resolve 后的 Registry stage：authenticated locator + `pread` + shard SHA-256 + decode/conversion + product-local `Index()` p50 约 4.35 ms、3.80 MB allocated |
| `dws schema --all` | 同一 product shard container | deterministic protobuf | 不压缩 | 与完整 Registry protobuf 相比只增加 7,170 B payload | 逐 shard 认证/解码后组装完整 Registry 并做全局 `Index()`；不维护第二份 monolith |

缓存文件不是新的 Schema 权威。只有同时通过以下检查，派生物才可被使用：

1. 固定文件头、格式版本、kind、codec 和长度上限与发布二进制内置预期值完全一致。
2. 在 protobuf decoder 接触任何 bytes 前，该 exact byte range 的 length 和 SHA-256 必须来自
   binary-pinned Meta artifact 或其已认证 product descriptor 并完全匹配；header 中的自描述副本
   不构成信任锚。
3. header 的 `source_hash`、`surface_hash` 和 `build_id` 与二进制预期值完全一致。
4. private protobuf 禁止 protobuf map/Any/Struct；optional、nil/empty map/slice、RawMessage 均
   使用显式 presence/sorted repeated entries（Meta v2 的列表用存在位，其余 optional 用 wrapper），generated message pointer 只表达 wire presence，
   解码后通过 exact semantic conversion/validation。
5. DTO 通过 typed validation；它与 public Catalog `source_hash`/`surface_hash` 的关系由
   release-time exact projection gate 和已认证 artifact digest 建立，cache hit 不重建 public
   `map[string]any` snapshot 来重算 hash。

对用户可写缓存文件的上述检查失败都视为 cache miss；内置 identity/帮助绑定的失败不适用此规则。进程同步回到声明驱动的
`NewSchemaSourceRootCommand → ResolveSchemaBuild`，成功后原子替换缓存。缓存读写失败
不得覆盖一次成功的实时装配结果。

普通 `go build`/dev 或明确 disabled target 的 identity 全部未注入时，持久缓存禁用，只保留进程内 lazy loader。未知 overlay、`RegisterExtraCommands` 非 nil 或运行时声明环境不在 proof 范围内时，不消费缓存。已注入 identity 的字段不完整、解析失败或与编译身份矛盾，按 §6.11.1 fail-closed；不能把它当作未启用。

## 2. 背景与问题定义

固定 pre-PR 基线的生产路径是：

```text
app.NewRootCommand
  └─ registerSchemaRuntimeDelivery
      └─ cli.RegisterSchemaSourceRoot(app.NewSchemaSourceRootCommand)

first ResolveMeta / leaf help / schema query
  └─ deliverySchemaCatalog sync.Once
      └─ app.NewSchemaSourceRootCommand
      └─ cli.ResolveSchemaBuild
      └─ validate + build SchemaRegistry/SchemaIndex
      └─ render SchemaCatalogSnapshot
      └─ installDeliveryCommandMeta
```

`sync.Once` 只消除同一进程内的重复装配。CLI 每次调用都是新进程，因此普通业务命令
第一次读取 safety/selection metadata 时仍支付完整 Schema 装配、JSON projection 和
`CommandMeta` 建索引成本。

基线上的进程级观测如下。wall time 会受当前机器上的 telemetry 和调度影响，因此 CPU
时间与最大 RSS 更适合作为归因证据：

| 命令 | real | user | 最大 RSS |
|---|---:|---:|---:|
| `dws --help` | 0.54 s | 0.10 s | 50 MB |
| `dws auth status --help` | 4.28 s | 2.45 s | 363 MB |
| `dws schema list -f json` | 3.82 s | 2.37 s | 380 MB |

root help 和 version 保留 Schema-lazy、零缓存 I/O 合同；exact `--version` 与符合边界的 root help 由 §1.2 热路径 runtime 执行。
会调用 `ResolveMeta` 的普通命令和显式 Schema 查询使用同一缓存体系。

## 3. 目标与非目标

### 3.1 目标

- Cache hit 时，普通业务命令不创建第二棵 Cobra tree，也不调用 `ResolveSchemaBuild`。
- `ResolveMeta` 与完整 Schema 查询仍由同一次声明装配产生，不出现两套语义权威。
- 本地文件损坏、截断、过期、不可信、只读目录和并发首次启动均可自动恢复。
- safety、confirmation、interface 和 provenance 不能被用户可写缓存静默篡改。
- 官方 release 和 source fork 对“是否启用缓存”有可验证的构建合同；external private
  overlay 在 v1 fail-safe disabled。
- 用户可写 Schema cache 失败时保持声明驱动行为；内置密封身份/帮助绑定损坏按 §6.11 fail-closed，不能混作 cache miss。
- 控制缓存大小和瞬时内存，不接受无界反序列化；v1 不引入 decompressor。

### 3.2 非目标

- 不追求 launcher 无 SDK、无网络依赖或只有校验/委派；官方上报是允许的限定职责。
- 不追求全部 help 入口无委派，不为 non-open edition 封存 root help，不把业务命令搬进 launcher。
- 不把 Lark/GWS 的 public/native 排名作为本 RFC Ready 或 release gate；领先声明必须另附同条件证据。
- 不追求委派入口与直接 core 的 wall 时间相差 ≤5%；逐次 core 校验成本作为诊断项单列。
- 不采用另一份手写帮助声明，不让 snapshot、Catalog dump 或缓存成为声明权威。
- 不通过减少安全校验、关闭默认上报、缩短 SDK flush budget 或默默丢弃慢样本达到预算。
- 不改变 `dws schema` 公开 JSON wire contract。
- 不提交 `schema_catalog/`、`schema_meta_index.gob` 或新的生成 Catalog 权威。
- 不把完整 Schema blob 嵌入可执行文件。
- 不为静态 Schema 增加 TTL、后台刷新或 stale-while-revalidate。
- 不提供用户可选 codec、cache path 或 cache lifecycle 命令。
- 不恢复 deprecated 服务发现兼容命令 `dws cache`。
- 不以 cache hit 为理由跳过 typed 数据的结构与安全校验。

## 4. 关键事实与约束

### 4.1 `source_hash` 不能直接作为廉价 freshness key

`SchemaCatalogSnapshot.SourceHash` 只在完整 `ResolveSchemaBuild`、Registry projection 和
JSON marshal 之后产生。若每次启动先实时计算它再判断 cache hit，昂贵工作已经发生，
缓存失去意义。

它仍适合作为发布时生成并注入二进制的信任锚，但不适合作为运行时先算后查的 key。

### 4.2 自描述 hash 只能防损坏，不能证明来源

用户可写文件可以同时修改 safety 字段和重算 manifest SHA-256。以下信息都不能单独
证明缓存属于当前二进制：

- CLI version；
- short Git commit；
- edition + version；
- snapshot 自己声明的 `SourceHash`；
- 只覆盖 command identity/navigation 的 `SurfaceHash`；
- 只把 executable digest 放入目录名。

因为 `ResolveMeta` 会返回 `risk` 和 `confirmation`，命中条件必须包含二进制内的预期
encoded payload SHA-256。校验必须先于任何 payload parser；manifest checksum 和 codec
checksum 仅负责发现随机损坏。

### 4.3 Meta 与 Registry 使用 generated private protobuf，不能直接 gob runtime model

直接 gob `SchemaRegistry` 不可接受：gob 不承诺跨 Go 版本稳定 wire；pointer flattening 会让
`Selected: &false`、nil pointer 等 optional 语义产生风险；empty collection 也可能恢复为 nil；
map iteration 不能提供确定字节；interface 会扩大可实例化类型面。

真实完整 mirror 复测显示 generated protobuf 在 Registry 路径优于 gob；Meta 的 protobuf
codec-stage latency 为 1.44 ms，给 5 ms 完整预算留下余量但尚未证明完整命中达标，且分配更低。约 0.15 ms 的 Meta gob 差异不足以证明第二种 parser/format
值得存在，因此 v1 的两个 artifact 都使用 generated private protobuf，并满足以下硬约束：

- payload 的 exact length/SHA-256 在创建对应 decoder 前匹配 binary-pinned expectation；任意
  用户构造字节不能进入 protobuf parser；hash 和 decode 必须使用同一个不再修改的内存
  byte slice，禁止验证后 reopen 或继续从文件 stream decode；
- `.proto` 不使用 `map`、`Any`、`Struct` 或 opaque JSON envelope，只使用 typed
  message/scalar/repeated fields；
- optional value 使用 message wrapper；nil/empty collection 使用 presence message + repeated
  items，Meta v2 的六个字符串列表例外，改用显式存在位（§6.2.1）；map 转为按 UTF-8 key 排序的 repeated entry；RawMessage 使用 presence message + bytes；
  protobuf generated pointer 不能直接泄漏到 runtime model；
- Meta 和每个 product shard 都使用 `proto.MarshalOptions{Deterministic:true}`；generator 连续编码
  两次必须逐 artifact byte-equal；protobuf schema digest、generator/runtime version、Go toolchain、
  DTO version 和 exact artifact digest 一起进入 Build ID；
- release gate 对每个字段执行 Registry/projection → generated protobuf → bytes → protobuf →
  Registry/runtime lookup，并分别做
  `reflect.DeepEqual` 和 public wire equality，不依赖“能解码”作为等价证明。

这个格式只是当前二进制私有、可丢弃的本地 transport，不是长期 public storage contract。
任何 wrapper、generated schema、conversion 或 determinism gate 无法满足时，不得发布 enabled-target 包；明确 disabled 的开发产物不启用 persistent cache。

### 4.4 现有 JSON snapshot 适合 wire，不适合本地热路径

当前 1,357-tool 样本的单文档 JSON 为 39,281,780 B。现有
`decodeSchemaCatalogSnapshot` 会先解成 `map[string]any`，再逐 tool marshal/unmarshal 为
typed wire struct，最后规范化、校验并建立 `SchemaIndex`。

实测中位数为约 2.32 s、1.33 GB allocated bytes、17.67 million allocations。它与实时
装配成本处于同一数量级，直接把该 JSON gzip 后落盘不能解决业务命令启动问题。

### 4.5 Meta index v1 不是可直接上线的 runtime contract

现有 `SchemaMetaIndexSnapshot` 明确是 CI/test dump。它的 entry 未包含
`CommandMeta.Selection.Prerequisites` 和 `Tips`，因此是有损投影。

实现不复用该 dump 格式，而是定义 generated `SchemaMetaCache` protobuf DTO v2，补齐 `CommandMeta` 全部字段，
并增加“Registry projection 与 DTO lookup 完全相等”的 generation/release gate。现有
MetaIndex v1 文件不能被 runtime cache 接受。

## 5. 候选压缩算法调研

### 5.1 候选集

| 方案 | Go 实现 | 完整性能力 | 依赖特征 | 本方案中的主要取舍 |
|---|---|---|---|---|
| raw | 标准文件 I/O | 依赖应用 envelope SHA-256 | 无 codec | 最低 CPU，文件最大 |
| gzip/DEFLATE | `compress/gzip` | CRC-32 + uncompressed size；必须读到 EOF 才完成验证 | 标准库，且 DWS 已在生产路径使用 | ratio 和 decode 不是最优，但没有新增依赖/二进制成本 |
| zstd | `klauspost/compress/zstd` | 可选 frame checksum；支持 decoder memory/window/cap 限制 | 新第三方 multi-codec module | ratio 和 decode 优秀，但本样本相对 gzip 的绝对收益较小 |
| LZ4 frame | `pierrec/lz4/v4` | header/content checksum，可选 block checksum | 新第三方 module | decode 快，ratio 介于 gzip 与 snappy；reader buffer 较大 |
| Snappy | `golang/snappy` | framed format 有 chunk CRC；block format需应用层 SHA | 新第三方 module | decode 最快之一，ratio 最弱 |
| raw DEFLATE | `compress/flate` | 无 gzip container checksum | 标准库 | 没有优于 gzip container 的收益，拒绝 |

上游文档事实：

- Go [`compress/gzip`](https://pkg.go.dev/compress/gzip) 明确说明只有读取到 EOF 才完成
  checksum 校验，且压缩后的精确字节不属于 Go 1 compatibility promise。
- [`klauspost/compress/zstd`](https://pkg.go.dev/github.com/klauspost/compress/zstd)
  是 pure Go 实现，提供 `WithDecoderMaxMemory`、`WithDecoderMaxWindow` 和
  `WithDecodeAllCapLimit`；上游也明确不保证不同版本产生相同压缩 bitstream。
- [Zstandard format](https://github.com/facebook/zstd/blob/dev/doc/zstd_compression_format.md)
  的 content checksum 是可选的 xxHash-64 低 32 位，不能替代应用信任锚。
- [`pierrec/lz4/v4`](https://pkg.go.dev/github.com/pierrec/lz4/v4) 是 pure Go frame/block
  实现；[LZ4 frame spec](https://github.com/lz4/lz4/blob/dev/doc/lz4_Frame_format.md)
  定义了 content checksum、block checksum 和最大 4 MB block。
- [`golang/snappy`](https://pkg.go.dev/github.com/golang/snappy) 区分 block 与 framed
  format；[Snappy framing spec](https://github.com/google/snappy/blob/main/framing_format.txt)
  明确指出 metadata 无 checksum，不能发现所有截断。

无论选择哪种 codec，应用层都必须校验二进制注入的 SHA-256；codec checksum 只是一层
快速损坏检测。

### 5.2 实测方法

环境：

| 项 | 值 |
|---|---|
| OS/Arch | macOS / arm64 |
| CPU | Apple M3 Pro |
| Go toolchain | go1.25.9（与当前 `go.mod`/release 一致） |
| Schema source hash | `sha256:89903f5ce914316ee4ed2094d8d9be881c9fa6fa698d3a4e6486896e08d3d96f` |
| Tool count | 1,357 |
| gzip | Go stdlib |
| zstd | `github.com/klauspost/compress v1.18.0` |
| LZ4 | `github.com/pierrec/lz4/v4 v4.1.22` |
| Snappy | `github.com/golang/snappy v1.0.0` |
| CBOR | `github.com/fxamacker/cbor/v2 v2.9.0` |
| MessagePack | `github.com/vmihailenco/msgpack/v5 v5.4.1` |
| Protobuf | token proxy：`github.com/golang/protobuf v1.5.4`；完整 mirror：`protoc 35.1`、`protoc-gen-go v1.33.0`、`google.golang.org/protobuf v1.33.0` |
| FlatBuffers | `github.com/google/flatbuffers v25.2.10` |
| Cap'n Proto | `capnproto.org/go/capnp/v3 v3.1.0-alpha.2` |

测试使用同一次生产 `ResolveSchemaBuild` 的当前 typed `SchemaRegistry` 和完整 Meta
projection 作为 cache-private DTO 的大小/codec proxy；DTO 落地后必须复跑 selected-path
benchmark。codec reader 每次重新创建并校验完整 round trip。解压预先按可信 envelope
length 分配目标 buffer，并读取到 EOF，以免把 `io.ReadAll` 的扩容差异误算为算法差异。
最终选择额外测量了
`open + read + binary-pinned SHA-256 + decode + Registry.Index()` prototype file-hit 路径。

该数据是算法选择证据，不是跨平台 SLA。实现 PR 仍需保留 selected-path benchmark，
并在 linux/amd64 和 windows/amd64 做 correctness/build 验证。

serializer matrix 不含磁盘 I/O，输入和 encoded bytes 已在内存中 warm up；使用默认
`GOMAXPROCS=12`，未锁定 CPU frequency，也未隔离系统其他进程。serializer 命令使用
`SERIALIZER=<name>`、`GOTOOLCHAIN=go1.25.9`、`-run '^$'`、
`-bench '^BenchmarkSerializers$'`、`-benchmem`、`-benchtime=1x` 和 `-count=5`；compression matrix 使用
`BenchmarkCombinations`/`count=3`，AES-GCM 使用 `BenchmarkAESGCM`/`count=7`。每次
serializer 隔离运行，报告中位数而非单次最好值。该 one-off harness 不进入 production
module；实现 PR 必须提交 selected-path benchmark，并报告样本分布、warm/cold file、
`GOMAXPROCS`、peak RSS 和 CPU profile。当前 matrix 只能排序实现候选，不能建立 release SLA。

### 5.3 typed Registry JSON 与压缩结果

输入为 23,562,521 B 的 typed Registry JSON：

| Codec | 输出大小 | Ratio | Codec decode 中位数 |
|---|---:|---:|---:|
| raw | 23,562,521 B | 100.00% | 1.69 ms |
| gzip-1 | 1,506,399 B | 6.39% | 47.6 ms |
| gzip-default | 1,108,938 B | 4.71% | 24.6 ms |
| gzip-9 | 1,089,862 B | 4.63% | 21.5 ms |
| zstd-fastest | 1,084,424 B | 4.60% | 11.5 ms |
| zstd-default | 967,665 B | 4.11% | 10.3 ms |
| zstd-better | 854,663 B | 3.63% | 12.2 ms |
| zstd-best | 808,472 B | 3.43% | 9.1 ms |
| LZ4-fast | 1,750,512 B | 7.43% | 9.3 ms |
| LZ4-HC | 1,305,067 B | 5.54% | 9.0 ms |
| Snappy block | 2,990,840 B | 12.69% | 7.8 ms |

生产工具链下的 prototype file-hit 路径各运行 10 个独立样本。它覆盖 read、binary-pinned
SHA-256、codec、typed JSON decode 和 `Registry.Index()`，但不冒充尚未实现的 private DTO
conversion 与全部 interface/provenance validation：

| 路径 | p50 | Allocated bytes | Allocations |
|---|---:|---:|---:|
| raw read + SHA-256 + JSON + Index | 254.3 ms | 77.5 MB | 1.049 million |
| gzip read + SHA-256 + gzip + JSON + Index | 264.4 ms | 78.8 MB | 1.050 million |
| public JSON snapshot → typed Registry → Index（探索基线） | 2.32 s | 1.33 GB | 17.67 million |

gzip-default 确实减少 22,453,583 B（95.3%），但该文件位于本地、数量有界、按 edition
原地覆盖，不承担网络传输成本。prototype 完整命中中 raw 反而快约 10 ms、少约 1.3 MB
allocated bytes；它还避免 24.6 ms 单核解压 burst、decompression bomb/multistream 边界和
gzip bitstream 与 Go toolchain 的精确绑定。这组数据支持“不压缩”，但 typed JSON 解析本身
仍约 254 ms，不能满足“尽可能快且 Go 原生”的优先级。

如果未来必须压缩，当前数据下优先复评 zstd-default，而不是 gzip：zstd 输出 967,665 B，
比 gzip-default 更小 141,273 B，纯 decode 约 10.3 ms，也低于 gzip 的 24.6 ms。它目前仍
不进入 v1，因为 raw 已满足大小预算且 CPU 更低，没有必要为了节约本地 22.6 MB 新增
dependency、binary code、解压攻击面和 codec 生命周期。

不压缩选择同样有撤销条件：private Registry shard data 超过 64 MiB - 208 B，或 enabled target 的受控
cold-read benchmark 证明 selected raw artifact 完整 hit p50 比某个 bounded codec 慢至少 25 ms，才重新打开
压缩决策。届时必须同时比较 end-to-end CPU、peak RSS、artifact size、binary size 和依赖面，
不能只比较压缩率。

### 5.4 native gob proxy 历史结果

当前直接 typed Registry gob 是 14,183,649 B：

| 路径 | 中位数 | Allocated bytes | Allocations |
|---|---:|---:|---:|
| raw gob → Registry → Index | 118.0 ms | 94.7 MB | 1.296 million |
| gzip gob → Registry → Index | 141.4 ms | 109.1 MB | 1.297 million |
| raw typed JSON read/hash → Registry → Index | 254.3 ms | 77.5 MB | 1.049 million |

raw gob 相对 typed JSON prototype 将 decode + Index latency 降低约 54%，代价是累计分配增加
约 17.2 MB。这证明二进制 private DTO 的方向有效，但 direct runtime gob 后续未通过 exact
fidelity gate，且 §5.8 的完整同形 benchmark 已由 protobuf 胜出，因此本表不再决定 Registry
serializer。它仍作为早期 baseline 保留，不能被描述为 selected path。

当前有损 Meta v1 gob 为 704,253 B，raw gob decode + alias expansion + lookup 中位数约
1.55 ms、2.94 MB allocated；同一 Meta 的 typed JSON proxy decode + lookup 为约 9.31 ms、
3.19 MB allocated。补齐 `Prerequisites`/`Tips` 与 presence wrapper 后必须复测，但现有数据
显示 stdlib gob 在普通命令热路径约快 6 倍。

| Meta codec | 输出大小 | Codec decode 中位数 |
|---|---:|---:|
| raw gob | 704,253 B | 0.05 ms |
| gzip-default gob | 181,672 B | 2.92 ms |
| zstd-default gob | 181,312 B | 1.63 ms |
| LZ4-fast gob | 254,845 B | 0.67 ms |
| Snappy block gob | 267,329 B | 0.46 ms |

Meta 是每次普通 CLI 调用的关键路径。压缩最多只节省约 0.5 MB，却增加 0.46-2.92 ms codec
CPU；该历史 proxy 支持 Meta 不压缩，serializer 最终由 §5.8 的完整 mirror 决定为 protobuf。

### 5.5 Meta JSON proxy 结果

当前 `BuildSchemaMetaIndex` v1 JSON 为 939,251 B。它缺少 `Prerequisites` 和 `Tips`，因此只
作为 codec/数量级 proxy，不能作为最终 Meta DTO 的 release 证据。raw JSON decode + alias
lookup 建立的 5 次中位数为 9.31 ms，约 3.19 MB allocated bytes。

| Codec | 输出大小 | Codec decode 中位数 |
|---|---:|---:|
| raw | 939,251 B | 0.02 ms |
| gzip-default | 175,490 B | 3.66 ms |
| zstd-default | 176,104 B | 1.64 ms |
| LZ4-fast | 255,408 B | 1.03 ms |
| Snappy block | 272,504 B | 0.53 ms |

该 JSON 数据保留为序列化对照，不是 v1 selected path。它同样说明对本地 Meta 做压缩会用
CPU 换取不足 1 MB 的磁盘，没有收益。

### 5.6 八种序列化方式实测

序列化和压缩是正交选择。为避免只比较不同对象模型，本次从同一份 23,562,521 B typed
Registry JSON 生成一个有序、map-free 的 common token DTO：1,060,253 个 token，每个 token
包含 `kind/name/has_value/value`。`has_value` 显式区分 absent、空 bytes 和非空 bytes；八种
serializer 都先执行完整 token equality gate。JSON、gob、canonical CBOR、sorted
MessagePack、protobuf repeated-message wire、FlatBuffers table/vector、Cap'n Proto
composite-list 和 bounds-checked custom binary 均通过该 gate。

为消除大对象 benchmark 顺序和 GC 干扰，每种 serializer 在独立进程运行
`-benchtime=1x -count=5`，下表是 p50。`decode-to-JSON` 包含恢复 common token DTO 后重新生成
canonical JSON，但不包含 DWS `SchemaRegistry` conversion、typed validation 或
`Registry.Index()`：

| Serializer | Artifact | Encode p50 | Decode p50 | Decode-to-JSON p50 | Decode B/op | Decode allocs/op |
|---|---:|---:|---:|---:|---:|---:|
| JSON | 76,082,769 B | 114.8 ms | 822.6 ms | 896.4 ms | 336.8 MB | 1.368 million |
| restricted-shape gob | 26,080,786 B | 60.9 ms | 115.5 ms | 264.1 ms | 401.5 MB | 2.553 million |
| canonical CBOR | 50,801,048 B | 92.3 ms | 269.4 ms | 413.7 ms | 82.2 MB | 1.368 million |
| sorted MessagePack | 52,391,776 B | 140.7 ms | 224.4 ms | 372.9 ms | 141.6 MB | 1.368 million |
| protobuf | 27,148,573 B | 233.2 ms | 176.2 ms | 329.7 ms | 227.3 MB | 4.081 million |
| FlatBuffers | 55,031,504 B | 73.9 ms | 37.9 ms | 185.8 ms | 82.7 MB | 1.368 million |
| Cap'n Proto | 49,889,480 B | 93.1 ms | 92.2 ms | 240.7 ms | 82.7 MB | 1.368 million |
| custom binary | 23,140,200 B | 11.0 ms | 29.1 ms | 175.4 ms | 82.7 MB | 1.368 million |

这张表是 codec mechanics proxy，不是最终 DTO 的伪装结果。token-per-table 会放大
FlatBuffers/Cap'n Proto 的结构开销；protobuf 使用真实 repeated-message wire，但 benchmark
中的 legacy reflection API 和每-token pointer 也会放大其 allocation。FlatBuffers/Cap'n Proto
通过 runtime API 构造真实 wire，不是把 JSON 塞进 opaque bytes；decode 仍 materialize common
token DTO，因此没有把 zero-copy lookup 当作已经实现。custom binary 同样完整做 length/type
边界检查，但没有演进中的 unknown-field 合同。最终选择以 §5.8 的完整 mirror
`decode → exact conversion → Registry.Index()` 和实现风险为主；§5.3/§5.4 只保留为历史基线。

### 5.7 40 组 serializer × compression 实测

八种 serializer 分别组合 raw、gzip-default、zstd-default、LZ4-fast 和 Snappy block；40 组
都执行 decompress、deserialize 和 token-count/fidelity 检查。下表是稳定的 artifact byte
count：

| Serializer | raw | gzip | zstd | LZ4 | Snappy |
|---|---:|---:|---:|---:|---:|
| JSON | 76,082,769 | 3,685,696 | 1,586,609 | 3,977,666 | 8,032,964 |
| gob | 26,080,786 | 1,248,288 | 998,432 | 1,760,914 | 3,234,464 |
| CBOR | 50,801,048 | 2,228,607 | 1,397,502 | 2,660,898 | 5,386,938 |
| MessagePack | 52,391,776 | 2,317,748 | 1,401,205 | 2,718,258 | 5,579,724 |
| protobuf | 27,148,573 | 1,423,874 | 1,076,011 | 1,893,492 | 3,650,569 |
| FlatBuffers | 55,031,504 | 9,809,966 | 9,567,992 | 15,184,182 | 17,080,324 |
| Cap'n Proto | 49,889,480 | 5,093,517 | 4,291,842 | 8,996,051 | 10,664,996 |
| custom binary | 23,140,200 | 1,183,317 | 958,686 | 1,684,400 | 2,974,969 |

common token DTO 高度重复，因此 gzip/zstd 对多数 row 的压缩率显著好于
FlatBuffers/Cap'n Proto；这不能推出最终 Schema DTO 的相同比率。完整矩阵的单次 decode
观测也对 GC、cache warmth 和大 buffer copy 敏感，不用于建立跨 serializer SLA。为校验 v1
候选，gob 五种组合又在独立进程各跑 3 次：raw/gzip/zstd/LZ4/Snappy 的 p50 分别约
175.2/172.8/160.7/169.9/184.3 ms。差异不足以覆盖进程级噪声，而且与更贴近目标对象的
§5.4 prototype 相反：后者的 raw gob 完整 file hit 是 118.0 ms，gzip gob 是 141.4 ms。
因此压缩矩阵只证明各组合可 round trip 并量化该 fixture 的 artifact size；最终 raw 决策以真实 Registry/Meta
路径为准，最终 private DTO 仍须做 cold-file、peak RSS 和 CPU profile gate。

### 5.8 完整 Registry mirror 实测

token proxy 不能决定生产格式，因此又为完整 `SchemaRegistry` 建立 generated protobuf mirror，
递归覆盖 §6.2 的全部字段。protobuf schema 不使用 map/Any/Struct；所有 map 转 sorted repeated
entry，所有 optional、nil/empty slice 和 RawMessage 使用 presence message。每种格式 decode 后
显式转换回真实 `cli.SchemaRegistry`，通过 `reflect.DeepEqual` 和 public payload equality，再执行
真实 `Registry.Index()`。直接 gob runtime model 未通过 DeepEqual，因此不作为可上线候选。

JSON/gob/CBOR/MessagePack 编码同一个 protobuf-shaped complete mirror，protobuf 使用 modern
generated API 和 deterministic wire。每个 serializer 独立进程执行，headline 路径只运行
`decode → exact conversion → Registry.Index()`，`-benchtime=3x -count=10`；`Index()` 内部执行
`ToolSpec.Validate`、identity/provenance validation 和 navigation map build，但不包含 envelope/
artifact-shape validation：

| Serializer | Artifact | Decode + conversion + Index p50 | B/op | allocs/op |
|---|---:|---:|---:|---:|
| generated protobuf | 14,920,723 B | 105.8 ms | 86.8 MB | 1.578 million |
| gob over the same mirror | 14,805,769 B | 135.8 ms | 133.6 MB | 1.885 million |
| canonical CBOR over the same mirror | 21,990,234 B | 144.8 ms | 87.2 MB | 1.602 million |
| MessagePack over the same mirror | 25,494,810 B | 166.8 ms | 90.8 MB | 1.768 million |
| JSON over the same mirror | 27,129,939 B | 282.0 ms | 87.3 MB | 1.580 million |

protobuf 相对同一 mirror 的 gob p50 快约 22%，累计分配少约 35%，artifact 只大 114,954 B；
相对旧 direct-runtime gob proxy 的 118 ms/94.7 MB 也更快、更省分配，而且通过 exact fidelity。
gob mirror 使用 generated message pointer graph，不代表手写 pointer-free gob DTO 的最佳值；
因此该数据不能证明 protobuf 击败所有可能的 gob 布局，但足以否定“direct gob 的 118 ms 已证明
优于 protobuf”这一旧依据。v1 格式仍固定为 protobuf；Phase 1 的可复现 selected-path gate 只
确认 protobuf 是否达标，失败时阻止 enabled-target 发布，不在实现 PR 中重新打开 gob 选择。

同一 Registry protobuf artifact 的 raw/gzip/zstd/LZ4/Snappy 大小分别为
14,920,723/1,310,234/929,284/1,645,498/2,387,203 B。压缩路径的 decode + Index 受当前机器
thermal/GC 干扰，没有得到可重复的 ≥25 ms latency 优势；五种 codec 均通过 exact round trip，
但 codec-stage compressed path 因 materialize 14.92 MB uncompressed buffer，观测累计分配约
101.7-110.1 MB，raw codec-stage 为约 86.8 MB。该比较未包含 raw file-read buffer，不能外推为
完整 file-hit 内存差。v1 仍以无新增 decompressor/攻击面为由选择 raw，不声称当前 prototype
已经证明 raw 完整命中更快；实现 PR 必须用相同 open/read/hash/validation accounting 的受控
warm/cold file benchmark 复核，若 bounded codec 稳定达到 §5.3 的撤销条件再提交新 codec RFC。

完整 Meta mirror 也补齐 `Prerequisites`、`Tips`、exact no-argument Overview projection、31 个
product descriptors 和 primary-path/alias lookup 构建。每个 serializer 独立进程执行
`-benchtime=10x -count=10`：

| Meta serializer | Artifact | Decode + complete lookup p50 | B/op | allocs/op |
|---|---:|---:|---:|---:|
| restricted-shape gob | 718,847 B | 1.29 ms | 3.25 MB | 36,948 |
| generated protobuf | 718,182 B | 1.44 ms | 2.44 MB | 33,057 |

Meta 上 gob 快约 0.15 ms，protobuf 分配低约 25%。该 p50 是 benchmark run average 的中位数，
不是 invocation-level percentile；在未锁频、未隔离系统进程的环境中，0.15 ms 小于可据以引入
第二种 parser 的可信差异。protobuf 的 1.44 ms codec-stage 只说明预算可行，完整
open/read/hash/envelope 路径仍必须通过 §8.4 gate。

### 5.9 serializer 决策

v1 首选实现候选调整为 **Meta 与 product-sharded Registry 均使用 generated protobuf + raw**。Registry 完整
mirror 的方向性证据同时显示更低 latency 和 allocation；Meta 统一使用 protobuf，以少约 0.15 ms
的未隔离 benchmark 差异换取单一 parser/format、更低 allocation 和一致的 schema/version gate。
Meta 和 Registry shard-data artifact 都远低于 payload 上限，且不需要 decompressor。

custom binary 在 token proxy 上最快、最小，但需要仓库自行拥有 parser 安全、字段演进、跳过
unknown field、fuzz corpus 和跨版本合同。FlatBuffers/Cap'n Proto token decode 很快，但尚未实现
完整 Registry mirror，不能声称它们已胜过 generated protobuf 的 105.8 ms 真实路径。若 selected
path 不能满足 §8.4，当前实现必须保持 persistent cache disabled；后续 RFC/envelope version
可按“FlatBuffers generated DTO → custom binary”的顺序复评，并包含 exact conversion、Index、
peak RSS、binary size 和 fuzz/security 成本，不能只引用 token decode 数字。

### 5.10 客户端 CPU 与内存压力

当前 benchmark 是单进程、单次命中，不代表持续吞吐 SLA，但足以判断一次 CLI 调用的
burst。gzip stdlib 是单线程，因此不会占满多核，但会在当前 goroutine 上形成短时单核 CPU：

| 路径 | 当前 proxy 观测 | 客户端压力判断 |
|---|---|---|
| 首次 `ResolveMeta` protobuf codec-stage proxy | p50 1.44 ms；2.44 MB allocated | 已含 typed overview 和 31 个 product descriptors；不含 open/read/hash/envelope；随后是零分配 map lookup |
| Registry raw protobuf codec-stage proxy | p50 105.8 ms；86.8 MB allocated | 完整 Registry 对照；包含 generated decode、exact conversion、validation/Index，不含 open/read/hash/envelope |
| selected product-shard file-hit prototype | target p50 约 4.35 ms；3.80 MB allocated | 包含 authenticated 1,857 B locator、`pread` 549,488 B、两段 SHA-256、decode/conversion、product-local validation/Index/lookup |
| rejected tool-offset file-hit prototype | target p50 约 1.06 ms；1.06 MB allocated | 比 product shard 再省约 3.3 ms，但增加 2,739 条 path locator、tool 粒度发布/组合合同和另一套 product/group/all 逻辑 |
| Registry raw gob mirror（未选择） | p50 135.8 ms；133.6 MB allocated | protobuf 相对它 latency 降低约 22%；它相对 protobuf 累计分配高约 54% |
| Registry raw typed JSON prototype hit（未选择） | p50 254.3 ms；77.5 MB allocated | JSON 对象构造与 Index 主导，CPU 高于 protobuf/gob |
| Registry gzip prototype hit（未选择） | p50 264.4 ms；78.8 MB allocated | 相对 raw 约 +10 ms；gzip 自身 decode 约 24.6 ms，较小文件读取抵消部分 CPU |
| cold authoritative assembly | 当前命令约 2.37 s user CPU、380 MB peak RSS | 声明装配是主成本；raw cache repair 不支付压缩 CPU |

allocated bytes 是累计分配量，不等于同时驻留内存；实现 PR 必须同时报告
`allocs/op`、`B/op`、peak RSS 和 CPU profile。cache 无后台线程、无后台刷新；每个进程最多
一次 loader/repair，因此不会产生持续 CPU。release benchmark 需要确认 protobuf、validation、
Index 和 GC 各自占比；如果 JSON/GC 仍显著，可先减少 DTO allocation，再考虑
替换序列化协议。
Meta 和完整 Registry mirror 数字都是 codec-stage prototype，不包含 open/read、208 B envelope、
binary-pinned SHA-256 或 secure-file checks。product/tool shard 行是 file-hit prototype，包含普通
`os.Open`/read/hash，但仍不包含最终 `openat` ownership/mode 检查和 208 B envelope parser；实现 PR
必须再次证明 §8.4 的完整 cache-hit 预算。

### 5.11 GWS 与 Lark CLI 的实际选择

对比对象只用于验证工程取向，不直接复用性能结论：GWS 是 Rust/Serde，Lark CLI 的 metadata
规模也与 DWS 不同。2026-09-04 又对真实安装入口做了独立子进程测量。DWS 使用当前 worktree
构建的 arm64 `version=dev` binary；Lark 使用 `/opt/homebrew/bin/lark-cli` v1.0.85；GWS 使用临时、
非全局安装的官方 `@googleworkspace/cli` v0.22.5。leaf 对比选择三者语义相近的 recurring-event
instances Schema，并完整生成输出后丢弃：

| 路径 | 样本 | 稳态 wall p50 | CPU p50（user + sys） | 输出 |
|---|---:|---:|---:|---:|
| DWS `--version`（native） | 30 | 251 ms | 91 + 59 ms | 16 B |
| DWS `--help`（native，稳定复跑） | 10 | 253 ms | 93 + 63 ms | 4,833 B |
| DWS compact leaf Schema（native） | 10 | 1,892 ms | 1,906 + 155 ms | 2,252 B |
| Lark leaf Schema（npm public shim） | 30 | 122 ms | 84 + 27 ms | 19,486 B |
| Lark leaf Schema（native，稳定复跑） | 30 | 78 ms | 59 + 18 ms | 19,486 B |
| GWS leaf Schema（npm public shim） | 30 | 149 ms | 42 + 24 ms | 6,966 B |
| GWS leaf Schema（native） | 30 | 9.8 ms | 3.7 + 3.8 ms | 6,966 B |

机器同时存在明显调度/内存压力：DWS 首轮出现 8-19 s，GWS public shim 复跑 p50 曾升到
377 ms，而 CPU 时间变化远小于 wall；因此本表是本机归因证据，不是可签 SLA。wrapper 的
`rusage` 也不包含其 native child，不能用来比较总 RSS。方向仍很清楚：当前 DWS compact leaf
约 334 MiB process RSS，且 compact/full leaf 都支付完整 Registry assembly；输出只有 2.2 KB
并未减少该成本。`DWS_PERF_DEBUG=1 DO_NOT_TRACK=1` 的一次观测为 `cmd_init=599 ms`、
total `2.385 s`，说明 Schema assembly 之外仍有独立启动成本。

| 客户端 | 本地 Schema/Discovery 格式 | 压缩 | 完整性/加密 | 与 DWS 的关系 |
|---|---|---|---|---|
| Google Workspace CLI `gws` v0.22.5 | Google Discovery JSON，经 `serde_json` typed parse；缓存为 `{service}_{version}.json` | 无 | Discovery cache 无 checksum/MAC/encryption；credential/token 的 JSON plaintext 才使用 AES-256-GCM 加密后落盘 | 证明本地 Discovery 默认优先 raw JSON；其 Rust decode 数据不能代替 Go/DWS benchmark |
| Lark CLI v1.0.85 | embedded `meta_data.json`；runtime `remote_meta.json` 经 typed `encoding/json` parse | 无 | metadata cache 无 SHA/MAC/encryption；token 使用 macOS Keychain/AES-GCM、Windows DPAPI 等独立保护 | 支持“不压缩、credential 单独加密”；DWS 因规模/CPU 目标对 Registry 改用 generated protobuf |

Lark 代码证据包括 `internal/registry/loader_embedded.go`、`internal/meta/meta.go` 和
`internal/registry/remote.go`；其 protobuf 依赖只用于 WebSocket binary frame，frame 内业务
payload 仍是 JSON，不用于 metadata cache。GWS 的 discovery 路径见上游
`crates/google-workspace/src/discovery.rs`，token/credential 加密见
`crates/google-workspace-cli/src/token_storage.rs` 和 `credential_store.rs`。本机 mule-run
只安装 npm 包 `@googleworkspace/cli` 并由 wrapper 转发，未 vendoring GWS Rust source。

两者都没有为公开 Schema/Discovery cache 支付压缩或加密 CPU。DWS 同样不压缩，但为更低
解析 CPU 使用 generated protobuf，并额外做 binary-pinned SHA-256；DWS cache 参与
safety/confirmation 决策，不能仅靠“格式能解析”接受用户可写内容。

### 5.12 Registry 分片实测与粒度决策

完整 mirror 上进一步构造了三种 raw deterministic protobuf file-hit 路径。所有路径都从同一
sorted Registry 生成，执行 exact SHA-256、generated decode、runtime conversion、typed validation
和 lookup；product/tool shard 均通过逐 product、逐 tool `reflect.DeepEqual` round trip，并验证
locator 的 offset/length/hash、requested lookup path 与 decoded identity 的绑定：

| 路径 | Artifact/target | p50 | B/op | allocs/op |
|---|---:|---:|---:|---:|
| full Registry：read + hash + decode + global `Index()` | 14,920,723 B | 114.2 ms | 101.73 MB | 1.578 million |
| selected：authenticated product index + `pread` product shard | 1,857 B index + 549,488 B target | 约 4.35 ms | 3.80 MB | 58,883 |
| tool index + `pread` tool shard | 284,221 B index + 13,164 B target | 1.06 ms | 1.06 MB | 15,218 |
| preloaded tool index + `pread` tool shard | 13,164 B target | 0.20 ms | 92.0 KB | 1,503 |

31 个 product shards 合计 14,927,893 B，只比 monolithic Registry 多 7,170 B；每个 shard 都
保留 exact `AgentMetadata` presence/content，product shard p50 为 268,599 B，最大 2,660,805 B。
1,357 个 tool blobs 合计 14,859,043 B，但 path locator
因为 canonical/path/CLI/alias 展开有 2,739 entries、284,221 B。表中 p50 是未隔离机器上的
benchmark-run 中位数；文件在 setup 后立即重复读取，因此是 warm page-cache file hit，不是 cold
disk。长时间系统竞争会放大 wall，但 product/tool 相对 full 的数量级优势不变。tool path 信任
已认证 locator，只校验目标 Tool 和 lookup identity，不重建全局 collision maps；其更低数字本来就
来自减少工作，release gate 必须另行证明完整 locator 与全局 Registry index 精确对应。

v1 选择 **product shard**，并把 31 条 sorted product descriptors 作为 `SchemaMetaCache` 的
cache-private locator 部分。Meta 的每个 command entry 已含 `product_id`，leaf 查询不需要第二套
path index；product/group 读取一个 shard，`--all` 顺序读取所有 shards。Registry data 使用一个
concatenated file，descriptor 保存 product ID、offset、length 和 SHA-256；reader 只把已认证
slice 交给 protobuf parser。4.35 ms 是 Meta 已解析后的 Registry stage；首次进程还需支付 Meta
完整 file hit，现有 codec-stage 为 1.44 ms、最终预算为 5 ms，因此 selected 数据层合计预算为
20 ms，而不是把 4.35 ms 误写成完整子进程时间。

tool-offset 不进入 v1。它相对 selected product path 只再省约 3.3 ms，却需要重复的 path locator、
tool-level identity/publish contract，并让 product/group/`--all` 组合使用另一套路径。更重要的是，
当前 DWS 非 Schema 基线自身约 250 ms；把 Registry 阶段从秒级降到 4.35 ms 后，启动/`cmd_init`
成为主成本。仅换成 tool shard 不能让 DWS 稳定超过 Lark/GWS。若后续先把进程和 root 初始化
压低到同一数量级，且 product shard 在隔离 E2E 中成为可见瓶颈，再通过新 DTO version 复评
tool offset。

### 5.13 启动成本归因与后续 fast path

2026-09-05 在同一 worktree 上补做了四个逐步扩大 workload 的进程入口实验。它们使用本机
Go 1.26.1 默认 `CGO_ENABLED=1`、`-buildmode=exe`，未使用 `-trimpath`/`-s -w`，也未附加
runtime payload，不是正式发布 binary。各 executable 的 linked dependency set 和大小不同，且
没有交错执行，因此结果不能相减为单一阶段的精确成本。机器当时为 12 logical CPU、load
average 约 20，且存在明显 swap/compressor 压力；下表只作为架构方向证据，不能替代 §8.4
的 release-built、隔离、交错门禁。runner 对一次未计入稳态的 first run 后顺序执行 12 个
subprocess，通过 `ProcessState.SysUsage()` 记录 user/sys CPU 和 maximum RSS：

| 原型 | 静态依赖/执行内容 | CPU p50（user + sys） | maximum RSS p50 | wall p50（受压机器） |
|---|---|---:|---:|---:|
| bare Go process | 只输出常量 | 1.4 + 2.0 ms | 4.1 MiB | 39.8 ms |
| synthetic Meta reader | `os.ReadFile` + SHA-256 + 905,436 B synthetic protobuf decode + sorted lookup | 3.1 + 3.1 ms | 8.5 MiB | 29.0 ms |
| import `internal/app` only | 执行完整静态 import/init，但不建 Cobra tree；直接 `RawVersion()` | 12.8 + 7.2 ms | 26.6 MiB | 94.2 ms |
| direct `app.ExecuteWithTelemetry --version` | app 入口和 Cobra construction；绕过 official `cmd/main` identity/`clitrack` wrapper | 101.9 + 82.4 ms | 45.5 MiB | 608.1 ms |

synthetic Meta reader 的 fixture 不是 §5.8 的最终 718,182 B Meta DTO。它没有 208 B envelope、
binary-injected expectation、secure-file checks、DTO-to-runtime conversion、typed validation、
alias map、product read、renderer、telemetry 或 fallback；digest 也通过 argv 传入。它只能证明
小依赖图下 read/hash/protobuf/lookup 的 process workload 很小，不能作为 selected cache path
端到端数字。

为直接验证同 binary 方向，随后把同一个 synthetic reader 与 `internal/app` 同时链接，并用
`CGO_ENABLED=0 -buildmode=pie -trimpath -s -w` 构建三个未附加 runtime payload 的
release-like probes。在系统出现一个较稳定的时间窗后，每个 probe 同样先执行一次不计入稳态的
first run，再顺序执行 20 次；三组没有交错，仍不是竞争性门禁：

| release-like probe | binary size | wall p50 | CPU p50（user + sys） | maximum RSS p50 |
|---|---:|---:|---:|---:|
| import `internal/app` only | 33,528,674 B | 14.7 ms | 10.8 + 4.7 ms | 27.8 MiB |
| synthetic Meta only | 4,162,162 B | 6.5 ms | 2.7 + 2.4 ms | 8.2 MiB |
| `internal/app` + synthetic Meta read/hash/decode/lookup | 39,422,178 B | 22.5 ms | 15.2 + 7.7 ms | 31.1 MiB |

combined probe 仍然缺失正式 runtime payload、official `cmd/main` telemetry、最终 envelope/DTO、
product shard 和 renderer，不能直接与 Lark/GWS public wall 排名。它提供的结论更窄：在接近
release linker/build flags 的单个大 Go executable 中，静态链接 `internal/app` 没有吞掉轻量
Meta fast path 的数量级优势，因此应先验证同 binary 方案，而不是先承担双 binary 交付复杂度。

一轮未保留 raw log 的 `GODEBUG=inittrace=1` 观测在约 19 ms 进入 `main`；先前高负载下约
112 ms 的 init wall 不能当作纯 CPU。初步 attribution 指向 doc/i18n JSON、Schema reviewed
inputs、shortcut semantic catalogs 和各 product shortcut package。另一次本地
`BenchmarkNewRootCommand` 约为 39.1 ms/op、17.1 MB/op、156,477 allocs/op，初步 profile 热点为
`corecmd.New`、shortcut conversion、pflag registration、Contract clone 和 annotations；实现
PR 必须保留 raw benchmark/profile 后才能把这些数字升级为可复现基线。

一轮同样在受压机器上顺序执行 12 个 subprocess 的本地比较中，仅增加 `-s -w` 把临时
binary 从 50,750,066 B 降到 40,251,826 B，但 `--version` CPU p50 从约
81.2 + 59.9 ms 变为 82.1 + 58.3 ms，maximum RSS 都约 49 MiB。该 comparison 尚未作为
repository benchmark 保留，不能声称 strip 绝对无效；它至少没有提供“只减小符号即可解决
当前数量级问题”的证据。另一个按正式 `CGO_ENABLED=0 -buildmode=pie -trimpath -s -w`
recipe 但未附加 runtime payload 的构建也受同一系统压力污染，不能用于发布性能排名。因此
不能用 strip、压缩 Schema 或继续微调 protobuf 代替消费边界调整。

本节原型是历史归因实验，不再决定产品形态。§1.1 已选择受限热路径 runtime；能力和依赖只以 §1.2 为准。同 binary 原型保留为替代方案的历史材料，不构成当前实现可自行改选 B 的授权。

历史上将“DWS public/native 同时领先 Lark/GWS”作为本 RFC 门禁的条款在 **2026-09-06 废止**。竞争对比继续提供诊断数据；它不能代替本 RFC 的固定 pre-PR 回归预算，也不阻挡符合 §8.4 的 release。不得用本文的 release gate 通过宣称竞争性领先。

一个不含 Schema、只处理 exact `--version` 的 release-like launcher prototype 已完成 100 次
随机交错子进程对比：DWS packed prototype p50/p95 为 4.22/6.51 ms，GWS native 为
8.23/10.75 ms；DWS maximum RSS p50 约 4.4 MiB，GWS 约 8.8 MiB。该结果尚非签名/notarized final
artifact，也不证明 Schema path 达标，只说明该历史 prototype 的启动成本；不能代表当前默认上报的热路径 runtime。

### 5.14 GWS、Lark CLI、Codex 与 Grok Build 的交付/签名对照

2026-09-05 对四个真实实现及本机 release artifact 复核后，没有一个把第二个完整 executable
append 到已签名的主 Mach-O 再运行时提取。DWS 的 EOF payload prototype 即使先移除 Go linker
ad-hoc signature，重新 `codesign` 仍报 `main executable failed strict validation`；该方向拒绝。

| CLI | runtime 拓扑 | macOS 签名/公证 | 可复用结论 |
|---|---|---|---|
| GWS 0.22.5 | npm tiny installer/wrapper + 单个 15.4 MB Rust binary | 仅 linker ad-hoc；无 Developer ID/notarization，`spctl` 拒绝 | 只能作为启动性能对照，不能作为 DWS release security 模板 |
| Lark CLI 1.0.93 | npm wrapper + 单个 Go binary；metadata baseline 编译进 binary | GoReleaser Developer ID + hardened runtime + timestamp + App Store Connect API notarization；双架构 final archive 原生复核 | 复用 credentials/notarization/final-artifact gate；它没有 multi-executable signing 问题 |
| Codex 0.153.4 | versioned canonical package，含 `bin/codex`、独立 host/rg/zsh/bwrap 等 sidecars | 每个 Mach-O 分别 Developer ID 签名；按 executable entitlements；分别/按 package 提交 notarization，final package 复核 | DWS launcher/core 应采用此 multi-executable 模型，不做 executable embedding |
| Grok Build 1.0.x | 单个大型 Rust binary；immutable version downloads + `grok`/`agent` 相对 symlink 原子切换 | Developer ID + hardened runtime + timestamp；现有材料不能证明 notarization | 复用 version directory、下载后 smoke、双链接事务切换、保留上一版本；补强其仅 HTTPS/smoke 的完整性不足 |

因此启动层固定采用 canonical package，而不是单文件 core container：

```text
<version-root>/
  package-manifest.json
  bin/dws                 # 受限热路径 runtime
  libexec/dws-core        # 当前完整 Go app，内部仍携带 native runtime payload
```

- launcher 和 core 使用同一 version/commit/edition proof；launcher 编译进 exact signed core
  SHA-256/size，release manifest 同时记录两个最终文件 digest。
- macOS 顺序为：签 native dylib → 注入 core runtime slot → Developer ID 签 core → build/inject
  core identity 到 launcher → Developer ID 签 launcher → 将两者放入 notarization ZIP 并等待 Apple
  Accepted → 用最终 tar.gz 中的两个文件分别执行 `codesign --verify --strict`、hardened-runtime、
  timestamp、TeamIdentifier 和 `codesign --check-notarization` gate。raw CLI 不声称可 staple。
- 当前 `rcodesign sign --for-notarization` 只表示兼容模式，不表示已公证。release 必须增加
  `rcodesign encode-app-store-connect-api-key` 和 `rcodesign notary-submit --wait`，缺少 issuer/key/P8
  的官方 release fail closed；fork/local build 保持 ad-hoc 且明确标记 unnotarized。
- 保留已发布 flat upgrader 的迁移合同：platform archive 根目录另有 `dws`（Windows 为
  `dws.exe`），其字节、mode、size 必须与最终 canonical core 一致。旧 `FindBinaryInDir`
  优先选择这个根文件，单文件替换后仍可运行；不得让旧升级器独立安装 launcher。
  新安装器不把这个重复文件作为 package 成员，只在 archive extraction 后与已校验的 core
  比对，随后安装完整 tree。release verifier 要求根迁移入口存在；篡改、symlink 和错平台名称
  均拒绝。这会增加 archive 体积，最终大小/下载测试必须计入，不能把重复 core 隐藏在统计外。
- standalone 安装先把完整 package 写入 immutable version directory，校验 archive、manifest 和
  两个文件后，再原子切换 `current`/公开 `dws` symlink；不覆盖正在映射的 macOS executable。
- Homebrew 使用 Cellar 的版本目录；npm 使用 OS/arch platform package 承载完整 tree，先发布所有
  platform package 再推进 root wrapper；Windows 使用 version directory + stable launcher，更新不
  覆盖正在运行的 core。
- Unix launcher 通过 `execve` 保持 PID/stdio/signal/exit；Windows spawn/wait。core 中 upgrade、
  rollback 和 executable-relative config 必须使用受 package-layout 约束的 launcher identity；daemon
  和内部 subprocess 继续直接使用 running core。
- launcher 已处理的快路径不打开 core；所有委派路径（包括默认 Schema）在执行 core 前都完整校验其 size/SHA-256。安装时验证、不可变目录和 OS 签名不替代逐次委派校验。本 RFC 不引入 mtime/size 验证缓存，也不跳过哈希来满足延迟预算；该成本按 §8.4 单列诊断。

## 6. 详细设计

### 6.1 两条消费路径

```text
ResolveMeta / leaf help
  └─ metaLoader sync.Once
      ├─ valid meta.cache hit → verify raw bytes → generated protobuf → lookup
      └─ miss → repairCoordinator.repair()

dws schema ...
  └─ metaLoader
      ├─ no-argument overview → authenticated SchemaOverviewCache projection（不读 Registry）
      └─ leaf/product/group → product_id + authenticated shard descriptor
  └─ registryShardLoader
      ├─ valid registry.shards hit → pread product range → verify range → generated protobuf
      │    → exact conversion → product-local Registry.Index
      └─ miss → repairCoordinator.repair()

repairCoordinator.repair()                  # 全进程唯一 repair owner
  ├─ acquire bounded process/file lock
  ├─ low-level re-read；禁止调用 metaLoader/registryShardLoader
  ├─ meta hit + matching registry shard → 直接返回
  └─ 否则 registered source-root factory
       └─ NewSchemaSourceRootCommand
       └─ ResolveSchemaBuild exactly once
       └─ validate exact expected identity/artifact bytes
       └─ publish registry.shards first，then meta.cache as commit marker
```

`ResolveMeta` 不再通过 `deliverySchemaCatalog()` 间接获得 map。它使用独立的
`deliveryCommandMetaIndex()`，但两个 artifact 只能由同一个 resolved Registry 生成。

generated `SchemaMetaCache` protobuf v1 必须覆盖：

- `CommandIdentity` 全字段；
- `CommandSafety` 全字段；
- `CommandSelection.AgentSummary`；
- `UseWhen`、`AvoidWhen`、`Prerequisites`、`Tips`、`Examples`。
- `SchemaOverviewCache`：Registry kind/level/source/AgentMetadata、tool count，以及
  `SchemaRegistry.ToOverviewPayload()` 中每个 product entry 所需的 id/tool_count/schema_path 与
  agent_summary/use_when/description fallback；它不包含 tool summary、leaf parameters 或 full tools。

generation gate 比较 DTO lookup 与 `buildMetaByCLIPathFromRegistry`，包括 alias 展开后的
exact map equality，并比较 Overview DTO wire 与 live no-argument overview byte-equivalent。Meta
payload 另含按 product ID 排序的 shard descriptors 和 Registry data
总长/aggregate digest；descriptor 的 offset/length 必须无重叠、无空洞并精确覆盖 data file。
这些 locator 只描述同次 Registry projection，不是第二份声明。

### 6.2 缓存 payload

Registry data file 按 product ID 排序，连续拼接 generated private `SchemaProductCache` protobuf
shards；它不编码 `SchemaIndex` 和 `SchemaCatalogSnapshot` 的 `map[string]any`，也不再保存一个
重复的 monolithic `SchemaRegistryCache`：

- 每个 descriptor 的 product ID、offset、length、SHA-256 位于已认证 Meta payload；
- leaf/product/group 只对一个 product shard 通过 `Registry.Index()` 重建局部 index 并校验；
- `schema --all` 逐 shard 认证/解码，拼成一个 Registry 后执行一次 global `Index()`；
- Catalog/Tools JSON maps 是 wire projection，重新缓存会恢复 39 MB JSON 和高分配 adapter；
- product shard 的 protobuf root 仍携带恢复 public wire 所需的 Registry envelope 字段；所有 shard
  的 envelope 字段必须完全相同；
- public `catalog_hash` 和 `surface_hash` 在 cache hit 时直接使用 binary-pinned expected
  values；DTO bytes 与这两个值的关系只由同次生成的 release-time projection gate 证明；
- protobuf 使用显式 `.proto` field number 和 typed message，不直接把 public wire map 或 runtime
  Go struct 布局当持久格式；
- pointer、nil/empty collection 和 optional field 必须通过 exact round-trip gate。

Meta cache 编码 generated `SchemaMetaCache` protobuf DTO v2，包括完整 CommandMeta projection、typed
Overview projection、Registry data 总长/aggregate digest 和 sorted product descriptors。两个
artifact 都不能包含 endpoint、token、tenant 或用户数据。

DTO mirror 的覆盖范围是规范性的，而不是实现时自行裁剪：product Registry DTO 的并集必须递归覆盖
`SchemaRegistry`、`ProductSpec`、`ToolSpec`、`ParameterSpec`、`RuntimeSchemaConstraints`，
以及 Tool 引用的 contract identity、positionals、dry-run、result、pagination、safety、
interface、selection 和 provenance 全部字段；Meta DTO 必须覆盖 `CommandMeta.Identity`、
`Safety`、`Selection`、Overview product summary/envelope 和 product descriptor 全部字段。private
protobuf 不使用 protobuf map/Any/Struct。optional 与 collection 通过 presence message/wrapper
恢复 nil、empty 和 zero-value 的区别；Meta v2 的六个列表使用 §6.2.1 的显式存在位，`*bool` 映射为 optional `BoolValue` message 以区分
nil、false、true。

所有 runtime `json.RawMessage` 位置使用统一的 private wrapper：Registry `AgentMetadata`，Parameter
`Default`/`InterfaceDefault`/`Example`，Result `DataSchema`，以及 provenance winner/candidate
`Value`。protobuf 使用 message presence + bytes：wrapper absent 恢复 nil，wrapper present 且
`value="null"` 恢复语义 JSON null。
DTO conversion 必须 deep-copy RawMessage 和 collection backing storage。新增字段必须 bump
kind-specific DTO version；release test 对整个 runtime model 使用 `reflect.DeepEqual` 加 public
wire equality，不能只比较 hash、tool count 或 `Index()` 成功。

为避免新增 runtime 字段在当前 release fixture 中恰好为 zero/nil 而逃过 round trip，CI 还必须
维护 machine-checked field inventory：递归反射允许进入 cache 的 runtime type graph，输出稳定的
fully-qualified field path/type/presence-kind 清单，并与 protobuf descriptor/conversion manifest
逐项双向比较。另有 all-fields sentinel fixture 为每个 scalar、optional、nil/empty collection、
RawMessage 和 provenance candidate 注入非零/两种 presence 值。新增、删除或改型字段若未同步
`.proto`、conversion、DTO version 和 sentinel，必须在编译/测试阶段失败。

### 6.2.1 Meta DTO v2：展开记录，保留完整语义

Linux DTO v1 的 Meta file-hit 在两轮原生反馈中分别为 7.530 ms、5.874 ms，均超过
5 ms；优化 map 拷贝已降低分配，但 protobuf 多层 message 分配与 GC 仍是主要成本。
隔离原型保留同一个 `protoc-gen-go v1.33.0` / protobuf runtime、全部字符串字段和列表存在性，
把每条 Meta 记录展开；在 1,370 tools 上能还原 byte-identical 的 v1 Meta，decode-only 的
7 轮中位数从 1.733 ms / 2.29 MB / 44,766 allocations 降至 1.426 ms / 1.88 MB /
34,953 allocations。该原型不是完整 file-hit 或 Linux 验收，正式 v2 必须重跑所有门槛。

当前私有 protobuf 包为 `dws.schemacache.v2`，root DTO version 与 envelope 的 DTO format
version 均为 `2`。208 B envelope 布局、magic/envelope version `1`、raw protobuf serializer、
cache 容器目录 `v1` 和公开 Catalog snapshot version 不变。旧缓存先在 envelope format
检查被拒绝，然后走既有权威装配修复；不尝试解释或迁移旧 payload。BuildID 已包含这些版本、
`.proto`、generated code、descriptor 和实际 artifact digest，v1 identity 不能授权 v2 数据。

`CommandMetaEntry` 本身承载完整 Identity/Safety/Selection 值，避免仅用于中转的嵌套对象。
旧字段号 `2` 和名称 `meta` 保留为 reserved，新 scalar/list 使用独立 field numbers；
unknown/reserved fields 仍被标准 protobuf 解码后的 visitor 拒绝。Identity 的 CLI/canonical/
product 非空、alias 展开、locator、overview、descriptor、选中 shard 的完整逐字段一致性校验
全部保留。Safety/Selection 在 runtime model 中本来就是值类型，记录存在即代表这两个值，
不会把空 safety 值推断成其他语义，也不省略 Selection 字段。

六个 repeated string 列表依次为 aliases/use_when/avoid_when/prerequisites/tips/examples；
`lists_present` 的 bits 0–5 逐一标记存在性。0 bit 恢复 nil，1 bit 且没有元素恢复非 nil 的
空 slice，1 bit 且有元素复制全部内容及顺序。非空列表却没有 bit、以及 bits 6–31 非零都失败。
转换结果继续持有独立 backing storage。729 种 nil/empty/nonempty 组合通过真实 protobuf
wire round trip；另测 bit 顺序、生成对象修改后的隔离、旧 DTO/旧字段号和非法 bit 拒绝。

该变更没有引入第二套 parser、字符串 intern/pool、跳过 UTF-8 或减少认证/语义校验。
DTO v2 的 performance/native/final-artifact 证据必须独立记录，不能沿用 v1 的绿色结果。
最新 alias validator 只比较 alias 与其 primary，省去 primary 与自身的重复逐字段比较；
expected alias 集合逐项验证加相同条目数，继续排除缺失/额外 alias，并通过原有 1,400 组
独立旧算法对照。产品条目计数与既有 locator 检查同次遍历完成。

本机 `protoc-gen-go --check` 再生成时子进程被系统 SIGKILL，未获得 drift 通过结论。
native feedback 现从官方 v35.1 release 下载并校验固定 SHA-256 的各平台 protoc，再以
固定 Go/plugin 版本执行 drift check；该检查待新 head 原生运行。

v2 本机组件 race 通过（schemaruntime 25.712 s、schemacache 1.637 s、reader 1.499 s、
launcher 2.013 s）；完整 real delivery parity 通过 41.525 s，app identity 与 generator
定向测试通过。[独立 7 轮完整 file-hit](benchmarks/schema-cache/2026-09-06-darwin-arm64-dto-v2-file-hit.json)
中位数为 Meta **3.066 ms / 3.48 MB / 40,701 allocations**，selected 为
**5.182 ms / 4.32 MB**。报告绑定基线提交和确切 runtime 源文件摘要；这是工作树验证，
尚未证明 v2 的原生 Linux 5 ms 门槛或最终候选进程指标。

### 6.3 固定 envelope

每个文件使用 208 B 固定二进制 header，整数为 big-endian，hash 使用 32 B 原始 SHA-256
而不是带 `sha256:` 前缀的文本，随后紧接 payload：

| Offset | Size | Field | v1 rule |
|---:|---:|---|---|
| 0 | 8 | magic | ASCII `DWSSCHC1` |
| 8 | 2 | envelope version | `1` |
| 10 | 2 | header size | `208` |
| 12 | 1 | artifact kind | `1=meta`, `2=registry-product-shards` |
| 13 | 1 | serializer | Meta 与每个 Registry shard 均只接受 `2=deterministic-protobuf` |
| 14 | 1 | codec | v1 只接受 `0=raw` |
| 15 | 1 | flags | 必须为 `0` |
| 16 | 4 | DTO format version | 当前 Meta 与 Registry 均为 `2`；拒绝 retired `1` |
| 20 | 4 | Catalog snapshot version | 必须等于 binary expected value |
| 24 | 8 | encoded length | payload 精确长度 |
| 32 | 8 | decoded length | raw serializer payload 精确长度；等于 encoded length |
| 40 | 32 | edition SHA-256 | canonical edition name 的 digest |
| 72 | 32 | source SHA-256 | 去掉文本前缀后的 digest |
| 104 | 32 | surface SHA-256 | 去掉文本前缀后的 digest |
| 136 | 32 | Schema Build ID | §6.5 计算值 |
| 168 | 32 | encoded payload SHA-256 | Meta 为 protobuf digest；Registry 为 concatenated shard data aggregate digest；自描述副本不单独构成信任锚 |
| 200 | 8 | reserved | 必须全部为 `0` |
| 208 | encoded length | payload | Meta generated protobuf / concatenated product protobuf shards |

约束：

| 项 | 上限/规则 |
|---|---|
| meta 文件总长 | 4 MiB |
| registry 文件总长 | 64 MiB |
| registry shard data payload | ≤ 64 MiB - 208 B，即 67,108,656 B |
| magic/kind/serializer/codec/version | 必须精确匹配，不做猜测 |
| raw payload | encoded length 必须等于 decoded length；Meta 或目标 product slice 的 SHA-256 必须先匹配可信预期值，再允许对应 parser 解析 |

固定 header 只包含定长 primitive，解析它不会按输入分配。`208 + encodedLength` 使用 checked
arithmetic 并必须等于 regular-file `stat.Size()`，拒绝 overflow、截断和 trailing bytes。Meta
reader 读取二进制预期的 exact payload、校验 binary-pinned SHA-256 后才创建 decoder。Registry
reader 先接受已认证 Meta 中的 descriptor，再对同一个已安全打开的 fd 执行 bounded `pread`；只有
offset/length 在总长内、目标 range 完整读出且 range SHA-256 匹配时，才为该 product 创建 decoder。
leaf/product/group 不扫描或 hash 无关 shards；`--all` 必须逐 descriptor 验证每个 range。aggregate
digest 用于 generation/build identity 和 repair/full-audit，不替代 targeted range digest。

Meta 与 product shard 都使用固定 generated root，并在 unmarshal 后递归遍历每个 nested message，拒绝
任意 unknown field、unknown enum、重复/乱序 entry keys 和超出语义上限的 repeated/message 内容。
所有 runtime map 已转换为按 UTF-8 key 排序的 repeated entry；连续两次 identity generation
必须分别证明 Meta、每个 product shard 和 concatenated shard data bytes 完全相等。

### 6.4 可信 identity 与校验顺序

需要四个不同概念，禁止混用：

| Hash | 覆盖范围 | 用途 |
|---|---|---|
| `SchemaCatalogSnapshot.SourceHash` | public Catalog/Tools wire contract | 保持现有 Schema compatibility 和 `catalog_hash` |
| `SchemaCatalogSnapshot.SurfaceHash` | public identity/navigation surface | 作为独立 release/cache identity 维度，不能替代 SourceHash |
| Registry aggregate SHA-256 | sorted product protobuf shards concatenation 的精确字节 | generation/build identity 与 full-audit；leaf 不为它读取无关 bytes |
| Product shard SHA-256 | 一个 generated product protobuf range 的精确字节，descriptor 位于已认证 Meta | 在 protobuf 前认证目标 product range |
| Meta encoded SHA-256 | deterministic generated protobuf 的精确字节 | 在 protobuf 前认证 meta payload |

Meta 校验顺序固定为：secure open/regular-file 检查 → exact file/header/encoded length → header
字段与 binary expected identity 比较 → encoded payload SHA-256 与 binary expected digest 比较 →
generated protobuf decode → descriptor/DTO shape/semantic validation → runtime Meta。Registry target
校验顺序为：复用 authenticated descriptor → secure open/header/total length/Build ID → checked range
`pread` → range SHA-256 → product protobuf decode → exact conversion/product identity/typed validation →
product-local Registry/Index。任何一步失败均为 miss。

cache hit 明确禁止调用 `SchemaRegistry.ToSnapshotPayload` 或
`schemaCatalogSnapshotHash`。`loadedSchemaCatalog` 需要把 `SourceHash`/`SurfaceHash` 从当前
eager `Snapshot` maps 中拆成独立字段；命中时直接使用 binary-pinned 值。overview/leaf/all
按请求从 typed Registry 渲染并 stamp 这两个可信值，不为了验证 cache 预先构造完整
Catalog/Tools maps。authoritative miss 和 identity generator 仍构造 public snapshot 并计算
hash；release gate 证明该 snapshot、private DTO artifact digest 和 binary-pinned hashes 来自
同一个 resolved Registry。这样认证关系在构建期建立，运行时不恢复 2.32 s adapter。

header 里的 digest 是诊断用自描述副本。真正的信任锚是 release build 注入
`internal/app`、再通过注册 options 传给 `internal/cli` 的 expected value。攻击者只改 cache
文件并重算 header hash 不能得到命中；若同一用户可以替换 executable，则已超出本 RFC
的 cache-file threat model。

deterministic Meta/Registry protobuf schema/generated code/runtime 和 Go toolchain
共同构成 release 产物合同。generator、GoReleaser 和 runtime rebuild 使用同一个 pinned
Go 1.25.9 与 protobuf runtime。任一 encoder/toolchain/schema 变化产生不同 bytes 时，build ID
改变并 miss，不做语义复用。

### 6.5 Build ID

唯一实现位于 `internal/generator/cmd_schema_cache_identity`。固定测试向量先于发布启用，
shell 和 runtime 只消费生成结果，不另行拼接 preimage：

```text
sha256("dws-schema-cache-build-id-v1\x00" || TLV(field1) || ... || TLV(field21))
TLV = field_id uint16 big-endian || length uint64 big-endian || value bytes
```

field ID 严格递增且只出现一次。所有整数 value 都使用 `uint64 big-endian`，hash 使用
32 B 原始 digest；字符串采用 UTF-8，edition 必须通过 `[a-z0-9][a-z0-9._-]{0,63}`。

| ID | Value |
|---:|---|
| 1 | edition |
| 2 | envelope version |
| 3 | envelope DTO format version |
| 4 | private protobuf SchemaCache DTO version |
| 5 | Catalog snapshot version |
| 6 | serializer（两个 artifact 相同） |
| 7 | codec（两个 artifact 固定 raw，encoded/decoded length 相同） |
| 8 | SourceHash digest |
| 9 | SurfaceHash digest |
| 10 | Meta length |
| 11 | Meta encoded SHA-256 |
| 12 | concatenated Registry shard data length |
| 13 | Registry aggregate SHA-256 |
| 14 | product count |
| 15 | exact Go runtime/toolchain version |
| 16 | checked-in `.proto` SHA-256 |
| 17 | checked-in generated `.pb.go` SHA-256 |
| 18 | deterministic `FileDescriptorProto` SHA-256 |
| 19 | protoc version |
| 20 | protoc-gen-go version |
| 21 | protobuf Go runtime module version |

固定向量见 `TestDeterministicBuildIDFixedVector`；向量输入为该测试列出的常量，输出为
`29e35e4b63ca4ff32e617e6cf31b0db23f33f851773811b7681e43b875a59012`。
这是尚未上线的 v1 编码合同；修改字段、域分隔符或 TLV 宽度必须一起更新版本与测试向量。

version、commit、GOOS 和 GOARCH 绑定 native build proof，但不决定缓存内容身份：内容相同
的新版本可复用缓存。GOOS/GOARCH 只有在同源码、同工具链的 native artifact proof 相等时
才允许共享 identity。edition 同时进入 Build ID 和目录 partition。generator 拒绝与实际
compiled edition 不同的 `-edition`，并拒绝 `RegisterExtraCommands` 非 nil 的 overlay。

### 6.6 构建与发布接入

**选择“正式包与候选同构注入”，不采用长期内部专用或运行时版本开关方案。** 生效条件固定为：**首次包含本 RFC 实现的官方 release（prerelease 与 stable 均包含）**。对 v1 enabled targets `open × {darwin/arm64, linux/amd64}`，该次发布必须同时包含 core/launcher 的完整 Schema identity，以及绑定最终 core 的 launcher help snapshot；不得发布能力字段缺失的 enabled-target 包。proof 缺失/失败即阻止该 target 发布，不准退化为空字段后继续宣称支持；不得临时删除 enabled target 或改标 disabled 绕过此条件，支持矩阵变化须先改 RFC。

候选与正式包必须调用**同一份封装实现**：生成/核验 target proof → 注入 Schema identity 构建 core → 注入 runtime payload 并完成 core 签名 → 从声明生成绑定最终 core digest 的帮助模型 → 同时注入 core digest/size、Schema identity 和 help snapshot 构建 launcher → 签名/公证 → 在对应 target 的 native runner 上将最终 core 的 en/zh 输出与模型逐字节比较，并验证无 core 副本的 launcher 输出 → 对最终 bytes 执行 proof → 归档/解包再验。

native 候选在同一机器上连续完成模型生成、core 比较和 launcher 构建。正式发布的归档集中在 Linux 构建机产生；对于同机可执行的 Linux/amd64，比较可在 launcher 构建前完成；对于交叉构建的 Darwin/arm64，构建机必须把比较明确记录为 pending，随后由发布工作流中的 Darwin/arm64 native runner 在公开发布前完成。pending 不是通过，native proof 缺失或失败必须阻止发布。两条路径使用同一 identity、snapshot generator、ldflags builder 和最终制品 verifier；只允许 native 比较所在阶段因交叉编译移动，不允许分别维护 capability、模型或失败语义。

manifest 必须显式记录每个 target 的能力模式和禁用原因。v1 未启用的 target、non-open launcher 和普通 dev build 可以声明 disabled；这是预先限定的支持矩阵，不是同一 enabled target 上候选/正式行为分裂。不得拿 enabled candidate 的数据替 disabled 正式包背书。回滚只切换到上一份已验证的完整版本包，不能通过清空注入字段或混装 launcher/core 回滚，详见 §9。

新增 build-time identity generator，使用与生产相同的：

```text
app.NewSchemaSourceRootCommand
  → cli.ResolveSchemaBuild
  → one SchemaRegistry
  → sorted product protobuf shards + Meta projection/locator protobuf
  → SourceHash + SurfaceHash + exact shard-data/Meta artifact metadata
```

generator 只输出 hash、format 和 size 元数据，不输出或提交 Catalog blob。

官方 GitHub release 先在每个 enabled native runner 运行 generator 并产出 proof manifest。
manifest 必须绑定 GOOS/GOARCH、commit、exact Go toolchain、edition、build tags、`CGO_ENABLED`、
Schema-affecting ldflags 和 normalized build recipe digest；协调 job 验证 §6.6 前置 proof 后，才把
共同 identity 写入 `$GITHUB_ENV`，由 `.goreleaser.yaml` 通过 ldflags 注入对应 enabled target。
enabled target 的 proof 缺失或不一致即阻止发布；只有支持矩阵中明确 disabled 的 target 才使用空 capability 元数据。该步骤不写仓库文件，因此仍满足 sealed source clean gate。
workflow 和 `scripts/dev/build-all.sh` 必须显式设置
`GOTOOLCHAIN=go1.25.9` 并校验 `go env GOVERSION`；只依赖 `go.mod` 下限不够，因为本地更高
版本会被直接使用并可能生成不同 protobuf bytes。generator 还必须校验 checked-in generated
`.pb.go` 与 pinned `.proto`/`protoc`/`protoc-gen-go`/descriptor set 无 drift。

`scripts/dev/build-all.sh` 不能把本机生成结果直接注入所有 cross-build target。它只允许消费
与目标完整 build recipe 精确匹配且已由 release job 校验的 native proof manifest；缺少
target-specific proof 时不得构建声称 enabled/release-ready 的产物；仅可输出明确 disabled 的开发产物。普通 `scripts/dev/build.sh`
保持 `dev` 行为并禁用持久缓存，避免每次开发 build 支付额外 generation 成本。

v1 不为 module 外的 private overlay 暴露 build-only generator API。只要
`edition.Get().RegisterExtraCommands != nil`，options provider 就返回 disabled，不读取或
写入 persistent cache；这避免声称能从 `internal/*` API 证明 external registration 已冻结。
source fork 若把命令作为 module 内声明编译，可复用同一 internal generator，并必须注入与
runtime edition 完全一致的 identity。private overlay 支持需后续 RFC 先定义 immutable
registration token/freeze API，不能在本实现中以 hook pointer 或 edition name 猜测 exactness。

release gate 必须证明：

1. generator 连续两次输出相同 identity；
2. Meta DTO 与同次 resolved Registry 的 `CommandMeta` projection 完全一致；
3. release binary 内 expected identity 非空；
4. release binary 空缓存实时装配得到相同 SourceHash、SurfaceHash 和 artifact bytes；
5. cache hit 的 Schema wire 与实时装配 wire 等价；
6. enabled package 同时具备完整 Schema identity、help snapshot、core digest/size，三者的 commit/edition/目标 proof 一致，拒绝任一缺失、半注入或混版；
7. 最终 launcher 在无 core 副本中完成 en/zh root help，与最终 core oracle 逐字节相同；版本、信号、上报语义不变；
8. candidate 与 release 调用同一封装入口，测试证明两者的 capability 注入与失败分支相同；最终签名/公证和安装/升级/整包回滚全部通过。

上述 proof 必须在每个启用 target 的 native runner（v1 仅 darwin/arm64、linux/amd64）对同一
commit 和 Go 1.25.9 分别执行。两个 runner 输出的完整 identity/artifact metadata 必须 byte
equal；release job 校验绑定 commit 的两个 proof artifact 后才能把该 identity 注入所有启用
target。proof runner 必须使用 isolated empty HOME/config/credential stores，拒绝出站 network，
并通过固定 clock seam 或静态/动态 gate 证明 Schema-producing path 不读取 wall clock。随后还要
在 sanitized environment 与 hostile-variation environment（PATH、locale、proxy、DWS_* 非凭据
配置）下得到相同 identity；任何访问 network、credential、非 fixture user file，或任何输出
差异都使 enabled target 的发布 gate 失败。`NewSchemaSourceRootCommand` 的构造合同禁止依赖 network、
credential、clock、user file 或未进入 identity 的 environment，也禁止调用 `ResolveMeta`、
`deliverySchemaCatalog`、Meta/Registry loader 或任何间接 Schema delivery consumer。独立 identity generator 必须用
`AuditSchemaAssembly` 包住 factory、`ResolveSchemaBuild` 和 projection/round-trip 全调用链。
六个 delivery 入口（包括 core 快速路径的注册身份读取）在进入任何 loader Once 或命中已有内存状态之前检查审计状态；访问立即
终止该次 proof，回调自行 recover 也不能清除已记录的 violation。审计本身的测试还须证明
错误/无关 panic 后状态恢复以及正常消费可继续。

声明树构造也不得修改活动调用的 runtime profile、ToolCaller 或 plugin endpoints。冷缓存
首次 `ResolveMeta` 会同步进入该工厂；若构造时重新从 `os.Args` 选择 profile，就会清空或覆盖
调用方在 argv 解析后选择的 profile，造成冷、热缓存行为不同。实现仅让普通 runtime root
执行 profile 初始化，声明树保留当前选择。回归覆盖无 profile 参数、分离参数和 `=` 参数，
并在独立进程中检查首次装配及随后复用 metadata 的 profile 不变；普通 root 仍按 argv 初始化。
该状态隔离验证不能替代上面的 network、clock、credential 和 environment 原生证明。

审计只在独立构建进程安装，禁止把全局“正在装配”标志用于正常 runtime：那会把另一 goroutine
的合法读取误判为重入。正常 runtime factory 仍须遵守同一不消费 delivery 的构造合同，由
对应完整 build recipe 的原生 proof 证明；它不安装进程级审计，不改变 loader/repair 并发行为。
未受 proof 覆盖的动态 factory/overlay 不得启用 persistent identity。该审计不等同于 network、
clock 或 user-file sandbox；只扫描 build constraints 或比较两次碰巧稳定的输出同样不能替代
hermetic native exact proof。

开发级环境检查由 `scripts/dev/check-schema-identity-environment.py` 实现，接到 native feedback
的候选性能采样之后；即使性能失败，只要候选已构建也继续收集环境检查结果。Linux 使用
本机 Docker 的独立网络/进程空间、无 capabilities 的 runner 用户进程与只读挂载；
macOS 使用 deny-default sandbox profile；只提供 generator、两个 protobuf source fixture、
空 HOME 和必要的 OS loader/library 读取路径。macOS 额外允许系统 LibreSSL 配置文件以执行
系统 curl 的控制探针，并记录该文件与导入的 `dyld-support.sb` 摘要；不导入更宽的 `system.sb`。
检查器先证明允许的 fixture 可读、限制外的无害哨兵在限制内不可读，以及同一 loopback HTTP
服务在限制外可达、限制内不可达。允许读取本身失败、curl 无法启动或隔离工具缺失都必须失败，
不能把任意非零退出码当作隔离成功。随后用 fresh empty HOME 执行两次 clean 和一次 hostile
PATH/locale/proxy/DWS 非凭据配置，对照 native candidate 的完整 identity bytes，并检查 fixture
未改变、HOME 未被写入。报告绑定 commit、dirty 状态、generator SHA-256、实际 sandbox argv 与
环境；此检查不重新生成 Catalog 声明或新增 payload authority。

前置 proof 不能替代最终 artifact proof。GoReleaser 生成 candidate binary 后，release job 必须
记录其 binary SHA-256，并把未改写的确切 binary 交给对应 native runner；runner 在相同 hermetic
条件下执行上述第 3-5 项，输出同时绑定 binary SHA-256、前置 manifest 和 injected identity 的
final proof。发布 job 只允许归档/发布该 SHA-256 对应的 binary；归档后还要解包复核 binary
digest。任何 rebuild、relink、codesign 或其他会改变 binary bytes 的步骤都必须发生在 final
proof 之前。enabled target 无法原生执行 final candidate 时不得发布；其他明确 disabled 的 cross-build 不得携带 enabled 能力声明。

### 6.7 包结构与依赖方向

声明装配、纯消费和文件传输分开；`internal/cli` 的类型别名保持现有调用方源码兼容，
不形成第二份 model 或语义解析器：

```text
internal/app → internal/cli（Cobra 绑定、声明装配、delivery/repair）
                        ├─ internal/schemareader（binary identity、共享认证读取）
                        ├─ internal/cli/schemaruntime（typed model/index/query/DTO conversion）
                        │                  ├─ internal/cli/schemacachepb
                        │                  └─ internal/corecmd/contract
                        └─ internal/schemacache（envelope、安全文件 I/O、锁、原子发布）

internal/generator/cmd_schema_cache_identity → internal/app + internal/cli
internal/generator/cmd_root_help_snapshot → internal/app → internal/roothelp（声明派生帮助）
internal/launcher → internal/roothelp（内置投影认证与共享 renderer，无文件读取）
                 → internal/clitelemetry → internal/profilemetadata + 官方 SDK
                 → internal/clisignal + internal/localename（与 core 共用纯规则）
cmd/dws-launcher → internal/launcher（argv 路由、同版本 core delegation）
                           └─ internal/schemafastpath（严格 argv/环境边界，准备完整输出）
internal/app ────────────────────────┘
                                    ├─ internal/schemareader → schemacache + schemaruntime
                                    ├─ internal/jsonutil（同一 JSON byte contract）
                                    └─ internal/skillpaths（共享兼容检查路径，无 I/O）
internal/upgrade → internal/packagemanifest（canonical package 校验）
```

`schemaruntime` 不 import 父包 cli、Cobra、app、auth 或 transport；消费方共用其 typed model、
query、compact renderer 和 protobuf conversion。`schemacachepb` 只包含 `.proto` 和 generated
code。`corecmd/contract` 仍不 import 任何 `internal/cli` 包；约束 DTO 的 normalization 由
`corecmd/contract` 拥有，Cobra annotation writer 委托该函数。上述边界必须由 dependency tests
验证，不能只检查直接 import 而遗漏转递依赖。

进程入口的两种消费方式共用 `schemafastpath.Prepare`：launcher 仍只在显式 telemetry opt-out
时直接输出；默认请求委托 core，由原 `cmd/main` 先取得只读身份快照并进入原 clitrack
生命周期。core 完成 metadata 验证、信号上下文及退出清理注册后，可在命中时跳过 Cobra
树构造，返回相同的 `schema` telemetry path、退出码和脱敏错误摘要。它不改变 tracker
字段、身份读取、flush timeout 或投递方式，也不另起上报进程。

core 必须先确认 `SchemaCacheFastPathIdentity()` 的当前注册身份等于 binary identity；
源码工厂替换或 runtime uncertain 会使快速路径失效。该身份读取在 generator 审计范围内。
宿主嵌入模式、配置目录 hook 或执行 hook 存在时也须回退，不能凭相同 Schema identity
推断这些 hook 没有行为。普通 Schema consumer 仍由 cli 负责。core 仅在准备阶段忽略三个已知 launcher 传输标记
（launcher path、core digest、core version），不修改实际环境；其余 DWS 选项、扩展路径、
用户 shortcut 和兼容 warning 条件继续按共享规则回退。全部认证和渲染成功前不输出；
输出一旦开始，即使短写或失败也不得再回退，错误按原 human/JSON 选择报告给 tracker。

native verifier 必须保留 direct-core 精确 wire 对照及 CPU/RSS 诊断；显式 opt-out 与默认 tracker 的测量分开。Ready/release 只按 §8.4 判定，不从关闭 telemetry 的数据推导默认入口延迟。

`internal/schemacache` 只依赖标准库和 `golang.org/x/sys`，不调用 payload parser；它校验
binary-pinned expectation、使用 bounded fd reads、认证 exact range、发布 Registry/Meta，
并提供 bounded Unix lock。未启用平台为零 I/O stub。Schema-specific loader/repair 留在 cli；
launcher 的 Schema reader 必须复用相同的认证与纯消费代码，禁止为绕开 cli import 重写语义。

App 注入 build vars、判断 edition/overlay/platform eligibility 和注册 options；对于符合严格
子集的进程级 Schema 查询，通过共享 `schemafastpath` 准备认证后的完整输出，不实现第二套
文件认证、语义解析或缓存写入。miss、损坏或不确定环境仍交给普通 cli delivery/repair。
注册顺序为 `RegisterSchemaSourceRoot(factory)` → `RegisterSchemaCacheOptions(options)`：

- 旧 `RegisterSchemaSourceRoot` 先清除缓存身份与 lazy delivery 状态，确保新 factory 不继承旧信任锚；
- 只有显式 `Enabled` 且完整有效的 identity 才启用缓存；无效选项先清除 registration，随后向调用者返回错误；发布入口按 §6.11.1 拒绝，不得以清除注册替代错误处理；
- options 的内容解析不做 I/O，文件打开仅在首次消费时发生；
- `RuntimeEligible` 在消费前重新检查 disable 环境、edition 和 overlay；plugin 注入会标记
  runtime uncertain，进程内后续调用绕过持久缓存；
- 注册属于启动阶段，不能和消费并发调用。正常消费、首次加载与 repair 必须支持并发；
- Catalog 与 Meta 全部构建成功后才发布 live pointer。该 release/acquire 是 `ResolveMeta`
  快速 map read 的可见性边界，不能提前发布 Catalog 后再安装 Meta；
- product memoization 的读取不能消耗未完成 loader 的 Once；成功 repair 发布新的 immutable
  load，必须替换既有失败状态。

Cache protobuf/DTO 不从 `pkg/cli` 导出，也不进入 public Schema wire。跨 package 测试辅助仅放在
`fortest.go`；production 不调用 ForTest API。声明仍是唯一权威，binary-pinned 缓存只作为其
可丢弃、可重建的传输派生物；旧 Catalog dump 不能替代声明输入。

### 6.7.1 Help 单一真相与委派矩阵

唯一帮助源为 **declarations → `app.RootHelpModel` → `roothelp.Model`**。core 从当前有效声明树取得模型；构建进程从隔离的声明树取得同源模型并与最终 core 核对。`roothelp.Snapshot` 只是绑定完整 commit、open edition、最终 core SHA-256 的构建派生品，不能手写服务表、成为第二份声明，或从用户可写配置读取。

可见性必须从声明与 edition 的静态信息得到，不能依赖先执行 runtime root 的全局注入副作用。回归必须在 fresh subprocess 中先生成声明投影、再执行 runtime root，分别覆盖 en/zh，并覆盖显式注册的 supplement-backed 命令。增加命令/描述、改变可见性或 core bytes 后必须重封整个 package。

| 调用 / 制品状态 | 执行所有者 | 必须满足的行为 |
|---|---|---|
| exact `--help`，open、enabled target、plain、完整有效绑定 | launcher | 使用同一 renderer；默认上报一次，opt-out 不读取身份；零 Schema cache I/O，不定位/打开 core |
| exact `--help`，适用范围内但 snapshot 缺失 | core | 原样委派；正式 enabled 包在构建阶段就应拒绝该状态 |
| exact `--help`，适用范围内且 snapshot present but invalid/mismatch | launcher 错误出口 | §6.11：退出 125、明确诊断、stdout 为空、不委派 |
| leaf `--help`、`help <path>`、`-h`、附加/未知 flags | core | argv 原样交付完整框架；不得猜测等价入口 |
| settings/plugin/shortcut 扩展、动态发现或非 plain 环境 | core | 不消费封存模型，保留运行时有效命令树 |
| edition ≠ open 或未启用 target | core | **v1 永远委派，不另封该 edition 的帮助投影**；扩展支持须先修改 RFC 与 gate |

先判断静态支持范围和调用是否属于允许能力；本来就应委派的调用不消费 snapshot，不把运行时扩展导致的树变化误判为绑定损坏。进入受支持的 exact-help 分支后，必须区分 missing 与 invalid，禁止将解码错误统一转换为 unhandled。模型完全校验后才能输出；任何已开始输出的路径不得再次委派或重复上报。

构建期模型解码、语言投影、stderr/退出码或最终 core 摘要校验任一失败，**候选与正式包构建均失败**。运行期采用下节明确的 fail-closed，不保留“坏 snapshot 静默委派”的 availability 例外。

### 6.8 路径与权限

默认路径：

```text
os.UserCacheDir()/dws/schema/<edition-sha256-hex>/v1/
  meta.cache
  registry.shards.cache
  rebuild.lock
```

规则：

- DWS 创建并拥有的 `dws/schema/<edition>/v1` suffix 目录必须为当前用户所有且 `0700`，文件
  必须为当前用户所有且 `0600`；不要求 `/`、root-owned 系统目录或预先存在的
  `os.UserCacheDir()` ancestry 都是 `0700`；
- edition 先通过 `[a-z0-9][a-z0-9._-]{0,63}` 校验，目录段使用完整 64-char SHA-256 hex，
  避免 Windows device name、Unicode normalization 和 path separator 歧义；
- `os.UserCacheDir()` 失败时禁用持久缓存；
- cache 不进入 `DWS_CONFIG_DIR`，因为它可安全删除且不属于用户配置/凭据；
- official open 和启用缓存的 source fork 使用独立 edition digest partition；
- 文件名稳定且数量有界，版本变化原子覆盖，不按 content hash 无限累积。

v1 只在 darwin/arm64 和 linux/amd64 启用 persistent cache；其他 target 编译一个零 I/O
disabled stub，并继续使用实时装配。安全打开不能使用 `Lstat → Open` 的可竞态组合。Unix 从 `/` 的可信 fd
开始逐段使用 `openat(..., O_DIRECTORY|O_NOFOLLOW|O_CLOEXEC)` 打开 `UserCacheDir`，再以
`mkdirat/openat` 创建并打开 DWS-owned suffix；任何 symlink 或不安全 ownership/mode 都
禁用 cache。读取 target 使用 `O_NOFOLLOW|O_CLOEXEC|O_NONBLOCK`，之后对已打开 fd 执行
`fstat`，只接受当前用户拥有、permission bits 精确为 `0600` 的 regular file。读取上限是 binary expected exact file
length + 1，不按不可信 header 分配。
staging 以随机名和 exclusive create 建立；发布通过 directory-relative atomic replace，
绝不跟随 target symlink 写入。平台无法证明安全打开条件时按 miss 处理，不降级为普通
`os.Open`。

ancestry 的规则是规范性的：每个已存在 component 只能由 root 或当前用户拥有；root-owned
component 不得 group/world writable，当前用户拥有的 component 同样不得 group/world writable。
不满足这些条件的自定义 `UserCacheDir` 直接禁用 cache，不尝试接受 sticky world-writable
ancestor。上述 ancestry 规则与 DWS-owned suffix 的精确 `0700` 规则分开检查。

这里的路径防护主要避免阻塞、TOCTOU 和误写；缓存内容的 provenance 仍只依赖
binary-pinned digest。active same-UID adversary 可以替换目录或 staging entry，破坏 cache
availability/self-heal，本 RFC 不声称阻止这种 DoS；但任意最终 target bytes 仍必须由 reader
从同一 fd 读取并通过 exact digest，才能进入 parser，因此这种干扰不能改变被接受的 safety
metadata。

### 6.9 命中、失效与自愈状态机

```text
read target
  ├─ valid + binary-pinned → HIT
  └─ missing/invalid → MISS
       └─ repairCoordinator.enter(target)
            └─ acquire rebuild.lock (bounded)
                 ├─ acquired → low-level re-read target
                 │    ├─ valid → HIT_AFTER_WAIT；只完成当前 loader，不消耗 authoritativeRepairOnce
                 │    └─ invalid → authoritativeRepairOnce.Do(assemble + publish) → use memory result
                 └─ timeout/error → authoritativeRepairOnce.Do(live assemble, skip publish)
```

meta miss 时不能从 registry data file 单独恢复：可信 product descriptors 本身位于 Meta，读取
整个 data file 再按 protobuf wire 猜边界会重新引入 parser/trust 问题。Meta miss 或目标 Registry
shard miss 都进入唯一 authoritative rebuild；Meta 不能反推完整 Schema。

并发 owner 明确且唯一：`metaLoader` 和 `registryShardLoader` 各有自己的 loader state/按 product
memoization，但两者只能调用同一个 repair coordinator。lock 内 recheck hit 只填充当前 loader，
不能把 process-wide `authoritativeRepairOnce` 标记为完成。只有确实开始 authoritative assembly
时才消费该 Once；其 immutable result 固定为 `{Registry, Meta, error}`，之后任何 loader miss 都
复用该完整结果，包括失败结果，不在同一进程内重试实时装配。coordinator 内部只能
调用 low-level `readMetaArtifact`/`readRegistryShardArtifact`，禁止回调任一 loader，避免
Meta miss → Registry shard loader → repair → Meta loader 的递归死锁。

如果 Meta 首先 miss，coordinator 的 lock 内只 recheck Meta；recheck hit 返回时不设置全局
repair result。仍 invalid 时最多调用一次
source-root factory 和 `ResolveSchemaBuild`，产生完整 Registry、product shards 与 Meta。
若 Meta 已 hit、之后目标 Registry shard 才 miss，仍由同一
coordinator 完成唯一 authoritative rebuild。该结构给出“每进程至多一次实时装配”的明确
ownership，不依赖嵌套 `sync.Once` 的调用顺序。

两个文件独立携带完整 header。发布顺序固定为先原子替换 `registry.shards.cache`，再原子替换
`meta.cache` 作为 commit marker：进程崩溃后出现“一新一旧”时，旧 Meta descriptor 与新 data
range hash 不匹配，只会 miss，不会成为部分有效状态；不允许先发布 Meta。

损坏文件不先删除、不 quarantine。只有新派生物完整生成、校验、sync 并原子发布后才
覆盖；这样 rebuild 失败不会丢失最后一个文件，也不会累积大体积 quarantine。

### 6.10 原子发布与跨进程锁

发布顺序：

1. 在目标目录创建随机同目录 staging file。
2. 以 `0600` 写入完整 header 和 payload。
3. `file.Sync()`。
4. `Close()`，任何 close error 都放弃发布。
5. 原子 rename staging → target。
6. 支持的平台对父目录执行 sync。
7. 清理失败的 staging file。

一次双 artifact repair 对 `registry.shards.cache` 完成上述 1-6 后，才能对 `meta.cache` 执行
上述流程；Meta rename 是该 generation 的 commit marker。任一步失败都不发布后续 Meta。

禁止使用固定 `.tmp` 文件名。正常并发 writer 即使失去锁，也只会 rename 一个完整的 candidate
file；publish 本身不建立信任。active same-UID adversary 可能替换 staging/target 并导致 miss，
所有 reader 仍必须独立执行 binary-pinned Meta 或 authenticated product-range digest gate，不能
因为文件来自本进程 publish 就跳过验证。

lock 只用于抑制多进程同时支付约 2.4 s 的 cold rebuild，不承担完整性或互斥正确性。
使用独立 `rebuild.lock`，通过同一 directory fd 以
`O_CREAT|O_RDWR|O_NOFOLLOW|O_CLOEXEC|O_NONBLOCK` 执行 `openat`，随后 `fstat` 并要求当前
用户拥有、permission bits 精确为 `0600` 的 regular file，再执行非阻塞 `flock` + bounded polling。拿锁后必须
二次读取。lock timeout 时不阻断命令，直接实时装配并跳过写入。

现有 event/auth lock 实现只能作为行为参考；Schema core 不应依赖 event-owned 包。

### 6.11 失败语义：密封制品与可丢弃缓存分开

#### 6.11.1 内置帮助/身份绑定

| 状态 | 对外行为 | 输出 / 观测 | release 验收 |
|---|---|---|---|
| 所需 capability 元数据完整缺失 | 委派 core；dev/disabled/兼容包可以走该路径 | 不产生部分帮助，不重复 tracker | enabled 正式包必须在封装前拒绝 missing |
| snapshot present，但编码/版本/大小/结构/模型无效 | **fail-closed，退出 125，不委派** | stdout 为空；stderr 使用稳定 `artifact` 分类及 `invalid_help_snapshot` 原因，不打印原始 payload | 子进程负向测试证明无 core 调用、无部分 help |
| snapshot 的 commit/edition/core digest 与当前内置身份不匹配 | **fail-closed，退出 125，不委派** | stdout 为空；稳定 `help_snapshot_mismatch` 原因 | stale/mixed-package 测试必须失败，不能按 missing 测试 |
| Schema identity 字段有任意值但不完整、解析失败或绑定矛盾 | **fail-closed，退出 125**；禁止当成“未注入” | 稳定 `invalid_schema_identity`，不读取不可信缓存 | launcher/core 构建入口和 runtime 的 missing/invalid 区分必须一致 |
| 委派需要的 core 缺失、size/hash 不符或路径不可信 | 保持 artifact error / 125，不执行 core | 保留稳定错误与空业务输出 | 错版本、篡改、symlink、校验竞争均必须拒绝 |

non-open、非 plain、leaf help 等适用范围外的调用先按 §6.7.1 委派，不解析本次不消费的 snapshot。对范围内 present-but-invalid 的密封投影，**没有静默回退例外**。失败至少通过退出码和稳定 stderr 原因可观测；不为上报失败再启动一套 tracker，也不泄露 snapshot、凭据或 profile 原文。

#### 6.11.2 用户可写 Schema cache 的显式可用性例外

以下表只适用于可重建的磁盘 Meta/product cache，**不适用于内置 help snapshot 或 binary-pinned identity**。其失败允许回到唯一的声明装配，因为它是派生数据，不是发布绑定；必须记录既有内部 counters/debug 原因，不在默认 stdout/stderr 增加 cache warning。

| 情况 | 行为 |
|---|---|
| cache 不存在/损坏/过期/hash 不匹配 | miss，尝试同步修复 |
| cache 目录只读、mkdir/write/sync/rename 失败 | 使用已成功的内存装配结果；不让业务命令失败 |
| lock timeout/error | 实时装配，跳过发布 |
| source-root 未注册 | 保持当前 fail-closed error |
| source-root 返回 nil | 保持当前 fail-closed error |
| 实时装配失败 | 返回实时装配错误；不使用 stale cache |
| 实时装配 hash 与注入 expected hash 不同 | 使用实时 authoritative 结果但禁写 cache；记录内部诊断，release gate 必须阻止该状态发布 |
| expected identity 全部为空（dev/明确 disabled 模式） | 完全绕过持久 cache；部分字段或解析失败按 §6.11.1 拒绝 |

默认不向 stderr 打 cache warning，避免污染 JSON/脚本输出。命中、miss、corrupt、repair、
write failure 和 identity mismatch 通过内部 counters/debug logging 暴露。

### 6.12 不加密 Schema cache

v1 不加密 Meta/Registry cache。它们只包含可通过 `dws schema` 输出的声明元数据，不包含
token、credential、tenant 或用户业务数据；§6.2 的 DTO gate 必须持续禁止这些内容进入。
SHA-256 不是加密，它只把本地 artifact 绑定到当前发布二进制，提供完整性和 provenance。

为避免把“不加密”建立在未量化的性能猜测上，使用 32-byte key、12-byte nonce 和固定 AAD，
对 26,080,786 B gob token proxy 执行 AES-256-GCM `Seal`/`Open`，独立运行 7 次的 p50 分别为
4.18 ms 和 4.03 ms；`Seal(nil, nonce, ...)`/`Open(nil, nonce, ...)` 的 ciphertext 只增加 16 B
tag，每次调用约分配 26.08 MB/1 allocation。可部署 envelope 还必须存储或安全推导唯一的
12 B nonce，因此不能把 `+16 B` 当作完整格式开销；传入可复用 destination 也会改变 allocation。
该数据只代表 M3 Pro 的硬件/实现，不能外推 linux/amd64，但足以说明拒绝加密的主因不是性能，
而是没有 confidentiality 需求、不存在合理的密钥来源，以及平台 key-store 会增加新的可用性依赖。

把固定 AES key 嵌入同一个可执行文件不能提供有意义的本地保密性；使用 Keychain/DPAPI
管理 per-user key 则会给每次短生命周期 CLI 启动增加 key retrieval 和失败面，却没有需要
保护的秘密。若未来 cache 内容边界改变，应优先禁止敏感字段进入 Schema；确有 at-rest
confidentiality 需求时另行设计 compress-before-encrypt 的 AES-256-GCM envelope，并把 key
生命周期、nonce、AAD、rotation 和平台 fallback 作为独立安全 RFC，不在本优化中顺带加入。

## 7. 拒绝的方案

| 方案 | 拒绝理由 |
|---|---|
| gzip 39 MB `SchemaCatalogSnapshot` JSON | typed load 实测约 2.32 s/1.33 GB allocations，无法优化热路径 |
| 直接 gob `SchemaRegistry` | pointer/map/interface 和 nil/empty 语义不可控，map bytes 不确定，且完整 Registry round trip 未通过 `reflect.DeepEqual` |
| 单个 full cache 同时服务 `ResolveMeta` | raw protobuf Registry + Index codec-stage 约 105.8 ms，显著慢于独立 protobuf Meta 的约 1.44 ms |
| monolithic Registry cache 服务所有 `dws schema` | 完整 file-hit p50 约 114.2 ms、101.73 MB/op；calendar product shard 约 4.35 ms、3.80 MB/op，且 31 shards 总 payload 只增加 7,170 B |
| tool-level offset shards v1 | target 约 1.06 ms，但相对 product shard 只再省约 3.3 ms；增加 2,739-entry path locator 和 product/group/all 双重组合逻辑，且不能解决约 250 ms 非 Schema 启动基线 |
| zstd 压缩本地 Registry | ratio 优于 gzip，但未证明相同 open/read/hash accounting 下稳定快 ≥25 ms；raw 已满足大小预算且无需新增依赖和解压边界 |
| gzip 压缩本地 Registry gob | 从 14,183,649 B 降到 1,069,827 B，但 decode + Index 从约 118.0 ms 增至 141.4 ms，allocated bytes 从 94.7 MB 增至 109.1 MB |
| gzip 压缩本地 Meta gob | 只节省约 522 KB，codec decode 从约 0.05 ms 增至 2.92 ms；Registry 也没有网络传输收益 |
| Meta 单独使用 gob | overview/locator-inclusive lookup p50 约 1.29 ms，只比 protobuf 的 1.44 ms 少约 0.15 ms；该差异小于未隔离环境噪声，不值得增加第二种 parser/format，且 protobuf B/op 更低 |
| gzip-9 | 相对 default 收益很小且 rebuild encode 更慢 |
| Snappy/LZ4 | full decode 快，但 ratio 更弱且仍需新依赖；不服务普通业务热路径 |
| CBOR/MessagePack | 完整同形 mirror 的 decode + Index p50 约 144.8/166.8 ms，均慢于 protobuf 105.8 ms，且 artifact 更大 |
| FlatBuffers/Cap'n Proto v1 | common DTO decode 快，但 raw artifact 约 55.0/49.9 MB，且需 schema compiler、generated API 和对象 conversion；最终 Registry 路径收益未证明 |
| custom binary v1 | common DTO 最快，但自研 parser、unknown-field 演进、fuzz corpus 和长期安全维护成本最高；只有 selected path 超预算才复评 |
| AES-GCM | 26.08 MB proxy 的 Open p50 约 4.03 ms，性能可接受；但 Schema 无秘密且无合理密钥来源，增加平台 key-store 可用性依赖没有安全收益 |
| version/commit cache key | 不覆盖真实 Schema 内容，也不验证用户可写 payload |
| snapshot self-hash | 攻击者可同时改 safety 和重算 hash |
| 只使用 `SurfaceHash` | 只覆盖 identity/navigation，不覆盖 safety/params/result/provenance；它仍必须作为独立 identity 字段校验 |
| TTL | Schema 对一个二进制是静态内容，时间不代表 freshness |
| stale-while-revalidate | 可短暂发布旧 safety/confirmation，拒绝 |
| background repair | CLI 可能立即退出，无法保证写完；增加 goroutine/lifecycle 风险 |
| 用户可写 Schema cache error 一律 fatal | 可重建 cache 不应降低声明驱动路径的可用性；内置密封绑定损坏必须 fatal，按 §6.11.1 |
| committed/generated Catalog authority | 违反“声明即 Catalog”和现有 drift policy |
| 直接复用 `dws cache` | 该命令是 deprecated 服务发现兼容 no-op，不属于 Schema cache 产品面 |

## 8. 测试与性能门禁

### 8.1 正确性

- valid meta hit 不调用 source-root factory。
- valid target product shard hit 不调用 source-root factory，也不读取其他 product ranges。
- Meta DTO primary path、alias、safety、selection 与 live Registry projection 完全相等。
- Meta locator 的 product set、offset/length/hash 与同次生成的 sorted product shard data 完全相等。
- Meta Overview DTO 与 live no-argument `dws schema` wire byte-equivalent，overview hit 不读取 Registry data。
- cached Schema overview/product/group/leaf/`--all` 与 live assembly wire 等价。
- runtime type field inventory 与 protobuf descriptor/conversion manifest 双向完全一致。
- all-fields sentinel 覆盖每个 scalar、pointer、nil/empty collection、RawMessage 和 provenance
  candidate；任一缺失或未 bump DTO version 均失败。
- root help 和 `--version` 不读取 cache。
- expected identity 全部为空时零 persistent-cache I/O；部分注入、无效绑定按 §6.11.1 拒绝。
- external overlay、运行时环境不在 proof 范围和明确 disabled target 不消费 persistent cache；编译身份与已注入 identity 的 edition 矛盾必须拒绝。

### 8.2 损坏与边界

- bad magic、未知 kind/codec/version。
- 截断 header、截断 payload、trailing bytes。
- 超大文件、encoded/decoded length 不相等、Registry shard data 超出 67,108,656 B，或总文件超出 64 MiB。
- product descriptor 未排序、重复、空洞、重叠、越界、零长度、错误 product ID 或 range hash mismatch；单 product shard 超过 8 MiB。
- protobuf decode failure、任意层级 unknown fields，以及 DTO version/shape 不符合合同。
- self-consistent header/payload 但 binary expected encoded SHA mismatch。
- `SourceHash`、`SurfaceHash`、BuildID、edition 任一 mismatch。
- `Selected: &false`、nil/empty slice 和 optional field exact round trip。
- invalid identity/interface/provenance/index。
- symlink、directory 和 non-regular target。
- cache/lock file wrong owner，或 permission bits 不是精确 `0600`。
- open/check race、target replacement 和 staging symlink 注入。

### 8.3 自愈与故障注入

- missing/corrupt cache 只触发一次 authoritative assembly。
- meta miss 不解析无 descriptor 的 registry data，直接进入唯一 authoritative rebuild。
- write、sync、close、rename、directory sync failure 不覆盖成功内存结果。
- rebuild 失败不删除旧文件。
- reader 在 publish 中只看到完整旧文件或完整新文件。
- 多 goroutine 只初始化一次。
- source-root factory/`ResolveSchemaBuild` 全调用栈访问任一 Schema delivery loader 时 fail-fast，不能等待自身 repair。
- lock 内 recheck hit 不消耗 `authoritativeRepairOnce`；随后另一个 shard miss 仍可完成唯一 rebuild。
- Meta-hit/shard-hit、Meta-hit/shard-miss、Meta-miss/data-present、双 miss 的两种先后
  顺序，以及两 loader 同时首次进入，均证明无递归且每进程最多一次 authoritative assembly。
- coordinator failure 对两个 loader 返回同一 immutable error，不在进程内隐式重试。
- 多 subprocess first-use 最终得到一个可解码文件。
- lock timeout 不死锁、不阻断 live fallback。

### 8.4 性能门槛：唯一生效口径

本节是 Ready/release 的唯一性能 gate 定义。**旧的 `launcher vs 同包 core ≤5%`（help/version p50、p95 共四项）自 2026-09-06 起废止**；必须从脚本的阻挡 `gates/passed` 聚合中移除，只能保留为 `diagnostics`。旧 raw reports 不改写、不追溯“改绿”。本节不允许同一指标同时存在新旧两套 release 判定。

#### 8.4.1 入口回归：只对固定 pre-PR 入口

基线固定为 `5243e5ca19b55a3e785e5cc09273b653ad5381dc`，在同一 native host 使用 Go 1.25.9、相同 release flags/runtime payload 和一致的默认上报、认证、locale 条件构建与测量。每种模式至少 30 个独立子进程，随机交错，保存完整样本；p50 为中位数，p95 为排序后 `ceil(n×0.95)` 项。

| 入口 / 模式 | release gate | 不属于 gate 的数据 |
|---|---|---|
| exact root `--help` 与 exact `--version`，默认上报与显式 opt-out **分开** | 每平台、每模式的 wall p50 和 p95 都 ≤固定 pre-PR 对应入口的 1.05 倍；help/version Schema cache I/O count = 0 | 同包 core 差值只用于定位装配与入口成本 |
| opt-out Schema 命中（launcher 直接执行）及默认 Schema 命中（core 执行） | 同一候选、同一上报模式的 authoritative assembly 对照；user CPU p50 至少降低 80%，全部有效样本 process peak RSS ≤100 MiB | 不设置“launcher ≤ core+5%”，不要求低于竞品 |
| 委派路径：leaf help、普通命令、扩展/非 plain 等 | 正确性、参数/信号/退出码、逐次 core 完整校验必过；**不设相对同包 core 的 wall release gate** | 成对上报 launcher/core 的 wall、CPU、RSS，以及 core 大小、哈希阶段耗时和启动开销；不把总差值全部归因于哈希 |

hash 阶段必须独立测量/归因，诊断可用专门 benchmark 或不进入验收耗时样本的 tracing；不能通过删校验、缓存校验结论或改变 SDK 默认 flush budget 换取通过。对委派路径没有墙钟门槛不等于声称其性能无回退，所有测到的变化必须如实报告。

#### 8.4.2 Schema 组件预算

| 路径 | release gate |
|---|---|
| Meta cache hit：read + verify + protobuf + exact conversion + lookup | p50 ≤5 ms，allocated bytes ≤8 MB |
| selected product-shard hit：authenticated locator + pread + verify + protobuf + exact conversion + product-local validate/Index | p50 ≤15 ms，allocated bytes ≤8 MB |
| `schema --all`：逐 product verify/decode + global validate/Index | p50 ≤150 ms，allocated bytes ≤110 MB |
| Registry raw shard data | 总 payload ≤67,108,656 B，单 shard ≤8 MiB，总文件 ≤64 MiB；超限须重新评审压缩/粒度 |
| full `--all` product-shard protobuf 相对同形 JSON | p50 和 user CPU 均 ≤JSON 的60%；peak RSS ≤JSON 的125% |
| steady `ResolveMeta` | map lookup，0 allocation |
| cache miss | 不设硬 latency gate；只 assemble 一次并成功自愈，失败遵守 §6.11 |

正确性覆盖 darwin/arm64、linux/amd64 enabled path，以及其他 target 的明确 disabled build/fallback。预算分别在两个 enabled native target 验收，不能跨机器拼接基线。

#### 8.4.3 竞争性与整体内存报告

Lark/GWS 对比独立于 Ready/release gate。public 对 public、native 对 native，固定版本和机器、元数据预热与认证/上报条件，至少 30 次交错样本，列出完整 argv 和输出大小。只有相应同条件 p50/p95 数据支持时才能宣称领先；一个入口的结论不得替代另一个。

普通命令与进程树 RSS 是完整性能报告的必报维度，不由 Schema 或 version 外推。wrapper 与 native 子进程同时驻留的 RSS 另做独立采样，披露采样间隔和下界性质；不能相加不同时刻的各自峰值。真实 RPC、mock、dry-run 和长期事件进程分开说明测量范围。该报告不新增“必须击败竞品”或“所有命令统一省内存”的 gate。

## 9. 实施顺序、发布生效条件与回滚

1. **设计冻结（2026-09-06）**：选 A；§1.2 allowlist、§6.7.1 委派、§6.11 失败语义与 §8.4 新门槛生效。§10 未对齐项按代码事实保持未勾选。
2. **代码对齐，仍保持 Draft**：先完成能力/依赖 gate 和旧延迟 gate 迁移，再实现 missing/invalid 分流及负向测试；把候选与 release 封装归并为一个入口。已有 CI 可以完成并保留原始结果，但不能算此阶段完成。
3. **Ready 条件**：§10 所有“阻挡 Ready：是”条目勾选且绑定可复核的代码/测试；在对齐后的同一源码上完成两平台新 gate、声明/全量 suite、候选最终制品身份、help、安装和整包回滚测试。PR runner 不持有正式 Developer ID/公证与发布凭据，因此 R8 的官方 release proof 不作为转 Ready 的前置条件，也不得被候选 proof 冒充。
4. **发布条件**：R8 必须在首次包含本实现的官方 prerelease/stable 发布工作流中完成，且该工作流在公开资产前执行 §6.6 同构注入、Developer ID/公证、enabled target native proof、安装/升级/整包回滚验证；任一证明不足即停止发布。不存在“先发布空快路径正式包、以后再补”的过渡产品态。支持矩阵外的 disabled target 必须显式标识，不能借 enabled target 的证据宣传。
5. **回滚**：保留上一份已验证的完整 immutable version package，原子切换公开入口/版本指针；对 launcher、core、manifest 和 runtime payload 一起回滚并做版本/业务 smoke。禁止清空注入字段、仅替换单个二进制或在发布后改写已签名 bytes。旧用户 cache 因 identity 不符而 miss，无须 migration/手工删除；下一次受支持版本按正常规则自愈。

## 10. 代码未对齐清单与 Ready 检查

审计基准：`c0f3aaca` 生产实现、本分支当前实现及本节所列自动化证明。勾选表示代码与本地定向测试已经对齐；依赖新提交 native CI 的项目仍保持未勾选。

- [x] **R1 — 能力 allowlist 可执行；阻挡 Ready：是。** [能力分类器](../internal/launcher/capabilities.go) 集中定义 version/help/Schema 的 argv 与环境边界，[路由矩阵](../internal/launcher/launcher_test.go)、[依赖 gate](../internal/launcher/dependencies_test.go) 和 [PR policy](../scripts/policy/check-launcher-capability-allowlist.sh) 同时约束能力扩大；Schema reader 的独立闭包 gate 保留。
- [x] **R2 — 延迟 gate 一次性迁移；阻挡 Ready：是。** [默认入口脚本](../scripts/dev/measure-schema-default-entry.py) 只用 default/opt-out 的 pre-PR help、version 与 Schema CPU/RSS 合同决定 `passed`；同包 core 比较和全量 SHA 阶段归因均进入 diagnostics，[测试](../scripts/dev/test_measure_schema_default_entry.py) 固定覆盖 480 个样本且拒绝缺 baseline 假绿。
- [x] **R3 — invalid help snapshot 拒绝；阻挡 Ready：是。** [tryTrackedHelp](../internal/launcher/help_tracking.go) 对 malformed/stale snapshot 返回稳定 artifact 错误；[负向测试](../internal/launcher/help_tracking_test.go) 验证空 stdout、零 core 委派、零 identity/tracking 副作用。
- [x] **R4 — Schema identity 的 missing/invalid 区分；阻挡 Ready：是。** [identity parser](../internal/schemareader/identity.go) 仅把全空字段识别为 disabled，launcher 与 core 对部分注入/非法字段 fail-closed；[真实 launcher 子进程测试](../cmd/dws-launcher/main_test.go) 证明部分注入以 125 退出。
- [x] **R5 — 唯一候选/正式封装入口与完整注入；阻挡 Ready：是。** [共享构建合同](../scripts/build/schema_package_contract.py) 统一验证 identity、生成 core/launcher ldflags 并密封最终 core help；candidate builder 与 [post-goreleaser](../scripts/release/post-goreleaser.sh) 共用该入口。正式 workflow 要求 Darwin/arm64 与 Linux/amd64 原生生成 byte-identical proof 后才允许打包。
- [x] **R6 — manifest 与部署失败分支；阻挡 Ready：是。** [package manifest](../internal/packagemanifest) 显式声明 enabled/disabled 能力矩阵；安装器和 [最终包 verifier](../scripts/release/verify-package-version.py) 校验 manifest、二进制绑定、core-free help/Schema，并确认执行前后 launcher/core bytes 未变。
- [x] **R7 — help 单源的最终制品证明；阻挡 Ready：是。** candidate/release 共用声明投影 generator 与 seal 合同；最终包 verifier 在 native runner 上逐字节比较 en/zh core help 和无 core launcher 输出，并覆盖真实 Schema live/warm/core-free cache-hit 路径。invalid snapshot、非 plain/leaf 委派继续由 launcher 单测覆盖。
- [ ] **R8 — 最终 release proof 与整包回滚；阻挡 Ready：否；阻挡官方发布：是。** [环境 proof](../scripts/dev/check-schema-identity-environment.py) 不等于 Developer ID/公证及正式 final-artifact proof；[release workflow](../.github/workflows/release.yml) 必须在发布前运行 enabled target 的 native help/Schema、签名/公证和最终 bytes 验证，安装/upgrade/rollback 链必须只切换完整 immutable package。PR 只验收这套 fail-closed 工作流和无凭据回滚测试，正式勾选须引用第一次包含本实现的 release run；不得用 ad-hoc candidate 替代。
- [ ] **R9 — 新 gate 的同源码原生复验；阻挡 Ready：是。** [native workflow](../.github/workflows/schema-cache-native.yml) 须在 R1–R7 的代码对齐后跑两平台全量/声明/生命周期/新性能 gate。`c0f3aaca` 当前测量继续作为历史数据保留，不能据此勾选本项。
- [ ] **D1 — 五维诊断与哈希归因报告；阻挡 Ready：否（不参与 release gate）。** [五维脚本](../scripts/dev/measure-cli-five-dimensions.py) 正在测固定场景；补完整 raw samples、同包 core 诊断、哈希阶段独立归因和竞品对比到[性能附件](rfc-schema-runtime-cache-performance.md)。缺失维度必须标明，不能发表该维度的领先/整体改善结论。

已定且不可在实现中临时改选的格式边界继续保留：Meta/product Registry 均为 raw deterministic generated protobuf；不恢复生成 Catalog 权威；缓存 envelope/I/O/锁由 `internal/schemacache` 统一拥有；唯一 repair coordinator 管装配；private overlay 未获 proof 不启用；Serializer/codec 变化须另行 RFC 与格式版本变更。磁盘 cache mismatch 的自愈例外不适用于密封 help/identity。

## 11. 决策结论

本 RFC 选定 **受限热路径 runtime + core 双二进制 canonical package**，固定执行能力与依赖 allowlist；Meta 和 product-sharded Registry 使用同源、可重建的 raw deterministic protobuf。help 只有 declarations → RootHelpModel 一个源，snapshot 仅为最终 core 绑定的构建派生品。

热路径按固定 pre-PR 入口验收；同包 core 和竞品对比只作诊断。正式 enabled 包从第一次包含本实现的 release 起与候选同构注入。缺失 capability 走明确委派，范围内 present-but-invalid 的密封绑定 fail-closed；用户可写 Schema cache 的自愈是单列的可用性例外。

**形态与规则已定；§10 中所有“阻挡 Ready：是”的代码差距清零并完成同源码 native 候选验证后，才可转 Ready；R8 在首次官方发布前单独清零。**
