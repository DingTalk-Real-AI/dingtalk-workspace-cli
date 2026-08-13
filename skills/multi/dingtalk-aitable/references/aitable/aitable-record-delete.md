# record delete — 删除记录

> 加载边界：仅在用户明确要求删除记录时读取。先 query 并展示真实 recordId、关键字段和数量，确认前零删除调用；删除后重新按 ID 查询验证不存在，状态未知时禁止自动重试。

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
