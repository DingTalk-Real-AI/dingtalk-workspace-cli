# 电子表格 (sheet) 命令参考

## 适用范围（重要）

`sheet` 产品线仅支持钉钉在线电子表格（`contentType=ALIDOC`、`extension=axls`），不支持上传的 `xlsx` / `xls` / `xlsm` / `csv` 等本地表格文件。

| 文件类型 | 处理方式 |
|---------|---------|
| 在线电子表格（`axls`） | 走 `sheet` 全部命令（读/写/筛选/合并/导出等服务端原子操作） |
| `xlsx` / `xls` / `xlsm` / `csv` 等本地表格文件 | 必须用 `dws doc download --node <ID> --output <路径>` 先下载到本地再解析处理，禁止调用任何 `sheet` 子命令（sheet 底层 MCP 工具仅认 `axls`，传入 xlsx 节点会直接报错） |
| 想把在线表格导出为 xlsx | 用 `dws sheet export` ——输入是 `axls`，输出是 xlsx（axls → xlsx 的格式转换） |

> 用户直接粘贴原始 `alidocs` URL 时必须先 probe：先执行 `dws doc info --node <URL> --format json`，按 alidocs URL 类型探测流程 校验 `contentType` 和 `extension`：
> - 仅当 `contentType=ALIDOC` 且 `extension=axls` 时，才继续走 `sheet`
> - 如果是 `xlsx` / `xls` / `xlsm` / `csv`，立即转向 `dws doc download`，并告知用户“这是本地表格文件，已为你下载到本地处理”

## 命令总览

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
      --folder string      目标文件夹 ID 或 URL
      --workspace string   目标知识库 ID
```

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
Flags:
      --node string       表格文档 ID 或 URL (必填)
      --sheet-id string   工作表 ID 或名称 (不传则返回第一个工作表)
```

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

### 读取工作表数据
```
Usage:
  dws sheet range read [flags]     # 别名: dws sheet range get
Example:
  dws sheet range read --node <NODE_ID>
  dws sheet range read --node <NODE_ID> --sheet-id <SHEET_ID>
  dws sheet range read --node <NODE_ID> --sheet-id "Sheet1" --range "A1:D10"
  dws sheet range read --node <NODE_ID> --range "Sheet1!A1:D10"

  # 使用 get 别名，与 read 等价
  dws sheet range get --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:D10"
Flags:
      --node string       表格文档 ID 或 URL (必填)
      --sheet-id string   工作表 ID 或名称 (不传则默认第一个工作表)
      --range string      读取范围，A1 表示法 (如 A1:D10，不传则读取全部数据)
```

**超时处理建议**：读取大范围数据时若出现超时或响应过慢，请主动缩小 `--range` 查询范围，**建议单次读取的单元格数量控制在 5000 个以内**（例如 50 行 × 100 列、100 行 × 50 列）。对于大表可采用分页读取策略：
- 先通过 `info` 获取 `rowCount` / `lastNonEmptyRow` / `columnCount` 确定数据边界
- 按行分批读取，如 `A1:J500`、`A501:J1000`、`A1001:J1500` ……
- 避免不传 `--range` 直接读取整个大工作表

### 更新工作表指定区域内容
```
Usage:
  dws sheet range update [flags]
Example:
  # 写入值
  dws sheet range update --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:B2" \
    --values '[["姓名","分数"],["张三",90]]'

  # 写入公式
  dws sheet range update --node <NODE_ID> --sheet-id <SHEET_ID> --range "C2" \
    --values '[["=A2&B2"]]'

  # 写入超链接
  dws sheet range update --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1" \
    --hyperlinks '[[{"type":"path","link":"https://dingtalk.com","text":"钉钉"}]]'

  # 清空区域
  dws sheet range update --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:B3" \
    --values '[[null,null],[null,null],["保留",null]]'
Flags:
      --node string            表格文档 ID (必填)
      --sheet-id string        工作表 ID 或名称 (必填)
      --range string           目标单元格区域地址，如 A1:B3 (必填)
      --values string          单元格值，二维 JSON 数组 (与 --hyperlinks 至少传一项)
      --hyperlinks string      超链接，二维 JSON 数组 (与 --values 至少传一项)
      --number-format string   数字格式，如 General/@/#,##0/0%/yyyy/m/d 等
```

### 在工作表中搜索单元格内容
```
Usage:
  dws sheet find [flags]
Example:
  # 基本搜索
  dws sheet find --node <NODE_ID> --sheet-id <SHEET_ID> --find "销售额"

  # 在指定范围内搜索
  dws sheet find --node <NODE_ID> --sheet-id <SHEET_ID> --find "合计" --range "A1:D100"

  # 正则表达式搜索（不区分大小写）
  dws sheet find --node <NODE_ID> --sheet-id <SHEET_ID> --find "^total" --use-regexp --match-case=false

  # 精确匹配整个单元格内容
  dws sheet find --node <NODE_ID> --sheet-id <SHEET_ID> --find "完成" --match-entire-cell

  # 搜索公式文本
  dws sheet find --node <NODE_ID> --sheet-id <SHEET_ID> --find "SUM" --match-formula
Flags:
      --node string         表格文档 ID 或 URL (必填)
      --sheet-id string     工作表 ID 或名称 (必填)
      --find string         搜索文本 (必填)
      --range string        搜索范围，A1 表示法 (如 A1:D10)
      --match-case          区分大小写 (默认 true)
      --match-entire-cell   精确匹配整个单元格内容
      --use-regexp          启用正则表达式搜索
      --match-formula       搜索公式文本而非显示值
      --include-hidden      包含隐藏单元格
```

### 在工作表末尾追加数据
```
Usage:
  dws sheet append [flags]
Example:
  dws sheet append --node <NODE_ID> --sheet-id <SHEET_ID> --values '[["张三","销售部",50000]]'
  dws sheet append --node <NODE_ID> --sheet-id "Sheet1" \
    --values '[["李四","市场部",38000],["王五","销售部",62000]]'
Flags:
      --node string       表格文档 ID 或 URL (必填)
      --sheet-id string   工作表 ID 或名称 (必填)
      --values string     追加数据，二维 JSON 数组 (必填)
```

`--values` 为二维 JSON 数组，外层每个元素代表一行，内层每个元素代表一个单元格值。
追加的数据列数应与工作表已有数据的列数保持一致。

