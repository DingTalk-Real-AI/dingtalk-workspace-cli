---
name: dws-calendar-event-create
description: "钉钉日历: 创建新的日程，支持设置时间、参与者、提醒等完整功能."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws calendar event create --help"
---

# calendar event create

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

创建新的日程，支持设置时间、参与者、提醒等完整功能

## Usage

```bash
dws calendar event create --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--desc` | — | — | 日程描述，最大不超过5000个字符。 |
| `--end` | ✓ | — | 日程结束时间，格式为ISO-8601的带时区的date-time格式，例如2025-11-14T10:00:00+08:00。 |
| `--start` | ✓ | — | 日程开始时间，格式为ISO-8601的带时区的date-time格式，例如2025-11-14T10:00:00+08:00。 |
| `--title` | ✓ | — | 日程标题，最大不超过2048个字符。 |

## Required Fields

- `endDateTime`
- `startDateTime`
- `summary`

> [!CAUTION]
> This is a **write** command — confirm with the user before executing.

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-calendar](../SKILL.md) — Product skill
