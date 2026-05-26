# AI 表格公式字段指南

> 当用户要创建 formula 类型字段、编写表内计算公式、做派生指标时，必须先读本文档。

## 1. 何时使用 formula 字段

| 场景 | 用 formula | 不用 formula |
|------|-----------|-------------|
| 长期展示在表中的派生值（如"总价=单价×数量"） | ✅ | |
| 条件标记（如"超期=IF(截止日期<TODAY(),'是','否')"） | ✅ | |
| 文本拼接（如"全名=姓&名"） | ✅ | |
| 一次性统计分析（如"本月总销售额"） | | ✅ 用 record query + 本地聚合 |
| 跨表查找引用 | | ✅ 用 lookup 字段（见下方说明） |

## 2. 创建 formula 字段

```bash
dws aitable field create \
  --base-id <baseId> \
  --table-id <tableId> \
  --name "总价" \
  --type formula \
  --config '{"formula": "[单价] * [数量]"}' \
  --format json
```

### config 结构

```json
{
  "formula": "<公式表达式>"
}
```

- `formula` 是唯一必填字段
- 表达式中引用字段使用 **方括号 + 字段名**：`[字段名]`
- 字段名必须精确匹配（含空格、大小写）

## 3. 公式语法

### 3.1 引用规则

| 引用方式 | 语法 | 说明 |
|---------|------|------|
| 引用本表字段 | `[字段名]` | 字段名必须精确匹配 |
| 引用关联表字段 | 不支持 | 需要用 lookup 字段 |

### 3.2 常用函数分类

#### 数值计算

| 函数 | 用途 | 示例 |
|------|------|------|
| `+` `-` `*` `/` | 四则运算 | `[单价] * [数量]` |
| `SUM(...)` | 求和 | `SUM([Q1], [Q2], [Q3], [Q4])` |
| `ROUND(value, digits)` | 四舍五入 | `ROUND([金额] * 0.1, 2)` |
| `ABS(value)` | 绝对值 | `ABS([差额])` |
| `MAX(a, b, ...)` | 最大值 | `MAX([成绩1], [成绩2])` |
| `MIN(a, b, ...)` | 最小值 | `MIN([报价1], [报价2])` |

#### 文本处理

| 函数 | 用途 | 示例 |
|------|------|------|
| `&` | 文本拼接 | `[姓] & [名]` |
| `CONCATENATE(...)` | 拼接多个值 | `CONCATENATE([城市], "-", [区])` |
| `LEFT(text, n)` | 取左侧 n 字符 | `LEFT([编号], 4)` |
| `RIGHT(text, n)` | 取右侧 n 字符 | `RIGHT([手机], 4)` |
| `LEN(text)` | 文本长度 | `LEN([备注])` |
| `UPPER(text)` / `LOWER(text)` | 大小写转换 | `UPPER([代码])` |

#### 逻辑判断

| 函数 | 用途 | 示例 |
|------|------|------|
| `IF(条件, 真值, 假值)` | 条件判断 | `IF([金额] > 1000, "大额", "普通")` |
| `AND(a, b, ...)` | 逻辑与 | `IF(AND([状态]="完成", [评分]>=4), "优秀", "")` |
| `OR(a, b, ...)` | 逻辑或 | `IF(OR([等级]="A", [等级]="B"), "通过", "未通过")` |
| `NOT(expr)` | 逻辑非 | `NOT([已归档])` |
| `SWITCH(expr, v1, r1, v2, r2, ..., default)` | 多条件匹配 | `SWITCH([状态], "待办","🔴", "进行中","🟡", "完成","🟢", "")` |

#### 日期函数

| 函数 | 用途 | 示例 |
|------|------|------|
| `TODAY()` | 当前日期 | `IF([截止日期] < TODAY(), "已逾期", "正常")` |
| `NOW()` | 当前时间 | `NOW()` |
| `YEAR(date)` / `MONTH(date)` / `DAY(date)` | 提取年/月/日 | `YEAR([创建时间])` |
| `DATEDIF(start, end, unit)` | 日期差 | `DATEDIF([开始], [结束], "d")` 返回天数 |
| `DATEADD(date, count, unit)` | 日期加减 | `DATEADD([创建时间], 7, "d")` |

> `DATEDIF` 的 unit 参数：`"y"`=年, `"m"`=月, `"d"`=天

#### 空值处理

