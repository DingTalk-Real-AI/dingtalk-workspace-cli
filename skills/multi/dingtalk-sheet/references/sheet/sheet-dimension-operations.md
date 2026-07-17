# 行列操作 (dimension operations)

## 使用场景

### 行列操作

用户说"插入行/插入列/在某行前插入/在某列前插入":
- 插入行或列 → `insert-dimension`
- 在末尾追加 → `append`（insert-dimension 不支持末尾追加）

用户说"删除行/删除列/删掉第几行/删掉某列/移除行/移除列":
- 删除行或列 → `delete-dimension`
- 仅清空内容但保留行/列 → `range clear`（默认清除值保留格式）

用户说"隐藏行/隐藏列/显示行/显示列/设置行高/设置列宽/调整行高/调整列宽/行列属性":
- 隐藏/显示行或列 → `update-dimension --hidden` / `--hidden=false`
- 设置行高/列宽 → `update-dimension --pixel-size`
- 同时修改尺寸与显隐 → `update-dimension --pixel-size --hidden`

用户说"移动行/移动列/调整行顺序/调整列顺序/行列拖拽/把第N行移到第M行":
- 移动行或列 → `move-dimension`
- 请勿用 `range read` + `range update` 读取再重写来模拟移动，`move-dimension` 是原子操作，能保留格式和合并状态

用户说"追加空行/追加空列/增加行数/增加列数/扩展表格/在末尾加空行":
- 追加空行/空列 → `add-dimension`
- 注意与 `append`（追加数据行）区分：`add-dimension` 追加的是空行/空列，`append` 追加的是带数据的行
- 请勿用 `range update` 写空数据来模拟追加，`add-dimension` 直接扩展表格维度

用户说"创建行分组/创建列分组/新建分组并折叠/新建分组并展开/取消分组":
- 创建连续行/列分组 → `group-dimension`
- 取消连续行/列分组 → `ungroup-dimension`
- 分组创建后用 `sheet info` 回读 `rowGroups` / `columnGroups`
- 仅创建分组时可用 `--group-state expand|fold` 指定初始展开/折叠；当前没有单独调整已有分组折叠状态的命令
- 用户要求"折叠/展开已有分组"时，当前不支持直接修改已有分组的 `collapsed` 状态，不能承诺可用 `group-dimension` 完成

**结构预检**：插入、删除、移动行列前，必须先执行 `dws sheet info --node <NODE_ID> --sheet-id <SHEET_ID> --format json` 查看 `mergedRanges`。合并区域跨过操作位置时，行列变更可能导致表头/分组标题断裂、空白或错位；需要先向用户说明影响，必要时取消合并后操作，再按原模式重新 `merge-cells`。

## 命令详细参考

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

> **CAUTION:** 不可逆操作 — 执行前必须向用户确认。

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
删除后后续的行/列会向前移动填补空位；若需要仅清空内容但保留行/列占位，请使用 `range clear`。

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

### 移动行或列
```
Usage:
  dws sheet move-dimension [flags]
Example:
  # 将第 2 行移动到第 5 行的位置
  dws sheet move-dimension --node <NODE_ID> --sheet-id <SHEET_ID> \
    --dimension ROWS --start-index "2" --end-index "2" --destination-index "5"

  # 将第 2~4 行（共 3 行）移动到第 1 行的位置（最前面）
  dws sheet move-dimension --node <NODE_ID> --sheet-id <SHEET_ID> \
    --dimension ROWS --start-index "2" --end-index "4" --destination-index "1"

  # 将 B~C 列（共 2 列）移动到 D 列的位置
  dws sheet move-dimension --node <NODE_ID> --sheet-id <SHEET_ID> \
    --dimension COLUMNS --start-index "B" --end-index "C" --destination-index "D"
Flags:
      --node string              表格文档 ID 或 URL (必填)
      --sheet-id string          工作表 ID 或名称 (必填)
      --dimension string         维度类型: ROWS 或 COLUMNS (必填)
      --start-index string       源起始位置，A1 表示法 (必填)
      --end-index string         源结束位置，A1 表示法 (必填)
      --destination-index string 目标位置，A1 表示法 (必填)
```

