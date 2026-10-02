# 审批人 — `approver`

> 分类：人工节点

核心决策节点，具有审批通过/拒绝的决策权。

## JSON Schema

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `name` | string | 是 | 节点显示名称 |
| `type` | string | 是 | 固定值 `"approver"` |
| `nodeId` | string | 是 | 节点唯一标识，格式 `{4位hex}_{4位hex}` |
| `prevId` | string | 是 | 上一个节点的 `nodeId` |
| `isDefaultName` | boolean | 否 | 是否为系统默认名称 |
| `properties` | object | 是 | 节点配置属性 |
| `childNode` | object | 否 | 下一个节点；无后续节点时省略该字段（禁止 `null`） |

## properties 字段

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `actionerRules` | array | 是 | 审批人选人规则列表（至少一条），详见 [选人规则](../process-nodes.md#选人规则actionerrules) |
| `activateType` | string | 是 | 多人激活方式：`"ALL"`（同时）或 `"ONE_BY_ONE"`（逐个） |
| `approvalType` | string | 是 | 固定值 `"MANUAL"` |
| `agreeAll` | boolean | 是 | 是否需要所有人同意：`true`（全部同意）/ `false`（任一同意） |
| `noneActionerAction` | string | 否 | 审批人为空时处理方式，如 `"admin"`（转交管理员） |
| `pickActioners` | array | 否 | 预设审批人列表 |

## 多人审批方式

| 审批方式 | `activateType` | `agreeAll` | 说明 |
|---------|---------------|-----------|------|
| 会签 | `"ALL"` | `true` | 所有审批人同时收到，全部同意才通过 |
| 或签 | `"ALL"` | `false` | 所有审批人同时收到，任一同意即通过 |
| 依次审批 | `"ONE_BY_ONE"` | `true` | 按顺序逐个审批，全部同意才通过 |

## 支持的选人规则

审批人节点支持全部 10 种选人规则，详见 [`rules/`](../rules/) 目录：

| 规则类型 | 说明 | 使用场景 |
|---------|------|---------|
| [`target_approval`](../rules/target_approval.md) | 指定成员 | 用户指定具体人名 |
| [`target_formula` (reportLineManager)](../rules/target_formula_reportLineManager.md) | 直属主管 | "直属主管"、"直属领导"、"汇报线主管" |
| [`target_originator`](../rules/target_originator.md) | 发起人自己 | "自己审"、"发起人确认" |
| [`target_management`](../rules/target_management.md) | 部门主管 | "部门负责人"、"部门经理"、"我的主管"、"主管审批"（默认） |
| [`target_formula` (managerOfDept)](../rules/target_formula_managerOfDept.md) | 表单部门主管 | "按表单部门走" |
| [`target_select`](../rules/target_select.md) | 发起人自选 | "自己挑审批人" |
| [`target_managers_labels`](../rules/target_managers_labels.md) | 角色标签主管 | "连续多级主管" |
| [`target_formcomponent_approval`](../rules/target_formcomponent_approval.md) | 表单内联系人 | "按表单选的人走" |
| [`target_label`](../rules/target_label.md) | 角色标签 | "财务审批"、"HR审批" |
| [`target_matrix_approval`](../rules/target_matrix_approval.md) | 审批矩阵 | "从审批矩阵获取" |

## 完整节点示例

```json
{
  "isDefaultName": false,
  "name": "部门主管审批",
  "prevId": "sid-startevent",
  "type": "approver",
  "nodeId": "3d09_da0a",
  "properties": {
    "actionerRules": [
      {
        "level": 1,
        "autoUp": true,
        "isEmpty": false,
        "actType": "",
        "type": "target_management"
      }
    ],
    "noneActionerAction": "admin",
    "activateType": "ONE_BY_ONE",
    "approvalType": "MANUAL",
    "agreeAll": false
  }
}
```
