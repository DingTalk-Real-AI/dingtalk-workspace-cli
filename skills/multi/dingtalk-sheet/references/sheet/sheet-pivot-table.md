# 透视表 (pivot-table)

## 真对象硬约束

当用户要求"透视表 / 分组汇总 / 交叉分析 / 按 X 统计 Y"时，**必须**通过 `pivot-table create` 创建真实的透视表对象。**禁止**用 `SUMIFS` / `COUNTIFS` 等普通公式 + `csv-put` 在原表中拼一张"看起来像透视表的汇总表"来代替——静态公式无法随源数据动态更新，且失去交互能力。判断标准：交付后 `pivot-table list` 必须能返回该对象。

## 使用场景

读写透视表对象。本 reference 覆盖 4 个命令：

| 操作需求 | 使用命令 | 说明 |
|---------|---------|------|
| 查看已有透视表 | `pivot-table list` | 获取透视表的结构、数据源和配置 |
| 创建透视表 | `pivot-table create` | 创建透视表对象 |
| 更新透视表 | `pivot-table update` | 更新透视表配置（行/列/值/筛选字段） |
| 删除透视表 | `pivot-table delete` | 删除透视表 |

典型工作流：先读取现有透视表了解配置 -> 执行创建/更新/删除 -> 再次读取验证结果。

## 行/值字段映射（创建前必做）

创建透视表前先识别用户需求中的分组维度和聚合指标，**不要搞反**：

- **rows（行字段）** = 分组维度，即"按什么分组"。例：部门、地区、医生、产品类别
- **values（值字段）** = 聚合指标，即"统计什么数值"。例：销售额（`summarize_by: "sum"`）、订单数（`summarize_by: "count"`）
- **columns（列字段）** = 交叉维度（可选），即"再按什么横向展开"。例：月份、性别

| 用户说 | rows | values | columns |
|--------|------|--------|---------|
| "按部门统计人数" | 部门 | 姓名（`"count"`） | -- |
| "按医生统计费用和结余" | 主管医生 | 费用（`"sum"`）、结余（`"sum"`） | -- |
| "各部门男女人数" | 部门 | 姓名（`"count"`） | 性别 |

**常见配置错误（必须注意）**：
- **数据源范围必须精确**：透视表的数据源范围必须包含表头行，且精确覆盖全部数据行列。范围过大（包含空行/空列）或过小（遗漏数据列）都会导致透视表结果错误
- **行列字段选择要匹配用户意图**：用户说"按商品统计金额" -> 行字段=商品，值字段=金额（`summarize_by: "sum"`）。不要把行列字段搞反
- **聚合类型要匹配**：用户说"统计数量" -> `"count"`；"统计总额" -> `"sum"`；"统计平均" -> `"average"`
- **创建后必须验证**：调用 `pivot-table list` 确认透视表结构正确

## 命令详细参考

### 获取透视表
```
Usage:
  dws sheet pivot-table list [flags]
Example:
  # 列出所有透视表
  dws sheet pivot-table list --node NODE_ID --sheet-id SHEET_ID

  # 获取单个透视表详情
  dws sheet pivot-table list --node NODE_ID --sheet-id SHEET_ID --pivot-table-id PT_ID
Flags:
      --node string             表格文档 ID 或 URL (必填)
      --sheet-id string         工作表 ID 或名称 (必填)
      --pivot-table-id string   透视表 ID (可选，不传则返回全部)
```

### 创建透视表
```
Usage:
  dws sheet pivot-table create [flags]
Example:
  # 按部门统计销售额（默认自动新建工作表存放）
  dws sheet pivot-table create --node NODE_ID \
    --source "'Sheet1'!A1:D100" \
    --properties '{
      "rows": [{"field": "部门"}],
      "values": [{"field": "销售额", "summarize_by": "sum"}],
      "show_row_grand_total": true
    }'

  # 指定放置到已有工作表的特定位置
  dws sheet pivot-table create --node NODE_ID \
    --source "'Sheet1'!A1:E200" \
    --target-sheet-id TARGET_SHEET_ID --target-position "A1" \
    --properties '{
      "rows": [{"field": "部门"}],
      "values": [{"field": "销售额", "summarize_by": "sum"}]
    }'

  # 通过文件传入配置
  dws sheet pivot-table create --node NODE_ID \
    --source "'Sheet1'!A1:D50" --properties @pivot.json
Flags:
      --node string              表格文档 ID 或 URL (必填)
      --source string            数据源区域，A1 表示法含 sheet 前缀 (必填，如 "'Sheet1'!A1:D100")
      --properties string        透视表配置 JSON (必填，含 rows/columns/values/filters)
      --target-sheet-id string   目标工作表 ID 或名称 (可选，不传则自动新建工作表)
      --target-position string   透视表放置位置 (可选，A1 格式单个 cell，如 "B5"，不传默认 A1)
```

