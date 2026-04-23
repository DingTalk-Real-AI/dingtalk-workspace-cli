# 电子表格 (sheet) 命令参考

> ⚠️ **命令名以 `dws sheet --help` 实际输出为准**：本文档的命令命名（如 `sheet list` / `sheet range read`）来自上游 wukong CLI 的层级 overlay；当前 cli 暴露的可能是扁平 MCP tool 名（如 `sheet get-all-sheets` / `sheet get-range`）。用户实际操作前请用 `dws sheet --help` / `dws sheet <cmd> --help` 确认子命令名与参数名。

## 命令总览

### 创建钉钉表格文档
```
Usage:
  dws sheet create [flags]
Example:
  dws sheet create --name "销售数据"
  dws sheet create --name "Q1 数据" --folder <FOLDER_ID>
  dws sheet create --name "知识库表格" --workspace <WS_ID>
Flags:
      --name string        表格名称 (必填)
      --folder string      目标文件夹 ID 或 URL
      --workspace string   目标知识库 ID
```

### 获取全部工作表列表
```
Usage:
  dws sheet list [flags]
Example:
  dws sheet list --node <NODE_ID>
  dws sheet list --node "https://alidocs.dingtalk.com/i/nodes/<DOC_UUID>"
Flags:
      --node string   表格文档 ID 或 URL (必填)
```

### 获取指定工作表详情
```
Usage:
  dws sheet info [flags]
Example:
  dws sheet info --node <NODE_ID>
  dws sheet info --node <NODE_ID> --sheet-id <SHEET_ID>
  dws sheet info --node <NODE_ID> --sheet-id "Sheet1"
Flags:
      --node string       表格文档 ID 或 URL (必填)
      --sheet-id string   工作表 ID 或名称 (不传则返回第一个工作表)
```

### 新建工作表
```
Usage:
  dws sheet new [flags]
Example:
  dws sheet new --node <NODE_ID> --name "Sheet2"
  dws sheet new --node <NODE_ID> --name "数据汇总"
Flags:
      --node string   表格文档 ID (必填)
      --name string   工作表名称 (必填)
```

### 读取工作表数据
```
Usage:
  dws sheet range read [flags]
Example:
  dws sheet range read --node <NODE_ID>
  dws sheet range read --node <NODE_ID> --sheet-id <SHEET_ID>
  dws sheet range read --node <NODE_ID> --sheet-id "Sheet1" --range "A1:D10"
  dws sheet range read --node <NODE_ID> --range "Sheet1!A1:D10"
Flags:
      --node string       表格文档 ID 或 URL (必填)
      --sheet-id string   工作表 ID 或名称 (不传则默认第一个工作表)
      --range string      读取范围，A1 表示法 (如 A1:D10，不传则读取全部数据)
```

### 更新工作表指定区域内容
```
Usage:
  dws sheet range update [flags]
Example:
  # 写入值
  dws sheet range update --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:B2" \
    --values '[["姓名","分数"],["张三",90]]'

  # 写入公式
  dws sheet range update --node <NODE_ID> --sheet-id <SHEET_ID> --range "C2" \
    --values '[["=A2&B2"]]'

  # 写入超链接
  dws sheet range update --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1" \
    --hyperlinks '[[{"type":"path","link":"https://dingtalk.com","text":"钉钉"}]]'

  # 清空区域
  dws sheet range update --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:B3" \
    --values '[[null,null],[null,null],["保留",null]]'
Flags:
      --node string            表格文档 ID (必填)
      --sheet-id string        工作表 ID 或名称 (必填)
      --range string           目标单元格区域地址，如 A1:B3 (必填)
      --values string          单元格值，二维 JSON 数组 (与 --hyperlinks 至少传一项)
      --hyperlinks string      超链接，二维 JSON 数组 (与 --values 至少传一项)
      --number-format string   数字格式，如 General/@/#,##0/0%/yyyy/m/d 等
```

## URL 识别与 NODE_ID 提取

当用户输入包含钉钉文档 URL 时，**必须先识别并提取 NODE_ID**，再判断意图。

### 支持的 URL 格式

| 格式 | 示例 | NODE_ID 提取方式 |
|------|------|----------------|
| `alidocs.dingtalk.com/i/nodes/{id}` | `https://alidocs.dingtalk.com/i/nodes/9E05BDRVQePjzLkZt2p2vE7kV63zgkYA` | 取 URL 路径最后一段 |
| `alidocs.dingtalk.com/i/nodes/{id}?queryParams` | `https://alidocs.dingtalk.com/i/nodes/abc123?doc_type=wiki_doc` | 忽略 query 参数，取路径最后一段 |

### 提取规则

1. 匹配 URL 中 `alidocs.dingtalk.com` 域名
2. 取 URL path 的最后一段作为 NODE_ID（去掉 query string 和 fragment）
3. 提取出的 NODE_ID 可直接用于所有 `--node` 参数，也可将完整 URL 传给 `--node`（CLI 会自动解析）

## 意图判断

用户说"创建表格/新建电子表格":
- 创建表格文档 → `create`

用户说"看工作表/有哪些工作表/表格结构":
- 列出工作表 → `list`
- 工作表详情 → `info`

用户说"加工作表/新增Sheet":
- 新建工作表 → `new`

用户说"读数据/看表格内容/导出数据":
- 读取数据 → `range read`

用户说"写数据/填表/更新单元格/写入公式":
- 更新数据 → `range update`

**用户直接粘贴表格 URL（无其他指令）**:
- 默认 → `list`（列出工作表）+ `range read`（读取第一个工作表数据）

**用户粘贴 URL + 附加指令**:
- "帮我看看这个表格有什么数据" → `range read`
- "这个表格有哪些工作表" → `list`
- "往这个表格写入数据" → `range update`

