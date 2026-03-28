---
name: dws-aitable-record-create
description: "钉钉 AI 表格: 在指定表格中批量新增记录."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws aitable record create --help"
---

# aitable record create

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

在指定表格中批量新增记录

## Usage

```bash
dws aitable record create --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--base-id` | ✓ | — | Base ID，可通过 list_bases 或 search_bases 获取 |
| `--records` | ✓ | — | 待创建的记录列表，单次最多 100 条 |
| `--table-id` | ✓ | — | Table ID，可通过 get_base 获取 |

## Required Fields

- `baseId`
- `records`
- `tableId`

> [!CAUTION]
> This is a **write** command — confirm with the user before executing.

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-aitable](../SKILL.md) — Product skill
