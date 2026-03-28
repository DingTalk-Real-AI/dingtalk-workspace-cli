---
name: dws-attendance-shift-list
description: "钉钉考勤打卡: 批量查询多个员工在指定日期的考勤班次信息，返回每条记录包含：用户 ID（userId）、工作日期（workDate，毫秒时间戳）、打卡类型（checkType，如 OnDuty 表示上班）、计划打卡时间（planCheck…"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws attendance shift list --help"
---

# attendance shift list

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

批量查询多个员工在指定日期的考勤班次信息，返回每条记录包含：用户 ID（userId）、工作日期（workDate，毫秒时间戳）、打卡类型（checkType，如 OnDuty 表示上班）、计划打卡时间（planCheckTime，毫秒时间戳）以及是否为休息日（isRest，"Y"/"N"）。结果基于组织考勤配置生成，仅返回调用者有权限查看的员工数据，适用于排班核对、考勤预览等场景。

## Usage

```bash
dws attendance shift list --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--start` | ✓ | — | 起始日期，Unix时间戳，单位毫秒。 开始时间和结束时间的间隔不能超过7天。 查询时间限制距今180天内。 |
| `--end` | ✓ | — | 结束日期，Unix时间戳，单位毫秒。开始时间和结束时间的间隔不能超过7天。 查询时间限制距今180天内。 |
| `--users` | ✓ | — | 要查询的人员userId列表，多个userId用列表表示，一次最多可传50个。 |

## Required Fields

- `fromDateTime`
- `toDateTime`
- `userIds`

> [!CAUTION]
> This is a **write** command — confirm with the user before executing.

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-attendance](../SKILL.md) — Product skill
