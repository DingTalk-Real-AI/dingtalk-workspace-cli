# Canonical Product: aitable

Generated from shared Tool IR. Do not edit by hand.

- Display name: 钉钉 AI 表格
- Description: 钉钉 AI 表格 MCP 让 AI 直接操作表格数据与字段，快速打通查询、维护与自动化办公流程。
- Server key: `23eb09885b9e8f26`
- Endpoint: `https://mcp-gw.dingtalk.com/server/5f0d121611f14e878f7d42c3e32bf6c4a790d433066adae38c062a657c397047`
- Protocol: `2025-03-26`
- Degraded: `false`

## Tools

- `base create`
  - Path: `aitable.create_base`
  - CLI route: `dws aitable base create`
  - Description: 创建一个新的 AI 表格 Base。当前仅要求 baseName，服务端按默认模板创建并返回 baseId/baseName
  - Flags: `--name`, `--template-id`
  - Schema: `skills/generated/docs/schema/aitable/create_base.json`
- `field create`
  - Path: `aitable.create_fields`
  - CLI route: `dws aitable field create`
  - Description: 在已有表格中批量新增字段。适用于建表后补充一批字段，或一次性添加多个关联、流转等复杂类型字段。单次最多创建 15 个字段；若超过该数量，请拆分多次调用。允许部分成功，返回结果会逐项说明每个字段是否创建成功；失败项会返回 reason 说明失败原因。
  - Flags: `--base-id`, `--fields`, `--table-id`
  - Schema: `skills/generated/docs/schema/aitable/create_fields.json`
- `record create`
  - Path: `aitable.create_records`
  - CLI route: `dws aitable record create`
  - Description: 在指定表格中批量新增记录
  - Flags: `--base-id`, `--records`, `--table-id`
  - Schema: `skills/generated/docs/schema/aitable/create_records.json`
- `table create`
  - Path: `aitable.create_table`
  - CLI route: `dws aitable table create`
  - Description: 在指定 Base 中新建表格，并可在创建时附带初始化一批基础字段。
建表时单次最多附带 15 个字段；若 fields 为空，服务会自动补一个名为“标题”的 primaryDoc 首列。
若 tableName 与当前 Base 下已有表重名，服务会自动续号为“原名 1 / 原名 2 ...”，并在 summary 中返回当前表名。
如需添加更多字段，或在已有表中增加字段，请使用 create_fields。
  - Flags: `--base-id`, `--fields`, `--name`
  - Schema: `skills/generated/docs/schema/aitable/create_table.json`
- `table create_view`
  - Path: `aitable.create_view`
  - CLI route: `dws aitable table create_view`
  - Description: 在指定数据表（Table）下创建一个新视图（View）。
当前稳定支持的 viewType：Grid、FormDesigner、Gantt、Calendar、Kanban、Gallery。
若未传 viewName，则会按视图类型自动生成不重名名称。
首列字段是每条数据的索引，不支持删除、移动或隐藏。
  - Flags: `--baseId`, `--filter`, `--group`, `--sort`, `--visibleFieldIds`, `--tableId`, `--viewDescription`, `--viewName`, `--viewSubType`, `--viewType`
  - Schema: `skills/generated/docs/schema/aitable/create_view.json`
- `base delete`
  - Path: `aitable.delete_base`
  - CLI route: `dws aitable base delete`
  - Description: 删除指定 Base（高风险、不可逆）。成功后应无法通过 get_base/search_bases 读取到该 Base
  - Flags: `--base-id`, `--reason`
  - Schema: `skills/generated/docs/schema/aitable/delete_base.json`
- `field delete`
  - Path: `aitable.delete_field`
  - CLI route: `dws aitable field delete`
  - Description: 删除指定 Table 中的一个字段（Field），删除操作不可逆。禁止删除主字段，且禁止删除最后一个字段

