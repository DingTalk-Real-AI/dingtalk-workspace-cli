# AI 表格最佳实践

## 1. 字段可写性分类

| 字段类型 | 可写 | 正确方式 |
|----------|------|----------|
| 文本/数字/日期/单选/多选/复选框/URL | ✅ | record create/update |
| 附件 | ⚠️ | 必须先走 [attachment upload 流程](./aitable-attachment.md) |
| 创建人/修改人/创建时间/修改时间 | ❌ | 系统字段，只读 |
| 公式/查找引用 | ❌ | 只读，由系统计算 |
| AI 字段 | ❌ | 只读，由 AI 自动计算 |

## 2. 查询执行契约

1. **不要拉全量后在 context 里手动统计** — 优先用 `--filters` 在服务端过滤
2. **has_more=true 时不能做全局结论** — 数据可能不完整
3. **优先用 `--filters` 在服务端过滤** — 不要拉全量后在本地 jq/grep
4. **字段名必须来自 `table get` 真实返回** — 不要猜测 fieldId
5. **减少响应体积** — 用 `--field-ids` 仅返回需要的字段

## 3. 任务选路

| 用户诉求 | 优先方案 | 不要误走 |
|---------|----------|----------|
| 查看几条数据 | `dws aitable record query --base-id <BASE_ID> --table-id <TABLE_ID>` | 不要默认 `--all` |
| 全量拉取/统计 | `dws aitable record query --base-id <BASE_ID> --table-id <TABLE_ID> --all` | 不要手动循环 cursor |
| 全量导出 | `dws aitable export data --base-id <BASE_ID> --scope all --export-format excel` | 不要 `--all` 拉全量再写文件 |
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
| `table create` | `table get --table-ids <新tableId>` | 表名、字段列表是否符合预期 |
| `field create` | `table get --table-ids <tableId>` | 新字段是否出现在字段列表中 |
| `record create/update` | `record query --record-ids <新recordId>` | 写入值是否正确 |

## 5. AI 字段注意事项

- `export data` 的导出格式用 `--export-format`（如 `--export-format excel`）；`--format` 在这里是全局输出格式，两者不要混用。
- 创建导出任务：
  ```bash
  dws aitable export data --base-id <BASE_ID> --scope table --table-id <TABLE_ID> \
    --export-format excel --timeout-ms 1000
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