### 在指定位置插入行或列
```
Usage:
  dws sheet insert-dimension [flags]
Example:
  # 在第 3 行之前插入 2 行
  dws sheet insert-dimension --node <NODE_ID> --sheet-id <SHEET_ID> --dimension ROWS --position "3" --length 2

  # 在 A 列之前插入 1 列
  dws sheet insert-dimension --node <NODE_ID> --sheet-id <SHEET_ID> --dimension COLUMNS --position "A" --length 1

  # 使用工作表前缀（忽略 --sheet-id）
  dws sheet insert-dimension --node <NODE_ID> --sheet-id <SHEET_ID> --dimension ROWS --position "Sheet1!3" --length 5

  # 在 AB 列之前插入 3 列
  dws sheet insert-dimension --node <NODE_ID> --sheet-id <SHEET_ID> --dimension COLUMNS --position "AB" --length 3
Flags:
      --node string        表格文档 ID 或 URL (必填)
      --sheet-id string    工作表 ID 或名称 (必填)
      --dimension string   插入维度: ROWS 或 COLUMNS (必填)
      --position string    插入位置，A1 表示法 (必填)。ROWS 时为行号如 "3"；COLUMNS 时为列字母如 "A"
      --length string      插入数量，正整数 (必填)，最大 5000
```

在钉钉表格指定工作表的指定位置之前插入若干空行或空列。
`--dimension ROWS` 时，`--position` 为 1-based 行号字符串；`--dimension COLUMNS` 时，`--position` 为列字母。
支持在 `--position` 中携带工作表前缀（如 `Sheet1!3`），此时忽略 `--sheet-id`。
若需要在末尾追加行/列，请使用 `append` 命令。

### 删除指定位置的行或列
```
Usage:
  dws sheet delete-dimension [flags]
Example:
  # 从第 3 行开始删除 2 行
  dws sheet delete-dimension --node <NODE_ID> --sheet-id <SHEET_ID> --dimension ROWS --position "3" --length 2

  # 从 A 列开始删除 1 列
  dws sheet delete-dimension --node <NODE_ID> --sheet-id <SHEET_ID> --dimension COLUMNS --position "A" --length 1

  # 使用工作表前缀（忽略 --sheet-id）
  dws sheet delete-dimension --node <NODE_ID> --sheet-id <SHEET_ID> --dimension ROWS --position "Sheet1!3" --length 5

  # 从 AB 列开始删除 3 列
  dws sheet delete-dimension --node <NODE_ID> --sheet-id <SHEET_ID> --dimension COLUMNS --position "AB" --length 3
Flags:
      --node string        表格文档 ID 或 URL (必填)
      --sheet-id string    工作表 ID 或名称 (必填)
      --dimension string   删除维度: ROWS 或 COLUMNS (必填)
      --position string    删除起始位置，A1 表示法 (必填)。ROWS 时为行号如 "3"；COLUMNS 时为列字母如 "A"
      --length string      删除数量，正整数 (必填)，最大 5000
```

在钉钉表格指定工作表中，从指定位置起删除若干连续的行或列。
`--dimension ROWS` 时，`--position` 为 1-based 行号字符串；`--dimension COLUMNS` 时，`--position` 为列字母。
支持在 `--position` 中携带工作表前缀（如 `Sheet1!3`），此时忽略 `--sheet-id`。
删除后后续的行/列会向前移动填补空位；若需要仅清空内容但保留行/列占位，请使用 `clear_range` 工具。

### 更新指定范围行/列属性
```
Usage:
  dws sheet update-dimension [flags]
Example:
  # 隐藏第 3~4 行
  dws sheet update-dimension --node <NODE_ID> --sheet-id <SHEET_ID> --dimension ROWS --start-index "3" --length 2 --hidden

  # 显示 A~B 列
  dws sheet update-dimension --node <NODE_ID> --sheet-id <SHEET_ID> --dimension COLUMNS --start-index "A" --length 2 --hidden=false

  # 设置第 1~5 行行高为 40px
  dws sheet update-dimension --node <NODE_ID> --sheet-id <SHEET_ID> --dimension ROWS --start-index "1" --length 5 --pixel-size 40

  # 设置 C 列列宽为 200px 并隐藏
  dws sheet update-dimension --node <NODE_ID> --sheet-id <SHEET_ID> --dimension COLUMNS --start-index "C" --length 1 --pixel-size 200 --hidden

  # 使用工作表前缀（忽略 --sheet-id）
  dws sheet update-dimension --node <NODE_ID> --sheet-id <SHEET_ID> --dimension ROWS --start-index "Sheet1!3" --length 2 --hidden
Flags:
      --node string          表格文档 ID 或 URL (必填)
      --sheet-id string      工作表 ID 或名称 (必填)
      --dimension string     更新维度: ROWS 或 COLUMNS (必填)
      --start-index string   起始位置，A1 表示法 (必填)。ROWS 时为行号如 "3"；COLUMNS 时为列字母如 "A"
      --length string        更新数量，正整数 (必填)，最大 5000
      --hidden               是否隐藏 (true=隐藏, false=显示)，与 --pixel-size 至少填其一
      --pixel-size int       行高或列宽（像素），ROWS 时为行高，COLUMNS 时为列宽，与 --hidden 至少填其一
```

批量更新钉钉表格指定工作表中连续多行/多列的属性，支持设置显隐状态（hidden）与行高/列宽（pixelSize）。
`--dimension ROWS` 时，`--start-index` 为 1-based 行号字符串；`--dimension COLUMNS` 时，`--start-index` 为列字母。
支持在 `--start-index` 中携带工作表前缀（如 `Sheet1!3`），此时忽略 `--sheet-id`。
`--hidden` 与 `--pixel-size` 至少必须提供一个。当同时提供时，将先应用尺寸再应用显隐，任一失败整体失败。
`--pixel-size` 单位为像素，`dimension=ROWS` 时表示行高、`dimension=COLUMNS` 时表示列宽。

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

### 上传附件到表格
```
Usage:
  dws sheet media-upload [flags]
Example:
  dws sheet media-upload --node <NODE_ID> --file ./report.pdf
  dws sheet media-upload --node <NODE_ID> --file ./data.bin --name "数据文件.dat" --mime-type application/octet-stream
Flags:
      --node string        目标表格文档的标识，支持传入 URL 或 ID (必填)
      --file string        本地文件路径 (必填)
      --name string        附件显示名称 (默认使用文件名)
      --mime-type string   文件 MIME 类型 (默认根据扩展名推断)
```

### 上传图片并写入表格单元格
```
Usage:
  dws sheet write-image [flags]
Example:
  dws sheet write-image --node <NODE_ID> --sheet-id <SHEET_ID> --range A1:A1 --file ./chart.png
  dws sheet write-image --node <NODE_ID> --sheet-id <SHEET_ID> --range B2:B2 --file ./logo.png --width 200 --height 100
Flags:
      --node string        目标表格文档的标识，支持传入 URL 或 ID (必填)
      --sheet-id string    工作表 ID 或名称 (必填)
      --range string       目标单元格区域地址，如 A1:A1 (必填)
      --file string        本地图片文件路径 (必填)
      --name string        图片显示名称 (默认使用文件名)
      --mime-type string   文件 MIME 类型 (默认根据扩展名推断)
      --width int          图片显示宽度 (可选)
      --height int         图片显示高度 (可选)
```

