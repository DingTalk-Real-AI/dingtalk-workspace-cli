# 成员管理与安全配置

## 成员查询

```bash
dws devapp member list --unified-app-id UNIFIED_APP_ID --format json
```

MCP tool: `list_dev_app_members`

## 添加成员

```bash
dws devapp member add --unified-app-id UNIFIED_APP_ID --users userId1,userId2 --member-type DEVELOPER --dry-run --format json
dws devapp member add --unified-app-id UNIFIED_APP_ID --users userId1,userId2 --member-type DEVELOPER --yes --format json
```

MCP tool: `add_dev_app_members`

## 移除成员

```bash
dws devapp member remove --unified-app-id UNIFIED_APP_ID --users userId1 --member-type DEVELOPER --dry-run --format json
dws devapp member remove --unified-app-id UNIFIED_APP_ID --users userId1 --member-type DEVELOPER --yes --format json
```

MCP tool: `remove_dev_app_members`

| CLI | MCP | 说明 |
|-----|-----|------|
| `--unified-app-id` | `unifiedAppId` | 统一应用 ID（必填） |
| `--users` | `memberUserIds` | userId 列表，逗号分隔（必填） |
| `--member-type` | `memberType` | 成员类型，如 `DEVELOPER`（必填） |

## 安全配置

```bash
dws devapp security config --unified-app-id UNIFIED_APP_ID --ip-whitelist 192.0.2.10 --dry-run --format json
dws devapp security config --unified-app-id UNIFIED_APP_ID --redirect-url https://callback.example.invalid/callback --sso-url https://sso.example.invalid/sso --yes --format json
```

MCP tool: `update_dev_app_security_config`

| CLI | MCP | 说明 |
|-----|-----|------|
| `--unified-app-id` | `unifiedAppId` | 统一应用 ID（必填） |
| `--ip-whitelist` | `ipWhitelist` | 出口 IP 白名单，逗号/分号分隔 |
| `--redirect-url` | `redirectUrls` | 登录重定向 URL，逗号/分号分隔 |
| `--sso-url` | `ssoUrls` | 端内免登 URL，逗号/分号分隔 |

至少提供一个配置字段。只下发显式提供的字段，未提供的不覆盖。
