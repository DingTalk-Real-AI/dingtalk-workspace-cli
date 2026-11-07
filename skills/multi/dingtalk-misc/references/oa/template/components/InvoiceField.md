# 发票 — `InvoiceField`

> 分类：业务控件

发票信息采集与验真。

## JSON Schema

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `componentName` | string | 是 | 固定值 `"InvoiceField"` |
| `props.id` | string | 是 | 唯一标识，格式 `InvoiceField_{随机字符串}` |
| `props.label` | string | 是 | 控件标题 |
| `props.required` | boolean | 否 | 是否必填，默认 false |
| `props.bizAlias` | string | 否 | 业务别名 |
| `props.appId` | string | 是 | 应用 ID，固定 `"78641"` |
| `props.invoiceChecking` | string | 否 | 是否验真，`"true"` 验真 |
| `props.commonSetterConfig` | object | 否 | 通用配置 |
| `props.commonSetterConfig.paymentBeforeInvoice` | array | 否 | 付款方式，如 `["NO_INVOICE", "TODO_COLLECT"]` |

## 示例

```json
{
  "componentName": "InvoiceField",
  "props": {
    "id": "InvoiceField_1E4UHFVGH2F",
    "label": "发票",
    "required": false,
    "bizAlias": "",
    "appId": "78641",
    "invoiceChecking": "true",
    "commonSetterConfig": {
      "paymentBeforeInvoice": ["NO_INVOICE", "TODO_COLLECT"]
    }
  }
}
```
