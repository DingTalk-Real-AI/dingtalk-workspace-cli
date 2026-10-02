# 流水号 — `SeqNumberField`

> 分类：高级控件

自动生成唯一流水号。

## JSON Schema

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `componentName` | string | 是 | 固定值 `"SeqNumberField"` |
| `props.id` | string | 是 | 唯一标识，格式 `SeqNumberField_{随机字符串}` |
| `props.label` | string | 是 | 控件标题 |
| `props.rule` | array | 是 | 流水号规则 |
| `props.rule[].type` | string | 是 | 规则类型：`"date"` 日期 / `"counter"` 计数器 |
| `props.rule[].value` | string/object | 是 | 日期格式或计数器配置 |

## 规则说明

- `type: "date"` — value 为日期格式字符串，如 `"YYYYMMDD"`
- `type: "counter"` — value 为对象：
  - `start`: 起始值
  - `length`: 位数
  - `reset`: 重置周期，可选 `"year"` / `"month"` / `"day"` / `"never"`

## 示例

```json
{
  "componentName": "SeqNumberField",
  "props": {
    "id": "SeqNumberField_2AX8EBLEVO6",
    "label": "流水号",
    "rule": [
      { "type": "date", "value": "YYYYMMDD" },
      { "type": "counter", "value": { "start": 1, "length": 8, "reset": "year" } }
    ]
  }
}
```
