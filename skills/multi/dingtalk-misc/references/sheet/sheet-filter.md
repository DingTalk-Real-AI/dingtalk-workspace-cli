# 全局筛选 (filter)

## 使用场景

### 筛选视图

用户说"筛选/过滤/只看某些值/只显示满足条件的行/筛选数据/创建筛选/删除筛选/设置筛选条件/清除筛选/排序":
- 查看当前筛选 → `filter get`
- 创建筛选 → `filter create`
- 删除筛选 → `filter delete`
- 批量设置多列条件 → `filter update`
- 清除某一列条件 → `filter clear-criteria`
- 按列排序 → `filter sort`
- **区分全局筛选与筛选视图**：如果用户说"筛选视图"则走 `filter-view` 系列；如果只说"筛选/过滤/只看"则默认走全局 `filter` 系列
- **禁止替代方案**：当用户要求"筛选/只看/仅保留某些行"时，必须通过 `filter create` / `filter update` 创建真实的筛选器。禁止用"删除不符合条件的行"或"新建工作表只放符合条件的行"来代替——这些做法会让原数据丢失或不可恢复

## 命令详细参考

### 获取筛选信息
```
Usage:
  dws sheet filter get [flags]
Example:
  dws sheet filter get --node <NODE_ID> --sheet-id <SHEET_ID>
  dws sheet filter get --node "https://alidocs.dingtalk.com/i/nodes/<DOC_UUID>" --sheet-id "Sheet1"
Flags:
      --node string       表格文档 ID 或 URL (必填)
      --sheet-id string   工作表 ID 或名称 (必填)
```

获取指定工作表的全局筛选信息，返回筛选范围和各列的筛选条件详情。
- **用途**：查看当前工作表上是否存在全局筛选及其配置。
- **场景**：在修改或删除筛选前，先读取当前筛选配置；创建筛选前先确认是否已存在（每个工作表只能有一个筛选）。
- **区分**：全局筛选（filter）影响所有协作者看到的数据展示；筛选视图（filter-view）是个人化的。
- **返回**：`range`（筛选范围，A1 表示法）和 `columnFilterCriteria`（各列条件，key 为列偏移量）。如果未设置筛选，返回筛选信息为空。

### 创建筛选
```
Usage:
  dws sheet filter create [flags]
Example:
  # 创建筛选框架（不设条件）
  dws sheet filter create --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:E100"

  # 创建筛选并同时设置条件（按值筛选）
  dws sheet filter create --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:E100" --criteria '[{"column":1,"filterType":"values","visibleValues":["北京","上海"]}]'

  # 创建筛选并设置条件筛选
  dws sheet filter create --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:E100" --criteria '[{"column":2,"filterType":"condition","conditions":[{"operator":"greater","value":"100"}]}]'
Flags:
      --node string       表格文档 ID 或 URL (必填)
      --sheet-id string   工作表 ID 或名称 (必填)
      --range string      筛选范围，A1 表示法，须包含表头行 (必填)
      --criteria string   筛选条件 JSON 数组 (可选)
```

在工作表中创建全局筛选。
- **用途**：为工作表建立筛选器，使数据可按条件过滤展示。
- **约束**：每个工作表只能有一个全局筛选，已存在时会报错。应先 `filter get` 确认不存在后再创建。
- **range 规范**：必须包含表头行（如 `A1:E100`），不能只包含数据行。
- **criteria 格式**：JSON 数组，每个元素含 `column`（列偏移量，从 0 开始）和筛选条件字段。不传则仅创建空筛选框架，后续可通过 `filter update` 设置条件。

### 删除筛选

> **CAUTION:** 不可逆操作 — 执行前必须向用户确认。

```
Usage:
  dws sheet filter delete [flags]
Example:
  dws sheet filter delete --node <NODE_ID> --sheet-id <SHEET_ID> --yes
Flags:
      --node string       表格文档 ID 或 URL (必填)
      --sheet-id string   工作表 ID 或名称 (必填)
```

删除工作表的全局筛选。
- **用途**：移除筛选器，所有被隐藏的行将重新显示。
- **不可逆**：删除后所有筛选条件丢失，需重新创建。
- **前置**：工作表没有筛选时调用会报错，应先 `filter get` 确认存在。

### 批量更新筛选条件
```
Usage:
  dws sheet filter update [flags]
Example:
  # 同时设置多列的筛选条件
  dws sheet filter update --node <NODE_ID> --sheet-id <SHEET_ID> --criteria '[{"column":0,"filterType":"values","visibleValues":["已完成","进行中"]},{"column":2,"filterType":"condition","conditions":[{"operator":"greater","value":"50"}]}]'

  # 按颜色筛选
  dws sheet filter update --node <NODE_ID> --sheet-id <SHEET_ID> --criteria '[{"column":1,"filterType":"color","backgroundColor":"#FF0000"}]'
Flags:
      --node string       表格文档 ID 或 URL (必填)
      --sheet-id string   工作表 ID 或名称 (必填)
      --criteria string   筛选条件 JSON 数组 (必填)
```

