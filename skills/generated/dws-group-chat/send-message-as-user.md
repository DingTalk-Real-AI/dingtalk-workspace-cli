---
name: dws-group-chat-send-message-as-user
description: "钉钉群聊: 以当前用户的身份发送群消息."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws chat send_message_as_user --help"
---

# group-chat send_message_as_user

> **PREREQUISITE:** Read `../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

以当前用户的身份发送群消息

## Usage

```bash
dws chat send_message_as_user --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--atAll` | — | — | 是否@所有人 |
| `--atUserIds` | — | — | 需要@的userId列表 |
| `--clawType` | — | — | clawType |
| `--openConversation-id` | ✓ | — | 目标会话ID |
| `--text` | ✓ | — | 消息正文，markdown格式 |
| `--title` | ✓ | — | 消息标题，标题内容显示在消息列表 |

## Required Fields

- `openConversation_id`
- `text`
- `title`

> [!CAUTION]
> This is a **write** command — confirm with the user before executing.

## See Also

- [dws-shared](../dws-shared/SKILL.md) — Global rules and auth
- [dws-group-chat](./SKILL.md) — Product skill
