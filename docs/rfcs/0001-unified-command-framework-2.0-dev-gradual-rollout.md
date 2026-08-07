# RFC-0001：DWS Unified Command Framework 2.0 与 dingtalk-dev 渐进迁移

- 状态：Proposed
- 日期：2026-08-07
- 适用仓库：`dingtalk-workspace-cli`
- 首个迁移域：`dingtalk-dev`（`dws dev ...` 与 `dws devapp +...`）
- 结果协议：`dws.output.v2`

## 1. 摘要

Framework 2.0 将命令的声明、执行结果、序列化、stdout/stderr 和退出码收口到统一框架。业务命令只负责返回类型化数据或类型化结果，不再自行决定 JSON 信封、日志流向或进程状态。

本 RFC 的核心裁决是：

1. **不公开 `--output-contract`，也不增加等价别名。** Agent 始终使用已有的 `--format json`。
2. **每条命令在每个 release 中只有一个 active contract。** 已迁移命令默认输出 v2；未迁移命令保持 legacy。
3. contract 选择是命令声明和 release 的属性，不由用户、Agent、会话能力协商或环境变量决定。
4. 内部迁移状态固定为：

   ```text
   legacy_only -> dual_validate -> v2_active -> v2_stable -> v2_only
   ```

5. 回滚通过修改内部 rollout 状态并重新发布完成，不给消费者增加切换参数。
6. `dws dev ...` 与 `dws devapp +...` 都是既有稳定入口。两套前缀均保留、原位迁移，不互相改名、合并为 alias 或强制 redirect。
7. 当前 Agent 使用的 `devapp +...` shortcut 必须在原命令路径接入 v2。两套命令可声明同一能力关系和各自适用条件，以消除 Agent 选择歧义，但显式输入哪个路径就执行哪个路径。
8. `dws dev connect` 同一 Cobra 命令同时承载前台长连接与 `--daemon`，因此整条命令（含 daemon start）暂留 legacy；`status/stop/restart/list` 等独立终态子命令进入 v2。后续若要迁移 daemon start，先拆成独立 terminal command 或另立 mode-aware stream RFC。

一句话边界：**框架保证结果被统一、诚实地表达，但不自动推断业务真实终态。**

## 2. 背景与问题

既有 CLI 的输出职责分散在 product handler、helper formatter、shortcut runtime 和 root error path 中，导致同类命令可能出现不同信封、字符串布尔值、日志污染 stdout、失败退出 0、部分结果被压缩成普通错误等问题。

Framework 2.0 解决的是协议层和执行框架层问题：

- 统一 success、pending、partial failure、failure 的表达；
- 统一 typed error 与退出码；
- 统一 `--format`、`--jq`、`--fields` 和输出流；
- 把异步未完成、分页未耗尽和批量部分成功保留下来；
- 让 Agent 可以只解析一种 v2 JSON 信封；
- 通过逐命令 rollout 控制兼容风险。

它不把下列业务判断伪装成框架能力：

- API success 是否等于资源最终生效；
- API failure 是否保证未产生任何写入；
- 搜索空结果是否证明业务数据不存在；
- 服务端索引、目录或事务是否健康。

## 3. 目标与非目标

### 3.1 目标

- 所有统一 JSON 结果具有稳定的 `ok/outcome/data|error/meta` 信封，协议版本不进入运行时业务结果。
- 同一命令的一次执行只产生一个 primary result。
- `ok`、`outcome`、exit code 三者不可分叉。
- 失败、部分成功、异步受理、分页停止均可被 Agent 可靠分支。
- product handler 不直接写机器结果；统一 emitter 负责渲染。
- `dws dev ...` 与 `dws devapp +...` 按 terminal command 独立迁移。
- legacy 命令在迁移前保持原有 stdout/stderr/rc。
- rollout 可观察、可审计、可按命令回滚。

### 3.2 非目标

