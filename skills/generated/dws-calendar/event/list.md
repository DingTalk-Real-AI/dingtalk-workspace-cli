---
name: dws-calendar-event-list
description: "钉钉日历: 仅允许查询当前用户指定时间范围内的日程列表，最多返回100条."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws calendar event list --help"
---

# calendar event list

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

仅允许查询当前用户指定时间范围内的日程列表，最多返回100条

## Usage

```bash
dws calendar event list --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--end` | — | — | 日程结束时间，时间戳（毫秒级） |
| `--start` | — | — | 日程开始时间，时间戳（毫秒级） |

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-calendar](../SKILL.md) — Product skill
