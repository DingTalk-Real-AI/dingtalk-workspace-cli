# 附件 — `DDAttachment`

> 分类：增强控件

文件上传。

## JSON Schema

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `componentName` | string | 是 | 固定值 `"DDAttachment"` |
| `props.id` | string | 是 | 唯一标识，格式 `DDAttachment_{随机字符串}` |
| `props.label` | string | 是 | 控件标题 |
| `props.required` | boolean | 否 | 是否必填，默认 false |

## 示例

```json
{
  "componentName": "DDAttachment",
  "props": {
    "id": "DDAttachment_1V6UJ230Q3NK0",
    "label": "附件",
    "required": false
  }
}
```
