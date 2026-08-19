# 下拉列表 (dropdown)

## 使用场景

### 下拉列表

用户说"设置下拉列表/下拉选项/下拉菜单/添加下拉/配置下拉":
- 设置下拉列表 → `set-dropdown`
- 设置多选下拉 → `set-dropdown --multi-select`
- 引用单元格区域作为候选项 → `set-dropdown --source-sheet-id ... --source-range ...`

用户说"查看下拉列表/获取下拉配置/下拉列表有哪些选项":
- 获取下拉列表配置 → `get-dropdown`

用户说"删除下拉列表/移除下拉/取消下拉/清除下拉":
- 删除下拉列表 → `delete-dropdown`

## 命令详细参考

### 设置下拉列表
```
Usage:
  dws sheet set-dropdown [flags]
Example:
  # 设置单选下拉列表
  dws sheet set-dropdown --node <NODE_ID> --sheet-id <SHEET_ID> --range "A2:A100" \
    --options '[{"value":"选项1"},{"value":"选项2"},{"value":"选项3"}]'

  # 设置带颜色的多选下拉列表
  dws sheet set-dropdown --node <NODE_ID> --sheet-id <SHEET_ID> --range "B2:B50" \
    --options '[{"value":"高","color":"#ff0000"},{"value":"中","color":"#ffaa00"},{"value":"低","color":"#00ff00"}]' \
    --multi-select

  # 引用同一工作簿内另一工作表的区域作为候选项来源
  dws sheet set-dropdown --node <NODE_ID> --sheet-id <TARGET_SHEET_ID> --range "C2:C100" \
    --source-sheet-id <SOURCE_SHEET_ID> --source-range "T1:T3"
Flags:
      --node string              表格文档 ID 或 URL (必填)
      --sheet-id string          工作表 ID 或名称 (必填)
      --range string             目标单元格范围，A1 表示法，如 A2:A100 (必填)
      --options string           Inline 下拉选项 JSON 数组，与 --source-range 二选一
      --source-sheet-id string   SourceRange 来源工作表 ID，与 --source-range 同时指定
      --source-range string      SourceRange 来源区域，与 --options 二选一；不带工作表前缀
      --multi-select             是否允许多选（默认单选）
```

在指定单元格范围内设置下拉列表。Inline 模式直接存储选项；SourceRange 模式引用同一工作簿内的来源区域，可跨工作表，并支持普通区域、整行和整列。
- **用途**：为单元格配置静态选项或区域来源下拉，两种模式都支持多选；颜色仅 Inline 支持。
- **场景**：规范数据输入，如状态选择（完成/进行中/待处理）、优先级（高/中/低）等。
- **注意**：`--options` 与 `--source-range` 必须且只能指定一个。`--source-range` 只写 `T1:T3`、`T:T`、`1:3` 这类 A1 区域，来源工作表通过 `--source-sheet-id` 单独指定；不接受工作表前缀、公式或多区域。SourceRange 颜色写入暂不支持。
- **结构操作行为**：已验证的工作表重命名、在引用前插入行/列、删除引用前的行会自动调整引用并保持 `valid`；已验证的 `move-dimension` 场景会使其变为 `invalid`。列删除、删除整个来源区域或来源工作表等场景未覆盖，不能预设结果；结构操作后先回读 `sourceRangeStatus`，仅在 `invalid` 时重新选择来源并写入。

### 获取下拉列表配置
```
Usage:
  dws sheet get-dropdown [flags]
Example:
  dws sheet get-dropdown --node <NODE_ID> --sheet-id <SHEET_ID> --range "A2:A100"
  dws sheet get-dropdown --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1"
Flags:
      --node string       表格文档 ID 或 URL (必填)
      --sheet-id string   工作表 ID 或名称 (必填)
      --range string      查询范围，A1 表示法，如 A1:A100 (必填)
```

查询指定范围内的下拉列表配置信息。
- **用途**：查看单元格已设置的下拉列表选项和配置。
- **场景**：在修改下拉列表前先查询现有配置；确认下拉列表是否设置成功。
- **返回**：`dataValidations` 数组由底层按配置聚合。Inline 组返回 `sourceType:"inline"`、`conditionValues`、`ranges` 和 `options`；SourceRange 组始终返回 `sourceType:"sourceRange"`、`sourceRangeStatus:"valid"/"invalid"`、`enableMultiSelect` 和 `ranges`，仅在 `sourceRangeStatus:"valid"` 时返回 `sourceRange:{sheetId,a1Notation}`。`invalid` 时仍保留配置组，但省略 `sourceRange`，不得依赖旧坐标修复。SourceRange 不会展开候选值，因此不返回 `conditionValues`、`options` 或颜色。范围内无下拉列表时 `hasDropdown` 为 false。

### 删除下拉列表
```
Usage:
  dws sheet delete-dropdown [flags]
Example:
  dws sheet delete-dropdown --node <NODE_ID> --sheet-id <SHEET_ID> --range "A2:A100"
  dws sheet delete-dropdown --node <NODE_ID> --sheet-id <SHEET_ID> --range "B1:D10"
Flags:
      --node string       表格文档 ID 或 URL (必填)
      --sheet-id string   工作表 ID 或名称 (必填)
      --range string      要删除下拉列表的范围，A1 表示法 (必填)
```

删除指定范围内的下拉列表配置，单元格恢复为普通文本格式。
- **用途**：移除不再需要的下拉列表约束。
- **注意**：已填写的单元格值不会被清除；目标范围不存在下拉列表时操作仍返回成功。

## 上下文传递

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `set-dropdown` | `range` 实际设置范围、`enableMultiSelect` 是否多选；仅 Inline 模式返回 `optionCount` | 确认下拉列表设置成功 |
| `get-dropdown` | `hasDropdown`、`dataValidations`；按 `sourceType` 区分 Inline 与 SourceRange | 查看已有下拉配置 |
| `delete-dropdown` | `range` 实际删除范围 | 确认下拉列表删除完成 |
| `list` | 工作表的 `sheetId` | info / range read / range update / find 的 --sheet-id |

## 注意事项

- ★ **`--sheet-id` 获取规范（强制）**：`sheetId` 未知时必须先通过 `dws sheet list --node <NODE_ID> --format json` 查询，禁止凭空编造（如臆测为 `Sheet1`、`sheet1`、`0`、`default` 等）
- `set-dropdown` 的 Inline 模式使用 `--options`，每个元素包含 `value`（必填）和 `color`（可选，`#RRGGBB`）；SourceRange 模式使用 `--source-sheet-id` + `--source-range`。两种模式均可用 `--multi-select`，并会覆盖目标范围已有下拉
- SourceRange 在已验证的重命名、引用前插入行/列、删除引用前行的场景会自动调整；已验证的 `move-dimension` 会使其变为 `invalid`。其他未覆盖删除/移动场景后先回读，仅 `invalid` 时重新选源写入；颜色写入暂不支持
- `get-dropdown` 查询指定范围内的下拉配置，聚合发生在底层服务。SourceRange 即使无效也保留一组并以 `sourceRangeStatus:"invalid"` 表示，但省略 `sourceRange`，不回退展开候选值
- `delete-dropdown` 删除指定范围内的下拉列表配置，单元格恢复为普通文本格式。已填写的值不会被清除。目标范围不存在下拉列表时操作仍返回成功
