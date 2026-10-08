# 身份证 — `IdCardField`

> 分类：基础控件

身份证号码输入，自带格式校验。

## JSON Schema

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `componentName` | string | 是 | 固定值 `"IdCardField"` |
| `props.id` | string | 是 | 唯一标识，格式 `IdCardField_{随机字符串}` |
| `props.label` | string | 是 | 控件标题 |
| `props.placeholder` | string | 否 | 占位提示文字 |
| `props.required` | boolean | 否 | 是否必填，默认 false |

## 示例

```json
{
  "componentName": "IdCardField",
  "props": {
    "id": "IdCardField_1JHL06H19U000",
    "label": "身份证",
    "placeholder": "请输入",
    "required": false
  }
}
```
