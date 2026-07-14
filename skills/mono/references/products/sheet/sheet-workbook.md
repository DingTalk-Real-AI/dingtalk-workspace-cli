# 表格与工作表管理

## 使用场景

用户说"创建表格/新建电子表格":
- 创建表格文档 → `create`

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

用户说"删除工作表/移除工作表/删掉这个Sheet":
- 删除工作表 → `delete-sheet`（不可逆操作，执行前必须向用户确认）

用户说"显示网格线/隐藏网格线/去掉单元格网格":
- 显示网格线 → `show-gridline`
- 隐藏网格线 → `hide-gridline`

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

返回字段中 `mergedRanges` 是当前工作表的合并单元格范围列表（A1 表示法，如 `["C7:D11"]`）。它属于工作表结构/布局元数据：读写单元格内容前，如需判断表头、分组标题、续写位置或避开合并冲突，应先看 `sheet info`，不要在 `range read` / `csv-get` 的单元格值里寻找合并信息。

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
      --title string             --name 的别名（兼容）
      --index int                新位置（从 0 开始）
      --hidden                   --hidden=true 隐藏，--hidden=false 取消隐藏
      --tab-color string         工作表标签颜色，Hex 如 #FF0000；传空字符串清除颜色
      --frozen-row-count int     冻结行数，0 表示取消冻结
      --frozen-column-count int  冻结列数，0 表示取消冻结
```

更新工作表名称、位置、隐藏状态、标签颜色、冻结行列。
`--name`（别名 `--title`）/ `--index` / `--hidden` / `--tab-color` / `--frozen-row-count` / `--frozen-column-count` 至少提供一个；多个属性可同时传入，将在同一次请求中更新。

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

### 显示或隐藏网格线
```
Usage:
  dws sheet show-gridline --node <NODE_ID> --sheet-id <SHEET_ID>
  dws sheet hide-gridline --node <NODE_ID> --sheet-id <SHEET_ID>
Flags:
      --node string       表格文档 ID 或 URL (必填)
      --sheet-id string   工作表 ID 或名称 (必填)
```

这两个命令只修改指定工作表的网格线显示状态，不修改单元格内容或边框样式。执行后可用同一命令反向恢复。

## 核心工作流

```bash
# ── 工作流 1: 创建表格并写入数据 ──

# 1. 创建表格文档 — 提取 nodeId
dws sheet create --name "销售数据" --format json

# 2. 查看工作表列表 — 提取 sheetId
dws sheet list --node <NODE_ID> --format json

# 3. 写入表头和数据（每个单元格必须是 object；数字也写成字符串）
dws sheet range update --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:C1" \
  --values '[[{"type":"text","text":"姓名"},{"type":"text","text":"部门"},{"type":"text","text":"销售额"}]]' --format json

dws sheet range update --node <NODE_ID> --sheet-id <SHEET_ID> --range "A2:C4" \
  --values '[[{"type":"text","text":"张三"},{"type":"text","text":"销售部"},{"type":"text","text":"50000"}],[{"type":"text","text":"李四"},{"type":"text","text":"市场部"},{"type":"text","text":"38000"}],[{"type":"text","text":"王五"},{"type":"text","text":"销售部"},{"type":"text","text":"62000"}]]' --format json

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

# 2. 在新工作表中写入汇总公式（公式写在 text，以 = 开头）
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
| `info` | `rowCount` / `nonEmptyRange.range` / `nonEmptyRange.lastRow` / `nonEmptyRange.lastColumn` / `mergedRanges` | 确定数据范围、追加写入起始行、判断合并单元格结构 |

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
- ★ 关键区分: sheet(电子表格/单元格读写) vs aitable(AI多维表/结构化记录/字段定义) vs doc(文档编辑/阅读)
