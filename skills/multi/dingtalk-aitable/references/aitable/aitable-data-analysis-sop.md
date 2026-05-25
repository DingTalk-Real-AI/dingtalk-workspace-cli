# AI 表格数据分析 SOP

> 当用户诉求涉及查询、筛选、排序、统计、Top/Bottom N、分组聚合、判断全局结论时，必须先读本文档再执行。

## 1. 查询决策树

```
用户要做什么？
│
├─ 查看/导出原始记录明细
│   → record query [--filters] [--sort] [--field-ids] [--limit]
│
├─ 按条件筛选记录（如"状态=进行中的记录"）
│   → record query --filters '{"operator":"and","operands":[...]}'
│
├─ 取 Top N / Bottom N（如"销售额最高的5条"）
│   → record query --sort '[{"fieldId":"xxx","direction":"desc"}]' --limit 5
│
├─ 全量统计（如"一共多少条"、"所有记录的总销售额"）
│   → record query --all --field-ids <目标字段>
│   → 本地计算 count / sum / avg
│
├─ 分组统计（如"每个状态各有多少条"）
│   → record query --all --field-ids <分组字段>,<度量字段>
│   → 本地按分组字段 groupby 再聚合
│
└─ 判断全局结论（如"是否所有记录都满足条件"）
    → record query --all（或用 --filters 反向筛选不满足的）
    → 基于全量结果判断
```

## 2. 核心规则

### 2.1 禁止基于默认分页下全局结论

`record query` 默认返回 100 条。如果返回 JSON 中 `data.nextCursor` 非空，表示还有后续数据，当前结果**不是全量**。

```json
{"data": {"nextCursor": "3hf5MtLbLZ", "records": [...]}}
```

- ❌ 错误：只查了默认 100 条就说"共 100 条记录"
- ✅ 正确：使用 `--all` 自动翻页拿全量，再统计

### 2.2 能在服务端过滤的，不要拉到本地再过滤

| 需求 | 正确做法 | 错误做法 |
|------|---------|---------|
| 筛选"状态=已完成" | `--filters '{"operator":"and","operands":[{"operator":"eq","operands":["fldXXX","已完成"]}]}'` | `--all` 拉全量再本地 filter |
| 按日期降序取最新5条 | `--sort '[...]' --limit 5` | `--all` 拉全量再本地 sort + slice |
| 模糊搜索标题含"Q1" | `--filters` 用 `contain` 操作符 | 全量拉取再本地 grep |

### 2.3 服务端无法完成时，才用 --all + 本地计算

以下场景服务端 filters/sort 无法满足，需要 `--all` 后本地处理：

- SUM / AVG / COUNT / MAX / MIN 聚合
- 分组统计（GROUP BY）
- 多字段联合计算（如"销售额 = 单价 × 数量"）
- 去重计数（COUNT DISTINCT）
- 百分比/占比计算

### 2.4 --all 使用注意

```bash
dws aitable record query \
  --base-id <baseId> \
  --table-id <tableId> \
  --all \
  --field-ids <只取需要的字段> \
  --format json
```

- **必须配合 `--field-ids`** 限制返回字段，减少数据量
- 对于大表（>1000条），先告知用户可能耗时
- `--all` 会自动处理分页，无需手动翻页

## 3. filters 快速参考

详细语法见 [aitable-filter-sort.md](./aitable-filter-sort.md)。

### 常用操作符速查

| 操作符 | 适用类型 | 含义 | 示例 operands |
|--------|---------|------|-------------|
| `eq` | 通用 | 等于 | `["fldXXX", "值"]` |
| `ne` | 通用 | 不等于 | `["fldXXX", "值"]` |
| `gt` / `lt` | 数值/日期 | 大于/小于 | `["fldXXX", "25"]` |
| `gte` / `lte` | 数值/日期 | 大于等于/小于等于 | `["fldXXX", "100"]` |
| `contain` | 文本 | 包含 | `["fldXXX", "关键词"]` |
| `exist` / `un_exist` | 通用 | 有值/为空 | `["fldXXX"]`（无第二参数） |
| `any_of` | 多选 | 包含任一 | `["fldXXX", "选项A"]` |

### filters 结构模板

```json
{
  "operator": "and",
  "operands": [
    {"operator": "eq", "operands": ["<fieldId>", "<值>"]},
    {"operator": "gt", "operands": ["<fieldId>", "<数值>"]}
  ]
}
```

## 4. 分析结果呈现规范

### 4.1 必须包含的信息

- **数据范围**：基于哪个表、哪些筛选条件、查询了多少条记录
- **计算方法**：用了什么聚合方式（sum/count/avg 等）
- **结果值**：精确到合理小数位

### 4.2 示例

> 基于「销售数据」表，筛选条件：日期 ≥ 2026-01-01，共查询到 342 条记录。
> - 总销售额：¥1,234,567.89（SUM）
> - 平均单价：¥3,610.46（AVG）
> - 最大单笔：¥89,000.00（MAX）

## 5. 任务选路心智模型

| 用户诉求 | 优先方案 | 不要误走 |
|---------|---------|---------|
| 一次性统计/临时分析 | `record query --all` + 本地聚合 | 不要创建 formula 字段 |
| 长期展示派生指标 | 创建 formula 字段（见 [formula-guide](./aitable-formula-guide.md)） | 不要每次手算再手动写入 |
| 按条件筛选记录 | `record query --filters` | 不要 `--all` 拉全量再本地 filter |
| 取最新/最大/前N | `--sort + --limit` | 不要 `--all` 再本地排序取前N |
| 关键词检索 | `record query --filters` 用 `contain` | 不要把表格当搜索引擎全文检索 |
| 验证"是否全部满足" | 反向 filters（筛不满足的），看是否有结果 | 不要 `--all` 逐条遍历 |
