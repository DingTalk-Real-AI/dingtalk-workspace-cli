---
name: dws-group-chat-group-members-remove
description: "钉钉群聊: 移除群成员."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws chat group members remove --help"
---

# group-chat group members remove

> **PREREQUISITE:** Read `../../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

移除群成员

## Usage

```bash
dws chat group members remove --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--id` | ✓ | — | 会话Id |
| `--users` | ✓ | — | 需要被移除的成员userId列表 |

## Required Fields

- `openconversationId`
- `userIdList`

> [!CAUTION]
> This is a **write** command — confirm with the user before executing.

## See Also

- [dws-shared](../../../dws-shared/SKILL.md) — Global rules and auth
- [dws-group-chat](../../SKILL.md) — Product skill
