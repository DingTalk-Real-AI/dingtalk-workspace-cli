# 外部联系人 — `ExternalContactField`

> 分类：增强控件

选择企业外部联系人。

## JSON Schema

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `componentName` | string | 是 | 固定值 `"ExternalContactField"` |
| `props.id` | string | 是 | 唯一标识，格式 `ExternalContactField_{随机字符串}` |
| `props.label` | string | 是 | 控件标题 |
| `props.placeholder` | string | 否 | 占位提示文字 |
| `props.required` | boolean | 否 | 是否必填，默认 false |

## 示例

```json
{
  "componentName": "ExternalContactField",
  "props": {
    "id": "ExternalContactField_1VPC59AATTR40",
    "label": "外部联系人",
    "placeholder": "请选择",
    "required": false
  }
}
```
