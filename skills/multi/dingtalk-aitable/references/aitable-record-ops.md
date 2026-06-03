# 记录操作详细指南

## 查询记录

```bash
dws aitable record query --base-id <BASE_ID> --table-id <TABLE_ID> --format json
```

返回:
```json
{
  "data": {
    "records": [
      {"recordId": "rec001", "cells": {"fldABC": "完成设计", "fldDEF": {"id":"opt1","name":"进行中"}}},
      {"recordId": "rec002", "cells": {"fldABC": "编写文档", "fldDEF": {"id":"opt2","name":"待开始"}}}
    ]
  }
}
```

按条件查询 (`--filters` 结构极易出错，请**强制**套用以下模板)
```bash
# 最外层必须是 "and" 或 "or"，单选字段传文本名称。
# 示例：基础条件查询模板（可改内部 operator 为 contain 等）
dws aitable record query --base-id <BASE_ID> --table-id <TABLE_ID> \
  --filters '{"operator":"and","operands":[{"operator":"eq","operands":["fld_state","进行中"]}]}' \
  --format json

# 关键词搜索
dws aitable record query --base-id <BASE_ID> --table-id <TABLE_ID> --keyword "设计" --format json

# 按 ID 查询
dws aitable record query --base-id <BASE_ID> --table-id <TABLE_ID> --record-ids rec001,rec002 --format json

# 游标分页
dws aitable record query --base-id <BASE_ID> --table-id <TABLE_ID> --limit 50 --cursor <CURSOR> --format json
```

## 添加记录

**必须先执行 `table get` 获取 fieldId，再写入。cells 的 key 必须是 fieldId（如 fldXXX），不是字段名。**

```bash
# 单条
dws aitable record create --base-id <BASE_ID> --table-id <TABLE_ID> \
  --records '[{"cells":{"fldABC":"完成设计","fldDEF":"进行中"}}]' \
  --format json

# 多条
dws aitable record create --base-id <BASE_ID> --table-id <TABLE_ID> \
  --records '[
    {"cells":{"fldABC":"任务A","fldDEF":"待开始"}},
    {"cells":{"fldABC":"任务B","fldDEF":"进行中"}}
  ]' --format json
```

返回:
```json
{"data": {"newRecordIds": ["rec-new-001", "rec-new-002"]}}
```

### --records 格式常见错误

```bash
#  正确: 参数名是 --records，cells key 是 fieldId
--records '[{"cells":{"fldABC":"值"}}]'

# 错误： 参数名写成 --data
--data '[{"cells":{"fldABC":"值"}}]'

# 错误：  cells key 用了字段名而非 fieldId
--records '[{"cells":{"任务名称":"值"}}]'

# 错误：  用 fields 而非 cells
--records '[{"fields":{"fldABC":"值"}}]'
```

## 更新记录

`--records` 中每条记录必须包含 `recordId`（从 `record query` 获取）:

```bash
dws aitable record update --base-id <BASE_ID> --table-id <TABLE_ID> \
  --records '[{"recordId":"rec001","cells":{"fldDEF":"已完成"}}]' \
  --format json
```

只需传入需修改的字段，未传入的保持原值。

## 删除记录

```bash
dws aitable record delete --base-id <BASE_ID> --table-id <TABLE_ID> \
  --record-ids rec001,rec002 --yes --format json
```

 不可逆操作。调用前建议先 `record query` 确认目标记录。

## 附件上传

>  **不要使用钉盘 (drive) 上传！** 钉盘 fileId 无法写入 attachment 字段。

使用 `upload_attachment.py` 脚本（内部自动完成 prepare + PUT to OSS），**2 步**完成：

```bash
# 步骤 1: 一键上传文件
python3 scripts/upload_attachment.py <BASE_ID> /path/to/report.pdf
# 输出: { "fileToken": "ft_xxx", "fileName": "report.pdf", "size": 204800 }

# 步骤 2: 在 record create/update 中使用 fileToken 写入附件字段
dws aitable record create --base-id <BASE_ID> --table-id <TABLE_ID> \
  --records '[{"cells":{"fldAttachId":[{"fileToken":"ft_xxx"}]}}]' --format json
```

>  attachment 字段值必须是数组 `[{"fileToken":"ft_xxx"}]`，支持多个附件。

## 字段类型写入规则

| 类型 | 写入格式 | 读取返回格式 |
|------|----------|-------------|
| text | `"fldXXX":"文本值"` | `"fldXXX":"文本值"` |
| number | `"fldXXX":123` | `"fldXXX":"123"` |
| singleSelect | `"fldXXX":"选项名"` | `"fldXXX":{"id":"xxx","name":"选项名"}` |
| multipleSelect | `"fldXXX":["选项1","选项2"]` | `"fldXXX":[{"id":"xxx","name":"选项1"}]` |
| date | `"fldXXX":"2026-03-04"` | ISO 日期字符串 |
| user | `"fldXXX":[{"userId":"123"}]` | `"fldXXX":[{"corpId":"x","userId":"123"}]` |
| attachment | `"fldXXX":[{"fileToken":"ft_xxx"}]`需先用脚本上传 | `"fldXXX":[{"url":"...","filename":"..."}]` |

### 只读字段（不要写入）

- 创建时间、修改时间、创建人、修改人
- 公式字段、引用字段
- 自动编号字段

执行 `table get` 后识别字段类型，跳过只读字段。
