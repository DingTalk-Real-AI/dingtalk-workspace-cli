# 抄送人 — `notifier`

> 分类：人工节点

仅接收通知，无审批决策权。抄送人节点支持配置多条 `actionerRules`，可同时抄送多种类型的人员。

## JSON Schema

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `name` | string | 是 | 节点显示名称 |
| `type` | string | 是 | 固定值 `"notifier"` |
| `nodeId` | string | 是 | 节点唯一标识，格式 `{4位hex}_{4位hex}` |
| `prevId` | string | 是 | 上一个节点的 `nodeId` |
| `isDefaultName` | boolean | 否 | 是否为系统默认名称 |
| `properties` | object | 是 | 节点配置属性 |
| `childNode` | object | 否 | 下一个节点；无后续节点时省略该字段（禁止 `null`） |

## properties 字段

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `actionerRules` | array | 是 | 抄送人选人规则列表，支持多条规则组合，详见 [选人规则](../process-nodes.md#选人规则actionerrules) |

## 支持的选人规则

抄送人节点支持以下选人规则，且可以在一个节点中组合使用多条规则：

| 规则类型 | 说明 | 典型用法 |
|--------|------|---------|
| [`target_select`](../rules/target_select.md) | 发起人自选 | 提交时选择抄送人 |
| [`target_label`](../rules/target_label.md) | 角色标签 | 抄送给“财务”、“负责人”等角色 |
| [`target_management`](../rules/target_management.md) | 部门主管 | 抄送给 N 级部门主管 |
| [`target_approval`](../rules/target_approval.md) | 指定成员 | 抄送给固定的人 |
| [`target_formula`](../rules/target_formula_reportLineManager.md) | 汇报线主管 | 抄送给直属主管 |

## 示例：多规则抄送

```json
{
  "name": "抄送人",
  "prevId": "c705_1f5b",
  "type": "notifier",
  "nodeId": "1cc3_959a",
  "properties": {
    "actionerRules": [
      {
        "select": ["allStaff"],
        "range": {},
        "type": "target_select",
        "key": "manual_1cc3_959a_6bf9_54c0",
        "multi": 1
      },
      {
        "type": "target_label",
        "labels": "TODO_LABEL_ID_2",
        "labelNames": "负责人"
      },
      {
        "type": "target_label",
        "labels": "TODO_LABEL_ID_3",
        "labelNames": "财务"
      },
      {
        "type": "target_management",
        "level": 2
      },
      {
        "type": "target_approval",
        "approvals": [
          {
            "userName": "示例审批人四",
            "workNo": "TODO_USER_ID_4",
            "value": "TODO_USER_ID_4",
            "name": "示例审批人四"
          }
        ]
      }
    ]
  },
  "isDefaultName": false
}
```
