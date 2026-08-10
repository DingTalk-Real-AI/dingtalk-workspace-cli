# 表格与工作表管理

## 使用场景

用户说"创建表格/新建电子表格":
- 创建空表格文档 → `create`
- 创建并写入初始数据（可选样式）→ `create-with-data`（`--values` / `--sheets` 必须给一个）

用户说"看工作表/有哪些工作表/表格结构":
- 列出工作表 → `list`
- 工作表详情 → `info`

用户说"加工作表/新增Sheet":
- 新建工作表 → `new`

用户说"修改工作表名称/重命名工作表/移动工作表位置/隐藏工作表/显示工作表/冻结行/冻结列/取消冻结/更新工作表属性":
- 更新工作表属性 → `update`
- 重命名工作表 → `update --name "新名称"`
- 移动工作表位置 → `update --index N`
- 隐藏工作表 → `update --hidden`
- 显示工作表 → `update --hidden=false`
- 冻结行列 → `update --frozen-row-count N --frozen-column-count M`
- 取消冻结 → `update --frozen-row-count 0 --frozen-column-count 0`

用户说"复制工作表/拷贝工作表/克隆工作表/工作表副本":
- 复制工作表 → `copy`
- 复制并指定名称 → `copy --name "副本名称"`
- 复制并指定位置 → `copy --index N`

用户说"显示网格线/显示网格/恢复网格线":
- 显示网格线 → `show-gridline`（连续多次调用幂等，不报错）

用户说"隐藏网格线/去掉网格/关闭网格线/展示模式/看板模式":
- 隐藏网格线 → `hide-gridline`（连续多次调用幂等，不报错）

用户说"删除工作表/移除工作表/删掉这个Sheet":
- 删除工作表 → `delete-sheet`（不可逆操作，执行前必须向用户确认）

## 命令详细参考

### 创建钉钉表格文档
```
Usage:
  dws sheet create [flags]
Example:
  dws sheet create --name "销售数据"
  dws sheet create --name "Q1 数据" --folder <FOLDER_ID>
  dws sheet create --name "知识库表格" --workspace <WS_ID>
Flags:
      --name string        表格名称 (必填)
      --folder string      目标文件夹 ID (dentryUuid 格式) 或 URL；禁止传入纯数字 dentryId
      --workspace string   目标知识库 ID
```

> **ID 格式约束**：`--folder` 只接受 UUID 格式的 `fileId`（如 `ZgpG2NdyVXYOR2D5UGDok65MJMwvDqPk`）或 alidocs 文件夹 URL。`drive list` 返回中有 `dentryId`（纯数字，如 `218595998810`）和 `fileId`（UUID 格式）两个字段，**必须使用 `fileId`，禁止使用 `dentryId`**，传入纯数字会导致命令失败。

### 创建表格文档并写入初始数据
```
Usage:
  dws sheet create-with-data [flags]
Example:
  dws sheet create-with-data --name "名单" --values '[["姓名","分数"],["张三","90"]]'
  dws sheet create-with-data --name "报表" --sheets '[{"name":"一月","columns":["项目","金额"],"data":[["房租",5000]]}]'
Flags:
      --name string        表格名称 (必填)
      --folder string      目标文件夹 ID (dentryUuid 格式) 或 URL；禁止传入纯数字 dentryId
      --workspace string   目标知识库 ID
      --values string      初始数据，二维 JSON 数组，写入默认工作表 (与 --sheets 二选一)
      --sheets string      多工作表 typed table JSON (与 --values 二选一)
      --styles string      建表时一并应用的视觉处理 JSON（需与 --values 或 --sheets 搭配）
```

建表并写入初始数据的多步编排：建文档 → 探活 → 定位默认工作表 → 写数据 → 回读校验 →（可选）应用样式。**只要一个空表格请用 `dws sheet create`**（单次调用）；本命令必须给数据。

**写入初始数据**（`--values` 与 `--sheets` **二选一，必须给一个**）：

