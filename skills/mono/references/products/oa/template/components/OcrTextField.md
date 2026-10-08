# 通用文字识别 — `OcrTextField`

> 分类：高级控件

OCR 文字识别，自动填入表单。

## JSON Schema

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `componentName` | string | 是 | 固定值 `"OcrTextField"` |
| `props.id` | string | 是 | 唯一标识，格式 `OcrTextField_{随机字符串}` |
| `props.label` | string | 是 | 控件标题 |
| `props.placeholder` | string | 否 | 占位提示文字 |
| `props.required` | boolean | 否 | 是否必填，默认 false |
| `props.type` | string | 是 | 固定值 `"ocr"` |
| `props.ocrType` | string | 是 | OCR 类型，`"advanced"` 通用识别 |

## 示例

```json
{
  "componentName": "OcrTextField",
  "props": {
    "id": "OcrTextField_G1X5Y19ENXC0",
    "label": "通用文字识别",
    "placeholder": "请输入",
    "required": false,
    "type": "ocr",
    "ocrType": "advanced"
  }
}
```
