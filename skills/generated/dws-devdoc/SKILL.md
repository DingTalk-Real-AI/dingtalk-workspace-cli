---
name: dws-devdoc
description: "钉钉开放平台文档搜索支持关键词查询，返回相关文档链接及摘要，快速定位开发指南。"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws devdoc --help"
---

# devdoc

> **PREREQUISITE:** Read `../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

- Display name: 钉钉开放平台文档搜索
- Description: 钉钉开放平台文档搜索支持关键词查询，返回相关文档链接及摘要，快速定位开发指南。
- Endpoint: `https://mcp-gw.dingtalk.com/server/47ec90fc0db1e68d84fdd2280129c219873b51e81a23adf9fe7fa29ee9b579b3`
- Protocol: `2025-03-26`
- Degraded: `false`

```bash
dws devdoc <group> <command> --json '{...}'
```

## Helper Commands

| Command | Tool | Description |
|---------|------|-------------|
| [`dws-devdoc-article-search`](./article/search.md) | `search_open_platform_docs` | 根据关键词搜索钉钉开放平台的开发文档，返回匹配的文档条目列表，包含标题、摘要、文档链接和相关标签。搜索结果按相关性排序。适用于开发者在集成或调试过程中快速查找 API 说明、接入指南、错误码解释等技术资料。 |
| [`dws-devdoc-search-open-platform-error-code`](./search-open-platform-error-code.md) | `search_open_platform_error_code` | 根据错误码搜索详细说明及对应的解决方法 |

## API Tools

### `search_open_platform_docs`

- Canonical path: `devdoc.search_open_platform_docs`
- CLI route: `dws devdoc article search`
- Description: 根据关键词搜索钉钉开放平台的开发文档，返回匹配的文档条目列表，包含标题、摘要、文档链接和相关标签。搜索结果按相关性排序。适用于开发者在集成或调试过程中快速查找 API 说明、接入指南、错误码解释等技术资料。
- Required fields: `keyword`
- Sensitive: `false`

### `search_open_platform_error_code`

- Canonical path: `devdoc.search_open_platform_error_code`
- CLI route: `dws devdoc search_open_platform_error_code`
- Description: 根据错误码搜索详细说明及对应的解决方法
- Required fields: `errorCode`
- Sensitive: `false`

## Discovering Commands

```bash
dws schema                       # list available products (JSON)
dws schema devdoc                     # inspect product tools (JSON)
dws schema devdoc.<tool>              # inspect tool schema (JSON)
```
