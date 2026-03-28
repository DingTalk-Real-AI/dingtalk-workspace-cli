---
name: dws-bot-batch-send-robot-msg-to-users
description: "机器人消息: 机器人批量发送单聊消息，在该机器人可使用范围内的员工，可接收到单聊消息。"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws bot batch_send_robot_msg_to_users --help"
---

# bot batch_send_robot_msg_to_users

> **PREREQUISITE:** Read `../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

机器人批量发送单聊消息，在该机器人可使用范围内的员工，可接收到单聊消息。

## Usage

```bash
dws bot batch_send_robot_msg_to_users --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--markdown` | ✓ | — | 消息内容，Markdown格式 |
| `--robotCode` | ✓ | — | 机器人Code |
| `--title` | ✓ | — | 消息标题 |
| `--userIds` | ✓ | — | 接收用户UserID列表，最多支持20个 |

## Required Fields

- `markdown`
- `robotCode`
- `title`
- `userIds`

> [!CAUTION]
> This is a **write** command — confirm with the user before executing.

## See Also

- [dws-shared](../dws-shared/SKILL.md) — Global rules and auth
- [dws-bot](./SKILL.md) — Product skill
