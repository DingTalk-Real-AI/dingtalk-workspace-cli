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

记录复合写入沿用服务端的 fieldId/唯一字段名寻址，CLI 不在写前读取字段目录或改写 cells。回读先直接比较；仅缺少对应字段键或需要类型投影时，才读取 `get_fields`，将校验期望中的字段名精确映射为 fieldId，同一轮校验复用目录。字段目录读取失败、不完整或映射存在歧义时，保留写入回执和已知 recordId，返回非零退出码、unknown 与 verification.status=pending，不代表写入失败，也不自动重发写入；`+record-upsert`、`+record-batch-create` 与按键创建复用有界只读回读重试（500ms、1500ms）；预算耗尽仍报告未确认，不重发写入。目录是写后观测，不能证明与写入时的 schema 原子一致。`+record-upsert-by-key` 的 `--key-field-id` 仍只接受字段 ID，原有唯一键预检不变。dry-run 不新增字段请求；`+record-bulk-patch` 仍查询匹配记录，按键 upsert 仍查询唯一键以生成计划。

## 添加记录

**优先通过 `table get` 获取真实 fieldId 作为 cells 的 key；服务端也支持当前表内唯一字段名，名称需精确匹配。**

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
# 推荐写法：参数名是 --records，此例使用真实 fieldId；唯一字段名示例见下方
--records '[{"cells":{"fldABC":"值"}}]'

# 错误： 参数名写成 --data
--data '[{"cells":{"fldABC":"值"}}]'

# 唯一字段名也受服务端支持；重名或字段变更时优先使用真实 fieldId
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
| date | 日期字符串（如 `"2026-03-15"`、`"2026-03-15 09:00"`）、含时区的 RFC3339 字符串或整数毫秒时间戳（允许 `0` 和负值） | 固定 `+08:00` 的 RFC3339 字符串 |
| user | `"fldXXX":[{"userId":"123"}]` | `"fldXXX":[{"corpId":"x","userId":"123"}]` |
| attachment | `"fldXXX":[{"fileToken":"ft_xxx"}]`需先用脚本上传 | `"fldXXX":[{"url":"...","filename":"..."}]` |

日期的含时区字符串和毫秒时间戳按固定 UTC+8 转为分钟精度写入，秒和毫秒不保留，转换后的年份须在 0001–9999。合法日期读出统一为东八区 RFC3339 字符串，不保留原始输入类型。写入后查询读回示例：`1788883200000` → `"2026-09-09T00:00:00+08:00"`；`0` → `"1970-01-01T08:00:00+08:00"`；`-60000` → `"1970-01-01T07:59:00+08:00"`。

### 只读字段（不要写入）

- 创建时间、修改时间、创建人、修改人
- 公式字段、引用字段
- 自动编号字段

执行 `table get` 后识别字段类型，跳过只读字段。
