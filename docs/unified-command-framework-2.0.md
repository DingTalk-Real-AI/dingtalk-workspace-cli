# DWS Unified Command Framework 2.0 设计概要

> 状态：以 [RFC-0001](./rfcs/0001-unified-command-framework-2.0-dev-gradual-rollout.md) 为权威设计。本文只提供评审入口和实施摘要；字段、状态机、兼容与验收细则以 RFC 为准。

## 1. 产品裁决

1. 不公开 `--output-contract`，也不增加任何等价别名。
2. Agent 继续只使用既有 `--format json`。
3. 每条 terminal command 在一个 release 中只有一个 active wire contract：已迁移命令直接 v2，未迁移命令保持 legacy。
4. contract 不由用户参数、环境变量、会话能力协商或 Agent 选择。
5. 回滚是命令声明与发布行为，不改变消费者 argv。
6. `dws dev ...` 和 `dws devapp +...` 两套现有前缀都保留并原位迁移；不改名、不 redirect、不要求 Agent 换前缀。

## 2. 渐进迁移

内部状态机：

```text
legacy_only -> dual_validate -> v2_active -> v2_stable -> v2_only
```

- `legacy_only`：只构造、输出 legacy。
- `dual_validate`：业务只执行一次；外部仍逐字输出 legacy；同一内存结果 shadow-build v2 并严格校验。
- `v2_active`：`--format json` 直接返回 `dws.output.v2`，可按发布声明回退。
- `v2_stable`：完成真实 Agent 消费观察和兼容窗口。
- `v2_only`：清理仅服务 legacy 的产品 renderer。

状态是每条 terminal command 的内部发布元数据。Help、Skill、Agent Schema 不展示迁移状态，也不让消费者选择协议。

## 3. 统一结果

Framework 2.0 统一表达四类结果：

```text
success          请求完成且命令认为操作已完成
pending          请求被受理，但异步操作尚未终结
partial_failure  批量操作有成功项，也有失败或未知项
failure          请求或操作失败
```

JSON 基本形态：

```json
{
  "ok": true,
  "outcome": "success",
  "data": {}
}
```

硬不变量：

```text
ok == (outcome in {success, pending})
process rc == 0 <=> ok == true
top-level error present <=> outcome == failure
one invocation emits exactly one primary result
```

框架负责 L1 request outcome 和 L2 operation outcome 的统一表达；L3 verification 必须由产品命令基于业务事实实现，框架不得自动推断 `changed/verified`。

## 4. 输出与错误纪律

- v2 JSON/NDJSON 的 primary result 统一写 stdout；stderr 只写诊断。
- 日志不得污染 stdout。
- `ok`、`retryable`、`dry_run` 等必须是 JSON boolean。
- 失败由框架根据 typed error 映射退出码；产品代码不能自报任意 rc。
- `partial_failure` 保留 `succeeded[]/failed[]/unknown[]`，使用非零 rc 7。
- `pending` 必须提供 operation id、state 和可执行的 `next_command`。
- `endpoint_exhausted` 只表示观察到当前 endpoint 分页耗尽；false 必须带 `next_token`，不得扩大成索引健康或业务数据完整。
- dry-run 是已经完成的无副作用预览，表达为 `success + dry_run:true`，不是 `pending`。

## 5. 重试与超时

- 自动重放默认关闭，尤其不得自动重放可能写入的 `tools/call`。
- 写调用发生网络超时、HTTP 5xx/408 等模糊失败时，不得声明 `retryable:true`；服务端明确拒绝执行的限流可透传 `Retry-After`。
- HTTP 请求使用 context deadline；已有调用方 deadline 时不叠加。
- `http.Client.Timeout` 不作为统一总预算，避免与 context 重复计时。
- 异步任务等待预算和前台事件流生命周期是业务层 timeout，不与单次 HTTP timeout 混用。

## 6. dingtalk-dev 首批范围

- `dws dev ...` 已迁移的 terminal leaf 直接 v2；未迁移 leaf 保持 legacy。
- `dws devapp +...` 保留原前缀并按 shortcut 独立 rollout；当前 Agent 高频入口必须原位接入 v2。
- 原子命令与 shortcut 共用同一个 dingtalk-dev payload-to-CommandResult 分类器，避免相同上游结果在两入口被判成不同 outcome。
- 带分页投影的 `+list/+permission-list/+event-list/+version-list` 在保留 `hasMore/nextCursor` 并完成对拍前不得晋级 v2_active。
- `dws dev connect` 同时承载前台 stream 与 `--daemon`，整条命令暂留 legacy；`status/stop/restart/list` 独立迁移。daemon start 如需 v2，应先拆独立 terminal command。
- `connect stop/restart` 保持旧版直接执行行为，不要求 `--yes` 或交互确认；两者仍声明为高风险本地副作用，dry-run 必须无副作用。

## 7. 对齐原则

- 对齐 Lark CLI：统一 envelope/emitter、typed error、partial、pending、分页窄语义和强类型结果。
- 对齐 GWS：机器结果稳定结构化、日志与数据分流、消费者不协商协议版本。
- DWS 保留差异：声明式 Agent Schema、安全门禁、静态命令与 shortcut 共存，以及四 outcome 模型。

## 8. 发布门禁

命令晋级 `v2_active` 前至少满足：

1. success/failure/dry-run golden；批量或异步命令另有 partial/pending golden。
2. 业务请求 exactly once；dual validation 不得二次调用服务端。
3. legacy 命令 stdout/stderr/rc 字节级回归不变。
4. Help、Skill、Schema 和全仓示例不存在 `--output-contract`。
5. `--format json` 输出单个合法 v2 文档，stdout 无日志污染。
6. typed error、进程 rc 与信封 `error.exit_code` 一致。
7. 安全声明、确认门禁与 dry-run 运行时行为同源。
8. rollout ledger 和 CI 禁止跳级；发布回滚无需修改 Agent argv。

完整实施计划、风险表和验收矩阵见 [RFC-0001](./rfcs/0001-unified-command-framework-2.0-dev-gradual-rollout.md)。
