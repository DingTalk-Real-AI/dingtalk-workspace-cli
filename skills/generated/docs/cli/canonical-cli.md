# Canonical CLI Surface

Generated from the shared Tool IR. Do not edit by hand.

## Command Pattern

- `dws <product> <tool> --json '{...}'`
- `dws schema <product>.<tool>`

## Products

### `ding`

- Display name: DING消息
- Description: DING消息
- Server key: `9d39ee2c7636f32c`
- Protocol: `2025-03-26`
- Degraded: `false`
- Tools:
  - `ding.recall_ding_message`: 撤回已发送的DING消息
    CLI route: `dws ding message recall`
    Flags: `--id`, `--robot-code`
    Schema: `skills/generated/docs/schema/ding/recall_ding_message.json`
  - `ding.search_my_robots`: 搜索我创建的机器人
    CLI route: `dws ding search_my_robots`
    Flags: `--currentPage`, `--pageSize`, `--robotName`
    Schema: `skills/generated/docs/schema/ding/search_my_robots.json`
  - `ding.send_ding_message`: 使用企业内机器人发送DING消息，可发送应用内DING、短信DING、电话DING。
    CLI route: `dws ding message send`
    Flags: `--content`, `--users`, `--type`, `--robot-code`
    Schema: `skills/generated/docs/schema/ding/send_ding_message.json`

### `bot`

- Display name: 机器人消息
- Description: 钉钉机器人消息MCP服务，支持创建企业机器人、将企业机器人添加到指定的群内、企业机器人发送群消息和单聊消息、企业机器人取消发送的群或单聊消息等能力。
- Server key: `3303015f1832b28d`
- Protocol: `2025-03-26`
- Degraded: `false`
- Tools:
  - `bot.add_robot_to_group`: 将自定义机器人添加到当前用户有管理权限的群聊中。如果没有权限则会报错
    CLI route: `dws bot group members add-bot`
    Flags: `--id`, `--robot-code`
    Schema: `skills/generated/docs/schema/bot/add_robot_to_group.json`
  - `bot.batch_recall_robot_users_msg`: 批量撤回机器人发送的单聊消息。
    CLI route: `dws bot message recall-by-bot`
    Flags: `--keys`, `--robot-code`
    Schema: `skills/generated/docs/schema/bot/batch_recall_robot_users_msg.json`
  - `bot.batch_send_robot_msg_to_users`: 机器人批量发送单聊消息，在该机器人可使用范围内的员工，可接收到单聊消息。
    CLI route: `dws bot batch_send_robot_msg_to_users`
    Flags: `--markdown`, `--robotCode`, `--title`, `--userIds`
    Schema: `skills/generated/docs/schema/bot/batch_send_robot_msg_to_users.json`
  - `bot.create_robot`: 创建企业机器人，调用本服务会在当前组织创建一个企业内部应用并自动开启stream功能的机器人，该应用被创建时自动完成发布，默认可见范围是当前用户。
    CLI route: `dws bot create_robot`
    Flags: `--desc`, `--robot-name`
    Schema: `skills/generated/docs/schema/bot/create_robot.json`
  - `bot.recall_robot_group_message`: 可批量撤回企业机器人在群内发送的消息。
    CLI route: `dws bot message recall_robot_group_message`
    Flags: `--group`, `--keys`, `--robot-code`
    Schema: `skills/generated/docs/schema/bot/recall_robot_group_message.json`
  - `bot.search_groups_by_keyword`: 根据关键词搜索我的群会话信息，包含群openconversationId、群名称等信息
    CLI route: `dws bot search_groups_by_keyword`
    Flags: `--cursor`, `--keyword`
    Schema: `skills/generated/docs/schema/bot/search_groups_by_keyword.json`
  - `bot.search_my_robots`: 搜索我创建的机器人，可获取机器人robotCode等信息。
    CLI route: `dws bot bot search`
    Flags: `--page`, `--size`, `--name`
    Schema: `skills/generated/docs/schema/bot/search_my_robots.json`
  - `bot.send_message_by_custom_robot`: 使用自定义机器人发送群消息，请注意自定义机器人与企业机器人的区别。
    CLI route: `dws bot message send-by-webhook`
    Flags: `--at-mobiles`, `--at-users`, `--at-all`, `--token`, `--text`, `--title`
    Schema: `skills/generated/docs/schema/bot/send_message_by_custom_robot.json`
  - `bot.send_robot_group_message`: 机器人发送群聊消息，该机器人必须已存在对应的群内。
    CLI route: `dws bot message send-by-bot`
    Flags: `--text`, `--group`, `--robot-code`, `--title`
    Schema: `skills/generated/docs/schema/bot/send_robot_group_message.json`

### `aitable`

