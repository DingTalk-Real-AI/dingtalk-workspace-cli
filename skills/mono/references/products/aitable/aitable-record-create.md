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
      --parent-record-id string  父记录 ID；传入后新记录作为它的子记录创建（子记录模式）
      --view-id string        子记录模式可选：从该视图读取层级配置；缺省自动找第一个配置了层级结构的表格视图
      --client-token string   子记录模式可选：幂等 token（UUID v4），重试时复用同一值防重复创建
```

## Windows / 超长 JSON 推荐

将 records JSON 写入文件，用 `--records-file ./records.json` 传入，避免命令行截断和引号转义问题。

## 子记录模式（层级记录）

在某条父记录下创建子记录：同一个 `record create` 命令，加 `--parent-record-id`：

```bash
dws aitable record create --base-id <BASE_ID> --table-id <TABLE_ID> \
  --parent-record-id <PARENT_RECORD_ID> \
  --records '[{"cells":{"fldABC123":"子记录内容"}}]' --format json
```

- cells 无需手写层级字段，服务端自动注入指向父记录的关联；写入格式与普通记录一致。
- 表尚未配置层级字段时，服务端会自动创建 association 字段对并更新视图配置（首次调用会改变表结构）。
- 子记录模式单次最多 100 条。
- `--view-id` / `--client-token` 仅子记录模式有效；不带 `--parent-record-id` 使用会被 CLI 拒绝。
- 返回 `data.recordIds[]`（子记录 ID）、`data.hierarchyFieldId`、`data.parentRecordId`；普通模式仍返回 `data.newRecordIds[]`。

## 常见错误（严格避免）

| 错误 | 说明 |
|------|------|
| 参数名用 `--data` | ❌ 参数名是 `--records`，不是 `--data` |
| cells key 用字段名 | ❌ cells key 必须是 fieldId（如 `fldXXX`），不是字段名称（如 `"课程名称"`） |
| 不先获取 fieldId | ❌ 必须先 `table get` 获取 fieldId，再写入记录 |
| 单次超 100 条 | ❌ 单次最多 100 条，超过需分批 |
| 子记录模式手写层级字段 | ❌ 传 `--parent-record-id` 即可，服务端自动注入关联，不要手填 |

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
