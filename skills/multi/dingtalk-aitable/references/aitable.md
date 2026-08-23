# AI 表格执行 Reference（兼容索引，非默认入口）

根 Skill 已包含高频 Golden Route 的准确参数，参数足够时直接执行，不读取本文件。本文件仅作兼容索引：只有根 Skill 与精确操作 Reference 都无法覆盖时，才可把它作为该 Case 唯一读取的 Reference；读取后不再加载第二个 Reference、`--help` 或产品级 Catalog。

## 高频 Golden Route（准确 flags）

```bash
# 新建 Base 和整套表字段；tables 必须是非空数组
dws aitable +base-bootstrap --name "项目管理" --tables '[{"name":"任务","fields":[{"fieldName":"标题","type":"text"}]}]'

# 只新建空 Base；folder-id / template-id 均为可选
dws aitable base create --name "项目管理"
dws aitable base create --name "项目管理" --folder-id <FOLDER_NODE_ID>
dws aitable base create --name "项目管理" --template-id <TEMPLATE_ID>

# 已有 Base 新建一张完整表；超过 15 个字段由 shortcut 自动分片
dws aitable +table-bootstrap --base-id <BASE_ID> --name "任务" --fields '[{"fieldName":"标题","type":"text"}]'

# 只补字段；单字段与批量字段二选一，单次最多 15 个
dws aitable field create --base-id <BASE_ID> --table-id <TABLE_ID> --name "状态" --type singleSelect --config '{"options":[{"name":"待办"},{"name":"完成"}]}'
dws aitable field create --base-id <BASE_ID> --table-id <TABLE_ID> --fields '[{"fieldName":"状态","type":"singleSelect","config":{"options":[{"name":"待办"}]}}]'

# 创建视图；view-type 合法值：Grid/FormDesigner/Gantt/Calendar/Kanban/Gallery
dws aitable view create --base-id <BASE_ID> --table-id <TABLE_ID> --view-type Grid --name "默认表格"
# Gantt 必须再绑定日期字段，否则只是空壳
dws aitable view update timebar --base-id <BASE_ID> --table-id <TABLE_ID> --view-id <VIEW_ID> --start-field <DATE_FIELD_ID>

# 创建仪表盘和图表；chart 的 config/layout 都必填
dws aitable dashboard create --base-id <BASE_ID> --name "运营看板"
dws aitable chart create --base-id <BASE_ID> --dashboard-id <DASHBOARD_ID> --config '<WIDGET_CONFIG_JSON>' --layout '{"x":0,"y":0,"w":12,"h":4}'

# 查询记录；选择器按意图传一个，不猜 flag
dws aitable +record-query --base-id <BASE_ID> --table-id <TABLE_ID>
dws aitable +record-query --base-id <BASE_ID> --table-id <TABLE_ID> --record-ids <RECORD_ID_1,RECORD_ID_2>
dws aitable +record-query --base-id <BASE_ID> --table-id <TABLE_ID> --record-ids <RECORD_ID_1,RECORD_ID_2> --field-ids <FIELD_ID_1,FIELD_ID_2>
dws aitable +record-query --base-id <BASE_ID> --table-id <TABLE_ID> --filters '<FILTER_JSON>'
dws aitable +record-query --base-id <BASE_ID> --table-id <TABLE_ID> --query "关键词"

# 文件导入：先申请凭证并上传，再用真实 importId 导入
dws aitable +import-upload --base-id <BASE_ID> --file-name data.xlsx --file-size <BYTES>
curl -X PUT "<UPLOAD_URL>" -H "Content-Type:" --data-binary @data.xlsx
dws aitable +import-data --import-id <IMPORT_ID>
dws aitable +import-data --import-id <IMPORT_ID> --table-id <TABLE_ID>
```

写命令是否追加 `--yes` 只以 Runtime confirmation 为准，不把 `--yes` 固化进示例。字段对象统一使用 `fieldName` / `type` / 可选 `config`；不要改写成相似的 `field-name`、`field-type` 或 `table-name`。建表时第一个字段会成为主字段，应优先使用 `text`；传空字段数组时由服务自动补标题主字段。导入 OSS PUT 的 `Content-Type` 必须显式置空，否则可能返回 403。

## 错误恢复

- 结构化错误存在 `actions` / `available_flags` 时，`actions` 中的完整命令就是唯一下一步；不要试相似 ID 或 flag。
- `partial_success`：保留 `knownSideEffects` 与 `checkpoint`，只执行返回的 `nextCommand` 检查或恢复，不重放已成功批次。
- `unknown`：先按 `nextCommand` 或业务唯一名称确认远端效果；未确认前不得重试非幂等创建。
- `retryable=false`：停止。目标文件夹无效时只用返回的 `dws drive +info --node <ID> --format json` 核对，不把其他类型 ID 轮流代入。
- 验证成功必须看到 `verification.status=verified`；退出码为 0 或写接口空回包本身不构成成功。

## 加载规则

- 先根据用户意图定位下表中的唯一命令族，再读取至多一个对应 leaf reference。
- 已知命令但参数不确定，只读一次该 leaf Schema；不要加载产品级 Schema 或完整 Catalog。
- 命令清单只在本索引仍无法定位时用 `dws shortcut list --service aitable --format json` 回退。
- 不使用 Python recipe 绕过已有 shortcut；不根据命令名猜 flag。

## 目标与 ID

