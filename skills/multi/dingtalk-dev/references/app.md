# 应用基础操作

> 概念锚点：操作的是「应用」容器本体（见 SKILL.md 概念地图）；启停/删除改的是应用 appStatus，不是版本 versionStatus。

应用列表查询、详情、创建、修改、生命周期启停和删除。

## 应用定位

所有单应用命令统一只用 `--unified-app-id`（全树主键，必填）定位：

| 标识 | 处理 |
|------|------|
| `--unified-app-id` | 直接使用，所有单应用命令都认（唯一定位手段） |

> 定位只认 `--unified-app-id`。`--app-key`/`--name` 只在 `dev app list` 里作**列表过滤**，不能用于定位单个应用做读写；旧的 `--agent-id`/`--app-id`/`--custom-key` 也已移除。拿到 appKey/agentId 时，先用 `dev app list` 查出 unifiedAppId 再操作。

## 应用列表

```bash
dws dev app list --format json
dws dev app list --name DemoApp --format json
dws dev app list --app-key dingxxx --format json
dws dev app list --creator 张三 --sort gmt_modified --order desc --format json
```

| CLI | 说明 |
|-----|------|
| `--cursor` | 游标令牌：首次留空，续翻传上次出参的 `nextCursor` |
| `--page-size` | 单页条数，默认 20 |
| `--name` / `--keyword` | 应用名搜索 |
| `--app-key` | 按 appKey/clientId 过滤 |
| `--creator` | 创建人关键词 |
| `--sort` | 如 `gmt_modified` |
| `--order` | `asc` / `desc` |

> 分页用游标：首页不传 `--cursor`，出参带 `nextCursor`（空=到底）；续翻把它原样回传。旧 `--page`/`--limit`/`--offset` 仍隐藏兼容（跳页/老脚本）。出参字段见 SKILL.md「通用出参约定」。

## 应用状态字段

列表/详情统一用 `appStatus`，按应用生命周期枚举判断；不要和版本 `versionStatus` 混用。

| appStatus | 枚举 | 含义 | 下一步 |
|-----------|------|------|--------|
| `0` | `IN_ACTIVE` | 已停用，应用不可用 | 需要恢复时走 `enable --dry-run` → 确认 → `--yes` |
| `1` | `ACTIVE` | 已激活，应用可用 | 可继续配置权限、网页应用、机器人或版本 |
| `2` | `WAIT_ACTIVE` | 待激活 | 先回读 `get/list` 确认状态；不要直接按已生效处理 |
| `3` | `EXPIRED` | 已过期 | 停止写操作，提示用户到开发者后台或管理员侧处理 |

`create/update` 返回的 `versionStatus` 是版本状态，语义见 `version.md`；它不等同于应用启停状态。

## 应用详情

```bash
dws dev app get --unified-app-id UNIFIED_APP_ID --format json
```

详情主要用于定位和核验应用。若上游偶尔随详情返回 `clientSecret/appSecret`，必须脱敏处理，不要复制到回答里；主动读取凭证仍走 `credentials get`。

## 创建应用

```bash
dws dev app create --name DemoApp --desc "内部应用" --dry-run --format json
dws dev app create --name DemoApp --desc "内部应用" --yes --format json
```

| CLI | 必填 |
|-----|------|
| `--name` | 是 |
| `--desc` | 否 |
| `--icon` | 否 |

## 修改应用

```bash
dws dev app update --unified-app-id ID --name DemoApp2 --desc "新描述" --dry-run --format json
dws dev app update --unified-app-id ID --name DemoApp2 --desc "新描述" --yes --format json
```

至少提供一个更新字段：`--name` / `--desc` / `--icon`。

## 停用 / 启用应用

```bash
dws dev app disable --unified-app-id ID --dry-run --format json
dws dev app disable --unified-app-id ID --yes --format json

dws dev app enable --unified-app-id ID --dry-run --format json
dws dev app enable --unified-app-id ID --yes --format json
```

停用保留数据但应用不可用，可通过 `enable` 恢复。

执行 `disable/enable` 后必须回读 `get` 或 `list`：看到 `appStatus=0` 才算停用完成，看到 `appStatus=1` 才算启用完成；如果接口只返回操作成功但未带状态，向用户说明需要以回读结果为准。

## 删除应用

```bash
dws dev app delete --unified-app-id ID --dry-run --format json
dws dev app delete --unified-app-id ID --yes --format json
```

删除前必须展示应用摘要。删除为异步操作，成功后应用延迟从列表消失。

## 错误处理

| 情况 | 处理 |
|------|------|
| `unknown command` | CLI 构建不含 dev 命令组 |
| `endpoint_not_resolved` | 检查 edition endpoint 注入 |
| 多应用命中 | 展示候选，停止写操作 |
| `ServiceResult.success=false` | 透传 `errorCode/errorMsg` |