- Display name: 钉钉 AI 表格
- Description: 钉钉 AI 表格 MCP 让 AI 直接操作表格数据与字段，快速打通查询、维护与自动化办公流程。
- Server key: `23eb09885b9e8f26`
- Protocol: `2025-03-26`
- Degraded: `false`
- Tools:
  - `aitable.create_base`: 创建一个新的 AI 表格 Base。当前仅要求 baseName，服务端按默认模板创建并返回 baseId/baseName
    CLI route: `dws aitable base create`
    Flags: `--name`, `--template-id`
    Schema: `skills/generated/docs/schema/aitable/create_base.json`
  - `aitable.create_fields`: 在已有表格中批量新增字段。适用于建表后补充一批字段，或一次性添加多个关联、流转等复杂类型字段。单次最多创建 15 个字段；若超过该数量，请拆分多次调用。允许部分成功，返回结果会逐项说明每个字段是否创建成功；失败项会返回 reason 说明失败原因。
    CLI route: `dws aitable field create`
    Flags: `--base-id`, `--fields`, `--table-id`
    Schema: `skills/generated/docs/schema/aitable/create_fields.json`
  - `aitable.create_records`: 在指定表格中批量新增记录
    CLI route: `dws aitable record create`
    Flags: `--base-id`, `--records`, `--table-id`
    Schema: `skills/generated/docs/schema/aitable/create_records.json`
  - `aitable.create_table`: 在指定 Base 中新建表格，并可在创建时附带初始化一批基础字段。
建表时单次最多附带 15 个字段；若 fields 为空，服务会自动补一个名为“标题”的 primaryDoc 首列。
若 tableName 与当前 Base 下已有表重名，服务会自动续号为“原名 1 / 原名 2 ...”，并在 summary 中返回当前表名。
如需添加更多字段，或在已有表中增加字段，请使用 create_fields。
    CLI route: `dws aitable table create`
    Flags: `--base-id`, `--fields`, `--name`
    Schema: `skills/generated/docs/schema/aitable/create_table.json`
  - `aitable.create_view`: 在指定数据表（Table）下创建一个新视图（View）。
当前稳定支持的 viewType：Grid、FormDesigner、Gantt、Calendar、Kanban、Gallery。
若未传 viewName，则会按视图类型自动生成不重名名称。
首列字段是每条数据的索引，不支持删除、移动或隐藏。
    CLI route: `dws aitable table create_view`
    Flags: `--baseId`, `--filter`, `--group`, `--sort`, `--visibleFieldIds`, `--tableId`, `--viewDescription`, `--viewName`, `--viewSubType`, `--viewType`
    Schema: `skills/generated/docs/schema/aitable/create_view.json`
  - `aitable.delete_base`: 删除指定 Base（高风险、不可逆）。成功后应无法通过 get_base/search_bases 读取到该 Base
    CLI route: `dws aitable base delete`
    Flags: `--base-id`, `--reason`
    Schema: `skills/generated/docs/schema/aitable/delete_base.json`
  - `aitable.delete_field`: 删除指定 Table 中的一个字段（Field），删除操作不可逆。禁止删除主字段，且禁止删除最后一个字段

此操作不可逆，会永久删除字段及其所有数据。
必须提供准确的 baseId、tableId 和 fieldId，不得使用名称代替 ID。
若字段不存在或无权限，将返回错误。
    CLI route: `dws aitable field delete`
    Flags: `--base-id`, `--field-id`, `--table-id`
    Schema: `skills/generated/docs/schema/aitable/delete_field.json`
  - `aitable.delete_records`: 在指定 Table 中批量删除记录（不可逆，数据将永久丢失）。
单次最多删除 100 条；超出请拆分多次调用。
调用前建议先通过 query_records 确认目标记录 ID 与内容，避免误删。
    CLI route: `dws aitable record delete`
    Flags: `--base-id`, `--record-ids`, `--table-id`
    Schema: `skills/generated/docs/schema/aitable/delete_records.json`
  - `aitable.delete_table`: 删除指定 tableId 的数据表（不可逆，数据将永久丢失），该操作为高风险写入。
调用前请先通过 get_base / get_tables 确认目标表 ID 与名称。
    CLI route: `dws aitable table delete`
    Flags: `--base-id`, `--reason`, `--table-id`
    Schema: `skills/generated/docs/schema/aitable/delete_table.json`
  - `aitable.delete_view`: 删除指定视图（View）。该操作不可逆。
已知保护：禁止删除数据表中的最后一个视图；锁定视图不允许删除。
    CLI route: `dws aitable table delete_view`
    Flags: `--baseId`, `--tableId`, `--viewId`
    Schema: `skills/generated/docs/schema/aitable/delete_view.json`
  - `aitable.export_data`: 导出 AI 表格数据的统一入口。
不传 taskId 时，会根据 scope / format 创建一个新的导出任务，并在 timeoutMs 时间内同步等待结果；若在等待窗口内完成，则直接返回 downloadUrl 和 fileName。
传入 taskId 时，不会重新创建任务，而是继续等待该任务；若仍未完成，则继续返回同一个 taskId，供下一次调用继续等待。
当前稳定支持的 scope：all、table、view；暂不开放按字段导出。
当前稳定支持的 format：excel、attachment、excel_and_attachment、excel_with_inline_images。
    CLI route: `dws aitable table export_data`
    Flags: `--baseId`, `--format`, `--scope`, `--tableId`, `--taskId`, `--timeoutMs`, `--viewId`
    Schema: `skills/generated/docs/schema/aitable/export_data.json`
  - `aitable.get_base`: 获取指定 Base 的资源目录级信息，返回 baseName、tables、dashboards 的 summary 信息（不含字段与记录详情）。
这是当前 Base 级目录入口：后续如需 tableId 或 dashboardId，优先从这里读取；table 详情再调用 get_tables，dashboard 详情再调用 get_dashboard
    CLI route: `dws aitable base get`
    Flags: `--base-id`
    Schema: `skills/generated/docs/schema/aitable/get_base.json`
  - `aitable.get_fields`: 批量获取指定字段的详细信息，包括 fieldId、名称、类型、description 以及类型相关完整配置（如格式化、选项、AI 配置等）。
