---
name: dws-attendance-summary
description: "钉钉考勤打卡: 获取考勤统计摘要."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws attendance summary --help"
---

# attendance summary

> **PREREQUISITE:** Read `../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

获取考勤统计摘要

## Usage

```bash
dws attendance summary --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--date` | — | — | 查询日期 |
| `--user` | — | — | 用户ID |

## See Also

- [dws-shared](../dws-shared/SKILL.md) — Global rules and auth
- [dws-attendance](./SKILL.md) — Product skill
