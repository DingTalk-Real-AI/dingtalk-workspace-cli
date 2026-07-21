# PAT 行为授权 (pat) 命令参考

`dws pat` 管理 Agent 的行为授权。它不管理开放平台应用权限；应用权限使用 `dws dev app permission`。

## 命令总览

### 配置浏览器策略

```bash
# 允许 PAT 授权流程打开本地浏览器
dws pat browser-policy --enabled --format json

# 禁止指定 Agent 的 PAT 授权流程打开本地浏览器
dws pat browser-policy --enabled=false --agentCode <AGENT_CODE> --format json
```

`--agentCode` 省略时写入全局默认策略；该命令只修改本地策略，不授予业务操作权限。

### 预览或授予行为权限

```bash
# 远程预览按产品展开的授权计划，不写入授权
dws pat chmod --products calendar,aitable --grant-type session --session-id <SESSION_ID> --dry-run --format json

# 执行授权（高影响，必须先让用户确认；任何真实 grant 都显式加 --yes）
dws pat chmod --products calendar,aitable --grant-type session --session-id <SESSION_ID> --yes --format json

# 预览服务端可操作的全部 scope；也可增加 --product / --domain 过滤
dws pat chmod --all --dry-run --format json

# 用户确认后，授予 calendar 下服务端计划选中的全部 scope
dws pat chmod --product calendar --all --yes --format json
```

scope 格式为 `<product>.<entity>:<permission>`。`grant-type` 支持 `once`、`session`、`permanent`；`session` 模式必须提供 `--session-id`。显式 scope、产品/域选择器、`--recommend` 和 `--all` 的任何真实授权都必须显式加 `--yes`；未加时 CLI 会在任何服务调用前阻断。`--all` 使用服务端可操作的全部 scope，可与产品/域过滤器组合，但不能与位置 scope 或 `--recommend` 组合。

### 撤回一个显式授权

```bash
# 远程预览单 scope 撤回计划，不写入授权
dws pat chmod --revoke --dry-run --format json calendar.event:read

# 用户确认后执行单 scope 撤回
dws pat chmod --revoke --yes --format json calendar.event:read
```

`--revoke` 只接受一个位置 scope，不能与 `--all`、`--product`、`--products`、`--domain`、`--domains`、`--recommend`、`--grant-type` 或 `--session-id` 组合。它只撤回 ACTIVE 的显式 PAT grant，不会退出 OAuth 登录或撤销 token；服务端必须拒绝 DENIED 记录。当前命令面没有 `batch_revoke`，CLI 也不会退化为多 scope 或选择器批量撤回。

### `--dry-run` 与 `--yes` 安全语义

- PAT `chmod --dry-run` 是一次需要认证的服务端只读计划，不是本地回显；它会原样返回服务端 challenge/error。
- dry-run 不执行授权或撤回，不打开浏览器、不轮询、不登录、不刷新或重试认证，也不写入 profile、token、keychain、凭证或其他本地认证状态。
- `--dry-run --yes` 仍以 dry-run 为准，不写入任何授权；不要把 `--yes` 当成浏览器、轮询或重试开关。
- `--yes` 只表示用户已明确确认真实写操作；每个真实 grant 和单 scope revoke 都需要它。

## 意图判断

用户说"PAT 授权时允许或禁止打开浏览器/配置浏览器授权策略" → `browser-policy`
用户说"授予 Agent 行为权限/授权 scope/按产品、域、推荐集合或 --all 授权/一次性授权/会话授权/永久授权" → `chmod`
用户说"撤回一个 scope 的显式 PAT 授权" → `chmod <scope> --revoke`

## 注意事项

- `browser-policy` 只写本地配置，不会发起授权。
- `chmod` 会改变 Agent 可执行范围；所有真实授权和单 scope 撤回都是受保护写操作，必须先展示 scope、授权类型/当前状态和影响范围并获得用户确认，再加 `--yes`。
- 预览优先使用 `--dry-run --format json`；它会访问服务端生成只读计划，但不得产生授权或本地凭证写入。
- 不存在批量撤回能力；不要编造 `batch_revoke` 或把 `--revoke` 与多个 scope/集合选择器组合。
- 不要把 PAT 行为授权与 `dws dev app permission` 的开放平台应用权限混用。
