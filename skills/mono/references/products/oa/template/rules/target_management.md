# 选人规则：部门主管 — `target_management`

> 适用节点：审批人（`approver`）、办理人（`handler`）、抄送人（`notifier`）

从发起人所在部门获取指定层级的主管。

## 字段说明

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `type` | string | 是 | 固定值 `"target_management"` |
| `level` | number | 是 | 主管层级（`1` = 直接部门主管） |
| `autoUp` | boolean | 否 | 找不到时是否向上级部门查找，默认 `true` |
| `isEmpty` | boolean | 是 | 固定 `false` |
| `actType` | string | 否 | `""` |

## 使用场景

- 用户说"部门负责人"、"部门经理"、"部门主管审批"
- 抄送给"部门负责人"

## 示例

### 审批人节点中使用

```json
{
  "level": 1,
  "autoUp": true,
  "isEmpty": false,
  "actType": "",
  "type": "target_management"
}
```

### 抄送人节点中使用（简化形式）

```json
{
  "type": "target_management",
  "level": 1
}
```

## 注意事项

- `level` 表示部门主管层级：`1` = 发起人直属部门主管，`2` = 上级部门主管
- `autoUp: true` 时，若当前部门无主管则自动向上级部门查找
- 在抄送人节点中使用时，字段可简化（无需 `isEmpty`、`autoUp` 等）
- 通常配合 `noneActionerAction: "admin"` 使用
