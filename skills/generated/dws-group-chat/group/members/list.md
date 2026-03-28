---
name: dws-group-chat-group-members-list
description: "钉钉群聊: 查群成员列表."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws chat group members list --help"
---

# group-chat group members list

> **PREREQUISITE:** Read `../../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

查群成员列表

## Usage

```bash
dws chat group members list --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--cursor` | — | — | 分页游标，首次从0开始 |
| `--id` | ✓ | — | 群Id，可以从根据关键词搜索群列表服务获取 |

## Required Fields

- `openconversation_id`

## See Also

- [dws-shared](../../../dws-shared/SKILL.md) — Global rules and auth
- [dws-group-chat](../../SKILL.md) — Product skill
