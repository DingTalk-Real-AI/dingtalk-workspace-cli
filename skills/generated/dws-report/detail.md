---
name: dws-report-detail
description: "钉钉日志: 获取指定一篇日志的详情信息."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws report detail --help"
---

# report detail

> **PREREQUISITE:** Read `../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

获取指定一篇日志的详情信息

## Usage

```bash
dws report detail --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--report-id` | ✓ | — | 日志Id，可以从查询当前用户收到的日志列表获取 |

## Required Fields

- `report_id`

## See Also

- [dws-shared](../dws-shared/SKILL.md) — Global rules and auth
- [dws-report](./SKILL.md) — Product skill
