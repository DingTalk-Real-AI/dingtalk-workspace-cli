# 单元格格式与合并 (style & format)

## 三种样式设置方式

钉钉表格支持三种样式设置方式，适用不同场景：

| 方式 | 命令 / 字段 | 适用场景 | 粒度 |
|------|------------|---------|------|
| **`set-style` / `batch-set-style`** | `dws sheet range set-style` | 批量刷整片区域的统一样式（表头加粗居中、数字格式等） | range 级别（2D 数组或全 range 统一值） |
| **`cellStyles`**（`range update` 内） | `--values` 中每个 cell 的 `cellStyles` 字段 | 写值同时附带样式，少量 cell 一步到位 | per-cell 级别 |
| **`style`**（richText 片段样式） | `--values` 中 richText 子项（`text`/`link`）的 `style` | 同一单元格内不同文字有不同字体样式 | 文本片段级别 |

选择建议：
- 只设样式不改值 → `set-style` / `batch-set-style`
- 写值 + 样式一步到位（少量 cell） → `range update` + `cellStyles`
- 文本内部分段样式（"重要"红色加粗，其余正常） → `range update` + `type:"richText"` 子项 `style`
- 大面积统一样式 → `set-style`（单值刷 range）或 `batch-set-style`（多 range 批量）

注意：`set-style` / `batch-set-style` 和 `range update` 的 `cellStyles` 最终都作用于 cell-level 样式，效果相同。区别在于调用方式——前者是独立命令，后者嵌在写值调用中。`range read` 返回的 `cellStyles` 字段能读回所有显式设置过的 cell-level 样式，无论是通过哪种方式设置的。

`type:"text"` 顶层旧 `style` 字段不要作为新写法使用；整格样式用 `cellStyles`，分段样式才用 richText 子项 `style`。

## 使用场景

### 单元格格式

用户说"设置样式/改颜色/设背景色/加粗/居中/换行/字体颜色/字号":
- 仅设样式不改值 → `range set-style`
- 批量设置不同 range 的样式 → `range batch-set-style --batch ./styles.json`（内部顺序循环调 `update_range`）
- 写值同时附带样式 → `range update --values` 中使用 `cellStyles` 字段（参见 sheet-write-data.md）
- 请勿用 `range update --values` 写空/重写来模拟纯样式变更

用户说"设置数字格式/改成百分比/用人民币显示/按日期显示/文本格式/保留几位小数":
- 批量设置数字格式 → `range set-style --number-format <格式代码>`（如 `0%` / `"¥"#,##0.00` / `yyyy/m/d` / `@`）
- 写值时顺带设置数字格式 → `range update` 中 `cellStyles.numberFormat`

用户说"合并单元格/合并/合并区域/按行合并/按列合并":
- 合并所有单元格 → `merge-cells`（默认 mergeAll）
- 按行合并 → `merge-cells --merge-type mergeRows`
- 按列合并 → `merge-cells --merge-type mergeColumns`

用户说"取消合并/拆分单元格/还原合并":
- 取消合并单元格 → `unmerge-cells`

## 命令详细参考

### 设置单元格样式
```
Usage:
  dws sheet range set-style [flags]
Example:
  # 给 A1:B3 打上黄底粗体居中
  dws sheet range set-style --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:B3" \
    --bg-color "#FFF2CC" --font-weight bold --h-align center

  # 给 C1:C5 逐单元格设置不同背景色
  dws sheet range set-style --node <NODE_ID> --sheet-id <SHEET_ID> --range "C1:C5" \
    --bg-colors-json '[["#FF0000"],["#00FF00"],["#0000FF"],["#FFFF00"],["#FF00FF"]]'

  # 整片 range 启用自动换行
  dws sheet range set-style --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:E10" --word-wrap autoWrap
Flags:
      --node string                 表格文档 ID 或 URL (必填)
      --sheet-id string             工作表 ID 或名称 (必填)
      --range string                目标区域，如 A1:B3 (必填)
      --bg-color string             背景色（#RRGGBB），一键刷整个 range；与 --bg-colors-json 二选一
      --bg-colors-json string       背景色二维 JSON 数组，维度需与 --range 一致
      --font-size int               字号，一键刷整个 range；与 --font-sizes-json 二选一
      --font-sizes-json string      字号二维 JSON 数组
      --h-align string              水平对齐：left/center/right/general
      --h-aligns-json string        水平对齐二维 JSON 数组
      --v-align string              垂直对齐：top/middle/bottom
      --v-aligns-json string        垂直对齐二维 JSON 数组
      --font-color string           字体颜色（#RRGGBB）
      --font-colors-json string     字体颜色二维 JSON 数组
      --font-weight string          字体粗细：bold/normal
      --font-weights-json string    字体粗细二维 JSON 数组
      --word-wrap string            换行方式：overflow/clip/autoWrap（整个 range 共用）
      --number-format string        数字格式代码，如 General/@/#,##0/#,##0.00/0%/0.00%/yyyy/m/d/h:mm:ss
```

