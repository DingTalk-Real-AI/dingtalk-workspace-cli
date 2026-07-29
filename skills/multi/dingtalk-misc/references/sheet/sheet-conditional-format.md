# 条件格式 (conditional format)

## 使用场景

### 条件格式

#### 强制走条件格式的触发词（硬约束）

当用户出现以下口语指令时，**强制**走 `cond-format create/update/delete`，**禁止**用 `range set-style` 写静态背景色/字体色代替：

- **颜色动作**："标红 / 标黄 / 标绿 / 上色 / 染色 / 涂色"
- **视觉强调**："高亮 / 突出 / 标记 / 标注 / 区分"
- **条件触发**："重复的标出来 / 异常的圈出来 / 过期的染红 / 大于 X 的标黄 / 不达标的标红"
- **联动语义**："颜色随数据变 / 联动 / 自动更新 / 改了数据颜色也跟着变"
- **数值可视化**："数据条 / 色阶 / 渐变色 / 进度条样式"

**判断标准**：交付后 `cond-format list` 必须能返回该规则；否则视为违规。

> 如果用 `range set-style` 写静态背景色，源数据变化时颜色不会跟着变。典型反例：用户要求"过期单元格标红"时用静态填充——日期变化后颜色不再准确。

**大数据量优势**：当数据量 > 1000 行时，条件格式是首选——它由服务端自身渲染，不需要逐行调用 `range set-style`，性能远优于静态样式写入。

#### 意图判断

用户说"条件格式/条件样式/自动变色/满足条件时高亮/按条件设样式/条件格式规则":
- 查看已有条件格式 → `cond-format list`
- 查看指定规则详情 → `cond-format list --rule-id RULE_ID`

用户说"创建条件格式/设置条件格式/大于某值时标红/包含某文本时高亮/数据条/图标集/色阶/重复值高亮":
- 创建条件格式规则 → `cond-format create`
- `--condition` 为 JSON 对象，key 为条件类型，value 为条件参数
- `--cell-style` 为命中时的样式（适用于数值/文本/空值/错误/重复/公式/排名/平均值/标准差类型）
- `--data-bar-style` 为数据条样式（仅数据条类型时使用）
- 色阶和图标集类型不需要 `--cell-style`（样式内置在条件定义中）

用户说"修改条件格式/更新条件格式规则/改条件/改样式/改范围":
- 更新条件格式规则 → `cond-format update`
- 未传入的字段保持原有值不变
- 传入 `--condition` 会替换原有条件类型

用户说"删除条件格式/移除条件格式/取消条件格式":
- 删除条件格式规则 → `cond-format delete --yes`
- 删除不可恢复，执行前必须向用户确认，同意后才加 `--yes` 执行
- 规则已不存在时操作仍返回成功

#### 常见配置错误（必须注意）

- **创建后必须验证**：条件格式创建后必须调用 `cond-format list` 验证规则是否生效。如果验证发现规则未生效或配置不正确，应立即修复并重试
- **范围要精确**：条件格式的应用范围必须精确覆盖用户指定的列/行，不要遗漏也不要过度扩大
- **`backgroundColor` vs `fontColor` 的中文语义**：用户中文语境下的"标红/高亮/染色/标记"指**单元格背景色**，用 `backgroundColor`；"文字红/字体红/把字变红"才用 `fontColor`。默认无说明时选 `backgroundColor`
- **日期/空值比较必须防空**：用户说"过期的标红"时，公式必须排除空单元格，否则空白格也会被误判为过期而全表标红。正确公式：`=AND(E1<>"", E1<=TODAY())`；错误公式：`=E1<=TODAY()`（空值会被当作 0 判为过期）
- **公式引用方式**：自定义公式条件中的单元格引用需要根据实际场景选择相对/绝对引用（如 `=E1<=TODAY()` 使用相对引用使公式随行变化，而非 `=$E$1<=TODAY()` 只比较一个格）
- **创建前必须确认列对应**：仅读表头不够——如果表头语义含糊，formula 里引用的列字母可能张冠李戴。建议先读 3-5 行数据样本（如 `range read --range "A1:Z5"`）确认列名对应的实际值和数据类型

#### 辅助列+条件格式两步走（高频致命错误防护）

**用户明确要求"辅助列+条件格式"两步走时，禁止用 `formulaCondition` 绕过**：

