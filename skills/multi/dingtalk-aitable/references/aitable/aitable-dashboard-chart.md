# dashboard & chart — 仪表盘与图表

## 建议操作顺序

```bash
# 1) 只在缺少配置结构时读取模板
dws aitable dashboard config-example --format json
dws aitable +chart-widgets-example --format json

# 2) 先拿 dashboard，再拿 chart 详情
dws aitable dashboard get --base-id <BASE_ID> --dashboard-id <DASHBOARD_ID> --format json
dws aitable chart get --base-id <BASE_ID> --dashboard-id <DASHBOARD_ID> --chart-id <CHART_ID> --format json
```

只按名称创建、改名并确认时，不需要读取配置示例或 Help：

```bash
dws aitable dashboard create --base-id <BASE_ID> --name <名称> --format json
dws aitable +dashboard-update --base-id <BASE_ID> --dashboard-id <DASHBOARD_ID> --name <新名称> --format json
dws aitable +dashboard-get --base-id <BASE_ID> --dashboard-id <DASHBOARD_ID> --format json
```

部分服务端更新回执可能仍回显更新前名称；只做一次 `+dashboard-get` 读回，以该后续权威读回为最终状态。读回已是目标名称时判定更新完成，不重放写操作，也不因旧回执继续探测 Help。

## 要点

- `dashboard get` 返回的 `charts[].chartId` 可直接给 `chart get` 使用
- 删除 dashboard 会级联删除其全部 chart；确认前必须说明该影响
- `dashboard share get` 可能返回 `404`（资源不存在或未开通），需按可重试错误处理，不要误判为参数拼错
- `chart share get` 可正常返回 `enabled/shareUrl`，用于分享状态判断

## dashboard 子命令

| 命令 | 用途 | 必填参数 | 说明 |
|------|------|----------|------|
| `dashboard get` | 获取仪表盘详情（含 charts 列表） | `--base-id` `--dashboard-id` | — |
| `dashboard create` | 创建仪表盘 | `--base-id` + (`--config` 或 `--name`) | `--name` 简化版创建空看板；`--config` 传完整 JSON |
| `dashboard update` | 更新仪表盘 | `--base-id` `--dashboard-id` + (`--config` 或 `--name`) | `--name` 仅改名；`--config` 更新完整配置 |
| `dashboard delete` | 删除仪表盘 | `--base-id` `--dashboard-id` | 级联删除全部 chart，不可逆；由 Runtime 请求确认，Reference 不携带确认绕过参数 |
| `dashboard config-example` | 查看仪表盘配置模板 | 无 | 创建前先调此命令了解 config 结构 |
| `dashboard arrange` | 自动重排图表布局 | `--base-id` `--dashboard-id` | 把图表按行铺满网格，避免某行只占半幅、留下大片空白；返回 `{totalColumns, layout, alignedChartCount}` |

## chart 子命令

| 命令 | 用途 | 必填参数 |
|------|------|----------|
| `chart get` | 获取图表详情 | `--base-id` `--dashboard-id` `--chart-id` |
| `chart create` | 创建图表 | `--base-id` `--dashboard-id` `--config` `--layout` |
| `chart update` | 更新图表配置 | `--base-id` `--dashboard-id` `--chart-id` `--config` |
| `chart delete` | 删除图表 | `--base-id` `--dashboard-id` `--chart-id` | 不可逆；由 Runtime 请求确认，Reference 不携带确认绕过参数 |
| `+chart-widgets-example` | 查看所有图表类型的 widgets 模板 | 无 |

## 配置获取流程

已有符合当前 leaf Schema 的合法 config 时直接创建/更新，不读取模板。只有缺少结构时调用一次 `+chart-widgets-example`；该命令当前返回所有图表类型示例，随后只使用目标类型，并按真实 tableId/fieldId 填充后执行。
