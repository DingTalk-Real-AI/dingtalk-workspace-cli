# 全局硬约束

## 全局硬约束

1. **`--sheet-id` 禁止臆测**：未知时必须 `dws sheet list --node <ID> --format json` 查询，禁止编造 `Sheet1`/`sheet1`/`0`/`default`
2. **合并单元格是结构信息**：`dws sheet info --node <ID> --sheet-id <SHEET_ID> --format json` 返回 `mergedRanges`（如 `["C7:D11"]`）；不要在 `range read` / `csv-get` 里寻找合并信息
3. **冻结行列是工作表元数据**：`sheet info` 顶层返回 `frozenRowCount` / `frozenColumnCount`，分别表示从顶部第 1 行、左侧第 A 列开始冻结的数量；`0` 表示未冻结。不要在 `range read` / `csv-get` 里寻找冻结信息
4. **行列分组必须回读**：创建/取消分组后，用 `dws sheet info --node <ID> --sheet-id <SHEET_ID> --format json` 回读 `rowGroups` / `columnGroups`。分组项返回 `range`、起止行列、`count`、`level`、`collapsed`；`level` 是 1-based 展示层级，不使用 `depth`
5. **最后非空坐标只用 A1 语义**：`sheet info` 通过 `nonEmptyRange` 返回 A1/UI 边界；优先使用 `nonEmptyRange.range`，需要追加行/列时使用 `nonEmptyRange.lastRow` / `nonEmptyRange.lastColumn`。不要使用旧的 0-based 字段 `lastNonEmptyRow` / `lastNonEmptyColumn`
6. **`range update` 维度校验**：`--values` 行列数必须与 `--range` 完全一致；只接 `--values` 一个数据参数，cell `type` 仅支持 `text` / `richText`；整格超链接通过 cell-level `hyperlink` 表达，富文本片段链接才使用 `richText.texts[].type="link"`
7. **dataValidation 三语义**：不传 `dataValidation` 字段=保留原 DV；`dataValidation:{type:"none"}`=显式清除；`dataValidation:{type:"dropdown"/"checkbox",...}`=覆盖。`{}` 跳过亦保留原 DV
8. **hyperlink 三语义**：不传 `hyperlink` 字段=保留原整格超链接；`hyperlink:{type:"none"}`=显式清除；`hyperlink:{type:"path"/"sheet"/"range",link,...}`=覆盖。Agent 调用不要用 `hyperlink:null`，避免网关/Schema 过滤 null 字段
9. **样式写法**：cell-level 样式用 `cellStyles` 或 `range set-style`；richText 片段级样式才用子项 `style`。不要在 `type:"text"` 顶层使用旧 `style` 字段
10. **用专用命令不用组合模拟**：搜索→`find`、替换→`replace`、清空→`range clear`、排序→`range sort`、填充→`range fill`、复制区域→`range copy-to`、移动区域→`range move-to`、移动行列→`move-dimension`
11. **大批量纯值用 `csv-put`**（>5 行或 >20 单元格），不用 `range update`
12. **单元格图片用 `write-image`**（`range update` 不支持图片参数）
13. **`export` 禁止自行轮询**（CLI 内部已完成渐进式退避，最多 30 次约 5 分钟）
14. **单次调用上限**：`range update` / `set-style` 行数 ≤ 1000，单元格总数建议 ≤ 5000（硬限 30000）
15. **大整数精度保护**：超过 `9007199254740991` 的整数和长数字标识符必须按字符串写入；需要固定文本格式时使用 `cellStyles.numberFormat: "@"`
16. **批量写操作推荐用 `batch-update`**：对多个区域重复调用同一写入工具时，推荐合并为单次原子请求（详见 [sheet-batch-operations](./sheet-batch-operations.md)）；逐个调用非原子，中途失败会留下半成品
17. **关键区分**：sheet（电子表格/单元格读写）vs aitable（AI多维表/结构化记录）vs doc（文档）