批量更新筛选条件，可同时设置多列的筛选条件。
- **用途**：一次性设置或替换多列的筛选条件。
- **前置**：工作表必须已创建筛选（通过 `filter create`）。
- **覆盖式**：指定列的条件会被替换，未指定的列保持不变。如只想修改某一列，建议先 `filter get` 读取现有配置。
- **criteria 格式**：JSON 数组，支持三种 `filterType`：
  - `values`：按值筛选，指定 `visibleValues` 数组
  - `condition`：按条件筛选，指定 `conditions` 数组（最多 2 个）和可选的 `conditionOperator`（`and`/`or`）
  - `color`：按颜色筛选，指定 `backgroundColor` 或 `fontColor`（二选一）

### 清除单列筛选条件
```
Usage:
  dws sheet filter clear-criteria [flags]
Example:
  # 清除第 2 列（B 列）的筛选条件
  dws sheet filter clear-criteria --node <NODE_ID> --sheet-id <SHEET_ID> --column 1

  # 清除第 1 列（A 列）的筛选条件
  dws sheet filter clear-criteria --node <NODE_ID> --sheet-id <SHEET_ID> --column 0
Flags:
      --node string       表格文档 ID 或 URL (必填)
      --sheet-id string   工作表 ID 或名称 (必填)
      --column number     列偏移量，从 0 开始 (必填)
```

清除筛选中某一列的筛选条件。
- **用途**：移除某列的筛选条件，该列不再参与筛选计算。
- **区分**：仅清除指定列的条件，不删除整个筛选。如需删除整个筛选，使用 `filter delete`。
- **幂等**：指定列没有设置筛选条件时调用不会报错。

### 筛选排序
```
Usage:
  dws sheet filter sort [flags]
Example:
  # 按第 1 列（A 列）升序排序
  dws sheet filter sort --node <NODE_ID> --sheet-id <SHEET_ID> --column 0 --ascending

  # 按第 3 列（C 列）降序排序
  dws sheet filter sort --node <NODE_ID> --sheet-id <SHEET_ID> --column 2 --ascending=false
Flags:
      --node string       表格文档 ID 或 URL (必填)
      --sheet-id string   工作表 ID 或名称 (必填)
      --column number     排序列偏移量，从 0 开始 (必填)
      --ascending         是否升序，默认 true (可选)
```

对筛选范围内的数据按指定列排序。
- **用途**：对数据行按某一列的值进行升序或降序排列。
- **前置**：工作表必须已创建筛选（通过 `filter create`）。
- **注意**：排序会实际改变工作表中数据行的物理顺序，不可撤销。
- **column**：列偏移量从 0 开始，相对于筛选范围首列。

## 上下文传递

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `filter get` | `range`（筛选范围）、`columnFilterCriteria`（各列条件） | 查看当前筛选配置，确认筛选是否存在 |
| `filter create` | 筛选创建成功的确认 | 确认筛选已建立，后续可通过 `filter update` 设置条件 |
| `filter delete` | 删除成功的确认 | 确认筛选已删除 |
| `filter update` | 更新成功的确认 | 确认条件已设置 |
| `filter clear-criteria` | 清除成功的确认 | 确认指定列的条件已清除 |
| `filter sort` | 排序成功的确认 | 确认排序已完成 |
| `list` | 工作表的 `sheetId` | info / range read / range update / find 的 --sheet-id |

## 注意事项

- ★ **`--sheet-id` 获取规范（强制）**：`sheetId` 未知时必须先通过 `dws sheet list --node <NODE_ID> --format json` 查询，禁止凭空编造（如臆测为 `Sheet1`、`sheet1`、`0`、`default` 等）
- ★ **全局筛选（filter）与筛选视图（filter-view）的区别**：全局筛选影响所有协作者看到的数据展示，每个工作表最多一个；筛选视图是个人化的，互不影响。用户只说"筛选"时默认走 `filter` 系列
- `filter get` 获取工作表的全局筛选信息，返回 `range`（筛选范围）和 `columnFilterCriteria`（各列条件）。无筛选时返回空
- `filter create` 创建全局筛选时 `--range` 必须包含表头行（如 `A1:E100`），不能只包含数据行。每个工作表只能有一个筛选，已存在时报错
- `filter create` 的 `--criteria` 可选，不传则仅创建空筛选框架，后续通过 `filter update` 设置条件
- `filter delete` 删除后所有筛选条件丢失且所有被隐藏行重新显示，不可恢复
- `filter delete` 工作表没有筛选时调用会报错，应先 `filter get` 确认存在
- `filter update` 是覆盖式：指定列的条件会被替换，未指定的列保持不变。如只想修改某一列，建议先 `filter get` 读取现有配置再 patch
- `filter update` 前置：工作表必须已创建筛选
- `filter clear-criteria` 仅清除指定列的条件，不删除整个筛选。指定列无条件时不报错（幂等）
- `filter sort` 会实际改变数据行的物理顺序，不可撤销。前置：工作表必须已创建筛选
- ★ **筛选操作规范**（参照飞书 core-operations）：
  - 当用户要求"筛选/只看/仅保留 X"时，**必须**通过 `filter create` / `filter update` 创建真实的筛选器。**禁止**用"删除不符合条件的行"或"新建工作表只放符合条件的行"来代替
  - 创建/更新筛选后**必须** `filter get` 回读验证配置正确
  - 更新已有筛选前先 `filter get` 读取当前配置，确认目标存在且了解现有条件后再操作
  - 筛选条件的列索引（`column`）必须与实际数据列精确对应，不要凭猜测填写
  - 筛选不支持正则表达式，传入正则会当成普通文本处理
