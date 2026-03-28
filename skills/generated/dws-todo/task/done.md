---
name: dws-todo-task-done
description: "钉钉待办: 修改执行者的待办完成状态."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws todo task done --help"
---

# todo task done

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

修改执行者的待办完成状态

## Usage

```bash
dws todo task done --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--status` | ✓ | — | 需要修改待办完成状态结果。true：已完成；false未完成。 |
| `--task-id` | ✓ | — | 待办任务唯一标识id。指定需要修改哪个待办 |

## Required Fields

- `isDone`
- `taskId`

> [!CAUTION]
> This is a **write** command — confirm with the user before executing.

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-todo](../SKILL.md) — Product skill
