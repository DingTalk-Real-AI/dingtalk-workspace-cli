---
name: dws-aitable-record-delete
description: "钉钉 AI 表格: 在指定 Table 中批量删除记录（不可逆，数据将永久丢失）。
单次最多删除 100 条；超出请拆分多次调用。
调用前建议先通过 query_records 确认目标记录 ID 与内容，避免误删。"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws aitable record delete --help"
---

# aitable record delete

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

在指定 Table 中批量删除记录（不可逆，数据将永久丢失）。
单次最多删除 100 条；超出请拆分多次调用。
调用前建议先通过 query_records 确认目标记录 ID 与内容，避免误删。

## Usage

```bash
dws aitable record delete --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--base-id` | ✓ | — | Base ID，可通过 list_bases 或 search_bases 获取 |
| `--record-ids` | ✓ | — | 待删除的记录 ID 列表，最多 100 条 |
| `--table-id` | ✓ | — | Table ID，可通过 get_base 获取 |

## Required Fields

- `baseId`
- `recordIds`
- `tableId`

> [!CAUTION]
> This is a **write** command — confirm with the user before executing.

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-aitable](../SKILL.md) — Product skill
