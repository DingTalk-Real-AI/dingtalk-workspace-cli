---
name: dws-report-template-list
description: "钉钉日志: 获取当前员工可使用的日志模版信息，包含日志模板的名称、模板Id等."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws report template list --help"
---

# report template list

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

获取当前员工可使用的日志模版信息，包含日志模板的名称、模板Id等

## Usage

```bash
dws report template list --json '{...}'
```

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-report](../SKILL.md) — Product skill