startIndex、endIndex 和 destinationIndex 均使用 A1 表示法：`--dimension ROWS` 时为 1-based 行号（如 "2"），`--dimension COLUMNS` 时为列字母（如 "B"）。
源行/列将移动到 destinationIndex 所指的位置。destinationIndex 不能落在源范围 [startIndex, endIndex] 内。

**合并单元格注意**：如果源范围或目标位置涉及合并单元格，操作会报错中断。移动前先通过 `dws sheet info --node <NODE_ID> --sheet-id <SHEET_ID> --format json` 查询 `mergedRanges`，必要时先用 `unmerge-cells` 取消合并再移动，移动后再用 `merge-cells` 恢复需要保留的合并区域。

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

### 创建行或列分组
```
Usage:
  dws sheet group-dimension [flags]
Example:
  # 创建第 3~7 行分组，默认展开
  dws sheet group-dimension --node <NODE_ID> --sheet-id <SHEET_ID> --range "3:7"

  # 创建 C~F 列分组，并在创建后折叠
  dws sheet group-dimension --node <NODE_ID> --sheet-id <SHEET_ID> --range "C:F" --group-state fold

  # 使用工作表前缀（忽略 --sheet-id）
  dws sheet group-dimension --node <NODE_ID> --sheet-id <SHEET_ID> --range "Sheet1!3:7"
Flags:
      --node string          表格文档 ID 或 URL (必填)
      --sheet-id string      工作表 ID 或名称 (必填)
      --range string         整行或整列范围，A1 表示法 (必填)。行如 "3:7"，列如 "C:F"
      --group-state string   创建后的分组状态: expand 或 fold (默认 expand)
```

创建连续整行或整列分组。`--range` 只能是整行范围或整列范围，不支持普通单元格矩形（如 `A1:C5`）。支持单行/单列（如 `"3:3"` / `"C:C"`）以及带工作表前缀（如 `Sheet1!3:7`），带前缀时忽略 `--sheet-id`。

返回中使用 `level` 表示分组展示层级，`level` 为 1-based；不要查找或依赖 `depth`。`--group-state fold` 只用于创建后的初始折叠状态，当前不提供"仅修改已有分组 collapsed 状态"的独立命令。

创建后用以下命令回读：
```bash
dws sheet info --node <NODE_ID> --sheet-id <SHEET_ID> --format json
```

回读字段为 `rowGroups` / `columnGroups`，单项包含 `range`、起止行列、`count`、`level`、`collapsed`。`collapsed=true` 表示当前分组折叠。

### 取消行或列分组
```
Usage:
  dws sheet ungroup-dimension [flags]
Example:
  # 取消第 3~7 行分组
  dws sheet ungroup-dimension --node <NODE_ID> --sheet-id <SHEET_ID> --range "3:7"

  # 取消 C~F 列分组
  dws sheet ungroup-dimension --node <NODE_ID> --sheet-id <SHEET_ID> --range "C:F"
Flags:
      --node string       表格文档 ID 或 URL (必填)
      --sheet-id string   工作表 ID 或名称 (必填)
      --range string      整行或整列范围，A1 表示法 (必填)。行如 "3:7"，列如 "C:F"
```

取消指定连续整行或整列范围上的分组。取消后同样用 `sheet info` 回读确认目标 `range` 已从 `rowGroups` / `columnGroups` 中移除。

`group-dimension` / `ungroup-dimension` 可以放进 `batch-update` 做原子组合；但 batch 中的 `group-dimension` 只适合默认展开分组。需要创建后立即折叠时，请使用独立 `dws sheet group-dimension --group-state fold`。

## 核心工作流

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

## 上下文传递

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `insert-dimension` | `a1Notation` 新插入区域范围 | 确认插入位置和范围 |
| `delete-dimension` | `a1Notation` 被删除区域范围 | 确认删除位置和范围 |
| `update-dimension` | `a1Notation` 被更新区域范围、`hidden` 生效的显隐状态、`pixelSize` 生效的尺寸 | 确认更新结果 |
| `move-dimension` | `sheetId` 工作表 ID | 确认操作完成 |
| `add-dimension` | `sheetId` 工作表 ID | 确认操作完成 |
| `group-dimension` | `range` / `level` / `groupState` | 确认创建的分组范围、层级和初始状态 |
| `ungroup-dimension` | `range` / `level` | 确认取消分组的范围和层级 |
| `list` | 工作表的 `sheetId` | info / range read / range update / find 的 --sheet-id |

