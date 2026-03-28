---
name: dws-aitable-table-get
description: "钉钉 AI 表格: 批量获取指定 Tables（数据表）的表级信息、字段目录与视图目录。
会返回 tables 列表；每个 table 直接包含 tableId、tableName、description、fields、views；字段…"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws aitable table get --help"
---

# aitable table get

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

批量获取指定 Tables（数据表）的表级信息、字段目录与视图目录。
会返回 tables 列表；每个 table 直接包含 tableId、tableName、description、fields、views；字段列表仅包含 fieldId、fieldName、type、description；views 仅包含 viewId、viewName、type。
若需读取字段的完整配置，请再调用 get_fields。

## Usage

```bash
dws aitable table get --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--base-id` | ✓ | — | 所属 Base ID（通过 list_bases / search_bases 获取） |
| `--table-ids` | — | — | 待获取详情的 Table ID 列表（通过 get_base 获取），单次最多 10 个；不传则默认返回当前 Base 下全部表。建议优先显式传入，以控制返回体大小，避免上下文突增 |

## Required Fields

- `baseId`

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-aitable](../SKILL.md) — Product skill