此操作不可逆，会永久删除字段及其所有数据。
必须提供准确的 baseId、tableId 和 fieldId，不得使用名称代替 ID。
若字段不存在或无权限，将返回错误。
  - Flags: `--base-id`, `--field-id`, `--table-id`
  - Schema: `skills/generated/docs/schema/aitable/delete_field.json`
- `record delete`
  - Path: `aitable.delete_records`
  - CLI route: `dws aitable record delete`
  - Description: 在指定 Table 中批量删除记录（不可逆，数据将永久丢失）。
单次最多删除 100 条；超出请拆分多次调用。
调用前建议先通过 query_records 确认目标记录 ID 与内容，避免误删。
  - Flags: `--base-id`, `--record-ids`, `--table-id`
  - Schema: `skills/generated/docs/schema/aitable/delete_records.json`
- `table delete`
  - Path: `aitable.delete_table`
  - CLI route: `dws aitable table delete`
  - Description: 删除指定 tableId 的数据表（不可逆，数据将永久丢失），该操作为高风险写入。
调用前请先通过 get_base / get_tables 确认目标表 ID 与名称。
  - Flags: `--base-id`, `--reason`, `--table-id`
  - Schema: `skills/generated/docs/schema/aitable/delete_table.json`
- `table delete_view`
  - Path: `aitable.delete_view`
  - CLI route: `dws aitable table delete_view`
  - Description: 删除指定视图（View）。该操作不可逆。
已知保护：禁止删除数据表中的最后一个视图；锁定视图不允许删除。
  - Flags: `--baseId`, `--tableId`, `--viewId`
  - Schema: `skills/generated/docs/schema/aitable/delete_view.json`
- `table export_data`
  - Path: `aitable.export_data`
  - CLI route: `dws aitable table export_data`
  - Description: 导出 AI 表格数据的统一入口。
不传 taskId 时，会根据 scope / format 创建一个新的导出任务，并在 timeoutMs 时间内同步等待结果；若在等待窗口内完成，则直接返回 downloadUrl 和 fileName。
传入 taskId 时，不会重新创建任务，而是继续等待该任务；若仍未完成，则继续返回同一个 taskId，供下一次调用继续等待。
当前稳定支持的 scope：all、table、view；暂不开放按字段导出。
当前稳定支持的 format：excel、attachment、excel_and_attachment、excel_with_inline_images。
  - Flags: `--baseId`, `--format`, `--scope`, `--tableId`, `--taskId`, `--timeoutMs`, `--viewId`
  - Schema: `skills/generated/docs/schema/aitable/export_data.json`
- `base get`
  - Path: `aitable.get_base`
  - CLI route: `dws aitable base get`
  - Description: 获取指定 Base 的资源目录级信息，返回 baseName、tables、dashboards 的 summary 信息（不含字段与记录详情）。
这是当前 Base 级目录入口：后续如需 tableId 或 dashboardId，优先从这里读取；table 详情再调用 get_tables，dashboard 详情再调用 get_dashboard
  - Flags: `--base-id`
  - Schema: `skills/generated/docs/schema/aitable/get_base.json`
- `field get`
  - Path: `aitable.get_fields`
  - CLI route: `dws aitable field get`
  - Description: 批量获取指定字段的详细信息，包括 fieldId、名称、类型、description 以及类型相关完整配置（如格式化、选项、AI 配置等）。
传 fieldIds 时单次最多获取 10 个字段；若需更多字段，请拆分多次调用。
适用于在 get_tables 拿到字段目录后，按需展开少量字段的完整配置，避免大 options 字段放大 get_tables 返回值。
AI 字段的返回结果中，config 仅包含字段物理配置，aiConfig 作为同级字段单独返回，结构与 create_fields 写入参数一致。
  - Flags: `--base-id`, `--field-ids`, `--table-id`
  - Schema: `skills/generated/docs/schema/aitable/get_fields.json`
- `table get`
  - Path: `aitable.get_tables`
  - CLI route: `dws aitable table get`
  - Description: 批量获取指定 Tables（数据表）的表级信息、字段目录与视图目录。
