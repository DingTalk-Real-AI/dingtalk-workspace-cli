# 数据读取

## 使用场景

用户说"读数据/看表格内容":
- 需要列名、二维数据、dtype 和 number format，或准备与 `table-put` 配套处理 → `table-get`
- 快速查看纯值数据、批量处理、大表分批读 → `csv-get`（token 消耗低，防爆保护）
- 需要结构化信息（值+样式+数据验证+富文本+单元格级超链接）、查看公式或原始值 → `range read`
- 需要查看合并单元格 / 表头合并结构 → `sheet info`，读取返回的 `mergedRanges`；不要在 `csv-get` 或 `range read` 里找合并信息

## 命令选择

| 读取目的 | 推荐命令 | 说明 |
|---------|---------|------|
| DataFrame 风格结构化读取 | `table-get` | 返回 columns / data / dtypes / formats，可与 table-put 配套 |
| 快速查看纯值、数据分析、大表分批读取 | `csv-get` | CSV 格式，token 消耗约为 JSON 的 1/3，内置 maxChars 防爆 |
| 查看数据验证配置（下拉/复选框） | `range read` | 返回 per-cell 结构，含 dataValidation |
| 查看单元格样式（背景色/字体/对齐等） | `range read` | 返回 per-cell 结构，含 cellStyles（仅显式设置的样式） |
| 查看单元格级超链接 | `range read` | 返回 per-cell 结构，含 hyperlink；富文本片段链接仍在 richText 内 |
| 查看公式文本 | `range read --value-render-option formula` | value 返回公式 |
| 获取原始值（数字/布尔而非格式化字符串） | `range read --value-render-option raw_value` | value 返回原始类型 |
| 查看合并单元格范围 | `sheet info` | 返回 `mergedRanges`，这是工作表结构信息，不属于单元格值读取 |

## 命令详细参考

### 读取结构化 table 数据
```
Usage:
  dws sheet table-get [flags]     # 别名: dws sheet table-read
Example:
  dws sheet table-get --node <NODE_ID>
  dws sheet table-get --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:D20"
  dws sheet table-get --node <NODE_ID> --sheet-id <SHEET_ID> --no-header
Flags:
      --node string       表格文档 ID 或 URL (必填)
      --sheet-id string   工作表 ID 或名称
      --range string      读取范围，A1 表示法；可带 sheet 前缀
      --no-header         首行不作为表头，自动生成 col1/col2/...
```

`table-get` 面向结构化数据处理，返回 `columns`、二维 `data`、`dtypes` 和可用时的 `formats`。未传 `--sheet-id` / `--range` 时由服务端读取目标文档中的 table 数据；大表仍应通过 `--range` 限定范围。需要单元格样式、公式、数据验证或富文本细节时，继续使用 `range read`。

### 以 CSV 格式读取工作表数据（推荐）
```
Usage:
  dws sheet csv-get [flags]
Example:
  dws sheet csv-get --node <NODE_ID>
  dws sheet csv-get --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:D10"
  dws sheet csv-get --node <NODE_ID> --range "A1:Z500" --value-render-option raw_value
  dws sheet csv-get --node <NODE_ID> --range "A1:D10" --max-chars 50000
Flags:
      --node string                  表格文档 ID 或 URL (必填)
      --sheet-id string              工作表 ID 或名称 (不传则默认第一个工作表)
      --range string                 读取范围，A1 表示法 (不传则读取全部非空数据)
      --value-render-option string   取值模式: formatted_value(默认) | raw_value | formula
      --max-chars int                CSV 最大字符数 (默认 200000，超出截断)
```

**返回字段说明**：
- `csv` — CSV 文本，每逻辑行前加 `[row=N]` 前缀标注真实表格行号。行号一律从此前缀读取，禁止手算
- `colIndices` — 列字母映射数组（如 `["A","B","C"]`）。定位列字母用 `colIndices[j]`，禁止手数逗号
- `rowIndices` — 行号映射数组（如 `[1,2,3]`）
- `hasMore` — 是否因 maxChars 截断。为 true 时需要调整 `--range` 继续分页读取

`csv-get` 不返回合并单元格结构。若 CSV 中出现合并区域的非左上角单元格为空，不能据此判断该区域"无内容"；需要先用 `dws sheet info --node <NODE_ID> --sheet-id <SHEET_ID> --format json` 读取 `mergedRanges`，再结合左上角单元格理解合并区域语义。

**取值模式说明**：
| 模式 | 返回内容 | 适用场景 |
|------|---------|---------|
| `formatted_value` | 格式化展示值（如 ¥1,000.00、2025-06-01） | 只看数据 |
| `raw_value` | 原始值（如 1000、45808） | 数据处理、计算 |
| `formula` | 公式文本（如 =SUM(A1:A10)），无公式时回退原始值 | 查看/复制公式 |

