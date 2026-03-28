---
name: dws-aitable-table-delete-view
description: "钉钉 AI 表格: 删除指定视图（View）。该操作不可逆。
已知保护：禁止删除数据表中的最后一个视图；锁定视图不允许删除。"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws aitable table delete_view --help"
---

# aitable table delete_view

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

删除指定视图（View）。该操作不可逆。
已知保护：禁止删除数据表中的最后一个视图；锁定视图不允许删除。

## Usage

```bash
dws aitable table delete_view --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--baseId` | ✓ | — | 所属 Base 的唯一标识。 |
| `--tableId` | ✓ | — | 所属数据表（Table）的唯一标识。 |
| `--viewId` | ✓ | — | 要删除的视图（View）的唯一标识。 |

## Required Fields

- `baseId`
- `tableId`
- `viewId`

> [!CAUTION]
> This is a **write** command — confirm with the user before executing.

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-aitable](../SKILL.md) — Product skill