### 更新透视表
```
Usage:
  dws sheet pivot-table update [flags]
Example:
  # 先获取现有配置
  dws sheet pivot-table list --node NODE_ID --sheet-id SHEET_ID --pivot-table-id PT_ID

  # 修改后回写
  dws sheet pivot-table update --node NODE_ID --sheet-id SHEET_ID --pivot-table-id PT_ID \
    --properties '{
      "rows": [{"field": "部门"}],
      "values": [
        {"field": "销售额", "summarize_by": "sum"},
        {"field": "订单号", "summarize_by": "count", "display_name": "订单数"}
      ],
      "show_row_grand_total": true
    }'
Flags:
      --node string             表格文档 ID 或 URL (必填)
      --sheet-id string         工作表 ID 或名称 (必填)
      --pivot-table-id string   透视表 ID (必填，可通过 pivot-table list 获取)
      --properties string       透视表配置 JSON (必填)
```

### 删除透视表
```
Usage:
  dws sheet pivot-table delete [flags]
Example:
  dws sheet pivot-table delete --node NODE_ID --sheet-id SHEET_ID --pivot-table-id PT_ID --yes
Flags:
      --node string             表格文档 ID 或 URL (必填)
      --sheet-id string         工作表 ID 或名称 (必填)
      --pivot-table-id string   透视表 ID (必填，可通过 pivot-table list 获取)
```

> [强制] 危险操作：删除不可恢复。必须先向用户展示操作摘要并获得明确同意，用户同意后才加 `--yes` 执行。

## `--properties` JSON Schema 速查

**顶层字段**：

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `rows` | object[] | 否 | 行字段数组（分组维度），详见下方「rows/columns 字段项」表 |
| `columns` | object[] | 否 | 列字段数组（交叉维度），结构同 rows |
| `values` | object[] | 是 | 值字段数组（聚合指标，至少一项），详见下方「values 字段项」表 |
| `filters` | object[] | 否 | 筛选字段数组，详见下方「filters 字段项」表 |
| `show_row_grand_total` | boolean | 否 | 是否显示行总计，默认 true |
| `show_col_grand_total` | boolean | 否 | 是否显示列总计，默认 true |
| `show_subtotals` | boolean | 否 | 是否显示分类小计，默认 true |
| `repeat_row_labels` | boolean | 否 | 是否显示重复项标签，默认 false |
| `collapse` | object | 否 | 行字段折叠状态：字段名 -> 要折叠的项目列表，如 `{"部门": ["A组", "B组"]}` |

**rows/columns 字段项**：

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `field` | string | 是 | 列名（表头文本），必须与数据源首行的列名完全匹配 |
| `display_name` | string | 否 | 显示名称（不传时使用 field 值） |

**filters 字段项**：

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `field` | string | 是 | 列名（表头文本），必须与数据源首行的列名完全匹配 |
| `display_name` | string | 否 | 显示名称（不传时使用 field 值） |

**values 字段项**：

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `field` | string | 是 | 列名（表头文本），必须与数据源首行的列名完全匹配 |
| `summarize_by` | string | 否 | 聚合方式，默认 sum，详见下方枚举表 |
| `display_name` | string | 否 | 显示名称（不传时自动生成，如"求和 - 销售额"） |
| `show_data_as` | string | 否 | 值显示方式：`normal`/`percent_of_row_total`/`percent_of_col_total`/`percent_of_grand_total` |

**summarize_by 枚举值**：

| 值 | 说明 |
|----|------|
| `sum` | 求和（默认） |
| `count` | 计数 |
| `average` | 平均值 |
| `max` | 最大值 |
| `min` | 最小值 |
| `product` | 乘积 |
| `count_numbers` | 数值计数 |
| `std_dev` | 标准偏差 |
| `std_dev_p` | 总体标准偏差 |
| `var` | 方差 |
| `var_p` | 总体方差 |
| `distinct` | 去重计数 |
| `median` | 中位数 |

## `--source` 数据源格式

- 格式：`'SheetName'!StartCell:EndCell`
- 示例：`'Sheet1'!A1:D100`、`'销售数据'!A1:F500`
- 必须包含表头行（通常从第 1 行开始）
- sheet 名称用单引号包裹（含空格或特殊字符时必须）
- MCP 内部自动将 source 解析为内部 sheetId + ISheetRange，CLI 直接透传即可

## 高级功能示例

