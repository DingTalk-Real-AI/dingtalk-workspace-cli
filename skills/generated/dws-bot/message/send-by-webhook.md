---
name: dws-bot-message-send-by-webhook
description: "机器人消息: 使用自定义机器人发送群消息，请注意自定义机器人与企业机器人的区别。"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws bot message send-by-webhook --help"
---

# bot message send-by-webhook

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

使用自定义机器人发送群消息，请注意自定义机器人与企业机器人的区别。

## Usage

```bash
dws bot message send-by-webhook --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--at-mobiles` | — | — | 被@的群成员手机号 |
| `--at-users` | — | — | 被@的群成员userId |
| `--at-all` | — | — | 是否@所有人 |
| `--token` | ✓ | — | 自定义机器人Token，在创建自定义机器人时得到的webhook地址中的accessToken值 |
| `--text` | ✓ | — | 消息内容,Markdown格式 |
| `--title` | ✓ | — | 消息标题 |

## Required Fields

- `robotToken`
- `text`
- `title`

> [!CAUTION]
> This is a **write** command — confirm with the user before executing.

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-bot](../SKILL.md) — Product skill