- 不提供 `--output-contract legacy|v2`。
- 不做会话级 contract negotiation。
- 不要求消费者把 `devapp +...` 改成 `dev ...`，也不做前缀 redirect。
- 不在本 RFC 中定义通用 `changed/verified` 自动校验。
- 不在本 RFC 中修复服务端“报错但已写入”、索引假阴或目录死条目。
- 不把 `dws dev connect` 前台事件流强塞进单结果 JSON 信封。
- 不宣称 MCP 已完成 v2 接入；当前只有 adapter 原型。

## 4. 与 Lark CLI、GWS 的对齐

### 4.1 取舍矩阵

| 主题 | Lark CLI | GWS | DWS Framework 2.0 |
|---|---|---|---|
| 机器输出 | 统一 envelope 和 emitter | 成功、错误、下载元数据均为结构化 JSON | JSON 使用固定 v2 envelope |
| contract 选择 | 框架决定 | CLI 固定输出，不要求消费者协商协议 | 每条命令、每个 release 唯一 active contract |
| 部分成功 | 一等 partial 通道 | 无统一四 outcome 模型 | `partial_failure` + 三通道 + rc 7 |
| 异步未完成 | task/ready/next command | 依具体 API | `pending + operation + next_command` |
| 分页 | complete 仅指 endpoint exhausted | Discovery/API 分页 | 显式 `endpoint_exhausted + next_token` |
| 格式入口 | 框架统一 format | 结构化 JSON/NDJSON | Agent 只用 `--format json`；其他 format 是 presentation |
| 机器错误流 | Lark 的 typed error 通常写 stderr | 所有结果保持结构化 JSON | v2 JSON/NDJSON primary result 统一写 stdout |
| 发现模型 | typed shortcut + runtime introspection | Discovery 动态生成 | DWS 声明式 ContractFinal + 运行时 Catalog |

DWS 借鉴 Lark 的结果语义、强类型 DTO、部分成功和异步恢复模型；借鉴 GWS 的“机器结果是结构化 JSON、日志走 stderr、消费者不选协议”原则。DWS 不照抄两者：它保留自己的静态 Agent Schema、安全声明和四 outcome 信封。

机器失败在 stdout 是 DWS 的显式选择：Agent 无需先根据成功/失败猜测读取哪个流；非零 exit code 仍是第一分支条件。stderr 只承载诊断。人读格式仍可把错误写 stderr。

参考：

