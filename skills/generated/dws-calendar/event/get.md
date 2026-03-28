---
name: dws-calendar-event-get
description: "钉钉日历: 获取我的日历指定日程的详细信息."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws calendar event get --help"
---

# calendar event get

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

获取我的日历指定日程的详细信息

## Usage

```bash
dws calendar event get --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--id` | ✓ | — | 日程ID，可调用创建日程接口或查询日程列表接口获取id参数值 |

## Required Fields

- `eventId`

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-calendar](../SKILL.md) — Product skill
