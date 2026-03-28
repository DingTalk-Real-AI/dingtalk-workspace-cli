---
name: dws-calendar-busy-search
description: "钉钉日历: 查询指定用户在给定时间范围内的闲忙状态，返回其日历中已占用时间段的详细日程信息（如标题、开始/结束时间），不包含具体日程内容细节（如参与人、地点），以保护隐私。结果受组织可见性策略控制：仅当调用者有权限查看该用户日历时方可获取…"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws calendar busy search --help"
---

# calendar busy search

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

查询指定用户在给定时间范围内的闲忙状态，返回其日历中已占用时间段的详细日程信息（如标题、开始/结束时间），不包含具体日程内容细节（如参与人、地点），以保护隐私。结果受组织可见性策略控制：仅当调用者有权限查看该用户日历时方可获取有效数据。适用于安排会议前快速确认他人可用时间。

## Usage

```bash
dws calendar busy search --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--end` | ✓ | — | 查询的结束时间，时间戳（毫秒级）格式。 |
| `--start` | ✓ | — | 查询的开始时间，时间戳（毫秒级）格式。 |
| `--users` | ✓ | — | 用户uid列表最大长度 20。 |

## Required Fields

- `endTime`
- `startTime`
- `userIds`

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-calendar](../SKILL.md) — Product skill
