---
name: dws-aitable-table-update
description: "钉钉 AI 表格: 重命名指定 Table（数据表）。若新名称不符合命名要求、与同一 Base 下其他表重名或无权限，将返回错误。"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws aitable table update --help"
---

# aitable table update

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

重命名指定 Table（数据表）。若新名称不符合命名要求、与同一 Base 下其他表重名或无权限，将返回错误。

## Usage

```bash
dws aitable table update --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--base-id` | ✓ | — | 所属 Base ID（用于定位目标表）。 |
| `--name` | ✓ | — | 新表名。需非空；不能包含 / \ ? * [ ] : 等特殊字符。 |
| `--table-id` | ✓ | — | 目标 Table ID（通过 get_base 获取）。 |

## Required Fields

- `baseId`
- `newTableName`
- `tableId`

> [!CAUTION]
> This is a **write** command — confirm with the user before executing.

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-aitable](../SKILL.md) — Product skill
