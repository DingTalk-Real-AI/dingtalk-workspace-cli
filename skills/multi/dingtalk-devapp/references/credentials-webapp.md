# 凭证读取与网页应用配置

## 凭证读取

```bash
dws devapp credentials get --unified-app-id UNIFIED_APP_ID --format json
```

MCP tool: `get_dev_app_credentials`

后端 facade: `OpenInnerAppQueryFacade.getCredentials`。

**规则：**
- CLI 只传应用定位字段，不传 `showSecret`/`confirmSecret`。
- 返回可能包含 `clientSecret/appSecret`，输出按敏感凭证处理。
- 不能用 `devapp get` 代替；如果 `devapp get` 偶尔返回密钥，也只用于内部判断并脱敏，不向用户展开。

关键返回字段：

| 字段 | 说明 |
|------|------|
| `clientId` / `appKey` | 非密钥标识 |
| `clientSecret` / `appSecret` | 敏感凭证 |
| `currentSecretStatus` | 当前密钥状态 |
| `pendingExpireTask` | 密钥过期任务信息 |

## 网页应用查询

```bash
dws devapp webapp get --unified-app-id UNIFIED_APP_ID --format json
```

MCP tool: `get_extension_webapp_config`

未配置网页应用前可能只返回空配置。

## 网页应用配置

```bash
dws devapp webapp config --unified-app-id UNIFIED_APP_ID --homepage-url https://example.com/mobile --dry-run --format json
dws devapp webapp config --unified-app-id UNIFIED_APP_ID --homepage-url https://example.com/mobile --pc-homepage-url https://example.com/pc --yes --format json
```

MCP tool: `set_extension_webapp_config`

| CLI | MCP | 说明 |
|-----|-----|------|
| `--h5-page-type` | `h5PageType` | 网页应用生效端 |
| `--homepage-url` | `homepageUrl` | 移动端首页地址 |
| `--pc-homepage-url` | `pcHomepageUrl` | PC 端首页地址 |
| `--omp-url` | `ompUrl` | 管理后台地址 |

至少提供一个配置字段。`h5PageType` 未显式传入时，不要假设固定默认值；配置后以 `webapp get` 回读为准（实跑可能返回 `mobile`）。
