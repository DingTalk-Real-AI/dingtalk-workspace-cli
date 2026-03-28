---
name: dws-aitable-field-create
description: "钉钉 AI 表格: 在已有表格中批量新增字段。适用于建表后补充一批字段，或一次性添加多个关联、流转等复杂类型字段。单次最多创建 15 个字段；若超过该数量，请拆分多次调用。允许部分成功，返回结果会逐项说明每个字段是否创建成功；失败项会返回…"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws aitable field create --help"
---

# aitable field create

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

在已有表格中批量新增字段。适用于建表后补充一批字段，或一次性添加多个关联、流转等复杂类型字段。单次最多创建 15 个字段；若超过该数量，请拆分多次调用。允许部分成功，返回结果会逐项说明每个字段是否创建成功；失败项会返回 reason 说明失败原因。

## Usage

```bash
dws aitable field create --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--base-id` | ✓ | — | Base ID（通过 list_bases 获取） |
| `--fields` | ✓ | — | 待新增字段列表，至少包含 1 个字段，单次最多 15 个。系统会按数组顺序依次创建，返回结果顺序与入参保持一致，并逐项标明成功/失败状态。 |
| `--table-id` | ✓ | — | Table ID（通过 get_base 获取） |

## Required Fields

- `baseId`
- `fields`
- `tableId`

> [!CAUTION]
> This is a **write** command — confirm with the user before executing.

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-aitable](../SKILL.md) — Product skill
