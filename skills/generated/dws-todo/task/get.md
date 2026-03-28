---
name: dws-todo-task-get
description: "钉钉待办: 查询待办详情."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws todo task get --help"
---

# todo task get

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

查询待办详情

## Usage

```bash
dws todo task get --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--task-id` | ✓ | — | taskId |

## Required Fields

- `taskId`

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-todo](../SKILL.md) — Product skill
