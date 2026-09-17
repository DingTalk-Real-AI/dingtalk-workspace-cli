# Drive Shortcut 对齐与超越 Lark CLI

## 目标与判定口径

本轮以 Lark CLI `drive` 的 38 个 shortcut 为对照，但不把“同名命令数量”当完成标准。对齐按用户任务判定：

1. `drive +...` 有更稳定的 Agent 主入口时，提供 Shortcut，并发布 Selection、Safety、Result 与 Pagination。
2. 钉钉已经在其他产品提供更成熟入口时，Skill 明确跨产品路由，不在 Drive 重复实现。
3. 只有原子能力且 Shortcut 不增加校验、编排或投影价值时，保留 Runtime Schema leaf，不制造同义别名。
4. 下层接口不存在或无法满足相同语义时，明确记录 gap；不得用空数组、空对象或只返回任务提交结果伪装完成。

成功判定统一为：进程成功 + 统一结果 `ok=true/outcome=success` + 必要业务字段 + 真实数据读回或本地字节校验。服务端显式返回空数组可以是合法业务空结果；空响应、缺少数组、数组类型错误、坏元素、`success=false`、写入缺少终态证据都必须失败。

## Lark 38 项映射

| Lark Drive shortcut | DWS 路由 | 结论与原因 |
|---|---|---|
| `+upload` | `drive +upload` | 对齐并增强：工作目录边界、OSS PUT、严格 commit、元数据读回。 |
| `+create-folder` | `drive +create-folder` | 对齐并增强：要求新 fileId 和名称读回。 |
| `+create-shortcut` | `drive +create-shortcut` | 对齐并增强：明确 shortcut≠copy，创建后读回。 |
| `+download` | `drive +download` | 对齐并增强：真实落盘、no-clobber、原子发布、非零字节。 |
| `+preview` | `drive +cover`（有限） | 不完全对齐：钉钉当前只提供封面/缩略图读取，没有等价的服务端多格式预览转换接口。 |
| `+cover` | `drive +cover` | 对齐：严格读取封面/缩略图对象。 |
| `+add-comment` | `doc +comment-create` | 用户任务对齐；评论归在线文档协作域，Drive 不复制一套。 |
| `+list-comments` | `doc +comment-list` | 用户任务对齐；Doc 已有类型、状态与分页。 |
| `+batch-query-comments` | `doc +review` / `doc +comment-list` | 超越：可聚合未解决评论与确定性正文上下文；跨文档批量仍由调用方按节点编排。 |
| `+resolve-comment` | `doc +comment-update`（有限） | 部分对齐：DWS 可更新评论，但当前下层未声明独立 resolve 状态接口。 |
| `+restore-comment` | 无等价 | gap：钉钉当前下层未暴露恢复已删除评论的等价能力。 |
| `+add-reply` | `doc +comment-reply` | 对齐。 |
| `+list-replies` | `doc +comment-list` | 用户任务对齐：评论列表返回回复上下文；无独立 Drive reply 目录。 |
| `+update-reply` | `doc +comment-update` | 对齐到评论/回复统一更新语义。 |
| `+delete-reply` | `doc +comment-delete` | 对齐到评论/回复统一删除语义，高风险确认。 |
| `+react-reply` | `doc +comment-reply`（有限） | 部分对齐：支持表情回复；不声称拥有 Lark 的独立 reaction identity。 |
| `+export` | `doc +export` | 用户任务对齐并增强：提交、轮询、安全下载一体化。 |
| `+export-download` | `doc +export` / `doc +export-get` | 超越：常规一体化，`+export-get` 仅作中断恢复。 |
| `+import` | `doc +import` | 用户任务对齐并增强：转换白名单、上传 fallback、轮询终态。 |
| `+version-history` | `drive +version-history` | 对齐并增强：严格空结果与分页。 |
| `+version-get` | `drive +version-get` | 对齐并增强：精确版本号，零命中失败。 |
| `+version-revert` | `drive +version-revert` | 对齐并增强：版本预检、高风险确认、节点读回。 |
| `+version-delete` | 无等价 | gap：钉钉当前普通文件版本接口没有删除历史版本能力。 |
| `+move` | `drive +move` | 对齐；与 copy/shortcut 明确消歧并发布确认。 |
| `+delete` | `drive +delete` | 对齐；移入回收站、高风险确认、终态证据。 |
| `+status` | `drive +inspect` | 超越：远端身份、统计、公开状态和封面按需聚合；不伪装成本地同步状态。 |
| `+push` | `drive +upload`（单文件） | 部分对齐：单文件上传可靠；没有可靠的目录 diff、冲突和远端删除传播语义，因此不提供同名批量 push。 |
| `+pull` | `drive +download`（单文件） | 部分对齐：单文件下载可靠；目录级增量拉取需稳定路径、hash 与冲突策略，当前接口不完整。 |
| `+sync` | 无等价 | gap：在缺少稳定远端内容 hash、rename/delete journal 和冲突版本向量时，双向同步会有数据覆盖风险。 |
| `+task_result` | `doc +export-get` / 导入任务恢复入口 | 用户任务对齐；DWS 按任务所属产品提供类型化恢复入口，不保留 Lark 的下划线泛化命令。 |
| `+apply-permission` | `drive permission apply` raw leaf | 下层能力存在但未提升为 Shortcut：需要真实申请上下文和权限夹具，无法在通用 E2E 中安全创建。 |
| `+member-add` | `doc +access-grant` | 用户任务对齐并增强：解析接收人、批量 ledger、首次写入前停止。 |
| `+member-list` | `drive permission list` / `doc +inspect --include-permissions` | 对齐；常规 Agent 场景优先 Doc 聚合检查。 |
| `+permission-get-setting` | `drive permission list` + `drive +publish-get` | 部分对齐：协作者与互联网公开是两个独立安全域，没有一个与 Lark setting 完全同构的钉钉接口。 |
| `+secure-label-list` | 无等价 | gap：当前 DWS/钉钉下层没有可声明的 Drive 安全标签目录接口。 |
| `+secure-label-update` | 无等价 | gap：没有安全标签写接口，不能用普通权限或公开状态替代。 |
| `+search` | `drive +search` | 对齐并增强：过滤、严格数组和分页；在线文档搜索路由 `doc +search`。 |
| `+inspect` | `drive +inspect` | 对齐并增强：必达元数据 + 可选聚合，部分失败不伪装成功。 |

