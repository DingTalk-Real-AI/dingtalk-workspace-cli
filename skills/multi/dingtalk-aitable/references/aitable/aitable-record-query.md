# record query — 查询记录

## 命令格式

```
Usage:
  dws aitable record query [flags]
Example:
  dws aitable record query --base-id <BASE_ID> --table-id <TABLE_ID>
  dws aitable record query --base-id <BASE_ID> --table-id <TABLE_ID> --record-ids rec1,rec2
  dws aitable record query --base-id <BASE_ID> --table-id <TABLE_ID> --query "关键词" --limit 50
Flags:
      --base-id string      Base ID (必填)
      --cursor string       分页游标，首次不传
      --field-ids string    返回字段 ID 列表，逗号分隔，单次最多 100 个
      --filters string      结构化过滤条件 JSON
      --query string        全文关键词搜索
      --limit int           单次最大记录数，默认 100，最大 100
      --record-ids string   指定记录 ID 列表，逗号分隔，单次最多 100 个
      --sort string         排序条件 JSON 数组
      --table-id string     Table ID (必填)
      --all                 启用自动翻页，循环获取并合并所有记录后统一输出
      --page-limit int      自动翻页最大页数（仅 --all 时生效）。默认 50，设为 0 表示无限制
```

两种模式: 按 ID 取（传 record-ids，忽略 filters/sort）或条件查（filters+sort+cursor 分页）。

## 自动翻页（--all + --page-limit）

- 传入 `--all` 启用自动翻页，CLI 自动循环获取并合并所有记录后统一输出
- `--page-limit` 控制最大翻页次数，默认 50 页（5000 条），设为 0 表示无限制
- 页间间隔 200ms，中途网络错误会 graceful stop 并输出已获取的数据
- **被截断时**（达到 page-limit 但仍有数据）：输出中包含 `"hasMore": true` 和 `"cursor": "..."` 字段，可通过 `--cursor` 从断点继续拉取
- 适用于需要一次性获取全量数据的场景（如导出、统计、批量处理）

```bash
# 默认（最多 50 页 = 5000 条）
dws aitable record query --base-id X --table-id Y --all
# 无限制（拉完为止）
dws aitable record query --base-id X --table-id Y --all --page-limit 0
# 从上次断点继续
dws aitable record query --base-id X --table-id Y --all --cursor "上次返回的cursor"
```

## 排序参数规范

`--sort` 需要传 JSON 数组，排序方向字段必须是 `direction`（`asc` 或 `desc`），不要使用 `order`。

正确示例：
```bash
--sort '[{"fieldId":"wm8ns9bw2vmucb45xj3ix","direction":"desc"}]'
```

## filters 结构

详细语法见 [aitable-filter-sort.md](./aitable-filter-sort.md)。

快速模板：
```json
{"operator":"and","operands":[{"operator":"eq","operands":["<fieldId>","<value>"]}]}
```

> **singleSelect/multipleSelect 过滤**：filters 中可传 option id 或 option name，但建议优先用 **option id**（通过 `field get` 获取），更可靠。

## 减少响应体积

字段较多时，用 `--field-ids` 仅返回需要的字段，可显著减少返回数据量。

## 常见错误

- `--filters` 根节点直接用 `"operator":"eq"` → API 静默忽略，返回全表
- `--sort` 用 `"order":"desc"` → 必须用 `"direction":"desc"`
- 不加 `--field-ids` 拉全字段 → 大表响应体积过大
- 全量拉取后在 context 里手动统计 → 应优先用 `--filters` 服务端过滤
