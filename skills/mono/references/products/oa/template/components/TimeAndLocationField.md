# 地点 — `TimeAndLocationField`

> 分类：增强控件

自动获取当前时间和地理位置。

## JSON Schema

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `componentName` | string | 是 | 固定值 `"TimeAndLocationField"` |
| `props.id` | string | 是 | 唯一标识，格式 `TimeAndLocationField_{随机字符串}` |
| `props.label` | array | 是 | 控件标题，格式 `["当前时间", "当前地点"]` |
| `props.required` | boolean | 否 | 是否必填，默认 false |

## 示例

```json
{
  "componentName": "TimeAndLocationField",
  "props": {
    "id": "TimeAndLocationField_O9DJISFUKAO0",
    "label": ["当前时间", "当前地点"],
    "required": false
  }
}
```
