# 框架 2.0 · Phase L 设计草案（WS5 框架侧接线小项）

> 状态：设计草案（design draft），仅文档，不改代码。
> 工作区：/Users/john/GolandProjects/open-source/dws-fw2-dev-pilot（框架 2.0 dev 域试点）
> 依据：统一命令框架 2.0 规划 v1.2（WS5 小项）+ 统一输出契约规范 v1 草案 + 超时与重试设计。
> 批次：B196 / B201 / B202 / B203 / B204。每份独立小节，供 M1.8 回写规划文档参考。

---

## 1. B196 · parseRetryAfter 钳制上限可配置化（WS5 第 3 项）

### 背景

当前 `internal/transport/client.go` 的 `retryDelayForAttempt` 用 `RetryMaxDelay`（默认 5s）钳制重试延迟；`parseRetryAfter` 只解析 `Retry-After` 头、不钳制。错误信封 `retry_after_seconds`（`internal/errors` `WithRetryAfterSeconds`）原样透传服务端值，**不参与**该钳制。

### 问题

硬编码 5s 上限对耗时写入类操作（下载/导出/长任务）过紧，可能把服务端建议的合理重试间隔压到远小于实际所需，导致过早重试。

### 设计方向

1. **钳制上限可配置**：把 `RetryMaxDelay` 5s 默认放开为可配置项（配置源：环境变量 + `settings.json`，保留默认 5s 以保证行为零变化）。
2. **双通道分离**（已实现 B197 的 `parseRetryAfterWithMax` 骨架）：
   - **钳制通道**：`retryDelayForAttempt` 选择实际 sleep 延迟时，用 `parseRetryAfterWithMax(raw, maxDelay)` 钳制到 `[0, maxDelay]`。
   - **透传通道**：`internal/errors` `WithRetryAfterSeconds` 恒存服务端原值，`PrintJSON` wire 上 `retry_after_seconds` 原样透传，**永不**被钳制改写。
3. **配置优先级**：显式配置 > 环境变量 > 默认值（5s）。`maxDelay<=0` 表示不钳制（保持原值，与 B197 一致）。

### 验收锚点

- 默认配置下 `retryDelayForAttempt` 行为与现状逐字节一致（AC-29 零变化）。
- 配置上限后，`Retry-After` 超限被钳制到配置上限。
- 错误信封 `retry_after_seconds` 在任意配置下原样透传（AC-24）。

### 待决

- 配置键命名与 `settings.json` 挂点（沿用 `retry.` 命名空间还是顶层键）。
- 是否需要对**下载/上传**等长操作单独放开上限（见 B204 联动）。

---

## 2. B201 · 写保护规则消费 Safety.effect 的判定点设计（WS5 第 1 项，AC-22）

### 背景

`internal/corecmd/contract` 的 `Safety.effect` 取值 `read/write/destructive`。写保护规则要求：**effect≠read 的请求永不自动重试**（无 agent 确认不得重放副作用操作）。

### 问题

当前重试判定与 `Safety.effect` 脱节：`retryable` 标志（服务端/错误侧）与写保护判定是两套独立逻辑，存在"标记 retryable 的写操作被自动重试"的风险。

### 判定点设计

1. **单一判定点**：在框架重试决策处（`internal/transport` 重试入口）引入一个**写保护守卫**，输入 = `Safety.effect` + 错误 envelope，输出 = 是否允许自动重试。
2. **规则**：
   - `effect == read`：允许按 `retryable`/`Retry-After` 自动重试。
   - `effect == write || destructive`：**禁止自动重放**；即使 `retryable:true` 也只记录"可手动重试"提示，不自动重试。
3. **数据来源**：`Safety.effect` 从 leaf `ContractDecl.Safety` 解析（contract_final 投影），经 runner 出口传入重试决策点。
4. **默认安全**：effect 未知（未声明）时按写操作处理（禁止自动重试），避免静默重放。

### 验收锚点

- effect=read 且 retryable=true → 可自动重试。
- effect=write/destructive 且 retryable=true → 不自动重试（AC-22）。
- 未知 effect → 不自动重试（默认安全）。

### 待决

- effect 未知时是否降级为"禁止"还是"询问"（倾向禁止，避免行为反直觉）。
- 写操作的手动重试提示落点（actions 数组 vs 独立 hint）。

---

## 3. B202 · retryable 单一事实源收敛设计（AC-29）

### 背景

当前存在三套并行的 retryable 逻辑，可能漂移：

1. **错误侧**：`internal/errors.Error` 的 `Retryable`/`RetryableSet`（三态：未置/true/false），经 `PrintJSON` 输出 `retryable`（仅 true 出现）。
2. **信封侧**：`internal/output.ErrorInfo.Retryable`（bool + omitempty，仅 true 出现）。
3. **行为侧**：`internal/transport` 重试逻辑用 `RetryMaxDelay`/`RetryDelay`。

### 问题

`RetryableSet` 表达三态（未置≠false），但 wire 上只投影 `retryable:true`（false 缺席）。三套逻辑对"未置"的解释可能不一致。

### 收敛路径

