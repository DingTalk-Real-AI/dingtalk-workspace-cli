---
name: dws-bot-group-members-add-bot
description: "机器人消息: 将自定义机器人添加到当前用户有管理权限的群聊中。如果没有权限则会报错."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws bot group members add-bot --help"
---

# bot group members add-bot

> **PREREQUISITE:** Read `../../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

将自定义机器人添加到当前用户有管理权限的群聊中。如果没有权限则会报错

## Usage

```bash
dws bot group members add-bot --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--id` | ✓ | — | 群会话Id，可通过关键词搜索群列表服务获取 |
| `--robot-code` | ✓ | — | 机器人code，可在开发者后台查看，或者调用创建机器人服务获取 |

## Required Fields

- `openConversationId`
- `robotCode`

> [!CAUTION]
> This is a **write** command — confirm with the user before executing.

## See Also

- [dws-shared](../../../dws-shared/SKILL.md) — Global rules and auth
- [dws-bot](../../SKILL.md) — Product skill
