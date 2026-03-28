---
name: dws-todo-task-create
description: "钉钉待办: 在当前企业组织内创建一条个人待办事项，支持设置标题、执行人列表（用户 ID）、截止时间、优先级（如高/中/低）。待办将归属于当前用户，并对有权限的协作者可见。"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws todo task create --help"
---

# todo task create

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

在当前企业组织内创建一条个人待办事项，支持设置标题、执行人列表（用户 ID）、截止时间、优先级（如高/中/低）。待办将归属于当前用户，并对有权限的协作者可见。

## Usage

```bash
dws todo task create --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--due` | — | — | 待办截止时间，unix时间戳，精确到毫秒 |
| `--executors` | ✓ | — | 待办执行者。可传入多个 |
| `--priority` | — | — | 待办优先级: 10:低，20:普通，30:较高，40:紧急 |
| `--title` | ✓ | — | 待办的标题 |

## Required Fields

- `PersonalTodoCreateVO.executorIds`
- `PersonalTodoCreateVO.subject`

> [!CAUTION]
> This is a **write** command — confirm with the user before executing.

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-todo](../SKILL.md) — Product skill
