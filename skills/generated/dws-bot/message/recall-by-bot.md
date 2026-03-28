---
name: dws-bot-message-recall-by-bot
description: "机器人消息: 批量撤回机器人发送的单聊消息。"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws bot message recall-by-bot --help"
---

# bot message recall-by-bot

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

批量撤回机器人发送的单聊消息。

## Usage

```bash
dws bot message recall-by-bot --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--keys` | ✓ | — | 消息Id列表，机器人发送单聊消息时返回的值 |
| `--robot-code` | ✓ | — | 机器人robotCode |

## Required Fields

- `processQueryKeys`
- `robotCode`

> [!CAUTION]
> This is a **write** command — confirm with the user before executing.

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-bot](../SKILL.md) — Product skill