当用户说以下任意一种表达时，必须按两步走（先建辅助列 → 再基于辅助列做条件格式），**禁止**直接用一个 `formulaCondition` 公式一步完成：
- "**增加辅助列**，再/然后标记……"
- "**先计算/判断** XX **是否** YY，**再**标记……"
- "**新建一列**放结果，再用结果染色"
- 明确要求用"辅助列"、"辅助字段"、"判断列"、"标记列"

**正确做法（两步走）**：
```bash
# Step 1: 用 range update 在新列写判断公式（形成"是/否"辅助列，以 H2:H4 为例）
dws sheet range update --node NODE_ID --sheet-id SHEET_ID --range "H2:H4" \
  --values '[[{"type":"text","text":"=IF(A2>B2,\"是\",\"否\")"}],[{"type":"text","text":"=IF(A3>B3,\"是\",\"否\")"}],[{"type":"text","text":"=IF(A4>B4,\"是\",\"否\")"}]]'

# Step 2: 基于辅助列值做条件格式（用 formulaCondition 引用辅助列）
dws sheet cond-format create --node NODE_ID --sheet-id SHEET_ID \
  --ranges '["A2:H4"]' \
  --condition '{"formulaCondition":{"formula":"=$H2=\"是\""}}' \
  --cell-style '{"backgroundColor":"#FFECEC"}'
```

**错误做法（一步走绕过辅助列）**：
```bash
# 虽然逻辑等价，但产物里缺辅助列 → 用户打开表格看不到"是/否"列
dws sheet cond-format create --node NODE_ID --sheet-id SHEET_ID \
  --ranges '["A2:H100"]' \
  --condition '{"formulaCondition":{"formula":"=$A2>$B2"}}' \
  --cell-style '{"backgroundColor":"#FFECEC"}'
```

**为什么禁止一步走**：用户明确要求辅助列是有**业务意图**的——让人肉眼能在表里看到判断结果列；条件格式只是视觉辅助。一步 `formulaCondition` 虽然效果对了，但用户打开表格看不到辅助列，被视为"操作不完整"。

> `formulaCondition` 单独使用的场景是：用户**没有**明确要求辅助列、只要"标红符合条件的行"时。

#### 典型工作流

```
1. 先读取现有条件格式了解当前配置
   dws sheet cond-format list --node NODE_ID --sheet-id SHEET_ID

2. 创建/更新/删除条件格式规则
   dws sheet cond-format create --node NODE_ID --sheet-id SHEET_ID --ranges '["A1:E100"]' \
     --condition '{"numberCondition":{"operator":"greater","value1":"80"}}' \
     --cell-style '{"backgroundColor":"#FFCDD2","fontColor":"#B71C1C","bold":true}'

3. 再次读取验证结果是否生效
   dws sheet cond-format list --node NODE_ID --sheet-id SHEET_ID
```

#### 条件类型参考

条件类型（`--condition` JSON 的 key，每次只能选一种）：

| 条件类型 | 说明 | 参数 |
|---------|------|------|
| `numberCondition` | 数值比较 | operator: equal/not-equal/greater/greater-equal/less/less-equal/between/not-between + value1 + value2 |
| `textCondition` | 文本匹配 | operator: contains/not-contains/starts-with/ends-with + value |
| `emptyCondition` | 空值判断 | operator: is-empty/is-not-empty |
| `errorCondition` | 错误值 | operator: error/no-error |
| `duplicateCondition` | 重复/唯一值 | operator: duplicate/unique |
| `formulaCondition` | 自定义公式 | formula: "=A1>100" |
| `rankCondition` | 排名 | value + isPercent + isBottom |
| `averageCondition` | 高于/低于平均值 | isAbove + andEqual |
| `stdevCondition` | 标准差 | value + isAbove + andEqual |
| `dataBarCondition` | 数据条 | minPoint + maxPoint（每个含 type + value） |
| `iconSetCondition` | 图标集 | iconSet（数组）+ showIconOnly |
| `colorScaleCondition` | 色阶 | criterias（数组，每项含 type + value + color） |

## 命令详细参考

### 获取条件格式规则
```
Usage:
  dws sheet cond-format list [flags]
Example:
  # 获取所有条件格式规则
  dws sheet cond-format list --node <NODE_ID> --sheet-id <SHEET_ID>

  # 获取单个规则的详情
  dws sheet cond-format list --node <NODE_ID> --sheet-id <SHEET_ID> --rule-id <RULE_ID>
Flags:
      --node string       表格文档 ID 或 URL (必填)
      --sheet-id string   工作表 ID 或名称 (必填)
      --rule-id string    条件格式规则 ID (可选，不传则返回全部)
```

