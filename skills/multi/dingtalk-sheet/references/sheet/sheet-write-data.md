# 数据写入

## 使用场景

用户说"写数据/填表/更新单元格/写入公式":
- 从 DataFrame/结构化对象一次写入一个或多个工作表 → `table-put`
- 更新数据 → `range update`
- 写入公式前先读 [sheet-formula](./sheet-formula.md)；写完必须回读公式文本和计算结果，不能只看写入返回成功
- 【强制】`--sheet-id` 必填：即使是单工作表也不能省略，不要参照 `range read` 的默认行为；未知时先执行 `dws sheet list --node <NODE_ID> --format json` 获取 `sheetId`，禁止凭空臆测为 `Sheet1`、`sheet1`、`0`、`default` 等
- 注意：如果用户的目的是替换文本、移动行列、追加空行空列、清空区域、排序、填充、复制区域或移动区域，请勿使用 `range update`，必须使用对应的专用命令（`replace`/`move-dimension`/`add-dimension`/`range clear`/`range sort`/`range fill`/`range copy-to`/`range move-to`）
- **批量纯值写入优先用 `csv-put`**：当写入场景同时满足以下条件时，必须优先使用 `csv-put` 而非 `range update`：(1) 写入的是纯值（不含公式、超链接、dataValidation、cellStyles、richText）；(2) 数据量较大（超过 5 行或超过 20 个单元格）；(3) 数据来源为表格/CSV 文本/结构化文本。`csv-put` 无需手动构造二维 JSON 数组，直接传 CSV 文本即可，更简洁高效且支持自动扩容

用户说"追加数据/添加行/在末尾加数据/新增记录":
- 追加数据 → `append`

用户说"批量写入CSV/导入CSV/CSV写入表格/把CSV贴到表格里":
- 写入 CSV → `csv-put`
- 与 `range update` 的区别：`csv-put` 接受 CSV 文本直接写入，无需手动构造二维 JSON 数组；适合大批量纯值写入
- 与 `append` 的区别：`csv-put` 写入指定位置（--start-cell），`append` 在末尾追加

**四种写入命令能力对比**：

| 能力 | `table-put` | `range update` | `append` | `csv-put` |
|------|-------------|----------------|----------|-----------|
| 多工作表结构化写入 | 支持 | 不支持 | 不支持 | 不支持 |
| columns / dtypes / formats | 支持 | 不支持 | 不支持 | 不支持 |
| 公式（`=` 开头） | 按 dtype/底层能力 | 支持 | 不支持 | 不支持（当文本） |
| 单元格级超链接（`hyperlink`） | 不使用此命令 | 支持 | 不支持 | 不支持 |
| 富文本（片段链接/附件/图片） | 不使用此命令 | 支持 | 不支持 | 不支持 |
| 原始值（纯数字/字符串） | 支持 | 支持 | 支持 | 支持 |
| 自动定位末尾 | mode=append | 不支持 | 支持 | 不支持 |
| 自动扩容行列 | 支持 | 不支持 | 支持 | 支持 |

`range update` 用于公式、超链接、dataValidation、richText 和少量 cellStyles 等精确单元格写入；`append` 只做末尾追加；`csv-put` 面向大批量纯值；`table-put` 面向多工作表结构化数据。公式写入流程见 [sheet-formula](./sheet-formula.md)；写入后至少回读 `formula` 模式确认公式文本，再回读 `raw_value` 或展示值检查计算结果。

## 命令详细参考

### 写入结构化 table 数据
```
Usage:
  dws sheet table-put [flags]     # 别名: dws sheet table-write
Example:
  dws sheet table-put --node <NODE_ID> \
    --sheets '[{"name":"Data","columns":["name","score"],"data":[["Alice",95]],"dtypes":{"score":"float64"}}]'

  dws sheet table-put --node <NODE_ID> --sheets @table.json
  cat table.json | dws sheet table-put --node <NODE_ID> --sheets -
Flags:
      --node string     表格文档 ID 或 URL (必填)
      --sheets string   sheet table JSON、@文件路径 或 - 表示 stdin (必填)
```

