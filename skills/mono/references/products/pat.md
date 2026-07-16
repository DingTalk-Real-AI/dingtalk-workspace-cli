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

### 授予行为权限

```bash
# 预览按产品展开的批量授权计划，不写入授权
dws pat chmod --products calendar,aitable --grant-type session --session-id <SESSION_ID> --dry-run --format json

# 执行批量行为授权（高影响，必须先让用户确认）
dws pat chmod --products calendar,aitable --grant-type session --session-id <SESSION_ID> --yes --format json
```

scope 格式为 `<product>.<entity>:<permission>`。`grant-type` 支持 `once`、`session`、`permanent`；`session` 模式必须提供 `--session-id`。使用 `--products`、`--product`、`--domains`、`--domain` 或 `--recommend` 批量展开 scope 时，先用 `--dry-run` 检查计划，用户明确确认后才可加 `--yes`。

## 意图判断

用户说"PAT 授权时允许或禁止打开浏览器/配置浏览器授权策略" → `browser-policy`
用户说"授予 Agent 行为权限/授权 scope/批量授权产品/一次性授权/会话授权/永久授权" → `chmod`

## 注意事项

- `browser-policy` 只写本地配置，不会发起授权。
- `chmod` 会改变 Agent 可执行范围；批量或永久授权属于高影响写操作，必须先展示 scope、授权类型和有效期并获得用户确认。
- 不要把 PAT 行为授权与 `dws dev app permission` 的开放平台应用权限混用。