传 fieldIds 时单次最多获取 10 个字段；若需更多字段，请拆分多次调用。
适用于在 get_tables 拿到字段目录后，按需展开少量字段的完整配置，避免大 options 字段放大 get_tables 返回值。
AI 字段的返回结果中，config 仅包含字段物理配置，aiConfig 作为同级字段单独返回，结构与 create_fields 写入参数一致。
    CLI route: `dws aitable field get`
    Flags: `--base-id`, `--field-ids`, `--table-id`
    Schema: `skills/generated/docs/schema/aitable/get_fields.json`
  - `aitable.get_tables`: 批量获取指定 Tables（数据表）的表级信息、字段目录与视图目录。
会返回 tables 列表；每个 table 直接包含 tableId、tableName、description、fields、views；字段列表仅包含 fieldId、fieldName、type、description；views 仅包含 viewId、viewName、type。
若需读取字段的完整配置，请再调用 get_fields。
    CLI route: `dws aitable table get`
    Flags: `--base-id`, `--table-ids`
    Schema: `skills/generated/docs/schema/aitable/get_tables.json`
  - `aitable.get_views`: 获取指定数据表（Table）中的视图（View）完整信息，包括列顺序、筛选、排序、分组、条件格式、自定义配置等。
支持两种模式：
- 显式选择：传入 viewIds，按入参顺序返回这些视图；单次最多 10 个。
- 默认全量：省略 viewIds，返回当前表下全部视图，顺序与当前表视图目录一致。
    CLI route: `dws aitable table get_views`
    Flags: `--baseId`, `--tableId`, `--viewIds`
    Schema: `skills/generated/docs/schema/aitable/get_views.json`
  - `aitable.import_data`: 将已通过 prepare_import_upload 上传完成的文件导入 AI 表格，每个 Sheet 会新建为独立的数据表（不支持追加到已有表格）。
工具内部会等待导入完成，大多数情况下一次调用即可拿到最终结果。若在 timeout 内未完成，再次传入相同 importId 继续等待，无需重新提交任务，也不要重新上传同一文件。
    CLI route: `dws aitable table import_data`
    Flags: `--importId`, `--timeout`
    Schema: `skills/generated/docs/schema/aitable/import_data.json`
  - `aitable.list_bases`: 列出当前用户可访问的 AI 表格 Base。默认返回最近访问结果，支持分页游标续取。返回 baseId 与 baseName，后续可直接用于 get_base。
AI 表格访问地址可按 baseId 拼接为：https://docs.dingtalk.com/i/nodes/{baseId}
    CLI route: `dws aitable base list`
    Flags: `--cursor`, `--limit`
    Schema: `skills/generated/docs/schema/aitable/list_bases.json`
  - `aitable.prepare_attachment_upload`: 为单个 attachment 字段文件申请带容量校验的 OSS 直传地址。
该工具仅适用于“需要先上传本地文件，再将其写入 attachment 字段”的场景，不是通用文件上传入口，也不适用于后续导入类任务上传。
如果已经有可直接下载的在线文件 URL，不要先下载文件再调用本工具；可直接在 create_records / update_records 的 attachment 字段中传入 [{"url":"https://..."}]，由服务端自动代拉外链并转存为内部附件。
该工具只负责准备上传，不直接接收文件二进制内容；实际文件字节流应由客户端在 MCP 外上传到返回的 uploadUrl。
上传文件时，向 uploadUrl 发起的 PUT 请求必须携带 Content-Type header，且其值必须是该文件的具体 MIME type。
上传成功后，请在 create_records / update_records 的 attachment 字段中写入 [{"fileToken":"..."}]。
    CLI route: `dws aitable attachment upload`
    Flags: `--base-id`, `--file-name`, `--mime-type`, `--size`
    Schema: `skills/generated/docs/schema/aitable/prepare_attachment_upload.json`
  - `aitable.prepare_import_upload`: 为导入任务申请 OSS 直传地址。返回 uploadUrl 和 importId。
客户端应通过 HTTP PUT 将原始文件字节流上传至 uploadUrl；除非 uploadUrl 对应的存储服务明确要求，否则不要额外附带 Content-Type 等自定义请求头。上传完成后将 importId 传入 import_data 即可触发导入，无需再传其他参数。
    CLI route: `dws aitable table prepare_import_upload`
    Flags: `--baseId`, `--fileName`, `--fileSize`
    Schema: `skills/generated/docs/schema/aitable/prepare_import_upload.json`
  - `aitable.query_records`: 查询指定表格中的记录，支持两种模式：
- 按 ID 取：传入 recordIds（单次最多 100 个），直接获取指定记录。
- 条件查：通过 filters 过滤、sort 排序、cursor 分页遍历全表。
两种模式均可通过 fieldIds（单次最多 100 个）限制返回字段以节省 token。
    CLI route: `dws aitable record query`
    Flags: `--base-id`, `--cursor`, `--field-ids`, `--filters`, `--keyword`, `--limit`, `--record-ids`, `--sort`, `--table-id`
    Schema: `skills/generated/docs/schema/aitable/query_records.json`
  - `aitable.search_bases`: 按名称关键词搜索 AI 表格 Base。返回 baseId/baseName，结果按相关性排序。返回的 baseId 可直接用于 get_base 等后续工具。