`--sheets` 接受 JSON 数组、`{"sheets":[...]}` 包装对象或单个 sheet 对象，CLI 会统一转换为非空数组。每个 sheet 至少需要 `columns`，并提供 `sheetId` 或 `name`；`name` 必须非空且少于 31 个字符。写入后必须用 `table-get` 回读验证。`table-get/table-put` 不支持放入 `sheet batch-update`。

### 更新工作表指定区域内容
```
Usage:
  dws sheet range update [flags]
Example:
  # 写入文本
  dws sheet range update --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:B2" \
    --values '[[{"type":"text","text":"姓名"},{"type":"text","text":"分数"}],[{"type":"text","text":"张三"},{"type":"text","text":"90"}]]'

  # 写入公式
  dws sheet range update --node <NODE_ID> --sheet-id <SHEET_ID> --range "C2" \
    --values '[[{"type":"text","text":"=A2&B2"}]]'

  # 写入单元格级超链接
  dws sheet range update --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1" \
    --values '[[{"type":"text","text":"钉钉","hyperlink":{"type":"path","link":"https://dingtalk.com"}}]]'

  # 清理单元格级超链接，保留当前值
  dws sheet range update --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1" \
    --values '[[{"hyperlink":{"type":"none"}}]]'

  # 清空单个单元格（text 为空字符串）
  dws sheet range update --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1" \
    --values '[[{"type":"text","text":""}]]'
Flags:
      --node string       表格文档 ID (必填)
      --sheet-id string   工作表 ID 或名称 (必填)
      --range string      目标单元格区域地址，如 A1:B3 (必填)
      --values string     单元格内容，二维 JSON 数组 (必填)；每个元素必须是 object：{type:text,text:...}、{type:richText,texts:[...]}、{dataValidation:...}、{cellStyles:...}、{hyperlink:...} 或 {}（详见下文 values 参数格式说明）
```

**合并单元格注意（`range update`）**：这里说的是 `range update` 写入单元格对象这一路径，不是所有写入命令的统一行为。目标范围与已有合并区域冲突时，服务端会拦截并返回 `MERGED_CELLS_CONFLICT` 错误，错误消息中通常会列出具体冲突的合并区域地址。收到此错误时按以下流程处理：
1. 从错误消息中获取冲突的合并区域地址（如 `A1:B2, C3:D4`），或通过 `dws sheet info --node <NODE_ID> --sheet-id <SHEET_ID> --format json` 查询完整的合并区域列表（`mergedRanges` 数组）
2. 用 `dws sheet unmerge-cells --range <冲突区域>` 取消这些合并
3. 执行 `range update` 写入数据
4. 如需保留原合并效果，用 `dws sheet merge-cells` 重新合并对应区域（注意合并后仅保留左上角单元格的值）

续写或改写已有格式化表格时，先用 `sheet info` 读取 `mergedRanges`。若原数据块存在跨列标题行（如 `A1:G1`），新增同类标题行后也要用 `merge-cells` 复制相同合并模式；仅写入值或样式不会自动创建合并区域。

**单次调用建议**：行数 ≤ 1000，单元格总数（行×列）≤ 5000；超过时请拆分多次调用。

**何时该用 `csv-put` 替代**：如果你准备用 `range update` 写入纯值（不含公式、超链接、富文本对象），且数据量超过 5 行或 20 个单元格，应改用 `csv-put`——它接受 CSV 文本直接写入，无需手动拼装二维 JSON 数组，且支持自动扩容行列。仅在需要写入公式（`=SUM(...)`）、单元格级超链接、富文本对象或修改少量单元格时才使用 `range update`。

**范围职责**：`range update` 负责写入单元格内容（原始值/公式/富文本对象），并支持通过 `cellStyles` 附带 per-cell 样式。如需批量设置整片区域的样式（不写值），请使用 `dws sheet range set-style`。

**公式回读要求**：公式用 `{"type":"text","text":"=..."}` 写入。写完后先执行 `range read --value-render-option formula` 确认公式文本，再执行 `range read --value-render-option raw_value` 检查计算结果和错误值。详见 [sheet-formula](./sheet-formula.md)。

