---
name: dws-contact-user-search-mobile
description: "钉钉通讯录: 通过手机号搜索获取用户名称和userId。"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws contact user search-mobile --help"
---

# contact user search-mobile

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

通过手机号搜索获取用户名称和userId。

## Usage

```bash
dws contact user search-mobile --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--mobile` | ✓ | — | 搜索的手机号 |

## Required Fields

- `mobile`

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-contact](../SKILL.md) — Product skill