### 全局查找替换
```
Usage:
  dws sheet replace [flags]
Example:
  dws sheet replace --node <NODE_ID> --sheet-id <SHEET_ID> --find "旧文本" --replacement "新文本"
  dws sheet replace --node <NODE_ID> --sheet-id <SHEET_ID> --find "待处理" --replacement "已完成" --match-entire-cell
  dws sheet replace --node <NODE_ID> --sheet-id <SHEET_ID> --find "\\d{4}" --replacement "****" --use-regexp
  dws sheet replace --node <NODE_ID> --sheet-id <SHEET_ID> --find "旧" --replacement "新" --range "A1:D100"
  dws sheet replace --node <NODE_ID> --sheet-id <SHEET_ID> --find "临时" --replacement ""
Flags:
      --node string            表格文档 ID 或 URL (必填)
      --sheet-id string        工作表 ID 或名称 (必填)
      --find string            查找文本 (必填)
      --replacement string     替换文本 (必填，可为空字符串表示删除)
      --range string           替换范围，A1 表示法 (如 A1:D100)
      --match-case             区分大小写 (默认 false)
      --match-entire-cell      完整单元格匹配
      --use-regexp             启用正则表达式匹配
      --include-hidden         包含隐藏行/列
```

返回被替换的单元格数量。`--replacement` 可以为空字符串，表示删除匹配内容。

### 移动行或列
```
Usage:
  dws sheet move-dimension [flags]
Example:
  # 将第 2 行移动到第 5 行的位置（索引从 0 开始）
  dws sheet move-dimension --node <NODE_ID> --sheet-id <SHEET_ID> \
    --dimension ROWS --start-index 1 --end-index 1 --destination-index 4

  # 将第 2~4 行移动到第 1 行之前
  dws sheet move-dimension --node <NODE_ID> --sheet-id <SHEET_ID> \
    --dimension ROWS --start-index 1 --end-index 3 --destination-index 0

  # 将 B~C 列移动到 E 列的位置
  dws sheet move-dimension --node <NODE_ID> --sheet-id <SHEET_ID> \
    --dimension COLUMNS --start-index 1 --end-index 2 --destination-index 4
Flags:
      --node string              表格文档 ID 或 URL (必填)
      --sheet-id string          工作表 ID 或名称 (必填)
      --dimension string         维度类型: ROWS 或 COLUMNS (必填)
      --start-index int          源起始索引，0-based (必填)
      --end-index int            源结束索引，0-based，包含 (必填)
      --destination-index int    目标位置索引，0-based (必填)
```

索引均为 0-based（第 1 行/列的索引为 0）。destination-index 不能在 [start-index, end-index] 范围内。

**destination-index 计算规则：**
destination-index 是目标位置的 0-based 索引，即移动到第 n 行/列则传 n-1：
- 通用公式：`destination-index = 目标行号(1-based) - 1`
- 例如：将第 2 行移到第 5 行位置 → `destination-index = 5 - 1 = 4`，即 `start-index=1, end-index=1, destination-index=4`
- 例如：将第 4 行移到第 1 行（最前面）→ `destination-index = 1 - 1 = 0`，即 `start-index=3, end-index=3, destination-index=0`

### 追加空行或空列
```
Usage:
  dws sheet add-dimension [flags]
Example:
  dws sheet add-dimension --node <NODE_ID> --sheet-id <SHEET_ID> --dimension ROWS --length 5
  dws sheet add-dimension --node <NODE_ID> --sheet-id <SHEET_ID> --dimension COLUMNS --length 3
Flags:
      --node string        表格文档 ID 或 URL (必填)
      --sheet-id string    工作表 ID 或名称 (必填)
      --dimension string   维度类型: ROWS 或 COLUMNS (必填)
      --length int         追加数量，正整数，最多 5000 (必填)
```

在工作表末尾追加指定数量的空行或空列。

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
- **场景**：之前通过 `set-criteria` 设置了某列的筛选条件，现在需要取消该列的筛选以显示全部数据。
- **区分**：`clear-criteria` 只清除指定列的条件，筛选视图本身和其他列的条件保持不变；`delete` 会删除整个筛选视图。如果指定列没有设置筛选条件，调用此命令不会报错（幂等操作）。

### 导出表格为 xlsx（异步任务一站式）
```
Usage:
  dws sheet export [flags]    # 一站式：提交 → 轮询 → 可选下载
Example:
  # 仅导出，返回 downloadUrl（链接有时效性，请尽快下载）
  dws sheet export --node <NODE_ID>
  dws sheet export --node "https://alidocs.dingtalk.com/i/nodes/<DOC_UUID>"

  # 导出并自动下载为本地文件
  dws sheet export --node <NODE_ID> --output ./report.xlsx

  # --output 为目录时，自动按下载链接中的文件名保存
  dws sheet export --node <NODE_ID> --output ./

Flags:
      --node string     表格文档 ID 或 URL (必填)
      --output string   本地保存路径（可选，支持文件路径或目录）
```

将钉钉在线电子表格导出为 Office xlsx 格式。**单命令一站式**：命令内部自动完成「提交任务 → 渐进式退避轮询 → （可选）下载文件」全流程，AI Agent 无需自行拆分步骤或实现轮询。

**内部流程**：
1. 调 `submit_export_job` 获取 `jobId`
2. 按渐进式退避策略轮询 `query_export_job` 直至任务终态或超时
3. 任务成功后取得 `downloadUrl`；若指定了 `--output`，自动 HTTP GET 下载 xlsx 到本地文件

**内置轮询策略（CLI 内实现，无需关心）**：
- 第 1~5 次：每次间隔 2 秒
- 第 6~10 次：每次间隔 5 秒
- 第 11~20 次：每次间隔 10 秒
- 第 21~30 次：每次间隔 15 秒
- **硬上限：最多轮询 30 次（约 5 分钟）**，超时后命令返回错误

**命令返回**：
- `--output` 未指定：进度日志 + 末尾输出 `jobId` 和 `downloadUrl`（链接有时效性，请尽快下载）
- `--output` 指定为文件路径：下载到该路径并输出 `导出完成: <path>`
- `--output` 指定为已存在目录：自动从 `downloadUrl` 推断文件名并保存到该目录下

**失败处理（命令内部已处理，Agent 仅需转述）**：
- MCP 返回 `FAILED`：命令立即返回错误并附带失败原因，**禁止自动重试 `dws sheet export`**，告知用户稍后再试
- 轮询 30 次仍 `PROCESSING`：命令返回超时错误，告知用户稍后再试

**限制**：仅支持钉钉在线电子表格（alxs）→ xlsx。导出钉钉文字文档请使用 `doc` 产品对应的导出工具。

## URL 识别与 NODE_ID 提取

当用户输入包含钉钉文档 URL 时，必须先识别并提取 NODE_ID，再判断意图。

硬性规则：对用户直接给出的原始 `alidocs` URL，不允许直接走 `sheet` 命令，必须先执行：