- `--values`：二维 JSON 数组，裸值写入默认工作表 A1 起。适合单表快速建表，无表头/类型语义，内部复用 csv-put 通道，自动识别数字/布尔。单元格只能是字符串/数字/布尔/null；上限 30000 单元格、编码为 CSV 后 2000000 字符。
- `--sheets`：typed table 数组，一次创建多个带数据的工作表，内部复用 table-put 通道。每项形如
  `{"name":"表名","columns":["列1","列2"],"data":[[...]],"dtypes":{...},"formats":{...},"cellStyles":[...]}`；
  `name`、`columns` **必填**。第一个条目写入默认工作表（自动重命名为其 `name`，避免残留空表），其余按 `name` 自动新建。
  - 字段名为 camelCase，只接受 `name` / `columns` / `data` / `dtypes` / `formats` / `cellStyles` / `startCell` / `mode`(`overwrite`|`append`) / `header` / `allowOverwrite`；**未知键与 snake_case 变体一律拒绝**（服务端会静默丢弃写错的键，`{"datas":[...]}` 会写出一张只有表头的表却报成功）
  - **不接受 `sheetId`**：文档此刻还不存在，工作表只能用 `name` 指定
  - `columns` 为非空字符串数组、列名不可为空/重复（按 trim 后比较）；`data` 每行长度须等于 `columns`，单元格只能是字符串/数字/布尔/null
  - `dtypes` / `formats` 的键须是 `columns` 里的列名（服务端按列名查表，写错既不报错也不生效）
  - 单表写入上限 30000 单元格（写表头时含表头行）

**建表时一并应用样式**（`--styles`，顶层键对齐飞书 snake_case，列表项内字段兼容 camelCase；两级都拒绝未知键）：

```json
{"styles":[{"name":"表名",
  "cell_styles":[{"range":"A1:D1","font_weight":"bold","background_color":"#FFF2CC","number_format":"@"}],
  "row_sizes":[{"range":"1:1","type":"pixel","size":28}],
  "col_sizes":[{"range":"A:D","type":"pixel","size":120}],
  "cell_merges":[{"range":"A1:B1","merge_type":"all"}]}]}
```

- 每项至少给 `cell_styles` / `row_sizes` / `col_sizes` / `cell_merges` 之一
- 配 `--sheets` 时 styles 的**项数/顺序/name 必须与子表一一对应**；配 `--values` 时只给 1 项（`name` 被忽略）
- 数据写入后按 `cell_styles` → `row_sizes` → `col_sizes` → `cell_merges` 顺序执行（**非原子**）
- `row_sizes.type`：`pixel`（需 `size`）/ `standard`（恢复默认行高）/ `auto`（按内容自适应）
- `col_sizes.type`：`pixel`（需 `size`）/ `standard`（恢复默认列宽）——与飞书一致，**列宽不提供 `auto`**
- `size` 必须是正整数（小数会被拒绝而不是静默取整，避免得到与配置不符的行高列宽）；`row_sizes.range` 形如 `"1:3"`、`col_sizes.range` 形如 `"A:C"`，带多余字符一律拒绝
- `merge_type` 取 `all` / `rows` / `columns`

**行为要点**：
- 所有 JSON 结构、字段类型与枚举都在**创建文档之前**校验（`--sheets` 按 table-put 的输入契约逐字段校验），非法配置直接失败，不会留下白建的空文档
- 创建后 CLI 会先探活（新建文档服务端仍在初始化，此时写入可能返回成功但不落盘），再写数据
- 写完会回读**首个预期非空单元格**校验确实落盘（不是死盯 A1：`--sheets` 会按 `startCell` / `header` / `mode` 推算实际写入位置）；若未落盘会报错并提示用 `csv-put` / `range update` / `table-put` 补写
- 报错信息里始终带上已创建的 `nodeId`，便于在部分成功时继续操作同一份文档

示例：

```bash
# 创建并写入初始数据（默认工作表，裸二维值）
dws sheet create-with-data --name "名单" --values '[["姓名","分数"],["张三","90"]]'

# 创建多个带数据的工作表
dws sheet create-with-data --name "报表" --sheets '[{"name":"一月","columns":["项目","金额"],"data":[["房租",5000]]},{"name":"二月","columns":["项目","金额"],"data":[["房租",5000]]}]'

# 创建 + 写数据 + 一并应用样式（表头加粗黄底、行高、列宽）
dws sheet create-with-data --name "带样式" --values '[["姓名","分数"],["张三","90"]]' \
  --styles '{"styles":[{"name":"Sheet1","cell_styles":[{"range":"A1:B1","font_weight":"bold","background_color":"#FFF2CC"}],"row_sizes":[{"range":"1:1","type":"pixel","size":28}],"col_sizes":[{"range":"A:B","type":"pixel","size":120}]}]}'
```

> `--folder` / `--workspace` 的 ID 格式约束与 `dws sheet create` 完全一致（只接受 UUID 格式的 `fileId` 或 alidocs URL）。

### 获取全部工作表列表
```
Usage:
  dws sheet list [flags]
Example:
  dws sheet list --node <NODE_ID>
  dws sheet list --node "https://alidocs.dingtalk.com/i/nodes/<DOC_UUID>"
Flags:
      --node string   表格文档 ID 或 URL (必填)
```