- [Lark CLI envelope](https://github.com/larksuite/cli/blob/v1.0.84/internal/output/envelope.go)
- [Lark CLI runner / partial result](https://github.com/larksuite/cli/blob/v1.0.84/shortcuts/common/runner.go)
- [Google Workspace CLI README](https://github.com/googleworkspace/cli/blob/main/README.md)
- 仓内对比：[command-framework-comparison.md](../command-framework-comparison.md)

## 5. 总体架构

```text
LeafSpec / Shortcut / bare Cobra migration adapter
                  |
                  v
            corecmd.Spec
       declaration + safety + rollout
                  |
                  v
          handler executes once
                  |
                  v
          output.CommandResult
                  |
                  v
          context ResultStore
                  |
                  v
       root PersistentPostRunE / error exit
                  |
                  v
       ValidateResult -> EmitResult
        |                       |
        v                       v
 primary result stream      diagnostic stderr
        |
        v
     process rc
```

代码落点：

| 职责 | 当前路径 |
|---|---|
| 统一命令构建与 per-command rollout | `internal/corecmd/corecmd.go` |
| v2 envelope 与不变量 | `internal/output/envelope.go` |
| immutable result / ResultStore | `internal/output/result.go` |
| emitter、format、流与退出码 | `internal/output/emitter.go` |
| rollout state / active contract | `internal/output/rollout.go` |
| root 单一进程出口 | `internal/app/root.go` |
| typed errors / rc | `internal/errors/` |
| LeafSpec adapter | `internal/helpers/leaf.go` |
| Shortcut adapter | `internal/shortcut/adapter.go`、`runner.go`、`types.go` |
| dingtalk-dev 原子命令 | `internal/helpers/devapp.go`、`devdoc.go`、`connect_daemon.go` |
| HTTP timeout/retry | `internal/transport/client.go` |
| MCP adapter 原型 | `internal/output/mcp.go` |

### 5.1 框架不变量

```text
I1: ok == (outcome in {success, pending})
I2: process_exit_code == 0 <=> ok == true
I3: top-level error is present <=> outcome == failure
I4: one invocation emits exactly one primary result
I5: top-level keys do not expose a runtime contract selector or version marker
```

`partial_failure` 没有顶层 `error`；逐项错误位于 `data.failed[].error`。任何无法通过严格校验的结果都按 framework internal error 处理，禁止输出半合法信封。

### 5.2 结果所有权

- product command 只构造业务 DTO 或 `CommandResult`。
- 框架计算 `ok` 和 exit code，业务代码不能覆盖。
- `CommandResult` 在构造和取出时深拷贝可变 DTO，避免业务代码在入库后修改 wire 结果。
- `ResultStore` 一次只接受一个结果；重复写入和 v2 命令无结果都视为 internal error。
- emitter 先在 buffer 内完整渲染，再一次写出，避免半截 JSON。
- 输出 sink 的关闭失败不得把已经发出的 success 改成 failure，也不得造成第二次 legacy error 输出。
- v2 命令 panic 时由 root 转为一个 `failure/internal` 结果；不得先写 success 再 panic 后双发。

## 6. 唯一 active contract 与内部 rollout

### 6.1 用户和 Agent 表面

v2 命令的标准调用只有：

```bash
dws dev app list --format json
dws devapp +list --format json
```

禁止新增或公开：

```text
--output-contract
--contract-version
--v2
--legacy-output
```

Framework 2.0 也不新增 `--json` 别名。历史命令自带的局部 `--json` 只作为 legacy 兼容事实存在，不进入新的 Agent 示例；迁移后的标准写法始终是 `--format json`。

### 6.2 内部状态机

| 状态 | active contract | 行为 |
|---|---|---|
| `legacy_only` | legacy | 仅 legacy renderer；保存原 golden |
| `dual_validate` | legacy | 业务只执行一次；后台构造并校验 v2；stdout 仍是 legacy |
| `v2_active` | v2 | 本 release 默认 v2；收集 consumer/contract 指标 |
| `v2_stable` | v2 | 通过 soak、回归和兼容门禁 |
| `v2_only` | v2 | 删除该命令的 legacy renderer；命令路径不变 |

合法前进只能逐级进行。回退必须附带显式审批和原因，但仍通过代码/发布完成。运行时没有双 contract 分支，也没有会话协商。

`internal/output/rollout.go` 的 `ActiveContract` 是运行时唯一判定；`LeafSpec.OutputRollout` 或 `Shortcut.OutputRollout` 是命令声明。Schema、help 和 skill 不创建第二份 active 状态权威。

### 6.3 rollout ledger

当前状态必须来自 live command declaration。ledger 是审计产物，不是第二运行时配置源：

1. CI 从真实 Cobra tree 导出排序后的 `cli_path -> rollout_state -> active_contract` inventory。
2. CI 与 merge base 的 inventory 比较，调用 `ValidateRolloutTransition` 拒绝跳级或无审批回退。
3. transition evidence 只记录 owner、测试报告、兼容样本、观测窗口和回滚人，不手写重复的 current state。
4. release artifact 保存 inventory，便于定位“哪个 release 把哪条命令切到 v2”。
5. `dual_validate` 失败或 v2 指标恶化时，阻止晋级；不能用用户 flag 绕过。

现有 `ValidateRolloutTransition` 只有库函数和单测，尚未接入真实 CI ledger；`scripts/policy/` 下的统一输出扫描仍是 prototype。两项是 RFC 落地门槛，不得在文档中宣称已完成。

## 7. v2 统一结果契约

### 7.1 顶层信封

```json
{
  "ok": true,
  "outcome": "success",
  "data": {},
  "meta": {}
}
```

稳定顶层键为：

```text
ok, outcome, identity, dry_run, data, meta, error, _notice
```

空的可选字段省略，不输出 `null`。`ok`、`dry_run`、`retryable`、`timed_out`、`endpoint_exhausted` 必须是 JSON boolean，禁止字符串布尔。

### 7.2 四种 outcome

| outcome | ok | rc | 含义 |
|---|---:|---:|---|
| `success` | true | 0 | 请求完成，业务载荷在 `data` |
| `pending` | true | 0 | 请求已受理，但操作尚未终结 |
| `partial_failure` | false | 7 | 批量操作同时存在成功与失败/未知 |
| `failure` | false | 非零 | 请求、门禁、鉴权、网络或业务失败 |

### 7.3 success

```json
{
  "ok": true,
  "outcome": "success",
  "data": {
    "apps": []
  },
  "meta": {
    "count": 0,
    "pagination": {
      "endpoint_exhausted": true
    }
  }
}
```

空列表是成功，但不能伪装数据覆盖范围。`endpoint_exhausted` 只表示服务端分页端点已耗尽，不表示索引健康或全库无数据。

### 7.4 pending

```json
{
  "ok": true,
  "outcome": "pending",
  "data": {
    "accepted": true
  },
  "meta": {
    "operation": {
      "id": "task_xxx",
      "state": "processing",
      "timed_out": true,
      "next_command": "dws dev app robot result --task-id task_xxx --format json"
    }
  }
}
```

`pending` 必须携带非空 `operation.id`、`state`、`next_command`。轮询等待超时仍是 pending；`timed_out:true` 不得与 `completed/success` 状态共存。

### 7.5 partial_failure

```json
{
  "ok": false,
  "outcome": "partial_failure",
  "data": {
    "total": 3,
    "succeeded": [
      {"id": "a"}
    ],
    "failed": [
      {
        "id": "b",
        "error": {
          "type": "api",
          "message": "permission denied"
        }
      }
    ],
    "unknown": [
      {
        "id": "c",
        "reason": "request timed out after submission"
      }
    ]
  }
}
```

硬规则：

- `total == len(succeeded) + len(failed) + len(unknown)`；
- `succeeded` 至少一项；全部失败使用普通 `failure`；
- 三通道的同一 id 互斥；
- `failed[].error` 必须存在并通过 typed error 校验；
- `unknown[]` 只放无法确认终态的条目，必须说明 reason；
- 已成功对象必须完整保留，防止 Agent 重试整个批次。

### 7.6 failure

```json
{
  "ok": false,
  "outcome": "failure",
  "error": {
    "type": "api",
    "subtype": "rate_limit",
    "exit_code": 1,
    "message": "too many requests",
    "retryable": true,
    "retry_after_seconds": 30,
    "request_id": "req_xxx"
  }
}
```

`type/subtype/code/retryable/retry_after_seconds` 是 Agent 可分支字段；`message/hint` 是说明文本，不得作为稳定分支条件。上游 HTTP、RPC、服务端 code 和 trace id 应尽可能无损投影。

单次写操作出现“可能已提交但响应未知”时，不得暗示“失败即无变更”：有可恢复 task id 时返回 pending；无 task id 时 failure 必须带 `execution_started:true`、稳定 request/idempotency 线索和恢复提示。批量操作使用 `unknown[]`。

## 8. stdout、stderr、format 与退出码

### 8.1 流契约

| 场景 | stdout | stderr |
|---|---|---|
| v2 JSON success/pending/partial/failure | 唯一 primary JSON result | 诊断、warning、progress |
| v2 NDJSON success | 裸 records，一行一条 | 分页诊断等 |
| v2 NDJSON failure | 单行完整 failure envelope | 诊断 |
| table/pretty/raw/csv | presentation data | 诊断；人读 failure 可写此处 |
| legacy | 保持旧行为 | 保持旧行为 |

`--format json` 是 Agent 的唯一规范模式。NDJSON/CSV/table/pretty/raw 是 presentation，不另定义 output contract；运行时结果不携带协议版本选择或版本标记。

JSON/NDJSON 的 failure 绕过 `--jq` 和 `--fields`，避免过滤器擦除 typed error。日志、`[INFO]`、进度和 ANSI 控制字符不得进入机器 stdout。

### 8.2 退出码

| rc | 类别 |
|---:|---|
| 0 | success / pending |
| 1 | API |
| 2 | auth |
| 3 | validation；`confirmation_required` 是其 subtype |
| 4 | permission / PAT |
| 5 | internal / 未知类别 |
| 6 | discovery |
| 7 | partial_failure |

实现 `ExitCoder` 的既有特殊状态（例如用户中断 130）必须保留原 rc，并把相同值写入 `error.exit_code`。禁止 envelope rc 与进程 rc 分叉。

PAT 的 raw stderr JSON 是兼容性例外：`RawStderrError` 在完成专门迁移前保持原始 stderr 与 rc 4，不得被二次包装或打印两次。它属于认证宿主协议，不代表 MCP/v2 输出已完成统一。

## 9. pagination、dry-run、retry 与 timeout

### 9.1 pagination

分页只有两种合法状态：

```text
endpoint_exhausted=true  => next_token 必须缺席
endpoint_exhausted=false => next_token 必须非空
```

`pages/items/count` 不得为负。不要恢复语义过宽的 `complete:true`。

### 9.2 dry-run

- 支持 dry-run 的命令返回 `success + dry_run:true + data:<preview>`。
- dry-run 必须保证不跨越写副作用边界；不得先执行再声称 preview。
- 参数、目标资源和契约校验仍需执行；无法预演时返回 typed validation failure。
- dry-run preview 不等于业务终态验证，也不产生 `verified:true`。
- 新 Agent 使用 `--dry-run --format json`；诊断预览走 stderr，结构化 preview 走 data。

### 9.3 retry

- `error.retryable` 描述 Agent 是否可重试，不表示 CLI 已自动重放。
- MCP `tools/call` 可能是写操作；transport 无法证明幂等时必须 `MaxRetries=0`。
- discovery/read-only 基础请求可做有界重试，但必须遵守 context deadline。
- 写命令只有声明幂等、提供稳定 idempotency key 且请求体可安全重放时，才允许 opt-in 自动重试。
- 429/限流透传 `Retry-After` 为 `retry_after_seconds`；实际 sleep 可钳制，wire 原值不可被静默改写。
- context cancellation 不标 retryable；瞬时网络/timeout 可标 retryable，但由调用方决定是否再次执行。

现有基础：`internal/transport/client.go` 已对 `tools/call` 禁止自动重放，对 discovery 保留有界重试，并将 Retry-After 纳入退避。

### 9.4 timeout

- HTTP 请求预算使用 context deadline；`http.Client.Timeout` 保持 0，避免把健康的 body/流式读取粗暴截断。
- 根级 `--timeout` 当前默认 30 秒，是请求预算，不是异步任务最终完成预算。
- 调用方已有 deadline 时框架不延长、不覆盖。
- 异步 polling 的等待预算由具体命令另行声明；耗尽预算返回 pending，而不是伪装 success。
- `dws dev connect` 前台长连接使用独立 stream 生命周期，不纳入单请求 timeout/result contract。

## 10. dingtalk-dev 命令表面与渐进迁移

### 10.1 两套既有前缀都保留

以下是两个独立且稳定的 command surface：

```text
dws dev app list ...
dws devapp +list ...
```

它们可能访问同一后端能力，但并不是 Cobra alias，也不互相 redirect：

- `dev ...` 通常是原子、完整参数表面；
- `devapp +...` 通常提供精选参数、投影或多步编排；
- 每条命令拥有自己的 identity、Safety、Selection、Schema 和 rollout state；
- 用户显式选择的前缀永远有效；
- 输出 v2 迁移不改变 argv。

为避免 Agent 在相近入口间摇摆，声明层记录同一 capability relation，同时保留各自 `UseWhen/AvoidWhen/SemanticDelta/PrimaryCommand`。该关系只影响选择说明，不合并 identity，不自动改写命令。语义不完全等价时禁止声明为 alias。

### 10.2 当前 rollout 范围

| command family | 当前/目标 | 说明 |
|---|---|---|
| `dev app ...` terminal leaves | v2 | `devAppLeafMeta` 已按 leaf 标记 rollout |
| `dev doc search` | v2 | 单结果终态命令 |
| `dev connect status/stop/restart/list` | v2 | 本地终态、可表达为单结果 |
| `dev connect` foreground stream | legacy | 长连接事件流，等待 stream contract |
| `devapp +...` shortcuts | 逐命令原位迁入 v2 | 需要 Shortcut adapter 支持 per-command rollout/result |

兼容裁决：`dev connect stop/restart` 保持旧版无需确认即可执行的 argv 与运行时行为。两条命令的 `confirmation` 均声明为 `not_required`，调用不需要 `--yes`；`--dry-run` 仍必须无副作用。输出迁移不得借安全元数据改变这一既有行为。

仓库当前 `schema_command_exclusions.go` 的 `devapp-legacy-shortcuts` 组会把若干 `devapp +...` 排除在 Agent Schema 外，这与“保留当前 Agent 使用的 shortcut 并原位迁移”目标不一致。新 Agent 发布前，所引用 shortcut 必须：

1. 声明独立 `OutputRollout`；
2. 在原路径接入 `CommandResult/ResultStore`；
3. 通过 v2 contract tests；
4. 从 exclusion 组按精确路径移除；
5. 保持既有命令名和参数兼容。

### 10.3 Shortcut adapter 目标形态

`Shortcut` 需要与 `LeafSpec` 对称，而不是全局一次性切换所有 shortcut：

- 增加 per-command `OutputRollout` 声明；
- `FromShortcut` 把状态透传到 `corecmd.Spec`；
- `RuntimeContext.Output(payload)` 在 v2 active 时包装为 `Success(payload)` 并 `StoreResult`，legacy 时保持旧 renderer；
- 增加显式 `OutputResult(CommandResult)`，供 partial/pending 等非普通 success 使用；
- `dual_validate` 业务执行一次，只 shadow-build/validate v2；
- 未声明状态 fail closed 为 `legacy_only`。

禁止通过“所有 shortcut 自动 v2”完成迁移；每条 terminal command 都要有独立 golden、Schema 和风险评审。

### 10.4 推荐迁移批次

1. **框架闭环**：root 单结果出口、严格校验、stdout/stderr、sink cleanup、panic/PAT/custom rc。
2. **已有 dev 终态命令**：固定 `dev app`、`dev doc search`、connect 管理子命令的 v2 golden。
3. **Agent 已使用的 devapp read shortcuts**：`+list/+get/+event-list/+member-list/+permission-list/+version-*` 等原位迁移。
4. **devapp write shortcuts**：`+create/+update/+enable/+disable/+delete/+member-*`，先锁定 confirmation/dry-run/idempotency。
5. **复杂和异步命令**：引入 pending/partial/unknown，不把超时伪装成 failure 或 success。
6. **stream 与 MCP**：分别立项，不阻塞单结果命令的渐进发布。

## 11. CLI 与 MCP 边界

`internal/output/mcp.go` 已提供 `AdaptMCP(CommandResult)`，可以把同一 envelope 映射到 MCP `structuredContent`，并令 failure/partial 对应 `isError:true`。但当前生产代码没有调用该 adapter，只有单元测试。

因此本 RFC 当前只承诺 CLI adapter。MCP 后续接入必须：

- 从同一个 `CommandResult` 投影，禁止重建第二套业务信封；
- 验证 `structuredContent` 与 CLI 使用同一 `ok/outcome/data|error/meta` 信封；
- 明确 MCP transport error 与业务 `outcome` 的映射；
- 增加真实 server/stdio E2E；
- 在完成前，文档不得宣称 CLI/MCP 已同构上线。

## 12. CI、测试与发布门禁

### 12.1 静态门禁

- Schema/help 不得出现 `--output-contract` 或任何 contract selector。
- Agent skill 中 v2 示例只允许 `--format json`，不得新增 `--json`。
- v2 handler 禁止直接写 stdout、手拼 envelope 或字符串布尔。
- active v2 command 必须存在 `CommandResult` 生产路径。
- `Shortcut.OutputRollout` 与 `LeafSpec.OutputRollout` 都必须逐 command 声明。
- live rollout inventory 必须通过合法状态迁移检查。
- `dev` 与 `devapp +` capability relation 不得被登记成 CLI alias。
- Agent 引用的 runnable terminal command 必须进入 Schema，不得残留 exclusion。

### 12.2 动态 contract tests

每个迁移命令至少覆盖：

1. success JSON 恰好一个 document，且不带运行时协议版本字段；
2. validation/API/auth failure 为 typed v2 JSON，rc 与 `error.exit_code` 相等；
3. stdout 零日志、stderr 不含第二份 primary result；
4. `--format json | jq` 成功和失败都可解析；
5. `--output` file sink 下仍只有一个结果，close error 不反转已发 outcome；
6. active v2 命令未 StoreResult 时 fail closed；
7. panic 只产生一个 internal failure；
8. PAT RawStderr 保持 rc 4 且不双重包装；
9. custom exit code（如 130）不丢失；
10. pending 必须有 id/state/next_command；
11. partial 三通道、failed error、total 和 id 互斥严格校验；
12. pagination 两态与非负计数严格校验；
13. typed DTO、map、slice、pointer 构造后修改不影响 stored result；
14. NDJSON 一行一条、failure 单行合法；
15. timeout 不覆盖已有 deadline，`HTTPClient.Timeout==0`；
16. `tools/call` 不自动重放，Retry-After 可透传；
17. `dws dev ...` 和对应 `dws devapp +...` 均保持原 argv 可执行并独立返回 v2。

### 12.3 发布门禁

```text
make build
DWS_PACKAGE_VERSION=0.0.0-test go test ./...
./scripts/policy/check-generated-drift.sh
./scripts/policy/check-schema-catalog.sh
```

统一输出的 policy prototypes 在成为 mandatory gate 前必须：

- 纳入 `make policy`；
- 允许并检查非零 rc 的 failure/partial 样本，不能把它们全部 skip；
- allowed envelope key 与 `ok/outcome/identity/dry_run/data/meta/error/_notice` 一致；
- 动态样本使用 `--format json`；
- 同时覆盖 `dev` 和 `devapp +`；
- 失败时不得 vacuous pass。

## 13. 兼容与回滚

### 13.1 兼容原则

- 未迁移命令：legacy byte golden 不变。
- 迁移命令：release notes 明确该 command path 从 legacy 切换到 v2。
- 命令名、前缀、参数和安全语义不因输出迁移而改变。
- `dev connect stop/restart` 保持无需 `--yes` 或交互确认的旧行为。
- `devapp +...` 不因存在 `dev ...` 而被移除、隐藏或 redirect。
- consumer 只使用 `--format json`；命令在当前 release 的唯一 active contract 由命令声明决定，不通过响应字段协商。
- `v2_only` 只删除该命令内部 legacy renderer，不删除命令入口。

### 13.2 回滚

1. 指标或 consumer 回归触发 rollback review。
2. 将目标命令状态从 `v2_active/stable` 显式回退到已批准状态。
3. 重新发布；同一 binary 内仍只有一个 active contract。
4. ledger 记录原因、受影响版本、owner 和恢复条件。
5. 不向用户下发临时 `--legacy` 参数，也不依赖会话协商。

## 14. 风险与缓解

| 风险 | 缓解 |
|---|---|
| 输出变化破坏存量 parser | 逐命令 rollout、legacy golden、release notes、可发布回滚 |
| 同一能力存在 `dev`/`devapp +` 两个入口导致 Agent 摇摆 | capability relation + 精确 UseWhen/AvoidWhen/SemanticDelta；不假冒 alias |
| shortcut 绕过 v2 ResultStore | 为 Shortcut 增加对称 per-command rollout 和 OutputResult |
| 已发 success 后 cleanup 失败导致 rc 非零/双输出 | sink lifecycle 纳入 root 单一出口测试；已发结果不可反转 |
| failure 被 jq/fields 擦除 | failure 绕过过滤器，完整 typed envelope 输出 |
| 自动重试产生重复写 | `tools/call` 默认零重放；仅显式幂等命令 opt-in |
| 分页耗尽被误解为业务完整 | 使用 `endpoint_exhausted`，不承诺 coverage/index health |
| 长连接污染单结果 stdout | foreground connect 暂留 legacy，另立 stream contract |
| CLI 已 v2 被误写成 MCP 也完成 | 文档明确 `AdaptMCP` 尚未生产接入，单独 E2E gate |
| rollout state 多处重复 | live declaration 唯一权威；ledger 由命令树生成 |

## 15. 验收标准

Framework 2.0 的 dingtalk-dev 阶段在以下条件同时满足后验收：

- [ ] 无公开 `--output-contract`、别名、环境变量或 session negotiation。
- [ ] Agent 文档统一使用 `--format json`。
- [ ] 每个 terminal command 在一个 release 只有一个 active contract。
- [ ] 已迁移命令默认返回 `dws.output.v2`；未迁移命令保持 legacy。
- [ ] `dev` 和 Agent 当前使用的 `devapp +` 命令均在原路径独立完成 v2 迁移。
- [ ] 两套前缀均保留；无消费者改名前置要求，无 redirect。
- [ ] foreground `dev connect` 明确保持 legacy，管理子命令 v2。
- [ ] success/pending/partial/failure、stream、exit code 全矩阵通过。
- [ ] partial/pending/pagination/dry-run/retry/timeout 的严格测试通过。
- [ ] active v2 无结果、重复结果、panic、sink close、PAT、custom rc 都有 root E2E。
- [ ] live rollout ledger 和 transition CI 已接入，policy 不再是 prototype。
- [ ] Agent 使用的 devapp shortcut 已从 Schema exclusion 精确移除。
- [ ] legacy golden、v2 golden、Lark/GWS 对齐矩阵和 release note 完整。
- [ ] MCP 未接入的限制仍被准确说明；不得用 CLI 验收替代 MCP E2E。

## 16. 待实现清单（基于当前代码）

### P0

- 完成 Shortcut 的 per-command `OutputRollout`、`OutputResult` 和 ResultStore 接入；把 Agent 当前使用的 `devapp +...` 原位迁入 v2。
- 收口 root 的 emit/cleanup/panic 顺序，保证 `--output` 下仍恰好一个结果且 rc 不反转。
- 为 v2 failure、partial、pending、PAT RawStderr、custom rc 增加真实 root E2E。
- 修订 Schema exclusion：迁移一个 `devapp +...` 就精确移除一个，不做整组前缀隐藏。

### P1

- 把 rollout transition 与生成式 ledger 接入 CI。
- 把统一输出 policy scripts 接入 `make policy`，覆盖非零 rc 和非标准顶层字段。
- 完善 typed error 投影，保留 HTTP/RPC/upstream code、hint、request id 和 retry metadata。
- 严格校验 `failed[].error`、非负 count/pages/items。

### P2

- 为 deep copy 增加 typed pointer DTO、共享 slice view、error upstream payload 测试。
- 为 NDJSON failure 和分页诊断增加生产 adapter 回归。
- 另立 foreground stream contract RFC。
- 另立 MCP production adapter 接入计划。

## 17. 最终原则

```text
协议由 release 决定，不由 Agent 选择。
格式由 --format 决定，不用格式参数选择协议。
一个命令、一个 release、一个 active contract。
两套既有命令名可以共存，但不能伪装成 alias。
迁移改变输出，不改变用户 argv。
框架统一表达已知事实，不替业务层发明未知真相。
```
