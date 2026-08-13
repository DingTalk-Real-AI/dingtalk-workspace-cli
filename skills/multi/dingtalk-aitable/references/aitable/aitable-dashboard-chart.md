# dashboard & chart — 仪表盘与图表

## 创建首选流程

```bash
# 仅建仪表盘
python3 <本 Skill 绝对目录>/scripts/aitable_ops.py dashboard <BASE_ID> "<仪表盘名>"

# 建仪表盘和常用图表
python3 <本 Skill 绝对目录>/scripts/aitable_ops.py dashboard <BASE_ID> "<仪表盘名>" \
  --chart-specs <workspace内/charts.json>
```

统一入口的 dashboard 操作是 `dashboard create → chart create（可选）→ dashboard get` 的唯一首选
recipe，并输出 `dws-skill-script-ledger/v1`。不要在脚本前调用 config-example 或
widgets-example，也不要在成功后重复创建或回读。

`charts.json` 是 1–6 项数组，每项参数：

| 参数 | 要求 |
|---|---|
| `name` | 必填，图表名 |
| `chart_type` | 必填：`AREA`、`BAR`、`HISTOGRAM`、`LINE`、`PIE`、`STATISTICS` |
| `table_id` | 必填，当前链路的真实 tableId |
| `measure_type` | `record-count`（默认）或 `field` |
| `measure_field_id` | `measure_type=field` 时必填 |
| `dimension_field_id` | 分组、分类或时间维度需要时填写 |
| `aggregation` | 可选：`sum`、`count`、`count_distinct`、`average`、`min`、`max` |
| `view_id` | 可选；不用视图时省略 |

例如按状态统计记录数：

```json
[{"name":"跟进状态记录数","chart_type":"HISTOGRAM","table_id":"<tableId>","measure_type":"record-count","dimension_field_id":"<状态fieldId>"}]
```

脚本不支持的图表类型或完整高级配置才走下方原子命令；这种例外至多读取一次
`chart widgets-example`，再按真实 tableId/fieldId 构造配置。

## 查询与管理要点

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
| `dashboard config-example` | 查看仪表盘配置模板 | 无 | 仅脚本不支持的高级配置按需读取一次 |
| `dashboard arrange` | 自动重排图表布局 | `--base-id` `--dashboard-id` | 把图表按行铺满网格，避免某行只占半幅、留下大片空白；返回 `{totalColumns, layout, alignedChartCount}` |

## chart 子命令

| 命令 | 用途 | 必填参数 |
|------|------|----------|
| `chart get` | 获取图表详情 | `--base-id` `--dashboard-id` `--chart-id` |
| `chart create` | 创建图表 | `--base-id` `--dashboard-id` `--config` `--layout` |
| `chart update` | 更新图表配置 | `--base-id` `--dashboard-id` `--chart-id` `--config` |
| `chart delete` | 删除图表 | `--base-id` `--dashboard-id` `--chart-id` `--yes` |
| `chart widgets-example` | 查看图表 widgets 配置模板 | 无；返回很大 | 仅脚本不支持的高级图表按需读取一次 |

原子 `chart create` 必须同时传 `--layout`。返回 chartId 后用 `chart get`，或最后
一次 `dashboard get` 核对；回读成功即停止。脚本已自动完成这些动作，不再重复执行。
