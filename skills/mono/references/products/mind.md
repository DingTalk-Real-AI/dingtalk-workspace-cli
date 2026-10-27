# 钉钉独立脑图

本页用于独立 `.amind` 脑图文件的带内容创建、主题查询和节点增删改。

| 目标 | 入口 |
|---|---|
| 创建带页面、主题和层级的独立脑图，或修改其中主题 | `dws mind` |
| 只在知识库创建空白脑图文件，不初始化主题内容 | `dws wiki node create --type amind` |
| 生成听记内置思维导图（结果在听记详情中） | `dws minutes +mindmap --id <taskUuid>`；multi 使用 dingtalk-minutes |
| 操作白板形状、坐标、连接线和样式 | `dws whiteboard` |

不能仅凭“OpenNodes”选择白板：脑图使用 `catalogVersion=mind-v1` 的页面/主题树，白板是不同协议。用户提供听记内容但明确要求另存独立脑图时，读取获授权的内容后走 `mind create`；不能用听记内置思维导图生成代替独立文件。若目标在听记内还是独立文件不明确，先消歧。

## 按需执行

只用 `dws`，结构化输出加 `--format json`；身份由登录/profile 和网关提供，不拼 HSF、HTTP 或 uid/组织参数。读写使用同一明确 profile，文件、页面、主题 ID 从真实返回取得。

已知命令直接使用。需要构造或理解 OpenNodes 时，读取 [脑图 OpenNodes 协议与十节点示例](./mind/open-nodes-v1.md)；普通查询不预读协议。参数或安全契约不明时查一次精确叶子 Schema，例如 `dws schema --cli-path 'mind node update' --compact --format json`。命令/参数漂移按公共 DWS 规则处理，不枚举全 Catalog。

写入按 Runtime/Schema 的确认要求执行；用户已确认同一目标、内容和影响时不重复索要确认，确认后才添加 `--yes`。下面示例不预置确认。不要求脑图执行白板的 render、+diff 或 sourceDigest 流程；脑图没有这些命令。可用 `--dry-run` 校验和预览请求摘要，`executed=false` 不代表远端已创建。

## 五个命令

| 命令 | 用途与关键输入 |
|---|---|
| `mind create` | `--name`、`--source`、`--request-id`；可选 `--workspace` 或 `--folder` |
| `mind node list` | `--node-id`；可选 `--mode`、`--page-id`、`--topic-id`、`--depth`、`--max-results`、`--next-token` |
| `mind node create` | 文件/页面/真实父主题 ID、`--topic` JSON 或文件、`--request-id`；可选 `--before-topic-id`、`--expected-version` |
| `mind node update` | 文件/页面/主题 ID、`--text`、`--request-id`；可选 `--expected-version` |
| `mind node delete` | 文件/页面/主题 ID、`--request-id`；可选 `--expected-version`；删除主题及整个子树 |

### 携带内容创建

先准备合法 source；名称最多 128 个 UTF-16 单元，不截断。示例中的文件须由本次任务准备：

```bash
dws mind create --workspace WORKSPACE_ID --name '项目计划' --source ./mind.json --request-id mind-create-001 --format json
```

`--source` 接受内联 JSON 或文件路径；CLI 校验后以 JSON 字符串发送，不手动再套一层 JSON 字符串。folder 与 workspace 同传时 folder 优先；它们不是同一种 ID。不传位置会创建到“我的文档”，创建成功不豁免后续组织和资源权限校验。`data.message` 是使用提示，不是错误或授权证明。

创建以 CLI `outcome=success`、`data.status=READY`、`data.requestedContentApplied=true` 确认。记录 `data.nodeId`、`data.pageIdMap`、`data.nodeIdMap`；临时 ID 不得当成后续真实 ID。需要验收初始化内容时按下面查询方式读回目标页。

### 查询、分页及写前定位

```bash
dws mind node list --node-id FILE_ID --format json
dws mind node list --node-id FILE_ID --page-id PAGE_ID --topic-id ROOT_TOPIC_ID --mode SUBTREE --depth 8 --max-results 200 --format json
```

