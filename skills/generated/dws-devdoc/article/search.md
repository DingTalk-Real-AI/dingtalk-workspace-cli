---
name: dws-devdoc-article-search
description: "钉钉开放平台文档搜索: 根据关键词搜索钉钉开放平台的开发文档，返回匹配的文档条目列表，包含标题、摘要、文档链接和相关标签。搜索结果按相关性排序。适用于开发者在集成或调试过程中快速查找 API 说明、接入指南、错误码解释等技术资料。"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws devdoc article search --help"
---

# devdoc article search

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

根据关键词搜索钉钉开放平台的开发文档，返回匹配的文档条目列表，包含标题、摘要、文档链接和相关标签。搜索结果按相关性排序。适用于开发者在集成或调试过程中快速查找 API 说明、接入指南、错误码解释等技术资料。

## Usage

```bash
dws devdoc article search --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--keyword` | ✓ | — | 搜索关键词 |
| `--page` | — | — | 分页页码 |
| `--size` | — | — | 分页大小 |

## Required Fields

- `keyword`

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-devdoc](../SKILL.md) — Product skill
