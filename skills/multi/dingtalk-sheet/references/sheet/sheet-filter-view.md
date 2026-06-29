# 筛选视图 (filter-view)

## 使用场景

### 筛选视图

用户说"筛选视图/查看筛选视图/有哪些筛选视图/筛选视图列表":
- 获取所有筛选视图 → `filter-view list`

用户说"筛选视图详情/查看某个筛选视图/筛选视图信息/筛选视图配置":
- 获取单个筛选视图详情 → `filter-view info`

用户说"创建筛选视图/新建筛选视图/添加筛选视图":
- 创建筛选视图 → `filter-view create`

用户说"更新筛选视图/修改筛选视图/改筛选视图名称/改筛选视图范围":
- 更新筛选视图属性 → `filter-view update`

用户说"删除筛选视图/移除筛选视图":
- 删除筛选视图 → `filter-view delete`

用户说"设置筛选条件/添加筛选条件/配置筛选视图条件/按值筛选/按条件筛选/按颜色筛选":
- 设置筛选视图列条件 → `filter-view update-criteria`

用户说"查看筛选条件/有哪些筛选条件/筛选视图设了什么条件/列出筛选条件":
- 列出所有列条件 → `filter-view list-criteria`
- 查看某一列的条件 → `filter-view get-criteria --column N`

用户说"清除筛选条件/移除筛选条件/取消筛选条件":
- 清除筛选视图列条件 → `filter-view delete-criteria`
- 注意与 `filter-view delete`（删除整个筛选视图）区分：`delete-criteria` 仅清除指定列的条件，不删除筛选视图本身

## 命令详细参考

### 获取所有筛选视图
```
Usage:
  dws sheet filter-view list [flags]
Example:
  dws sheet filter-view list --node <NODE_ID> --sheet-id <SHEET_ID>
  dws sheet filter-view list --node "https://alidocs.dingtalk.com/i/nodes/<DOC_UUID>" --sheet-id "Sheet1"
Flags:
      --node string       表格文档 ID 或 URL (必填)
      --sheet-id string   工作表 ID 或名称 (必填)
```

获取指定工作表的所有筛选视图列表，返回每个筛选视图的 ID、名称和范围信息。
- **用途**：查看当前工作表上已创建的所有筛选视图，获取视图 ID、名称和范围。
- **场景**：在对筛选视图进行 update / delete / update-criteria 等操作前，先用 list 获取可用的 filterViewId。
- **区分**：筛选视图（filter-view）是个人化的数据过滤方式，与全局筛选不同。每个用户可以创建自己的筛选视图，互不影响原始数据。如果没有筛选视图，返回空列表。

### 创建筛选视图
```
Usage:
  dws sheet filter-view create [flags]
Example:
  # 创建不带筛选条件的筛选视图
  dws sheet filter-view create --node <NODE_ID> --sheet-id <SHEET_ID> --name "我的视图" --range "A1:E10"

  # 创建带按值筛选条件的筛选视图
  dws sheet filter-view create --node <NODE_ID> --sheet-id <SHEET_ID> --name "销售筛选" --range "A1:E10" \
    --criteria '[{"column":0,"filterType":"values","visibleValues":["销售部"]}]'

  # 创建带按条件筛选的筛选视图（大于等于 200000）
  dws sheet filter-view create --node <NODE_ID> --sheet-id <SHEET_ID> --name "高预算" --range "A1:C10" \
    --criteria '[{"column":1,"filterType":"condition","conditions":[{"operator":"greater-equal","value":"200000"}]}]'
Flags:
      --node string       表格文档 ID 或 URL (必填)
      --sheet-id string   工作表 ID 或名称 (必填)
      --name string       筛选视图名称 (必填)
      --range string      筛选视图范围，A1 表示法，如 A1:E10 (必填)
      --criteria string   筛选条件，JSON 数组 (可选)
```

在指定工作表中创建一个筛选视图。
- **用途**：为指定数据区域创建一个可命名的个人化筛选视图，可选同时设置筛选条件。
- **场景**：用户需要针对某个数据区域建立固定的筛选视角（如"高绩效员工""研发部数据"），方便反复查看。
- **区分**：与全局筛选不同，筛选视图是个人化的，不影响其他用户看到的数据。如果只需创建视图不设条件，后续可通过 `update-criteria` 单独设置；如果要一步到位，可通过 `--criteria` 在创建时直接设置。
`--criteria` 为 JSON 数组，每个元素包含 `column`（列偏移量，从 0 开始）和筛选条件字段。支持三种筛选类型：
- `values`：按值筛选，通过 `visibleValues` 指定允许显示的值列表
- `condition`：按条件筛选，通过 `conditions` 指定条件列表（最多 2 个），每个条件包含 `operator` 和 `value`。支持的操作符（kebab-case）：`equal`、`not-equal`、`contains`、`not-contains`、`starts-with`、`not-starts-with`、`ends-with`、`not-ends-with`、`greater`、`greater-equal`、`less`、`less-equal`。多条件之间通过 `conditionOperator` 指定逻辑关系：`and`（且，默认）或 `or`（或）
- `color`：按颜色筛选，通过 `backgroundColor` 或 `fontColor` 指定颜色值（十六进制，如 `#FF0000`），二选一

