---
name: dws-oa-approval-revoke
description: "钉钉OA审批: 撤销当前用户已经发起的审批实例，需要的参数processInstanceId可以从."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws oa approval revoke --help"
---

# oa approval revoke

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

撤销当前用户已经发起的审批实例，需要的参数processInstanceId可以从

## Usage

```bash
dws oa approval revoke --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--instance-id` | ✓ | — | 需要撤销的审批实例Id |
| `--remark` | — | — | 撤销审批的说明 |

## Required Fields

- `processInstanceId`

> [!CAUTION]
> This is a **write** command — confirm with the user before executing.

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-oa](../SKILL.md) — Product skill
