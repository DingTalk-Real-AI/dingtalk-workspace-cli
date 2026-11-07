# 选人规则：角色标签主管 — `target_managers_labels`

> 适用节点：审批人（`approver`）、办理人（`handler`）

获取拥有指定角色标签的主管人员。

## 字段说明

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `type` | string | 是 | 固定值 `"target_managers_labels"` |
| `labelNames` | array | 是 | 角色标签名称列表 |
| `labels` | array | 是 | 角色标签 ID 列表 |
| `levels` | array | 否 | 层级列表（空数组表示所有层级） |
| `isEmpty` | boolean | 是 | 固定 `false` |

## 使用场景

- 用户说"连续多级主管"、"具有某角色标签的主管逐级审批"
- 需要按角色标签查找多级主管

## 示例

```json
{
  "labelNames": [
    "子管理员"
  ],
  "isEmpty": false,
  "type": "target_managers_labels",
  "levels": [],
  "labels": [
    "TODO_LABEL_ID_1"
  ]
}
```

## 注意事项

- `labels` 为角色标签 ID（非名称），**严禁编造**：已知角色名时用通讯录（contact）MCP server 的 `search_label_by_name` 直接查询；未知时先 `get_org_labels` 获取全部角色列表后匹配
- `labelNames` 与 `labels` 一一对应，取上述查询返回中的角色名称
- `levels` 为空数组时表示不限定层级
- 通常配合 `noneActionerAction: "admin"` 使用