### 获取指定工作表详情
```
Usage:
  dws sheet info [flags]
Example:
  dws sheet info --node <NODE_ID>
  dws sheet info --node <NODE_ID> --sheet-id <SHEET_ID>
  dws sheet info --node <NODE_ID> --sheet-id "Sheet1"
  dws sheet info --node <NODE_ID> --sheet-id <SHEET_ID> --include groups
  dws sheet info --node <NODE_ID> --sheet-id <SHEET_ID> --include row_heights,col_widths,hidden_rows,hidden_cols
Flags:
      --node string       表格文档 ID 或 URL (必填)
      --sheet-id string   工作表 ID 或名称 (不传则返回第一个工作表)
      --include strings   可选扩展信息，逗号分隔；支持 groups / row_heights / col_widths / hidden_rows / hidden_cols / frozen
```

返回字段中 `mergedRanges` 是当前工作表的合并单元格范围列表（A1 表示法，如 `["C7:D11"]`）。它属于工作表结构/布局元数据：读写单元格内容前，如需判断表头、分组标题、续写位置或避开合并冲突，应先看 `sheet info`，不要在 `range read` / `csv-get` 的单元格值里寻找合并信息。

返回字段中 `frozenRowCount` / `frozenColumnCount` 是冻结行列数量：冻结总是从工作表顶部第 1 行、左侧第 A 列开始连续计算；`0` 表示未冻结。它们是工作表级元数据，默认随 `sheet info` 返回，不需要额外 `include`。

最后非空数据边界通过 `nonEmptyRange` 返回，字段均为 A1/UI 语义：`range` 是从 `A1` 到最后非空单元格的范围，`lastCell` 是最后非空单元格地址，`lastRow` 是 1-based 行号，`lastColumn` 是列字母。空表时 `nonEmptyRange` 为 `null`。不要使用旧的 0-based 字段 `lastNonEmptyRow` / `lastNonEmptyColumn`。

需要读取行列分组时，加 `--include groups`。返回字段：
- `rowGroups`：行分组列表，单项包含 `range`、`startRow`、`endRow`、`count`、`level`、`collapsed`
- `columnGroups`：列分组列表，单项包含 `range`、`startColumn`、`endColumn`、`count`、`level`、`collapsed`

其中 `range` 使用 A1 行/列范围（如 `"3:7"` / `"C:F"`），起止行号为 1-based，列使用字母；`level` 是 1-based 展示层级，不返回 `depth`；`collapsed` 表示当前折叠状态。`range read` / `csv-get` 不返回 `mergedRanges`、冻结或分组等结构元数据。

需要读取行高、列宽、隐藏行列时，按需加对应 `--include` 值（可组合，逗号分隔）。返回字段：
- `--include row_heights` → `rowHeights`（逐条行高，单项 `{row, pixelSize}`，`row` 为 1-based）+ `defaultRowHeight`（默认行高像素）
- `--include col_widths` → `colWidths`（逐条列宽，单项 `{column, pixelSize}`，`column` 为列字母）+ `defaultColumnWidth`（默认列宽像素）
- `--include hidden_rows` → `hiddenRows`（隐藏行区间列表，单项 `{startRow, endRow}`，1-based，连续隐藏行合并为一个区间）
- `--include hidden_cols` → `hiddenCols`（隐藏列区间列表，单项 `{startColumn, endColumn}`，列字母，连续隐藏列合并为一个区间）

注意：`rowHeights` / `colWidths` 只返回**显式设置过**尺寸的行列（未自定义的行列不出现，用 `defaultRowHeight` / `defaultColumnWidth` 兜底）；隐藏行列即使未改尺寸也会出现在 `hiddenRows` / `hiddenCols`。`frozen` 也在 include 白名单中，但冻结字段本就默认返回，传 `frozen` 与否结果一致。

### 新建工作表
```
Usage:
  dws sheet new [flags]
Example:
  dws sheet new --node <NODE_ID> --name "Sheet2"
  dws sheet new --node <NODE_ID> --name "数据汇总"
Flags:
      --node string   表格文档 ID (必填)
      --name string   工作表名称 (必填)
```

