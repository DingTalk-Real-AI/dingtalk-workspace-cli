---
name: dws-aitable-base-create
description: "钉钉 AI 表格: 创建一个新的 AI 表格 Base。当前仅要求 baseName，服务端按默认模板创建并返回 baseId/baseName."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws aitable base create --help"
---

# aitable base create

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

创建一个新的 AI 表格 Base。当前仅要求 baseName，服务端按默认模板创建并返回 baseId/baseName

## Usage

```bash
dws aitable base create --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--name` | ✓ | — | Base 名称，1-50 字符；会去除首尾空格后校验 |
| `--template-id` | — | — | 创建 Base 模板 ID，默认创建一个空 Base。可通过 search_templates 获取模板。 |

## Required Fields

- `baseName`

> [!CAUTION]
> This is a **write** command — confirm with the user before executing.

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-aitable](../SKILL.md) — Product skill
