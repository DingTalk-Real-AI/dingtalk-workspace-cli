# 选人规则：表单部门控件的主管 — `target_formula` (managerOfDept)

> 适用节点：审批人（`approver`）、办理人（`handler`）

从表单中选择的部门控件获取该部门的主管。

## 字段说明

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `type` | string | 是 | 固定值 `"target_formula"` |
| `subType` | string | 是 | 固定值 `"managerOfDept"` |
| `formula` | string | 是 | 公式：`"ManagerOfDept(corpId,$('DepartmentField_ID'),N)"` |
| `isEmpty` | boolean | 是 | 固定 `false` |
| `actType` | string | 否 | `""` |

## 使用场景

- 用户说"按表单选的部门走"、"由表单里选择的部门的负责人审批"
- 审批人取决于表单中填写的部门

## formula 参数说明

`ManagerOfDept(corpId,$('DepartmentField_ID'),N)` 中：
- `corpId`：固定参数，表示企业 ID
- `$('DepartmentField_ID')`：引用表单中的部门控件 ID
- `N`：主管层级，`1` = 该部门直接主管

## 示例

```json
{
  "isEmpty": false,
  "formula": "ManagerOfDept(corpId,$('DepartmentField_EA5DNJML9JS0'),1)",
  "subType": "managerOfDept",
  "actType": "",
  "type": "target_formula"
}
```

## 注意事项

- 依赖表单中存在 `DepartmentField` 类型的控件
- 控件 ID 必须是表单中已存在的部门控件 `props.id`
- 通常配合 `noneActionerAction: "admin"` 使用
- 流程通过控件 `props.id` 引用（公式内 `$('DepartmentField_ID')`），无需为该控件设置 `bizAlias`
