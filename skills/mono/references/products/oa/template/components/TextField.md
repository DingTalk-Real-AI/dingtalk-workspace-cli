# 单行输入框 — `TextField`

> 分类：基础控件

单行文本输入，适用于姓名、标题等短文本场景。

## JSON Schema

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `componentName` | string | 是 | 固定值 `"TextField"` |
| `props.id` | string | 是 | 唯一标识，格式 `TextField_{随机字符串}` |
| `props.label` | string | 是 | 控件标题 |
| `props.placeholder` | string | 否 | 占位提示文字 |
| `props.required` | boolean | 否 | 是否必填，默认 false |
| `props.ratio` | number | 否 | 宽度比例 |

## 示例

```json
{
  "componentName": "TextField",
  "props": {
    "id": "TextField-K2AD4O5B",
    "label": "单行输入框",
    "placeholder": "请输入",
    "required": false
  }
}
```