### 在工作表末尾追加数据
```
Usage:
  dws sheet append [flags]
Example:
  dws sheet append --node <NODE_ID> --sheet-id <SHEET_ID> --values '[["张三","销售部",50000]]'
  dws sheet append --node <NODE_ID> --sheet-id <SHEET_ID> \
    --values '[["李四","市场部",38000],["王五","销售部",62000]]'
Flags:
      --node string       表格文档 ID 或 URL (必填)
      --sheet-id string   工作表 ID 或名称 (必填)
      --values string     追加数据，二维 JSON 数组 (必填)
```

`--values` 为二维 JSON 数组，外层每个元素代表一行，内层每个元素代表一个单元格值。
追加的数据列数应与工作表已有数据的列数保持一致。

### 将 CSV 数据写入指定位置
```
Usage:
  dws sheet csv-put [flags]
Example:
  dws sheet csv-put --node <NODE_ID> --sheet-id <SHEET_ID> --start-cell A1 \
    --csv 'name,score\nAlice,95\nBob,87'

  dws sheet csv-put --node <NODE_ID> --sheet-id <SHEET_ID> --start-cell B2 \
    --csv @data.csv --allow-overwrite

  cat data.csv | dws sheet csv-put --node <NODE_ID> --sheet-id <SHEET_ID> \
    --start-cell A1 --csv -

  dws sheet csv-put --node <NODE_ID> --sheet-id <SHEET_ID> --start-cell A1 \
    --csv @data.csv --dry-run
Flags:
      --node string         表格文档 ID 或 URL (必填)
      --sheet-id string     工作表 ID 或名称 (必填)
      --csv string          CSV 文本、@文件路径 或 - 表示 stdin (必填)
      --start-cell string   起始单元格，A1 表示法 (必填)
      --allow-overwrite     允许覆盖已有数据 (默认 false)
```

将 RFC 4180 格式的 CSV 文本写入指定工作表的指定单元格位置。
- **分隔符必须是英文逗号 `,`**（ASCII 0x2C），禁止使用中文逗号 `，`（U+FF0C）。中文逗号不会被识别为分隔符，会导致整行被写入同一个单元格。生成 CSV 内容时务必检查分隔符
- 只写纯值，不支持公式/样式/批注。`=` 开头的内容当文本处理，不会被解析为公式
- 数字/日期/百分数由表格系统自动识别类型（如 `95` 存为数字，`2025-03-01` 存为日期）
- 自动扩容行列：CSV 数据超出当前工作表维度时自动追加行/列
- 与 `range update` 不同，目标区域如含合并单元格，`csv-put` 会打散合并并写入纯值
- 若需要保留原有合并结构，写入前先用 `sheet info` 记录 `mergedRanges`，写入后用 `merge-cells` 恢复对应区域
- `--allow-overwrite` 默认 false，目标区域有数据时需显式传 `--allow-overwrite` 才能覆盖
- `--csv` 支持三种输入：直接传文本、`@filepath` 从本地文件读取、`-` 从 stdin 管道读取
- CSV 文本上限 2M 字符，单元格总数上限 30000
- 特殊字符处理：CLI 会自动过滤 `\r`（Windows 换行符）和 BOM（UTF-8 文件头标记），Excel/Windows 导出的 CSV 可直接使用；如 CSV 数据中含零宽字符（U+200B 等）或 Bidi 控制符，CLI 会拒绝并报错

## values 参数格式说明

`range update` 只接受 `--values` 一个数据参数，为二维 JSON 数组，第一维为行，第二维为列。每个 cell 是以下之一：

- `{}` 空对象：**跳过该单元格，保留原值不变**。只更新部分单元格时用 `{}` 占位，避免拆分多次调用
- `{type:"text",...}` 或 `{type:"richText",...}` 对象
- 任何 cell 可附加 `dataValidation` 字段，在写值的同时设置数据校验（下拉列表 / 复选框）
- 任何 cell 可附加 `cellStyles` 字段，在写值的同时设置 cell-level 样式（背景色 / 字体 / 对齐等）
- 任何 cell 可附加 `hyperlink` 字段设置单元格级超链接；`{"hyperlink":{"type":"none"}}` 表示清理单元格级超链接并保留当前值

