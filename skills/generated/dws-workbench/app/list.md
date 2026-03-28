---
name: dws-workbench-app-list
description: "钉钉工作台: 获取用户所有工作台应用."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws workbench app list --help"
---

# workbench app list

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

获取用户所有工作台应用

## Usage

```bash
dws workbench app list --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--input` | — | — | 字段名 |

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-workbench](../SKILL.md) — Product skill
