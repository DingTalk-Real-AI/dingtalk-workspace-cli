# 公式写入、回读与错误校验

## 使用场景

用户说"写公式/计算列/辅助列/总计/占比/增长率/查找计算/自动计算/校验公式/检查公式错误"时使用本页。

- 能由表内其他单元格推导的派生值，优先写公式，不要写一次性的静态结果。
- 写公式前先读表头和 3-5 行样本，确认列含义、数据类型、真实行号和目标范围。
- 用户明确要求"辅助列"时，需要真实写入辅助列公式；不要只用条件格式或本地计算绕过。

## 当前能力边界

- 写少量或需要单元格对象的公式：使用 `dws sheet range update`。
- 从 CSV/表格文本批量写公式：使用 `dws sheet csv-put`，字段值以 `=` 开头时默认按公式解析；如需写入以 `=` 开头的字面文本，在字段值前加单引号。
- 公式载体：公式写在 cell object 的 `text` 字段中，例如 `{"type":"text","text":"=SUM(B2:B10)"}`。
- 读取公式文本：使用 `dws sheet range read --value-render-option formula`。
- 读取计算结果：使用 `dws sheet range read --value-render-option raw_value` 或默认 `formatted_value`。
- 聚合错误校验：使用 `dws sheet formula-verify`，支持整本表格、单个目标和多个目标。
- `formula-verify` 扫描已经落表的公式计算结果，按 `#ERROR!` / `#NAME?` / `#DIV/0!` 等错误类型汇总；它不判断一个正常数值是否符合业务预期。
- `append` / `table-put` 不作为公式写入协议；需要公式时用 `range update` 或 `csv-put`。

## 命令选择

| 目的 | 命令 | 说明 |
|------|------|------|
| 写入少量或中等范围公式 | `range update` | `--values` 必须是二维 cell object，维度与 `--range` 完全一致 |
| 从 CSV/表格文本批量写公式 | `csv-put` | `=` 开头按公式；前导单引号写入以 `=` 开头的字面文本；不支持富格式对象 |
| 查看已写入的公式文本 | `range read --value-render-option formula` | 确认公式本身是否落表、范围和引用是否正确 |
| 查看公式计算结果 | `range read --value-render-option raw_value` | 用于数值对账、错误值检查 |
| 查看格式化展示结果 | `range read` 或 `csv-get` 默认模式 | 用于用户肉眼看到的展示值检查 |
| 扫描整本表格公式错误 | `formula-verify --node <NODE_ID>` | 不传目标时扫描全部工作表的非空范围 |
| 扫描单个工作表或范围 | `formula-verify --sheet-id ... [--range ...]` | `--range` 只传 A1 范围，不带工作表前缀 |
| 扫描多个目标 | `formula-verify --targets ...` | 必须是非空数组；每项为 `{"sheetId":"...","range":"..."}`，`range` 可省略 |

## 推荐流程

1. 用 `dws sheet list --node <NODE_ID> --format json` 获取真实 `sheetId`。
2. 用 `range read` 或 `csv-get` 读取表头和样本数据，确认目标列与行号。
3. 明确相对引用和绝对引用：向下填充时检查固定汇率、税率、查找表、标题行是否需要 `$` 锁定。
4. 按数据形态写入公式：精确 cell object 用 `range update`，CSV/表格文本用 `csv-put`。`range update` 的矩阵行列数必须与 `--range` 完全一致。
5. 用 `range read --value-render-option formula` 回读公式文本，确认实际公式、范围和引用。
6. 对本次写入目标运行 `formula-verify`；若返回 `partial` / `hasMore=true`，缩小目标或提高 `--max-cells` 后继续扫描，直到结果完整。
7. 用 `range read --value-render-option raw_value` 抽样对账业务数值；正常数值不会被 `formula-verify` 判定为业务计算错误。
8. 若发现错误，先定位依赖单元格、空值、除数为 0、引用范围越界或函数名错误，再重写公式并重新执行文本回读、错误扫描和数值抽样。

## 聚合式公式校验

### 整本表格

不指定 `--sheet-id`、`--range` 或 `--targets` 时，扫描整本表格的全部工作表：

```bash
dws sheet formula-verify --node <NODE_ID> --format json
```

### 单个工作表或范围

`--sheet-id` 支持工作表 ID 或名称；省略 `--range` 时扫描该工作表的非空范围：

```bash
dws sheet formula-verify --node <NODE_ID> --sheet-id <SHEET_ID> --format json

dws sheet formula-verify --node <NODE_ID> --sheet-id <SHEET_ID> \
  --range "D2:D100" --format json
```

`--range` 必须和 `--sheet-id` 一起使用，且只传 `D2:D100` 这类 A1 范围，不能传 `Sheet1!D2:D100`。

### 多个目标

```bash
dws sheet formula-verify --node <NODE_ID> \
  --targets '[{"sheetId":"Sheet1","range":"D2:D100"},{"sheetId":"Summary"}]' \
  --format json
```

`--targets` 也支持 `@targets.json` 和 `-`（stdin）。数组必须至少包含一个目标，不能传 `[]`；每项只允许非空 `sheetId` 和可选的字符串 `range`，`range` 只写 A1 范围且不能带工作表前缀。使用 `--targets` 时不能再传 `--sheet-id` 或 `--range`。

### 扫描限制与自动化

```bash
dws sheet formula-verify --node <NODE_ID> \
  --max-locations-per-error 20 --max-cells 30000 --format json

dws sheet formula-verify --node <NODE_ID> --exit-on-error --format json
```

