# 开放平台应用 (devapp) 命令参考

管理钉钉开放平台企业内部应用。覆盖应用 CRUD、生命周期启停、凭证读取、权限管理、网页应用配置、成员管理和安全配置。

> `dws devapp ...` 是内置 helper 命令，不依赖 MCP 服务发现。`dws app ...` 是兼容别名。执行前用 `dws devapp --help` 验证可用。

## 核心规则

1. 所有命令加 `--format json`。
2. 写操作先 `--dry-run`，确认后才加 `--yes`。
3. 应用名/appKey/customKey 命中多条时展示候选，不取第一条。
4. 权限申请/取消只接受 `scopeValue`，不传 API 名或分组名。
5. `app get` 不读完整 secret，secret 走 `credentials get`。

## 应用定位

| 优先级 | 标识 | 处理 |
|--------|------|------|
| 1 | `--unified-app-id` | 直接使用 |
| 2 | `--agent-id` / `--app-id` | 直接使用 |
| 3 | `--app-key` / `--custom-key` | 先查询，唯一命中才继续 |
| 4 | `--name` | 模糊搜索，写操作必须唯一命中 |

---

## 一、应用基础操作

### 列表

```bash
dws devapp list --format json
dws devapp list --name DemoApp --format json
dws devapp list --agent-id 123456 --format json
```

MCP tool: `list_open_dev_apps_by_condition`

| CLI | MCP | 说明 |
|-----|-----|------|
| `--page` | `currentPage` | 1-based，默认 1 |
| `--page-size` | `pageSize` | 默认 20 |
| `--name` / `--keyword` | `appName` | 应用名搜索 |
| `--agent-id` | `agentId` | 精确定位 |
| `--app-key` | `appKey` | appKey/clientId |
| `--creator` | `creator` | 创建人关键词 |
| `--sort` | `sortType` | 如 `gmt_modified` |
| `--order` | `sortOrder` | `asc` / `desc` |

### 详情

```bash
dws devapp get --unified-app-id UNIFIED_APP_ID --format json
```

MCP tool: `get_open_dev_app_detail`

### 创建

```bash
dws devapp create --name DemoApp --desc "内部应用" --type internal --dry-run --format json
dws devapp create --name DemoApp --desc "内部应用" --type internal --yes --format json
```

MCP tool: `create_inner_app`。`--type` 只做 CLI 校验，当前仅支持 `internal`。

### 修改

```bash
dws devapp update --unified-app-id ID --name DemoApp2 --desc "新描述" --dry-run --format json
```

MCP tool: `update_inner_app`。至少一个更新字段：`--name` / `--desc` / `--icon`。

### 停用 / 启用

```bash
dws devapp inactive --unified-app-id ID --dry-run --format json
dws devapp active --unified-app-id ID --dry-run --format json
```

MCP tools: `inactive_inner_app` / `active_inner_app`

### 删除

```bash
dws devapp delete --unified-app-id ID --dry-run --format json
```

MCP tool: `delete_inner_app`。删除前必须展示应用摘要，异步生效。

---

## 二、凭证与网页应用

### 凭证读取

```bash
dws devapp credentials get --unified-app-id UNIFIED_APP_ID --format json
```

MCP tool: `get_open_dev_app_credentials`。返回含 `clientSecret/appSecret`，按敏感凭证处理。不能用 `devapp get` 代替。

### 网页应用

```bash
dws devapp webapp get --agent-id AGENT_ID --format json
dws devapp webapp config --agent-id AGENT_ID --homepage-link https://example.com --dry-run --format json
```

MCP tools: `get_webapp_config` / `set_webapp_config`

---

## 三、权限管理

### 权限列表

```bash
dws devapp permission list --unified-app-id ID --format json
dws devapp permission list --unified-app-id ID --keyword "机器人" --status UNAUTHED --format json
dws devapp permission list --unified-app-id ID --scope qyapi_robot_sendmsg --format json
```

MCP tool: `list_open_dev_app_permissions`

| CLI | MCP | 说明 |
|-----|-----|------|
| `--keyword` | `keyword` | 关键词搜索 |
| `--status` | `authStatus` | `ALL/AUTHED/UNAUTHED` |
| `--scope-type` | `firstLevelType` | `APP/SNS`，空返回两者 |
| `--scope` | `scopeValue` | 单权限详情模式 |
| `--limit` | `limit` | 每页数量，默认 20，建议不超过 50 |
| `--offset` | `offset` | 翻页偏移量，默认 0 |

翻页：`--limit 50 --offset 0`（第1页）、`--offset 50`（第2页）、`--offset 100`（第3页），返回条数 < limit 表示末尾。

### 申请权限

```bash
dws devapp permission add --unified-app-id ID --permissions qyapi_robot_sendmsg --dry-run --format json
```

MCP tool: `apply_open_dev_app_permissions`。`requiredApproval=true` 允许申请，写入版本变更。

### 取消权限

```bash
dws devapp permission remove --unified-app-id ID --permission qyapi_robot_sendmsg --dry-run --format json
```

MCP tool: `remove_open_dev_app_permission`。一次只取消一个。

---

## 四、成员与安全

### 成员管理

```bash
dws devapp member list --app-id UNIFIED_APP_ID --format json
dws devapp member add --app-id UNIFIED_APP_ID --users userId1,userId2 --member-type DEVELOPER --dry-run --format json
dws devapp member remove --app-id UNIFIED_APP_ID --users userId1 --member-type DEVELOPER --dry-run --format json
```

MCP tools: `list_open_dev_app_members` / `add_open_dev_app_members` / `remove_open_dev_app_members`

### 安全配置

```bash
dws devapp security config --app-id UNIFIED_APP_ID --ip-whitelist 192.0.2.10 --redirect-url https://callback.example.invalid/callback --dry-run --format json
```

MCP tool: `update_app_security_config`

---

## 五、操作流程

### 创建应用全流程

```text
create --dry-run → 确认 → create --yes → get 确认 → credentials get → webapp config → permission add → member add
```

### 权限管理全流程

```text
permission list → permission list --keyword → permission list --scope → permission add --dry-run → 确认 → --yes → permission list 验证
```

### 生命周期

```text
停用: get → inactive --dry-run → --yes → get 验证
启用: active --dry-run → --yes → get 验证
删除: get 展示 → delete --dry-run → 确认 → --yes → list 验证消失
```

---

## 错误处理

| 情况 | 处理 |
|------|------|
| `unknown command` | CLI 构建不含 devapp helper |
| `endpoint_not_resolved` | 检查 edition endpoint 注入 |
| 多应用命中 | 展示候选，停止写操作 |
| `ServiceResult.success=false` | 透传 `errorCode/errorMsg` |

## 待实现能力

- `dws devapp event list/config` — 事件订阅（待后端发布）
- `dws devapp version create/check-approval/publish/status` — 版本发布（待后端发布）
