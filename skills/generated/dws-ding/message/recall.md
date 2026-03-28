---
name: dws-ding-message-recall
description: "DING消息: 撤回已发送的DING消息."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws ding message recall --help"
---

# ding message recall

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

撤回已发送的DING消息

## Usage

```bash
dws ding message recall --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--id` | ✓ | — | 要撤回的钉钉消息ID，可通过发送DING消息接口获取 |
| `--robot-code` | ✓ | — | 发送钉钉消息的机器人ID，必须与发送消息的机器人为同一个 |

## Required Fields

- `openDingId`
- `robotCode`

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-ding](../SKILL.md) — Product skill