**特性说明**：
- 每个样式维度提供两种写法，二选一：`--xxx`（单值刷整个 range，CLI 本地展开为二维数组）vs `--xxx-json`（逐单元格指定，维度需与 `--range` 完全一致）
- 至少需传入一个样式参数。单次调用建议：行数 ≤ 1000，单元格总数 ≤ 5000
- 枚举值按驼峰书写：`autoWrap`、`bold`、`normal`、`center` 等

### 批量设置单元格样式
```
Usage:
  dws sheet range batch-set-style [flags]
Example:
  dws sheet range batch-set-style --node <NODE_ID> --batch ./styles.json
  dws sheet range batch-set-style --node <NODE_ID> --batch ./styles.json --continue-on-error
Flags:
      --node string               表格文档 ID 或 URL (必填)
      --batch string              批次配置 JSON 文件路径 (必填)
      --continue-on-error         遇到失败时继续执行后续条目（默认遇错即停）
```

配置文件格式（JSON 数组，每个元素一条批次项）：
```json
[
  {
    "sheetId": "Sheet1",
    "range":   "A1:B3",
    "bgColor":      "#FFF2CC",
    "fontSize":     12,
    "hAlign":       "center",
    "vAlign":       "middle",
    "fontColor":    "#333333",
    "fontWeight":   "bold",
    "wordWrap":     "autoWrap",
    "numberFormat": "General"
  },
  {
    "sheetId": "Sheet1",
    "range":   "C1:C5",
    "bgColorsJson": "[[\"#FF0000\"],[\"#00FF00\"],[\"#0000FF\"],[\"#FFFF00\"],[\"#FF00FF\"]]"
  }
]
```

**特性说明**：
- CLI 侧顺序循环逐条调用 `update_range`（非服务端批量），运行时输出 `[N/M]` 进度
- 每条记录执行与 `set-style` 一致的校验：至少一项样式字段 + rows ≤ 1000 + rows×cols ≤ 30000 + 枚举合法
- 默认遇错即停（返回非 0），`--continue-on-error` 时所有条目跑完再返回首个错误

### 合并单元格
```
Usage:
  dws sheet merge-cells [flags]
Example:
  # 合并所有单元格（默认）
  dws sheet merge-cells --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:B3"

  # 按行合并
  dws sheet merge-cells --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:C3" --merge-type mergeRows

  # 按列合并
  dws sheet merge-cells --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:C3" --merge-type mergeColumns

  # 使用带工作表前缀的范围（忽略 --sheet-id）
  dws sheet merge-cells --node <NODE_ID> --sheet-id <SHEET_ID> --range "Sheet1!A1:B3"
Flags:
      --node string         表格文档 ID 或 URL (必填)
      --sheet-id string     工作表 ID 或名称 (必填)
      --range string        目标单元格区域地址，如 A1:B3 (必填)
      --merge-type string   合并方式: mergeAll(默认)/mergeRows/mergeColumns
```

支持三种合并方式：
- `mergeAll`（默认）：合并所有单元格，将选定区域内的所有单元格合并成一个
- `mergeRows`：按行合并，在选定区域内将同一行相邻的单元格合并
- `mergeColumns`：按列合并，在选定区域内将同一列相邻的单元格合并

注意：合并时只保留左上角单元格的值，其他单元格的值会被丢弃。
`--range` 支持带工作表前缀的写法（如 `Sheet1!A1:B3`），此时将优先使用前缀解析出的工作表，忽略 `--sheet-id`。
合并完成后，可通过 `dws sheet info --node <NODE_ID> --sheet-id <SHEET_ID> --format json` 查看 `mergedRanges` 验证合并结构。

### 取消合并单元格
```
Usage:
  dws sheet unmerge-cells [flags]
Example:
  dws sheet unmerge-cells --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:D5"
Flags:
      --node string       表格文档 ID 或 URL (必填)
      --sheet-id string   工作表 ID 或名称 (必填)
      --range string      取消合并的范围，A1 表示法 (必填)
```

取消指定范围内所有合并的单元格，恢复为独立单元格。

