# 省市区 — `AddressField`

> 分类：增强控件

省市区三级地址选择。

## JSON Schema

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `componentName` | string | 是 | 固定值 `"AddressField"` |
| `props.id` | string | 是 | 唯一标识，格式 `AddressField_{随机字符串}` |
| `props.label` | string | 是 | 控件标题 |
| `props.required` | boolean | 否 | 是否必填，默认 false |
| `props.needDetail` | boolean | 否 | 是否需要详细地址输入，默认 false |

## 示例

```json
{
  "componentName": "AddressField",
  "props": {
    "id": "AddressField_W3XUWOIN0V40",
    "label": "省市区",
    "required": false,
    "needDetail": false
  }
}
```
