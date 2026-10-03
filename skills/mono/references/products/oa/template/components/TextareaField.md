# 多行输入框 — `TextareaField`

> 分类：基础控件

多行文本输入，适用于说明、备注等较长文本场景。

## JSON Schema

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `componentName` | string | 是 | 固定值 `"TextareaField"` |
| `props.id` | string | 是 | 唯一标识，格式 `TextareaField_{随机字符串}` |
| `props.label` | string | 是 | 控件标题 |
| `props.placeholder` | string | 否 | 占位提示文字 |
| `props.required` | boolean | 否 | 是否必填，默认 false |

## 示例

```json
{
  "componentName": "TextareaField",
  "props": {
    "id": "TextareaField_BHWO7SB108W0",
    "label": "多行输入框",
    "placeholder": "请输入",
    "required": false
  }
}
```
