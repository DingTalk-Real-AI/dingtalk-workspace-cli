# record delete — 删除记录

## 命令格式

```
Usage:
  dws aitable record delete [flags]
Example:
  dws aitable record delete --base-id <BASE_ID> --table-id <TABLE_ID> --record-ids rec1,rec2 --yes
Flags:
      --base-id string      Base ID (必填)
      --record-ids string   待删除记录 ID 列表，逗号分隔，最多 100 条 (必填)
      --table-id string     Table ID (必填)
```

## 注意事项

- **不可逆操作**，调用前建议先 `record query` 确认目标记录
- 需要先通过 `record query` 获取 recordId
- 单次最多删除 100 条记录
