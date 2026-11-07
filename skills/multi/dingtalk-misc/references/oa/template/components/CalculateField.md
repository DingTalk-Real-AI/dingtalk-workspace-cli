# 计算公式 — `CalculateField`

> 分类：增强控件

对表单内数值字段自动计算。

## JSON Schema

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `componentName` | string | 是 | 固定值 `"CalculateField"` |
| `props.id` | string | 是 | 唯一标识，格式 `CalculateField_{随机字符串}` |
| `props.label` | string | 是 | 控件标题 |
| `props.formula` | array | 是 | 计算公式（**必填，不能为空数组**），数组元素按顺序拼接，见下文「formula 结构」 |
| `props.placeholder` | string | 否 | 占位提示文字 |
| `props.notUpper` | string | 否 | `"0"` 显示大写金额，`"1"` 不显示 |

## 示例

```json
{
  "componentName": "CalculateField",
  "props": {
    "id": "CalculateField_PIIZHXI39V40",
    "label": "合计金额",
    "formula": [
      { "id": "NumberField_UNITPRICE01" },
      "*",
      { "id": "NumberField_QUANTITY01" }
    ],
    "placeholder": "自动计算数值",
    "notUpper": "0"
  }
}
```

## formula 结构

`props.formula` 是一个**数组**，按顺序拼成一个四则运算表达式，元素共三种：

| 元素形式 | 说明 |
|---------|------|
| `{ "id": "<数值字段 props.id>" }` | 引用另一个数值型字段的值，只能引用 `NumberField` / `MoneyField` / `CalculateField` / `DiscountField`，且其 `id` **必须是本表单中已定义的控件 `props.id`**（不能引用自身） |
| 运算符字符串 | `"+"`、`"-"`、`"*"`、`"/"`、`"("`、`")"` |
| 数字字面量 | 如 `2`、`100`；小数点用 `"."` |

示例：“合计 = 单价 × 数量” → `[ { "id": "NumberField_UNITPRICE01" }, "*", { "id": "NumberField_QUANTITY01" } ]`。

## 约束

- `formula` **必填且不得为空数组**；公式必须能构成合法的数学表达式（括号配对、运算符不悬空）。
- `formula` 中引用的字段 `id` 必须是当前表单已存在的数值型控件 `id`（由脚本生成），**不得引用未定义或非数值型字段，也不能编造 id**。
