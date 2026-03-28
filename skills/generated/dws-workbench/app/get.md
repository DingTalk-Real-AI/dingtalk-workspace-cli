---
name: dws-workbench-app-get
description: "钉钉工作台: 根据应用id批量拉取应用详情."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws workbench app get --help"
---

# workbench app get

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

根据应用id批量拉取应用详情

## Usage

```bash
dws workbench app get --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--ids` | ✓ | — | 应用id列表 |

## Required Fields

- `appIds`

> [!CAUTION]
> This is a **write** command — confirm with the user before executing.

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-workbench](../SKILL.md) — Product skill
