# 浮动图表 (chart)

## 真对象硬约束

当用户要求"画个图 / 数据可视化 / 趋势图 / 对比图 / 占比图"时，**必须**通过 `chart create` 创建真实的图表对象。**禁止**用本地脚本调 matplotlib / seaborn 生成图片再插入到表格代替——静态图片无法随源数据更新，且失去交互能力。判断标准：交付后 `chart list` 必须能返回该对象。

## 使用场景

读写浮动图表对象。本 reference 覆盖 4 个命令：

| 操作需求 | 使用命令 | 说明 |
|---------|---------|------|
| 查看已有图表 | `chart list` | 获取图表的类型、数据源和样式配置 |
| 创建图表 | `chart create` | 创建浮动图表对象 |
| 更新图表 | `chart update` | 更新图表配置（PUT 语义整体覆盖） |
| 删除图表 | `chart delete` | 删除浮动图表 |

典型工作流：先读取现有图表了解配置 → 执行创建/更新/删除 → 再次读取验证结果。

## 需求→图表类型映射（创建前必查）

| 用户说 | 图表类型 | 备注 |
|--------|---------|------|
| "占比"、"比例"、"各XX占多少" | 饼图（pie） | 单维度占比首选 |
| "环形图"、"空心饼图" | 环形图（doughnut） | 与 pie 类似但中心空心 |
| "对比"、"各XX的YY" | 柱形图（column） | 多类别数值对比 |
| "堆叠柱"、"堆积" | 堆积柱形图（columnStacked） | 纵向堆叠 |
| "条形"、"横向对比" | 条形图（bar） | 横向柱形 |
| "横向堆叠" | 堆积条形图（barStacked） | 横向堆叠 |
| "趋势"、"变化"、"走势" | 折线图（line） | 时间序列首选 |
| "堆叠折线" | 堆积折线图（lineStacked） | 堆叠折线 |
| "面积图" | 面积图（area） | 填充折线下方区域 |
| "堆叠面积" | 堆积面积图（areaStacked） | 堆叠面积 |
| "百分比面积" | 百分比堆积面积图（areaPercentStacked） | 占比式面积 |
| "散点图"、"相关性" | 散点图（scatter） | 两变量相关性 |
| "雷达图"、"多维度" | 雷达图（radar） | 多维度对比 |
| "柱+线"、"组合图"、"双轴" | 组合图（combo） | 多种图表叠加，支持副轴 |

**多图表需求**：当用户同时提到多种分析（如"统计占比 + 对比数量"），必须创建多个图表，每个对应一种类型，不要只做一个。

## 命令详细参考

### 获取浮动图表
```
Usage:
  dws sheet chart list [flags]
Example:
  # 列出所有浮动图表
  dws sheet chart list --node NODE_ID --sheet-id SHEET_ID

  # 获取单个图表详情
  dws sheet chart list --node NODE_ID --sheet-id SHEET_ID --chart-id CHART_ID
Flags:
      --node string       表格文档 ID 或 URL (必填)
      --sheet-id string   工作表 ID 或名称 (必填)
      --chart-id string   浮动图表 ID (可选，不传则返回全部)
```

### 创建浮动图表
```
Usage:
  dws sheet chart create [flags]
Example:
  dws sheet chart create --node NODE_ID --sheet-id SHEET_ID --properties '{
    "position": {"row": 12, "col": "A"},
    "dimensions": {"width": 600, "height": 400},
    "chart": {
      "type": "column",
      "series": [{"name": "B1", "value": ["B2:B10"]}],
      "category": ["A2:A10"],
      "title": {"show": true, "text": "销售数据"}
    }
  }'
Flags:
      --node string         表格文档 ID 或 URL (必填)
      --sheet-id string     工作表 ID 或名称 (必填)
      --properties string   图表完整配置 JSON (必填，含 position/dimensions/chart)
```

### 更新浮动图表
```
Usage:
  dws sheet chart update [flags]
Example:
  dws sheet chart update --node NODE_ID --sheet-id SHEET_ID --chart-id CHART_ID --properties '{
    "position": {"row": 12, "col": "A"},
    "dimensions": {"width": 800, "height": 500},
    "chart": {
      "type": "line",
      "series": [{"name": "B1", "value": ["B2:B20"]}],
      "category": ["A2:A20"],
      "title": {"show": true, "text": "月度趋势（更新）"}
    }
  }'
Flags:
      --node string         表格文档 ID 或 URL (必填)
      --sheet-id string     工作表 ID 或名称 (必填)
      --chart-id string     浮动图表 ID (必填，可通过 chart list 获取)
      --properties string   图表完整配置 JSON (必填，PUT 语义整体覆盖)
```