### {}（跳过，保留原值）

```json
{}
```

只更新范围内部分单元格时，用 `{}` 占位不需要修改的位置。示例：`--range "A1:C1" --values '[[{"type":"text","text":"新值"},{},{}]]'` 只更新 A1，B1 和 C1 保持不变。

### type=text（普通文本）

```json
{ "type": "text", "text": "文本内容" }
{ "type": "text", "text": "重要", "cellStyles": { "fontWeight": "bold", "fontColor": "#FF0000" } }
```

- `text` 必须为字符串；`text=""` 表示**清空该 cell**
- `text` 以 `=` 开头识别为公式（如 `"=SUM(B2:B4)"`）
- 写数字 / 布尔请用字符串形式（如 `{"type":"text","text":"100"}` / `"true"`），服务端按内容自动识别
- 字体样式（加粗/颜色/字号等）统一走 `cellStyles`，不支持 `style` 字段

### hyperlink 子结构（可选，与 type 同级，单元格级超链接）

`hyperlink` 作用于整个单元格，适合“这个单元格整体可点击跳转”的场景。它和 richText 的片段级 `link` 不同。

> **hyperlink 三种语义**：
> - **不传 `hyperlink` 字段** → 保留原超链接（系统自动保留）
> - **`hyperlink: {"type":"none"}`** → 显式清除单元格超链接
> - **`hyperlink: {"type":"path"/"sheet"/"range", link, text?}`** → 写新超链接（覆盖）
>
> `{}` 跳过也会保留原超链接。

```json
{ "type": "text", "text": "钉钉", "hyperlink": { "type": "path", "link": "https://dingtalk.com" } }
{ "hyperlink": { "type": "sheet", "link": "Sheet2" } }
{ "hyperlink": { "type": "range", "link": "Sheet1!A4" } }
{ "hyperlink": { "type": "none" } }
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `type` | string | 必填，`path`（外部链接）/ `sheet`（工作表链接）/ `range`（单元格范围链接）/ `none`（显式清除） |
| `link` | string | type=path/sheet/range 时必填。`path` 为 URL；`sheet` 为工作表 ID 或名称；`range` 为 A1 表示法 |
| `text` | string | 可选显示文本。通常只传 cell 的 `text`，不用重复传 `hyperlink.text` |

注意：
- 不传 `hyperlink` 字段同于 “保留原超链接”，无需先 read 再回传
- Agent 调用统一使用 `hyperlink: {type:"none"}` 清除超链接；不要把 `hyperlink:null` 当默认写法，避免空字段在调用链路中被过滤后语义不清
- `hyperlink` 可以不带 `type/text` cell 单独出现，用于只设置或清理链接并保留原值
- 不要把 `hyperlink` 和 `type:"richText"` 混用；整格链接用 `hyperlink`，片段链接用 richText 子项 `type:"link"`

### type=richText（富文本：片段链接 / 附件 / 图片 / 多片段组合）

```json
{ "type": "richText", "texts": [ ...子项数组... ] }
```

`texts` 子项 `type` 枚举与字段：

| 子项 type | 必填字段 | 可选字段 | 说明 |
|-----------|---------|---------|------|
| `text` | `text`（字符串） | `style` | 普通文本片段 |
| `link` | `text` + `link`（都非空字符串） | `subType` / `style` | 富文本片段链接。`subType` 默认为 `path`；`path` 的 `link` 是 URL，`sheet` 的 `link` 是真实工作表名称，`range` 的 `link` 是 A1 表示法（如 `Sheet1!A1:B2`） |
| `attachment` | `text` + `resourceId` + `mimeType` | `size`（字节数） | 附件。`text` 是显示文件名，`resourceId` 通过 `dws sheet media-upload` 获取 |
| `image` | `resourceId` + `resourceUrl` | `text`（建议传 `""`） / `width` / `height` | 图片。两个 resource 字段都通过 `dws sheet media-upload` 获取；像素 |

### style 子结构（仅 richText 子项的 `text` / `link` 类型支持）

用于 richText 内部片段级样式，实现同一单元格内不同文字有不同样式（如部分文字红色加粗）。

| 字段 | 类型 | 说明 |
|------|------|------|
| `bold` | boolean | 加粗 |
| `italic` | boolean | 斜体 |
| `underline` | boolean | 下划线 |
| `strike` | boolean | 删除线 |
| `color` | string | 字体颜色，16 进制色值（如 `#FF0000`） |
| `size` | number | 字号，正整数 |