| 已有信息 | 最短入口 | 输出复用 |
|---|---|---|
| AI 表格 URL | `dws aitable +url-resolve --url <URL>` | 解析 URL 中已有的 baseId/tableId/viewId/recordId；不做远端名称搜索 |
| 已知 Base/Table ID | 直接传给最终命令 | 不重复解析 |
| Base 精确名称，且要唯一定位后操作 | `dws aitable +resolve-base --name <名称>` | 默认精确匹配；明确需要模糊匹配时加 `--fuzzy`，零/多候选均停止 |
| 搜索 Base 候选、关键词查找或存在性检查 | `dws aitable +base-search --query <关键词>` | 直接检查返回候选，不先执行 `+resolve-base`；对象明确为 Base 时不得改走人员搜索 |
| Base ID + Table 名称 | `dws aitable +resolve-table --base <B> --name <T>` | 默认精确匹配；明确需要模糊匹配时加 `--fuzzy` |
| Base ID，只需目录 | `dws aitable +list-tables --base <B>` | 只投影 tableId/tableName，不额外读取字段 |
| 需要字段或视图目录 | `dws aitable +field-get ...` / `dws aitable +view-get ...` | 只读取当前任务需要的目标范围 |

`+record-query` 只接受真实 `base-id` / `table-id`，因此 URL 或名称须先按上表解析。`+base-list` 只表示最近访问，不是组织内全量 Base；`+base-search --query <Q>` 是单次搜索。唯一操作目标零命中或多候选时停止，不选第一项；“搜索候选/如果没有就创建”不要再串行调用 resolver 和 search。

## 低频命令族

| 意图 | 推荐 shortcut | 精确 reference |
|---|---|---|
| Base 新建整套结构 | `+base-bootstrap` | 使用本文上方准确 flags |
| Base 查看、搜索、复制、改名、删除、快照 | `+base-get` / `+base-search` / `+base-copy` / `+base-update` / `+base-delete` / `+base-schema-snapshot` | 对应 leaf Schema |
| Table 新建、查看、复制、改名、删除 | `+table-bootstrap` / `+table-get` / `+table-copy` / `+table-update` / `+table-delete` | 新建使用本文上方准确 flags；其余对应 leaf Schema |
| Field 完整配置、修改、删除 | `+field-get` / `+field-update` / `+field-delete` | [field](aitable/aitable-field.md)；属性细节才读 [field-properties](aitable/aitable-field-properties.md) |
| Record 历史、空行、分享、主键文档 | `+record-history-list` / `+record-query-empty` / `+record-share-*` / `+record-primary-doc-*` | [record-ops](aitable-record-ops.md) |
| Record 统计、分组聚合和去重率 | `record stats` / `record group-stats` | [record-stats](aitable/aitable-record-stats.md) |
| Filter、sort、日期操作符 | `+record-query` / `+record-bulk-patch` | [filter-sort](aitable/aitable-filter-sort.md) |
| View 配置、复制、锁定、冻结列、行高、填色 | 对应 `+view-*` | [view-config](aitable/aitable-view-config.md)；冻结列/行高/填色才读 [view-extras](aitable/aitable-view-extras.md) |
| Form 字段、分享、修改、删除 | 对应 `+form-*` | [form](aitable/aitable-form.md) |
| Dashboard 与 Chart | 对应 `+dashboard-*` / `+chart-*` | [dashboard-chart](aitable/aitable-dashboard-chart.md) |
| 导入、导出和任务恢复 | `+import-upload` / `+import-data` / `+export-data` | [export-import](aitable/aitable-export-import.md) |
| 上传或移除附件 | `+attachment-put` / `+attachment-remove` | [attachment](aitable/aitable-attachment.md) |
| 自动化工作流 | 对应 `+workflow-*` | [workflow](aitable/aitable-workflow.md) |
| 普通角色和高级权限 | 对应 `+role-*` / `+advperm-*` | [advperm](aitable/aitable-advperm.md) |
| AI 表格内部 Section/节点 | 对应 `+section-*` | [section](aitable-section.md) |
| 模板检索 | `+template-search` | leaf Schema 足够，不再读其他 reference |

## 写操作通用约束

- 创建/更新记录前只读取目标 Table 或目标 Field 的真实配置；`cells` key 优先使用 fieldId。
- 只读字段（公式、查找引用、创建/修改信息等）不写入。
- 删除 Base/Table/Field/Record、停用工作流、删除附件和关闭高级权限均按 Runtime confirmation 执行。
- 批量操作检查 completed/failed/checkpoint/nextCommand；`partial_success` 不等于成功。
- 写入效果未知时按返回的稳定 ID 或业务唯一键回读，不能整批盲目重放。

兼容包仍包含 `scripts/aitable_export_via_task.py` 与 `scripts/bulk_add_fields.py`；当前主路径分别是 `+export-data` 与 `+table-bootstrap` / `field create`，只有旧版运行时缺少对应命令时才使用脚本。

## 跨产品边界

- AI 表格记录、字段、视图和自动化留在 AITable。
- Base 结构复制/删除与 Base 内 Table、Dashboard、Section 操作走 AITable；只有整个 Base 的普通文件夹位置移动或外层存储重命名走 Drive。
- 记录主键文档正文拿到真实 nodeId 后走 Doc。
- Excel 式单元格、区域和公式操作走 Sheet，而不是 AITable。
