---
name: dws-aitable-base-list
description: "钉钉 AI 表格: 列出当前用户可访问的 AI 表格 Base。默认返回最近访问结果，支持分页游标续取。返回 baseId 与 baseName，后续可直接用于 get_base。
AI 表格访问地址可按 baseId 拼接为：http…"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws aitable base list --help"
---

# aitable base list

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

列出当前用户可访问的 AI 表格 Base。默认返回最近访问结果，支持分页游标续取。返回 baseId 与 baseName，后续可直接用于 get_base。
AI 表格访问地址可按 baseId 拼接为：https://docs.dingtalk.com/i/nodes/{baseId}

## Usage

```bash
dws aitable base list --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--cursor` | — | — | 首次不传；传入上次返回的游标继续获取下一页 |
| `--limit` | — | — | 每页数量，默认 10，最大 10 |

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-aitable](../SKILL.md) — Product skill
