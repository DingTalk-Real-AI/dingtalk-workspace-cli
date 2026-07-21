# Wukong 统一异步任务能力同步设计

- 状态：已确认
- 日期：2026-07-17
- 交付目标：追加到 GitHub PR #643 共同审核
- OpenDWS 开发基线：`2f0cd4fe4db45c7dcbcff7233a985f177f8bc08c`
- Wukong 参考提交：`fae989ccb163d0aa986283acefd113bddcfaf7c3`

## 1. 背景与结论

Wukong 在 `fae989cc` 中新增了 Drive 域统一异步任务查询能力，并同步调整了 Doc/Sheet 的导出与导入命令。OpenDWS 当前已经具备提交、轮询和下载的底层流程，但缺少以下完整命令契约：

- 没有 `dws drive task get` 统一查询入口。
- Doc/Sheet 导出和 Doc 导入不能选择“只提交、不轮询”。
- `doc export get` 仍以 `--job-id` 为公开参数，`sheet export get` 不存在。
- Doc 导入、Doc 导出和 Sheet 导出查询结果没有统一为稳定的任务结果结构。

本次实现以 Wukong 的对外能力为参考，不机械复制其内部代码。实现将复用 OpenDWS 已有的 `pkg/asynctask`，修正 Wukong 已知的状态归一化和帮助文案问题，并保持 OpenDWS 现有同步模式兼容。

最终代码追加到 PR #643 的源分支 `codex/sync-wukong-drive-sheet-parity`，与已有 Drive/Sheet 对齐能力一起审核。开发阶段继续使用隔离子分支，避免直接扰动正在审核的 PR。

## 2. 目标

1. 提供统一、可脚本化的异步任务查询入口。
2. 允许 Doc/Sheet 导出和 Doc 导入提交任务后立即返回。
3. 对外统一任务状态和 JSON 字段，避免调用方理解各 MCP 的不同原始响应。
4. 保留现有命令路径和旧参数兼容性，不改变未传 `--async` 时的默认行为。
5. 让新增命令可由 Cobra Help、Agent Schema 和 Skills 一致发现。
6. 在加入 PR #643 前完成自动化测试、真实命令验证和生成资产检查。

## 3. 非目标

- 不修改 Wukong 仓库。
- 不重复同步已经进入 OpenDWS 的 Contact 命令。
- 不新增或修改服务端 MCP 工具。
- 不把所有产品的异步任务统一到本次模型；范围仅限 Wukong `fae989cc` 涉及的 Drive/Doc/Sheet 流程。
- 不改变同步导出、导入的默认轮询与下载语义。
- 不强制 Doc 导入必须指定 `--folder` 或 `--workspace`；OpenDWS 继续保留当前可由服务端选择默认位置的行为，并修正与之冲突的帮助文案。
- 不把开发过程日志、临时验证数据或凭据提交到 PR。

## 4. 命令面设计

### 4.1 新增 `drive task get`

```bash
dws drive task get --type export --id <TASK_ID>
dws drive task get --type import --id <TASK_ID>
```

契约：

- `--type` 必填，只接受 `export` 或 `import`。
- `--id` 必填，表示服务端返回的任务标识；命令层统一称为 task ID。
- `export` 调用 Doc MCP 的 `query_export_job`，参数为 `jobId`。
- `import` 调用 Doc MCP 的 `query_import_task`，参数为 `taskId`。
- 两种类型都返回统一 `TaskResult`。
- Dry-run 只输出将要查询的类型和 ID，不调用 MCP。
- 参数缺失、类型非法、MCP 调用失败、业务级 `success=false` 或响应无法解析时返回非零退出码。
- 服务端返回了合法任务结果时，命令本身查询成功；调用方通过 `status` 判断任务状态。

### 4.2 Doc 导出

```bash
dws doc export --node <DOC_ID> --export-format docx --async
dws doc export get --task-id <TASK_ID>
```

变化：