### 更新筛选视图属性
```
Usage:
  dws sheet filter-view update [flags]
Example:
  # 更新筛选视图名称
  dws sheet filter-view update --node <NODE_ID> --sheet-id <SHEET_ID> --filter-view-id <FV_ID> --name "新名称"

  # 更新筛选视图范围
  dws sheet filter-view update --node <NODE_ID> --sheet-id <SHEET_ID> --filter-view-id <FV_ID> --range "A1:F20"

  # 更新筛选条件
  dws sheet filter-view update --node <NODE_ID> --sheet-id <SHEET_ID> --filter-view-id <FV_ID> \
    --criteria '[{"column":1,"filterType":"condition","conditions":[{"operator":"greater","value":"100"}]}]'
Flags:
      --node string             表格文档 ID 或 URL (必填)
      --sheet-id string         工作表 ID 或名称 (必填)
      --filter-view-id string   筛选视图 ID (必填)
      --name string             筛选视图新名称
      --range string            筛选视图新范围，A1 表示法
      --criteria string         筛选条件，JSON 数组
```

更新筛选视图的名称、范围和/或筛选条件，`--name`、`--range`、`--criteria` 至少传入一个。
- **用途**：修改已有筛选视图的名称、数据范围或筛选条件。
- **场景**：数据区域扩展后需要扩大筛选视图范围，或重命名视图，或通过 `--criteria` 一次性批量更新多列筛选条件。
- **区分**：`update` 可同时修改名称、范围和条件，适合批量更新；`update-criteria` 只能设置单列条件，适合精确控制某一列的筛选逻辑。`--criteria` 指定列的条件会被替换，未指定的列保持不变。

`--criteria` 为 JSON 数组，格式与 `filter-view create` 的 `--criteria` 相同，支持的筛选类型和操作符参见「创建筛选视图」说明。

### 删除筛选视图

> **CAUTION:** 不可逆操作 — 执行前必须向用户确认。

```
Usage:
  dws sheet filter-view delete [flags]
Example:
  dws sheet filter-view delete --node <NODE_ID> --sheet-id <SHEET_ID> --filter-view-id <FV_ID>
Flags:
      --node string             表格文档 ID 或 URL (必填)
      --sheet-id string         工作表 ID 或名称 (必填)
      --filter-view-id string   筛选视图 ID (必填)
```

删除指定的筛选视图。
- **用途**：永久删除一个不再需要的筛选视图及其所有筛选条件。
- **场景**：筛选视图已过时或不再需要时，清理无用的视图。
- **区分**：`delete` 删除整个筛选视图（包括所有列的条件），操作不可恢复；`delete-criteria` 只删除某一列的筛选条件，视图本身保留。此操作不影响全局筛选或其他筛选视图，也不影响原始数据。

### 更新筛选视图列条件
```
Usage:
  dws sheet filter-view update-criteria [flags]
Example:
  # 按值筛选：只显示"销售部"和"市场部"
  dws sheet filter-view update-criteria --node <NODE_ID> --sheet-id <SHEET_ID> --filter-view-id <FV_ID> \
    --column 0 --filter-criteria '{"filterType":"values","visibleValues":["销售部","市场部"]}'

  # 按条件筛选：大于 100
  dws sheet filter-view update-criteria --node <NODE_ID> --sheet-id <SHEET_ID> --filter-view-id <FV_ID> \
    --column 2 --filter-criteria '{"filterType":"condition","conditions":[{"operator":"greater","value":"100"}]}'

  # 按条件筛选：大于等于 200000
  dws sheet filter-view update-criteria --node <NODE_ID> --sheet-id <SHEET_ID> --filter-view-id <FV_ID> \
    --column 1 --filter-criteria '{"filterType":"condition","conditions":[{"operator":"greater-equal","value":"200000"}]}'

  # 按条件筛选：小于 100
  dws sheet filter-view update-criteria --node <NODE_ID> --sheet-id <SHEET_ID> --filter-view-id <FV_ID> \
    --column 1 --filter-criteria '{"filterType":"condition","conditions":[{"operator":"less","value":"100"}]}'

  # 多条件筛选：大于等于 60 且 小于等于 90
  dws sheet filter-view update-criteria --node <NODE_ID> --sheet-id <SHEET_ID> --filter-view-id <FV_ID> \
    --column 2 --filter-criteria '{"filterType":"condition","conditionOperator":"and","conditions":[{"operator":"greater-equal","value":"60"},{"operator":"less-equal","value":"90"}]}'

  # 按颜色筛选：背景色为红色
  dws sheet filter-view update-criteria --node <NODE_ID> --sheet-id <SHEET_ID> --filter-view-id <FV_ID> \
    --column 1 --filter-criteria '{"filterType":"color","backgroundColor":"#FF0000"}'
Flags:
      --node string              表格文档 ID 或 URL (必填)
      --sheet-id string          工作表 ID 或名称 (必填)
      --filter-view-id string    筛选视图 ID (必填)
      --column int               列偏移量，从 0 开始 (必填)
      --filter-criteria string   筛选条件，JSON 对象 (必填)
```

