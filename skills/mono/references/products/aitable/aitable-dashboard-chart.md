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
- `dashboard share get` 可能返回 `404`（资源不存在或未开通），需按可重试错误处理，不要误判为参数拼错
- `chart share get` 可正常返回 `enabled/shareUrl`，用于分享状态判断

## dashboard 子命令

| 命令 | 用途 | 必填参数 | 说明 |
|------|------|----------|------|
| `dashboard get` | 获取仪表盘详情（含 charts 列表） | `--base-id` `--dashboard-id` | — |
| `dashboard create` | 创建仪表盘 | `--base-id` + (`--config` 或 `--name`) | `--name` 简化版创建空看板；`--config` 传完整 JSON |
| `dashboard update` | 更新仪表盘 | `--base-id` `--dashboard-id` + (`--config` 或 `--name`) | `--name` 仅改名；`--config` 更新完整配置 |
| `dashboard delete` | 删除仪表盘 | `--base-id` `--dashboard-id` `--yes` | — |
| `dashboard config-example` | 查看仪表盘配置模板 | 无 | 创建前先调此命令了解 config 结构 |

## chart 子命令

| 命令 | 用途 | 必填参数 |
|------|------|----------|
| `chart get` | 获取图表详情 | `--base-id` `--dashboard-id` `--chart-id` |
| `chart create` | 创建图表 | `--base-id` `--dashboard-id` `--config` `--layout` |
| `chart update` | 更新图表配置 | `--base-id` `--dashboard-id` `--chart-id` `--config` |
| `chart delete` | 删除图表 | `--base-id` `--dashboard-id` `--chart-id` `--yes` |
| `chart widgets-example` | 查看图表 widgets 配置模板 | 无 |

## 配置获取流程

创建图表前，必须先调用 `chart widgets-example` 查看配置模板，了解每种图表类型需要的字段结构，然后根据实际 tableId 和 fieldId 填充配置；同时必须传 `--layout` 指定图表位置和尺寸，例如 `--layout '{"x":0,"y":0,"w":6,"h":4}'`。