AI 表格访问地址可按 baseId 拼接为：https://docs.dingtalk.com/i/nodes/{baseId}
    CLI route: `dws aitable base search`
    Flags: `--cursor`, `--query`
    Schema: `skills/generated/docs/schema/aitable/search_bases.json`
  - `aitable.search_templates`: 按名称关键词搜索 AI 表格模板，支持分页。
返回每个模板的 templateId、name、description，以及分页信息 hasMore / nextCursor。
返回的 templateId 可直接用于 create_base。
模板预览链接可通过 https://docs.dingtalk.com/table/template/{templateId} 拼接得到
    CLI route: `dws aitable template search`
    Flags: `--cursor`, `--limit`, `--query`
    Schema: `skills/generated/docs/schema/aitable/search_templates.json`
  - `aitable.update_base`: 更新 Base 名称（可选备注）。当前不支持修改主题、封面等扩展属性
    CLI route: `dws aitable base update`
    Flags: `--base-id`, `--desc`, `--name`
    Schema: `skills/generated/docs/schema/aitable/update_base.json`
  - `aitable.update_field`: 更新指定字段的名称或配置。不可变更字段类型（type 不可修改）。
newFieldName、config、aiConfig 至少传入一项
    CLI route: `dws aitable field update`
    Flags: `--base-id`, `--config`, `--field-id`, `--name`, `--table-id`
    Schema: `skills/generated/docs/schema/aitable/update_field.json`
  - `aitable.update_records`: 批量更新指定记录的字段值，只需传入需修改的字段，未传入的字段保持原值
    CLI route: `dws aitable record update`
    Flags: `--base-id`, `--records`, `--table-id`
    Schema: `skills/generated/docs/schema/aitable/update_records.json`
  - `aitable.update_table`: 重命名指定 Table（数据表）。若新名称不符合命名要求、与同一 Base 下其他表重名或无权限，将返回错误。
    CLI route: `dws aitable table update`
    Flags: `--base-id`, `--name`, `--table-id`
    Schema: `skills/generated/docs/schema/aitable/update_table.json`
  - `aitable.update_view`: 更新指定视图（View）的名称、描述或配置。
当前稳定支持更新：newViewName、viewDescription、visibleFieldIds、filter、sort、group；fieldWidths 仅支持 Grid 视图。
首列字段是每条数据的索引，不支持删除、移动或隐藏。
    CLI route: `dws aitable table update_view`
    Flags: `--baseId`, `--fieldWidths`, `--filter`, `--group`, `--sort`, `--visibleFieldIds`, `--newViewName`, `--tableId`, `--viewDescription`, `--viewId`
    Schema: `skills/generated/docs/schema/aitable/update_view.json`

### `oa`

- Display name: 钉钉OA审批
- Description: 钉钉OA审批MCP服务，支持查询待处理审批、审批详情、同意/拒绝/撤销审批、操作记录、已发起实例列表、待审批任务及可见表单列表。
- Server key: `2721767f4caa8a6e`
- Protocol: `2025-03-26`
- Degraded: `false`
- Tools:
  - `oa.approve_processInstance`: 处理某个需要我处理的实例任务，拒绝审批实例任务，所需要的参数processInstanceId可以从list_pending_approvals工具获取。
    CLI route: `dws oa approval approve`
    Flags: `--instance-id`, `--remark`, `--task-id`
    Schema: `skills/generated/docs/schema/oa/approve_processInstance.json`
  - `oa.get_processInstance_detail`: 获取指定审批实例的详情信息
    CLI route: `dws oa approval detail`
    Flags: `--instance-id`
    Schema: `skills/generated/docs/schema/oa/get_processInstance_detail.json`
  - `oa.get_processInstance_records`: 获取某个审批实例的审批操作记录信息，获取的是该审批实例有哪些人做了什么操作，以及操作结果是什么
    CLI route: `dws oa approval records`
    Flags: `--instance-id`
    Schema: `skills/generated/docs/schema/oa/get_processInstance_records.json`
  - `oa.list_initiated_instances`: 查询当前用户已发起的审批实例列表，查询的信息包含审批实例Id、审批实例发起时间、审批实例当前状态等基础信息
    CLI route: `dws oa approval list-initiated`
    Flags: `--end`, `--max-results`, `--next-token`, `--process-code`, `--start`
    Schema: `skills/generated/docs/schema/oa/list_initiated_instances.json`
  - `oa.list_pending_approvals`: 查询当前用户待处理的审批单列表，返回每条审批单的名称、唯一编码（如审批实例 ID）、处理跳转链接（用于一键进入审批页面）等关键信息。结果仅包含用户作为审批人且尚未处理的审批事项，适用于工作台待办集成、审批提醒等场景。
    CLI route: `dws oa approval list-pending`
    Flags: `--end`, `--page`, `--size`, `--start`
    Schema: `skills/generated/docs/schema/oa/list_pending_approvals.json`
  - `oa.list_pending_tasks`: 查询待我审批的任务Id，获取任务Id之后，可以执行同意、拒绝审批单操作。
    CLI route: `dws oa approval tasks`
    Flags: `--instance-id`
    Schema: `skills/generated/docs/schema/oa/list_pending_tasks.json`
  - `oa.list_user_visible_process`: 获取当前用户可见的审批表单列表，可获取审批表单的processCode。
    CLI route: `dws oa approval list-forms`
    Flags: `--cursor`, `--size`
    Schema: `skills/generated/docs/schema/oa/list_user_visible_process.json`
  - `oa.reject_processInstance`: 处理某个需要我处理的实例任务，拒绝审批实例任务，所需要的参数processInstanceId可以从list_pending_approvals工具获取。
    CLI route: `dws oa approval reject`
    Flags: `--instance-id`, `--remark`, `--task-id`
    Schema: `skills/generated/docs/schema/oa/reject_processInstance.json`
  - `oa.revoke_processInstance`: 撤销当前用户已经发起的审批实例，需要的参数processInstanceId可以从
    CLI route: `dws oa approval revoke`
    Flags: `--instance-id`, `--remark`
    Schema: `skills/generated/docs/schema/oa/revoke_processInstance.json`

