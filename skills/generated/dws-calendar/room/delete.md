---
name: dws-calendar-room-delete
description: "钉钉日历: 移除日程中预约的会议室."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws calendar room delete --help"
---

# calendar room delete

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

移除日程中预约的会议室

## Usage

```bash
dws calendar room delete --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--event` | ✓ | — | 日程ID，调用查询日程列表接口获取id参数值。 |
| `--rooms` | ✓ | — | 需要删除的会议室roomId列表 |

## Required Fields

- `eventId`
- `roomIds`

> [!CAUTION]
> This is a **write** command — confirm with the user before executing.

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-calendar](../SKILL.md) — Product skill
