# 应用基础操作

应用列表查询、详情、创建、修改、生命周期启停和删除。

## 应用定位

写操作前必须定位到唯一应用：

| 优先级 | 标识 | 处理 |
|--------|------|------|
| 1 | `--unified-app-id` | 直接使用 |
| 2 | `--agent-id` / `--app-id` | 直接使用 |
| 3 | `--app-key` / `--custom-key` | 先查询，唯一命中才继续 |
| 4 | `--name` | 模糊搜索，写操作必须唯一命中 |

## 应用列表

```bash
dws devapp list --format json
dws devapp list --name DemoApp --format json
dws devapp list --agent-id 123456 --format json
dws devapp list --creator 张三 --sort gmt_modified --order desc --format json
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

## 应用状态字段

列表/详情原始字段里如果出现 `status` / `appStatus`，按应用生命周期枚举判断；不要和版本 `versionStatus` 混用。

| status | 枚举 | 含义 | 下一步 |
|--------|------|------|--------|
| `0` | `IN_ACTIVE` | 已停用，应用不可用 | 需要恢复时走 `active --dry-run` → 确认 → `--yes` |
| `1` | `ACTIVE` | 已激活，应用可用 | 可继续配置权限、网页应用、机器人或版本 |
| `2` | `WAIT_ACTIVE` | 待激活 | 先回读 `get/list` 确认状态；不要直接按已生效处理 |
| `3` | `EXPIRED` | 已过期 | 停止写操作，提示用户到开发者后台或管理员侧处理 |

`create/update` 返回的 `versionStatus` 是版本状态，语义见 `version.md`；它不等同于应用启停状态。

## 应用详情

```bash
dws devapp get --unified-app-id UNIFIED_APP_ID --format json
dws devapp get --agent-id 123456 --format json
```

MCP tool: `get_open_dev_app_detail`

详情主要用于定位和核验应用。若上游偶尔随详情返回 `clientSecret/appSecret`，必须脱敏处理，不要复制到回答里；主动读取凭证仍走 `credentials get`。

## 创建应用

```bash
dws devapp create --name DemoApp --desc "内部应用" --type internal --dry-run --format json
dws devapp create --name DemoApp --desc "内部应用" --type internal --yes --format json
```

MCP tool: `create_inner_app`

| CLI | MCP | 必填 |
|-----|-----|------|
| `--name` | `appName` | 是 |
| `--desc` | `appDesc` | 否 |
| `--icon` | `appIcon` | 否 |

`--type` 只做 CLI 校验（当前仅支持 `internal`），不下发 MCP。

## 修改应用

```bash
dws devapp update --unified-app-id ID --name DemoApp2 --desc "新描述" --dry-run --format json
dws devapp update --unified-app-id ID --name DemoApp2 --desc "新描述" --yes --format json
```

MCP tool: `update_inner_app`

至少提供一个更新字段：`--name` / `--desc` / `--icon`。

## 停用 / 启用应用

```bash
dws devapp inactive --unified-app-id ID --dry-run --format json
dws devapp inactive --unified-app-id ID --yes --format json

dws devapp active --unified-app-id ID --dry-run --format json
dws devapp active --unified-app-id ID --yes --format json
```

MCP tools: `inactive_inner_app` / `active_inner_app`

停用保留数据但应用不可用，可通过 `active` 恢复。

执行 `inactive/active` 后必须回读 `get` 或 `list`：看到 `status=0` 才算停用完成，看到 `status=1` 才算启用完成；如果接口只返回操作成功但未带状态，向用户说明需要以回读结果为准。

## 删除应用

```bash
dws devapp delete --unified-app-id ID --dry-run --format json
dws devapp delete --unified-app-id ID --yes --format json
```

MCP tool: `delete_inner_app`

删除前必须展示应用摘要。删除为异步操作，成功后应用延迟从列表消失。

## 错误处理

| 情况 | 处理 |
|------|------|
| `unknown command` | CLI 构建不含 devapp helper |
| `endpoint_not_resolved` | 检查 edition endpoint 注入 |
| 多应用命中 | 展示候选，停止写操作 |
| `ServiceResult.success=false` | 透传 `errorCode/errorMsg` |