- `--max-locations-per-error` 只限制每类错误返回的 `locations` 和 `samples` 数量，`count` 与 `totalErrors` 仍保留实际扫描到的总数。
- `--max-cells` 是本次调用跨全部 targets 共享的扫描预算；预算不足时返回 `status=partial`、`hasMore=true`。
- `--exit-on-error` 适合 CI/自动化：发现公式错误时打印 JSON 结果并返回非 0；`partial` 结果中已经发现错误时同样返回非 0。

### 结果判定

| 字段 | 含义 |
|------|------|
| `status` | `success` / `errors_found` / `partial` |
| `totalErrors` | 实际扫描到的错误公式单元格总数 |
| `totalFormulas` | 实际扫描到的公式单元格总数 |
| `scannedCells` | 实际扫描的单元格数 |
| `hasMore` | `true` 表示结果不完整，不能据此声称目标范围零错误 |
| `errorSummary` | 按错误类型聚合的 `count`、`locations` 和 `samples` |
| `warningMessage` | `partial` 等情况下的扫描限制提示 |

判定规则：

- `status=success`、`hasMore=false`、`totalErrors=0`：本次目标范围未发现公式错误。
- `status=errors_found`：按 `errorSummary` 修复后重新校验。
- `status=partial` 或 `hasMore=true`：当前结果不完整；缩小 targets/range 或提高 `--max-cells` 后继续校验。

## 写入示例

### 单格公式

```bash
dws sheet range update --node <NODE_ID> --sheet-id <SHEET_ID> --range "D2" \
  --values '[[{"type":"text","text":"=B2*C2"}]]' --format json
```

### 整列公式

```bash
dws sheet range update --node <NODE_ID> --sheet-id <SHEET_ID> --range "D2:D5" \
  --values '[
    [{"type":"text","text":"=B2*C2"}],
    [{"type":"text","text":"=B3*C3"}],
    [{"type":"text","text":"=B4*C4"}],
    [{"type":"text","text":"=B5*C5"}]
  ]' --format json
```

### 含绝对引用

税率在 `G1` 时，向下填充应锁定税率单元格：

```bash
dws sheet range update --node <NODE_ID> --sheet-id <SHEET_ID> --range "E2:E5" \
  --values '[
    [{"type":"text","text":"=D2*$G$1"}],
    [{"type":"text","text":"=D3*$G$1"}],
    [{"type":"text","text":"=D4*$G$1"}],
    [{"type":"text","text":"=D5*$G$1"}]
  ]' --format json
```

## 公式文本与结果回读

### 1. 回读公式文本

```bash
dws sheet range read --node <NODE_ID> --sheet-id <SHEET_ID> --range "D2:D5" \
  --value-render-option formula --format json
```

检查点：
- `value` 应返回以 `=` 开头的公式文本。
- 行号、列号、相对引用、绝对引用应与写入计划一致。
- 无公式的单元格在 `formula` 模式下可能回退为原始值，不能把这种回退误判为公式已写入。

### 2. 回读计算结果

```bash
dws sheet range read --node <NODE_ID> --sheet-id <SHEET_ID> --range "D2:D5" \
  --value-render-option raw_value --format json
```

检查点：
- 数值结果应与样本手算或本地复算一致。
- 检查结果中是否出现 `#REF!` / `#DIV/0!` / `#VALUE!` / `#NAME?` / `#NULL!` / `#NUM!` / `#N/A`。
- 对大范围公式，至少抽样检查首行、末行、边界行和异常数据行；用户要求全量处理时，应分批回读并断言处理数量。

### 3. 数值正确性边界

`formula-verify` 负责聚合已经落表的公式错误值；`range read` 负责确认实际公式文本和具体计算结果。即使 `formula-verify` 返回 `success`，也仍需对金额、比例、汇率、边界行等关键业务结果做 `raw_value` 抽样对账，因为一个公式可能计算出合法数值但业务逻辑仍然写错。

## 常见错误

- 想用 `csv-put` 写入 `=SUM(...)` 文本却忘记加前导单引号，导致内容被解析为公式。
- 用原始二维数组 `--values '[["=B2*C2"]]'`，而不是 cell object。
- 写整列公式时只写第一行，忘记把 `--range` 和 `--values` 扩成同样行数。
- 复制公式时没有锁定固定引用，例如税率、汇率、查找表范围。
- 没有回读 `formula` 模式，只看写入返回 `success`。
- 只回读展示值，不运行 `formula-verify` 聚合扫描错误。
- 把 `max-locations-per-error` 误解为错误计数上限；它只截断位置和样本。
- 看到 `status=partial` 或 `hasMore=true` 仍声称整本表公式零错误。
- 在 `--range` 中传 `Sheet1!A1:D10`，或把 `--targets` 与 `--sheet-id` 混用。
- 显式传空的 `--targets '[]'`；这不是“无目标”，应改为至少一个目标，整本扫描则完全省略 `--targets`。

## 关联文档

- [sheet-write-data](./sheet-write-data.md)：`range update` 的 `--values` cell object 结构、维度校验、富格式能力。
- [sheet-read-data](./sheet-read-data.md)：`value-render-option` 的 `formatted_value` / `raw_value` / `formula` 读取模式。
- [sheet-conditional-format](./sheet-conditional-format.md)：条件格式中的 `formulaCondition` 与辅助列公式的职责边界。
