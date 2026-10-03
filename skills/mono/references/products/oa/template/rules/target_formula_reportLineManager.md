# 选人规则：直属主管 — `target_formula` (reportLineManager)

> 适用节点：审批人（`approver`）、办理人（`handler`）、抄送人（`notifier`）

从发起人的汇报线获取指定层级的主管。

## 字段说明

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `type` | string | 是 | 固定值 `"target_formula"` |
| `subType` | string | 是 | 固定值 `"reportLineManager"` |
| `formula` | string | 是 | 公式：`"ReportLineManager(corpId,originator,N)"`，N 为主管层级 |
| `isEmpty` | boolean | 是 | 固定 `false` |

## 使用场景

- 用户**明确**说"直属主管"、"直属领导"、"汇报线主管"
- 需要按汇报线逐级审批（如"一级主管"、"二级主管"）

> ⚠️ "我的主管"、"主管审批"等未明确指定"直属 / 汇报线"的说法，**默认使用部门主管 `target_management`**，而非本规则。

## formula 参数说明

`ReportLineManager(corpId,originator,N)` 中：
- `corpId`：固定参数，表示企业 ID
- `originator`：固定参数，表示发起人
- `N`：主管层级，`1` = 直接主管，`2` = 主管的主管，依此类推

## 示例

```json
{
  "isEmpty": false,
  "formula": "ReportLineManager(corpId,originator,1)",
  "subType": "reportLineManager",
  "type": "target_formula"
}
```

## 注意事项

- 通常配合 `noneActionerAction: "admin"` 使用，当汇报线上找不到主管时转交管理员
- 层级 N 的值取决于用户需求："直属主管"→1，"主管的主管"→2
