# 选人规则：角色标签 — `target_label`

> 适用节点：审批人（`approver`）、办理人（`handler`）、抄送人（`notifier`）

获取拥有指定角色标签的人员。

本页 `TODO_LABEL_ID_*` 是待替换的示意占位符。真实查询返回的 `labelId` 保持数值类型；组装审批规则时，再按 `labels` 字段要求使用字符串或字符串数组。

## 字段说明

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `type` | string | 是 | 固定值 `"target_label"` |
| `labelNames` | string | 是 | 角色名称 |
| `labels` | string | 是 | 角色标签 ID |
| `isEmpty` | boolean | 是 | 固定 `false` |
| `actType` | string | 否 | `"and"`（会签）或 `""`（其他） |

## 使用场景

- 用户说"财务审批"、"HR 审批"、"法务审批"等岗位/角色词
- 抄送给某个角色（如"通知财务"）

## 角色查询（强制）

使用本规则时，**禁止**凭空猜测或手写 `labels`（角色标签 ID）。**必须**先通过钉钉通讯录 MCP 的 `get_org_labels` 工具查询当前组织的角色信息，再从返回结果中匹配目标角色，取其 `labelId` 填入 `labels`、`name` 填入 `labelNames`。

> 🚫 **禁止**把 `labels` 写成 `TODO_...` 之类占位或任意编造值。若查不到真实角色 ID（角色不存在、有歧义未确认、MCP 不可用等），**宁可不生成该 `target_label` 规则乃至该节点**，并在回复中说明缺失、请用户补充，**绝不**用未定义值填充。

### 返回结构

`get_org_labels` 无入参，返回按**角色分组**组织，结构如下：

```json
{
  "success": true,
  "result": [
    {
      "groupName": "职务",
      "labels": [
        { "labelId": "TODO_LABEL_ID_8", "name": "财务" },
        { "labelId": "TODO_LABEL_ID_9", "name": "人事" }
      ]
    },
    {
      "groupName": "岗位",
      "labels": [
        { "labelId": "TODO_LABEL_ID_10", "name": "总监" }
      ]
    }
  ]
}
```

字段含义：

| 返回字段 | 含义 | 映射到规则 |
|---------|------|-----------|
| `result[].groupName` | 角色分组名（如“职务”“岗位”） | 仅用于缩小匹配范围 / 消除歧义 |
| `result[].labels[].labelId` | 角色 ID（number） | → `labels`（转为 string） |
| `result[].labels[].name` | 角色显示名称 | → `labelNames` |

### 步骤

1. 调用钉钉通讯录 MCP 的 `get_org_labels`，获取当前组织的全部角色分组及其角色列表。
2. 遍历所有分组的 `labels`，按 `name` 匹配目标角色（如“财务”“出纳”“HR”）。
3. 取匹配角色的 `labelId`（转为字符串）→ `labels`，`name` → `labelNames`。
4. 若同一 `name` 在多个分组中重复出现（如“研发”可能同时属于“职务”和“岗位”），**向用户确认具体分组 / 角色**，不要自行选择。
5. 若未找到匹配角色，向用户确认角色名称或提示该角色不存在，**不要**编造 ID。

> 补充：若只知道角色名称，也可用 `search_label_by_name` 按名称搜索角色；但名称仍可能多个，需同样确认。

## 示例

### 审批人节点中使用

```json
{
  "labelNames": "出纳",
  "isEmpty": false,
  "actType": "and",
  "type": "target_label",
  "labels": "TODO_LABEL_ID_5"
}
```

### 抄送人节点中使用（简化形式）

```json
{
  "type": "target_label",
  "labels": "TODO_LABEL_ID_3",
  "labelNames": "财务"
}
```

## 注意事项

- `labels` 为角色标签 ID（非名称），**必须**取自 `get_org_labels` 返回的 `labelId`（见上文“角色查询”）；填入时转为字符串
- `labelNames` 为角色的显示名称，取自同一角色的 `name`，与 `labels` 一一对应
- `actType: "and"` 表示该角色下所有人都需操作（会签）
- 这是最常用的"按角色/岗位"选人规则
- 注意：`labelNames` 和 `labels` 在此规则中为 **string** 类型（非数组），与 `target_managers_labels` 不同
