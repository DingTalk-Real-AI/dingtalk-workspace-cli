# dashboard & chart — 专项配置

仅在 Chart 创建/更新缺少 config，或 Dashboard 明确需要完整 config/自动布局时读取。普通 Dashboard 按名称创建、读取、改名或删除走根 Skill 直达，不读取本文。

本文是 Dashboard/Chart 专项配置的终点；命令和参数已覆盖时直接执行，不再读取其他 AITable Reference 或 Help。

## 专项配置最短路径

```bash
# Dashboard 完整 config 缺少结构时，只读取 Dashboard 模板
dws aitable dashboard config-example --format json

# Chart config 缺少结构时，只读取 Chart 模板
dws aitable +chart-widgets-example --format json

# 已有真实 ID 时按需读取详情
dws aitable dashboard get --base-id <BASE_ID> --dashboard-id <DASHBOARD_ID> --format json
dws aitable chart get --base-id <BASE_ID> --dashboard-id <DASHBOARD_ID> --chart-id <CHART_ID> --format json
```

## 要点

- `dashboard get` 返回的 `charts[].chartId` 可直接给 `chart get` 使用
- `chart create` 以及携带 `--layout` 的 `chart update` 会在写入前自动调用 `get_dashboard` 并强制校验根网格。只有 `schemaVersionTypeVerified=true` 时才使用原始类型判据：JSON number `2` 为 48 列，其他已验证值为 12 列；证据缺失时命令停止且不调用 Chart 写工具
- 任务上下文已确认应用模式时，Chart 写命令显式传 `--is-app-mode=true`，DWS 按只读上下文强制使用 48 列；该 flag 不会进入 MCP payload。未确认应用模式时省略，不能为了绕过 schemaVersion 证据传 true
- 全局 12/48 列只适用于根布局。存在非根 `parentId` 时使用容器自己的坐标系；拿不到容器真实列数时，创建操作停止，更新操作省略 layout 以保留原值
- `schemaVersion` 与 `isAppMode` 都是只读字段，不能写入或透传到 Dashboard config
- 删除 dashboard 会级联删除其全部 chart；确认前必须说明该影响
- `dashboard share get` 可能返回 `404`（资源不存在或未开通），需按可重试错误处理，不要误判为参数拼错
- `chart share get` 可正常返回 `enabled/shareUrl`，用于分享状态判断
- Dashboard 普通 CRUD 不在本文重新维护；需要完整配置时才读取 `dashboard config-example`

## Dashboard 专项命令

| 命令 | 用途 | 必填参数 | 说明 |
|------|------|----------|------|
| `dashboard config-example` | 查看完整仪表盘配置模板 | 无 | 仅在完整 config 结构缺失时调用一次 |
| `dashboard arrange` | 自动重排图表布局 | `--base-id` `--dashboard-id` | 把图表按行铺满网格，避免某行只占半幅、留下大片空白；返回 `{totalColumns, layout, alignedChartCount}` |

## chart 子命令

| 命令 | 用途 | 必填参数 |
|------|------|----------|
| `chart get` | 获取图表详情 | `--base-id` `--dashboard-id` `--chart-id` |
| `chart create` | 创建图表 | `--base-id` `--dashboard-id` `--config` `--layout`；根布局按 Dashboard 元信息选择 12/48 列 |
| `chart update` | 更新图表配置 | `--base-id` `--dashboard-id` `--chart-id` `--config` |
| `chart delete` | 删除图表 | `--base-id` `--dashboard-id` `--chart-id` | 不可逆；由 Runtime 请求确认，Reference 不携带确认绕过参数 |
| `+chart-widgets-example` | 查看所有图表类型的 widgets 模板 | 无 |

## 配置获取流程

已有符合当前 leaf Schema 的合法 config 时直接创建/更新，不读取模板。只有 Chart config 缺少结构时调用一次 `+chart-widgets-example`；该命令当前返回所有图表类型示例，随后只使用目标类型，并按真实 tableId/fieldId 填充后执行。Dashboard 完整 config 缺少结构时只调用一次 `dashboard config-example`，不要同时读取两个模板。
