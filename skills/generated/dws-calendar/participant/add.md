---
name: dws-calendar-participant-add
description: "钉钉日历: 向已存在的指定日程添加参与者，支持批量添加多人，可设置参与者类型和通知方式."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws calendar participant add --help"
---

# calendar participant add

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

向已存在的指定日程添加参与者，支持批量添加多人，可设置参与者类型和通知方式

## Usage

```bash
dws calendar participant add --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--users` | ✓ | — | 需要添加的参与人uid列表。 |
| `--event` | ✓ | — | 日程ID，可调用创建日程接口或查询日程列表接口获取id参数值 |

## Required Fields

- `attendeesToAdd`
- `eventId`

> [!CAUTION]
> This is a **write** command — confirm with the user before executing.

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-calendar](../SKILL.md) — Product skill