**richText link 的 `subType`**：

```json
{ "type": "link", "text": "钉钉", "link": "https://dingtalk.com", "subType": "path" }
{ "type": "link", "text": "工作表", "link": "Sheet2", "subType": "sheet" }
{ "type": "link", "text": "明细区域", "link": "Sheet2!A1:B20", "subType": "range" }
```

- 不传 `subType` 时按 `path` 处理，适合外部 URL
- `subType:"sheet"` / `"range"` 需要使用真实工作表名称或 A1 范围；未知时先 `dws sheet list --node <NODE_ID> --format json`，禁止猜 `Sheet1`
- 这只影响富文本片段链接；整格链接仍使用 cell-level `hyperlink`
- 写入后用 `range read` 读取时，`richText.texts[].subType` 会按同样语义返回；不要把 richText 片段链接和整格 `hyperlink` 混淆

注意：`type:"text"` 的顶层旧 `style` 字段只作为历史兼容存在，新请求不要使用；整个单元格的字体样式请用 `cellStyles`，同一 cell 内分段样式才用 richText 子项 `style`。

### dataValidation 子结构（可选，与 type 同级）

任何 cell 可附加 `dataValidation` 字段，在写值的同时设置数据校验。支持两种类型：

> **dataValidation 三种语义**：
> - **不传 `dataValidation` 字段** → 自动保留原 DV（无需 read 后回写）
> - **`dataValidation: {"type":"none"}`** → 显式清除该单元格 DV
> - **`dataValidation: {"type":"dropdown"/"checkbox", ...}`** → 写新 DV（覆盖原 DV）
>
> `{}` 跳过和不传 dataValidation 字段都会保留原 DV。

**dropdown（下拉列表）**：
```json
{ "type": "text", "text": "High", "dataValidation": { "type": "dropdown", "options": [{"value":"High","color":"#00ff00"},{"value":"Low","color":"#ff0000"}], "enableMultiSelect": false } }
```
- `options`：必填，`[{value, color?}]` 数组
- `enableMultiSelect`：可选，是否多选，默认 false

**checkbox（复选框）**：
```json
{ "dataValidation": { "type": "checkbox", "checked": true } }
```
- `checked`：可选，初始勾选状态，默认 false
- checkbox 通常不需要 type/text（保留原值），也可以和 `type:"text"` 共存

**翻译场景示例**（一次调用更新文本 + 翻译 dropdown 选项 + 跳过 checkbox）：
```bash
dws sheet range update --node NODE_ID --sheet-id SHEET_ID --range "A1:C1" \
  --values '[[{"type":"text","text":"High","dataValidation":{"type":"dropdown","options":[{"value":"High"},{"value":"Medium"},{"value":"Low"}]}},{},{"type":"text","text":"Translated"}]]'
```

### cellStyles 子结构（可选，与 type 同级）

任何 cell 可附加 `cellStyles` 字段，在写值的同时设置 cell-level 样式。与 `style`（内联文本样式）的区别见下方说明。