```bash
dws doc info --node "<URL>" --format json
```

根据返回路由：
- `contentType=ALIDOC` + `extension=axls` → 继续走 `sheet`
- `contentType≠ALIDOC` + `extension=xlsx` / `xls` / `xlsm` / `csv` → 转向 `dws doc download --node <ID> --output <路径>`，禁止调用任何 sheet 子命令
- 其他类型 → 按 alidocs URL 类型探测流程 路由

补充：如果这是用户直接提供的原始 `alidocs` URL，先按 alidocs URL 类型探测流程 probe 一次确认真实类型，再判断是否继续走 `sheet`。

### 支持的 URL 格式

| 格式 | 示例 | NODE_ID 提取方式 |
|------|------|----------------|
| `alidocs.dingtalk.com/i/nodes/{id}` | `https://alidocs.dingtalk.com/i/nodes/9E05BDRVQePjzLkZt2p2vE7kV63zgkYA` | 取 URL 路径最后一段 |
| `alidocs.dingtalk.com/i/nodes/{id}?queryParams` | `https://alidocs.dingtalk.com/i/nodes/abc123?doc_type=wiki_doc` | 忽略 query 参数，取路径最后一段 |

### 提取规则

1. 匹配 URL 中 `alidocs.dingtalk.com` 域名
2. 取 URL path 的最后一段作为 NODE_ID（去掉 query string 和 fragment）
3. 提取出的 NODE_ID 可直接用于所有 `--node` 参数，也可将完整 URL 传给 `--node`（CLI 会自动解析）
4. 对用户直接提供的原始 `alidocs` URL，先按 alidocs URL 类型探测流程 probe；只有 probe 确认 `contentType=ALIDOC` 且 `extension=axls` 时，才继续留在 `sheet`；如果 `extension=xlsx` / `xls` / `xlsm` / `csv`，必须转向 `dws doc download`，不能走任何 sheet 命令

## 意图判断

用户说"创建表格/新建电子表格":
- 创建表格文档 → `create`

用户说"看工作表/有哪些工作表/表格结构":
- 列出工作表 → `list`
- 工作表详情 → `info`

用户说"加工作表/新增Sheet":
- 新建工作表 → `new`

用户说"读数据/看表格内容":
- 读取数据 → `range read`

用户说"导出/下载xlsx/存为Excel/存成表格文件/把表格变成xlsx":
- 导出为 xlsx 文件 → `export`
- 仅需传 `--node`，可选 `--output` 指定本地文件/目录（不传则返回 downloadUrl）
- CLI 内部已完成提交 → 轮询 → 下载，Agent 不要再自己拆 submit/query，也不要在失败时自动重试

用户说"写数据/填表/更新单元格/写入公式":
- 更新数据 → `range update`
- 注意：如果用户的目的是替换文本、移动行列或追加空行空列，请勿使用 `range update`，必须使用对应的专用命令（`replace`/`move-dimension`/`add-dimension`）

用户说"搜索/查找/找单元格/搜内容/精确搜索/精确匹配/完全匹配/全字匹配":
- 搜索单元格 → `find`
- 精确匹配（只匹配完全等于的，不匹配包含的） → `find --match-entire-cell`
- 正则搜索 → `find --use-regexp`
- 搜索公式 → `find --match-formula`
- 不要用 `range read` 读取全量数据后在客户端过滤来替代 `find`，必须使用 `find` 命令的服务端搜索能力

用户说"替换/查找替换/全局替换/批量替换/把A替换成B/把所有的X改成Y":
- 查找替换 → `replace`
- 精确匹配后替换（只替换内容完全等于的单元格） → `replace --match-entire-cell`
- 正则替换 → `replace --use-regexp`
- 删除匹配内容 → `replace --replacement ""`
- 请勿用 `find` + `range update`、`range read` + `range update` 等组合来模拟替换，`replace` 是服务端原子操作，效率更高且返回替换计数

用户说"移动行/移动列/调整行顺序/调整列顺序/行列拖拽/把第N行移到第M行":
- 移动行或列 → `move-dimension`
- 请勿用 `range read` + `range update` 读取再重写来模拟移动，`move-dimension` 是原子操作，能保留格式和合并状态

用户说"追加空行/追加空列/增加行数/增加列数/扩展表格/在末尾加空行":
- 追加空行/空列 → `add-dimension`
- 注意与 `append`（追加数据行）区分：`add-dimension` 追加的是空行/空列，`append` 追加的是带数据的行
- 请勿用 `range update` 写空数据来模拟追加，`add-dimension` 直接扩展表格维度

用户说"取消合并/拆分单元格/还原合并":
- 取消合并单元格 → `unmerge-cells`

用户说"筛选视图/查看筛选视图/有哪些筛选视图/筛选视图列表":
- 获取所有筛选视图 → `filter-view list`

用户说"创建筛选视图/新建筛选视图/添加筛选视图":
- 创建筛选视图 → `filter-view create`

用户说"更新筛选视图/修改筛选视图/改筛选视图名称/改筛选视图范围":
- 更新筛选视图属性 → `filter-view update`

用户说"删除筛选视图/移除筛选视图":
- 删除筛选视图 → `filter-view delete`

用户说"设置筛选条件/添加筛选条件/配置筛选视图条件/按值筛选/按条件筛选/按颜色筛选":
- 设置筛选视图列条件 → `filter-view set-criteria`

用户说"清除筛选条件/移除筛选条件/取消筛选条件":
- 清除筛选视图列条件 → `filter-view clear-criteria`
- 注意与 `filter-view delete`（删除整个筛选视图）区分：`clear-criteria` 仅清除指定列的条件，不删除筛选视图本身

用户说"导出表格/下载表格/导出 xlsx/导出为 excel":
- 导出表格 → `export`（单命令一站式，内部自动完成提交、轮询、可选下载）
- 未指定本地路径 → `dws sheet export --node <NODE_ID>`，命令返回 `downloadUrl`
- 需要落盘到本地 → `dws sheet export --node <NODE_ID> --output <path>`，命令自动下载 xlsx
- 禁止用 `range read` 全量读取后自行拼接 xlsx 来模拟导出，必须使用 `export` 命令（服务端原子导出，保留格式/合并/公式等属性）
- 禁止在 AI Agent 侧实现轮询或重试，CLI 内部已按渐进式退避策略完成（最多 30 次约 5 分钟）

用户说"追加数据/添加行/在末尾加数据/新增记录":
- 追加数据 → `append`

用户说"插入行/插入列/在某行前插入/在某列前插入":
- 插入行或列 → `insert-dimension`
- 在末尾追加 → `append`（insert-dimension 不支持末尾追加）

用户说"删除行/删除列/删掉第几行/删掉某列/移除行/移除列":
- 删除行或列 → `delete-dimension`
- 仅清空内容但保留行/列 → `clear_range`（如有）

