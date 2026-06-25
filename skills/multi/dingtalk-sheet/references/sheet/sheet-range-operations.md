# 区域操作

## 使用场景

用户说"清空/清除区域/擦除内容/清除格式":
- 清除区域 → `range clear`
- 仅清除值 → `range clear --type content`（默认）
- 仅清除格式 → `range clear --type format`
- 全部清除 → `range clear --type all`
- 请勿用 `range update` 写入空字符串来模拟清空，`range clear` 更简洁且支持按类型清除

用户说"排序/给数据排序/按某列排序/升序/降序":
- 区域排序 → `range sort`
- **排序前必须先 `range read` 前 3-5 行**：读取排序范围的前几行（如范围是 A1:D100 则读 A1:D5），对比首行与后续行的模式来判断是否有表头：
  - 首行全文本 + 后续行含数字/日期 → 有表头，加 `--has-header`
  - 首行与后续行模式一致（都是数字或都是文本） → 无表头，不加
  - 首行值语义像列标题（如"姓名""金额""日期"）且与后续行明显不同 → 有表头
  禁止不读就排——表头误排入数据是不可撤销的破坏性操作
- 请勿用 `range read` 读取数据后客户端排序再 `range update` 写回，`range sort` 是服务端原子操作

用户说"自动填充/填充序列/向下填充/拖拽填充/序列递增":
- 自动填充 → `range fill`
- 请勿用 `range read` 读取源数据后手动计算规律再 `range update` 写入，`range fill` 支持服务端智能填充

用户说"复制区域/把这块数据复制到/复制到另一个工作表":
- 复制区域 → `range copy-to`
- 跨工作表 → `range copy-to --target-sheet-id Sheet2` 或 `--target-range "Sheet2!A1"`
- 请勿用 `range read` + `range update` 读取再写入来模拟复制，`range copy-to` 是原子操作，保留公式引用调整

用户说"移动区域/把数据移到/剪切粘贴/移到另一个工作表":
- 移动区域 → `range move-to`
- 跨工作表 → `range move-to --target-sheet-id Sheet2` 或 `--target-range "Sheet2!A1"`
- 请勿用 `range read` + `range update` + `range clear` 读取-写入-清空来模拟移动，`range move-to` 是原子操作

## 命令详细参考

### 清除区域
```
Usage:
  dws sheet range clear [flags]
Example:
  dws sheet range clear --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:B3"
  dws sheet range clear --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:B3" --type format
  dws sheet range clear --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:B3" --type all
Flags:
      --node string            表格文档 ID 或 URL (必填)
      --sheet-id string        工作表 ID 或名称 (必填)
      --range string           清除范围，A1 表示法 (必填)
      --type string            清除类型: content(仅值,默认) / format(仅格式) / all(全部)
```

### 区域排序
```
Usage:
  dws sheet range sort [flags]
Example:
  dws sheet range sort --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:D10" \
    --sort-keys '[{"column":"A","ascending":true}]'
  dws sheet range sort --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:D10" \
    --sort-keys '[{"column":"A","ascending":true},{"column":"C","ascending":false}]' --has-header
Flags:
      --node string            表格文档 ID 或 URL (必填)
      --sheet-id string        工作表 ID 或名称 (必填)
      --range string           排序范围，A1 表示法 (必填)
      --sort-keys string       排序规则 JSON 数组 (必填)
      --has-header             首行是否为表头（不参与排序）
```

`--sort-keys` 格式：`[{"column":"A","ascending":true}]`，`column` 使用字母列名（如 "A"、"B"、"AA"）。多级排序按数组顺序优先级递减。

### 区域自动填充
```
Usage:
  dws sheet range fill [flags]
Example:
  dws sheet range fill --node <NODE_ID> --sheet-id <SHEET_ID> \
    --source-range "A1:A5" --target-range "A6:A20"
  dws sheet range fill --node <NODE_ID> --sheet-id <SHEET_ID> \
    --source-range "A1:A5" --target-range "A6:A20" --fill-type copy
Flags:
      --node string            表格文档 ID 或 URL (必填)
      --sheet-id string        工作表 ID 或名称 (必填)
      --source-range string    源数据范围，A1 表示法 (必填)
      --target-range string      目标填充范围，A1 表示法 (必填)
      --fill-type string       填充类型: series(序列,默认) / copy(复制) / onlystyle(仅格式) / withoutstyle(仅值)
```

