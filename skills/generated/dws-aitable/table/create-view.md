---
name: dws-aitable-table-create-view
description: "钉钉 AI 表格: 在指定数据表（Table）下创建一个新视图（View）。
当前稳定支持的 viewType：Grid、FormDesigner、Gantt、Calendar、Kanban、Gallery。
若未传 viewName…"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws aitable table create_view --help"
---

# aitable table create_view

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

在指定数据表（Table）下创建一个新视图（View）。
当前稳定支持的 viewType：Grid、FormDesigner、Gantt、Calendar、Kanban、Gallery。
若未传 viewName，则会按视图类型自动生成不重名名称。
首列字段是每条数据的索引，不支持删除、移动或隐藏。

## Usage

```bash
dws aitable table create_view --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--baseId` | ✓ | — | 所属 Base 的唯一标识。 |
| `--filter` | — | — | 可选，视图筛选规则列表。 |
| `--group` | — | — | 可选，视图分组规则列表。 |
| `--sort` | — | — | 可选，视图排序规则列表。 |
| `--visibleFieldIds` | — | — | 可选，创建后视图的可见字段列，以及顺序（fieldId 列表）。首列字段是每条数据的索引，必须保留在数组第一个位置，不能删除、移动或隐藏。 |
| `--tableId` | ✓ | — | 所属数据表（Table）的唯一标识。 |
| `--viewDescription` | — | — | 可选，视图描述，结构与前端 ViewDTO.description 保持一致。 |
| `--viewName` | — | — | 可选，新视图名称；未传时自动生成。 |
| `--viewSubType` | — | — | 可选，视图子类型。 |
| `--viewType` | ✓ | — | 新视图类型。当前支持：Grid、FormDesigner、Gantt、Calendar、Kanban、Gallery。 |

## Required Fields

- `baseId`
- `tableId`
- `viewType`

> [!CAUTION]
> This is a **write** command — confirm with the user before executing.

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-aitable](../SKILL.md) — Product skill
