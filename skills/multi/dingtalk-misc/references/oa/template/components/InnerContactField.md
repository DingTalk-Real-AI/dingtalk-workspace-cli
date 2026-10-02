# 联系人 — `InnerContactField`

> 分类：增强控件

选择企业内部成员。

## JSON Schema

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `componentName` | string | 是 | 固定值 `"InnerContactField"` |
| `props.id` | string | 是 | 唯一标识，格式 `InnerContactField_{随机字符串}` |
| `props.label` | string | 是 | 控件标题 |
| `props.placeholder` | string | 否 | 占位提示文字 |
| `props.required` | boolean | 否 | 是否必填，默认 false |
| `props.choice` | string | 否 | 选择模式，`"0"` 单选，`"1"` 多选 |

## 示例

```json
{
  "componentName": "InnerContactField",
  "props": {
    "id": "InnerContactField_1U1MDQCY7DKW0",
    "label": "联系人",
    "placeholder": "请选择",
    "required": false,
    "choice": "0"
  }
}
```
