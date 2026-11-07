# 分栏 — `ColumnLayout`

> 分类：布局控件

将多个控件并排显示，通过 ratio 控制宽度比例。

## JSON Schema

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `componentName` | string | 是 | 固定值 `"ColumnLayout"` |
| `props.id` | string | 是 | 唯一标识，格式 `ColumnLayout_{随机字符串}` |
| `props.label` | string | 是 | 控件标题，默认 `"分栏"` |
| `props.notPrint` | string | 否 | 是否不打印，`"1"` 不打印 |
| `props.group` | array | 是 | 分栏分组，每组包含子控件 |

## 示例

```json
{
  "componentName": "ColumnLayout",
  "props": {
    "id": "ColumnLayout_1U8IPGN6Q4ZK0",
    "label": "分栏",
    "notPrint": "1",
    "group": []
  }
}
```
