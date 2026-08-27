# AITable 低频原子能力索引

> 返回入口：[DingTalk AITable Skill](../SKILL.md)

本文件只用于根 Skill 和精确 task reference 都未覆盖的低频底层能力。Base/Table
定位、建表、记录查询与写入、字段配置、视图编排、导入导出等常见任务必须回到根 Skill
的 Golden Route 或对应 task reference，不在这里重新选路。

## 使用边界

1. 先确认任务确实需要 Shortcut 未发布的底层字段、原始响应或运维控制；
2. 只读取精确原子 leaf Schema/Help，不加载产品级 Catalog 猜参数；
3. URL、名称和自然目标仍须解析为唯一稳定 ID，禁止选第一项；
4. 原子写 leaf 的 confirmation 若与对应 Golden Shortcut 不一致，停止并报告交付漂移；
5. 后续 ID 只使用当前 profile 的真实返回，不跨组织复用；
6. 完成后保留 verification、partial failure、checkpoint 和可继续编排的稳定 ID。

## 高频任务返回表

| 用户终点 | 返回入口 |
|---|---|
| URL/名称解析、Base 搜索、建 Base/Table、记录查询与 CRUD | 根 Skill Golden Route |
| 记录值格式、批量写、历史、分享和统计 | [record-ops](aitable-record-ops.md) |
| 筛选、排序和日期操作符 | [filter-sort](aitable/aitable-filter-sort.md) |
| 字段类型、创建与复杂配置 | [field](aitable/aitable-field.md) |
| 视图列顺序、筛选、排序、冻结和展示配置 | [view-config](aitable/aitable-view-config.md) |
| 表单字段、题目与分享 | [form](aitable/aitable-form.md) |
| Dashboard 与 Chart 配置 | [dashboard-chart](aitable/aitable-dashboard-chart.md) |
| 导入、导出和异步任务恢复 | [export-import](aitable/aitable-export-import.md) |
| 附件、工作流与高级权限 | 对应的 [attachment](aitable/aitable-attachment.md)、[workflow](aitable/aitable-workflow.md) 或 [advperm](aitable/aitable-advperm.md) |
| 记录主键文档 | [primary-doc](aitable/aitable-primary-doc.md) |
| Base 内 Section/节点编排 | [section](aitable-section.md) |
| 相邻产品或低频意图仍需消歧 | [intent-guide](intent-guide.md) |

## Base、Table 与 Field 底层能力

| 原子命令 | 仅用于 |
|---|---|
| `aitable base get` / `list` / `search` | Shortcut 未投影的 Base 原始详情、最近访问列表或原始搜索响应 |
| `aitable base create` / `copy` / `update` / `delete` | Shortcut 未发布的底层创建、复制或变更字段；普通整套创建回根 Skill |
| `aitable base get-primary-doc-id` | 需要 Base 视角的记录主键文档 ID |
| `aitable table get` / `list` | 需要原始 Table 结构或目录响应 |
| `aitable table create` / `update` / `delete` | Shortcut 未发布的底层 Table 字段；完整建表回根 Skill |
| `aitable field get` / `list` / `search-options` | 字段原始配置、目录或选项搜索 |
| `aitable field create` / `update` / `delete` | 精确字段原子写入，且字段配置已由 task reference 校验 |
| `aitable template search` | 需要模板原始响应；普通模板检索使用 `+template-search` |

`base list` 仅表示最近访问，不是组织内全量 Base。Table、Field 和记录 ID 均属于指定
Base；同名对象零命中或多候选时停止，不能把名称、URL 末段或其他产品节点 ID 直接当稳定 ID。

## Record 底层能力

| 原子命令 | 仅用于 |
|---|---|
| `aitable record get` / `list` / `query` | Shortcut 未投影的原始记录响应、显式 continuation 或窄 ID 查询 |
| `aitable record query-empty` | 查找未填写用户字段的空行 |
| `aitable record history-list` | 已知 recordId 的原始变更历史 |
| `aitable record share-url` | 已知记录的原始分享链接响应 |
| `aitable record create` / `update` / `batch-update` / `upsert` | Shortcut 未发布的底层写参数；写前须有真实 fieldId 和字段类型 |
| `aitable record delete` | 删除已唯一确认的记录，按最终 Runtime gate 执行 |
| `aitable record primary-doc-get` / `primary-doc-create` | 取得或创建记录主键文档；正文编辑转交 Doc |

`cells` 使用真实 fieldId；公式、查找引用、创建人和修改时间等只读字段不得写入。批量结果
必须检查 completed/failed/checkpoint，写入效果未知时先按稳定 ID 或业务唯一键回读，不盲目重放。

## View、Form 与可视化底层能力

