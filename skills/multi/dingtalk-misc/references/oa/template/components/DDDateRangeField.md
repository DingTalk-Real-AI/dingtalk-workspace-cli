# 日期区间 — `DDDateRangeField`

> 分类：基础控件

选择开始和结束日期，可自动计算时长。

## JSON Schema

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `componentName` | string | 是 | 固定值 `"DDDateRangeField"` |
| `props.id` | string | 是 | 唯一标识，格式 `DDDateRangeField_{随机字符串}` |
| `props.label` | array | 是 | 控件标题，格式 `["开始时间", "结束时间"]` |
| `props.placeholder` | string | 否 | 占位提示文字 |
| `props.required` | boolean | 否 | 是否必填，默认 false |
| `props.format` | string | 否 | 日期格式，如 `"yyyy-MM-dd"` |
| `props.duration` | boolean | 否 | 是否显示时长，默认 false |
| `props.durationLabel` | string | 条件必填 | 时长标签，如 `"时长"`。**当 `duration` 为 `true` 时必须存在且非空** |
| `props.unit` | string | 否 | 时间单位，如 `"天"` |

## 示例

```json
{
  "componentName": "DDDateRangeField",
  "props": {
    "id": "DDDateRangeField_1VJ5F4XR8WHS0",
    "label": ["开始时间", "结束时间"],
    "placeholder": "请选择",
    "required": false,
    "format": "yyyy-MM-dd",
    "duration": false,
    "durationLabel": "时长",
    "unit": "天"
  }
}
```

## 约束

- **`duration` 与 `durationLabel` 联动**：当 `props.duration` 为 `true`（显示时长）时，**必须**同时提供非空的 `props.durationLabel`（如 `"时长"`）与 `props.unit`（如 `"天"`）；缺失 `durationLabel` 会导致时长行无标题。`duration` 为 `false` 或未设置时，`durationLabel` 可省略。