- `doc export` 新增 `--async`。
- 同步模式保持 `--output` 必填，并继续执行“提交、轮询、下载”。
- 异步模式下 `--output` 不再必填；提交成功后立即返回 `PENDING` 的 `TaskResult`，不轮询、不下载。
- `doc export get` 新流程推荐 `--task-id`。
- 为保持 authoritative 历史命令合同，公开的 `--job-id` 继续可见、可执行；两者都提供时以显式冲突错误结束，避免静默选值。
- 查询结果由原始 MCP 响应改为统一 `TaskResult`。

### 4.3 Doc 导入

```bash
dws doc import --file ./report.docx --async
dws doc import get --task-id <TASK_ID>
```

变化：

- `doc import` 新增 `--async`。
- 异步模式仍需完成创建导入会话、上传文件、确认导入三步；拿到 `taskId` 后立即返回，不进入轮询。
- 未传 `--async` 时保持现有完整导入流程。
- `doc import get` 返回统一 `TaskResult`，不再透传大小写不稳定的原始状态。
- Doc 导入目标位置继续保持现有可选行为；帮助和 Skills 不再宣称 `--folder`/`--workspace` 至少一个必填。

### 4.4 Sheet 导出

```bash
dws sheet export --node <SHEET_ID> --async
dws sheet export get --task-id <TASK_ID>
```

变化：

- `sheet export` 新增 `--async`；提交成功后返回 `PENDING` 的 `TaskResult`。
- 新增 `sheet export get`。
- `sheet export get` 公开使用 `--task-id`，保留隐藏 `--job-id` 兼容别名。
- 未传 `--async` 时保持当前轮询、返回下载链接或下载本地文件的行为。

## 5. 统一结果与状态模型

统一输出：

```json
{
  "id": "task-id",
  "type": "export",
  "status": "SUCCESS",
  "resultUrl": "https://example.invalid/download",
  "createTime": "2026-07-17T10:00:00+08:00"
}
```

字段规则：

- `id`：命令输入或提交响应中的任务 ID。
- `type`：`export` 或 `import`。
- `status`：统一大写枚举。
- `resultUrl`：导出使用 `downloadUrl`，导入使用 `documentUrl`；无结果时省略。
- `message`、`createTime`：服务端有值时返回，无值时省略。

状态归一化规则：

| 原始状态 | 统一状态 |
|---|---|
| `PENDING`, `QUEUED` | `PENDING` |
| `PROCESSING`, `RUNNING`, `IN_PROGRESS`, `DOING` | `PROCESSING` |
| `SUCCESS`, `SUCCEED`, `SUCCEEDED`, `DONE`, `FINISHED`, `COMPLETE`, `COMPLETED` | `SUCCESS` |
| `FAILED`, `FAILURE`, `FAIL`, `ERROR` | `FAILED` |
| `TIMEOUT`, `TIMED_OUT` | `TIMEOUT` |
| 空值或未知值 | `PROCESSING` |

未知状态保守视为处理中，避免因服务端新增中间态而错误报告失败。`TIMEOUT` 必须显式映射；这修正了 Wukong 定义了枚举但遗漏映射的问题。

## 6. 代码结构

### 6.1 `pkg/asynctask`

扩展现有通用包，而不是复制 Wukong 的 `products/drive/task.go`：

- 继续复用已有 `Status` 枚举。
- 新增 `NormalizeStatus(raw string) Status`。
- 新增稳定的 `TaskResult` 结构。
- 为状态映射、JSON 字段和未知状态补齐表驱动测试。

该包只负责通用模型和轮询状态，不依赖 Cobra、helpers 或 MCP 调用，保持依赖方向单一。

### 6.2 `internal/helpers/async_task.go`

新增 OpenDWS 侧的任务查询适配层：

