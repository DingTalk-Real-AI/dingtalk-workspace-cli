# 选人规则：发起人自己 — `target_originator`

> 适用节点：审批人（`approver`）、办理人（`handler`）

发起人自己作为审批人/办理人。

## 字段说明

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `type` | string | 是 | 固定值 `"target_originator"` |
| `isEmpty` | boolean | 是 | 固定 `false` |

## 使用场景

- 用户说"自己审"、"发起人本人确认"
- 需要发起人自己确认或办理某个步骤

## 示例

```json
{
  "isEmpty": false,
  "type": "target_originator"
}
```

## 注意事项

- 该规则结构最简单，仅需 `type` 和 `isEmpty` 两个字段
- 常用于需要发起人自己确认的环节
