---
name: dws-group-chat-message-list-topic-replies
description: "钉钉群聊: 针对话题群中的单个话题，分页拉取话题的回复消息."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws chat message list-topic-replies --help"
---

# group-chat message list-topic-replies

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

针对话题群中的单个话题，分页拉取话题的回复消息

## Usage

```bash
dws chat message list-topic-replies --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--forward` | — | — | true是从老往新，false是新往老拉 |
| `--group` | ✓ | — | 群会话唯一标识 |
| `--limit` | — | — | 返回数量 |
| `--time` | — | — | 格式：yyyy-MM-dd HH:mm:ss，非必填 |
| `--topic-id` | ✓ | — | 由 dws chat message list 指令 返回 |

## Required Fields

- `openconversationId`
- `topicId`

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-group-chat](../SKILL.md) — Product skill
