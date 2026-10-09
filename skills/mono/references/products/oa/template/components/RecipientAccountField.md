# 收款账户 — `RecipientAccountField`

> 分类：业务控件

选择收款账户信息。

## JSON Schema

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `componentName` | string | 是 | 固定值 `"RecipientAccountField"` |
| `props.id` | string | 是 | 唯一标识，格式 `RecipientAccountField_{随机字符串}` |
| `props.label` | string | 是 | 控件标题 |
| `props.placeholder` | string | 否 | 占位提示文字 |
| `props.required` | boolean | 否 | 是否必填，默认 false |

## 示例

```json
{
  "componentName": "RecipientAccountField",
  "props": {
    "id": "RecipientAccountField_1SVC4LTF93MO0",
    "label": "收款账户",
    "placeholder": "请选择",
    "required": true
  }
}
```
