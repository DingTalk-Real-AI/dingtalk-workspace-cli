# 网页应用配置

> 概念锚点：网页应用=应用的能力扩展之一，钉钉内打开的 H5；见 SKILL.md 概念地图。

## 查询网页应用配置

```bash
dws dev app webapp get --unified-app-id UNIFIED_APP_ID --format json
```

未配置网页应用前可能只返回 `agentId`。

## 配置网页应用

```bash
dws dev app webapp config --unified-app-id UNIFIED_APP_ID --homepage-url https://example.com/mobile --dry-run --format json
dws dev app webapp config --unified-app-id UNIFIED_APP_ID --homepage-url https://example.com/mobile --pc-homepage-url https://example.com/pc --yes --format json
```

| CLI | 说明 |
|-----|------|
| `--h5-page-type` | 网页应用生效端 |
| `--homepage-url` | 移动端首页地址 |
| `--pc-homepage-url` | PC 端首页地址 |
| `--omp-url` | 管理后台地址 |

至少提供一个配置字段。`h5PageType` 未显式传入时不要假设固定默认值；配置后以 `webapp get` 回读为准（实跑可能返回 `mobile`）。
