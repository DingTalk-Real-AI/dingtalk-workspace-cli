---
category: Fixed
---

- **Direct-runtime MCP endpoint resolution** (#1331) — restore the
  environment-aware `mcpdev` endpoint so `dws dev mcp` commands reach the
  backend instead of failing locally with `endpoint_not_resolved`.
