---
name: dws-devdoc-search-open-platform-error-code
description: "钉钉开放平台文档搜索: 根据错误码搜索详细说明及对应的解决方法."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws devdoc search_open_platform_error_code --help"
---

# devdoc search_open_platform_error_code

> **PREREQUISITE:** Read `../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

根据错误码搜索详细说明及对应的解决方法

## Usage

```bash
dws devdoc search_open_platform_error_code --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--errorCode` | ✓ | — | 错误码 |

## Required Fields

- `errorCode`

## See Also

- [dws-shared](../dws-shared/SKILL.md) — Global rules and auth
- [dws-devdoc](./SKILL.md) — Product skill