用户说"隐藏行/隐藏列/显示行/显示列/设置行高/设置列宽/调整行高/调整列宽/行列属性":
- 隐藏/显示行或列 → `update-dimension --hidden` / `--hidden=false`
- 设置行高/列宽 → `update-dimension --pixel-size`
- 同时修改尺寸与显隐 → `update-dimension --pixel-size --hidden`

用户说"合并单元格/合并/合并区域/按行合并/按列合并":
- 合并所有单元格 → `merge-cells`（默认 mergeAll）
- 按行合并 → `merge-cells --merge-type mergeRows`
- 按列合并 → `merge-cells --merge-type mergeColumns`
> 补充：如果用户直接粘贴的是原始 `alidocs` URL，先按 alidocs URL 类型探测流程 probe；只有 probe 确认是 `axls`（在线电子表格）后，才继续按下列意图执行；如果是 `xlsx` / `xls` / `xlsm` / `csv` 本地表格文件，转向 `dws doc download`，不得走任何 sheet 命令。

用户说"上传附件/传文件到表格/上传文件到表格/上传到表格":
- 上传附件 → `media-upload`（需表格 ID 或 URL + 本地文件路径）
- 用户指定了上传后的名称 → `media-upload --name "自定义名称"`
- `media-upload` 的 `--name` 参数用于指定附件在表格中显示的名称（不改变本地文件名）；不传时默认使用本地文件名

用户说"写入图片/插入图片/加图片/放图片到单元格/嵌入图片到表格":
- 写入图片 → `write-image`（需表格 ID + 工作表 ID + 单元格范围 + 本地图片路径）
- 禁止使用 `range update` 写入图片，因为 `update_range` 的 MCP 工具不支持图片类型参数，调用必定失败。必须使用 `write-image` 命令
- 用户指定了图片尺寸 → `write-image --width N --height M`

用户直接粘贴表格 URL（无其他指令）:
- 先 probe：`dws doc info --node <URL> --format json` 校验 `contentType` 和 `extension`
- `extension=axls` → `list`（列出工作表）+ `range read`（读取第一个工作表数据）
- `extension=xlsx` / `xls` / `xlsm` / `csv` → 转 `dws doc download --node <URL> --output ./`，告知用户“这是本地表格文件，已为你下载到本地”，然后基于本地文件继续后续处理

用户粘贴 URL + 附加指令:
- 已 probe 为 `axls` 时：
  - "帮我看看这个表格有什么数据" → `range read`
  - "这个表格有哪些工作表" → `list`
  - "往这个表格写入数据" → `range update`
  - "帮我找一下表格里的XXX" → `find`
- probe 为 xlsx/xls/xlsm/csv 时：无论用户说“读数据/查看/分析”，先走 `dws doc download` 下载到本地，由用户或后续步骤对本地 xlsx 进行解析，严禁调用 `sheet list` / `range read` 等命令

关键区分: sheet(电子表格/单元格读写) vs aitable(AI多维表/结构化记录) vs doc(文档编辑/阅读)

## 核心工作流

```bash
# ── 工作流 1: 创建表格并写入数据 ──

# 1. 创建表格文档 — 提取 nodeId
dws sheet create --name "销售数据" --format json

# 2. 查看工作表列表 — 提取 sheetId
dws sheet list --node <NODE_ID> --format json

# 3. 写入表头和数据
dws sheet range update --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:C1" \
  --values '[["姓名","部门","销售额"]]' --format json

dws sheet range update --node <NODE_ID> --sheet-id <SHEET_ID> --range "A2:C4" \
  --values '[["张三","销售部",50000],["李四","市场部",38000],["王五","销售部",62000]]' --format json

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
  --values '[["指标","数值"]]' --format json

dws sheet range update --node <NODE_ID> --sheet-id <NEW_SHEET_ID> --range "A2:B2" \
  --values '[["总销售额","=SUM(Sheet1!C2:C100)"]]' --format json

# ── 工作流 4: 批量更新与格式化 ──

# 1. 写入数据
dws sheet range update --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:C3" \
  --values '[["商品","单价","数量"],["苹果",5.5,100],["香蕉",3.2,200]]' --format json

# 2. 设置数字格式（人民币）
dws sheet range update --node <NODE_ID> --sheet-id <SHEET_ID> --range "B2:B3" \
  --values '[[5.5],[3.2]]' --number-format "¥#,##0.00" --format json

# 3. 写入超链接
dws sheet range update --node <NODE_ID> --sheet-id <SHEET_ID> --range "D1" \
  --hyperlinks '[[{"type":"path","link":"https://dingtalk.com","text":"详情"}]]' --format json

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

```bash
# ── 工作流 6: 插入行或列 ──

# 1. 获取工作表列表
dws sheet list --node <NODE_ID> --format json

# 2. 在第 3 行之前插入 2 行
dws sheet insert-dimension --node <NODE_ID> --sheet-id <SHEET_ID> \
  --dimension ROWS --position "3" --length 2 --format json

# 3. 在 A 列之前插入 1 列
dws sheet insert-dimension --node <NODE_ID> --sheet-id <SHEET_ID> \
  --dimension COLUMNS --position "A" --length 1 --format json

# 4. 使用工作表前缀指定位置
dws sheet insert-dimension --node <NODE_ID> --sheet-id <SHEET_ID> \
  --dimension ROWS --position "Sheet1!5" --length 3 --format json
```

```bash
# ── 工作流 6b: 删除行或列 ──

# 1. 获取工作表列表
dws sheet list --node <NODE_ID> --format json

# 2. 从第 3 行开始删除 2 行
dws sheet delete-dimension --node <NODE_ID> --sheet-id <SHEET_ID> \
  --dimension ROWS --position "3" --length 2 --format json

# 3. 从 A 列开始删除 1 列
dws sheet delete-dimension --node <NODE_ID> --sheet-id <SHEET_ID> \
  --dimension COLUMNS --position "A" --length 1 --format json

# 4. 使用工作表前缀指定位置
dws sheet delete-dimension --node <NODE_ID> --sheet-id <SHEET_ID> \
  --dimension ROWS --position "Sheet1!5" --length 3 --format json
```

```bash
# ── 工作流 6c: 更新行/列属性（显隐、行高/列宽） ──

# 1. 获取工作表列表
dws sheet list --node <NODE_ID> --format json

# 2. 隐藏第 3~4 行
dws sheet update-dimension --node <NODE_ID> --sheet-id <SHEET_ID> \
  --dimension ROWS --start-index "3" --length 2 --hidden --format json

# 3. 显示 A~B 列
dws sheet update-dimension --node <NODE_ID> --sheet-id <SHEET_ID> \
  --dimension COLUMNS --start-index "A" --length 2 --hidden=false --format json

# 4. 设置第 1~5 行行高为 40px
dws sheet update-dimension --node <NODE_ID> --sheet-id <SHEET_ID> \
  --dimension ROWS --start-index "1" --length 5 --pixel-size 40 --format json

