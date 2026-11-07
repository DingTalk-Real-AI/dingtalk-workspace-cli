# 数字输入框 — `NumberField`

> 分类：基础控件

纯数字输入，适用于数量、天数等数值场景。

## JSON Schema

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `componentName` | string | 是 | 固定值 `"NumberField"` |
| `props.id` | string | 是 | 唯一标识，格式 `NumberField_{随机字符串}` |
| `props.label` | string | 是 | 控件标题 |
| `props.placeholder` | string | 否 | 占位提示文字 |
| `props.required` | boolean | 否 | 是否必填，默认 false |
| `props.ratio` | number | 否 | 宽度比例 |

## 示例

```json
{
  "componentName": "NumberField",
  "props": {
    "id": "NumberField_1YSOWSQA4PKW0",
    "label": "数字输入框",
    "placeholder": "请输入数字",
    "required": false,
    "ratio": 50
  }
}
```
