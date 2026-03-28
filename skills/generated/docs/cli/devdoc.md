# Canonical Product: devdoc

Generated from shared Tool IR. Do not edit by hand.

- Display name: 钉钉开放平台文档搜索
- Description: 钉钉开放平台文档搜索支持关键词查询，返回相关文档链接及摘要，快速定位开发指南。
- Server key: `c1110038088c6134`
- Endpoint: `https://mcp-gw.dingtalk.com/server/47ec90fc0db1e68d84fdd2280129c219873b51e81a23adf9fe7fa29ee9b579b3`
- Protocol: `2025-03-26`
- Degraded: `false`

## Tools

- `article search`
  - Path: `devdoc.search_open_platform_docs`
  - CLI route: `dws devdoc article search`
  - Description: 根据关键词搜索钉钉开放平台的开发文档，返回匹配的文档条目列表，包含标题、摘要、文档链接和相关标签。搜索结果按相关性排序。适用于开发者在集成或调试过程中快速查找 API 说明、接入指南、错误码解释等技术资料。
  - Flags: `--keyword`, `--page`, `--size`
  - Schema: `skills/generated/docs/schema/devdoc/search_open_platform_docs.json`
- `search_open_platform_error_code`
  - Path: `devdoc.search_open_platform_error_code`
  - CLI route: `dws devdoc search_open_platform_error_code`
  - Description: 根据错误码搜索详细说明及对应的解决方法
  - Flags: `--errorCode`
  - Schema: `skills/generated/docs/schema/devdoc/search_open_platform_error_code.json`
