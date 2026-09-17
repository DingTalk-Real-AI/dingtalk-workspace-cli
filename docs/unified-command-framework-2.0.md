# DWS 统一命令框架设计概要

> 状态：Framework core 已实现，dingtalk-dev/devapp 首批命令渐进接入中。本文定义框架能力、集成边界和首批 pilot 的发布纪律；其余产品命令迁移、Skill 更新和真实服务复验继续由后续 PR 独立完成。

## 1. 产品裁决

1. 不公开 `--output-contract`，也不增加任何等价别名。
2. Agent 继续只使用既有 `--format json`。
3. 每条 terminal command 在一个 release 中只有一个 active wire contract：已迁移命令直接使用统一结果，未迁移命令保持 legacy。
4. contract 不由用户参数、环境变量、会话能力协商或 Agent 选择。
5. 回滚是命令声明与发布行为，不改变消费者 argv。
6. 本 PR 只迁移完成命令级兼容审计的 dingtalk-dev/devapp pilot；其他命令路径、参数和输出保持不变。

## 2. 渐进迁移

内部状态机：

```text
legacy_only -> dual_validate -> unified_active -> unified_stable -> unified_only
```

- `legacy_only`：只构造、输出 legacy。
- `dual_validate`：业务只执行一次；外部仍逐字输出 legacy；同一内存结果 shadow-build 统一结果并严格校验。
- `unified_active`：`--format json` 直接返回统一结果信封，可按发布声明回退。
- `unified_stable`：完成真实 Agent 消费观察和兼容窗口。
- `unified_only`：清理仅服务 legacy 的产品 renderer。

状态是每条 terminal command 的内部发布元数据。Help、Skill、Agent Schema 不展示迁移状态，也不让消费者选择协议。

## 3. 统一结果

统一命令框架表达四类结果：

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

- 统一 JSON primary result 写 stdout；stderr 只写诊断。普通命令不把
  NDJSON 作为通用结果契约；持续事件流若需要逐事件输出，由 event 命令
  自己声明专用流协议。
- 分页统一输出到信封 `meta.pagination`，并在命令 Schema 中作为与 `result`
  同级的 `pagination` 能力声明；`result.data_schema` 只描述业务 data，不再
  混入分页控制字段。
- 日志不得污染 stdout。
- `ok`、`retryable`、`dry_run` 等必须是 JSON boolean。
- 失败由框架根据 typed error 映射退出码；产品代码不能自报任意 rc。
- `partial_failure` 保留 `succeeded[]/failed[]/unknown[]`，使用非零 rc 7。
- `pending` 必须提供 operation id、state 和可执行的 `next_command`。
- `endpoint_exhausted` 只表示观察到当前 endpoint 分页耗尽；false 必须带 `next_token`，不得扩大成索引健康或业务数据完整。
- dry-run 是已经完成的无副作用预览，表达为 `success + dry_run:true`，不是 `pending`。

## 5. 重试与超时边界

- 框架只统一表达 `retryable`、`retry_after_seconds` 和 `execution_started`，不自动决定业务操作能否安全重放。
- 写调用的模糊失败、HTTP timeout 和异步等待预算属于 transport/产品集成范围，不在本 PR 改动。
- 产品迁移必须证明其重试声明与幂等性、安全等级一致。

## 6. 集成范围

- 产品命令通过 `corecmd.ResultInvoke` 构造 `CommandResult`，由 root 单一出口渲染。
- 首批 dingtalk-dev/devapp 命令用于验证原子命令与 shortcut 的接入缝；未进入 pilot 的 shortcut、长连接、批量写和异步任务各自需要独立集成 PR。框架 core 不替产品推断 success、pending、partial 或分页事实。
- 每条 terminal command 独立 rollout；不能整域一次切换，也不能通过 Agent 参数选择协议。
- 已有命令在进入 `unified_active` 前必须保留 legacy byte golden，并完成真实 Agent 语义扫描。

## 7. 对齐原则

- 对齐 Lark CLI：统一 envelope/emitter、typed error、partial、pending、分页窄语义和强类型结果。
- 对齐 GWS：机器结果稳定结构化、日志与数据分流、消费者不协商协议版本。
- DWS 保留差异：声明式 Agent Schema、安全门禁、静态命令与 shortcut 共存，以及四 outcome 模型。

## 8. 发布门禁

命令晋级 `unified_active` 前至少满足：

1. success/failure/dry-run golden；批量或异步命令另有 partial/pending golden。
2. 业务请求 exactly once；dual validation 不得二次调用服务端。
3. legacy 命令 stdout/stderr/rc 字节级回归不变。
4. Help、Schema 和全仓示例不存在协议选择参数。
5. `--format json` 输出单个合法统一结果文档，stdout 无日志污染。
6. typed error、进程 rc 与信封 `error.exit_code` 一致。
7. 安全声明、确认门禁与 dry-run 运行时行为同源。
8. Agent 语义扫描记录命令级迁移证据；发布回滚无需修改 Agent argv。
