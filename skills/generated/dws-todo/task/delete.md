---
name: dws-todo-task-delete
description: "钉钉待办: 删除待办（所有执行者都删除）."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws todo task delete --help"
---

# todo task delete

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

删除待办（所有执行者都删除）

## Usage

```bash
dws todo task delete --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--task-id` | ✓ | — | taskId |

## Required Fields

- `taskId`

> [!CAUTION]
> This is a **write** command — confirm with the user before executing.

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-todo](../SKILL.md) — Product skill
