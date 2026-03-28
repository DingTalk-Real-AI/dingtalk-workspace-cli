---
name: dws-workbench
description: "钉钉工作台MCP支持查询用户所有应用及批量获取应用详情，助力快速了解和管理办公应用。"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws workbench --help"
---

# workbench

> **PREREQUISITE:** Read `../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

- Display name: 钉钉工作台
- Description: 钉钉工作台MCP支持查询用户所有应用及批量获取应用详情，助力快速了解和管理办公应用。
- Endpoint: `https://mcp-gw.dingtalk.com/server/59dbb4c38c1febc44648a3bfb4409d8a347ffb1af38fde73b30ba8e38fdcdf46`
- Protocol: `2025-03-26`
- Degraded: `false`

```bash
dws workbench <group> <command> --json '{...}'
```

## Helper Commands

| Command | Tool | Description |
|---------|------|-------------|
| [`dws-workbench-app-get`](./app/get.md) | `batch_get_app_details` | 根据应用id批量拉取应用详情 |
| [`dws-workbench-app-list`](./app/list.md) | `get_user_workspace_apps` | 获取用户所有工作台应用 |

## API Tools

### `batch_get_app_details`

- Canonical path: `workbench.batch_get_app_details`
- CLI route: `dws workbench app get`
- Description: 根据应用id批量拉取应用详情
- Required fields: `appIds`
- Sensitive: `false`

### `get_user_workspace_apps`

- Canonical path: `workbench.get_user_workspace_apps`
- CLI route: `dws workbench app list`
- Description: 获取用户所有工作台应用
- Required fields: none
- Sensitive: `false`

## Discovering Commands

```bash
dws schema                       # list available products (JSON)
dws schema workbench                     # inspect product tools (JSON)
dws schema workbench.<tool>              # inspect tool schema (JSON)
```