### `workbench`

- Display name: 钉钉工作台
- Description: 钉钉工作台MCP支持查询用户所有应用及批量获取应用详情，助力快速了解和管理办公应用。
- Server key: `62a9cf4de3d881c9`
- Protocol: `2025-03-26`
- Degraded: `false`
- Tools:
  - `workbench.batch_get_app_details`: 根据应用id批量拉取应用详情
    CLI route: `dws workbench app get`
    Flags: `--ids`
    Schema: `skills/generated/docs/schema/workbench/batch_get_app_details.json`
  - `workbench.get_user_workspace_apps`: 获取用户所有工作台应用
    CLI route: `dws workbench app list`
    Flags: `--input`
    Schema: `skills/generated/docs/schema/workbench/get_user_workspace_apps.json`

### `devdoc`

- Display name: 钉钉开放平台文档搜索
- Description: 钉钉开放平台文档搜索支持关键词查询，返回相关文档链接及摘要，快速定位开发指南。
- Server key: `c1110038088c6134`
- Protocol: `2025-03-26`
- Degraded: `false`
- Tools:
  - `devdoc.search_open_platform_docs`: 根据关键词搜索钉钉开放平台的开发文档，返回匹配的文档条目列表，包含标题、摘要、文档链接和相关标签。搜索结果按相关性排序。适用于开发者在集成或调试过程中快速查找 API 说明、接入指南、错误码解释等技术资料。
    CLI route: `dws devdoc article search`
    Flags: `--keyword`, `--page`, `--size`
    Schema: `skills/generated/docs/schema/devdoc/search_open_platform_docs.json`
  - `devdoc.search_open_platform_error_code`: 根据错误码搜索详细说明及对应的解决方法
    CLI route: `dws devdoc search_open_platform_error_code`
    Flags: `--errorCode`
    Schema: `skills/generated/docs/schema/devdoc/search_open_platform_error_code.json`

### `todo`

- Display name: 钉钉待办
- Description: 钉钉待办MCP服务提供高效的任务管理能力，支持创建待办事项、更新任务状态（如完成/未完成）、以及按条件查询待办列表。
- Server key: `b693ef3984e8d311`
- Protocol: `2025-03-26`
- Degraded: `false`
- Tools:
  - `todo.create_personal_todo`: 在当前企业组织内创建一条个人待办事项，支持设置标题、执行人列表（用户 ID）、截止时间、优先级（如高/中/低）。待办将归属于当前用户，并对有权限的协作者可见。
    CLI route: `dws todo task create`
    Flags: `--due`, `--executors`, `--priority`, `--title`
    Schema: `skills/generated/docs/schema/todo/create_personal_todo.json`
  - `todo.delete_todo`: 删除待办（所有执行者都删除）
    CLI route: `dws todo task delete`
    Flags: `--task-id`
    Schema: `skills/generated/docs/schema/todo/delete_todo.json`
  - `todo.get_user_todos_in_current_org`: 获取当前用户在所属组织中的个人待办事项列表，返回每项待办的标题、截止日期、优先级（如高/中/低）、完成状态。
    CLI route: `dws todo task list`
    Flags: `--page`, `--size`, `--status`
    Schema: `skills/generated/docs/schema/todo/get_user_todos_in_current_org.json`
  - `todo.query_todo_detail`: 查询待办详情
    CLI route: `dws todo task get`
    Flags: `--task-id`
    Schema: `skills/generated/docs/schema/todo/query_todo_detail.json`
  - `todo.update_todo_done_status`: 修改执行者的待办完成状态
    CLI route: `dws todo task done`
    Flags: `--status`, `--task-id`
    Schema: `skills/generated/docs/schema/todo/update_todo_done_status.json`
  - `todo.update_todo_task`: 修改整个待办任务
    CLI route: `dws todo task update`
    Flags: `--due`, `--done`, `--priority`, `--title`, `--task-id`
    Schema: `skills/generated/docs/schema/todo/update_todo_task.json`

### `calendar`