- **用途**：查看指定工作表中已有的条件格式规则，或获取单个规则的详情。
- **场景**：创建/更新/删除条件格式前后验证规则状态；获取 ruleId 供后续 update/delete 使用。
- **返回**：rules 数组，每条规则包含 id、type、ranges、条件参数、cellStyle/dataBarStyle 等。

### 创建条件格式规则
```
Usage:
  dws sheet cond-format create [flags]
Example:
  # 数值条件：大于 80 时标红加粗
  dws sheet cond-format create --node <NODE_ID> --sheet-id <SHEET_ID> \
    --ranges '["A1:A100"]' \
    --condition '{"numberCondition":{"operator":"greater","value1":"80"}}' \
    --cell-style '{"backgroundColor":"#FFCDD2","fontColor":"#B71C1C","bold":true}'

  # 文本条件：包含"延期"时加删除线
  dws sheet cond-format create --node <NODE_ID> --sheet-id <SHEET_ID> \
    --ranges '["B1:B50"]' \
    --condition '{"textCondition":{"operator":"contains","value":"延期"}}' \
    --cell-style '{"backgroundColor":"#FFF3E0","strikethrough":true}'

  # 数据条
  dws sheet cond-format create --node <NODE_ID> --sheet-id <SHEET_ID> \
    --ranges '["C1:C20"]' \
    --condition '{"dataBarCondition":{"minPoint":{"type":"auto"},"maxPoint":{"type":"auto"}}}' \
    --data-bar-style '{"fill":["#4CAF50","#F44336"],"isGradient":true}'

  # 色阶（三色）
  dws sheet cond-format create --node <NODE_ID> --sheet-id <SHEET_ID> \
    --ranges '["D1:D50"]' \
    --condition '{"colorScaleCondition":{"criterias":[{"type":"maxmin","color":"#F44336"},{"type":"percentile","value":"50","color":"#FFEB3B"},{"type":"maxmin","color":"#4CAF50"}]}}'

  # 重复值高亮
  dws sheet cond-format create --node <NODE_ID> --sheet-id <SHEET_ID> \
    --ranges '["E1:E100"]' \
    --condition '{"duplicateCondition":{"operator":"duplicate"}}' \
    --cell-style '{"backgroundColor":"#FCE4EC"}'
Flags:
      --node string            表格文档 ID 或 URL (必填)
      --sheet-id string        工作表 ID 或名称 (必填)
      --ranges string          应用范围 JSON 数组 (必填)，如 '["A1:E10"]'
      --condition string       条件类型及参数 JSON 对象 (必填)
      --cell-style string      单元格样式 JSON 对象 (可选)
      --data-bar-style string  数据条样式 JSON 对象 (可选，仅数据条类型)
```

- **用途**：在指定工作表中创建一条条件格式规则。
- **注意事项**：
  - 创建后**必须**用 `cond-format list` 验证规则是否生效
  - 中文"标红/高亮/染色"默认指 `backgroundColor`，"字体红"才是 `fontColor`
  - 日期/空值公式必须防空：`=AND(E1<>"", E1<=TODAY())` 而非 `=E1<=TODAY()`
  - 公式中用相对引用使公式随行变化
  - 创建前建议先 `range read` 读 3-5 行数据确认列对应关系
- **条件类型**（`--condition` JSON 的 key，每次只能选一种）：numberCondition / textCondition / emptyCondition / errorCondition / duplicateCondition / formulaCondition / rankCondition / averageCondition / stdevCondition / dataBarCondition / iconSetCondition / colorScaleCondition

### 更新条件格式规则
```
Usage:
  dws sheet cond-format update [flags]
Example:
  # 修改条件（改为大于 90）
  dws sheet cond-format update --node <NODE_ID> --sheet-id <SHEET_ID> --rule-id <RULE_ID> \
    --condition '{"numberCondition":{"operator":"greater","value1":"90"}}'

  # 修改样式
  dws sheet cond-format update --node <NODE_ID> --sheet-id <SHEET_ID> --rule-id <RULE_ID> \
    --cell-style '{"backgroundColor":"#C8E6C9","fontColor":"#1B5E20"}'

  # 修改应用范围
  dws sheet cond-format update --node <NODE_ID> --sheet-id <SHEET_ID> --rule-id <RULE_ID> \
    --ranges '["A1:F200"]'
Flags:
      --node string            表格文档 ID 或 URL (必填)
      --sheet-id string        工作表 ID 或名称 (必填)
      --rule-id string         条件格式规则 ID (必填)
      --ranges string          应用范围 JSON 数组 (可选)
      --condition string       条件类型及参数 JSON 对象 (可选，传入后替换原有条件)
      --cell-style string      单元格样式 JSON 对象 (可选)
      --data-bar-style string  数据条样式 JSON 对象 (可选，仅数据条类型)
```

