# 上下文传递表

## 上下文传递表

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `base list/search` | `baseId` | 所有后续命令的 --base-id，拼接文档 URI |
| `base create` | `baseId` | 后续命令 + 文档 URI |
| `base get` | `tables[].tableId` | --table-id，拼接指定数据表 URI |
| `table create` | `tableId` | 后续命令 + 拼接指定数据表 URI |
| `table get` | `fields[].fieldId` | record 操作的 cells key, field get/update/delete |
| `record query` | `recordId` | record update/delete；按 ID 反查字段值用 `record get` |
| `template search` | `templateId` | base create --template-id，拼接模板预览 URI |
