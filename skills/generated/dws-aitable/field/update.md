---
name: dws-aitable-field-update
description: "钉钉 AI 表格: 更新指定字段的名称或配置。不可变更字段类型（type 不可修改）。
newFieldName、config、aiConfig 至少传入一项."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws aitable field update --help"
---

# aitable field update

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

更新指定字段的名称或配置。不可变更字段类型（type 不可修改）。
newFieldName、config、aiConfig 至少传入一项

## Usage

```bash
dws aitable field update --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--base-id` | ✓ | — | Base ID（可通过 list_bases 获取） |
| `--config` | — | — | 更新后的字段物理配置，结构与 create_fields.fields[].config 完全一致。
不修改配置时省略。
注意：更新 singleSelect/multipleSelect 的 options 时，需传入完整列表（含已有选项），系统以新列表整体覆盖，不是追加。
注意：为避免已有单元格因 option id 变化而丢数据，更新时已有选项应尽量回传原 id；新增选项无需传 id。
注意：如果请求中传入的 option id 在当前字段配置中不存在，系统会丢弃该 id，并按新增选项处理；若 id 合法但 name 改了，属于正常更新，会保留该 id。 |
| `--field-id` | ✓ | — | Field ID（可通过 get_tables 获取） |
| `--name` | — | — | 更新后的字段名称，最大100字。不修改名称时省略 |
| `--table-id` | ✓ | — | Table ID（可通过 get_base 获取） |

## Required Fields

- `baseId`
- `fieldId`
- `tableId`

> [!CAUTION]
> This is a **write** command — confirm with the user before executing.

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-aitable](../SKILL.md) — Product skill
