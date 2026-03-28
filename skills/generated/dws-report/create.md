---
name: dws-report-create
description: "钉钉日志: 创建日志."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws report create --help"
---

# report create

> **PREREQUISITE:** Read `../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

创建日志

## Usage

```bash
dws report create --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--contents` | ✓ | — | 日志内容列表 |
| `--dd-from` | ✓ | — | 创建日志的来源，自定义值 |
| `--template-id` | ✓ | — | 需要发送哪个日志模板的日志，可通过获取可见日志模板服务获取 |
| `--to-chat` | ✓ | — | 是否发送到日志接收人的单聊 |
| `--to-user-ids` | — | — | 该日志发送到的人员userId列表 |

## Required Fields

- `contents`
- `ddFrom`
- `templateId`
- `toChat`

> [!CAUTION]
> This is a **write** command — confirm with the user before executing.

## See Also

- [dws-shared](../dws-shared/SKILL.md) — Global rules and auth
- [dws-report](./SKILL.md) — Product skill