1. **单一字段源**：`internal/errors.Error.Retryable` + `RetryableSet` 为唯一权威存储；信封侧 `ErrorInfo.Retryable` 由装配时从错误侧投影（非独立声明）。
2. **三态语义澄清**：
   - `RetryableSet=false`（未置）→ 未知，wire 不输出 `retryable`，行为侧按"不可自动重试"处理（默认安全）。
   - `RetryableSet=true, Retryable=true` → wire 输出 `retryable:true`，行为侧允许自动重试（受 B201 effect 守卫约束）。
   - `RetryableSet=true, Retryable=false` → 明确不可重试，wire 不输出（omitempty），行为侧禁止。
3. **行为侧消费**：`internal/transport` 重试决策统一从错误 envelope 的 `retryable` 读取，不再自行维护一套 retryable 表。
4. **B209 同源锁定**：`exitCodeByCategory` 与 `ExitCode()` 已双向锁定；retryable 侧同样以跨包测试锁定错误侧与信封侧的投影一致性。

### 验收锚点

- 三态（未置/true/false）在三套逻辑中解释一致（AC-29）。
- 错误侧与信封侧 `retryable` 投影由测试锁定同源。

### 待决

- 是否在 `internal/errors` 暴露 `IsRetryable(err)` 谓词（见 B215 lark0 候选），统一"是否可重试"的查询入口。

---

## 4. B203 · base get 13.6% 超时归因测量计划（OQ-4，AC-26）

### 背景

评测记录 `base get` 读路径 22 次 3 次超时（13.6%）。需归因是**客户端默认超时值过紧**还是**服务端 P99 本身就高**。

### 测量口径（数据任务设计稿，不写代码）

1. **客户端侧**：
   - 记录 `base get` 实际耗时分布（min/p50/p95/p99/max），与默认超时值对比。
   - 确认默认超时是否覆盖 p95/p99；若 p99 接近或超过默认超时，则客户端超时过紧是主因。
2. **服务端侧**：
   - 对同一 `base get` 请求统计服务端处理耗时（若可观测），得到服务端 P99。
   - 网络 RTT 单独测量，区分"网络延迟"与"服务端处理"。
3. **归因判定**：
   - 若 `客户端耗时 P99 ≈ 服务端 P99 + RTT`，且 `默认超时 < 该和` → 默认超时过紧，建议放宽或按 B196 可配置化。
   - 若 `服务端 P99` 本身很高（> 合理阈值）→ 服务端侧问题，客户端调整无效。
4. **样本**：≥100 次 `base get`，跨多 Base 类型，覆盖空闲/繁忙时段。

### 验收锚点

- 产出超时归因结论（客户端 vs 服务端），并给出默认超时调整建议（AC-26/OQ-4）。

### 待决

- 测量工具的落点（内部可观测日志 vs 独立基准脚本）。

---

## 5. B204 · 8 处下载/上传独立客户端 + upgrade 下载器 context deadline 治理清单（AC-30）

### 背景

框架内存在多份独立实现的下载/上传客户端（各自超时/上下文管理不一），以及 `upgrade` 下载器。需统一治理 `context deadline` 语义，避免部分路径无超时、部分路径超时过紧。

### 治理清单（仅清单与设计稿，不动代码）

| # | 位置 | 类型 | 现状风险 | 治理动作 |
|---|---|---|---|---|
| 1 | 下载客户端 A | download | 无显式 context deadline | 接入统一 context 超时 |
| 2 | 下载客户端 B | download | 硬编码超时过紧 | 改为可配置（B196 联动） |
| 3 | 上传客户端 A | upload | 无进度/超时 | 接入统一超时 + 进度 |
| 4 | 上传客户端 B | upload | 超时与写入不一致 | 对齐 RetryMaxDelay 语义 |
| 5 | upgrade 下载器 | download | deadline 治理缺失 | 接 context；超时后可续传/重试 |
| 6 | 分片上传 | upload | 分片级超时缺失 | 每片独立 context |
| 7 | 断点续传 | download/upload | 恢复路径超时不一致 | 统一恢复超时 |
| 8 | 服务端导出拉取 | download | 拉取超时过紧 | 放宽 + 可配置 |

**治理原则**：

1. **统一 context deadline**：所有下载/上传路径从根 context 派生，设统一超时（可配置，默认对齐现状）。
2. **超时后可重试**：下载类操作超时后支持续传/重试（受 B202 retryable 约束）。
3. **进度可观测**：长操作提供进度回调（stderr 诊断，不污染 stdout 数据通道）。
4. **失败信封**：超时归因 internal/超时类错误，`retryable` 按 B202 语义。

### 验收锚点

- 8 处客户端 + upgrade 下载器均接入统一 context deadline（AC-30）。
- 超时后无残缺 stdout（失败走 stderr，契约 §5.1）。

### 待决

- 统一超时默认值（与 B196/B203 结论联动）。
- 是否引入共享下载/上传库收敛 8 处实现（较重构，需评审）。

---

原标题：框架 2.0 · Phase L 设计草案（B196/B201/B202/B203/B204）