- Display name: 钉钉日历
- Description: 支持创建日程，查询日程，约空闲会议室等能力
- Server key: `279de7cc536c672c`
- Protocol: `2025-03-26`
- Degraded: `false`
- Tools:
  - `calendar.add_calendar_participant`: 向已存在的指定日程添加参与者，支持批量添加多人，可设置参与者类型和通知方式
    CLI route: `dws calendar participant add`
    Flags: `--users`, `--event`
    Schema: `skills/generated/docs/schema/calendar/add_calendar_participant.json`
  - `calendar.add_meeting_room`: 添加会议室
    CLI route: `dws calendar room add`
    Flags: `--event`, `--rooms`
    Schema: `skills/generated/docs/schema/calendar/add_meeting_room.json`
  - `calendar.create_calendar_event`: 创建新的日程，支持设置时间、参与者、提醒等完整功能
    CLI route: `dws calendar event create`
    Flags: `--desc`, `--end`, `--start`, `--title`
    Schema: `skills/generated/docs/schema/calendar/create_calendar_event.json`
  - `calendar.delete_calendar_event`: 删除指定日程，组织者删除将通知所有参与者，参与者删除仅从自己日历移除
    CLI route: `dws calendar event delete`
    Flags: `--id`
    Schema: `skills/generated/docs/schema/calendar/delete_calendar_event.json`
  - `calendar.delete_meeting_room`: 移除日程中预约的会议室
    CLI route: `dws calendar room delete`
    Flags: `--event`, `--rooms`
    Schema: `skills/generated/docs/schema/calendar/delete_meeting_room.json`
  - `calendar.get_calendar_detail`: 获取我的日历指定日程的详细信息
    CLI route: `dws calendar event get`
    Flags: `--id`
    Schema: `skills/generated/docs/schema/calendar/get_calendar_detail.json`
  - `calendar.get_calendar_participants`: 获取指定日程的所有参与者列表及其状态信息
    CLI route: `dws calendar participant list`
    Flags: `--event`
    Schema: `skills/generated/docs/schema/calendar/get_calendar_participants.json`
  - `calendar.list_calendar_events`: 仅允许查询当前用户指定时间范围内的日程列表，最多返回100条
    CLI route: `dws calendar event list`
    Flags: `--end`, `--start`
    Schema: `skills/generated/docs/schema/calendar/list_calendar_events.json`
  - `calendar.list_meeting_room_groups`: 分页查询当前企业下的会议室分组列表，返回每个分组的名称（groupName）、唯一 ID（groupId）及其父分组 ID（parentId，0 表示根分组）。结果按组织架构权限过滤，仅包含调用者有权限查看的分组。
    CLI route: `dws calendar room list-groups`
    Flags: `--pageIndex`, `--pageSize`
    Schema: `skills/generated/docs/schema/calendar/list_meeting_room_groups.json`
  - `calendar.query_available_meeting_room`: 根据时间筛选出符合闲忙条件的会议室列表。
    CLI route: `dws calendar room search`
    Flags: `--end`, `--group-id`, `--available`, `--start`
    Schema: `skills/generated/docs/schema/calendar/query_available_meeting_room.json`
  - `calendar.query_busy_status`: 查询指定用户在给定时间范围内的闲忙状态，返回其日历中已占用时间段的详细日程信息（如标题、开始/结束时间），不包含具体日程内容细节（如参与人、地点），以保护隐私。结果受组织可见性策略控制：仅当调用者有权限查看该用户日历时方可获取有效数据。适用于安排会议前快速确认他人可用时间。
    CLI route: `dws calendar busy search`
    Flags: `--end`, `--start`, `--users`
    Schema: `skills/generated/docs/schema/calendar/query_busy_status.json`
  - `calendar.remove_calendar_participant`: 从已存在的指定日程中移除参与者，支持批量移除多人
    CLI route: `dws calendar participant delete`
    Flags: `--users`, `--event`
    Schema: `skills/generated/docs/schema/calendar/remove_calendar_participant.json`
  - `calendar.update_calendar_event`: 修改现有日程的信息，支持更新标题、时间、地点等任意字段，需要组织者权限。（修改参与人需要使用给日程添加参与人或给日程删除参与人工具）
    CLI route: `dws calendar event update`
    Flags: `--end`, `--id`, `--start`, `--title`
    Schema: `skills/generated/docs/schema/calendar/update_calendar_event.json`

### `report`

- Display name: 钉钉日志
- Description: 钉钉日志MCP，包含获取日志模板、读取日志内容、写日志等功能
- Server key: `379b7411e5ab4e32`
- Protocol: `2025-03-26`
- Degraded: `false`
- Tools:
  - `report.create_report`: 创建日志
    CLI route: `dws report create`
    Flags: `--contents`, `--dd-from`, `--template-id`, `--to-chat`, `--to-user-ids`
    Schema: `skills/generated/docs/schema/report/create_report.json`
  - `report.get_available_report_templates`: 获取当前员工可使用的日志模版信息，包含日志模板的名称、模板Id等
    CLI route: `dws report template list`
    Flags: none
    Schema: `skills/generated/docs/schema/report/get_available_report_templates.json`
  - `report.get_received_report_list`: 查询当前人收到的日志列表
    CLI route: `dws report list`
    Flags: `--cursor`, `--end`, `--size`, `--start`
    Schema: `skills/generated/docs/schema/report/get_received_report_list.json`
  - `report.get_report_entry_details`: 获取指定一篇日志的详情信息
    CLI route: `dws report detail`
    Flags: `--report-id`
    Schema: `skills/generated/docs/schema/report/get_report_entry_details.json`
  - `report.get_report_statistics_by_id`: 获取日志统计数据，包括评论数量、点赞数量、已读数等
    CLI route: `dws report stats`
    Flags: `--report-id`
    Schema: `skills/generated/docs/schema/report/get_report_statistics_by_id.json`
  - `report.get_send_report_list`: 查询当前人创建的日志详情列表，包含日志的内容、日志名称、创建时间等信息
    CLI route: `dws report sent`
    Flags: `--cursor`, `--end`, `--modified-end`, `--modified-start`, `--template-name`, `--size`, `--start`
    Schema: `skills/generated/docs/schema/report/get_send_report_list.json`
  - `report.get_template_details_by_name`: 获取当前员工可使用的日志模版详情信息，包括日志模板Id、日志模板内字段的名称、字段类型、字段排序等
    CLI route: `dws report template detail`
    Flags: `--name`
    Schema: `skills/generated/docs/schema/report/get_template_details_by_name.json`

