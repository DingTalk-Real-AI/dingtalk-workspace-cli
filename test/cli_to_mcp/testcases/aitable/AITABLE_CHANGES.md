# aitable.go 修改总结

## 修改日期
2026-04-26

## 修改原因
根据 MCP Server 最新的 tools 定义,比对并更新 aitable.go 的实现,确保出入参和描述与 MCP 一致。

---

## 一、新增功能

### 1. get_base_primary_doc_id

**新增命令**: `dws aitable base get-primary-doc-id`

**功能**: 根据 baseId、tableId 和 recordId 获取主键文档对应的文档信息(返回 dentryUuid)

**参数**:
- `--base-id` (必填): Base ID
- `--table-id` (必填): Table ID  
- `--record-id` (必填): 记录 ID

**调用 MCP Tool**: `get_base_primary_doc_id`

**使用场景**: 当 AI 表格使用文档类型作为主键字段时,获取对应文档的 ID,进而读取文档内容或进行其他操作

---

## 二、参数变更

### 1. create_base - 新增 folderId 参数 ⚠️ 补充修复

**新增参数**:
- `--folder-id`: 目标父节点的 dentryUuid (知识库节点 ID)
- 可选参数，不传时在默认位置创建
- 支持传入标准节点 URL，MCP 会在创建前解析出实际生效的节点 ID

**描述更新**:
- 新增 folderId 说明：如果需要创建在特定的文件夹路径下，则需要传递 folderId
- 新增示例：`dws aitable base create --name "项目跟踪" --folder-id FOLDER_ID`

**MCP 定义**:
```
folderId (string, 可选): 对外协议字段名固定为folderId，表示目标父节点的 dentryUuid。
MCP 层会进一步兼容同字段传入的标准节点 URL，并在创建前解析出实际生效的节点 ID。
```

### 2. create_table - fields 参数

**变更**:
- fields 从"必填"改为"必填但可传空数组 []"
- 默认值从 `""` 改为 `"[]"`
- 新增说明: 若传空数组,系统会自动补一个名为"标题"的 primaryDoc 首列

**描述更新**:
- 新增: 若 tableName 与当前 Base 下已有表重名,服务会自动续号
- 更新: 使用 create_fields 而非 field create

### 3. create_view - 新增 viewDescription 参数

**新增参数**:
- `--desc`: 视图描述 JSON,如 `{"content":[]}`

**描述更新**:
- "当前支持的 viewType" → "当前稳定支持的 viewType"

### 4. update_view - 新增 viewDescription 参数

**新增参数**:
- `--desc`: 新的视图描述 JSON,不修改时省略;如需清空可传 `{"content":[]}`

**描述更新**:
- "当前支持更新" → "当前稳定支持更新"
- 新增 --desc 说明

---

## 三、描述更新

### 1. 新增字段类型支持 ⚠️ 补充修复

**新增 3 个字段类型**:

1. **address** (行政区域)
   - 无需 config
   - 用于存储中国行政区域信息

2. **filterUp** (查找引用)
   - config 要求: targetSheet*, filters*, valuesField*, aggregator*
   - 只读字段，不能通过 record create/update 写入值
   - 创建新表时 filters 只能使用 value（字段对常量）
   - 在已有表中添加字段时可使用 currentSheetFieldId（字段对字段）
   - filters 的 link 必须统一（全部 AND 或全部 OR）
   - aggregator: SUM|AVERAGE|COUNT|MAX|MIN|CONCATENATE

3. **lookup** (关联引用)
   - config 要求: associateField*, valuesField*, aggregator*
   - 只读字段，不能通过 record create/update 写入值
   - associateField 为双向/单向链接字段的 fieldId
   - aggregator: SUM|AVERAGE|COUNT|MAX|MIN|CONCATENATE

**AI 字段配置增强**:
- outputType 新增: image, video
- 新增 imageConfig: 配置图片分辨率和 AI 水印
- 新增 videoConfig: 配置视频宽高比、分辨率、时长
- 新增 autoRecompute: 弓|用字段变化后是否自动重算
- 新增 enableThinking: 是否启用深度思考
- 新增 enableWebSearch: 是否启用联网搜索
- 新增 computeOnEmptyRef: 引用字段为空时是否继续触发 AI 计算

**其他文案优化**:
- formula 类型新增 config 示例: `{"formula":"[单价] * [数量]"}`
- currency formatter 补充说明：若需指定小数位可用 INT|FLOAT_1|FLOAT_2|FLOAT_3|FLOAT_4
- options 更新说明：更新时已有选项建议回传原 id

### 2. delete_table
- "通过 base get / table get 确认" → "通过 get_base / get_tables 确认"
- 与 MCP tool 名称保持一致

### 3. create_fields
- MCP Server 的 title 是英文 "create_fields",但描述正确
- 代码中保持中文描述 "创建字段"

### 4. query_records
- MCP Server 的 title 是英文 "query_records",但描述正确
- 代码中保持中文描述 "获取行记录"
- 参数同时支持 `--query` 和 `--keyword`,都映射到 MCP 的 `keyword` 参数

---

## 四、忽略的问题

### create_chart 的 title 错误
- MCP Server 中 create_chart 的 title 写成了"更新图表"
- 但 description 是正确的("在指定 dashboard 下创建 chart...")
- 这是 MCP Server 的配置错误,暂不处理

---

## 五、完整性验证

### MCP Server tools 总数: 42 个
### 代码实现 tools 总数: 42 个 (新增 1 个)

**所有 tools 已完全覆盖**:
- ✅ list_bases
- ✅ search_bases
- ✅ get_base
- ✅ create_base
- ✅ update_base
- ✅ delete_base
- ✅ **get_base_primary_doc_id** (新增)
- ✅ get_tables
- ✅ create_table
- ✅ update_table
- ✅ delete_table
- ✅ get_fields
- ✅ create_fields
- ✅ update_field
- ✅ delete_field
- ✅ query_records
- ✅ create_records
- ✅ update_records
- ✅ delete_records
- ✅ search_templates
- ✅ prepare_attachment_upload
- ✅ get_views
- ✅ create_view
- ✅ update_view
- ✅ delete_view
- ✅ get_dashboard_config_example
- ✅ get_dashboard
- ✅ create_dashboard
- ✅ update_dashboard
- ✅ delete_dashboard
- ✅ get_dashboard_share
- ✅ update_dashboard_share
- ✅ get_dashboard_widgets_example
- ✅ get_chart
- ✅ create_chart
- ✅ update_chart
- ✅ delete_chart
- ✅ get_chart_share
- ✅ update_chart_share
- ✅ export_data
- ✅ prepare_import_upload
- ✅ import_data

---

## 六、测试建议

1. 测试 `dws aitable base get-primary-doc-id` 新命令
2. 测试 `dws aitable table create` 使用空 fields 数组 `[]`
3. 测试 `dws aitable view create` 和 `update` 的 `--desc` 参数
4. 回归测试所有修改描述的工具

---

## 七、后续优化建议

1. 反馈给 MCP Server 团队:
   - 修复 create_chart 的 title 错误(应该是"创建图表")
   - 为 create_fields 添加中文 title
   - 为 query_records 添加中文 title

2. 统一 share 相关工具的中文描述风格(当前已有细微差异,但不影响功能)