### 删除浮动图表
```
Usage:
  dws sheet chart delete [flags]
Example:
  dws sheet chart delete --node NODE_ID --sheet-id SHEET_ID --chart-id CHART_ID
Flags:
      --node string       表格文档 ID 或 URL (必填)
      --sheet-id string   工作表 ID 或名称 (必填)
      --chart-id string   浮动图表 ID (必填，可通过 chart list 获取)
```

> [强制] 危险操作：删除不可恢复。必须先向用户展示操作摘要并获得明确同意，用户同意后才加 `--yes` 执行。

## `--properties` JSON Schema 速查

**顶层字段**：

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `position` | object | 是 | 锚点位置，详见下方 position 字段表 |
| `offset` | object | 否 | 偏移量，详见下方 offset 字段表 |
| `dimensions` | object | 是 | 尺寸，详见下方 dimensions 字段表 |
| `chart` | object | 是 | 图表配置，详见下方 chart 字段表 |

**position 字段**：

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `row` | number | 是 | 锚点行索引（0-based），如 0 表示第一行 |
| `col` | string/number | 是 | 锚点列。支持字母表示法（`"A"`=第1列, `"B"`=第2列, `"Z"`=第26列, `"AA"`=第27列）或 0-based 数字（`0`=第1列） |

**offset 字段**（可选，不传时默认 0）：

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `offsetX` | number | 否 | 相对锚点的水平偏移量，单位像素，默认 0 |
| `offsetY` | number | 否 | 相对锚点的垂直偏移量，单位像素，默认 0 |

**dimensions 字段**：

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `width` | number | 是 | 图表宽度，单位像素，正整数 |
| `height` | number | 是 | 图表高度，单位像素，正整数 |

**chart 字段**：

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `type` | string | 是 | 图表类型，枚举值：`column` / `bar` / `columnStacked` / `barStacked` / `line` / `lineStacked` / `area` / `areaStacked` / `areaPercentStacked` / `pie` / `doughnut` / `scatter` / `radar` / `combo` |
| `series` | object[] | 非 combo 必填 | 数据系列数组，至少一项，详见下方 series 字段表。combo 类型不使用此字段 |
| `subCharts` | object[] | combo 必填 | 组合图子图数组，至少一项，详见下方 subCharts 字段表。仅 combo 类型使用 |
| `category` | string[] | 否 | 分类轴（X轴）数据区域，A1 表示法字符串数组（如 `["A2:A10"]`）。combo 类型在 subCharts 内声明 |
| `title` | object | 否 | 图表标题配置，详见下方 title 字段表 |
| `legend` | object | 否 | 图例配置，详见下方 legend 字段表 |
| `catAx` | object | 否 | 分类轴（X轴）样式配置，详见下方轴配置表 |
| `valAx` | object | 否 | 值轴（Y轴）样式配置，详见下方轴配置表 |

**series 数组项**：

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `name` | string | 否 | 系列名称的单元格引用，A1 表示法（如 `"B1"`）。不传时图例显示为"系列1"等默认名 |
| `value` | string[] | 是 | 系列数据区域，A1 表示法字符串数组（如 `["B2:B10"]`）。每个元素为一段连续区域 |

**subCharts 数组项**（仅 combo 类型）：

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `type` | string | 是 | 子图类型，仅允许笛卡尔坐标系类型：`column` / `bar` / `columnStacked` / `barStacked` / `line` / `lineStacked` / `area` / `areaStacked` / `areaPercentStacked`。不允许 pie/doughnut/scatter/radar/combo |
| `series` | object[] | 是 | 子图数据系列，格式同上方 series 数组项 |
| `category` | string[] | 是 | 子图分类轴数据区域（笛卡尔坐标系必须提供） |
| `useSecondaryAxis` | boolean | 否 | 是否使用副轴（右侧 Y 轴），默认 false |

**title 字段**：

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `show` | boolean | 否 | 是否显示标题，默认 false |
| `text` | string | 否 | 标题文本内容 |

