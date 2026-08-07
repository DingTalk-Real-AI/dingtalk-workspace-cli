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
| `retryable` | 服务端明确业务执行尚未开始，且允许重试 | 遵循 `retry_after`，最多有界重试一次 |
| `failed` | 已确认没有完成目标动作 | 根据 `retryable` 和 `suggestedActions` 决定是否重试 |

`ok=true` 或进程退出码为零不能替代 `status/verified`。写回执应保留 operation、target、steps、warnings、failures、compensation 和 verification 摘要。

## 分页与完整性

列表和搜索结果需要区分：

- `complete=true`：已证明覆盖请求范围；
- `hasMore=true`：仍有后续页，必须保留有效 continuation；
- `truncated=true`：因 maxPages/maxItems/timeout 等边界停止；
- `failures[]`：已返回部分数据但某一页或后处理失败。

只有 `complete=true` 且没有未处理失败时，才能把结果描述为完整集合。

## 错误

结构化错误至少区分 validation、not_found、ambiguous、type_mismatch、revision_conflict、confirmation_required、permission_denied、partial_success 和 commit_unknown，并提供 failure stage、retryable 和 suggested actions。权限、认证和参数错误直接进入 `failed`；写请求只有明确 `execution_started=false` 才能进入 `retryable`，其余传输异常进入 `unknown`。`retryable=false` 表示自动重放不安全，不代表用户检查状态后永远不能重新发起。

## 安全落盘

导出、下载和媒体预览只使用工作目录内相对路径。完成条件包括临时文件写入成功、内容校验、文件关闭和原子发布；失败时不得留下看似最终产物的半成品。默认 no-clobber，覆盖必须由用户显式选择。