### 更新工作表属性
```
Usage:
  dws sheet update [flags]
Example:
  # 改名 + 调整冻结
  dws sheet update --node <NODE_ID> --sheet-id <SHEET_ID> --name "汇总表" --frozen-row-count 2 --frozen-column-count 1

  # 隐藏工作表
  dws sheet update --node <NODE_ID> --sheet-id <SHEET_ID> --hidden=true

  # 显示工作表
  dws sheet update --node <NODE_ID> --sheet-id <SHEET_ID> --hidden=false

  # 移动工作表到第一个位置
  dws sheet update --node <NODE_ID> --sheet-id <SHEET_ID> --index 0

  # 取消冻结
  dws sheet update --node <NODE_ID> --sheet-id <SHEET_ID> --frozen-row-count 0 --frozen-column-count 0
Flags:
      --node string              表格文档 ID 或 URL (必填)
      --sheet-id string          工作表 ID 或名称 (必填)
      --name string              新名称，最长 100 字符，不能包含 / \ ? * [ ] :
      --index int                新位置（从 0 开始）
      --hidden                   --hidden=true 隐藏，--hidden=false 取消隐藏
      --frozen-row-count int     冻结行数，0 表示取消冻结
      --frozen-column-count int  冻结列数，0 表示取消冻结
```

更新工作表名称、位置、隐藏状态、冻结行列。
`--name` / `--index` / `--hidden` / `--frozen-row-count` / `--frozen-column-count` 至少提供一个；多个属性可同时传入，将在同一次请求中更新。

注意：
- 至少需要保留一个可见的工作表，不能将所有工作表都隐藏
- 冻结行数/列数不能超过工作表的总行数/列数

### 复制工作表
```
Usage:
  dws sheet copy [flags]
Example:
  # 按默认位置复制
  dws sheet copy --node <NODE_ID> --sheet-id <SHEET_ID>

  # 指定副本名称和位置
  dws sheet copy --node <NODE_ID> --sheet-id <SHEET_ID> --name "销售副本" --index 2

  # 只指定名称
  dws sheet copy --node <NODE_ID> --sheet-id <SHEET_ID> --name "备份"
Flags:
      --node string       表格文档 ID 或 URL (必填)
      --sheet-id string   源工作表 ID 或名称 (必填)
      --name string       副本名称，最长 100 字符，不能包含 / \ ? * [ ] : (不传则系统自动生成)
      --index int         副本位置（从 0 开始）(不传则放在源工作表之后)
```

复制指定工作表，在同一表格中创建一个副本。
复制操作会将源工作表的所有内容（包括数据、格式、公式等）完整复制到新工作表中。
传 `--index` 时，CLI 会先复制，再追加一次位置更新，把副本移动到目标索引。
名称与已有工作表重复时系统会自动重命名。

### 删除工作表
```
Usage:
  dws sheet delete-sheet [flags]
Example:
  dws sheet delete-sheet --node <NODE_ID> --sheet-id <SHEET_ID>
Flags:
      --node string       表格文档 ID 或 URL (必填)
      --sheet-id string   要删除的工作表 ID 或名称 (必填)
```

> **CAUTION:** 不可逆操作 — 执行前必须向用户确认。

删除指定的工作表及其所有数据。约束：
- 不能删除隐藏的工作表（需先通过 `sheet update --hidden false` 取消隐藏再删除）
- 不能删除最后一个可见工作表（至少保留一个可见工作表）

### 显示工作表网格线
```
Usage:
  dws sheet show-gridline [flags]
Example:
  dws sheet show-gridline --node <NODE_ID> --sheet-id <SHEET_ID>
  dws sheet show-gridline --node <NODE_ID> --sheet-id "Sheet1"
Flags:
      --node string       表格文档 ID 或 URL (必填)
      --sheet-id string   工作表 ID 或名称 (必填)
```

### 隐藏工作表网格线
```
Usage:
  dws sheet hide-gridline [flags]
Example:
  dws sheet hide-gridline --node <NODE_ID> --sheet-id <SHEET_ID>
  dws sheet hide-gridline --node <NODE_ID> --sheet-id "Sheet1"
Flags:
      --node string       表格文档 ID 或 URL (必填)
      --sheet-id string   工作表 ID 或名称 (必填)
```

切换子表网格线显隐；二态语义在命令名里，无需额外参数（同 `update --hidden` / `update --hidden=false` 的隐藏/显示工作表模式）。
网格线默认显示；隐藏后工作表背景为纯白色，适合截图、演示、仪表盘/看板场景（不影响打印和数据）。
连续多次 show 或多次 hide 均为幂等操作，不会报错。

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

# 4. 独立回读实际写入范围
dws sheet range read --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:C4" --format json

# ── 工作流 2: 读取已有表格数据 ──

# 1. 获取工作表列表
dws sheet list --node <NODE_ID> --format json

# 2. 查看工作表详情（行列数、最后非空位置等）
dws sheet info --node <NODE_ID> --sheet-id <SHEET_ID> --format json

# 3. 读取全部数据
dws sheet range read --node <NODE_ID> --sheet-id <SHEET_ID> --format json

