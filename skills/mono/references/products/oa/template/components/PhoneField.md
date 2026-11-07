# 电话 — `PhoneField`

> 分类：基础控件

电话号码输入。

## JSON Schema

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `componentName` | string | 是 | 固定值 `"PhoneField"` |
| `props.id` | string | 是 | 唯一标识，格式 `PhoneField_{随机字符串}` |
| `props.label` | string | 是 | 控件标题 |
| `props.placeholder` | string | 否 | 占位提示文字 |
| `props.required` | boolean | 否 | 是否必填，默认 false |
| `props.mode` | string | 否 | 电话类型，`"phone"` 手机 |

## 示例

```json
{
  "componentName": "PhoneField",
  "props": {
    "id": "PhoneField_V1XP8RQLOOW0",
    "label": "电话",
    "placeholder": "请输入",
    "required": false,
    "mode": "phone"
  }
}
```