- **用途**：更新已有条件格式规则的部分或全部配置。
- **场景**：修改阈值、切换条件类型、调整样式、扩大应用范围。
- **注意**：未传入的字段保持原有值不变；`--ranges`/`--condition`/`--cell-style`/`--data-bar-style` 至少传入一个。
- **ruleId 获取**：通过 `cond-format list` 获取。

### 删除条件格式规则

> **CAUTION:** 不可逆操作 — 执行前必须向用户确认。

```
Usage:
  dws sheet cond-format delete [flags]
Example:
  # 删除条件格式规则（必须加 --yes 确认）
  dws sheet cond-format delete --node <NODE_ID> --sheet-id <SHEET_ID> --rule-id <RULE_ID> --yes
Flags:
      --node string       表格文档 ID 或 URL (必填)
      --sheet-id string   工作表 ID 或名称 (必填)
      --rule-id string    条件格式规则 ID (必填)
```

- **用途**：删除指定条件格式规则。
- **幂等性**：规则已不存在时操作仍返回成功。
- **ruleId 获取**：通过 `cond-format list` 获取。

## 上下文传递

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `cond-format list` | `rules` 数组（含 `id`、`type`、`ranges`、条件参数、`cellStyle`/`dataBarStyle`） | 获取 ruleId 供 update / delete 使用；验证规则是否生效 |
| `cond-format create` | 新创建规则的 `id` | 用于后续 update / delete 的 --rule-id |
| `cond-format update` | 更新后的规则信息 | 确认更新结果 |
| `cond-format delete` | 操作结果 | 确认删除完成 |
| `list` | 工作表的 `sheetId` | cond-format list / create / update / delete 的 --sheet-id |

## 注意事项

- ★ **`--sheet-id` 获取规范（强制）**：`sheetId` 未知时必须先通过 `dws sheet list --node <NODE_ID> --format json` 查询，禁止凭空编造（如臆测为 `Sheet1`、`sheet1`、`0`、`default` 等）
- ★ **强制走条件格式的触发词**：用户说"标红/标黄/高亮/突出/标记/数据条/色阶/颜色随数据变"时，强制走 `cond-format create/update/delete`，禁止用 `range set-style` 写静态背景色代替
- **创建后必须验证**：条件格式创建后必须调用 `cond-format list` 验证规则是否生效
- **范围要精确**：条件格式的应用范围必须精确覆盖用户指定的列/行，不要遗漏也不要过度扩大
- **`backgroundColor` vs `fontColor` 的中文语义**：用户中文语境下的"标红/高亮/染色/标记"指**单元格背景色**，用 `backgroundColor`；"文字红/字体红/把字变红"才用 `fontColor`。默认无说明时选 `backgroundColor`
- **日期/空值比较必须防空**：用户说"过期的标红"时，公式必须排除空单元格。正确公式：`=AND(E1<>"", E1<=TODAY())`；错误公式：`=E1<=TODAY()`（空值会被当作 0 判为过期）
- **公式引用方式**：自定义公式条件中的单元格引用需要根据实际场景选择相对/绝对引用（如 `=E1<=TODAY()` 使用相对引用使公式随行变化）
- **创建前必须确认列对应**：仅读表头不够——如果表头语义含糊，formula 里引用的列字母可能张冠李戴。建议先读 3-5 行数据样本（如 `range read --range "A1:Z5"`）确认列名对应的实际值和数据类型
- **辅助列+条件格式两步走**：用户明确要求"辅助列"时，必须按两步走（先建辅助列 → 再基于辅助列做条件格式），禁止直接用 `formulaCondition` 一步绕过
- **大数据量优势**：当数据量 > 1000 行时，条件格式是首选——它由服务端自身渲染，不需要逐行调用 `range set-style`，性能远优于静态样式写入
- **判断标准**：交付后 `cond-format list` 必须能返回该规则；否则视为违规