- 解析顶层响应和可选的 `result` 包装。
- 同时检查顶层与内层业务错误，避免解包后丢失 `success=false`。
- 分别提取导出和导入的结果 URL。
- 强制通过 `callMCPToolReturnTextOnServer(..., "doc", ...)` 调用 Doc MCP，避免 `drive task get` 根据当前产品错误路由到 Drive MCP。
- 接受可注入的调用函数，便于测试服务端、工具名和参数绑定。

### 6.3 现有流程接入

- `internal/helpers/drive.go`：注册 `task` 命令组和 `get` 叶子。
- `internal/helpers/doc.go`：接入 Doc export `--async`、参数别名和统一查询。
- `internal/helpers/import_flow.go`：通过配置项接入 Doc import `--async`，不复制上传流程。
- `internal/helpers/sheet_export.go`：接入 Sheet export `--async` 和 `get` 子命令。

`--format json` 下新增路径必须只在标准输出产生一个合法 JSON 文档；进度和后续提示不得污染 JSON 输出。

## 7. 兼容性与错误处理

- 未使用 `--async` 的用户行为不变。
- `doc export get --job-id` 继续可执行且保持公开，避免破坏历史 Help/Schema 合同。
- `sheet export get --job-id` 作为同样的隐藏兼容入口。
- 缺少任务 ID、同时传两个 ID 参数、非法 `--type` 均在 MCP 调用前失败。
- 非 JSON MCP 响应视为协议错误，不再伪装为成功并直接透传。
- `success=false` 使用服务端 message 构造可诊断错误。
- 查询接口中的生命周期 `status=ERROR` 不是传输/协议错误，必须归一化为
  `TaskResult.status=FAILED`；只有 `success=false`、非空 `error` 或错误码等
  envelope 信号才在解析前返回错误。
- 查询到 `FAILED` 或 `TIMEOUT` 时仍输出结构化任务结果；产品级 `get` 命令保留现有非零退出语义，统一 `drive task get` 作为状态查询在收到合法结果后返回成功，由调用方读取 `status`。
- 成功查询或同步导出可按 CLI 合同向直接调用者返回 `resultUrl`/`downloadUrl`；除此之外，进度、错误、测试日志、PR 证据和提交物不得记录登录凭据、OSS 临时凭证或完整的临时 URL。

## 8. Agent Schema 与 Skills

新增或变更的公开命令必须同步以下 reviewed source：

- `internal/cli/schema_command_registry.json`
- `internal/cli/schema_hints/metadata/{drive,doc,sheet}.json`
- `internal/cli/schema_hints/selection/{drive,doc,sheet}.json`
- 必要的 `internal/cli/schema_parameter_bindings.json`
- mono/multi Skills 中的 Drive、Doc、Sheet 参考文档

所有新增异步命令的 `--dry-run --format json` 统一输出单个、无副作用的 plan JSON；大小写和首尾空白格式值按统一 formatter 规则处理。预览和错误输出不得包含上传签名 URL、下载 URL 或临时凭证；显式成功结果仍按合同返回结果地址。

生成文件只通过 `go generate ./internal/cli` 或仓库生成目标产生，不手工编辑。新增公开叶子必须通过 Cobra 到 Agent Schema 的双向完整性检查。

Schema 默认只绑定“可执行且没有子命令”的叶子。为同时保留历史人类入口和
发布异步提交能力，采用 fail-closed 兼容模型：

- `doc export` 继续可直接执行；Schema 主路径为 `doc export create`。
- `doc import` 继续可直接执行；Schema 主路径为 `doc import create`。
- `sheet export` 继续可直接执行，并为保持历史 Schema identity 继续作为 `sheet.submit_export_job` 主路径；`sheet export create` 是公开等价 alias。
- 三个 `create` 叶子与各自父命令共享同一 handler 和公开 flags。
- runnable parent 不作为 Registry alias。唯一例外是已存在的历史 primary：必须同时注册至少一个经审阅的可执行 alias，且 binder 必须证明两者 handler、flags、required facts、Args、constraints 和 positionals 完全等价。
- `doc.query_export_job` 在 Schema 中保留公开 `job-id` 并增加 `task-id`；两者各自可选，Schema 通过 `require_one_of` 与 `mutually_exclusive` 精确表达“必须且只能提供一个”。兼容门禁只放行这一组参数别名迁移。
- `sheet.submit_export_job` 保留历史 `sheet export` 主路径，但从不准确的单一 `mcp` 映射迁为 `composite` 且不发布 `interface_ref`，因为真实流程包含提交、查询和可选本地下载。兼容门禁只放行该 canonical path 的精确 before/after 合同，其他字段变化仍失败。
- 查询入口仍为各自的 `get` 叶子。

