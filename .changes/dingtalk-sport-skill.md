---
category: Added
---

- **钉钉运动 Skill** — 新增 `dingtalk-sport` multi Skill，封装钉钉运动官方 MCP（mcpId=10816）的 22 个工具：企业步数榜、部门人均排行、部门内员工排行、员工与部门多日步数明细、企业健康概况，以及运动比赛的创建、配置与分期查询、参与/达标统计、逐日趋势和开赛群播报。调用方先读取 `mcp-config.json` 确认服务配置；随附的 `scripts/call_mcp.py` 接受 URL 参数或 `$DINGTALK_SPORT_HEALTH_URL` 环境变量引用直连 MCP，脚本本身不读取该 JSON，不新增 `dws` 命令。
