---
name: dws-aitable-base-update
description: "钉钉 AI 表格: 更新 Base 名称（可选备注）。当前不支持修改主题、封面等扩展属性."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws aitable base update --help"
---

# aitable base update

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

更新 Base 名称（可选备注）。当前不支持修改主题、封面等扩展属性

## Usage

```bash
dws aitable base update --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--base-id` | ✓ | — | 目标 Base ID |
| `--desc` | — | — | 备注文本 |
| `--name` | ✓ | — | 新名称，1-50 字符 |

## Required Fields

- `baseId`
- `newBaseName`

> [!CAUTION]
> This is a **write** command — confirm with the user before executing.

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-aitable](../SKILL.md) — Product skill