关键区分: sheet(电子表格/单元格读写) vs aitable(AI多维表/结构化记录) vs doc(文档编辑/阅读)

## 核心工作流

```bash
# ── 工作流 1: 创建表格并写入数据 ──

# 1. 创建表格文档 — 提取 nodeId
dws sheet create --name "销售数据" --format json

# 2. 查看工作表列表 — 提取 sheetId
dws sheet list --node <NODE_ID> --format json

# 3. 写入表头和数据
dws sheet range update --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:C1" \
  --values '[["姓名","部门","销售额"]]' --format json

dws sheet range update --node <NODE_ID> --sheet-id <SHEET_ID> --range "A2:C4" \
  --values '[["张三","销售部",50000],["李四","市场部",38000],["王五","销售部",62000]]' --format json

# ── 工作流 2: 读取已有表格数据 ──

# 1. 获取工作表列表
dws sheet list --node <NODE_ID> --format json

# 2. 查看工作表详情（行列数、最后非空位置等）
dws sheet info --node <NODE_ID> --sheet-id <SHEET_ID> --format json

# 3. 读取全部数据
dws sheet range read --node <NODE_ID> --sheet-id <SHEET_ID> --format json

# 4. 读取指定区域
dws sheet range read --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:D10" --format json

# ── 工作流 3: 多工作表管理 ──

# 1. 新建工作表
dws sheet new --node <NODE_ID> --name "汇总" --format json

# 2. 在新工作表中写入汇总公式
dws sheet range update --node <NODE_ID> --sheet-id <NEW_SHEET_ID> --range "A1:B1" \
  --values '[["指标","数值"]]' --format json

dws sheet range update --node <NODE_ID> --sheet-id <NEW_SHEET_ID> --range "A2:B2" \
  --values '[["总销售额","=SUM(Sheet1!C2:C100)"]]' --format json

# ── 工作流 4: 批量更新与格式化 ──

# 1. 写入数据
dws sheet range update --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:C3" \
  --values '[["商品","单价","数量"],["苹果",5.5,100],["香蕉",3.2,200]]' --format json

# 2. 设置数字格式（人民币）
dws sheet range update --node <NODE_ID> --sheet-id <SHEET_ID> --range "B2:B3" \
  --values '[[5.5],[3.2]]' --number-format "¥#,##0.00" --format json

# 3. 写入超链接
dws sheet range update --node <NODE_ID> --sheet-id <SHEET_ID> --range "D1" \
  --hyperlinks '[[{"type":"path","link":"https://dingtalk.com","text":"详情"}]]' --format json
```

## 上下文传递表

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `create` | `nodeId` | list / info / new / range read / range update 的 --node |
| `list` | 工作表的 `sheetId` | info / range read / range update 的 --sheet-id |
| `new` | 新工作表的 `sheetId` | range read / range update 的 --sheet-id |
| `info` | `rowCount` / `lastNonEmptyRow` | 确定数据范围、追加写入起始行 |

## nodeId 双格式说明

所有 `--node` 参数同时支持两种格式，系统自动识别：
- **文档 ID**: 字母数字字符串，如 `9E05BDRVQePjzLkZt2p2vE7kV63zgkYA`
- **文档 URL**: `https://alidocs.dingtalk.com/i/nodes/{dentryUuid}`

两种方式等价，以下命令效果相同：
```bash
dws sheet list --node 9E05BDRVQePjzLkZt2p2vE7kV63zgkYA
dws sheet list --node "https://alidocs.dingtalk.com/i/nodes/9E05BDRVQePjzLkZt2p2vE7kV63zgkYA"
```

## values 参数格式说明

`--values` 为二维 JSON 数组，第一维为行，第二维为列：
- 字符串值: `"文本"`
- 数字值: `100` 或 `3.14`
- 公式: `"=SUM(B2:B4)"`（以 `=` 开头的字符串自动识别为公式）
- 清空单元格: `null`

维度必须与 `--range` 范围一致，例如 `--range "A1:B3"` 需要 3 行 2 列的数组。

## hyperlinks 参数格式说明

`--hyperlinks` 为二维 JSON 数组，每个元素为对象或 null：
- `type`: 链接类型，可选 `path`（外部链接）、`sheet`（工作表跳转）、`range`（单元格跳转）
- `link`: 链接地址
- `text`: 显示文本

与 `--values` 共存时，hyperlinks 优先级更高。

## number-format 常用值

| 格式代码 | 说明 | 示例 |
|----------|------|------|
| `General` | 常规 | 1234.5 |
| `@` | 文本 | 001234 |
| `#,##0` | 整数千分位 | 1,235 |
| `#,##0.00` | 两位小数 | 1,234.50 |
| `0%` | 百分比 | 85% |
| `yyyy/m/d` | 日期 | 2026/3/15 |
| `hh:mm:ss` | 时间 | 14:30:00 |
| `¥#,##0` | 人民币 | ¥1,235 |

## 注意事项

- `create` 不传 `--folder` 和 `--workspace` 时，默认创建在"我的文档"根目录
- `list` 返回所有工作表的 ID 和名称，是后续操作的必要前置步骤
- `info` 不传 `--sheet-id` 时默认返回第一个工作表的详情
- `range read` 不传 `--range` 时默认读取整个工作表的全部非空数据
- `range read` 的 `--range` 支持 `Sheet1!A1:D10` 格式直接指定工作表（此时忽略 `--sheet-id`）
- `range update` 的 `--values` 和 `--hyperlinks` 至少传入一项
- `new` 创建工作表时，如名称与已有工作表重复，系统会自动重命名
- 关键区分: sheet(电子表格/单元格读写) vs aitable(AI多维表/结构化记录/字段定义) vs doc(文档编辑/阅读)
