---
name: dws-ding-message-send
description: "DING消息: 使用企业内机器人发送DING消息，可发送应用内DING、短信DING、电话DING。"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws ding message send --help"
---

# ding message send

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

使用企业内机器人发送DING消息，可发送应用内DING、短信DING、电话DING。

## Usage

```bash
dws ding message send --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--content` | ✓ | — | 消息内容 |
| `--users` | ✓ | — | 接收者用户ID列表 |
| `--type` | ✓ | — | 提醒类型，1：应用内钉钉，2：短信，3：电话 |
| `--robot-code` | ✓ | — | 机器人Code |

## Required Fields

- `content`
- `receiverUserIdList`
- `remindType`
- `robotCode`

> [!CAUTION]
> This is a **write** command — confirm with the user before executing.

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-ding](../SKILL.md) — Product skill
