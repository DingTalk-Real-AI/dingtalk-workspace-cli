---
name: dws-contact-search-contact-by-key-word
description: "钉钉通讯录: 根据关键词搜索好友和同事."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws contact search_contact_by_key_word --help"
---

# contact search_contact_by_key_word

> **PREREQUISITE:** Read `../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

根据关键词搜索好友和同事

## Usage

```bash
dws contact search_contact_by_key_word --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--keyword` | ✓ | — | 搜索的关键词；按照关联性排序 |

## Required Fields

- `keyword`

## See Also

- [dws-shared](../dws-shared/SKILL.md) — Global rules and auth
- [dws-contact](./SKILL.md) — Product skill