### `group-chat`

- Display name: 钉钉群聊
- Description: 钉钉群聊MCP支持创建内部群、搜索群会话、管理群成员、修改群名称及查询话题回复等群聊管理能力。
- Server key: `27f939aef74c67b5`
- Protocol: `2025-03-26`
- Degraded: `false`
- Tools:
  - `group-chat.add_group_member`: 添加群成员
    CLI route: `dws chat group members add`
    Flags: `--id`, `--users`
    Schema: `skills/generated/docs/schema/group-chat/add_group_member.json`
  - `group-chat.create_internal_group`: 创建一个组织内部的群聊（仅限本企业成员加入），支持指定群名称、初始成员列表等参数。操作成功后返回是否创建成功的结果。群聊创建受组织安全策略限制（如成员必须属于同一企业）。适用于项目协作、临时沟通等需要快速建群的场景。
    CLI route: `dws chat group create`
    Flags: `--users`, `--name`
    Schema: `skills/generated/docs/schema/group-chat/create_internal_group.json`
  - `group-chat.create_internal_org_group`: 创建一个组织内部的群聊（仅限本企业成员加入），支持指定群名称、初始成员列表等参数。操作成功后返回是否创建成功的结果。群聊创建受组织安全策略限制（如成员必须属于同一企业）。适用于项目协作、临时沟通等需要快速建群的场景。
    CLI route: `dws chat create_internal_org_group`
    Flags: `--groupMembers`, `--groupName`
    Schema: `skills/generated/docs/schema/group-chat/create_internal_org_group.json`
  - `group-chat.get_group_members`: 查群成员列表
    CLI route: `dws chat group members list`
    Flags: `--cursor`, `--id`
    Schema: `skills/generated/docs/schema/group-chat/get_group_members.json`
  - `group-chat.list_conversation_message`: 已废弃！！！！拉取指定单聊或群聊的会话消息内容
    CLI route: `dws chat list_conversation_message`
    Flags: `--endTime`, `--openconversation-id`, `--startTime`
    Schema: `skills/generated/docs/schema/group-chat/list_conversation_message.json`
  - `group-chat.list_conversation_message_v2`: 拉取指定群聊的会话消息内容
    CLI route: `dws chat list_conversation_message_v2`
    Flags: `--forward`, `--limit`, `--openconversation-id`, `--time`
    Schema: `skills/generated/docs/schema/group-chat/list_conversation_message_v2.json`
  - `group-chat.list_individual_chat_message`: 拉取指定用户的单聊会话消息内容
    CLI route: `dws chat list_individual_chat_message`
    Flags: `--forward`, `--limit`, `--time`, `--userId`
    Schema: `skills/generated/docs/schema/group-chat/list_individual_chat_message.json`
  - `group-chat.list_topic_replies`: 针对话题群中的单个话题，分页拉取话题的回复消息
    CLI route: `dws chat message list-topic-replies`
    Flags: `--forward`, `--group`, `--limit`, `--time`, `--topic-id`
    Schema: `skills/generated/docs/schema/group-chat/list_topic_replies.json`
  - `group-chat.remove_group_member`: 移除群成员
    CLI route: `dws chat group members remove`
    Flags: `--id`, `--users`
    Schema: `skills/generated/docs/schema/group-chat/remove_group_member.json`
  - `group-chat.search_groups_by_keyword`: 根据群名称关键词，搜索符合条件的群，返回群的openconversion_id、群名称等信息
    CLI route: `dws chat search`
    Flags: `--cursor`, `--query`
    Schema: `skills/generated/docs/schema/group-chat/search_groups_by_keyword.json`
  - `group-chat.send_direct_message_as_user`: 以当前用户的身份给某用户发送单聊消息。
    CLI route: `dws chat send_direct_message_as_user`
    Flags: `--clawType`, `--receiverUserId`, `--text`, `--title`
    Schema: `skills/generated/docs/schema/group-chat/send_direct_message_as_user.json`
  - `group-chat.send_message_as_user`: 以当前用户的身份发送群消息
    CLI route: `dws chat send_message_as_user`
    Flags: `--atAll`, `--atUserIds`, `--clawType`, `--openConversation-id`, `--text`, `--title`
    Schema: `skills/generated/docs/schema/group-chat/send_message_as_user.json`
  - `group-chat.update_group_name`: 更新群名称
    CLI route: `dws chat group rename`
    Flags: `--name`, `--id`
    Schema: `skills/generated/docs/schema/group-chat/update_group_name.json`

### `attendance`

