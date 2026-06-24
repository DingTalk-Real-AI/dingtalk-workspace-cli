# 应用凭证读取

> 凭证=应用调 OpenAPI 的身份（appKey=clientId / appSecret=clientSecret）；见 SKILL.md 概念地图。

`dws dev app credentials get --unified-app-id <id>` 读取应用凭证。参数查 `dws schema dev.app.credentials.get`。

返回字段：`clientId`/`appKey`（同值）、`clientSecret`/`appSecret`（同值）、`currentSecretStatus`、`hasPendingExpireTask`、`unifiedAppId` 等。

规则：
- 该命令只需 `--unified-app-id`。
- 返回里 `clientSecret/appSecret` 是明文密钥，按敏感凭证处理，不写进回答文本。
- 不能用 `dev app get` 代替；`dev app get` 也会带密钥，同样只用于内部判断并脱敏，不向用户展开。
