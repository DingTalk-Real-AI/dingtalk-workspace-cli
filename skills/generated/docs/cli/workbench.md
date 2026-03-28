# Canonical Product: workbench

Generated from shared Tool IR. Do not edit by hand.

- Display name: 钉钉工作台
- Description: 钉钉工作台MCP支持查询用户所有应用及批量获取应用详情，助力快速了解和管理办公应用。
- Server key: `62a9cf4de3d881c9`
- Endpoint: `https://mcp-gw.dingtalk.com/server/59dbb4c38c1febc44648a3bfb4409d8a347ffb1af38fde73b30ba8e38fdcdf46`
- Protocol: `2025-03-26`
- Degraded: `false`

## Tools

- `app get`
  - Path: `workbench.batch_get_app_details`
  - CLI route: `dws workbench app get`
  - Description: 根据应用id批量拉取应用详情
  - Flags: `--ids`
  - Schema: `skills/generated/docs/schema/workbench/batch_get_app_details.json`
- `app list`
  - Path: `workbench.get_user_workspace_apps`
  - CLI route: `dws workbench app list`
  - Description: 获取用户所有工作台应用
  - Flags: `--input`
  - Schema: `skills/generated/docs/schema/workbench/get_user_workspace_apps.json`
