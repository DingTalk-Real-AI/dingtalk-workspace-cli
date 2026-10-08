# 办理人 — `handler`

> 分类：人工节点

执行具体工作后返回流程，无审批决策权。

> ⚠️ 注意：以下 `properties` 内部字段结构需结合实际配置验证，当前基于与审批人节点的结构对齐推导。

## JSON Schema

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `name` | string | 是 | 节点显示名称 |
| `type` | string | 是 | 固定值 `"handler"` |
| `nodeId` | string | 是 | 节点唯一标识，格式 `{4位hex}_{4位hex}` |
| `prevId` | string | 是 | 上一个节点的 `nodeId` |
| `isDefaultName` | boolean | 否 | 是否为系统默认名称 |
| `properties` | object | 是 | 节点配置属性 |
| `childNode` | object | 否 | 下一个节点；无后续节点时省略该字段（禁止 `null`） |

## properties 字段

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `actionerRules` | array | 是 | 办理人选人规则列表，详见 [选人规则](../process-nodes.md#选人规则actionerrules) |
| `activateType` | string | 是 | 多人激活方式：`"ALL"` 或 `"ONE_BY_ONE"` |

## 支持的选人规则

办理人节点支持以下选人规则（与审批人相同，但不含审批矩阵）：

- [`target_approval`](../rules/target_approval.md) — 指定成员
- [`target_formula` (reportLineManager)](../rules/target_formula_reportLineManager.md) — 直属主管
- [`target_originator`](../rules/target_originator.md) — 发起人自己
- [`target_management`](../rules/target_management.md) — 部门主管
- [`target_formula` (managerOfDept)](../rules/target_formula_managerOfDept.md) — 表单部门主管
- [`target_select`](../rules/target_select.md) — 发起人自选
- [`target_managers_labels`](../rules/target_managers_labels.md) — 角色标签主管
- [`target_formcomponent_approval`](../rules/target_formcomponent_approval.md) — 表单内联系人
- [`target_label`](../rules/target_label.md) — 角色标签

## 示例

```json
{
  "isDefaultName": false,
  "name": "行政办理",
  "prevId": "a1b2_c3d4",
  "type": "handler",
  "nodeId": "f1e2_d3c4",
  "properties": {
    "actionerRules": [
      {
        "labelNames": "行政",
        "isEmpty": false,
        "actType": "",
        "type": "target_label",
        "labels": "TODO_LABEL_ID_6"
      }
    ],
    "activateType": "ONE_BY_ONE"
  }
}
```