# 5. 设置 C 列列宽为 200px 并隐藏
dws sheet update-dimension --node <NODE_ID> --sheet-id <SHEET_ID> \
  --dimension COLUMNS --start-index "C" --length 1 --pixel-size 200 --hidden --format json
```

```bash
# ── 工作流 7: 搜索表格数据 ──

# 1. 获取工作表列表
dws sheet list --node <NODE_ID> --format json

# 2. 基本搜索 — 在指定工作表中查找文本
dws sheet find --node <NODE_ID> --sheet-id <SHEET_ID> --find "销售额" --format json

# 3. 在指定范围内搜索
dws sheet find --node <NODE_ID> --sheet-id <SHEET_ID> --find "合计" --range "A1:D100" --format json

# 4. 正则搜索（不区分大小写）
dws sheet find --node <NODE_ID> --sheet-id <SHEET_ID> --find "^total" --use-regexp --match-case=false --format json

# 5. 精确匹配整个单元格
dws sheet find --node <NODE_ID> --sheet-id <SHEET_ID> --find "完成" --match-entire-cell --format json

# 6. 搜索公式文本
dws sheet find --node <NODE_ID> --sheet-id <SHEET_ID> --find "SUM" --match-formula --format json
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

```bash
# ── 工作流 9: 上传附件到表格 ──

# 1. 基本用法: 上传本地文件到表格
dws sheet media-upload --node <NODE_ID> --file ./report.pdf -f json

# 2. 自定义附件显示名称 (--name 指定上传后在表格中显示的名称)
dws sheet media-upload --node <NODE_ID> --file ./data.csv --name "销售数据.csv" -f json

# 3. 指定 MIME 类型 (文件扩展名无法推断时)
dws sheet media-upload --node <NODE_ID> --file ./data.bin --name "导出数据.dat" --mime-type application/octet-stream -f json

# 4. 完整流程: 创建表格 → 上传附件
dws sheet create --name "项目资料" -f json
# 提取 nodeId 后:
dws sheet media-upload --node <NODE_ID> --file ./design.pdf -f json
dws sheet media-upload --node <NODE_ID> --file ./timeline.xlsx --name "项目时间线.xlsx" -f json

# ── 工作流 10: 写入图片到表格单元格 ──

# 1. 基本用法: 写入图片到指定单元格
dws sheet write-image --node <NODE_ID> --sheet-id <SHEET_ID> --range A1:A1 --file ./chart.png -f json

# 2. 指定显示尺寸
dws sheet write-image --node <NODE_ID> --sheet-id <SHEET_ID> --range B2:B2 --file ./logo.png --width 200 --height 100 -f json

# 3. 自定义图片名称
dws sheet write-image --node <NODE_ID> --sheet-id <SHEET_ID> --range C3:C3 --file ./photo.jpg --name "产品图.jpg" -f json

# 4. 完整流程: 创建表格 → 写表头 → 写入图片
dws sheet create --name "产品目录" -f json
# 提取 nodeId 后:
dws sheet range update --node <NODE_ID> --sheet-id Sheet1 --range "A1:B1" --values '[["产品名称","产品图片"]]' -f json
dws sheet range update --node <NODE_ID> --sheet-id Sheet1 --range "A2:A2" --values '[["MacBook Pro"]]' -f json
dws sheet write-image --node <NODE_ID> --sheet-id Sheet1 --range B2:B2 --file ./macbook.png --width 150 --height 100 -f json
```

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
dws sheet filter-view set-criteria --node <NODE_ID> --sheet-id <SHEET_ID> --filter-view-id <FV_ID> \
  --column 0 --filter-criteria '{"filterType":"values","visibleValues":["销售部","市场部"]}' -f json

# 5. 为筛选视图设置列条件（按条件筛选）
dws sheet filter-view set-criteria --node <NODE_ID> --sheet-id <SHEET_ID> --filter-view-id <FV_ID> \
  --column 2 --filter-criteria '{"filterType":"condition","conditions":[{"operator":"greater","value":"100"}]}' -f json

# 6. 更新筛选视图名称和范围
dws sheet filter-view update --node <NODE_ID> --sheet-id <SHEET_ID> --filter-view-id <FV_ID> \
  --name "销售数据筛选" --range "A1:F200" -f json

# 7. 清除某列的筛选条件
dws sheet filter-view clear-criteria --node <NODE_ID> --sheet-id <SHEET_ID> --filter-view-id <FV_ID> \
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

```bash
# ── 工作流 12: 导出表格为 xlsx（单命令一站式）──

# 场景 A：仅获取下载链接（命令内部自动完成提交+轮询，最终返回 downloadUrl）
dws sheet export --node <NODE_ID> --format json
# 传入 URL 也可：
# dws sheet export --node "https://alidocs.dingtalk.com/i/nodes/<DOC_UUID>" --format json

# 场景 B：导出并自动下载为本地文件
dws sheet export --node <NODE_ID> --output ./report.xlsx

# 场景 C：下载到目录，自动按链接推断文件名
dws sheet export --node <NODE_ID> --output ./

# 禁止在 Agent 侧实现任何轮询或重试，CLI 内部已按 2s/5s/10s/15s 渐进式退避自动完成（最多 30 次）。
# 若命令返回失败或超时，直接告知用户稍后再试，不要自动重调 dws sheet export。
```

## 上下文传递表

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `create` | `nodeId` | list / info / new / range read / range update / find 的 --node |
| `list` | 工作表的 `sheetId` | info / range read / range update / find 的 --sheet-id |
| `new` | 新工作表的 `sheetId` | range read / range update / find 的 --sheet-id |
| `info` | `rowCount` / `lastNonEmptyRow` | 确定数据范围、追加写入起始行 |
| `find` | `matchedCells` 中的 `a1Notation` | 定位目标单元格，用于 range read / range update |
| `append` | `a1Notation` 追加数据所在范围 | 确认追加位置 |
| `insert-dimension` | `a1Notation` 新插入区域范围 | 确认插入位置和范围 |
| `delete-dimension` | `a1Notation` 被删除区域范围 | 确认删除位置和范围 |
| `update-dimension` | `a1Notation` 被更新区域范围、`hidden` 生效的显隐状态、`pixelSize` 生效的尺寸 | 确认更新结果 |
| `merge-cells` | `a1Notation` 实际被合并的范围、`mergeType` 生效的合并方式 | 确认合并结果 |
| `media-upload` | `resourceId` | 附件已上传到表格，可用于后续引用 |
| `write-image` | `resourceId` | 图片已写入指定单元格 |
| `replace` | `replaceCount` 被替换的单元格数量 | 确认替换结果 |
| `move-dimension` | `sheetId` 工作表 ID | 确认操作完成 |
| `add-dimension` | `sheetId` 工作表 ID | 确认操作完成 |
| `unmerge-cells` | `sheetId` 工作表 ID | 确认操作完成 |
| `filter-view list` | `filterViews` 筛选视图列表（含 `id`、`name`、`range`） | 获取 filterViewId 用于 update / delete / set-criteria / clear-criteria |
| `filter-view create` | `id` 筛选视图 ID、`name`、`range` | 用于后续 update / delete / set-criteria / clear-criteria 的 --filter-view-id |
| `filter-view update` | `id`、`name`、`range`、`criteria` | 确认更新结果 |
| `filter-view delete` | `id` 被删除的筛选视图 ID | 确认删除完成 |
| `filter-view set-criteria` | `id` 筛选视图 ID | 确认条件设置完成 |
| `filter-view clear-criteria` | `id` 筛选视图 ID | 确认条件清除完成 |
| `export` | `downloadUrl`（未指定 --output）/ `导出完成: <path>`（指定 --output） | 直接下发给用户或告知文件已保存到本地。命令内部已完成轮询，不要再调用其他 export 相关命令 |