更新筛选视图中某一列的筛选条件。
- **用途**：为筛选视图的指定列创建或更新筛选条件，控制该列哪些数据行可见。
- **场景**：只显示某些特定值的行（如"只看研发部"）→ `filterType: values`；按数值条件筛选（如"绩效 ≥ 85"）→ `filterType: condition` + `operator: greater-equal`；按文本条件筛选（如"名称包含关键字"）→ `filterType: condition` + `operator: contains`。
- **区分**：`update-criteria` 精确控制单列条件，适合逐列设置不同的筛选逻辑；`filter-view update --criteria` 可以批量更新多列条件；`delete-criteria` 是 `update-criteria` 的逆操作，删除指定列的条件。

`--column` 为列偏移量（从 0 开始），相对于筛选视图范围首列。
例如筛选视图范围为 `B1:E10`，则 `--column 0` 代表 B 列，`--column 1` 代表 C 列。

`--filter-criteria` 为 JSON 对象，支持三种筛选类型：
- `values`：按值筛选，通过 `visibleValues` 指定允许显示的值列表
- `condition`：按条件筛选，通过 `conditions` 指定条件列表（最多 2 个），每个条件包含 `operator` 和 `value`。支持的操作符：`equal`、`not-equal`、`contains`、`not-contains`、`starts-with`、`not-starts-with`、`ends-with`、`not-ends-with`、`greater`、`greater-equal`、`less`、`less-equal`。多条件之间通过 `conditionOperator` 指定逻辑关系：`and`（且，默认）或 `or`（或）
- `color`：按颜色筛选，通过 `backgroundColor` 或 `fontColor` 指定颜色值（十六进制，如 `#FF0000`），二选一

### 删除筛选视图列条件
```
Usage:
  dws sheet filter-view delete-criteria [flags]
Example:
  # 删除第 1 列（A 列）的筛选条件
  dws sheet filter-view delete-criteria --node <NODE_ID> --sheet-id <SHEET_ID> --filter-view-id <FV_ID> --column 0

  # 删除第 3 列（C 列）的筛选条件
  dws sheet filter-view delete-criteria --node <NODE_ID> --sheet-id <SHEET_ID> --filter-view-id <FV_ID> --column 2
Flags:
      --node string             表格文档 ID 或 URL (必填)
      --sheet-id string         工作表 ID 或名称 (必填)
      --filter-view-id string   筛选视图 ID (必填)
      --column int              列偏移量，从 0 开始 (必填)
```

清除筛选视图中指定列的筛选条件。
- **用途**：移除筛选视图中指定列的筛选条件，使该列不再参与过滤。
- **场景**：之前通过 `update-criteria` 设置了某列的筛选条件，现在需要取消该列的筛选以显示全部数据。
- **区分**：`delete-criteria` 只清除指定列的条件，筛选视图本身和其他列的条件保持不变；`delete` 会删除整个筛选视图。如果指定列没有设置筛选条件，调用此命令不会报错（幂等操作）。

### 获取单个筛选视图详情
```
Usage:
  dws sheet filter-view info [flags]
Example:
  # 查看指定筛选视图的详情
  dws sheet filter-view info --node <NODE_ID> --sheet-id <SHEET_ID> --filter-view-id <FV_ID>
Flags:
      --node string             表格文档 ID 或 URL (必填)
      --sheet-id string         工作表 ID 或名称 (必填)
      --filter-view-id string   筛选视图 ID (必填)
```

