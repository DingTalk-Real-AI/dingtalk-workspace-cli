---
name: dws-oa-approval-records
description: "钉钉OA审批: 获取某个审批实例的审批操作记录信息，获取的是该审批实例有哪些人做了什么操作，以及操作结果是什么."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws oa approval records --help"
---

# oa approval records

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

获取某个审批实例的审批操作记录信息，获取的是该审批实例有哪些人做了什么操作，以及操作结果是什么

## Usage

```bash
dws oa approval records --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--instance-id` | ✓ | — | 审批实例Id |

## Required Fields

- `processInstanceId`

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-oa](../SKILL.md) — Product skill
