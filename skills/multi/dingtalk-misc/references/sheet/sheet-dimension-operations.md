# Sheet 行列操作

## 适用命令

| 目标 | 命令 |
|---|---|
| 在位置前插入 | insert-dimension |
| 删除连续行/列 | delete-dimension |
| 隐藏、显示、调整尺寸 | update-dimension |
| 移动连续行/列 | move-dimension |
| 末尾追加空行/列 | add-dimension |
| 创建/取消分组 | group-dimension / ungroup-dimension |

所有命令前缀均为 dws sheet，并要求当前任务真实 nodeId、sheetId。

## 坐标规则

- dimension 只取 ROWS 或 COLUMNS。
- ROWS 的 position/start-index/end-index/destination-index 使用 1 起始行号字符串，如 "3"。
- COLUMNS 使用列字母，如 "A"、"AB"。
- 这些参数不是 A1 矩形范围，不要传 A1:C5。
- 分组范围必须是整行 "3:7" 或整列 "C:F"；普通矩形无效。
- insert/delete/update/add-dimension 的 length 都是正整数，单次最多 5000。
- position/start-index/range 带工作表前缀时，以前缀解析出的工作表为准并忽略 sheet-id；只在确有跨表坐标需求时使用，避免名称歧义。

## 操作前检查

插入、删除、移动之前先执行：

    dws sheet info --node <NODE_ID> --sheet-id <SHEET_ID> --format json

检查 mergedRanges、冻结、隐藏和现有 groups。若合并区域跨过操作位置，先说明影响；必要时明确取消合并，完成结构操作后按原意恢复。删除、移动会改变公式引用、命名范围和后续坐标，下一阶段必须使用更新后的范围。

move-dimension 的 start/end 为包含端点的连续源区间；destination-index 不能落在源区间内。向下/向右移动时目标应大于 end，向上/向左时目标应小于 start。涉及合并单元格的移动可能失败；不得改用“读出、删除、写回”模拟移动，先记录 mergedRanges，必要时取消合并并在操作后恢复。

删除行列属于不可逆结构修改。展示维度、起点、长度及可能影响，获得明确同意后才执行需要的确认参数。

## 精确示例

    dws sheet insert-dimension       --node <NODE_ID> --sheet-id <SHEET_ID>       --dimension ROWS --position "3" --length 2 --format json

    dws sheet update-dimension       --node <NODE_ID> --sheet-id <SHEET_ID>       --dimension COLUMNS --start-index "C" --length 1       --pixel-size 200 --hidden --format json

    dws sheet move-dimension       --node <NODE_ID> --sheet-id <SHEET_ID>       --dimension ROWS --start-index "2" --end-index "4"       --destination-index "1" --format json

update-dimension 的尺寸模式：pixel 必须配合非负 `--pixel-size`；standard 恢复默认尺寸且不能同时传 pixel-size；auto 按内容自适应且只支持 ROWS，也不能同时传 pixel-size。只改 hidden 时省略 size-type/pixel-size；只改尺寸时不要顺带提交 hidden。

## 分组

    dws sheet group-dimension       --node <NODE_ID> --sheet-id <SHEET_ID>       --range "3:7" --group-state fold --format json

    dws sheet info       --node <NODE_ID> --sheet-id <SHEET_ID>       --include groups --format json

分组后在 rowGroups/columnGroups 中按 range 验证；collapsed=true 表示折叠。group/ungroup 可进入 batch-update，但 batch 内只使用默认展开；要求创建后立即折叠时单独调用 group-dimension --group-state fold。

group-state 只决定新建分组的初始 expand/fold 状态。当前没有“只修改已有分组 collapsed 状态”的独立命令；不要通过再次 group 猜测为状态更新，也不要从 groups 的 level/depth 推导可写参数。

## 完成条件

结构写响应后必须重新 info；验证行列数量、隐藏/尺寸、mergedRanges 和 groups 与预期一致。若坐标变化，向后续阶段传播新范围，不能继续使用修改前的 A1 地址。