获取指定筛选视图的完整信息，包括 ID、名称、范围和筛选条件。
- **用途**：查看某个筛选视图的当前配置，包括已设置的所有筛选条件详情。
- **场景**：在修改或删除筛选视图前，先确认其当前状态；或在 `update-criteria` 后验证条件是否生效。
- **区分**：`info` 返回单个视图的完整信息（含 criteria）；`list` 返回所有视图的列表概要。`info` 需要指定 `--filter-view-id`，ID 可通过 `list` 获取。
- **实现**：内部调用 `get_filter_views` 获取全部列表后按 ID 过滤。

### 列出筛选视图所有列条件
```
Usage:
  dws sheet filter-view list-criteria [flags]
Example:
  # 列出筛选视图的所有条件
  dws sheet filter-view list-criteria --node <NODE_ID> --sheet-id <SHEET_ID> --filter-view-id <FV_ID>
Flags:
      --node string             表格文档 ID 或 URL (必填)
      --sheet-id string         工作表 ID 或名称 (必填)
      --filter-view-id string   筛选视图 ID (必填)
```

列出指定筛选视图中已设置的所有列筛选条件。
- **用途**：查看某个筛选视图当前设置了哪些列的筛选条件，包括每列的条件类型和具体规则。
- **场景**：在管理筛选条件（修改/删除特定列条件）前，先了解当前视图有哪些条件；或排查筛选结果不符合预期时检查条件配置。
- **区分**：`list-criteria` 返回所有列的条件（按列偏移量为 key 的对象）；`get-criteria` 只返回指定列的条件。如果没有设置任何条件，返回空对象 `{}`。
- **实现**：内部调用 `get_filter_views` 获取视图详情后提取 `criteria` 字段。

### 获取单列筛选条件
```
Usage:
  dws sheet filter-view get-criteria [flags]
Example:
  # 查看第 1 列（偏移量 0）的筛选条件
  dws sheet filter-view get-criteria --node <NODE_ID> --sheet-id <SHEET_ID> --filter-view-id <FV_ID> --column 0

  # 查看第 3 列（偏移量 2）的筛选条件
  dws sheet filter-view get-criteria --node <NODE_ID> --sheet-id <SHEET_ID> --filter-view-id <FV_ID> --column 2
Flags:
      --node string             表格文档 ID 或 URL (必填)
      --sheet-id string         工作表 ID 或名称 (必填)
      --filter-view-id string   筛选视图 ID (必填)
      --column int              列偏移量，从 0 开始 (必填)
```

获取指定筛选视图中某一列的筛选条件详情。
- **用途**：查看某个筛选视图中指定列当前设置的筛选条件，包括条件类型、运算符和比较值。
- **场景**：在修改某列条件前，先查看其当前配置；或验证 `update-criteria` 后该列条件是否正确。
- **区分**：`get-criteria` 只返回指定列的条件；`list-criteria` 返回所有列的条件。`--column` 为列偏移量（从 0 开始），相对于筛选视图范围首列。
- **实现**：内部调用 `get_filter_views` 获取视图详情后按列偏移量过滤 `criteria` 中的对应条件。

## 核心工作流

```bash
# ── 工作流 11: 筛选视图管理 ──

# 1. 获取工作表列表
dws sheet list --node <NODE_ID> -f json

# 2. 查看已有筛选视图
dws sheet filter-view list --node <NODE_ID> --sheet-id <SHEET_ID> -f json

# 3. 创建筛选视图（不带条件）
dws sheet filter-view create --node <NODE_ID> --sheet-id <SHEET_ID> \
  --name "我的筛选" --range "A1:E100" -f json

# 4. 为筛选视图设置列条件（按值筛选）
dws sheet filter-view update-criteria --node <NODE_ID> --sheet-id <SHEET_ID> --filter-view-id <FV_ID> \
  --column 0 --filter-criteria '{"filterType":"values","visibleValues":["销售部","市场部"]}' -f json

# 5. 为筛选视图设置列条件（按条件筛选）
dws sheet filter-view update-criteria --node <NODE_ID> --sheet-id <SHEET_ID> --filter-view-id <FV_ID> \
  --column 2 --filter-criteria '{"filterType":"condition","conditions":[{"operator":"greater","value":"100"}]}' -f json

# 6. 更新筛选视图名称和范围
dws sheet filter-view update --node <NODE_ID> --sheet-id <SHEET_ID> --filter-view-id <FV_ID> \
  --name "销售数据筛选" --range "A1:F200" -f json

# 7. 清除某列的筛选条件
dws sheet filter-view delete-criteria --node <NODE_ID> --sheet-id <SHEET_ID> --filter-view-id <FV_ID> \
  --column 0 -f json

# 8. 删除筛选视图
dws sheet filter-view delete --node <NODE_ID> --sheet-id <SHEET_ID> --filter-view-id <FV_ID> -f json
```

