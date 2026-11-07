# 评分 — `StarRatingField`

> 分类：增强控件

星级评分。

## JSON Schema

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `componentName` | string | 是 | 固定值 `"StarRatingField"` |
| `props.id` | string | 是 | 唯一标识，格式 `StarRatingField_{随机字符串}` |
| `props.label` | string | 是 | 控件标题 |
| `props.placeholder` | string | 否 | 占位提示文字 |
| `props.required` | boolean | 否 | 是否必填，默认 false |
| `props.limit` | number | 否 | 最大评分星数，默认 5 |

## 示例

```json
{
  "componentName": "StarRatingField",
  "props": {
    "id": "StarRatingField_1APSCMZIEG000",
    "label": "评分",
    "placeholder": "请输入",
    "required": false,
    "limit": 5
  }
}
```
