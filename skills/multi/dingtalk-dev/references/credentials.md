# 应用凭证读取

> 概念锚点：凭证=应用调 OpenAPI 的身份（appKey=clientId / appSecret=clientSecret）；见 SKILL.md 概念地图。

## 读取凭证

```bash
dws dev app credentials get --unified-app-id UNIFIED_APP_ID --format json
```

**规则：**
- CLI 只传应用定位字段，不传 `showSecret`/`confirmSecret`。
- 返回可能包含 `clientSecret/appSecret`，输出按敏感凭证处理，不写进回答文本。
- 不能用 `dev app get` 代替；如果 `dev app get` 偶尔返回密钥，也只用于内部判断并脱敏，不向用户展开。

关键返回字段：

| 字段 | 说明 |
|------|------|
| `clientId` / `appKey` | 非密钥标识 |
| `clientSecret` / `appSecret` | 敏感凭证 |
| `currentSecretStatus` | 当前密钥状态 |
| `pendingExpireTask` | 密钥过期任务信息 |