# 4. 读取指定区域
dws sheet range read --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:D10" --format json

# ── 工作流 3: 多工作表管理 ──

# 1. 新建工作表
dws sheet new --node <NODE_ID> --name "汇总" --format json

# 2. 在新工作表中写入汇总公式
dws sheet range update --node <NODE_ID> --sheet-id <NEW_SHEET_ID> --range "A1:B1" \
  --values '[[{"type":"text","text":"指标"},{"type":"text","text":"数值"}]]' --format json

dws sheet range update --node <NODE_ID> --sheet-id <NEW_SHEET_ID> --range "A2:B2" \
  --values '[[{"type":"text","text":"总销售额"},{"type":"text","text":"=SUM(Sheet1!C2:C100)"}]]' --format json
```

## 上下文传递

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `create` | `nodeId` | list / info / new / range read / range update / find 的 --node |
| `list` | 工作表的 `sheetId` | info / range read / range update / find 的 --sheet-id |
| `new` | 新工作表的 `sheetId` | range read / range update / find 的 --sheet-id |
| `info` | `rowCount` / `nonEmptyRange.range` / `nonEmptyRange.lastRow` / `nonEmptyRange.lastColumn` / `mergedRanges` / `frozenRowCount` / `frozenColumnCount` / `rowGroups` / `columnGroups` / `rowHeights` / `colWidths` / `hiddenRows` / `hiddenCols` | 确定数据范围、追加写入起始行、判断合并单元格结构、回读冻结与行列分组、回读行高列宽与隐藏行列 |

## 注意事项

- ★ **`--sheet-id` 获取规范（强制）**：所有涉及 `--sheet-id` 参数的命令，除非用户主动提供了工作表 ID 或工作表名称，否则在 `sheetId` 未知时必须先通过 `dws sheet list --node <NODE_ID> --format json` 查询真实的 `sheetId` / 工作表名称后再调用，禁止凭空编造（如臆测为 `Sheet1`、`sheet1`、`0`、`default` 等）；用户仅给出工作表名称时，也应通过 `list` 校验该名称是否存在，避免名称大小写或拼写不一致导致失败
- `mergedRanges` 中的范围表示一个整体语义区域。合并区域内非左上角单元格为空并不代表无内容，通常应以左上角单元格的值作为该合并区域的含义。
- `create` 不传 `--folder` 和 `--workspace` 时，默认创建在"我的文档"根目录
- `list` 返回所有工作表的 ID 和名称，是后续操作的必要前置步骤
- `info` 不传 `--sheet-id` 时默认返回第一个工作表的详情
- `new` 创建工作表时，如名称与已有工作表重复，系统会自动重命名
- `update` 的 `--name`、`--index`、`--hidden`、`--frozen-row-count`、`--frozen-column-count` 至少必须提供一个
- `update` 的 `--name` 最长 100 字符，不能包含 `/ \ ? * [ ] :` 等特殊字符
- `update` 的 `--index` 为 0-based 非负整数，0 表示移动到最前面
- `update` 的 `--hidden` 设为 true 时，至少需要保留一个可见的工作表，不能将所有工作表都隐藏
- `update` 的 `--frozen-row-count` / `--frozen-column-count` 为非负整数，不能超过工作表的总行数/列数，设为 0 表示取消冻结
- `update` 当同时提供多个属性时，所有属性将在同一次请求中更新
- `copy` 复制操作会将源工作表的所有内容（包括数据、格式、公式等）完整复制到新工作表
- `copy` 的 `--name` 可选，不传时系统自动生成名称（通常为"源名称 副本"或类似格式）
- `copy` 的 `--name` 最长 100 字符，不能包含 `/ \ ? * [ ] :` 等特殊字符
- `copy` 当指定名称与已有工作表重复时，系统会自动重命名为合法值
- `copy` 的 `--index` 可选，不传时副本将放置在源工作表之后的默认位置
- `delete-sheet` 为不可逆操作，执行前必须向用户确认
- `delete-sheet` 不能删除隐藏的工作表，需先通过 `update --hidden=false` 取消隐藏再删除
- `delete-sheet` 不能删除最后一个可见工作表，至少保留一个可见工作表
- `show-gridline` / `hide-gridline` 为幂等写操作：连续调用同一命令不会报错，适合 Agent 不确定当前状态时直接调用
- `show-gridline` / `hide-gridline` 仅控制网格线视觉显示，不影响数据、打印、冻结行列等任何其他属性
- ★ 关键区分: sheet(电子表格/单元格读写) vs aitable(AI多维表/结构化记录/字段定义) vs doc(文档编辑/阅读)
