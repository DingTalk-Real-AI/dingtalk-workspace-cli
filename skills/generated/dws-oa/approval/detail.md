---
name: dws-oa-approval-detail
description: "钉钉OA审批: 获取指定审批实例的详情信息."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws oa approval detail --help"
---

# oa approval detail

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

获取指定审批实例的详情信息

## Usage

```bash
dws oa approval detail --json '{...}'
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