- `--node-id` 是脑图文件 ID；`--page-id` 是真实页面 ID；`--topic-id` 是该页面中的真实主题 ID，三者不可互换。
- 默认 ROOT 发现各页及根主题；必须省略 topic-id，next-token 可省略或为空。NODE 读指定主题，CHILDREN 读直接子主题，SUBTREE 读起点及限定深度的后代；后三者均要求 page-id/topic-id。
- CLI 返回 `data.items[]` 页面及 `items[].nodes[]` 主题；从中取得真实 ID。MCP 的原生 pages/nodes 数组已由 CLI 按 pageId 归组，不需要对结果再做 JSON 字符串解码。
- depth 为 1..8，起点计第 1 层；max-results 为 1..200。一次命令只读一页，下一页使用 `meta.pagination.next_token`，保持身份、文件、页面、主题、模式、depth 和 max-results 不变。
- `endpoint_exhausted=true` 仅说明本次查询没有下一页；`data.truncated=true` 仍可能表示深度截断，不能宣称完整读取整棵树。非空 token 为 `mindc1_` 加 43 个 Base64URL 字符，不自行构造或修改。
- 写入前用已有的有效查询结果定位目标、确认 `writeSupport` 和版本；缺少时查询所需范围，不重复读取整个脑图。根主题可作为新增子主题的父节点，不能通过工具修改或删除。受保护、不支持写入的主题不强行覆盖。

### 新增、编辑与删除

```bash
dws mind node create --node-id FILE_ID --page-id PAGE_ID --parent-topic-id ROOT_TOPIC_ID --topic '{"id":"task-1","type":"topic","text":"任务"}' --request-id mind-add-001 --expected-version 4 --format json
dws mind node update --node-id FILE_ID --page-id PAGE_ID --topic-id TOPIC_ID --text '' --request-id mind-clear-001 --expected-version 5 --format json
dws mind node delete --node-id FILE_ID --page-id PAGE_ID --topic-id TOPIC_ID --request-id mind-delete-001 --expected-version 6 --format json
```

示例 request-id 仅展示格式；每个新操作使用独立的稳定标识，恢复沿用原值和全部参数。版本示例仅展示格式；执行时使用实际查询返回的 `data.readVersion`，不得照抄示例数字。新增返回的 nodeIdMap 将临时主题 ID 映射成真实 topicId；插入位置 before-topic-id 必须属于同一父主题。`--text` 必须显式提供，空字符串表示清空；不把缺失或 null 当清空。删除覆盖整个子树，先明确后代影响。

CLI 会把 topic 编码为 MCP 的 node JSON 字符串，把 text 编成 patch JSON 字符串；调用者不传 checkpoint、内部模型树或 HSF 请求类包装。

## 完成与恢复

- 节点写入以 `outcome=success`、`status=APPLIED`、`applied=true` 确认；编辑的 NO_CHANGE 表示服务端确认文本已一致，无须再写。删除数量仅在 deletedCountKnown=true 时可信。
- INITIALIZING / COMMITTED_UNVERIFIED 可返回 `outcome=pending`，不是完成。使用返回的恢复信息及 `meta.operation.next_command`，保留原 request-id 和全部业务参数；恢复命令会携带可解析的原 profile，多行内容使用 Bash/zsh 引用并保留尾部换行。其他 shell 请按原参数恢复并明确指定同一 profile；不自动循环重放，也不附加未经确认的 `--yes`。
- UNKNOWN、网络超时或效果未明时，不换 request-id 重新提交；原请求有界恢复后仍未知则停止并保留回执/trace。明确 CONFLICT 后先重新查询，再决定是否以新 request-id 发起新操作，不能把未知提交当冲突重做。
- 原始 MCP `success=true` 不等于写入已确认；以 CLI outcome、业务状态和确认字段联合判断。需要确认实际内容或排查未决状态时，执行同一文件的有界只读核对。
- 失败保留稳定错误码、阶段和 trace；公共 runner 提取的错误类别位于 `error.details.error_category`。TIMEOUT / VALIDATION / DOWNSTREAM 不授予自动重试权限；DOWNSTREAM 可表示权限、资源、版本、保护或功能未启用，不应统一解释成入参错误。
- mind.feature.disabled 时报告能力尚未启用或未放量，不修改开关、不换组织或调用入口绕过；权限拒绝也不切换目标或账号。message 不覆盖错误说明。
