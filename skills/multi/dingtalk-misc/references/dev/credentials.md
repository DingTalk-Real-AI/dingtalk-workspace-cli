# 应用凭证读取

凭证是应用调用 OpenAPI 的身份：`appKey=clientId`，`appSecret=clientSecret`。

## 最短流程

```bash
dws dev app list --name <应用名> --format json
dws dev app credentials get --unified-app-id <唯一命中的id> --format json
```

- 已有明确 `unifiedAppId` 时只执行第二条；不要扫描本地目录、环境变量或配置文件找凭证。
- 只需要 `--unified-app-id`，不要改用 `app get` 绕路。
- 非空 `clientSecret/appSecret` 可证明凭证已生成，但其值属于敏感证据：不得在最终答复、后续命令或文件中复述。只返回实际 `data` 中存在的公开字段，如 `unifiedAppId`、`clientId/appKey`、`agentId`；CLI 不承诺额外的“凭证状态”字段，缺失时不要编造。
- 应准确表述为“本次答复未展示密钥值”或“答复中已隐藏”，不得声称 API、CLI 或既有执行日志已经脱敏；若源响应返回过明文，只做最小处理，不再打印或传递该值。
- 名称多匹配时展示候选，不读取任何候选的密钥。

已知路径直接执行；仅当前 leaf flag 确有疑问时查一次 compact Schema，Schema 不可用才查一次 leaf help。
