# 公式写入与有限回读校验

## 使用场景

用户说"写公式/计算列/辅助列/总计/占比/增长率/查找计算/自动计算"时使用本页。

- 能由表内其他单元格推导的派生值，优先写公式，不要写一次性的静态结果。
- 写公式前先读表头和 3-5 行样本，确认列含义、数据类型、真实行号和目标范围。
- 用户明确要求"辅助列"时，需要真实写入辅助列公式；不要只用条件格式或本地计算绕过。

## 当前能力边界

- 写公式：使用 `dws sheet range update`。
- 公式载体：公式写在 cell object 的 `text` 字段中，例如 `{"type":"text","text":"=SUM(B2:B10)"}`。
- 读取公式文本：使用 `dws sheet range read --value-render-option formula`。
- 读取计算结果：使用 `dws sheet range read --value-render-option raw_value` 或默认 `formatted_value`。
- 当前没有聚合式 `formula-verify` 工具，不能宣称已经完成全表零错误校验。
- `csv-put` / `append` 不作为公式写入协议；`=` 开头内容会按普通值处理。需要公式时用 `range update`。

## 命令选择

| 目的 | 命令 | 说明 |
|------|------|------|
| 写入少量或中等范围公式 | `range update` | `--values` 必须是二维 cell object，维度与 `--range` 完全一致 |
| 查看已写入的公式文本 | `range read --value-render-option formula` | 确认公式本身是否落表、范围和引用是否正确 |
| 查看公式计算结果 | `range read --value-render-option raw_value` | 用于数值对账、错误值检查 |
| 查看格式化展示结果 | `range read` 或 `csv-get` 默认模式 | 用于用户肉眼看到的展示值检查 |

## 推荐流程

1. 用 `dws sheet list --node <NODE_ID> --format json` 获取真实 `sheetId`。
2. 用 `range read` 或 `csv-get` 读取表头和样本数据，确认目标列与行号。
3. 明确相对引用和绝对引用：向下填充时检查固定汇率、税率、查找表、标题行是否需要 `$` 锁定。
4. 用 `range update` 写入公式矩阵；矩阵行列数必须与 `--range` 完全一致。
5. 用 `range read --value-render-option formula` 回读公式文本。
6. 用 `range read --value-render-option raw_value` 回读计算结果，并检查明显错误值。
7. 若发现错误值，先定位依赖单元格、空值、除数为 0、引用范围越界或函数名错误，再重写公式并重新回读。

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

## 回读校验

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

### 3. 当前不等价于公式零错误校验

有限回读只能发现公式文本错误、明显运行时错误值和样本不一致。它不能替代未来可能的聚合式公式校验工具，也不能证明整本表所有公式都没有隐藏错误。

## 常见错误

- 用 `csv-put` 写 `=SUM(...)`，导致公式没有按公式协议落表。
- 用原始二维数组 `--values '[["=B2*C2"]]'`，而不是 cell object。
- 写整列公式时只写第一行，忘记把 `--range` 和 `--values` 扩成同样行数。
- 复制公式时没有锁定固定引用，例如税率、汇率、查找表范围。
- 没有回读 `formula` 模式，只看写入返回 `success`。
- 只回读展示值，不检查 `raw_value` 中的错误值。

## 关联文档

- [sheet-write-data](./sheet-write-data.md)：`range update` 的 `--values` cell object 结构、维度校验、富格式能力。
- [sheet-read-data](./sheet-read-data.md)：`value-render-option` 的 `formatted_value` / `raw_value` / `formula` 读取模式。
- [sheet-conditional-format](./sheet-conditional-format.md)：条件格式中的 `formulaCondition` 与辅助列公式的职责边界。
