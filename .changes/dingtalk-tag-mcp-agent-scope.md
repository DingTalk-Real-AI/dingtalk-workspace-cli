---
category: Fixed
---

- **数字员工 MCP 创建与员工域** — 修复 configString 被嵌套在 config 中导致工具映射丢失的问题；capability mcp create/list/query 新增必填 --agent-uuid。同步 Help、Schema、skill 和平台映射说明；需要联动部署 OpenAPI 与 MCP 配置。创建资源仍需显式保存选择并发布。
