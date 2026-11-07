# 关联表单 — `FormRelateField`

> 分类：关联表单控件

关联其他审批模板的表单数据。

## JSON Schema

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `componentName` | string | 是 | 固定值 `"FormRelateField"` |
| `props.id` | string | 是 | 唯一标识，格式 `FormRelateField_{随机字符串}` |
| `props.label` | string | 是 | 控件标题 |
| `props.title` | string | 否 | 标题 |
| `props.required` | boolean | 否 | 是否必填，默认 false |
| `props.multi` | number | 否 | 是否多选，`0` 单选，`1` 多选 |
| `props.quote` | number | 否 | 是否引用，`1` 引用 |
| `props.extract` | boolean | 否 | 是否提取字段 |
| `props.displayExtract` | boolean | 否 | 是否展示提取字段 |
| `props.securityMode` | number | 否 | 安全模式 |
| `props.procType` | string | 否 | 流程类型，`"inner"` 内部 |
| `props.dataSource` | object | 是 | 数据源配置 |
| `props.dataSource.type` | string | 是 | 固定值 `"form"` |
| `props.dataSource.target.formCode` | string | 是 | 关联审批模板 processCode |
| `props.fields` | array | 否 | 关联字段列表 |

## 关键说明

- `dataSource.target.formCode` 指定关联的审批模板 processCode
- `fields` 中每个字段的 `_extractId` 为提取映射 ID，`_oriId` 为原始表单中的字段 ID

## 示例

```json
{
  "componentName": "FormRelateField",
  "props": {
    "id": "FormRelateField_K8W2ZATWKWG0",
    "label": "用车申请",
    "title": "",
    "required": false,
    "multi": 0,
    "quote": 1,
    "extract": false,
    "displayExtract": true,
    "securityMode": 0,
    "procType": "inner",
    "dataSource": {
      "type": "form",
      "params": { "filter": "" },
      "target": {
        "appUuid": "",
        "bizType": "",
        "formCode": "TODO_FORM_CODE",
        "appType": 0
      }
    },
    "fields": [
      {
        "componentName": "TextField",
        "props": {
          "id": "TextField-IH4UNLKF",
          "label": "申请部门",
          "placeholder": "请输入",
          "_extractId": "TextField_1U73ZYBTWCGW0",
          "_oriId": "TextField-IH4UNLKF"
        }
      }
    ]
  }
}
```