```json
{ "type": "text", "text": "重要", "cellStyles": { "fontWeight": "bold", "backgroundColor": "#FFF2CC" } }
```

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
| `numberFormat` | string | 数字格式 code，如 `@`、`#,##0.00`、`yyyy/m/d`；格式 code 说明见 [「number-format 格式 code」](sheet-style-format.md#number-format-格式-code) |
| `textUnderline` | boolean | 下划线 |
| `textLineThrough` | boolean | 删除线 |

所有字段均可选，只传需要设置的字段。也可以不传 `type`/`text`，仅用 `{cellStyles:{...}}` 对已有单元格追加样式（保留原值）。

选择 `numberFormat` 前，先阅读 [「number-format 格式 code」](sheet-style-format.md#number-format-格式-code)，确认目标格式类型对应的 code。

长数字标识符请显式设置文本格式：商品 ID、规格 ID、SKU、订单号、手机号、工号等字段建议写成 `{"type":"text","text":"528545015680","cellStyles":{"numberFormat":"@"}}`。仅把值写成文本不一定能阻止常规格式展示；`@` 可以避免 11 位以上数字形态 ID 被显示成科学计数法。`range append` 不支持随行传 `cellStyles`，追加后请对返回的 `a1Notation` 或目标 ID 列执行 `range set-style --number-format "@"`。

**`cellStyles` vs `style` vs `set-style` 的区别**：

| 方式 | 适用场景 | 写在哪里 | 作用范围 |
|------|---------|---------|---------|
| `style`（richText 片段样式） | 同一 cell 内不同文字有不同字体样式 | richText 子项（`text`/`link` 类型）的 `style` | 文本片段级别 |
| `cellStyles`（cell-level 样式） | 背景色、对齐、换行、数字格式等 | cell 的 `cellStyles` | 整个单元格 |
| `set-style` / `batch-set-style` | 批量设置整片区域的样式 | 单独命令，与 `range update` 分开调用 | 指定 range 内所有单元格 |

典型用法：
- 写入少量单元格 + 样式 → 用 `range update` 的 `cellStyles`，一次调用搞定
- 批量刷整片区域统一样式 → 用 `set-style`（如 "给 A1:Z1 表头加粗居中"）
- 文本内部分段样式（如"重要"二字红色加粗，其余正常） → 用 `type:"richText"` + 子项 `style`

### 混合示例（普通文字 + 带样式片段链接）

```json
{
  "type": "richText",
  "texts": [
    { "type": "text", "text": "请访问 " },
    { "type": "link", "text": "钉钉官网", "link": "https://dingtalk.com", "style": { "color": "#0080FF", "underline": true } }
  ]
}
```

### 重要约束

- 不再支持 `{type:"number"}` / `{type:"boolean"}` / `{type:"null"}` —— 当前单元格对象协议仅接受 `text` / `richText` 两种 type，或 `{}` 跳过。数字 / 布尔走 `{type:"text","text":"<字符串形式>"}`
- 不支持直接传入原始值（字符串、数字、布尔、null、空字符串）；`null` 不等同于 `{}`，`null` 会报错
- 维度必须与 `--range` 范围完全一致，例如 `--range "A1:B3"` 需要 3 行 2 列的数组
- 清理整格超链接使用 `{"hyperlink":{"type":"none"}}`；不要使用 `{"hyperlink":null}` 作为 agent 默认调用形态
- 写图片到单元格建议直接用 `dws sheet write-image`（更简洁）
- 清空整片区域请用 `dws sheet range clear`；只清空单个 cell 可在 `--values` 中传 `{"type":"text","text":""}`

## 核心工作流

```bash
# ── 工作流 1: 创建表格并写入数据 ──

# 1. 创建表格文档 — 提取 nodeId
dws sheet create --name "销售数据" --format json

# 2. 查看工作表列表 — 提取 sheetId
dws sheet list --node <NODE_ID> --format json

# 3. 写入表头和数据
dws sheet range update --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:C1" \
  --values '[[{"type":"text","text":"姓名"},{"type":"text","text":"部门"},{"type":"text","text":"销售额"}]]' --format json

dws sheet range update --node <NODE_ID> --sheet-id <SHEET_ID> --range "A2:C4" \
  --values '[[{"type":"text","text":"张三"},{"type":"text","text":"销售部"},{"type":"text","text":"50000"}],[{"type":"text","text":"李四"},{"type":"text","text":"市场部"},{"type":"text","text":"38000"}],[{"type":"text","text":"王五"},{"type":"text","text":"销售部"},{"type":"text","text":"62000"}]]' --format json

# ── 工作流 4: 写入数据并设置样式 ──

# 1. 写入数据
dws sheet range update --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:C3" \
  --values '[[{"type":"text","text":"商品"},{"type":"text","text":"单价"},{"type":"text","text":"数量"}],[{"type":"text","text":"苹果"},{"type":"text","text":"5.5"},{"type":"text","text":"100"}],[{"type":"text","text":"香蕉"},{"type":"text","text":"3.2"},{"type":"text","text":"200"}]]' --format json

# 2. 设置数字格式（人民币）——两种方式均可：
#    方式 A: 写值时通过 cellStyles 一步到位
#    方式 B: 单独用 set-style 设置（适合只改格式不改值）
dws sheet range set-style --node <NODE_ID> --sheet-id <SHEET_ID> --range "B2:B3" \
  --number-format '"¥"#,##0.00' --format json

# 3. 长数字 ID 写值时同步设置文本格式，避免科学计数法
dws sheet range update --node <NODE_ID> --sheet-id <SHEET_ID> --range "D2:D3" \
  --values '[[{"type":"text","text":"528545015680","cellStyles":{"numberFormat":"@"}}],[{"type":"text","text":"528545015681","cellStyles":{"numberFormat":"@"}}]]' --format json

# 4. 写入单元格级超链接
dws sheet range update --node <NODE_ID> --sheet-id <SHEET_ID> --range "D1" \
  --values '[[{"type":"text","text":"详情","hyperlink":{"type":"path","link":"https://dingtalk.com"}}]]' --format json

# ── 工作流 5: 追加数据 ──

# 1. 获取工作表列表
dws sheet list --node <NODE_ID> --format json

# 2. 查看工作表详情（确认列结构）
dws sheet info --node <NODE_ID> --sheet-id <SHEET_ID> --format json

# 3. 追加单行数据
dws sheet append --node <NODE_ID> --sheet-id <SHEET_ID> \
  --values '[["张三","销售部",50000]]' --format json

# 4. 追加多行数据
dws sheet append --node <NODE_ID> --sheet-id <SHEET_ID> \
  --values '[["李四","市场部",38000],["王五","销售部",62000]]' --format json
```

## 上下文传递

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `create` | `nodeId` | list / info / new / range update / append / csv-put 的 --node |
| `list` | 工作表的 `sheetId` | range update / append / csv-put 的 --sheet-id |
| `new` | 新工作表的 `sheetId` | range update / append / csv-put 的 --sheet-id |
| `info` | `rowCount` / `nonEmptyRange.range` / `nonEmptyRange.lastRow` / `nonEmptyRange.lastColumn` / `mergedRanges` | 确定数据范围、追加写入起始行、识别合并单元格结构 |
| `append` | `a1Notation` 追加数据所在范围 | 确认追加位置 |
| `csv-put` | `a1Notation` 实际写入的单元格范围 | 确认写入位置和范围 |

## 注意事项

- ★ **`--sheet-id` 获取规范（强制）**：`sheetId` 未知时必须先通过 `dws sheet list --node <NODE_ID> --format json` 查询真实的 `sheetId` / 工作表名称后再调用，禁止凭空编造（如臆测为 `Sheet1`、`sheet1`、`0`、`default` 等）；用户仅给出工作表名称时，也应通过 `list` 校验该名称是否存在，避免名称大小写或拼写不一致导致失败
- ★ **`range update` 维度校验（强制）**：调用 `range update` 写入 `--values` 时，必须严格校验二维 JSON 数组的行数与列数与 `--range` 指定的范围完全一致：
  - 例如 `--range "A1:C3"` 表示 3 行 × 3 列，`--values` 必须是 `[[v1,v2,v3],[v4,v5,v6],[v7,v8,v9]]` 这样 3×3 的数组
  - `--range "A1"` 表示 1 行 × 1 列，`--values` 必须是 `[[v]]`
  - 维度不足请按行 / 列补齐为同等大小；不需要修改的位置用 `{}` 跳过（保留原值），需要清空的位置用 `{"type":"text","text":""}`；禁止出现各行列数不一致或与 `--range` 不匹配的情况，否则调用会直接报错
  - 如需写整格超链接，把 `{"type":"text","text":"...","hyperlink":{"type":"path","link":"..."}}` 放进 `--values` 二维数组对应的单元格里；富文本片段链接才使用 richText 子项 `type:"link"`
- ★ **清空区域优先用 `range clear`（强制）**：需要清空整片区域时必须使用 `range clear`，禁止用 `range update` 模拟。仅在 `range update` 写入混合数据时个别 cell 需要清空，才在 `--values` 中用 `{"type":"text","text":""}`
- ★ **不再支持 `{type:"number"}` / `{type:"boolean"}` / `{type:"null"}`（强制）**：当前单元格对象协议仅接受 `type:"text"` 与 `type:"richText"` 两种，CLI 会在本地直接拦截非法 type 并报错。写数字 / 布尔请用 `{"type":"text","text":"<字符串形式>"}`（服务端按内容自动识别），不要再用旧的 `value` 字段
- **dataValidation 三语义**：不传字段=保留；`{type:"none"}`=清除；`{type:"dropdown"/"checkbox",...}`=覆盖。无需先 read 再回传，系统会保留原 DV
- **hyperlink 三语义**：不传字段=保留；`{type:"none"}`=清除；`{type:"path"/"sheet"/"range",...}`=覆盖。Agent 调用不要使用 `hyperlink:null`
- ★ **单次调用上限（强制）**：`range update` / `set-style` 行数 ≤ 1000，单元格总数建议 ≤ 5000（硬限 30000）
- ★ **大批量纯值写入用 `csv-put` 不用 `range update`**：当写入纯值（无公式、无超链接、无富文本对象）且数据量较大时（>5 行或 >20 单元格），必须使用 `csv-put`。`csv-put` 接受 CSV 文本直接写入，无需构造二维 JSON 数组，支持自动扩容，更简洁高效。仅在需要写入公式、单元格级超链接、富文本对象，或仅更新少量单元格时才使用 `range update`
- ★ **公式写完必须回读校验**：先用 `range read --value-render-option formula` 确认公式文本，再用 `range read --value-render-option raw_value` 检查计算结果和明显错误值；当前没有聚合式公式校验工具
- ★ **大整数/长数字 ID 必须按文本写（强制）**：超过 `9007199254740991` 的整数、订单号、手机号、SKU 等需要逐位精确的值，使用字符串写入；需要固定文本格式时用 `range update` 的 `cellStyles.numberFormat: "@"`，并以 `range read --value-render-option raw_value` 回读确认
- `range update` 必填 `--values`；单元格级超链接通过 cell 的 `hyperlink` 字段表达，附件 / 图片 / 带样式片段通过 `--values` 内的 richText 富格式表达，CLI 不再有 `--hyperlinks` 参数
- `range update` 职责边界：`range update` 写入单元格内容（文本 / 公式 / 富文本对象），支持通过 `cellStyles` 附带 per-cell 样式（背景色 / 字号 / 对齐等）。但批量刷整片区域的统一样式时，应使用 `dws sheet range set-style`（如 "给表头加粗居中"）或 `dws sheet range batch-set-style --batch <config.json>`。两种方式各有适用场景：少量 cell 写值 + 样式一步到位用 `cellStyles`；大面积统一样式用 `set-style`
- `append` 自动定位到最后一行有数据的位置下方插入，无需手动计算行号
- `append` 的 `--values` 二维数组中每行的列数必须一致，否则会报错。如果用户提供的数据中各行长度不同，必须先将短行用空字符串 `""` 补齐到与最长行相同的列数后再调用。追加的数据列数也应与工作表已有数据列数保持一致
- `append` vs `range update`：追加新行用 `append`，修改已有单元格用 `range update`
- ★ **`append` / `csv-put` 不支持 `{}` skip、dataValidation、富文本、超链接**：这些能力仅限 `range update`；需要公式、超链接、下拉列表或跳过部分单元格时，使用 `range update`
