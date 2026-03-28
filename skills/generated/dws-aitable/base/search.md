---
name: dws-aitable-base-search
description: "钉钉 AI 表格: 按名称关键词搜索 AI 表格 Base。返回 baseId/baseName，结果按相关性排序。返回的 baseId 可直接用于 get_base 等后续工具。
AI 表格访问地址可按 baseId 拼接为：http…"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws aitable base search --help"
---

# aitable base search

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

按名称关键词搜索 AI 表格 Base。返回 baseId/baseName，结果按相关性排序。返回的 baseId 可直接用于 get_base 等后续工具。
AI 表格访问地址可按 baseId 拼接为：https://docs.dingtalk.com/i/nodes/{baseId}

## Usage

```bash
dws aitable base search --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--cursor` | — | — | 分页游标，首次不传 |
| `--query` | ✓ | — | Base 名称关键词，建议至少 2 个字符 |

## Required Fields

- `query`

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-aitable](../SKILL.md) — Product skill
