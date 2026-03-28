---
name: dws-calendar-event-update
description: "钉钉日历: 修改现有日程的信息，支持更新标题、时间、地点等任意字段，需要组织者权限。（修改参与人需要使用给日程添加参与人或给日程删除参与人工具）."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws calendar event update --help"
---

# calendar event update

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

修改现有日程的信息，支持更新标题、时间、地点等任意字段，需要组织者权限。（修改参与人需要使用给日程添加参与人或给日程删除参与人工具）

## Usage

```bash
dws calendar event update --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--end` | — | — | 日程结束时间，格式为ISO-8601的带时区的date-time格式，例如2025-11-14T10:00:00+08:00。 |
| `--id` | ✓ | — | 日程ID，可调用创建日程接口或查询日程列表接口获取id参数值 |
| `--start` | — | — | 日程开始时间，格式为ISO-8601的带时区的date-time格式，例如2025-11-14T10:00:00+08:00。 |
| `--title` | — | — | 日程标题，最大不超过2048个字符。 |

## Required Fields

- `eventId`

> [!CAUTION]
> This is a **write** command — confirm with the user before executing.

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-calendar](../SKILL.md) — Product skill
