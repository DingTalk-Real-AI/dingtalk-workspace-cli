---
name: dws-aitable-record-update
description: "钉钉 AI 表格: 批量更新指定记录的字段值，只需传入需修改的字段，未传入的字段保持原值."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws aitable record update --help"
---

# aitable record update

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

批量更新指定记录的字段值，只需传入需修改的字段，未传入的字段保持原值

## Usage

```bash
dws aitable record update --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--base-id` | ✓ | — | Base ID，可通过 list_bases 或 search_bases 获取 |
| `--records` | ✓ | — | 待更新的记录内容列表，单次最多 100 条 |
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
