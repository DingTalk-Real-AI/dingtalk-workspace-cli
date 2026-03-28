---
name: dws-todo-task-list
description: "钉钉待办: 获取当前用户在所属组织中的个人待办事项列表，返回每项待办的标题、截止日期、优先级（如高/中/低）、完成状态。"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws todo task list --help"
---

# todo task list

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

获取当前用户在所属组织中的个人待办事项列表，返回每项待办的标题、截止日期、优先级（如高/中/低）、完成状态。

## Usage

```bash
dws todo task list --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--page` | ✓ | — | 当前页。从1开始 |
| `--size` | ✓ | — | 分页大小。 |
| `--status` | — | — | 待办完成状态。true：已完成；false：未完成。 |

## Required Fields

- `pageNum`
- `pageSize`

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-todo](../SKILL.md) — Product skill