## nodeId 双格式说明

所有 `--node` 参数同时支持两种格式，系统自动识别：
- 文档 ID: 字母数字字符串，如 `9E05BDRVQePjzLkZt2p2vE7kV63zgkYA`
- 文档 URL: `https://alidocs.dingtalk.com/i/nodes/{dentryUuid}`

两种方式等价，以下命令效果相同：
```bash
dws sheet list --node 9E05BDRVQePjzLkZt2p2vE7kV63zgkYA
dws sheet list --node "https://alidocs.dingtalk.com/i/nodes/9E05BDRVQePjzLkZt2p2vE7kV63zgkYA"
```

> 这只表示 `sheet` 命令参数兼容 URL 或 ID。对用户直接给出的原始 `alidocs` URL，仍然先走上面的 probe 补充规则。

## values 参数格式说明

`--values` 为二维 JSON 数组，第一维为行，第二维为列：
- 字符串值: `"文本"`
- 数字值: `100` 或 `3.14`
- 公式: `"=SUM(B2:B4)"`（以 `=` 开头的字符串自动识别为公式）
- 清空单元格: `null`

维度必须与 `--range` 范围一致，例如 `--range "A1:B3"` 需要 3 行 2 列的数组。

## hyperlinks 参数格式说明

`--hyperlinks` 为二维 JSON 数组，每个元素为对象或 null：
- `type`: 链接类型，可选 `path`（外部链接）、`sheet`（工作表跳转）、`range`（单元格跳转）
- `link`: 链接地址
- `text`: 显示文本

与 `--values` 共存时，hyperlinks 优先级更高。

## number-format 常用值

| 格式代码 | 说明 | 示例 |
|----------|------|------|
| `General` | 常规 | 1234.5 |
| `@` | 文本 | 001234 |
| `#,##0` | 整数千分位 | 1,235 |
| `#,##0.00` | 两位小数 | 1,234.50 |
| `0%` | 百分比 | 85% |
| `yyyy/m/d` | 日期 | 2026/3/15 |
| `hh:mm:ss` | 时间 | 14:30:00 |
| `¥#,##0` | 人民币 | ¥1,235 |

## 注意事项

