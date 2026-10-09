# 说明文字 — `TextNote`

> 分类：基础控件

静态文字展示，用于添加表单说明。不采集数据。

## JSON Schema

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `componentName` | string | 是 | 固定值 `"TextNote"` |
| `props.id` | string | 是 | 唯一标识，格式 `TextNote_{随机字符串}` |
| `props.content` | string | 是 | 显示的说明文字内容 |
| `props.notPrint` | string | 否 | 是否不打印，`"0"` 打印，`"1"` 不打印 |

## 示例

```json
{
  "componentName": "TextNote",
  "props": {
    "id": "TextNote_1N180VZEVHS00",
    "content": "请输入说明文字",
    "notPrint": "0"
  }
}
```