**大表分批读取**：当 `hasMore=true` 或数据量很大时，按行窗口分批：
- 先通过 `info` 获取 `nonEmptyRange.range`，或用 `nonEmptyRange.lastRow` / `nonEmptyRange.lastColumn` 确定 A1 边界
- 分批读取：`--range "A1:J500"`、`--range "A501:J1000"` ……
- 单次建议 ≤5000 单元格

### 读取工作表数据（per-cell 结构化信息）
```
Usage:
  dws sheet range read [flags]     # 别名: dws sheet range get
Example:
  dws sheet range read --node <NODE_ID>
  dws sheet range read --node <NODE_ID> --sheet-id <SHEET_ID>
  dws sheet range read --node <NODE_ID> --sheet-id "Sheet1" --range "A1:D10"
  dws sheet range read --node <NODE_ID> --range "Sheet1!A1:D10"
  dws sheet range read --node <NODE_ID> --value-render-option raw_value
  dws sheet range read --node <NODE_ID> --value-render-option formula

  # 使用 get 别名，与 read 等价
  dws sheet range get --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:D10"
Flags:
      --node string                  表格文档 ID 或 URL (必填)
      --sheet-id string              工作表 ID 或名称 (不传则默认第一个工作表)
      --range string                 读取范围，A1 表示法 (如 A1:D10，不传则读取全部数据)
      --value-render-option string   取值模式: formatted_value(默认) | raw_value | formula
```

**返回字段说明**：
- `cells` — 二维数组，第一维为行，第二维为列。每个元素为 per-cell 对象，字段如下：

| 字段 | 类型 | 是否必有 | 说明 |
|------|------|---------|------|
| `value` | string / number / boolean / null | 始终存在 | 单元格值。`formatted_value` 模式为 string；`raw_value` 模式为原始类型（number/boolean/string/null）；`formula` 模式为公式字符串或回退原始值 |
| `dataValidation` | object | 仅有数据验证时出现，无则省略 | 数据验证配置，见下表 |
| `hyperlink` | object | 仅有单元格级超链接时出现，无则省略 | 整格超链接，结构为 `{type, link, text?}`，见下表 |
| `richText` | object | 仅富文本单元格出现 | 富文本结构（含超链接、附件、图片、样式片段等），普通纯文本不含此字段 |
| `cellStyles` | object | 仅有显式设置的样式时出现；MCP 序列化层也可能返回全 null 空壳 | cell-level 样式，见下表。读取时只看非 null 字段；全 null 等同不存在 |

`range read` / `range get` 不返回合并单元格结构。要看合并单元格，请先或另行调用 `dws sheet info --node <NODE_ID> --sheet-id <SHEET_ID> --format json`，使用其中的 `mergedRanges`。

**dataValidation 结构**：

| type | 字段 | 说明 |
|------|------|------|
| `dropdown` | `options: [{value: string, color?: string}]` | 下拉选项列表 |
| `dropdown` | `enableMultiSelect: boolean` | 是否允许多选 |
| `checkbox` | `checked: boolean` | 当前勾选状态 |

**hyperlink 结构**：

| type | 字段 | 说明 |
|------|------|------|
| `path` | `link` + 可选 `text` | 外部 URL 链接 |
| `sheet` | `link` + 可选 `text` | 工作表链接，`link` 为工作表 ID 或名称 |
| `range` | `link` + 可选 `text` | 单元格范围链接，`link` 为 A1 表示法，如 `Sheet1!A4` |

**richText 结构**：

`richText` 表示单元格内的富文本片段，常见结构为 `{type:"richText", texts:[...]}`。`texts` 数组内每个子项代表一个片段：

| 子项 type | 常见字段 | 说明 |
|-----------|----------|------|
| `text` | `text` / `style` | 普通文本片段；`style` 是片段级样式 |
| `link` | `text` / `link` / `subType` / `style` | 富文本片段链接。`subType` 不存在时按 `path` 理解；`path` 表示外部 URL，`sheet` 表示工作表链接，`range` 表示单元格范围链接 |
| `attachment` | `text` / `resourceId` / `mimeType` / `size` | 附件片段 |
| `image` | `resourceId` / `resourceUrl` / `width` / `height` | 图片片段 |

`richText.texts[].link.subType` 与 cell-level `hyperlink.type` 含义一致，但作用范围不同：`hyperlink` 是整个单元格可点击，richText `link` 只作用于该文本片段。读取到 `subType:"sheet"` 时，`link` 通常是真实工作表名称；读取到 `subType:"range"` 时，`link` 通常是 A1 范围（如 `Sheet2!A1:B20`）。不要把 richText 片段链接误当成整格 `hyperlink`。

**cellStyles 字段说明**（仅关注显式设置过的非 null 属性；未设置属性不存在或为 null，应忽略）：

