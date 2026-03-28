---
name: dws-bot-search-groups-by-keyword
description: "机器人消息: 根据关键词搜索我的群会话信息，包含群openconversationId、群名称等信息."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws bot search_groups_by_keyword --help"
---

# bot search_groups_by_keyword

> **PREREQUISITE:** Read `../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

根据关键词搜索我的群会话信息，包含群openconversationId、群名称等信息

## Usage

```bash
dws bot search_groups_by_keyword --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--cursor` | — | — | 分页游标，从0开始 |
| `--keyword` | ✓ | — | 搜索关键词 |

## Required Fields

- `keyword`

## See Also

- [dws-shared](../dws-shared/SKILL.md) — Global rules and auth
- [dws-bot](./SKILL.md) — Product skill
