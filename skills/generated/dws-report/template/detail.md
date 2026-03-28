---
name: dws-report-template-detail
description: "钉钉日志: 获取当前员工可使用的日志模版详情信息，包括日志模板Id、日志模板内字段的名称、字段类型、字段排序等."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws report template detail --help"
---

# report template detail

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

获取当前员工可使用的日志模版详情信息，包括日志模板Id、日志模板内字段的名称、字段类型、字段排序等

## Usage

```bash
dws report template detail --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--name` | ✓ | — | 日志模板名称 |

## Required Fields

- `report_template_name`

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-report](../SKILL.md) — Product skill
