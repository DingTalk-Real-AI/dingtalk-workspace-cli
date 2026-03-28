---
name: dws-group-chat-list-conversation-message
description: "钉钉群聊: 已废弃！！！！拉取指定单聊或群聊的会话消息内容."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws chat list_conversation_message --help"
---

# group-chat list_conversation_message

> **PREREQUISITE:** Read `../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

已废弃！！！！拉取指定单聊或群聊的会话消息内容

## Usage

```bash
dws chat list_conversation_message --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--endTime` | — | — | 结束时间，格式：yyyy-MM-dd HH:mm:ss，非必填 |
| `--openconversation-id` | ✓ | — | 会话Id |
| `--startTime` | — | — | 开始时间，格式：yyyy-MM-dd HH:mm:ss，非必填 |

## Required Fields

- `openconversation_id`

## See Also

- [dws-shared](../dws-shared/SKILL.md) — Global rules and auth
- [dws-group-chat](./SKILL.md) — Product skill
