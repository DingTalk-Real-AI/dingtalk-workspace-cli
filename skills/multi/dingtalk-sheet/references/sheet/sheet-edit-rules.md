# Agent 编辑总则

## Agent 编辑总则

1. **先定位再编辑**：未知 `sheet-id` 时必须先 `dws sheet list --node <ID> --format json`，禁止猜 `Sheet1` / `sheet1` / `0` / `default`。用户只给工作表名称时，也先用 `list` 确认真实名称或 ID。
2. **先读结构再动结构**：合并、冻结、行列分组、最后非空边界都属于工作表结构信息，先读 `dws sheet info --node <ID> --sheet-id <SHEET_ID> --format json`。
3. **按目的选择读取方式**：快速看值和大表分批用 `csv-get`；需要公式、样式、数据验证、超链接、富文本等 per-cell 元数据用 `range read`。
4. **区分写入成功与内容验收**：`success` / `updatedRows` / `updatedCells` 可确认写请求成功；用户要求内容准确性验收，或涉及复杂公式、格式、结构变更时，再用 `csv-get` / `range read` / `sheet info` 或对应 `list` / `get` 回读。用户明确禁止额外读取时不强制回读。
5. **大批量纯值不要拼大 JSON**：超过 5 行或 20 个单元格的纯值写入优先 `csv-put`。
6. **公式是有限回读，不是零错误工具**：写公式前读 [sheet-formula](./sheet-formula.md)。当前没有聚合式 `formula-verify`，只能回读公式文本和计算结果，并检查明显错误值。
7. **专用操作用专用命令**：搜索用 `find`、替换用 `replace`、清空用 `range clear`、排序用 `range sort`、复制/移动区域用 `range copy-to` / `range move-to`，不要用 `range read` + `range update` 客户端模拟。
8. **大整数按文本写**：超过 `9007199254740991` 的整数、长数字 ID、订单号、手机号等需要逐位精确的值，不要按 JSON number 或 `int64` / `uint64` 写入；用字符串值 + `object` dtype + 文本格式 `@`。