**legend 字段**：

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `show` | boolean | 否 | 是否显示图例，默认 true |
| `pos` | string | 否 | 图例位置，枚举值：`t`（顶部）/ `b`（底部）/ `l`（左侧）/ `r`（右侧）/ `none`（隐藏） |

**catAx / valAx 轴配置**：

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `show` | boolean | 否 | 是否显示该轴，默认 true |
| `pos` | string | 否 | 轴位置，枚举值：`l`（左）/ `t`（上）/ `b`（下）/ `r`（右） |
| `titleConfig` | object | 否 | 轴标题配置：`{ show: boolean, title: string }`。show 控制是否显示轴标题，title 为轴标题文本 |
| `axisMin` | number \| null | 否 | 轴最小值，传 null 或不传表示自动计算 |
| `axisMax` | number \| null | 否 | 轴最大值，传 null 或不传表示自动计算 |
| `splitLine` | boolean | 否 | 是否显示网格线，默认 true |
| `minorSplitLine` | boolean | 否 | 是否显示次网格线，默认 false |
| `axisLabel` | boolean | 否 | 是否显示轴标签（刻度文字），默认 true |
| `axisLine` | boolean | 否 | 是否显示轴线，默认 true |

## A1 区域引用规则

- `series[].value`、`series[].name`、`category` 均使用 A1 表示法字符串
- 格式：`"A2:B10"`（默认使用 `--sheet-id` 对应的工作表）
- 跨 sheet 引用：`"SHEET_ID!A2:B10"`（前缀为工作表 ID）
- 单元格引用：`"B1"`（等同于 `"B1:B1"`）
- 直接传 A1 表示法字符串即可，命令会处理成图表所需的区域引用

## 图表位置选择（创建前必做）

凭感觉挑行列号可能越界或遮挡数据。按以下四步走：

1. **查尺寸**：`dws sheet info --node <NODE_ID> --sheet-id <SHEET_ID>` 获取工作表行列数信息。
2. **估跨度**：默认单元格约 **100 px 宽 × 25 px 高**，`needCols = ceil(width/100)`，`needRows = ceil(height/25)`。
3. **校验**：`position.row + needRows ≤ rowCount` 且 `col + needCols ≤ columnCount`。
4. **不够就先扩表**：先调 `insert-dimension` 扩行/列，再 create。

**图表落点禁止压在已有数据矩形内**——必须落在数据区**右侧或下方的空白**，否则图表浮层会遮挡原始数据。

**示例**：数据在 A1:E20，放 600×400 图 → `needCols=6, needRows=16`
- [正确] `{"row": 22, "col": "A"}` — 放数据下方
- [正确] `{"row": 0, "col": "G"}` — 放数据右侧
- [错误] `{"row": 5, "col": "B"}` — 压在数据区域上

## 核心工作流

```bash
# ── 工作流 1: 创建柱形图 ──

# 1. 先查 sheetId
dws sheet list --node <NODE_ID> -f json

# 2. 查看数据范围确认列对应关系
dws sheet csv-get --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:E5"

# 3. 创建柱形图
dws sheet chart create --node <NODE_ID> --sheet-id <SHEET_ID> --properties '{
  "position": {"row": 22, "col": "A"},
  "dimensions": {"width": 600, "height": 400},
  "chart": {
    "type": "column",
    "series": [
      {"name": "B1", "value": ["B2:B10"]},
      {"name": "C1", "value": ["C2:C10"]}
    ],
    "category": ["A2:A10"],
    "title": {"show": true, "text": "各月销售额对比"},
    "legend": {"show": true, "pos": "t"}
  }
}'

# 4. 验证创建结果
dws sheet chart list --node <NODE_ID> --sheet-id <SHEET_ID>

# ── 工作流 2: 创建折线图 ──

dws sheet chart create --node <NODE_ID> --sheet-id <SHEET_ID> --properties '{
  "position": {"row": 22, "col": "H"},
  "dimensions": {"width": 600, "height": 400},
  "chart": {
    "type": "line",
    "series": [
      {"name": "B1", "value": ["B2:B13"]},
      {"name": "C1", "value": ["C2:C13"]}
    ],
    "category": ["A2:A13"],
    "title": {"show": true, "text": "年度趋势"},
    "legend": {"show": true, "pos": "b"}
  }
}'

# ── 工作流 3: 创建饼图 ──

dws sheet chart create --node <NODE_ID> --sheet-id <SHEET_ID> --properties '{
  "position": {"row": 0, "col": "G"},
  "dimensions": {"width": 500, "height": 400},
  "chart": {
    "type": "pie",
    "series": [{"value": ["B2:B6"]}],
    "category": ["A2:A6"],
    "title": {"show": true, "text": "各部门人数占比"}
  }
}'

# ── 工作流 4: 创建组合图（combo）──
# 当用户需要"柱+线"叠加、双轴对比时，使用 combo 类型：

dws sheet chart create --node <NODE_ID> --sheet-id <SHEET_ID> --properties '{
  "position": {"row": 40, "col": "A"},
  "dimensions": {"width": 800, "height": 500},
  "chart": {
    "type": "combo",
    "subCharts": [
      {"type": "column", "series": [{"name": "B1", "value": ["B2:B10"]}], "category": ["A2:A10"]},
      {"type": "line", "series": [{"name": "C1", "value": ["C2:C10"]}], "category": ["A2:A10"], "useSecondaryAxis": true}
    ],
    "title": {"show": true, "text": "销售额与增长率"}
  }
}'
```

