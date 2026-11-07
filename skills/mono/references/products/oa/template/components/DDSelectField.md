# 单选框 — `DDSelectField`

> 分类：基础控件

从预设选项中选择一项。

## JSON Schema

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `componentName` | string | 是 | 固定值 `"DDSelectField"` |
| `props.id` | string | 是 | 唯一标识，格式 `DDSelectField_{随机字符串}` |
| `props.label` | string | 是 | 控件标题 |
| `props.placeholder` | string | 否 | 占位提示文字 |
| `props.required` | boolean | 否 | 是否必填，默认 false |
| `props.spread` | boolean | 否 | 是否平铺展示选项，默认 false |
| `props.ratio` | number | 否 | 宽度比例 |
| `props.options` | array | 是 | 选项列表，每项含 `key` 和 `value`。`key` **必须为英文字符**，推荐用 `option`、`_` 与有序数字拼接（如 `option_0`、`option_1`）；`value` 为选项显示文本 |

## 示例

```json
{
  "componentName": "DDSelectField",
  "props": {
    "id": "DDSelectField_1KH2A2YCMOF40",
    "label": "单选框",
    "placeholder": "请选择",
    "required": false,
    "spread": false,
    "ratio": 50,
    "options": [
      { "key": "option_0", "value": "选项1" },
      { "key": "option_1", "value": "选项2" },
      { "key": "option_2", "value": "选项3" }
    ]
  }
}
```