## 注意事项

- ★ **`--sheet-id` 获取规范（强制）**：`sheetId` 未知时必须先通过 `dws sheet list --node <NODE_ID> --format json` 查询，禁止凭空编造（如臆测为 `Sheet1`、`sheet1`、`0`、`default` 等）
- `sheet info` 的 `mergedRanges` 是行列结构操作的重要预检信息。插入列时尤其要检查多行表头合并区，原有合并区域通常不会自动扩展到新列，必要时需重新设置合并区域
- `insert-dimension` 在指定位置之前插入空行或空列，不写入数据；如需在末尾追加行/列，使用 `append`；如需"先插入行列再写入数据"的组合操作，使用 [batch-update](./sheet-batch-operations.md) 打包为原子事务
- `insert-dimension` 的 `--dimension` 只接受 `ROWS` 或 `COLUMNS`
- `insert-dimension` 的 `--position` 支持工作表前缀（如 `Sheet1!3`），此时忽略 `--sheet-id`
- `insert-dimension` 的 `--length` 最大为 5000
- `delete-dimension` 从指定位置起删除若干连续的行或列，删除后后续行/列向前移动填补空位
- `delete-dimension` 的 `--dimension` 只接受 `ROWS` 或 `COLUMNS`
- `delete-dimension` 的 `--position` 支持工作表前缀（如 `Sheet1!3`），此时忽略 `--sheet-id`
- `delete-dimension` 的 `--length` 最大为 5000
- `delete-dimension` 若需仅清空内容但保留行/列占位，请使用 `range clear`（默认清除值保留格式，比手动构造空数组更简洁）
- `update-dimension` 批量更新连续行/列的显隐状态与行高/列宽
- `update-dimension` 的 `--dimension` 只接受 `ROWS` 或 `COLUMNS`
- `update-dimension` 的 `--start-index` 支持工作表前缀（如 `Sheet1!3`），此时忽略 `--sheet-id`
- `update-dimension` 的 `--length` 最大为 5000
- `update-dimension` 的 `--hidden` 与 `--pixel-size` 至少必须提供一个
- `update-dimension` 的 `--pixel-size` 单位为像素，`dimension=ROWS` 时表示行高、`dimension=COLUMNS` 时表示列宽
- `update-dimension` 当同时提供 `--hidden` 与 `--pixel-size` 时，将先应用尺寸再应用显隐，任一失败整体失败
- ★ `move-dimension` vs `range update`：需要移动行或列时，必须使用 `move-dimension` 命令，禁止用 `range update` 读取数据后手动重写来模拟移动效果。`move-dimension` 是原子操作，能保留单元格的格式、合并状态等属性
- `move-dimension` 的 `--start-index`、`--end-index` 和 `--destination-index` 均使用 A1 表示法（ROWS 时为 1-based 行号，COLUMNS 时为列字母）
- `move-dimension` 的 `--destination-index` 不能落在源范围 [startIndex, endIndex] 内
- `move-dimension` 的源范围 [startIndex, endIndex] 最大跨度为 5000
- `add-dimension` vs `range update`：需要在末尾追加空行/空列时，必须使用 `add-dimension` 命令，禁止用 `range update` 写空数据来模拟追加效果
- `add-dimension` 追加的是空行/空列，与 `append`（追加带数据的行）不同
- `add-dimension` 的 `--length` 必须为正整数（>= 1），行列均不超过 5000
- `group-dimension` / `ungroup-dimension` 的 `--range` 只接受整行或整列范围，不接受普通单元格矩形
- `group-dimension` 输出和 `sheet info` 回读均使用 `level`，且为 1-based；不要使用旧的 `depth` 字段
- `sheet info` 是分组回读入口；`range read` / `csv-get` 不返回行列分组
- 当前不能直接调整已有分组的 `collapsed` 状态；`--group-state fold` 只在创建分组时生效
- `batch-update` 支持 `group-dimension` / `ungroup-dimension`，但不适合创建后立即折叠；需要 `fold` 时使用独立 `group-dimension`