目标范围须与源范围在行或列维度对齐（不支持对角填充）。

### 复制区域
```
Usage:
  dws sheet range copy-to [flags]
Example:
  dws sheet range copy-to --node <NODE_ID> --sheet-id <SHEET_ID> \
    --source-range "A1:C5" --target-range "D1"
  dws sheet range copy-to --node <NODE_ID> --sheet-id <SHEET_ID> \
    --source-range "A1:C5" --target-range "A1" --target-sheet-id "Sheet2"
  dws sheet range copy-to --node <NODE_ID> --sheet-id <SHEET_ID> \
    --source-range "A1:C5" --target-range "D1" --paste-type values
Flags:
      --node string            表格文档 ID 或 URL (必填)
      --sheet-id string        源工作表 ID 或名称 (必填)
      --source-range string    源范围，A1 表示法 (必填)
      --target-range string    目标位置，A1 表示法 (必填)
      --target-sheet-id string 目标工作表 ID 或名称（可选，不传则复制到同一工作表）
      --paste-type string      粘贴类型: values(仅值) / formulas(仅公式) / formats(仅格式) / all(全部,默认)
```

支持跨工作表复制，两种方式指定目标工作表：
- `--target-sheet-id "Sheet2"` 显式指定
- `--target-range "Sheet2!A1"` 在目标范围中携带工作表前缀

源和目标范围不能重叠（同表时）。

### 移动区域
```
Usage:
  dws sheet range move-to [flags]
Example:
  dws sheet range move-to --node <NODE_ID> --sheet-id <SHEET_ID> \
    --source-range "A1:C5" --target-range "D1"
  dws sheet range move-to --node <NODE_ID> --sheet-id <SHEET_ID> \
    --source-range "A1:C5" --target-range "A1" --target-sheet-id "Sheet2"
Flags:
      --node string            表格文档 ID 或 URL (必填)
      --sheet-id string        源工作表 ID 或名称 (必填)
      --source-range string    源范围，A1 表示法 (必填)
      --target-range string    目标位置，A1 表示法 (必填)
      --target-sheet-id string 目标工作表 ID 或名称（可选，不传则移动到同一工作表）
```

支持跨工作表移动，两种方式指定目标工作表：
- `--target-sheet-id "Sheet2"` 显式指定
- `--target-range "Sheet2!A1"` 在目标范围中携带工作表前缀

源和目标范围不能重叠（同表时）。移动后源区域将被清空。

## 上下文传递

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `list` | 工作表的 `sheetId` | range clear / range sort / range fill / range copy-to / range move-to 的 --sheet-id |

## 注意事项

- ★ **`--sheet-id` 获取规范（强制）**：`sheetId` 未知时必须先通过 `dws sheet list --node <NODE_ID> --format json` 查询真实的 `sheetId` / 工作表名称后再调用，禁止凭空编造（如臆测为 `Sheet1`、`sheet1`、`0`、`default` 等）；用户仅给出工作表名称时，也应通过 `list` 校验该名称是否存在，避免名称大小写或拼写不一致导致失败
- ★ **清空区域用 `range clear` 不用 `range update`**：`range clear` 支持按类型（值/格式/全部）清除，比手动构造全空数组更简洁可靠
- ★ **复制区域用 `range copy-to` 不用 `range read` + `range update`**：原子操作，保留公式引用自动调整，支持跨工作表
- ★ **移动区域用 `range move-to` 不用 `range read` + `range update` + `range clear`**：原子操作，源区域自动清空，支持跨工作表
- ★ **排序用 `range sort` 不用 `range read` + 客户端排序 + `range update`**：服务端原子操作，支持多级排序
- ★ **排序前必须 `range read` 前几行判断表头**：读取排序范围前 3-5 行，对比首行与后续行的数据模式（类型、语义）来判断是否有表头。禁止不读就排，表头被排入数据不可撤销
- ★ **填充用 `range fill` 不用 `range read` + 手动计算 + `range update`**：服务端智能填充，支持序列递增、公式扩展等