- Display name: 钉钉考勤打卡
- Description: 考勤打卡MCP，支持查询考勤统计数据、排班信息等
- Server key: `33b1011e8b8382a6`
- Protocol: `2025-03-26`
- Degraded: `false`
- Tools:
  - `attendance.batch_get_employee_shifts`: 批量查询多个员工在指定日期的考勤班次信息，返回每条记录包含：用户 ID（userId）、工作日期（workDate，毫秒时间戳）、打卡类型（checkType，如 OnDuty 表示上班）、计划打卡时间（planCheckTime，毫秒时间戳）以及是否为休息日（isRest，"Y"/"N"）。结果基于组织考勤配置生成，仅返回调用者有权限查看的员工数据，适用于排班核对、考勤预览等场景。
    CLI route: `dws attendance shift list`
    Flags: `--start`, `--end`, `--users`
    Schema: `skills/generated/docs/schema/attendance/batch_get_employee_shifts.json`
  - `attendance.get_attendance_summary`: 获取考勤统计摘要
    CLI route: `dws attendance summary`
    Flags: `--date`, `--user`
    Schema: `skills/generated/docs/schema/attendance/get_attendance_summary.json`
  - `attendance.get_user_attendance_record`: 查询指定用户在某一天的考勤详情，包括实际打卡记录（如上班/下班时间、是否正常打卡）、当日所排班次、所属考勤组信息、是否为休息日、出勤工时（如 "0Hours"）、加班时长等。返回数据受组织权限和隐私策略限制，仅当调用者有权限查看该用户考勤信息时才返回有效内容。适用于员工自助查询、HR 核对出勤或审批关联场景。
    CLI route: `dws attendance record get`
    Flags: `--user`, `--date`
    Schema: `skills/generated/docs/schema/attendance/get_user_attendance_record.json`
  - `attendance.query_attendance_group_or_rules`: 查询考勤组/考勤规则："我属于哪个考勤组""我们的打卡范围是什么""弹性工时是怎么算的"
    CLI route: `dws attendance rules`
    Flags: `--date`
    Schema: `skills/generated/docs/schema/attendance/query_attendance_group_or_rules.json`

### `contact`

- Display name: 钉钉通讯录
- Description: 钉钉通讯录MCP支持搜索人员/部门、查询成员详情及部门结构，快速获取组织架构信息。
- Server key: `e1e13a2fc7ab1f1b`
- Protocol: `2025-03-26`
- Degraded: `false`
- Tools:
  - `contact.get_current_user_profile`: 获取当前登录用户的基本信息（如姓名、工号、手机号）、当前组织信息（corpId、组织名称）、直属主管信息、所属部门列表（含部门 ID 与名称）以及角色信息（如管理员类型、自定义角色标签等）。返回内容受组织隐私与权限策略控制：若某些字段（如主管、手机号）被设为不可见，则可能被过滤或省略。
    CLI route: `dws contact user get-self`
    Flags: none
    Schema: `skills/generated/docs/schema/contact/get_current_user_profile.json`
  - `contact.get_dept_members_by_deptId`: 获取指定部门下的所有成员，返回每位成员的用户 ID（userId）和显示名称（如真实姓名或昵称）。结果受组织可见性控制：若调用者无权查看某成员（例如该成员所在子部门被隐藏，或其个人信息设为私密），则该成员不会出现在返回列表中。适用于需要展示部门人员列表、选择协作成员等场景，仅支持调用者有权限访问的部门。
    CLI route: `dws contact dept list-members`
    Flags: `--ids`
    Schema: `skills/generated/docs/schema/contact/get_dept_members_by_deptId.json`
  - `contact.get_sub_depts_by_dept_id`: 根据指定的部门 ID，获取其直接子部门列表，返回每个子部门的部门 ID、名称。结果受组织架构可见性控制：仅返回调用者有权限查看的子部门；若父部门不可见或无子部门，则返回空列表。
    CLI route: `dws contact get_sub_depts_by_dept_id`
    Flags: `--deptId`
    Schema: `skills/generated/docs/schema/contact/get_sub_depts_by_dept_id.json`
  - `contact.get_user_info_by_user_ids`: 获取指定用户 ID 列表对应的员工详细信息，包括人员基本信息（ID、名称、主管名称、主管userId等）、所属角色信息、所在部门信息。返回结果受组织可见性规则限制：若调用者无权查看某员工（如部门隐藏、手机号设为私密等），则相应字段可能被过滤或不返回该员工。适用于需要批量获取同事信息的场景，如组织架构展示、审批人选择等。仅返回调用者权限范围内的有效数据。
    CLI route: `dws contact user get`
    Flags: `--ids`
    Schema: `skills/generated/docs/schema/contact/get_user_info_by_user_ids.json`
  - `contact.search_contact_by_key_word`: 根据关键词搜索好友和同事
    CLI route: `dws contact search_contact_by_key_word`
    Flags: `--keyword`
    Schema: `skills/generated/docs/schema/contact/search_contact_by_key_word.json`
  - `contact.search_dept_by_keyword`: 根据关键词模糊搜索部门，返回匹配的部门列表，包含每个部门的 ID、名称。搜索范围限于调用者有权限查看的组织架构；若关键词无匹配结果或部门因可见性设置被隐藏，则相应部门不会出现在结果中。
    CLI route: `dws contact dept search`
    Flags: `--keyword`
    Schema: `skills/generated/docs/schema/contact/search_dept_by_keyword.json`
  - `contact.search_user_by_key_word`: 搜索组织内成员，并返回成员的userId。如果需要查询详情，需要调用另外一个工具
    CLI route: `dws contact user search`
    Flags: `--keyword`
    Schema: `skills/generated/docs/schema/contact/search_user_by_key_word.json`
  - `contact.search_user_by_mobile`: 通过手机号搜索获取用户名称和userId。
    CLI route: `dws contact user search-mobile`
    Flags: `--mobile`
    Schema: `skills/generated/docs/schema/contact/search_user_by_mobile.json`
