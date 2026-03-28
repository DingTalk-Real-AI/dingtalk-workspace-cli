---
name: dws-calendar-participant-list
description: "钉钉日历: 获取指定日程的所有参与者列表及其状态信息."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws calendar participant list --help"
---

# calendar participant list

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

获取指定日程的所有参与者列表及其状态信息

## Usage

```bash
dws calendar participant list --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--event` | ✓ | — | 日程ID，可调用创建日程接口或查询日程列表接口获取id参数值。 |

## Required Fields

- `eventId`

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-calendar](../SKILL.md) — Product skill