## number-format 格式 code

适用范围：`number-format` 在 `range set-style` / `range batch-set-style` 中接受（CLI 对应 `--number-format`，batch 配置文件对应 `numberFormat`）。`range update` 没有 `--number-format` 参数，但可在写入值时通过每个 cell 的 `cellStyles.numberFormat` 设置同样的数字格式。

商品 ID、规格 ID、SKU、订单号、手机号、工号等数字形态标识符，使用文本格式 code：`@`。

常用格式：

| 格式类型 | 推荐 code | 展示示例 | 适用场景 |
| --- | --- | --- | --- |
| 常规 | `General` | `1234` / `普通文本` | 普通文本/数字展示 |
| 文本 | `@` | `528545015680` | 商品 ID、规格 ID、SKU、订单号、手机号、工号 |
| 整数 | `0` | `1235` | 数量、计数 |
| 两位小数 | `0.00` | `1234.50` | 单价、评分 |
| 整数千分位 | `#,##0` | `1,235` | 数量、金额整数 |
| 千分位两位小数 | `#,##0.00` | `1,234.50` | 金额、单价 |
| 百分比 | `0%` / `0.00%` | `85%` / `85.00%` | 转化率、占比 |
| 日期 | `yyyy/m/d` | `2026/3/15` | 日期列 |
| 日期时间 | `yyyy/m/d h:mm` | `2026/3/15 14:30` | 日期时间列 |
| 时间 | `h:mm` / `h:mm:ss` | `14:30` / `14:30:05` | 时间列 |
| 科学计数法 | `0.00E+00` / `##0.0E+0` | `1.23E+05` | 科学数据 |
| 人民币 | `"¥"#,##0_);("¥"#,##0)` / `"¥"#,##0.00_);("¥"#,##0.00)` | `¥1,235` / `¥1,234.50` | 金额列 |
| 美元 | `$#,##0_);($#,##0)` / `$#,##0.00_);($#,##0.00)` | `$1,235` / `$1,234.50` | 金额列 |

选择规则：没有特殊展示要求时，优先使用上面的常用格式。只有用户明确要求负数显示方式、中文日期、12 小时制、累计时长、分数或会计格式时，再选择下面的可选变体。

可选变体：

| 用户要求 | 推荐 code | 推荐展示示例 | 可选 code（差异） |
| --- | --- | --- | --- |
| 负数用括号显示 | `#,##0 ;(#,##0)` | `(1,235)` | `#,##0.00;(#,##0.00)`：保留两位小数，如 `(1,234.50)` |
| 负数标红显示 | `#,##0 ;[red](#,##0)` | 红色 `(1,235)` | `#,##0.00;[red](#,##0.00)`：保留两位小数，如红色 `(1,234.50)` |
| 分数 | `# ?/?` | `1 1/2` | `# ??/??`：分母最多两位，如 `1 23/32` |
| 英文月份日期 | `d-mmm-yy` | `15-Mar-26` | `d-mmm`：省略年份，如 `15-Mar`；`mmm-yy`：只显示月年，如 `Mar-26` |
| 中文日期 | `yyyy"年"m"月"d"日"` | `2026年3月15日` | `yyyy"年"m"月"`：只显示年月，如 `2026年3月`；`m"月"d"日"`：只显示月日，如 `3月15日` |
| 12 小时制时间 | `h:mm AM/PM` | `2:30 PM` | `h:mm:ss AM/PM`：显示秒，如 `2:30:05 PM` |
| 中文上午/下午时间 | `上午/下午 h"时"mm"分"` | `下午 2时30分` | `上午/下午 h"时"mm"分"ss"秒"`：显示秒，如 `下午 2时30分05秒` |
| 分秒/累计时长 | `mm:ss` | `05:30` | `[h]:mm:ss`：累计小时，如 `27:05:30`；`mm:ss.0`：显示十分之一秒，如 `05:30.5` |
| 人民币负数标红 | `"¥"#,##0_);[red]("¥"#,##0)` | 红色 `(¥1,235)` | `"¥"#,##0.00_);[red]("¥"#,##0.00)`：保留两位小数，如红色 `(¥1,234.50)` |
| 美元负数标红 | `$#,##0_);[Red]($#,##0)` | 红色 `($1,235)` | `$#,##0.00_);[Red]($#,##0.00)`：保留两位小数，如红色 `($1,234.50)` |
| 会计数字 | `_(* #,##0_);_(* (#,##0);_(* "-"_);_(@_)` | `1,235`，零值显示 `-` | `_(* #,##0.00_);_(* (#,##0.00);_(* "-"??_);_(@_)`：保留两位小数，如 `1,234.50` |
| 人民币会计格式 | `_("¥"* #,##0_);_("¥"* (#,##0);_("¥"*"-"_);_(@_)` | `¥ 1,235`，零值显示 `¥ -` | `_("¥"* #,##0.00_);_("¥"*(#,##0.00);_("¥"* "-"??_);_(@_)`：保留两位小数，如 `¥ 1,234.50` |

