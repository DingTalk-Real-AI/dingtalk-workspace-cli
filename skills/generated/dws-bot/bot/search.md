---
name: dws-bot-bot-search
description: "机器人消息: 搜索我创建的机器人，可获取机器人robotCode等信息。"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws bot bot search --help"
---

# bot bot search

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

搜索我创建的机器人，可获取机器人robotCode等信息。

## Usage

```bash
dws bot bot search --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--page` | ✓ | — | 页码，从1开始 |
| `--size` | — | — | 每页条数，默认50 |
| `--name` | — | — | 要搜索的名称 |

## Required Fields

- `currentPage`

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-bot](../SKILL.md) — Product skill
