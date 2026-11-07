# 发起人 — `start`

> 分类：发起节点

流程起点，系统默认添加，不可删除。发起人节点是整个流程树的根节点，`nodeId` 固定为 `sid-startevent`。

## JSON Schema

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `name` | string | 是 | 节点名称，默认 `"发起人"` |
| `type` | string | 是 | 固定值 `"start"` |
| `nodeId` | string | 是 | 固定值 `"sid-startevent"` |
| `properties` | object | 是 | 空对象 `{}` |
| `childNode` | object/null | 是 | 下一个节点 |

## 示例

```json
{
  "name": "发起人",
  "type": "start",
  "nodeId": "sid-startevent",
  "properties": {},
  "childNode": {
    "name": "审批人",
    "type": "approver",
    "nodeId": "1a2b_3c4d",
    "prevId": "sid-startevent",
    "properties": {
      "actionerRules": [],
      "activateType": "ONE_BY_ONE",
      "approvalType": "MANUAL",
      "agreeAll": false
    }
  }
}
```
