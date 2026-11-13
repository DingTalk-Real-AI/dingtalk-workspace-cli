# record create — 新增记录

## 命令格式

```
Usage:
  dws aitable record create [flags]
Example:
  dws aitable record create --base-id <BASE_ID> --table-id <TABLE_ID> \
    --records '[{"cells":{"fldTextId":"文本内容","fldNumId":123}}]'
Flags:
      --base-id string        Base ID (必填)
      --records string        记录列表 JSON 数组，单次最多 100 条 (必填，与 --records-file 二选一)
      --records-file string   从文件读取 records JSON（替代 --records，适合超长数据或 Windows 环境）
      --table-id string       Table ID (必填)
```

## Windows / 超长 JSON 推荐

将 records JSON 写入文件，用 `--records-file ./records.json` 传入，避免命令行截断和引号转义问题。

## 常见错误（严格避免）

| 错误 | 说明 |
|------|------|
| 参数名用 `--data` | ❌ 参数名是 `--records`，不是 `--data` |
| cells key 使用不存在、重名或未精确匹配的字段名 | 支持 fieldId 或当前表内唯一字段名；推荐 fieldId，字段名须精确匹配 |
| 猜测 fieldId | 使用 fieldId 时，先 `field get` 获取真实 ID；也可使用已确认的当前表内唯一字段名 |
| 单次超 100 条 | ❌ 单次最多 100 条，超过需分批 |
| 附件/图片字段直传 URL | ❌ 严禁 `{"url":"https://..."}` — 会触发 TIMEOUT_ERROR。必须先 `attachment upload` 获取 `fileToken`，再用 `{"fileToken":"ft_xxx"}` 写入。详见 [aitable-attachment.md](./aitable-attachment.md) |

## 正确流程

```bash
# 先获取 fieldId
dws aitable field get --base-id <BASE_ID> --table-id <TABLE_ID> --format json
# 从返回中提取 fieldId（如 fldABC123）

# 再用 fieldId 写入记录
dws aitable record create --base-id <BASE_ID> --table-id <TABLE_ID> \
  --records '[{"cells":{"fldABC123":"Python入门"}}]' --format json

# 从创建响应的 data.newRecordIds[] 提取新 ID，并回读确认真实写入值
dws aitable record query --base-id <BASE_ID> --table-id <TABLE_ID> \
  --record-ids <NEW_RECORD_ID> --format json
```

创建成功以 `data.newRecordIds[]` 为 ID 来源；不要把整个 `data` 当作单个 recordId，也不要只以退出码作为写入成功证据。

## cells 写入格式

各字段类型的写入格式见 [aitable-cell-value.md](./aitable-cell-value.md)。
