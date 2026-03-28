---
name: dws-todo-task-update
description: "钉钉待办: 修改整个待办任务."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws todo task update --help"
---

# todo task update

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

修改整个待办任务

## Usage

```bash
dws todo task update --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--due` | — | — | 待办截止时间，unix时间戳，精确到毫秒 |
| `--done` | — | — | 待办完成状态。true：已完成；alse：未完成 |
| `--priority` | — | — | 待办优先级: 10:低，20:普通，30:较高，40:紧急 |
| `--title` | — | — | 待办标题 |
| `--task-id` | ✓ | — | 待办任务唯一标识id |

## Required Fields

- `TodoUpdateRequest.taskId`

> [!CAUTION]
> This is a **write** command — confirm with the user before executing.

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-todo](../SKILL.md) — Product skill
