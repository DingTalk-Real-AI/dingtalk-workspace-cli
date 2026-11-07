---
category: Added
---

- 新增 `dws mcp market search`：按名称关键词搜索钉钉 MCP 市场，返回 `mcpId`、名称、描述、服务名、提供方、是否官方和是否收费，可直接衔接 `dws mcp published tools <mcpId>` 与 `dws mcp url get <mcpId>`。支持 `--limit` 单页数量与 `--cursor` 续页；只覆盖已上架市场的 MCP，企业内发布但未上架市场的 MCP 不在结果中。
