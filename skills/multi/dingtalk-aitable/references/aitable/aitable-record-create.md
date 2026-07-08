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
| cells key 用字段名 | ❌ cells key 必须是 fieldId（如 `fldXXX`），不是字段名称（如 `"课程名称"`） |
| 不先获取 fieldId | ❌ 必须先 `table get` 获取 fieldId，再写入记录 |
| 单次超 100 条 | ❌ 单次最多 100 条，超过需分批 |

## 正确流程

```bash
# 先获取 fieldId
dws aitable table get --base-id <BASE_ID> --table-id <TABLE_ID> --format json
# 从返回中提取 fieldId（如 fldABC123）

# 再用 fieldId 写入记录
dws aitable record create --base-id <BASE_ID> --table-id <TABLE_ID> \
  --records '[{"cells":{"fldABC123":"Python入门"}}]' --format json
```

## cells 写入格式

各字段类型的写入格式见 [aitable-cell-value.md](./aitable-cell-value.md)。
