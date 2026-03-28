---
name: dws-aitable-base-get
description: "钉钉 AI 表格: 获取指定 Base 的资源目录级信息，返回 baseName、tables、dashboards 的 summary 信息（不含字段与记录详情）。
这是当前 Base 级目录入口：后续如需 tableId 或 das…"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws aitable base get --help"
---

# aitable base get

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

获取指定 Base 的资源目录级信息，返回 baseName、tables、dashboards 的 summary 信息（不含字段与记录详情）。
这是当前 Base 级目录入口：后续如需 tableId 或 dashboardId，优先从这里读取；table 详情再调用 get_tables，dashboard 详情再调用 get_dashboard

## Usage

```bash
dws aitable base get --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--base-id` | ✓ | — | Base 唯一标识。优先使用 search_bases/list_bases 返回值 |

## Required Fields

- `baseId`

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-aitable](../SKILL.md) — Product skill