| 函数 | 用途 | 示例 |
|------|------|------|
| `BLANK()` | 空值常量 | `IF([备注] = BLANK(), "无", [备注])` |
| `IF(field, ...)` | 字段为空时视为 false | `IF([评分], [评分], 0)` |

## 4. 常见公式模板

### 4.1 计算类

```
// 含税价格
[不含税价] * (1 + [税率])

// 完成率百分比
[已完成数] / [总数]

// 折扣后价格
[原价] * (1 - [折扣率])
```

### 4.2 状态标记类

```
// 逾期标记
IF([截止日期] < TODAY(), "⚠️ 已逾期", "正常")

// 优先级标签
SWITCH([优先级], "紧急","🔴P0", "高","🟠P1", "中","🟡P2", "低","🟢P3", "")

// 进度状态
IF([进度] >= 1, "✅ 已完成", IF([进度] > 0, "🔄 进行中", "⏳ 未开始"))
```

### 4.3 文本拼接类

```
// 编号生成
"PRJ-" & [项目编码] & "-" & [序号]

// 地址拼接
[省] & [市] & [区] & [详细地址]
```

## 5. 注意事项与限制

### 5.1 formula 字段是只读的

- formula 字段的值由系统自动计算，**不能通过 `record create/update` 写入**
- 如果用户要"设置某个计算结果"，应引导其修改源字段

### 5.2 字段名必须精确

- 公式中的 `[字段名]` 必须与表中实际字段名完全一致
- 创建 formula 字段前，先通过 `field get` 确认字段名

### 5.3 循环引用

- formula 字段不能引用自身
- 不能形成 A→B→A 的循环引用

### 5.4 与跨表引用字段的区别

钉钉 AI 表格有两种跨表取值方式：`lookup`（关联引用）和 `filterUp`（查找引用）。

| 维度 | formula | lookup (关联引用) | filterUp (查找引用) |
|------|---------|-----------------|-------------------|
| 字段类型 | `formula` | `lookup` | `filterUp` |
| 数据来源 | 本表字段 | 通过已有关联字段（bidirectionalLink/unidirectionalLink）取关联表字段 | 直接指定目标表 + 筛选条件取值 |
| 前置条件 | 无 | 必须先有关联字段 | 无需关联字段 |
| 适用场景 | 本表内计算、条件判断 | "我关联了某条记录，取它的某个字段值" | "在另一张表里按条件查找记录并聚合取值" |

#### lookup config（已验证）

```json
{
  "associateField": "<本表中的关联字段 fieldId（bidirectionalLink/unidirectionalLink 类型）>",
  "valuesField": "<关联目标表中要取值的字段 fieldId>",
  "aggregator": "SUM|AVERAGE|COUNT|MAX|MIN|CONCATENATE"
}
```

创建示例：
```bash
dws aitable field create --base-id <baseId> --table-id <tableId> \
  --name "关联名称" --type lookup \
  --config '{"associateField":"<linkFieldId>","valuesField":"<targetFieldId>","aggregator":"CONCATENATE"}'
```

#### filterUp config（已验证）

```json
{
  "targetSheet": "<目标表 tableId>",
  "filters": [
    {
      "fieldId": "<目标表字段Id>",
      "operator": "equal|contain",
      "value": "<匹配值>",
      "link": "AND"
    }
  ],
  "valuesField": "<目标表中要取值的字段Id>",
  "aggregator": "SUM|AVERAGE|COUNT|MAX|MIN|CONCATENATE"
}
```

> `filters` 必须非空（至少一条筛选规则）。
> `filters[].operator` 仅支持：`equal`、`contain`（`not_equal`/`not_contain`/`is_empty` 等均不支持）。
> `filters[].link` 统一为 `"AND"` 或 `"OR"`。

### 5.5 创建前检查清单

1. 已通过 `field get` 确认所有引用字段的精确名称
2. 引用字段不包含 formula/lookup 等只读字段（可能导致二次计算延迟）
3. 公式语法正确（括号匹配、函数名正确）
4. 字段类型兼容（数值运算的字段确实是 number 类型）

## 6. 更新 formula 字段

```bash
dws aitable field update \
  --base-id <baseId> \
  --table-id <tableId> \
  --field-id <fieldId> \
  --config '{"formula": "[新字段A] + [新字段B]"}' \
  --format json
```

更新时只需传新的 `formula` 表达式，系统会自动重新计算所有记录。
