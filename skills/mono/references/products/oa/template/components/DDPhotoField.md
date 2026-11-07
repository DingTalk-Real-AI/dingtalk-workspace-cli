# 图片 — `DDPhotoField`

> 分类：增强控件

图片上传，可选水印。

## JSON Schema

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `componentName` | string | 是 | 固定值 `"DDPhotoField"` |
| `props.id` | string | 是 | 唯一标识，格式 `DDPhotoField_{随机字符串}` |
| `props.label` | string | 是 | 控件标题 |
| `props.required` | boolean | 否 | 是否必填，默认 false |
| `props.watermark` | boolean | 否 | 是否添加水印，默认 false |

## 示例

```json
{
  "componentName": "DDPhotoField",
  "props": {
    "id": "DDPhotoField_1YQDBCE4I1340",
    "label": "图片",
    "required": false,
    "watermark": false
  }
}
```
