# 金额 — `MoneyField`

> 分类：增强控件

金额输入，自动显示大写金额。

## JSON Schema

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `componentName` | string | 是 | 固定值 `"MoneyField"` |
| `props.id` | string | 是 | 唯一标识，格式 `MoneyField_{随机字符串}` |
| `props.label` | string | 是 | 控件标题 |
| `props.placeholder` | string | 否 | 占位提示文字 |
| `props.required` | boolean | 否 | 是否必填，默认 false |
| `props.notUpper` | string | 否 | `"0"` 显示大写金额，`"1"` 不显示 |

## 示例

```json
{
  "componentName": "MoneyField",
  "props": {
    "id": "MoneyField_ZZ6UVVOE5DS0",
    "label": "金额（元）",
    "placeholder": "请输入金额",
    "required": false,
    "notUpper": "0"
  }
}
```
