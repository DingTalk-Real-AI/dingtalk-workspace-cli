# Doc Runtime Contracts

本文只定义 Agent 需要稳定消费的文档运行时语义。字段以 leaf Schema 和实际 JSON 返回为准；不要从终端展示文本反推状态。

## 目标

文档目标至少保留 `nodeId`、资源类型、canonical URL（服务返回时）和容器信息。名称或标题不是稳定身份。按自然标题解析出现零命中、多候选或分页不完整时，写操作必须停止。

## 写操作状态

| status | 含义 | 后续动作 |
|---|---|---|
| `success` | 所有计划步骤完成，且需要验证的内容已经回读 | 可以向用户报告完成 |
| `partial_success` | 已发生部分副作用，后续步骤失败 | 检查 `steps` 与 `compensation`，不得重放成功步骤 |
| `unknown` | 请求已发出但无法确定服务端是否提交 | 先回读目标；创建和追加禁止自动重试 |
| `retryable` | 服务端明确业务执行尚未开始，且允许重试 | 遵循 `retry_after_seconds`，最多有界重试一次 |
| `failed` | 已确认没有完成目标动作 | 根据 `retryable`、`actions` 和 details 决定是否重试 |

进程退出码为零不能替代业务证据。采用 `doc.operation.v1` 的 shortcut 回执固定提供 `contractVersion/ok/status/complete/operation/steps/data/warnings/compensation`；`target/failures/verification` 只有实际操作返回时才能消费，不是通用字段。`+import` 使用现有导入回执，成功时检查 `success=true`、`taskId`、`documentUrl`、`documentName` 与 `documentType`，不要要求不存在的 `status/steps`。业务 `status` 不应与框架外层 `outcome` 混为一谈。

稳定返回 ID/任务 ID 只用于定位目标、恢复流程或继续查询，不能单独证明写操作成功。成功证据优先级从高到低为：与该操作匹配的明确服务端成功终态 → 契约要求的写后读回匹配 → 仅 transport/退出码；异步任务必须使用返回的任务 ID 查询到成功终态。只有成功终态成立且所有必要回读均匹配时才报告成功；`partial_success`、`unknown`、未完成任务或仅返回资源/任务 ID 都不得报告完成。Runtime 已给出充分回读证据时，不为“再确认一次”重复请求。

## 分页与完整性

列表和搜索结果需要区分：

- `complete=true`：已证明覆盖请求范围；
- `hasMore=true`：仍有后续页，必须保留有效 continuation；
- `truncated=true`：成功返回因 `max_pages` 或 `max_items` 边界停止，并通过 `stopReason` 说明原因；
- 后续页请求失败、cursor 缺失/停滞/循环时返回 typed error；部分结果位于 `details.items`，并保留 `details.status=partial_success`、`complete=false`、`reason/page/nextCursor/count`。不要期待成功回执的空 `failures[]` 承载这类错误，也不要把 timeout 写成 `truncated`。

只有 `complete=true` 且没有未处理失败时，才能把结果描述为完整集合。用户问“有哪些”“全部”或要求“列出结果”时，使用返回的 cursor/continuation 继续取页直至完整；只有用户明确要示例、前 N 条或接受部分结果时才可提前停止，并说明已覆盖范围。分页应复用原查询与过滤条件，不得换命令或放宽关键词。

## 错误

结构化错误至少区分 validation、not_found、ambiguous、type_mismatch、revision_conflict、confirmation_required、permission_denied、partial_success 和 commit_unknown，并提供 failure stage、retryable 与已有的 `actions`/details。权限、认证和参数错误直接进入 `failed`；写请求只有明确 `execution_started=false` 才能进入 `retryable`，其余传输异常进入 `unknown`。`retryable=false` 表示自动重放不安全，不代表用户检查状态后永远不能重新发起。不要发明 `suggestedAction` 或顶层 `nextCommand` 字段。

## 安全落盘

导出、下载和媒体预览只使用工作目录内相对路径。完成条件包括临时文件写入成功、内容校验、文件关闭和原子发布；失败时不得留下看似最终产物的半成品。默认 no-clobber，覆盖必须由用户显式选择。
