---
name: dws-group-chat-search
description: "钉钉群聊: 根据群名称关键词，搜索符合条件的群，返回群的openconversion_id、群名称等信息."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws chat search --help"
---

# group-chat search

> **PREREQUISITE:** Read `../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

根据群名称关键词，搜索符合条件的群，返回群的openconversion_id、群名称等信息

## Usage

```bash
dws chat search --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--cursor` | — | — | cursor |
| `--query` | ✓ | — | query |

## Required Fields

- `OpenSearchRequest.query`

## See Also

- [dws-shared](../dws-shared/SKILL.md) — Global rules and auth
- [dws-group-chat](./SKILL.md) — Product skill
