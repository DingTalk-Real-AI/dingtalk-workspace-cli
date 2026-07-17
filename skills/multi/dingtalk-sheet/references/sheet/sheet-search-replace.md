# 搜索与替换

## 使用场景

用户说"搜索/查找/找单元格/搜内容/精确搜索/精确匹配/完全匹配/全字匹配":
- 搜索单元格 → `find`
- 精确匹配（只匹配完全等于的，不匹配包含的） → `find --match-entire-cell`
- 正则搜索 → `find --use-regexp`
- 搜索公式 → `find --match-formula`
- 不要用 `range read` 读取全量数据后在客户端过滤来替代 `find`，必须使用 `find` 命令的服务端搜索能力

用户说"替换/查找替换/全局替换/批量替换/把A替换成B/把所有的X改成Y":
- 查找替换 → `replace`
- 精确匹配后替换（只替换内容完全等于的单元格） → `replace --match-entire-cell`
- 正则替换 → `replace --use-regexp`
- 删除匹配内容 → `replace --replacement ""`
- 请勿用 `find` + `range update`、`range read` + `range update` 等组合来模拟替换，`replace` 是服务端原子操作，效率更高且返回替换计数

## 命令详细参考

### 在工作表中搜索单元格内容
```
Usage:
  dws sheet find [flags]
Example:
  # 基本搜索
  dws sheet find --node <NODE_ID> --sheet-id <SHEET_ID> --query "销售额"

  # 在指定范围内搜索
  dws sheet find --node <NODE_ID> --sheet-id <SHEET_ID> --query "合计" --range "A1:D100"

  # 正则表达式搜索（不区分大小写）
  dws sheet find --node <NODE_ID> --sheet-id <SHEET_ID> --query "^total" --use-regexp --match-case=false

  # 精确匹配整个单元格内容
  dws sheet find --node <NODE_ID> --sheet-id <SHEET_ID> --query "完成" --match-entire-cell

  # 搜索公式文本
  dws sheet find --node <NODE_ID> --sheet-id <SHEET_ID> --query "SUM" --match-formula
Flags:
      --node string         表格文档 ID 或 URL (必填)
      --sheet-id string     工作表 ID 或名称 (必填)
      --find string         搜索文本 (必填)
      --range string        搜索范围，A1 表示法 (如 A1:D10)
      --match-case          区分大小写 (默认 true)
      --match-entire-cell   精确匹配整个单元格内容
      --use-regexp          启用正则表达式搜索
      --match-formula       搜索公式文本而非显示值
      --include-hidden      包含隐藏单元格
```

### 全局查找替换
```
Usage:
  dws sheet replace [flags]
Example:
  dws sheet replace --node <NODE_ID> --sheet-id <SHEET_ID> --find "旧文本" --replacement "新文本"
  dws sheet replace --node <NODE_ID> --sheet-id <SHEET_ID> --find "待处理" --replacement "已完成" --match-entire-cell
  dws sheet replace --node <NODE_ID> --sheet-id <SHEET_ID> --find "\\d{4}" --replacement "****" --use-regexp
  dws sheet replace --node <NODE_ID> --sheet-id <SHEET_ID> --find "旧" --replacement "新" --range "A1:D100"
  dws sheet replace --node <NODE_ID> --sheet-id <SHEET_ID> --find "临时" --replacement ""
Flags:
      --node string            表格文档 ID 或 URL (必填)
      --sheet-id string        工作表 ID 或名称 (必填)
      --find string            查找文本 (必填)
      --replacement string     替换文本 (必填，可为空字符串表示删除)
      --range string           替换范围，A1 表示法 (如 A1:D100)
      --match-case             区分大小写 (默认 false)
      --match-entire-cell      完整单元格匹配
      --use-regexp             启用正则表达式匹配
      --include-hidden         包含隐藏行/列
```

返回被替换的单元格数量。`--replacement` 可以为空字符串，表示删除匹配内容。

## 核心工作流

```bash
# ── 工作流: 搜索表格数据 ──

# 1. 获取工作表列表
dws sheet list --node <NODE_ID> --format json

# 2. 基本搜索 — 在指定工作表中查找文本
dws sheet find --node <NODE_ID> --sheet-id <SHEET_ID> --query "销售额" --format json

# 3. 在指定范围内搜索
dws sheet find --node <NODE_ID> --sheet-id <SHEET_ID> --query "合计" --range "A1:D100" --format json

# 4. 正则搜索（不区分大小写）
dws sheet find --node <NODE_ID> --sheet-id <SHEET_ID> --query "^total" --use-regexp --match-case=false --format json

# 5. 精确匹配整个单元格
dws sheet find --node <NODE_ID> --sheet-id <SHEET_ID> --query "完成" --match-entire-cell --format json

# 6. 搜索公式文本
dws sheet find --node <NODE_ID> --sheet-id <SHEET_ID> --query "SUM" --match-formula --format json
```

## 上下文传递

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `list` | 工作表的 `sheetId` | find / replace 的 --sheet-id |
| `find` | `matchedCells` 中的 `a1Notation` | 定位目标单元格，用于 range read / range update |
| `replace` | `replaceCount` 被替换的单元格数量 | 确认替换结果 |

## 注意事项

- ★ **`--sheet-id` 获取规范（强制）**：`sheetId` 未知时必须先通过 `dws sheet list --node <NODE_ID> --format json` 查询真实的 `sheetId` / 工作表名称后再调用，禁止凭空编造（如臆测为 `Sheet1`、`sheet1`、`0`、`default` 等）；用户仅给出工作表名称时，也应通过 `list` 校验该名称是否存在，避免名称大小写或拼写不一致导致失败
- ★ **搜索用 `find` 不用 `range read`**：`find` 是服务端搜索，禁止用 `range read` 全量读取后客户端过滤
- ★ **替换用 `replace` 不用 `range update`**：`replace` 是服务端原子操作，返回替换计数
- `find` 返回匹配单元格的地址（A1 表示法）和值，无匹配时返回空数组
- `find` 的 `--match-entire-cell` 用于精确匹配：只返回单元格内容完全等于搜索文本的结果，不会匹配包含该文本的单元格（例如搜索"苹果"时，只匹配"苹果"，不匹配"苹果手机""苹果汁"等）。用户说"精确搜索/完全匹配/只搜等于XX的"时必须使用此参数
- `find` 的 `--match-case` 默认为 true（区分大小写），设为 false 可忽略大小写
- `find` 的 `--use-regexp` 启用后，`--find` 参数作为正则表达式处理
- `replace` 的 `--find` 不能为空字符串，`--replace` 可以为空字符串（表示删除匹配内容）
- `replace` 的 `--match-case` 默认为 false（不区分大小写），与 `find` 的默认行为不同
