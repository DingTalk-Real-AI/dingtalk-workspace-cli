---
name: dws-aitable-table-delete
description: "钉钉 AI 表格: 删除指定 tableId 的数据表（不可逆，数据将永久丢失），该操作为高风险写入。
调用前请先通过 get_base / get_tables 确认目标表 ID 与名称。"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws aitable table delete --help"
---

# aitable table delete

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

删除指定 tableId 的数据表（不可逆，数据将永久丢失），该操作为高风险写入。
调用前请先通过 get_base / get_tables 确认目标表 ID 与名称。

## Usage

```bash
dws aitable table delete --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--base-id` | ✓ | — | 目标 Base ID（通过 list_bases 获取） |
| `--reason` | — | — | 一句话描述一下删除该数据表的原因，用于审计 |
| `--table-id` | ✓ | — | 将被删除的 Table ID（通过 get_base / get_tables 获取） |

## Required Fields

- `baseId`
- `tableId`

> [!CAUTION]
> This is a **write** command — confirm with the user before executing.

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-aitable](../SKILL.md) — Product skill
