---
category: Fixed
---

- **数字员工 MCP 创建与员工域** — 修复 configString 被嵌套在 config 中导致工具映射丢失的问题；capability mcp create/list/query 新增必填 --agent-uuid。同步 Help、Schema、skill 和平台映射说明；需要联动部署 OpenAPI 与 MCP 配置。MCP 创建成功后自动追加到员工草稿并保留已有选择，仍需显式发布。
