# dashboard & chart — 仪表盘与图表

## 建议操作顺序

```bash
# 1) 先看配置模板（JSONC）
dws aitable dashboard config-example --format json
dws aitable chart widgets-example --format json

# 2) 先拿 dashboard，再拿 chart 详情
dws aitable dashboard get --base-id <BASE_ID> --dashboard-id <DASHBOARD_ID> --format json
dws aitable chart get --base-id <BASE_ID> --dashboard-id <DASHBOARD_ID> --chart-id <CHART_ID> --format json
```

## 要点

- `dashboard get` 返回的 `charts[].chartId` 可直接给 `chart get` 使用
- `dashboard share get` 可能返回 `404`（`retryable:true`）——从未分享、甚至刚开启分享后立即查都可能 404；`data` 为 `{}`。按可重试错误处理，不要误判为参数拼错。它有时也会返回 `success + data`（如关闭分享之后），行为不稳定，别用它当"是否已分享"的唯一判据。
- `chart share get` 稳定返回 `success + data`（含 `enabled` 等），可用于分享状态判断；从未分享时 `enabled=false`，不会 404。
- ⚠️ **`dashboard share update` 开启 ORG 分享后返回的 `shareType` 是 `"[1]"`（未映射回 `ORG`，服务端已知问题）**；`chart share update --share-type ORG` 则正确返回 `shareType="ORG"`。判断 dashboard 是否 ORG 分享时对 `"[1]"` 做兼容。
- `chart share update` / `dashboard share update` 的 `--enabled` 是字符串 flag：`--enabled false`（空格）和 `--enabled=false` 都能正确关闭分享。

## dashboard 子命令

| 命令 | 用途 | 必填参数 | 说明 |
|------|------|----------|------|
| `dashboard get` | 获取仪表盘详情（含 charts 列表） | `--base-id` `--dashboard-id` | — |
| `dashboard create` | 创建仪表盘 | `--base-id` + (`--config` 或 `--name`) | `--name` 简化版创建空看板；`--config` 传完整 JSON |
| `dashboard update` | 更新仪表盘 | `--base-id` `--dashboard-id` + (`--config` 或 `--name`) | `--name` 仅改名；`--config` 更新完整配置 |
| `dashboard arrange` | 自动重排仪表盘图表布局 | `--base-id` `--dashboard-id` | 让服务端重新排布 charts 位置 |
| `dashboard delete` | 删除仪表盘 | `--base-id` `--dashboard-id` `--yes` | — |
| `dashboard config-example` | 查看仪表盘配置模板 | 无 | 创建前先调此命令了解 config 结构 |
| `dashboard share get` | 获取仪表盘分享配置 | `--base-id` `--dashboard-id` | 可能 404，见上方要点 |
| `dashboard share update` | 更新仪表盘分享配置 | `--base-id` `--dashboard-id` `--enabled` | `--enabled true` 开启，`--share-type` 用 `ORG`（ORG 回显 `shareType="[1]"`）；部分组织禁用了 `PUBLIC` 公开分享；若报 `Illegal argument`，改用 `ORG`（组织内分享）。`--enabled false` 关闭 |

## chart 子命令

| 命令 | 用途 | 必填参数 |
|------|------|----------|
| `chart get` | 获取图表详情 | `--base-id` `--dashboard-id` `--chart-id` |
| `chart create` | 创建图表 | `--base-id` `--dashboard-id` `--config` `--layout` |
| `chart update` | 更新图表配置 | `--base-id` `--dashboard-id` `--chart-id` `--config` |
| `chart delete` | 删除图表 | `--base-id` `--dashboard-id` `--chart-id` `--yes` |
| `chart widgets-example` | 查看图表 widgets 配置模板 | 无 |
| `chart share get` | 获取图表分享配置 | `--base-id` `--dashboard-id` `--chart-id` |
| `chart share update` | 更新图表分享配置（`--enabled true/false`，开启配 `--share-type`，用 `ORG`；部分组织禁用了 `PUBLIC` 公开分享；报 `Illegal argument` 时改用 `ORG`） | `--base-id` `--dashboard-id` `--chart-id` `--enabled` |

> `chart create` 的 `--layout` 是**必填**；它和携带 layout 的 `chart update` 都会在写前自动调用 `get_dashboard` 并强制校验根布局。已确认应用模式时传本地只读参数 `--is-app-mode=true`，使用 48 列；否则只有 `schemaVersionTypeVerified=true` 时才按原始类型判断，JSON number `2` 为 48 列，其他已验证值为 12 列。证据缺失、布局越界或使用非根容器 `parentId` 时停止且不调用 Chart 写工具；`parentId` 省略、为空或 `root-responsive-layout` 均属于根布局。`schemaVersion` 与 `isAppMode` 不能写入或透传到 Dashboard/Chart payload。`chart update` 的 `--config` 也**必填**——即便只想改 layout，也要带完整 config，否则服务端拒绝。

## 配置获取流程

创建图表前，必须先调用 `chart widgets-example` 查看配置模板，了解每种图表类型需要的字段结构，然后根据实际 tableId 和 fieldId 填充配置。
