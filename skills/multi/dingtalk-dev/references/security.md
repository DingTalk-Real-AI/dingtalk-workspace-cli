# 安全配置

> 概念锚点：安全配置=应用的 IP 白名单 / 登录重定向 / 端内免登 URL；见 SKILL.md 概念地图。

## 配置安全设置

```bash
dws dev app security config --unified-app-id UNIFIED_APP_ID --ip-whitelist 192.0.2.10 --dry-run --format json
dws dev app security config --unified-app-id UNIFIED_APP_ID --redirect-url https://callback.example.invalid/callback --sso-url https://sso.example.invalid/sso --yes --format json
```

| CLI | 说明 |
|-----|------|
| `--unified-app-id` | 统一应用 ID（必填） |
| `--ip-whitelist` | 出口 IP 白名单，逗号/分号分隔 |
| `--redirect-url` | 登录重定向 URL，逗号/分号分隔 |
| `--sso-url` | 端内免登 URL，逗号/分号分隔 |

至少提供一个配置字段。

覆盖语义：未提供的字段不动；显式提供的列表是**整组覆盖**（传入即全量替换该项，不是追加）——要保留旧值就把旧值一起带上。
