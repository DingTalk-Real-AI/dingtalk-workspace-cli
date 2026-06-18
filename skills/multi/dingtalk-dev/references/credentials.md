# 应用凭证读取

> 凭证=应用调 OpenAPI 的身份（appKey=clientId / appSecret=clientSecret）；见 SKILL.md 概念地图。

`dws dev app credentials get --unified-app-id <id>` 读取应用凭证。参数查 `dws schema dev.app.credentials.get`。

规则：
- CLI 只传应用定位字段，不传 `showSecret`/`confirmSecret`。
- 返回可能含 `clientSecret/appSecret`，按敏感凭证处理，不写进回答文本。
- 不能用 `dev app get` 代替；`dev app get` 偶尔返回密钥也只用于内部判断并脱敏，不向用户展开。
