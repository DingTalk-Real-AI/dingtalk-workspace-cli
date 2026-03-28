---
name: dws-aitable-table-get-views
description: "钉钉 AI 表格: 获取指定数据表（Table）中的视图（View）完整信息，包括列顺序、筛选、排序、分组、条件格式、自定义配置等。
支持两种模式：
- 显式选择：传入 viewIds，按入参顺序返回这些视图；单次最多 10 个。…"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws aitable table get_views --help"
---

# aitable table get_views

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

获取指定数据表（Table）中的视图（View）完整信息，包括列顺序、筛选、排序、分组、条件格式、自定义配置等。
支持两种模式：
- 显式选择：传入 viewIds，按入参顺序返回这些视图；单次最多 10 个。
- 默认全量：省略 viewIds，返回当前表下全部视图，顺序与当前表视图目录一致。

## Usage

```bash
dws aitable table get_views --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--baseId` | ✓ | — | 所属 Base 的唯一标识。 |
| `--tableId` | ✓ | — | 所属数据表（Table）的唯一标识。 |
| `--viewIds` | — | — | 可选，待获取详情的视图（View）ID 列表。显式传入时单次最多 10 个；省略时默认返回当前表下全部视图。 |

## Required Fields

- `baseId`
- `tableId`

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-aitable](../SKILL.md) — Product skill