## DWS 超出 Lark Drive 的可挖掘能力

- `+list`：严格目录分页，而不是把缺字段当空目录。
- `+recent`：最近访问/编辑与创建人筛选。
- `+stats`：阅读、编辑、评论、点赞、预览和下载统计。
- `+recycle-list` / `+recycle-restore`：显式回收项身份与恢复后读回。
- `+star-list` / `+star-add` / `+star-remove`：个人收藏完整闭环。
- `+publish-get` / `+publish-unset`：互联网公开独立安全域与关闭后读回；`+publish-set` 保留为 unavailable 诊断入口。
- `+version-download`：历史版本真实字节下载与本地 artifact 校验。
- `+rename`：写后读回验证。

普通钉盘文件的独立 `copy` 是额外确认出的部分 gap：钉钉当前 `doc/copy_document` 对该对象会生成 `.dlink`，不是字节独立副本。`drive +copy` 因此只接受在线对象；普通文件需要快捷入口时用 `+create-shortcut`，需要独立副本时用 `+download` 后 `+upload`。这不是完整的服务端原子 copy，对大文件也不能宣称完全等价。

互联网公开开启也是账号/对象能力 gap：真实普通文件与在线文档夹具都由服务端返回 `operation.notSupported`。`+publish-get` 与关闭语义可验证，但 `+publish-set` 在找到 eligible 节点完成 set→get→unset 闭环前保持 `unavailable` 且不进入公开 Agent catalog。

## 端到端门禁

每个公开 Drive shortcut 必须至少覆盖：

- Cobra 参数、静态确认、Shortcut Execute、MCP 调度和最终输出；
- 明确业务空集合、空响应、缺字段、错误类型、坏元素、`success=false`；
- 写入的 ID/终态证据与读回不一致；
- 下载的本地路径边界、no-clobber、真实字节数；
- 真实账号数据：读命令必须命中已知非空夹具或明确验证合法空集合；写命令必须创建隔离资源、读回、必要时下载比对字节并清理。

发布前运行 `make build`、完整 Go 测试、Schema 生成/漂移/策略检查，并保存不含账号业务内容的结构化 E2E 汇总。
