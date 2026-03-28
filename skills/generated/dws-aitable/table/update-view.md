---
name: dws-aitable-table-update-view
description: "钉钉 AI 表格: 更新指定视图（View）的名称、描述或配置。
当前稳定支持更新：newViewName、viewDescription、visibleFieldIds、filter、sort、group；fieldWidths 仅支…"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws aitable table update_view --help"
---

# aitable table update_view

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

更新指定视图（View）的名称、描述或配置。
当前稳定支持更新：newViewName、viewDescription、visibleFieldIds、filter、sort、group；fieldWidths 仅支持 Grid 视图。
首列字段是每条数据的索引，不支持删除、移动或隐藏。

## Usage

```bash
dws aitable table update_view --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--baseId` | ✓ | — | 所属 Base 的唯一标识。 |
| `--fieldWidths` | — | — | 可选，列宽映射（key 为 fieldId，value 为宽度，单位像素，默认为 200 像素，合法范围由下游校验）。仅支持 Grid 视图；若目标视图不是 Grid，请不要传该字段。 |
| `--filter` | — | — | 可选，新的筛选规则列表，会全量覆盖当前 filter。 |
| `--group` | — | — | 可选，新的分组规则列表，会全量覆盖当前 group。 |
| `--sort` | — | — | 可选，新的排序规则列表，会全量覆盖当前 sort。 |
| `--visibleFieldIds` | — | — | 可选，新的视图可见字段列，以及顺序（fieldId 列表）。需要传全量，不传则不修改。首列字段是每条数据的索引，必须保留在数组第一个位置，不能删除、移动或隐藏。 |
| `--newViewName` | — | — | 可选，新的视图名称。 |
| `--tableId` | ✓ | — | 所属数据表（Table）的唯一标识。 |
| `--viewDescription` | — | — | 可选，新的视图描述。若不传则不修改；如需清空，可传 {"content": []}。 |
| `--viewId` | ✓ | — | 目标视图（View）的唯一标识。 |

## Required Fields

- `baseId`
- `tableId`
- `viewId`

> [!CAUTION]
> This is a **write** command — confirm with the user before executing.

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-aitable](../SKILL.md) — Product skill
