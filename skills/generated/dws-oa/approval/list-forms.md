---
name: dws-oa-approval-list-forms
description: "钉钉OA审批: 获取当前用户可见的审批表单列表，可获取审批表单的processCode。"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws oa approval list-forms --help"
---

# oa approval list-forms

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

获取当前用户可见的审批表单列表，可获取审批表单的processCode。

## Usage

```bash
dws oa approval list-forms --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--cursor` | ✓ | — | 分页游标，首次调用需要传0 |
| `--size` | ✓ | — | 每页大小，最大值100 |

## Required Fields

- `cursor`
- `pageSize`

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-oa](../SKILL.md) — Product skill
