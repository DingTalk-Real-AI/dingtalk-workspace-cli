---
name: dws-calendar-participant-delete
description: "钉钉日历: 从已存在的指定日程中移除参与者，支持批量移除多人."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws calendar participant delete --help"
---

# calendar participant delete

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

从已存在的指定日程中移除参与者，支持批量移除多人

## Usage

```bash
dws calendar participant delete --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--users` | ✓ | — | 需要被删除的日程参与者userId列表。 |
| `--event` | ✓ | — | 日程ID，可调用创建日程接口或查询日程列表接口获取id参数值。 |

## Required Fields

- `attendeesToRemove`
- `eventId`

> [!CAUTION]
> This is a **write** command — confirm with the user before executing.

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-calendar](../SKILL.md) — Product skill
