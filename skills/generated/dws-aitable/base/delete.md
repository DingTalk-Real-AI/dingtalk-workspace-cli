---
name: dws-aitable-base-delete
description: "钉钉 AI 表格: 删除指定 Base（高风险、不可逆）。成功后应无法通过 get_base/search_bases 读取到该 Base."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws aitable base delete --help"
---

# aitable base delete

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

删除指定 Base（高风险、不可逆）。成功后应无法通过 get_base/search_bases 读取到该 Base

## Usage

```bash
dws aitable base delete --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--base-id` | ✓ | — | 待删除 Base ID。建议先通过 get_base 确认目标 |
| `--reason` | — | — | 一句话描述删除的原因 |

## Required Fields

- `baseId`

> [!CAUTION]
> This is a **write** command — confirm with the user before executing.

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-aitable](../SKILL.md) — Product skill