会返回 tables 列表；每个 table 直接包含 tableId、tableName、description、fields、views；字段列表仅包含 fieldId、fieldName、type、description；views 仅包含 viewId、viewName、type。
若需读取字段的完整配置，请再调用 get_fields。
  - Flags: `--base-id`, `--table-ids`
  - Schema: `skills/generated/docs/schema/aitable/get_tables.json`
- `table get_views`
  - Path: `aitable.get_views`
  - CLI route: `dws aitable table get_views`
  - Description: 获取指定数据表（Table）中的视图（View）完整信息，包括列顺序、筛选、排序、分组、条件格式、自定义配置等。
支持两种模式：
- 显式选择：传入 viewIds，按入参顺序返回这些视图；单次最多 10 个。
- 默认全量：省略 viewIds，返回当前表下全部视图，顺序与当前表视图目录一致。
  - Flags: `--baseId`, `--tableId`, `--viewIds`
  - Schema: `skills/generated/docs/schema/aitable/get_views.json`
- `table import_data`
  - Path: `aitable.import_data`
  - CLI route: `dws aitable table import_data`
  - Description: 将已通过 prepare_import_upload 上传完成的文件导入 AI 表格，每个 Sheet 会新建为独立的数据表（不支持追加到已有表格）。
工具内部会等待导入完成，大多数情况下一次调用即可拿到最终结果。若在 timeout 内未完成，再次传入相同 importId 继续等待，无需重新提交任务，也不要重新上传同一文件。
  - Flags: `--importId`, `--timeout`
  - Schema: `skills/generated/docs/schema/aitable/import_data.json`
- `base list`
  - Path: `aitable.list_bases`
  - CLI route: `dws aitable base list`
  - Description: 列出当前用户可访问的 AI 表格 Base。默认返回最近访问结果，支持分页游标续取。返回 baseId 与 baseName，后续可直接用于 get_base。
AI 表格访问地址可按 baseId 拼接为：https://docs.dingtalk.com/i/nodes/{baseId}
  - Flags: `--cursor`, `--limit`
  - Schema: `skills/generated/docs/schema/aitable/list_bases.json`
- `attachment upload`
  - Path: `aitable.prepare_attachment_upload`
  - CLI route: `dws aitable attachment upload`
  - Description: 为单个 attachment 字段文件申请带容量校验的 OSS 直传地址。
该工具仅适用于“需要先上传本地文件，再将其写入 attachment 字段”的场景，不是通用文件上传入口，也不适用于后续导入类任务上传。
如果已经有可直接下载的在线文件 URL，不要先下载文件再调用本工具；可直接在 create_records / update_records 的 attachment 字段中传入 [{"url":"https://..."}]，由服务端自动代拉外链并转存为内部附件。
该工具只负责准备上传，不直接接收文件二进制内容；实际文件字节流应由客户端在 MCP 外上传到返回的 uploadUrl。
上传文件时，向 uploadUrl 发起的 PUT 请求必须携带 Content-Type header，且其值必须是该文件的具体 MIME type。
上传成功后，请在 create_records / update_records 的 attachment 字段中写入 [{"fileToken":"..."}]。
  - Flags: `--base-id`, `--file-name`, `--mime-type`, `--size`
  - Schema: `skills/generated/docs/schema/aitable/prepare_attachment_upload.json`
- `table prepare_import_upload`
  - Path: `aitable.prepare_import_upload`
  - CLI route: `dws aitable table prepare_import_upload`
  - Description: 为导入任务申请 OSS 直传地址。返回 uploadUrl 和 importId。
客户端应通过 HTTP PUT 将原始文件字节流上传至 uploadUrl；除非 uploadUrl 对应的存储服务明确要求，否则不要额外附带 Content-Type 等自定义请求头。上传完成后将 importId 传入 import_data 即可触发导入，无需再传其他参数。
  - Flags: `--baseId`, `--fileName`, `--fileSize`
  - Schema: `skills/generated/docs/schema/aitable/prepare_import_upload.json`
