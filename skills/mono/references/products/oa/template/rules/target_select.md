# 选人规则：发起人自选 — `target_select`

> 适用节点：审批人（`approver`）、办理人（`handler`）、抄送人（`notifier`）

发起人提交审批时自行选择审批人/办理人/抄送人。

## 字段说明

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `type` | string | 是 | 固定值 `"target_select"` |
| `select` | array | 是 | 选择范围，`["allStaff"]` 为全公司可选 |
| `range` | object | 否 | 范围限制 |
| `range.approvals` | array | 否 | 限定可选人员列表 |
| `range.labels` | array | 否 | 限定可选角色标签 |
| `key` | string | 是 | 唯一标识，格式 `manual_{nodeId}_{随机hex}` |
| `multi` | number | 是 | 可选人数（`1` = 单选） |
| `isEmpty` | boolean | 是 | 固定 `false` |
| `actType` | string | 否 | `""` |

## 使用场景

- 用户说"发起人自选"、"自己挑审批人"、"提交时选择"
- 需要灵活指定审批人，不固定

## 示例

### 全公司可选，单选

```json
{
  "select": ["allStaff"],
  "isEmpty": false,
  "range": {
    "approvals": [],
    "labels": []
  },
  "actType": "",
  "type": "target_select",
  "key": "manual_d22c_5854_f505_7ec7",
  "multi": 1
}
```

### 抄送人节点中使用（简化形式）

```json
{
  "select": ["allStaff"],
  "range": {},
  "type": "target_select",
  "key": "manual_1cc3_959a_6bf9_54c0",
  "multi": 1
}
```

## 注意事项

- `key` 的格式为 `manual_{当前nodeId}_{随机4位hex}_{随机4位hex}`
- `multi` 控制可选人数：`1` = 只能选一人，大于 1 时可选多人
- `range` 为空对象 `{}` 或含空数组时表示无范围限制
