---
name: dws-bot-message-send-by-bot
description: "机器人消息: 机器人发送群聊消息，该机器人必须已存在对应的群内。"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws bot message send-by-bot --help"
---

# bot message send-by-bot

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

机器人发送群聊消息，该机器人必须已存在对应的群内。

## Usage

```bash
dws bot message send-by-bot --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--text` | ✓ | — | 消息内容，Markdown格式 |
| `--group` | ✓ | — | 群聊会话ID |
| `--robot-code` | ✓ | — | 机器人Code |
| `--title` | ✓ | — | 消息标题 |

## Required Fields

- `markdown`
- `openConversationId`
- `robotCode`
- `title`

> [!CAUTION]
> This is a **write** command — confirm with the user before executing.

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-bot](../SKILL.md) — Product skill
