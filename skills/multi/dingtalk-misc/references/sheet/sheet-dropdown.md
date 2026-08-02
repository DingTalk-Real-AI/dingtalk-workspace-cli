# 下拉列表 (dropdown)

## 使用场景

### 下拉列表

用户说"设置下拉列表/下拉选项/下拉菜单/添加下拉/配置下拉":
- 设置下拉列表 → `set-dropdown`
- 设置多选下拉 → `set-dropdown --multi-select`

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
Flags:
      --node string         表格文档 ID 或 URL (必填)
      --sheet-id string     工作表 ID 或名称 (必填)
      --range string        目标单元格范围，A1 表示法，如 A2:A100 (必填)
      --options string      下拉选项 JSON 数组 (必填)，如 '[{"value":"选项1","color":"#ff0000"}]'
      --multi-select        是否允许多选（默认单选）
```

在指定单元格范围内设置下拉列表。设置后用户可从预定义选项中选择值。
- **用途**：为单元格配置下拉列表，支持自定义选项颜色和多选。
- **场景**：规范数据输入，如状态选择（完成/进行中/待处理）、优先级（高/中/低）等。
- **注意**：选项值不能包含英文逗号；如果目标范围已存在下拉列表，会被新配置覆盖。

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

查询指定范围内的下拉列表配置信息，包括选项值、颜色和是否多选。
- **用途**：查看单元格已设置的下拉列表选项和配置。
- **场景**：在修改下拉列表前先查询现有配置；确认下拉列表是否设置成功。
- **返回**：`dataValidations` 数组，相同选项的单元格聚合为一组，每组包含 `conditionValues`（选项值）、`ranges`（覆盖范围）、`options`（含 `enableMultiSelect` 和 `colorValueMap`）。范围内无下拉列表时 `hasDropdown` 为 false。

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
| `set-dropdown` | `range` 实际设置范围、`optionCount` 选项数量、`enableMultiSelect` 是否多选 | 确认下拉列表设置成功 |
| `get-dropdown` | `hasDropdown` 是否存在下拉、`dataValidations` 下拉配置列表（含 `conditionValues`、`ranges`、`options`） | 查看已有下拉配置 |
| `delete-dropdown` | `range` 实际删除范围 | 确认下拉列表删除完成 |
| `list` | 工作表的 `sheetId` | info / range read / range update / find 的 --sheet-id |

## 注意事项

- ★ **`--sheet-id` 获取规范（强制）**：`sheetId` 未知时必须先通过 `dws sheet list --node <NODE_ID> --format json` 查询，禁止凭空编造（如臆测为 `Sheet1`、`sheet1`、`0`、`default` 等）
- `set-dropdown` 在指定范围内设置下拉列表，`--options` 为 JSON 数组，每个元素包含 `value`（必填）和 `color`（可选，`#RRGGBB` 格式）。选项值不能包含英文逗号。`--multi-select` 启用多选模式。如果目标范围已存在下拉列表，会被新配置覆盖
- `get-dropdown` 查询指定范围内的下拉列表配置，返回 `dataValidations` 数组，相同选项的单元格聚合为一组。无下拉列表时 `hasDropdown` 为 false
- `delete-dropdown` 删除指定范围内的下拉列表配置，单元格恢复为普通文本格式。已填写的值不会被清除。目标范围不存在下拉列表时操作仍返回成功
