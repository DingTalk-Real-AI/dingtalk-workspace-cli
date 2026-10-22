# OIDC Provider 通过 edition Hook 注册

状态：核心 Hook 已实现；dws-ant 的真实 SDK 适配和注册留待后续接入。

`auth provider-login <provider>` 从 `edition.Hooks.OIDCProviders` 选择授权实现。
必填位置参数 `<provider>` 同时传给 transactions 和 exchange 接口。换票请求包含
`provider`、`code`、`state`；显式指定 DWS 应用凭据时再加入 `client_id`、`client_secret`。
核心通过注册项选择授权实现，具体 SDK 由 overlay 提供。
服务地址使用系统统一的 MCP 配置（`mcp_url` 文件或默认地址），登录不修改端点配置。

## Overlay 接入

公共类型位于 `pkg/edition`，overlay 不需要也不得导入核心的 `internal/auth`。
在已有 `NewHooks()` 中注册名称及授权回调，再沿用启动入口的 `edition.Override(h)`：

```go
h.OIDCProviders = func() []edition.OIDCProvider {
    return []edition.OIDCProvider{
        {
            Name:      "ant",
            Authorize: authorizeWithAntSDK,
        },
    }
}
```

`authorizeWithAntSDK` 的函数签名为：

```go
func(context.Context, edition.OIDCAuthorizationRequest) (edition.OIDCAuthorizationResult, error)
```

蚂蚁适配负责调用自己的 SDK，并把 SDK 实际返回的 `code/state` 转为公共结果类型。
若已有 `AuthorizeOIDC` 使用业务侧 DTO，只在这个回调中转换类型；不用复制核心事务、
换票或保存凭证代码。SDK 初始化与认证发生在回调执行时，注册函数只返回声明。
名称可由 overlay 自定义，但必须满足 `[a-z][a-z0-9_-]{0,63}`，并与 Portal 已配置的
provider 一致。重复名称、空回调、非法名称均为注册错误。

## 执行边界

1. 核心校验 CLI 参数并选择已注册的回调。
2. 核心向 Portal 创建事务，校验服务端 provider、授权参数与有效期。
3. 核心向回调传入 `authorization_endpoint`、独立的参数 map 及受事务期限约束的 context。
4. SDK 尊重取消与超时，原样使用服务端授权参数并返回实际取得的 `code/state`。
5. 核心验证授权码及原事务 state，再换票、补全身份、保存 DWS profile。

SDK 会话、密码、DWS 换票用的 client secret 和 PKCE verifier 不属于这个回调的输入输出。
错误中不得包含认证材料；核心也不记录 SDK 原始错误文本。授权失败、取消、超时或
state 不匹配不会触发其他 SDK 或 Mock 兜底，也不会换票或保存凭证。

没有注册对应实现时，核心仍先检查 Portal 配置；服务端接受 provider 后返回
`OIDC_SDK_UNAVAILABLE`。Help 和 dry-run 不枚举注册项、不初始化 SDK、不访问网络。
普通 OAuth/PAT 登录和分步 `exchange-prepare` / `token-exchange` 入口保持独立。

## 联调与依赖更新

核心的 Hook API 提交并推送后，overlay 需要把远程核心依赖更新到包含该 API 的提交，
再编译上述注册代码。只修改另一 Go 包里的同名文件不会替换核心实现。

回归验证覆盖自定义名称和 ant 注册、真实命令入口及凭证保存、非法/重复/缺失注册、
后端拒绝、SDK 错误脱敏、取消/超时、state 篡改、空授权码和 help/dry-run 无副作用。

```bash
DWS_PACKAGE_VERSION=0.0.0-test CGO_ENABLED=0 go test ./pkg/edition ./internal/auth ./internal/app
```

已验证：上述三个包的完整测试、Hook 定向 race 测试、`go vet ./...`、`make build`，
以及构建后二进制的 provider-login help / dry-run。未执行真实蚂蚁网关登录。
