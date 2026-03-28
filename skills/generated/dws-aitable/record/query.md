---
name: dws-aitable-record-query
description: "钉钉 AI 表格: 查询指定表格中的记录，支持两种模式：
- 按 ID 取：传入 recordIds（单次最多 100 个），直接获取指定记录。
- 条件查：通过 filters 过滤、sort 排序、cursor 分页遍历全表。…"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws aitable record query --help"
---

# aitable record query

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

查询指定表格中的记录，支持两种模式：
- 按 ID 取：传入 recordIds（单次最多 100 个），直接获取指定记录。
- 条件查：通过 filters 过滤、sort 排序、cursor 分页遍历全表。
两种模式均可通过 fieldIds（单次最多 100 个）限制返回字段以节省 token。

## Usage

```bash
dws aitable record query --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--base-id` | ✓ | — | Base ID（通过 list_bases / search_bases 获取） |
| `--cursor` | — | — | 可选。分页游标，首次查询不传。当返回结果包含 cursor 字段时，将其传入下一次请求以获取后续数据；
cursor 为空表示已取完全部记录。 |
| `--field-ids` | — | — | 可选。指定要返回的字段 ID 列表。省略则返回所有字段。
建议在字段较多时按需传入，可显著减少响应体积；单次最多 100 个。 |
| `--filters` | — | — | 结构化过滤条件，不传则返回全部记录（受 limit 限制） |
| `--keyword` | — | — | 全文关键词。将对整表内容做文本匹配搜索，并返回符合条件的记录。 |
| `--limit` | — | — | 可选。单次返回的最大记录数，默认 100，最大 100。 |
| `--record-ids` | — | — | 可选。指定要获取的记录 ID 列表，单次最多 100 个。传入时直接按 ID 返回，忽略 filters 和 sort。
适用于已知 recordId（如关联字段中的 linkedRecordIds）时的精准取数。 |
| `--sort` | — | — | 可选。排序条件列表，按数组顺序依次生效。

每个元素：{"fieldId": "<fieldId>", "direction": "asc" | "desc"}

示例（先按优先级升序，再按截止日期降序）：
[
  {"fieldId": "fldPriorityId", "direction": "asc"},
  {"fieldId": "fldDueDateId",  "direction": "desc"}
] |
| `--table-id` | ✓ | — | Table ID（通过 get_base 获取） |

## Required Fields

- `baseId`
- `tableId`

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-aitable](../SKILL.md) — Product skill