| 原子命令 | 用途 |
|---|---|
| `aitable view list` / `get` | 原始视图目录或完整配置 |
| `aitable view create` / `duplicate` / `delete` / `lock` | Shortcut 未发布的视图生命周期与锁定字段 |
| `aitable view get aggregate` / `card` / `field-widths` / `fill-color-rule` / `filter` / `frozen-cols` / `group` / `lock` / `row-height` / `sort` / `timebar` / `visible-fields` | 读取单个视图配置面 |
| `aitable view update aggregate` / `card` / `field-widths` / `fill-color-rule` / `filter` / `frozen-cols` / `group` / `name` / `row-height` / `sort` / `timebar` / `visible-fields` | 更新一个已完整读取的视图配置面 |
| `aitable form list` / `get` / `create` / `update` / `delete` | 表单视图原子生命周期 |
| `aitable form field list` / `update` / `hide` | 表单字段顺序、展示和隐藏 |
| `aitable form questions create` / `delete` | 表单题目原子写入 |
| `aitable form share get` / `update` | 表单分享配置 |
| `aitable dashboard get` / `create` / `update` / `delete` / `arrange` | 仪表盘原始配置和布局 |
| `aitable dashboard share get` / `update` | 仪表盘分享配置 |
| `aitable chart get` / `create` / `update` / `delete` | 图表原始配置和生命周期 |
| `aitable chart share get` / `update` | 图表分享配置 |

视图更新是配置面写入，不是字段本体修改。调整可见列前读取完整有序 fieldId 数组并固定主字段；
创建或更新图表前使用 `aitable chart widgets-example` 获取当前合法配置，不猜 config。

## 导入导出、附件与自动化

| 原子命令 | 用途 |
|---|---|
| `aitable import upload` / `data` | 申请导入上传凭证并用真实 importId 发起导入 |
| `aitable export data` | 发起或恢复底层导出任务 |
| `aitable attachment upload` | 准备 AI 表格附件上传；不是 Drive 文件上传 |
| `aitable workflow list` / `get` / `edit-example` | 工作流目录、详情与当前 DSL 示例 |
| `aitable workflow create` / `update` | 校验完整 DSL 后创建或全量更新工作流 |
| `aitable workflow enable` / `disable` | 启停已唯一确认的工作流 |

上传、导入、导出和工作流可能异步完成；accepted/pending 不等于成功。保留 taskId/importId、轮询状态、
超时和真实 next command。非幂等创建在提交状态未知时只核对，不自动重试。

旧版 Runtime 缺少当前导出或分片建字段能力时，才分别使用
[aitable_export_via_task.py](../scripts/aitable_export_via_task.py) 或
[bulk_add_fields.py](../scripts/bulk_add_fields.py)；当前 Runtime 已有对应 Shortcut 时不得绕回脚本。

## 权限与 Base 内节点

| 原子命令 | 用途 |
|---|---|
| `aitable advperm role-list` / `role-get` | 高级权限角色读取 |
| `aitable advperm enable` / `disable` | Base 高级权限总开关 |
| `aitable advperm role-create` / `role-update` / `role-delete` | 自定义角色原子管理 |
| `aitable section list-nodes` / `list-empty` | Base 导航树和空 Section 读取 |
| `aitable section create` / `rename` / `reorder` | Section 创建、重命名和排序 |
| `aitable section move-node` | 在 Base 内移动 Table、Dashboard 等 nsheet 节点 |
| `aitable section delete` | 删除已确认的 Section |

高级权限角色 ID、Section ID 与 Drive 节点 ID 不可互换。整个 Base 在普通文件夹中的外层移动或重命名
归 Drive；Base 内 Table、Dashboard、Section 的结构操作仍归 AITable。

## 稳定 ID 传递

| 来源 | 只可用于 |
|---|---|
| `+url-resolve` / 唯一 Base 解析 | 当前 profile 下的 baseId，以及 URL 实际携带的 tableId/viewId/recordId |
| Base/Table/Field 读取 | 同一 Base 下后续命令的 tableId、fieldId |
| Record 查询或写入回执 | recordId、主键文档 nodeId、分享链接与历史查询 |
| View/Form/Dashboard/Chart 创建或读取 | 对应对象自己的稳定 ID，不以名称或列表序号替代 |
| 导入导出回执 | importId/taskId 及其 continuation；不能当 Base/Table ID |
| Workflow/AdvPerm/Section 读取 | workflowId、roleId、sectionId，仅限原资源与 profile |

## 故障处理

- `unknown command` / `unknown flag`：读取精确 leaf Help，最多修正一次；
- confirmation 或参数约束不清：读取精确 leaf Schema，以最终 Runtime gate 为准；
- 自然目标零命中、多候选或类型不明：停止并展示候选，不选择第一项；
- `partial_success`：保留已完成项、失败 ledger 和 checkpoint，只从真实 continuation 继续；
- commit unknown：按稳定 ID 或业务唯一键核对远端效果，未确认前不重放写入；
- 权限、认证或 profile：按 `dingtalk-shared` 对应 reference 分流；
- 本索引仍无法定位命令时，才用 `dws shortcut list --service aitable --format json` 做最终回退。