```bash
# ── 工作流 11b: 创建带条件的筛选视图（一步完成） ──

# 创建筛选视图时直接指定筛选条件
dws sheet filter-view create --node <NODE_ID> --sheet-id <SHEET_ID> \
  --name "高销售额视图" --range "A1:E100" \
  --criteria '[{"column":0,"filterType":"values","visibleValues":["销售部"]},{"column":2,"filterType":"condition","conditions":[{"operator":"greater","value":"50000"}]}]' \
  -f json
```

## 上下文传递

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `filter-view list` | `filterViews` 筛选视图列表（含 `id`、`name`、`range`） | 获取 filterViewId 用于 info / update / delete / update-criteria / delete-criteria / list-criteria / get-criteria |
| `filter-view info` | `id`、`name`、`range`、`criteria` | 查看单个视图完整配置，确认条件是否生效 |
| `filter-view create` | `id` 筛选视图 ID、`name`、`range` | 用于后续 update / delete / update-criteria / delete-criteria 的 --filter-view-id |
| `filter-view update` | `id`、`name`、`range`、`criteria` | 确认更新结果 |
| `filter-view delete` | `id` 被删除的筛选视图 ID | 确认删除完成 |
| `filter-view update-criteria` | `id` 筛选视图 ID | 确认条件设置完成 |
| `filter-view delete-criteria` | `id` 筛选视图 ID | 确认条件清除完成 |
| `filter-view list-criteria` | 所有列条件（按列偏移量为 key 的对象） | 了解当前视图已设置哪些列的条件 |
| `filter-view get-criteria` | 指定列的条件详情（`filterType`、`conditions` 等） | 查看某列的具体筛选规则 |
| `list` | 工作表的 `sheetId` | info / range read / range update / find 的 --sheet-id |

## 注意事项

- ★ **`--sheet-id` 获取规范（强制）**：`sheetId` 未知时必须先通过 `dws sheet list --node <NODE_ID> --format json` 查询，禁止凭空编造（如臆测为 `Sheet1`、`sheet1`、`0`、`default` 等）
- ★ **全局筛选（filter）与筛选视图（filter-view）的区别**：全局筛选影响所有协作者看到的数据展示，每个工作表最多一个；筛选视图是个人化的，互不影响。用户只说"筛选"时默认走 `filter` 系列
- `filter-view list` 获取指定工作表的所有筛选视图列表，返回的 `id` 可用于后续 info / update / delete / update-criteria / delete-criteria / list-criteria / get-criteria 的 `--filter-view-id`
- `filter-view info` 获取单个筛选视图的完整信息（含 criteria），内部复用 `get_filter_views` MCP 按 ID 过滤
- `filter-view list-criteria` 列出指定筛选视图已设置的所有列条件，返回按列偏移量为 key 的对象；无条件时返回空对象 `{}`
- `filter-view get-criteria` 获取指定列的条件详情，`--column` 为列偏移量（从 0 开始）；该列无条件时返回错误提示
- `filter-view create` 创建筛选视图时 `--range` 应包含表头行。`--criteria` 可选，不传则创建后无筛选条件，后续可通过 `filter-view update-criteria` 设置
- `filter-view update` 的 `--name`、`--range`、`--criteria` 至少需要传入一个，未指定的字段保持不变
- `filter-view update` 的 `--criteria` 中指定列的条件会被替换，未指定的列保持不变
- `filter-view delete` 删除后该视图及其所有筛选条件将被永久移除，不可恢复
- `filter-view delete` 不影响全局筛选或其他筛选视图
- `filter-view update-criteria` 的 `--column` 为列偏移量（从 0 开始），相对于筛选视图范围首列。例如筛选视图范围为 `B1:E10`，则 `--column 0` 代表 B 列
- `filter-view update-criteria` 设置条件后立即在该筛选视图中生效，仅影响当前视图，不影响全局筛选或其他筛选视图
- `filter-view update-criteria` 的 `--filter-criteria` 中 `conditions` 最多 2 个条件，多条件之间通过 `conditionOperator` 指定逻辑关系（`and` 或 `or`）
- `filter-view delete-criteria` 仅清除指定列的条件，不会删除整个筛选视图。如需删除整个筛选视图，请使用 `filter-view delete`
- `filter-view delete-criteria` 如果指定列没有设置筛选条件，调用不会报错
- 筛选视图相关操作需要"可阅读"权限（list / info / list-criteria / get-criteria）或"可编辑"权限（create / update / delete / update-criteria / delete-criteria），不支持跨组织操作