## 9. 测试策略

实现遵循先失败测试、再最小实现：

1. `pkg/asynctask`
   - 全部状态别名，包括 `TIMEOUT`、`TIMED_OUT` 和未知值。
   - `TaskResult` JSON 字段和 `omitempty`。
2. 查询适配层
   - 顶层/内层 `result` 两种响应。
   - `success=false`、非法 JSON、缺少状态和 URL。
   - Export 使用 `jobId`，Import 使用 `taskId`。
   - 两种查询均路由到 Doc MCP。
3. Cobra 命令
   - 必填参数、非法类型、互斥别名、dry-run。
   - `--async` 不轮询、不下载，返回 `PENDING`。
   - 未传 `--async` 的旧行为回归。
   - JSON 输出不混入进度文本。
4. Schema/Skills
   - Help 路径和 flags 可发现。
   - CommandRegistry、参数映射、selection 和 safety 元数据一致。
   - 生成资产无漂移。

发布前验证命令：

```bash
gofmt -w <modified-go-files>
go test ./pkg/asynctask ./internal/helpers
DWS_PACKAGE_VERSION=0.0.0-test go test ./...
go build ./cmd
go generate ./internal/cli
./scripts/policy/check-generated-drift.sh
./scripts/policy/check-schema-catalog.sh
make policy
```

已知基线说明：未修改代码的 #643 基线在本机全量测试中出现 OAuth 本地回调测试超时，以及生成器被系统以 137 杀死；这两项需要在最终验证时单独复跑并与 CI 结果交叉确认，不能归因于本功能。

## 10. 真实命令验证

PR 更新前至少完成：

- 使用真实导出任务验证 `drive task get --type export`、`doc export get` 和 `sheet export get`。
- 使用真实导入任务验证 `drive task get --type import` 和 `doc import get`。
- 验证 `doc export --async`、`sheet export --async` 只提交不轮询。
- Doc import 会创建在线文档；只在明确的测试工作区执行，记录创建结果并完成清理。若没有安全测试目标，不擅自创建业务数据，并在 PR 验证说明中明确该项只完成自动化/契约验证。
- 下载地址只检查字段存在和可用性，验证记录中脱敏，不提交完整临时 URL。

## 11. 分支与 PR 交付

1. 在隔离分支 `codex/sync-wukong-async-task-parity` 开发和验证。
2. 设计说明先独立提交，作为本地实现检查点。
3. 实现完成后，将功能提交追加到 #643 源分支 `codex/sync-wukong-drive-sheet-parity`。
4. 若更新 #643 前需要重写远端历史或 force-push，必须再次取得明确授权；否则使用非破坏性的追加提交。
5. 更新 #643 的标题或描述，明确新增的异步任务范围和验证证据，不创建新的 PR。

## 12. 验收标准

- 本文列出的命令面变化均可通过真实 Cobra 命令发现和执行。
- 新旧参数兼容，默认同步行为无回归。
- 所有查询输出符合统一 `TaskResult`，状态映射覆盖 `TIMEOUT`。
- Agent Schema 和 Skills 可发现新增能力，生成资产通过检查。
- 聚焦测试、构建、策略检查通过；全量测试结果与已知基线问题分开报告。
- 安全范围内的真实命令验证通过，临时数据完成清理。
- 最终改动进入 PR #643，不创建重复 PR。
