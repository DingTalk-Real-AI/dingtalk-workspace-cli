---
name: dws-calendar-room-add
description: "钉钉日历: 添加会议室."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws calendar room add --help"
---

# calendar room add

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

添加会议室

## Usage

```bash
dws calendar room add --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--event` | ✓ | — | 日程ID，调用查询日程列表接口获取id参数值。 |
| `--rooms` | ✓ | — | 需要预定的会议室roomId列表，可调用查询空闲会议室接口获取，一个日程最多添加5个会议室。 |

## Required Fields

- `eventId`
- `roomIds`

> [!CAUTION]
> This is a **write** command — confirm with the user before executing.

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-calendar](../SKILL.md) — Product skill
