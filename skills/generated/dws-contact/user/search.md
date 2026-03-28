---
name: dws-contact-user-search
description: "钉钉通讯录: 搜索组织内成员，并返回成员的userId。如果需要查询详情，需要调用另外一个工具."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws contact user search --help"
---

# contact user search

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

搜索组织内成员，并返回成员的userId。如果需要查询详情，需要调用另外一个工具

## Usage

```bash
dws contact user search --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--keyword` | ✓ | — | 搜索关键词 |

## Required Fields

- `keyWord`

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-contact](../SKILL.md) — Product skill
