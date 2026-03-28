---
name: dws-report-stats
description: "钉钉日志: 获取日志统计数据，包括评论数量、点赞数量、已读数等."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws report stats --help"
---

# report stats

> **PREREQUISITE:** Read `../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

获取日志统计数据，包括评论数量、点赞数量、已读数等

## Usage

```bash
dws report stats --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--report-id` | ✓ | — | 日志Id |

## Required Fields

- `report_id`

## See Also

- [dws-shared](../dws-shared/SKILL.md) — Global rules and auth
- [dws-report](./SKILL.md) — Product skill
