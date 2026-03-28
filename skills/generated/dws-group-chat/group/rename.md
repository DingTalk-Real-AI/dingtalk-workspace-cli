---
name: dws-group-chat-group-rename
description: "钉钉群聊: 更新群名称."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws chat group rename --help"
---

# group-chat group rename

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

更新群名称

## Usage

```bash
dws chat group rename --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--name` | ✓ | — | 修改后的群名称 |
| `--id` | ✓ | — | 群Id，可从根据关键词搜索群列表服务获取 |

## Required Fields

- `group_name`
- `openconversation_id`

> [!CAUTION]
> This is a **write** command — confirm with the user before executing.

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-group-chat](../SKILL.md) — Product skill
