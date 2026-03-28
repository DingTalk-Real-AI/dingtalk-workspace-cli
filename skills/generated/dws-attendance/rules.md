---
name: dws-attendance-rules
description: "钉钉考勤打卡: 查询考勤组/考勤规则：'我属于哪个考勤组''我们的打卡范围是什么''弹性工时是怎么算的'."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws attendance rules --help"
---

# attendance rules

> **PREREQUISITE:** Read `../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

查询考勤组/考勤规则："我属于哪个考勤组""我们的打卡范围是什么""弹性工时是怎么算的"

## Usage

```bash
dws attendance rules --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--date` | ✓ | — | 考勤日期 格式：yyyy-MM-dd HH:mm:ss |

## Required Fields

- `date`

## See Also

- [dws-shared](../dws-shared/SKILL.md) — Global rules and auth
- [dws-attendance](./SKILL.md) — Product skill