- `record query`
  - Path: `aitable.query_records`
  - CLI route: `dws aitable record query`
  - Description: 查询指定表格中的记录，支持两种模式：
- 按 ID 取：传入 recordIds（单次最多 100 个），直接获取指定记录。
- 条件查：通过 filters 过滤、sort 排序、cursor 分页遍历全表。
两种模式均可通过 fieldIds（单次最多 100 个）限制返回字段以节省 token。
  - Flags: `--base-id`, `--cursor`, `--field-ids`, `--filters`, `--keyword`, `--limit`, `--record-ids`, `--sort`, `--table-id`
  - Schema: `skills/generated/docs/schema/aitable/query_records.json`
- `base search`
  - Path: `aitable.search_bases`
  - CLI route: `dws aitable base search`
  - Description: 按名称关键词搜索 AI 表格 Base。返回 baseId/baseName，结果按相关性排序。返回的 baseId 可直接用于 get_base 等后续工具。
AI 表格访问地址可按 baseId 拼接为：https://docs.dingtalk.com/i/nodes/{baseId}
  - Flags: `--cursor`, `--query`
  - Schema: `skills/generated/docs/schema/aitable/search_bases.json`
- `template search`
  - Path: `aitable.search_templates`
  - CLI route: `dws aitable template search`
  - Description: 按名称关键词搜索 AI 表格模板，支持分页。
返回每个模板的 templateId、name、description，以及分页信息 hasMore / nextCursor。
返回的 templateId 可直接用于 create_base。
模板预览链接可通过 https://docs.dingtalk.com/table/template/{templateId} 拼接得到
  - Flags: `--cursor`, `--limit`, `--query`
  - Schema: `skills/generated/docs/schema/aitable/search_templates.json`
- `base update`
  - Path: `aitable.update_base`
  - CLI route: `dws aitable base update`
  - Description: 更新 Base 名称（可选备注）。当前不支持修改主题、封面等扩展属性
  - Flags: `--base-id`, `--desc`, `--name`
  - Schema: `skills/generated/docs/schema/aitable/update_base.json`
- `field update`
  - Path: `aitable.update_field`
  - CLI route: `dws aitable field update`
  - Description: 更新指定字段的名称或配置。不可变更字段类型（type 不可修改）。
newFieldName、config、aiConfig 至少传入一项
  - Flags: `--base-id`, `--config`, `--field-id`, `--name`, `--table-id`
  - Schema: `skills/generated/docs/schema/aitable/update_field.json`
- `record update`
  - Path: `aitable.update_records`
  - CLI route: `dws aitable record update`
  - Description: 批量更新指定记录的字段值，只需传入需修改的字段，未传入的字段保持原值
  - Flags: `--base-id`, `--records`, `--table-id`
  - Schema: `skills/generated/docs/schema/aitable/update_records.json`
- `table update`
  - Path: `aitable.update_table`
  - CLI route: `dws aitable table update`
  - Description: 重命名指定 Table（数据表）。若新名称不符合命名要求、与同一 Base 下其他表重名或无权限，将返回错误。
  - Flags: `--base-id`, `--name`, `--table-id`
  - Schema: `skills/generated/docs/schema/aitable/update_table.json`
- `table update_view`
  - Path: `aitable.update_view`
  - CLI route: `dws aitable table update_view`
  - Description: 更新指定视图（View）的名称、描述或配置。
当前稳定支持更新：newViewName、viewDescription、visibleFieldIds、filter、sort、group；fieldWidths 仅支持 Grid 视图。
首列字段是每条数据的索引，不支持删除、移动或隐藏。
  - Flags: `--baseId`, `--fieldWidths`, `--filter`, `--group`, `--sort`, `--visibleFieldIds`, `--newViewName`, `--tableId`, `--viewDescription`, `--viewId`
  - Schema: `skills/generated/docs/schema/aitable/update_view.json`
