# 选人规则：表单内联系人 — `target_formcomponent_approval`

> 适用节点：审批人（`approver`）、办理人（`handler`）

从表单内的联系人控件（`InnerContactField`）获取审批人/办理人。

## 字段说明

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `type` | string | 是 | 固定值 `"target_formcomponent_approval"` |
| `paramKey` | string | 是 | 联系人控件 ID（如 `InnerContactField_XXX`） |
| `label` | string | 否 | 控件标签名（显示用） |
| `isEmpty` | boolean | 是 | 固定 `false` |
| `actType` | string | 否 | `""` |

## 使用场景

- 用户说"按表单选的人走"、"由表单里选择的联系人审批"
- 审批人取决于表单中填写的联系人字段

## 示例

```json
{
  "paramKey": "InnerContactField_RVKBXBDIT7K0",
  "isEmpty": false,
  "actType": "",
  "label": "联系人",
  "type": "target_formcomponent_approval"
}
```

## 注意事项

- 依赖表单中存在 `InnerContactField` 类型的控件
- `paramKey` 必须是表单中已存在的联系人控件 `props.id`
- 通常配合 `noneActionerAction: "admin"` 使用
- 流程通过控件 `props.id` 引用（`paramKey`），无需为该控件设置 `bizAlias`
