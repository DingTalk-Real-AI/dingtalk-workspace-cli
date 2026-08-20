# AITable 低频原子能力索引

> 返回入口：[DingTalk AITable Skill](../SKILL.md)

本文件只用于根 Skill 和精确操作 Reference 都未覆盖的低频底层能力。Base/Table 创建、记录 CRUD、筛选排序、视图、导入导出和 Dashboard 必须返回根 Skill 的 Golden Route 或对应的精确 Reference，不在这里重新选路。

## 使用边界

1. 只有任务确实需要 Shortcut 未发布的底层字段、原始响应或运维控制时才读取本文件；
2. 先按下表返回根 Skill 或一个精确 Reference，不连读多个 Reference；
3. 已知命令只读取它的精确 leaf Schema；只有 Schema 与当前 Cobra 不一致时才读该 leaf Help；
4. 名称或 URL 目标仍必须解析为当前 profile 下的唯一稳定 ID，禁止选择第一个候选；
5. 原子写 leaf 的 confirmation 与对应 Golden Shortcut 不一致时停止，以 Runtime gate 和精确 leaf Schema 为准；
6. 完成后保留稳定 ID、验证证据、partial failure、checkpoint 和真实错误。

## 高频任务返回表

| 用户终点 | 返回入口 |
|---|---|
| Base 搜索、创建、复制、改名或删除 | 根 Skill Golden Route；参数不明时只读最终 leaf Schema |
| Table 创建、查看、复制、改名或删除 | 根 Skill Golden Route；参数不明时只读最终 leaf Schema |
| Field 创建、完整配置、改名或删除 | [field](aitable/aitable-field.md) |
| 公式或查找引用字段 | [formula-guide](aitable/aitable-formula-guide.md) |
| Record CRUD、历史、空行或分享 | [record-ops](aitable-record-ops.md) |
| Record 统计、分组聚合或去重率 | [record-stats](aitable/aitable-record-stats.md) |
| filters、sort 或日期操作符 | [filter-sort](aitable/aitable-filter-sort.md) |
| 记录主键文档 | [primary-doc](aitable/aitable-primary-doc.md) |
| 视图列、筛选、排序或分组 | [view-config](aitable/aitable-view-config.md) |
| 视图锁定、冻结列、行高或填色 | [view-extras](aitable/aitable-view-extras.md) |
| Dashboard 或 Chart | [dashboard-chart](aitable/aitable-dashboard-chart.md) |
| Form 创建、题目或分享 | [form](aitable/aitable-form.md) |
| 导入、导出或异步任务恢复 | [export-import](aitable/aitable-export-import.md) |
| 附件上传或移除 | [attachment](aitable/aitable-attachment.md) |
| Base 内 Section 或节点 | [section](aitable-section.md) |
| 自动化工作流 | [workflow](aitable/aitable-workflow.md) |
| 普通角色或高级权限 | [advperm](aitable/aitable-advperm.md) |
| 产品边界或相邻意图不明确 | [intent-guide](intent-guide.md) |

## 低频底层命令族

下表只是最后回退的导航，不是可预加载的命令目录。命中后必须读取精确 leaf Schema 确认当前参数和安全语义。

| 原子命令或命令族 | 仅用于 |
|---|---|
| `base get-primary-doc-id` | 统一 Base/Record 路由未投影所需底层主文档 ID 时 |
| `record get` | 必须获取单条记录原始响应，且 `+record-query --record-ids` 不能交付所需字段时 |
| `record stats` / `record group-stats` | 服务端统计或分组聚合，不用全量记录在本地重算 |
| `field search-options` | 只需在已知选项字段中搜索选项，不需要完整字段配置时 |
| `view get <facet>` / `view update <facet>` | 精确 View Reference 已选定某个底层属性，且无同构 Shortcut 时 |
| `form questions *` | 表单字段投影不能表达的底层题目创建或删除时 |
| `chart widgets-example` / `dashboard config-example` | 创建或修改前确实缺少当前运行时的合法配置形状时 |
| `workflow edit-example` | 工作流精确 Reference 仍无法表达当前编辑 payload 时 |
| `advperm *` | 普通角色 Shortcut 无法表达的高级权限控制，且安全边界已由精确 Schema 确认时 |

## 低频批处理脚本

只有对应精确 Reference 已选路、原生命令需要重复编排时才使用；脚本参数以 `--help` 和脚本内校验为准，不因脚本存在而跳过目标解析、确认或结果验证。

| 脚本 | 仅用于 |
|---|---|
| `python scripts/aitable_export_via_task.py <baseId> --scope all` | 导出任务需要轮询 `taskId` 并下载结果；表或视图范围继续按 [export-import](aitable/aitable-export-import.md) 传入稳定 `tableId` / `viewId` |
| `python scripts/bulk_add_fields.py <baseId> <tableId> fields.json` | 已完成字段类型与配置校验后，批量创建大量字段；少量字段仍走根 Skill 或 [field](aitable/aitable-field.md) |

## 稳定 ID 传递

| 来源 | 只可用于 |
|---|---|
| `+url-resolve` / `+resolve-base` / `+base-search` 唯一结果 | 当前 profile 下的 `baseId` |
| `+resolve-table` / `+list-tables` | 当前 Base 下的 `tableId` |
| `field list` / `+field-get` | 当前 Table 下的 `fieldId` |
| `record create` / `+record-query` | 当前 Table 下的 `recordId` |
| `view create` / `+view-get` | 当前 Table 下的 `viewId` |
| Dashboard 或 Chart 创建结果 | 当前 Base 下的 `dashboardId` / `chartId` |
| `dws doc info` | `folderId` 供 Base 复制目标使用；`nodeId` / `rootFolderId` 改传 `--target-folder-node` |

`baseId` / `tableId` / `fieldId` / `recordId` / `viewId` / `folderId` / `nodeId` / `spaceId` / `dentryId` 是不同类型，不得轮流代入试错。

## 故障处理

- `unknown command` / `unknown flag`：读取精确 leaf Help，最多做一次有证据的修正；
- confirmation 或参数约束不清：读取精确 leaf Schema，以 Runtime gate 为准；
- `partial_success`：保留已完成项和 checkpoint，只执行结果给出的继续或恢复命令；
- 写入结果为 `unknown`：先按稳定 ID 或业务唯一键回读，未确认前不重试非幂等写；
- `retryable=false` 或 ID 类型错误：停止，不换同义原子命令或其他 ID 类型试错；
- 部分成功：保留 completed/failed/unknown 明细，不表述为完整成功。
