# MCP 鉴权与凭证

鉴权配置是“如何取值和注入”的说明书，凭证账号保存真实密钥。二者必须分离，密钥不得放进工具定义、回答、日志、文档或代码仓库。

## 类型选择

| 下游要求 | `auth-type` | 配置方式 |
|---|---|---|
| 无鉴权 | `NO_AUTH` | 不保存密钥 |
| HTTP Basic | `BASIC` | `--basic-auth-config` |
| 静态 API key | `SIGNATURE` | 自定义 `authFields`，通过 `authQuery` 或 `authHeaders` 直引 |
| 动态 access token | `TOKEN` | 配置换 token 请求、注入位置、失效规则和刷新策略 |
| 自定义摘要或签名 | `SIGNATURE` | 自定义字段和表达式 |

不确定鉴权方式时先询问用户，不要默认 NO_AUTH。

## 安全执行顺序

带鉴权服务按以下顺序执行：

1. `auth save` 保存鉴权说明书。
2. `credential save --content-file` 保存实际凭证。
3. `credential debug` 验证下游鉴权。
4. `credential bind` 选择发布实例使用的凭证。
5. 创建并调试工具；工具 debug 仍须显式传 `--credential-id`。
6. 发布工具。

凭证应在发布前绑定。已发布后才绑定但实例仍停留在草稿态时，不要循环重试取址，按 [troubleshooting.md](troubleshooting.md) 核对实例状态。

所有写命令先 `--dry-run --format json`，展示脱敏预览并获得用户明确确认后，将同一命令仅改为 `--yes`。

## 静态 API Key

query 参数示例：

```json
{
  "authFields": [
    {"dataId":"apiKey","description":"API Key","type":"password","required":true}
  ],
  "authQuery": [
    {"key":"api_key","type":"authField","value":"#(\"apiKey\")"}
  ],
  "testRequest": {"method":"GET","url":"https://api.example.com/health"}
}
```

```bash
dws dev mcp auth save --mcp-id <mcpId> --auth-type SIGNATURE \
  --signature-auth-config '<上方JSON>' --dry-run --format json
```

API key 放 header 时使用 `authHeaders`，`key` 改为真实 header 名。凭证文件只含 dataId 对应值，例如 `{"apiKey":"<secret>"}`。

## 动态 Token

```json
{
  "authFields": [
    {"dataId":"appKey","type":"string","required":true},
    {"dataId":"appSecret","type":"password","required":true}
  ],
  "fetchTokenRequest": {
    "method":"POST",
    "url":"https://api.example.com/token",
    "body":[
      {"key":"app_key","type":"authField","value":"#(\"appKey\")"},
      {"key":"app_secret","type":"authField","value":"#(\"appSecret\")"}
    ]
  },
  "authHeaders": [
    {"key":"Authorization","type":"text","value":"$.Body.access_token"}
  ],
  "tokenExpireRules": [
    {"func":"EQ(${@(\"Body/errorCode\")},'TOKEN_EXPIRED')"}
  ],
  "refreshToken": true,
  "testRequest": {"method":"GET","url":"https://api.example.com/health"}
}
```

按真实下游协议选择 `authHeaders`、`authQuery` 或 `authBody`，不能照抄占位字段。换 token 响应引用使用 `$.Body.<字段>`；若下游值需要 `Bearer ` 前缀，按平台当前支持的表达式显式拼接并通过 credential debug 验证，不能假设 text 引用会自动补前缀。失效规则可基于 HTTP status 或业务错误码。

## 凭证命令

```bash
dws dev mcp credential list --mcp-id <mcpId> --format json
dws dev mcp credential save --mcp-id <mcpId> --name <账号名> \
  --content-file <local-json-file> --dry-run --format json
dws dev mcp credential debug --mcp-id <mcpId> --credential-id <id> \
  --dry-run --format json
dws dev mcp credential bind --mcp-id <mcpId> --credential-id <id> \
  --dry-run --format json
dws dev mcp credential unbind --mcp-id <mcpId> --dry-run --format json
```

优先使用 `--content-file`；需要 stdin 时精确传 `--content-file -`。不要把真实 JSON 密钥放入 shell history。

`credential debug` 会真实访问下游。外层流程成功不等于鉴权通过，必须检查响应状态和业务错误。`credential bind` 只影响正式实例；`tool debug` 不使用绑定凭证，必须在本次调试显式指定 `--credential-id`，无鉴权工具显式指定 `--no-credential`。

删除已绑定凭证前先 unbind。任何删除均须回读对象、dry-run 并取得明确确认。
