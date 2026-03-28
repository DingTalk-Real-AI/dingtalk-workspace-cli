---
name: dws-group-chat-list-individual-chat-message
description: "钉钉群聊: 拉取指定用户的单聊会话消息内容."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws chat list_individual_chat_message --help"
---

# group-chat list_individual_chat_message

> **PREREQUISITE:** Read `../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

拉取指定用户的单聊会话消息内容

## Usage

```bash
dws chat list_individual_chat_message --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--forward` | ✓ | — | 方向，true是从老往新，false是新往老拉 |
| `--limit` | — | — | 返回数量 |
| `--time` | ✓ | — | 开始时间，格式：yyyy-MM-dd HH:mm:ss，非必填 |
| `--userId` | ✓ | — | 单聊用户ID |

## Required Fields

- `forward`
- `time`
- `userId`

## See Also

- [dws-shared](../dws-shared/SKILL.md) — Global rules and auth
- [dws-group-chat](./SKILL.md) — Product skill
