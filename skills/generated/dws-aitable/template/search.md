---
name: dws-aitable-template-search
description: "钉钉 AI 表格: 按名称关键词搜索 AI 表格模板，支持分页。
返回每个模板的 templateId、name、description，以及分页信息 hasMore / nextCursor。
返回的 templateId 可直接用…"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws aitable template search --help"
---

# aitable template search

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

按名称关键词搜索 AI 表格模板，支持分页。
返回每个模板的 templateId、name、description，以及分页信息 hasMore / nextCursor。
返回的 templateId 可直接用于 create_base。
模板预览链接可通过 https://docs.dingtalk.com/table/template/{templateId} 拼接得到

## Usage

```bash
dws aitable template search --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--cursor` | — | — | 分页游标。首次请求不传；后续请原样传入上次返回的 nextCursor |
| `--limit` | — | — | 每页返回数量。默认 10，最大 30 |
| `--query` | ✓ | — | 模板名称关键词 |

## Required Fields

- `query`

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-aitable](../SKILL.md) — Product skill
