---
name: dws-ding-search-my-robots
description: "DING消息: 搜索我创建的机器人."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws ding search_my_robots --help"
---

# ding search_my_robots

> **PREREQUISITE:** Read `../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

搜索我创建的机器人

## Usage

```bash
dws ding search_my_robots --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--currentPage` | ✓ | — | 页码，从1开始 |
| `--pageSize` | — | — | 每页条数，默认50 |
| `--robotName` | — | — | 要搜索的名称 |

## Required Fields

- `currentPage`

## See Also

- [dws-shared](../dws-shared/SKILL.md) — Global rules and auth
- [dws-ding](./SKILL.md) — Product skill