## 核心工作流

```bash
# ── 工作流 4: 写入数据并设置样式 ──

# 1. 写入数据（每个单元格必须是 object；数字也写成字符串）
dws sheet range update --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:C3" \
  --values '[[{"type":"text","text":"商品"},{"type":"text","text":"单价"},{"type":"text","text":"数量"}],[{"type":"text","text":"苹果"},{"type":"text","text":"5.5"},{"type":"text","text":"100"}],[{"type":"text","text":"香蕉"},{"type":"text","text":"3.2"},{"type":"text","text":"200"}]]' --format json

# 2. 设置数字格式（人民币）
dws sheet range set-style --node <NODE_ID> --sheet-id <SHEET_ID> --range "B2:B3" \
  --number-format '"¥"#,##0.00' --format json

# 3. 商品 ID / 规格 ID 按文本展示，避免科学计数法
dws sheet range set-style --node <NODE_ID> --sheet-id <SHEET_ID> --range "A2:A3" \
  --number-format "@" --format json

# 4. 写入单元格级超链接
dws sheet range update --node <NODE_ID> --sheet-id <SHEET_ID> --range "D1" \
  --values '[[{"type":"text","text":"详情","hyperlink":{"type":"path","link":"https://dingtalk.com"}}]]' --format json
```

```bash
# ── 工作流 8: 合并单元格 ──

# 1. 获取工作表列表
dws sheet list --node <NODE_ID> --format json

# 2. 合并所有单元格（默认 mergeAll）
dws sheet merge-cells --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:B3" --format json

# 3. 按行合并
dws sheet merge-cells --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:C3" --merge-type mergeRows --format json

# 4. 按列合并
dws sheet merge-cells --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:C3" --merge-type mergeColumns --format json
```

## 上下文传递

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `merge-cells` | `a1Notation` 实际被合并的范围、`mergeType` 生效的合并方式 | 确认合并结果 |
| `unmerge-cells` | `sheetId` 工作表 ID | 确认操作完成 |
| `list` | 工作表的 `sheetId` | info / range read / range update / find 的 --sheet-id |

## 注意事项

- ★ **`--sheet-id` 获取规范（强制）**：`sheetId` 未知时必须先通过 `dws sheet list --node <NODE_ID> --format json` 查询，禁止凭空编造（如臆测为 `Sheet1`、`sheet1`、`0`、`default` 等）
- ★ `range update` / `range set-style` / `range batch-set-style` 单次调用上限（强制）：行数 ≤ 1000，单元格总数（行×列）建议≤ 5000（服务端硬限 30000）；超限请拆分多次调用。CLI 会在调用前做本地预校验，服务端超 30000 会直接报错
- `range set-style` / `range batch-set-style` 的样式枚举按驼峰书写：`wordWrap` 取 `overflow`/`clip`/`autoWrap`，`fontWeight` 取 `bold`/`normal`，`hAlign` 取 `left`/`center`/`right`/`general`，`vAlign` 取 `top`/`middle`/`bottom`；背景色/字体颜色统一使用 `#RRGGBB` 格式
- `range update` 支持通过 `cellStyles` 在写值时附带 per-cell 样式，适合少量单元格写值 + 样式一步到位的场景。批量设置整片区域的统一样式时，仍应使用 `set-style` / `batch-set-style`
- `merge-cells` 合并时只保留左上角单元格的值，其他单元格的值会被丢弃
- `merge-cells` 的 `--merge-type` 不传时默认为 `mergeAll`（合并所有单元格）
- `merge-cells` 的 `--range` 支持带工作表前缀的写法（如 `Sheet1!A1:B3`），此时忽略 `--sheet-id`
- `merge-cells` 如果目标区域与其他合并单元格、锁定区域或表格区域存在交集，合并将失败
- `unmerge-cells` 取消指定范围内所有合并单元格，使用 A1 表示法指定范围
- 对已有表格做格式延续、插入列后修复表头、或写入前临时取消合并时，先记录 `sheet info` 返回的 `mergedRanges`，操作后按需用 `merge-cells` 恢复
