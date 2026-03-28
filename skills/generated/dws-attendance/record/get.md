---
name: dws-attendance-record-get
description: "钉钉考勤打卡: 查询指定用户在某一天的考勤详情，包括实际打卡记录（如上班/下班时间、是否正常打卡）、当日所排班次、所属考勤组信息、是否为休息日、出勤工时（如 '0Hours'）、加班时长等。返回数据受组织权限和隐私策略限制，仅当调用者有权…"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws attendance record get --help"
---

# attendance record get

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

查询指定用户在某一天的考勤详情，包括实际打卡记录（如上班/下班时间、是否正常打卡）、当日所排班次、所属考勤组信息、是否为休息日、出勤工时（如 "0Hours"）、加班时长等。返回数据受组织权限和隐私策略限制，仅当调用者有权限查看该用户考勤信息时才返回有效内容。适用于员工自助查询、HR 核对出勤或审批关联场景。

## Usage

```bash
dws attendance record get --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--user` | ✓ | — | 要查询的用户的userId |
| `--date` | ✓ | — | 要查询的日期的Unix时间戳，仅保留日期信息，单位毫秒。 |

## Required Fields

- `userId`
- `workDate`

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-attendance](../SKILL.md) — Product skill
