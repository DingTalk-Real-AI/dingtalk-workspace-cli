---
name: dws-group-chat-group-members-add
description: "钉钉群聊: 添加群成员."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws chat group members add --help"
---

# group-chat group members add

> **PREREQUISITE:** Read `../../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

添加群成员

## Usage

```bash
dws chat group members add --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--id` | ✓ | — | 群Id，可以从根据关键词搜索群列表服务获取 |
| `--users` | ✓ | — | 用户ID列表 |

## Required Fields

- `openconversation_id`
- `userId`

> [!CAUTION]
> This is a **write** command — confirm with the user before executing.

## See Also

- [dws-shared](../../../dws-shared/SKILL.md) — Global rules and auth
- [dws-group-chat](../../SKILL.md) — Product skill
