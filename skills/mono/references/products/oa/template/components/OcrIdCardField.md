# 身份证识别 — `OcrIdCardField`

> 分类：高级控件

OCR 识别身份证，通过 children 映射字段。

## JSON Schema

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `componentName` | string | 是 | 固定值 `"OcrIdCardField"` |
| `props.id` | string | 是 | 唯一标识，格式 `OcrIdCardField_{随机字符串}` |
| `props.label` | string | 是 | 控件标题 |
| `props.placeholder` | string | 否 | 占位提示文字 |
| `props.required` | boolean | 否 | 是否必填 |
| `props.type` | string | 是 | 固定值 `"ocr"` |
| `props.ocrType` | string | 是 | 固定值 `"idcard"` |
| `props.options` | array | 是 | OCR 识别字段列表，如 `["name", "idCardNo", "gender"]` |
| `children` | array | 是 | 映射子控件列表 |

## 关键机制

children 中每个控件的 `bizAlias` 与 `options` 数组一一对应，OCR 识别后自动映射填充。

## 示例

```json
{
  "componentName": "OcrIdCardField",
  "props": {
    "id": "OcrIdCardField_OR42XF5T70W0",
    "label": "身份证识别",
    "placeholder": "识别出的信息会自动填充到表单",
    "required": true,
    "type": "ocr",
    "ocrType": "idcard",
    "options": ["name", "idCardNo", "gender"]
  },
  "children": [
    {
      "componentName": "TextField",
      "props": {
        "id": "TextField_1YF738771SPS0",
        "label": "姓名",
        "placeholder": "请输入",
        "required": true,
        "bizAlias": "name"
      }
    },
    {
      "componentName": "IdCardField",
      "props": {
        "id": "IdCardField_1IKTLPOOHTEO0",
        "label": "身份证号码",
        "placeholder": "请输入",
        "required": true,
        "bizAlias": "idCardNo"
      }
    }
  ]
}
```
