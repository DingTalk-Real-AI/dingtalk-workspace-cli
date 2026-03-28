---
name: dws-bot-message-recall-robot-group-message
description: "机器人消息: 可批量撤回企业机器人在群内发送的消息。"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws bot message recall_robot_group_message --help"
---

# bot message recall_robot_group_message

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

可批量撤回企业机器人在群内发送的消息。

## Usage

```bash
dws bot message recall_robot_group_message --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--group` | ✓ | — | 群ID，可通过客户端调用chooseChat接口获取 |
| `--keys` | ✓ | — | 消息Id列表，机器人发送消息服务返回的值 |
| `--robot-code` | ✓ | — | 机器人Code |

## Required Fields

- `openConversationId`
- `processQueryKeys`
- `robotCode`

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-bot](../SKILL.md) — Product skill