- `create` 不传 `--folder` 和 `--workspace` 时，默认创建在"我的文档"根目录
- `list` 返回所有工作表的 ID 和名称，是后续操作的必要前置步骤
- `info` 不传 `--sheet-id` 时默认返回第一个工作表的详情
- `range read` 不传 `--range` 时默认读取整个工作表的全部非空数据
- `range read` 的 `--range` 支持 `Sheet1!A1:D10` 格式直接指定工作表（此时忽略 `--sheet-id`）
- `range read` 遇到超时或响应过慢时，应缩小 `--range` 查询范围，**单次读取的单元格数量建议控制在 5000 个以内**；数据量较大时通过 `info` 获取边界后分批读取，避免不传 `--range` 直接读取整个大工作表
- `range update` 的 `--values` 和 `--hyperlinks` 至少传入一项
- `new` 创建工作表时，如名称与已有工作表重复，系统会自动重命名
- `find` 返回匹配单元格的地址（A1 表示法）和值，无匹配时返回空数组
- `find` 的 `--match-entire-cell` 用于精确匹配：只返回单元格内容完全等于搜索文本的结果，不会匹配包含该文本的单元格（例如搜索"苹果"时，只匹配"苹果"，不匹配"苹果手机""苹果汁"等）。用户说"精确搜索/完全匹配/只搜等于XX的"时必须使用此参数
- `find` 的 `--match-case` 默认为 true（区分大小写），设为 false 可忽略大小写
- `find` 的 `--use-regexp` 启用后，`--find` 参数作为正则表达式处理
- 当用户要求搜索/查找表格数据时，使用 `find` 命令，不要用 `range read` 读取全量数据后自行过滤——`find` 支持服务端搜索，效率更高、语义更准确
- `append` 自动定位到最后一行有数据的位置下方插入，无需手动计算行号
- `append` 的 `--values` 二维数组中每行的列数必须一致，否则会报错。如果用户提供的数据中各行长度不同，必须先将短行用空字符串 `""` 补齐到与最长行相同的列数后再调用。追加的数据列数也应与工作表已有数据列数保持一致
- `append` vs `range update`：追加新行用 `append`，修改已有单元格用 `range update`
- `insert-dimension` 在指定位置之前插入空行或空列，不写入数据；如需在末尾追加行/列，使用 `append`
- `insert-dimension` 的 `--dimension` 只接受 `ROWS` 或 `COLUMNS`
- `insert-dimension` 的 `--position` 支持工作表前缀（如 `Sheet1!3`），此时忽略 `--sheet-id`
- `insert-dimension` 的 `--length` 最大为 5000
- `delete-dimension` 从指定位置起删除若干连续的行或列，删除后后续行/列向前移动填补空位
- `delete-dimension` 的 `--dimension` 只接受 `ROWS` 或 `COLUMNS`
- `delete-dimension` 的 `--position` 支持工作表前缀（如 `Sheet1!3`），此时忽略 `--sheet-id`
- `delete-dimension` 的 `--length` 最大为 5000
- `delete-dimension` 若需仅清空内容但保留行/列占位，请使用 `clear_range` 工具
- `update-dimension` 批量更新连续行/列的显隐状态与行高/列宽
- `update-dimension` 的 `--dimension` 只接受 `ROWS` 或 `COLUMNS`
- `update-dimension` 的 `--start-index` 支持工作表前缀（如 `Sheet1!3`），此时忽略 `--sheet-id`
- `update-dimension` 的 `--length` 最大为 5000
- `update-dimension` 的 `--hidden` 与 `--pixel-size` 至少必须提供一个
- `update-dimension` 的 `--pixel-size` 单位为像素，`dimension=ROWS` 时表示行高、`dimension=COLUMNS` 时表示列宽
- `update-dimension` 当同时提供 `--hidden` 与 `--pixel-size` 时，将先应用尺寸再应用显隐，任一失败整体失败
- `merge-cells` 合并时只保留左上角单元格的值，其他单元格的值会被丢弃
- `merge-cells` 的 `--merge-type` 不传时默认为 `mergeAll`（合并所有单元格）
- `merge-cells` 的 `--range` 支持带工作表前缀的写法（如 `Sheet1!A1:B3`），此时忽略 `--sheet-id`
- `merge-cells` 如果目标区域与其他合并单元格、锁定区域或表格区域存在交集，合并将失败
- `media-upload` 是两步自动完成的流程 (获取附件上传凭证 → OSS 上传)，无需手动分步操作
- `write-image` 是三步自动完成的流程 (获取附件上传凭证 → OSS 上传 → 写入图片到单元格)，无需手动分步操作
- 向表格单元格中写入图片必须使用 `write-image`，禁止使用 `range update`。`range update` 底层调用的 `update_range` MCP 工具不支持图片类型参数，调用会失败
- `write-image` 与 `media-upload` 的区别：`media-upload` 仅上传附件到表格获取 resourceId；`write-image` 在上传后还会将图片写入指定单元格
- `replace` 的 `--find` 不能为空字符串，`--replace` 可以为空字符串（表示删除匹配内容）
- `replace` 的 `--match-case` 默认为 false（不区分大小写），与 `find` 的默认行为不同
- `replace` vs `range update`：需要批量替换文本时，必须使用 `replace` 命令，禁止用 `range update` 手动重写单元格来实现替换效果。`replace` 支持服务端全局替换，效率更高且会返回替换计数
- `move-dimension` vs `range update`：需要移动行或列时，必须使用 `move-dimension` 命令，禁止用 `range update` 读取数据后手动重写来模拟移动效果。`move-dimension` 是原子操作，能保留单元格的格式、合并状态等属性
- `move-dimension` 的索引均为 0-based（第 1 行/列的索引为 0），`endIndex` 包含在移动范围内
- `move-dimension` 的 `--destination-index` 不能在 [start-index, end-index] 范围内
- `move-dimension` 的移动跨度（end-index - start-index + 1）不超过 5000
- `move-dimension` 的 `--destination-index` 是目标位置的 0-based 索引，即移动到第 n 行/列则传 `n - 1`（通用公式：`destination-index = 目标行号(1-based) - 1`）
- `add-dimension` vs `range update`：需要在末尾追加空行/空列时，必须使用 `add-dimension` 命令，禁止用 `range update` 写空数据来模拟追加效果
- `add-dimension` 追加的是空行/空列，与 `append`（追加带数据的行）不同
- `add-dimension` 的 `--length` 必须为正整数（>= 1），行列均不超过 5000
- `unmerge-cells` 取消指定范围内所有合并单元格，使用 A1 表示法指定范围
- `filter-view list` 获取指定工作表的所有筛选视图列表，返回的 `id` 可用于后续 update / delete / set-criteria / clear-criteria 的 `--filter-view-id`
- `filter-view create` 创建筛选视图时 `--range` 应包含表头行。`--criteria` 可选，不传则创建后无筛选条件，后续可通过 `filter-view set-criteria` 设置
- `filter-view update` 的 `--name`、`--range`、`--criteria` 至少需要传入一个，未指定的字段保持不变
- `filter-view update` 的 `--criteria` 中指定列的条件会被替换，未指定的列保持不变
- `filter-view delete` 删除后该视图及其所有筛选条件将被永久移除，不可恢复
- `filter-view delete` 不影响全局筛选或其他筛选视图
- `filter-view set-criteria` 的 `--column` 为列偏移量（从 0 开始），相对于筛选视图范围首列。例如筛选视图范围为 `B1:E10`，则 `--column 0` 代表 B 列
- `filter-view set-criteria` 设置条件后立即在该筛选视图中生效，仅影响当前视图，不影响全局筛选或其他筛选视图
- `filter-view set-criteria` 的 `--filter-criteria` 中 `conditions` 最多 2 个条件，多条件之间通过 `conditionOperator` 指定逻辑关系（`and` 或 `or`）
- `filter-view clear-criteria` 仅清除指定列的条件，不会删除整个筛选视图。如需删除整个筛选视图，请使用 `filter-view delete`
- `filter-view clear-criteria` 如果指定列没有设置筛选条件，调用不会报错
- 筛选视图相关操作需要"可阅读"权限（list）或"可编辑"权限（create / update / delete / set-criteria / clear-criteria），不支持跨组织操作
- `export` 仅支持钉钉在线电子表格（alxs）→ xlsx；传入钉钉文字文档会报 `invalidRequest.document.typeIllegal`
- `export` 为单命令一站式，CLI 内部已自动完成「提交 → 渐进式退避轮询 → 可选下载」，**Agent 不得在外部实现轮询或重试**；命令返回成功后不再调用其他 export 相关命令
- `export` 内置轮询策略：1~5 次间隔 2s、6~10 次间隔 5s、11~20 次间隔 10s、21~30 次间隔 15s，硬上限 30 次（约 5 分钟）；超时后命令返回错误，告知用户稍后再试即可
- `export` 命令返回失败或超时时，**禁止自动重调 `dws sheet export`**；直接告知用户导出失败并建议稍后再试
- `export` 未指定 `--output` 时，返回的 `downloadUrl` 具有时效性，获取后请尽快下载；若用户需要本地文件，优先直接传 `--output` 让 CLI 代为下载
- `export` 的 `--output` 可为文件路径或已存在目录；为目录时自动从 `downloadUrl` 推断文件名，为文件路径时直接按该路径保存
- 用户要求“导出表格/下载 xlsx”时，必须使用 `export` 单命令，禁止用 `range read` 读全量数据后自行拼 xlsx 模拟导出（服务端导出会保留格式/合并/公式等完整属性）
- 关键区分: sheet(在线电子表格/单元格读写) vs aitable(AI多维表/结构化记录/字段定义) vs doc(文档编辑/阅读)
- sheet 产品线仅支持 `axls`（在线电子表格，`contentType=ALIDOC`），不支持 `xlsx` / `xls` / `xlsm` / `csv` 等本地表格文件
- 遇到未知 `alidocs` URL 时，必须先 probe（`dws doc info --node <URL> --format json`）确认 `contentType` 和 `extension`，才能决定是否走 sheet
- 当节点 `extension=xlsx` / `xls` / `xlsm` / `csv`（`contentType≠ALIDOC`）时，必须用 `dws doc download --node <ID> --output <路径>` 先下载到本地再处理，禁止调用任何 sheet 子命令（sheet 底层 MCP 工具只识别 axls，调用 xlsx 节点必失败）
- 要把在线表格导出为 xlsx 文件——走 `dws sheet export`（axls → xlsx 的格式转换）；要读已有的 xlsx 文件——走 `dws doc download` 后在本地解析，两者方向相反
