# Sheet 读取数据

## 读取路径

| 需求 | 首选 | 结果形态 |
|---|---|---|
| Agent 快速查看值、低 token | dws sheet csv-get | CSV 文本加范围/截断元数据 |
| 必须完整且截断即失败 | dws sheet +read | 严格完整读取 |
| columns/data/dtypes/formats | dws sheet table-get | typed table/dataframe |
| 需要公式、富文本、链接、验证等逐格结构 | dws sheet range read | per-cell JSON |
| 合并、冻结、分组和尺寸 | dws sheet info | 工作表结构 |

当前没有 sheetId 时只执行一次 +list-sheets，并把真实 sheetId 传播到后续阶段。不要为不需要 sheetId 的命令额外探活。

## CSV 快速读取

    dws sheet csv-get       --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:H200"       --format json

返回契约必须按字段解释：

- csv 每行带 `[row=N]` 定位前缀，该前缀不是单元格数据；真正 CSV 内容仍按 RFC 4180 解析。
- `rowIndices[i]` 是第 i 个返回行的真实行号，`colIndices[j]` 是第 j 列的真实列字母。后续定位单元格必须使用这两个映射，禁止通过 CSV 中逗号数量推算列号。
- `resolvedRange` 是未显式传 range 时服务端解析出的完整目标范围；`returnedRange` 是本次实际完整返回的范围，两者不能混用。

读取成功后必须检查：

- returnedRange 是否覆盖请求范围；
- hasMore 是否为 false；
- truncationReasons 是否为空；
- 返回行列数是否符合预期。

csv-get 单次最多 30000 单元格，并受 maxChars 约束。hasMore=true、范围缩短或出现截断原因时，只能说“当前块已读取”，不能说全量完成；按 returnedRange 的下一行构造下一块，保证无遗漏、无重叠并设置页数/块数上限。`max_cells` 不能靠增大 maxChars 解决。需要失败关闭时直接用 +read，避免由 Agent 手工实现完整性判断。

`forbidden.document.sizeOverLimit` 表示工作簿整体无法装载，不是范围过大或空结果；缩小 range 不能修复。应建议创建更小副本或拆分工作簿，不得不断缩小范围绕过。

csv-get 不返回合并单元格结构。合并区域的非左上角为空不能推导“没有内容”，需要 info 的 mergedRanges 配合解释。

## 渲染选项

- formatted_value：面向展示，可能包含格式化日期、货币或百分比。
- raw_value：需要计算值或保留数值语义时使用。
- formula：需要确认写入的公式文本时使用。

公式验证通常分两次：先 formula 确认文本，再 raw_value 或 formula-verify 检查计算。不要把展示字符串当作原始数值，也不要把公式字符串当计算结果。

## typed table 与逐格读取

table-get 用于后续明确需要 columns、data、dtypes、formats 的处理链；普通问答不应为了“结构化”增加 token。默认首行作为表头，确实没有表头时才用 `--no-header`；返回 data 中 `{}` 表示空位，不是待执行的单元格对象。table-get 不返回逐格超链接、验证或样式元数据。

range read 只在 CSV 无法承载的 per-cell 信息确实需要时调用，并尽量缩小范围和返回配置。其 cells 与 `rowIndices`/`colIndices` 对齐，可返回 value/formula/richText/hyperlink/dataValidation/cellStyles 等逐格结构；不能用它推断 mergedRanges。

    dws sheet table-get --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:D100" --format json

    dws sheet range read --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:B5" --format json

如果命令返回非零、统一 envelope 的 ok=false、JSON 解析失败、条数不一致或分页游标不前进，立即失败。只有明确成功且终止状态成立时，空数组/空 CSV 才代表真实空结果。

## 读取完成条件

最终答复前确认：profile 一致；ID 来自当前任务；所有块都已覆盖；无 hasMore、截断、重复块或游标停滞；涉及结构时另有 info/object list 证据。只报告已验证范围，不夸大到整张表。