```bash
# 含折叠 + show_data_as
dws sheet pivot-table create --node <NODE_ID> \
  --source "'Sheet1'!A1:E200" \
  --properties '{
    "rows": [{"field": "部门"}],
    "values": [
      {"field": "销售额", "summarize_by": "sum"},
      {"field": "订单数", "summarize_by": "count", "show_data_as": "percent_of_col_total"}
    ],
    "collapse": {"部门": ["A组"]},
    "show_row_grand_total": true
  }'
```

> collapse/show_data_as 为可选透传字段，CLI 和 MCP 层不做额外校验，直接传递给底层引擎处理。

## 核心工作流

```bash
# -- 工作流 1: 创建简单分组汇总 --

# 1. 先查 sheetId
dws sheet list --node <NODE_ID> -f json

# 2. 查看数据范围确认列名和边界
dws sheet csv-get --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:F5"

# 3. 创建透视表（按部门统计销售额）
dws sheet pivot-table create --node <NODE_ID> \
  --source "'Sheet1'!A1:D100" \
  --properties '{
    "rows": [{"field": "部门"}],
    "values": [{"field": "销售额", "summarize_by": "sum"}],
    "show_row_grand_total": true
  }'

# 4. 验证创建结果（用 create 返回的 targetSheetId 查询）
dws sheet pivot-table list --node <NODE_ID> --sheet-id <TARGET_SHEET_ID>

# -- 工作流 2: 多维度交叉分析 --

dws sheet pivot-table create --node <NODE_ID> \
  --source "'Sheet1'!A1:E200" \
  --properties '{
    "rows": [{"field": "部门"}, {"field": "产品"}],
    "columns": [{"field": "季度"}],
    "values": [
      {"field": "销售额", "summarize_by": "sum"},
      {"field": "订单号", "summarize_by": "count", "display_name": "订单数"}
    ],
    "show_row_grand_total": true,
    "show_col_grand_total": true,
    "show_subtotals": true
  }'

# -- 工作流 3: 更新透视表配置 --

# 先获取现有配置
dws sheet pivot-table list --node <NODE_ID> --sheet-id <SHEET_ID> --pivot-table-id <PT_ID>

# 修改后回写（增加一个值字段）
dws sheet pivot-table update --node <NODE_ID> --sheet-id <SHEET_ID> --pivot-table-id <PT_ID> \
  --properties '{
    "rows": [{"field": "部门"}],
    "values": [
      {"field": "销售额", "summarize_by": "sum"},
      {"field": "利润", "summarize_by": "average", "display_name": "平均利润"}
    ],
    "show_row_grand_total": true
  }'
```

## 上下文传递

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `pivot-table list` | `pivotTables[].id` | 后续 update / delete 的 `--pivot-table-id` |
| `pivot-table list --pivot-table-id` | 完整配置（rows/columns/values/filters/collapse/options） | update 时作为基础配置修改后回写 |
| `pivot-table create` | `pivotTable.id` | 后续 update / delete 的 `--pivot-table-id` |
| `pivot-table create` | `pivotTable.targetSheetId` | 后续 list / update / delete 的 `--sheet-id`（透视表所在工作表） |
| `pivot-table delete` | `message` | 确认删除完成 |
| `sheet list` | 工作表的 `sheetId` | 所有 pivot-table 命令的 `--sheet-id` |

## 注意事项

- [强制] **`--sheet-id` 获取规范**：`sheetId` 未知时必须先通过 `dws sheet list --node <NODE_ID> --format json` 查询，禁止凭空编造（如臆测为 `Sheet1`、`sheet1`、`0`、`default` 等）
- [强制] **创建后必须验证**：透视表创建后必须调用 `pivot-table list` 验证配置是否正确
- [强制] **pivot-table-id 禁止臆测**：必须通过 `pivot-table list` 获取真实的透视表 ID，不可编造
- [强制] **source 必须精确**：数据源范围必须从表头行开始，精确覆盖数据区域，先用 `csv-get` 确认数据边界
- **field 名称必须准确**：rows/columns/values/filters 中的 field 值必须与源数据表头完全一致（区分大小写）
- **透视表自动新建子表**：创建的透视表默认放置在自动新建的子表中，不会覆盖源数据。可通过 `--target-sheet-id` 和 `--target-position` 指定放置到已有工作表
- **不支持修改数据源**：update 仅可修改字段配置和显示选项，不可修改 source
- **折叠状态**：collapse 字段用于控制行字段的展开/折叠，格式为 {字段名: [要折叠的项]}
- **大 JSON 用 @file**：`--properties` 支持 `@文件路径` 读取本地 JSON 文件
