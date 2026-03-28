---
name: dws-calendar-event-delete
description: "钉钉日历: 删除指定日程，组织者删除将通知所有参与者，参与者删除仅从自己日历移除."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws calendar event delete --help"
---

# calendar event delete

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

删除指定日程，组织者删除将通知所有参与者，参与者删除仅从自己日历移除

## Usage

```bash
dws calendar event delete --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--id` | ✓ | — | 日程ID，可调用创建日程接口或查询日程列表接口获取id参数值。 |

## Required Fields

- `eventId`

> [!CAUTION]
> This is a **write** command — confirm with the user before executing.

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-calendar](../SKILL.md) — Product skill
