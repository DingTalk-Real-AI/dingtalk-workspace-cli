# 日期 — `DDDateField`

> 分类：基础控件

选择单个日期。

## JSON Schema

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `componentName` | string | 是 | 固定值 `"DDDateField"` |
| `props.id` | string | 是 | 唯一标识，格式 `DDDateField_{随机字符串}` |
| `props.label` | string | 是 | 控件标题 |
| `props.placeholder` | string | 否 | 占位提示文字 |
| `props.required` | boolean | 否 | 是否必填，默认 false |
| `props.format` | string | 否 | 日期格式，如 `"yyyy-MM-dd"` |
| `props.unit` | string | 否 | 时间单位，如 `"天"` |

## 示例

```json
{
  "componentName": "DDDateField",
  "props": {
    "id": "DDDateField_1OHVCWCN2YXS0",
    "label": "日期",
    "placeholder": "请选择",
    "required": false,
    "format": "yyyy-MM-dd",
    "unit": "天"
  }
}
```