| 字段 | 类型 | 说明 |
|------|------|------|
| `fontWeight` | string | `bold` / `normal` |
| `fontColor` | string | 字体颜色，`#RRGGBB` |
| `fontSize` | number | 字号 |
| `fontStyle` | string | `italic` / `normal` |
| `backgroundColor` | string | 背景色，`#RRGGBB` |
| `horizontalAlignment` | string | `left` / `center` / `right` / `general` |
| `verticalAlignment` | string | `top` / `middle` / `bottom` |
| `wordWrap` | string | `overflow` / `clip` / `autoWrap` |
| `numberFormat` | string | 数字格式代码，如 `@`、`#,##0.00`、`yyyy/m/d`；`@` 表示文本 |
| `textUnderline` | boolean | 下划线 |
| `textLineThrough` | boolean | 删除线 |

**返回示例**：
```json
{
  "cells": [
    [
      {"value": "姓名", "cellStyles": {"fontWeight": "bold", "backgroundColor": "#FFF2CC"}},
      {"value": "状态", "cellStyles": {"fontWeight": "bold"}, "dataValidation": {"type": "dropdown", "options": [{"value": "进行中"}, {"value": "已完成", "color": "#52C41A"}], "enableMultiSelect": false}}
    ],
    [
      {"value": "张三"},
      {"value": "钉钉", "hyperlink": {"type": "path", "link": "https://dingtalk.com", "text": "钉钉"}}
    ]
  ],
  "message": "Successfully retrieved cell data.",
  "success": true
}
```

说明：第一行表头有 `cellStyles`（加粗 + 背景色），第二行第二格有单元格级 `hyperlink`。注意：MCP 平台序列化会将未设置的字段填充为 null（如 `"fontStyle": null`），读取时应忽略值为 null 的字段，仅关注非 null 的属性；如果 `cellStyles` 全字段都是 null，视同不存在。`richText` 字段同理——无富文本的普通单元格可能返回 `{"type": null, "texts": null}`，视同不存在。

**取值模式说明**：
| 模式 | value 返回内容 | 适用场景 |
|------|---------|---------|
| `formatted_value` | 格式化展示值（如 ¥1,000.00、2025-06-01） | 只看数据（默认） |
| `raw_value` | 原始值（如 1000、45808） | 数据处理、计算 |
| `formula` | 公式文本（如 =SUM(A1:A10)），无公式时回退原始值 | 查看/复制公式 |

**超时处理建议**：读取大范围数据时若出现超时或响应过慢，请主动缩小 `--range` 查询范围，**建议单次读取的单元格数量控制在 5000 个以内**（例如 50 行 × 100 列、100 行 × 50 列）。对于大表可采用分页读取策略：
- 先通过 `info` 获取 `nonEmptyRange.range`，或用 `nonEmptyRange.lastRow` / `nonEmptyRange.lastColumn` 确定 A1 边界
- 按行分批读取，如 `A1:J500`、`A501:J1000`、`A1001:J1500` ……
- 避免不传 `--range` 直接读取整个大工作表

## 核心工作流

```bash
# ── 工作流: 读取已有表格数据 ──

# 1. 获取工作表列表
dws sheet list --node <NODE_ID> --format json

# 2. 查看工作表详情（行列数、最后非空位置、mergedRanges 等）
dws sheet info --node <NODE_ID> --sheet-id <SHEET_ID> --format json

# 3. 读取全部数据
dws sheet range read --node <NODE_ID> --sheet-id <SHEET_ID> --format json

# 4. 读取指定区域
dws sheet range read --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:D10" --format json
```

## 上下文传递

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `list` | 工作表的 `sheetId` | info / range read 的 --sheet-id |
| `info` | `rowCount` / `nonEmptyRange.range` / `nonEmptyRange.lastRow` / `nonEmptyRange.lastColumn` / `mergedRanges` | 确定数据范围、分页读取边界、识别合并单元格结构 |

## 注意事项

- ★ **`--sheet-id` 获取规范（强制）**：`sheetId` 未知时必须先通过 `dws sheet list --node <NODE_ID> --format json` 查询真实的 `sheetId` / 工作表名称后再调用，禁止凭空编造（如臆测为 `Sheet1`、`sheet1`、`0`、`default` 等）；用户仅给出工作表名称时，也应通过 `list` 校验该名称是否存在，避免名称大小写或拼写不一致导致失败
- `range read` 不传 `--range` 时默认读取整个工作表的全部非空数据
- `range read` 的 `--range` 支持 `Sheet1!A1:D10` 格式直接指定工作表（此时忽略 `--sheet-id`）
- ★ `csv-get` / `range read` / `range get` 不返回合并单元格结构；查看合并范围必须用 `sheet info` 的 `mergedRanges`
- `range read` 遇到超时或响应过慢时，应缩小 `--range` 查询范围，**单次读取的单元格数量建议控制在 5000 个以内**；数据量较大时通过 `info` 获取边界后分批读取，避免不传 `--range` 直接读取整个大工作表
- ★ 当用户要求搜索/查找表格数据时，使用 `find` 命令，不要用 `range read` 读取全量数据后自行过滤——`find` 支持服务端搜索，效率更高、语义更准确
