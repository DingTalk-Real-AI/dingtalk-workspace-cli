---
name: dws-aitable-field-delete
description: "钉钉 AI 表格: 删除指定 Table 中的一个字段（Field），删除操作不可逆。禁止删除主字段，且禁止删除最后一个字段

此操作不可逆，会永久删除字段及其所有数据。
必须提供准确的 baseId、tableId 和 field…"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws aitable field delete --help"
---

# aitable field delete

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

删除指定 Table 中的一个字段（Field），删除操作不可逆。禁止删除主字段，且禁止删除最后一个字段

此操作不可逆，会永久删除字段及其所有数据。
必须提供准确的 baseId、tableId 和 fieldId，不得使用名称代替 ID。
若字段不存在或无权限，将返回错误。

## Usage

```bash
dws aitable field delete --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--base-id` | ✓ | — | Base ID（通过 list_bases 获取） |
| `--field-id` | ✓ | — | 待删除字段 ID（通过 get_tables 获取） |
| `--table-id` | ✓ | — | Table ID（通过 get_base 获取） |

## Required Fields

- `baseId`
- `fieldId`
- `tableId`

> [!CAUTION]
> This is a **write** command — confirm with the user before executing.

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-aitable](../SKILL.md) — Product skill
