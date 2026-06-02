# AI 表格最佳实践

## 1. 字段可写性分类

| 字段类型 | 可写 | 正确方式 |
|----------|------|----------|
| 文本/数字/日期/单选/多选/复选框/URL | 是 | `dws aitable record create` / `dws aitable record update` |
| 附件 | 是，但需先上传 | 先 `dws aitable attachment upload` 取 `uploadUrl/fileToken`，PUT 后把 `fileToken` 写入记录 |
| 创建人/修改人/创建时间/修改时间 | 否 | 系统字段，只读 |
| 公式/查找引用 | 否 | 由系统计算，只读 |
| AI 字段 | 否 | 由 AI 自动计算，只读 |

## 2. 查询执行契约

1. 优先用 `dws aitable record query --filters` 在服务端过滤，不要先拉全量再在上下文里手动筛选。
2. 返回 `has_more=true` 时不能做全局结论，数据可能不完整。
3. 查询前先用 `dws aitable table get --base-id <BASE_ID> --table-ids <TABLE_ID>` 获取真实 fieldId，不要猜字段 ID。
4. 只需要部分字段时，用 `dws aitable record query --field-ids fld1,fld2` 降低响应体积。
5. 已知 recordId 时，用 `dws aitable record get --record-ids rec1,rec2`，不要构造无意义 filters。

## 3. 任务选路

| 用户诉求 | 优先方案 | 不要误走 |
|---------|----------|----------|
| 查看几条数据 | `dws aitable record query --base-id <BASE_ID> --table-id <TABLE_ID>` | 不要默认 `--all` |
| 全量拉取/统计 | `dws aitable record query --base-id <BASE_ID> --table-id <TABLE_ID> --all` | 不要手动循环 cursor |
| 全量导出 | `dws aitable export data --base-id <BASE_ID> --scope all --format excel` | 不要 `--all` 拉全量再写文件 |
| 文件级导入 | `dws aitable import upload --base-id <BASE_ID> --file-name data.xlsx --file-size <字节数>` + `dws aitable import data --import-id <ID>` | 不要手动解析 xlsx 再逐条写入 |
| 批量写入多条不同数据 | `dws aitable record create --base-id <BASE_ID> --table-id <TABLE_ID> --records '[{"cells":{"<FIELD_ID>":"值"}}]'` | 不要一次超过 100 条 |
| 批量给多条记录写同一组值 | `dws aitable record update --base-id <BASE_ID> --table-id <TABLE_ID> --records '[{"recordId":"rec1","cells":{"<FIELD_ID>":"值"}},{"recordId":"rec2","cells":{"<FIELD_ID>":"值"}}]'` | 不要使用隐藏兼容命令 |
| 附件上传 | `dws aitable attachment upload --base-id <BASE_ID> --file-name report.pdf --size <字节数>` + PUT + `record create/update` | 不要用钉盘 drive 上传 |
| 调整字段顺序 | `dws aitable view update --base-id <BASE_ID> --table-id <TABLE_ID> --view-id <VIEW_ID> --config '{"visibleFieldIds":["fld1","fld2"]}'` | 没有 `field reorder` 命令 |
| 查看视图列表 | `dws aitable view list --base-id <BASE_ID> --table-id <TABLE_ID>` | 不需要用 `view get --view-ids` |
| 创建收集表/问卷 | `dws aitable view create --base-id <BASE_ID> --table-id <TABLE_ID> --view-type FormDesigner --name "表单名"` | 不要使用隐藏兼容命令 |
| 仪表盘/图表 | 先 `dashboard config-example` / `chart widgets-example`，再 create/update | 不要猜 config 结构 |

## 4. 创建/修改后回读确认

执行写操作后，建议立即回读确认结果：

| 写操作 | 建议回读命令 | 确认内容 |
|--------|-------------|----------|
| `dws aitable base create` | `dws aitable base get --base-id <BASE_ID>` | base 名称、tables 列表 |
| `dws aitable table create` | `dws aitable table get --base-id <BASE_ID> --table-ids <TABLE_ID>` | 表名、字段列表是否符合预期 |
| `dws aitable field create` | `dws aitable field get --base-id <BASE_ID> --table-id <TABLE_ID>` | 新字段是否出现在字段列表中 |
| `dws aitable record create/update` | `dws aitable record get --base-id <BASE_ID> --table-id <TABLE_ID> --record-ids <RECORD_ID>` | 写入值是否正确 |
| `dws aitable view update` | `dws aitable view get --base-id <BASE_ID> --table-id <TABLE_ID> --view-ids <VIEW_ID>` | `visibleFieldIds` 顺序是否正确 |
| `dws aitable view create/update` | `dws aitable view get --base-id <BASE_ID> --table-id <TABLE_ID> --view-ids <VIEW_ID>` | 表单视图名称、描述和配置 |

## 5. 导入导出与异步任务

- `export data` 的 `--format` 是导出格式，不要在此命令上追加全局 `--format json`。
- 创建导出任务：
  ```bash
  dws aitable export data --base-id <BASE_ID> --scope table --table-id <TABLE_ID> \
    --format excel --timeout-ms 1000
  ```
- 续等已有导出任务：
  ```bash
  dws aitable export data --base-id <BASE_ID> --task-id <TASK_ID> --timeout-ms 3000
  ```
- 导入本地文件：
  ```bash
  dws aitable import upload --base-id <BASE_ID> --file-name data.xlsx --file-size <字节数> --format json
  curl -X PUT "<uploadUrl>" -H "Content-Type:" --data-binary @data.xlsx
  dws aitable import data --import-id <IMPORT_ID> --format json
  ```

## 6. AI 字段注意事项

- AI 字段的 prompt 必须至少包含一个 `fieldRef` 引用，纯文本 prompt 会被后端拒绝。
- 先创建/确认被引用字段的 fieldId，再在 prompt 中引用。
- `outputType` 必须与字段类型一致，例如 `outputType=text` 配 `--type text`。