## Update 三步法（PUT 语义）

`chart update` 是 PUT 语义（整体覆盖），不是 PATCH。必须按三步操作：

1. `chart list --chart-id <id>` 获取完整配置
2. 在获取的配置基础上**局部**修改要改的字段，其余保持不变
3. 把**完整配置**整体回写到 `--properties`

> 关键：**不能只提交局部配置**，否则未传字段会被还原为默认值。

```bash
# 1. 获取现有配置
dws sheet chart list --node <NODE_ID> --sheet-id <SHEET_ID> --chart-id <CHART_ID> -f json
# → 得到完整的 properties 结构（position/dimensions/chart）

# 2. 基于返回结果修改（如改标题 + 调整尺寸），构造 --properties JSON

# 3. 整体回写
dws sheet chart update --node <NODE_ID> --sheet-id <SHEET_ID> --chart-id <CHART_ID> \
  --properties '<完整修改后的 JSON>'
```

## 上下文传递

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `chart list` | `floatCharts[].id` | 后续 update / delete 的 `--chart-id` |
| `chart list --chart-id` | 完整的 properties（position/dimensions/chart） | update 时作为基础配置修改后回写 |
| `chart create` | `floatChart.id` | 后续 update / delete 的 `--chart-id` |
| `chart update` | 更新后的完整信息 | 确认更新结果 |
| `chart delete` | `message` | 确认删除完成 |
| `sheet list` | 工作表的 `sheetId` | 所有 chart 命令的 `--sheet-id` |

## 注意事项

- [强制] **`--sheet-id` 获取规范**：`sheetId` 未知时必须先通过 `dws sheet list --node <NODE_ID> --format json` 查询，禁止凭空编造（如臆测为 `Sheet1`、`sheet1`、`0`、`default` 等）
- [强制] **创建后必须验证**：图表创建后必须调用 `chart list` 验证配置是否正确
- [强制] **chart-id 禁止臆测**：必须通过 `chart list` 获取真实的图表 ID，不可编造
- **图表类型映射**：用户说"柱形图"用 `column`，"条形图"（横向）用 `bar`，"散点图"用 `scatter`，"雷达图"用 `radar`，"环形图"用 `doughnut`，"柱+线/组合/双轴"用 `combo`
- **堆积类型后缀**：堆叠版本的图表在基本类型后加 `Stacked`（如 `columnStacked`、`lineStacked`、`areaStacked`）
- **combo 组合图**：不用顶层 `series`/`category`，改用 `chart.subCharts` 数组，每个子图独立声明 `type`/`series`/`category`，可选 `useSecondaryAxis: true` 指定副轴。子图 type 仅允许笛卡尔坐标系类型（column/bar/line/area 及其 Stacked 变体），不允许 pie/doughnut/scatter/radar/combo
- **位置不得压在数据区上**：图表浮层会遮挡原始数据，必须放在空白区域
- **多图表需求**：多次调用 `chart create`，不要试图用一个图表包含所有分析
- **跨 sheet 引用数据**：在 A1 区域前加 `SHEET_ID!` 前缀，如 `"sheetXXX!A2:B10"`
- **大 JSON 用 @file**：`--properties` 支持 `@文件路径` 读取本地 JSON 文件
