# 付款人 — `payer`

> 分类：套件节点

财务套件节点，执行付款操作。

> ⚠️ 注意：以下 `properties` 内部字段结构需结合实际配置验证，当前基于流程设计器通用模式推导。

## JSON Schema

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `name` | string | 是 | 节点显示名称 |
| `type` | string | 是 | 固定值 `"payer"` |
| `nodeId` | string | 是 | 节点唯一标识，格式 `{4位hex}_{4位hex}` |
| `prevId` | string | 是 | 上一个节点的 `nodeId` |
| `isDefaultName` | boolean | 否 | 是否为系统默认名称 |
| `properties` | object | 是 | 节点配置属性 |
| `childNode` | object | 否 | 下一个节点；无后续节点时省略该字段（禁止 `null`） |

## properties 字段

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `actionerRules` | array | 是 | 付款人规则列表（结构同审批人 `actionerRules`） |
| `paymentConfig` | object | 否 | 付款配置 |
| `paymentConfig.amountField` | string | 否 | 金额字段（表单控件 ID） |
| `paymentConfig.accountField` | string | 否 | 收款账户字段（表单控件 ID） |

## 示例

```json
{
  "isDefaultName": false,
  "name": "付款执行",
  "prevId": "a1b2_c3d4",
  "type": "payer",
  "nodeId": "9e0f_1a2b",
  "properties": {
    "actionerRules": [
      {
        "labelNames": "财务",
        "isEmpty": false,
        "actType": "",
        "type": "target_label",
        "labels": "TODO_LABEL_ID_3"
      }
    ],
    "paymentConfig": {
      "amountField": "MoneyField_TOTAL001",
      "accountField": "RecipientAccountField_ACCT001"
    }
  }
}
```
