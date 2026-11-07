# 并行分支 — `parallel`

> 分类：分支节点

并行执行多条分支，所有分支完成后汇合继续流程。

> ⚠️ 注意：以下 `properties` 内部字段结构需结合实际配置验证，当前基于流程设计器通用模式推导。

## JSON Schema

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `name` | string | 是 | 节点显示名称 |
| `type` | string | 是 | 固定值 `"parallel"` |
| `nodeId` | string | 是 | 节点唯一标识，格式 `{4位hex}_{4位hex}` |
| `prevId` | string | 是 | 上一个节点的 `nodeId` |
| `isDefaultName` | boolean | 否 | 是否为系统默认名称 |
| `properties` | object | 是 | 节点配置属性 |
| `childNode` | object | 否 | 并行分支汇合后的下一个节点；无后续时省略（禁止 `null`） |

## properties 字段

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `branches` | array | 是 | 并行分支列表 |
| `branches[].name` | string | 是 | 分支名称 |
| `branches[].childNode` | object | 是 | 该分支内的第一个子节点；分支末尾节点省略 `childNode`（禁止 `null`） |

## 示例

```json
{
  "isDefaultName": false,
  "name": "并行审批",
  "prevId": "a1b2_c3d4",
  "type": "parallel",
  "nodeId": "7e8f_9a0b",
  "properties": {
    "branches": [
      {
        "name": "法务审批",
        "childNode": {
          "isDefaultName": false,
          "name": "法务审批",
          "prevId": "7e8f_9a0b",
          "type": "approver",
          "nodeId": "1c2d_3e4f",
          "properties": {
            "actionerRules": [
              {
                "labelNames": "法务",
                "isEmpty": false,
                "actType": "",
                "type": "target_label",
                "labels": "TODO_LABEL_ID_7"
              }
            ],
            "noneActionerAction": "admin",
            "activateType": "ONE_BY_ONE",
            "approvalType": "MANUAL",
            "agreeAll": false
          }
        }
      },
      {
        "name": "财务审批",
        "childNode": {
          "isDefaultName": false,
          "name": "财务审批",
          "prevId": "7e8f_9a0b",
          "type": "approver",
          "nodeId": "5a6b_7c8d",
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
            "noneActionerAction": "admin",
            "activateType": "ONE_BY_ONE",
            "approvalType": "MANUAL",
            "agreeAll": false
          }
        }
      }
    ]
  }
}
```
