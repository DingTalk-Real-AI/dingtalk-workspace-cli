# OA 审批流程节点与审批人规则参考

本文档描述钉钉 OA 审批的流程节点类型、审批模式、条件分支和审批人选择规则，用于理解审批模板结构和正确填写 `create-instance` 的节点参数。

---

## 流程结构概览

审批流程是一个嵌套树结构：

- **根节点**：发起人节点（`type: "start"`，`nodeId: "sid-startevent"`），固定不可删除
- **后续节点**：通过 `childNode` 链接形成链式结构
- **分支节点**：条件分支（`route` + `condition`）或并行分支（`parallel`）
- 当没有后续节点时，`childNode` 字段**必须省略**（不可设为 `null`）

---

## 7 种节点类型

### 1. 发起人节点（start）

| 属性 | 值 |
|------|-----|
| `type` | `start` |
| `nodeId` | `sid-startevent`（固定） |
| `properties` | `{}`（空对象） |

唯一、不可删除。是流程的起点。

### 2. 审批人节点（approver）

核心决策节点，有审批/拒绝权限。

| 属性 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `actionerRules` | Array | 是 | 审批人选择规则，至少一条 |
| `activateType` | String | 是 | 多人审批模式（见下方） |
| `approvalType` | String | 是 | 固定 `"MANUAL"` |
| `agreeAll` | Boolean | 是 | `true` 全部通过 / `false` 任一通过 |
| `noneActionerAction` | String | 否 | 如 `"admin"`（找不到审批人时转管理员） |

支持全部 10 种 actionerRules 类型。

### 3. 办理人节点（handler）

执行工作，无审批决策权。

| 属性 | 类型 | 必填 |
|------|------|------|
| `actionerRules` | Array | 是 |
| `activateType` | String | 是 |

支持 9 种 actionerRules（不支持 `target_matrix_approval`）。

### 4. 抄送人节点（notifier）

仅接收通知，无决策权。

| 属性 | 类型 | 必填 |
|------|------|------|
| `actionerRules` | Array | 是 |

支持多条 actionerRules 组合在一个节点中，实现同时抄送多类人员。

### 5. 条件分支（route + condition）

条件路由节点，包含多个条件分支。

**route 节点：**
- `type: "route"`
- `conditionNodes[]`：分支数组，按优先级排序，**默认分支必须在最后**
- `properties: {}`

**condition 节点（conditionNodes 的每个元素）：**
- `type: "condition"`
- `isdefault: true`：标记默认分支
- `properties.conditions`：二维条件数组
  - 外层数组：多个条件组，**OR 关系**
  - 内层数组：多个条件对象，**AND 关系**
  - 默认分支：`[[]]`（一个空组）

### 6. 并行分支（parallel）

多个分支同时执行，全部完成后才继续。

| 属性 | 说明 |
|------|------|
| `branches[]` | 分支数组 |
| `branches[].name` | 分支名称 |
| `branches[].childNode` | 该分支的第一个节点 |

### 7. 付款人节点（payer）

财务付款节点。

| 属性 | 说明 |
|------|------|
| `actionerRules` | 审批人规则 |
| `paymentConfig.amountField` | 金额控件 ID |
| `paymentConfig.accountField` | 收款账户控件 ID |

---

## 多人审批模式

| 模式 | `activateType` | `agreeAll` | 说明 |
|------|---------------|-----------|------|
| 会签 | `"ALL"` | `true` | 所有审批人都必须审批通过 |
| 或签 | `"ALL"` | `false` | 任一审批人审批即可 |
| 依次审批 | `"ONE_BY_ONE"` | `true` | 按顺序逐级审批 |

---

## 10 种审批人选择规则（actionerRules）

### 1. 指定成员（target_approval）

明确指定具体人员。

```json
{
  "type": "target_approval",
  "approvals": [
    { "userName": "张三", "workNo": "manager123" }
  ],
  "isEmpty": false
}
```

- `workNo` 必须通过 `dws aisearch person --keyword "<工号>" --dimension jobNumber --format json` 获取，**严禁编造**
- 在 `create-instance` 中对应 `directAppointedApprovers` 的 `staffIds`

### 2. 直属主管（target_formula / reportLineManager）

按汇报线找到直属主管。

```json
{
  "type": "target_formula",
  "subType": "reportLineManager",
  "formula": "ReportLineManager(corpId,originator,1)",
  "isEmpty": false
}
```

- `formula` 中最后的数字 N 表示第 N 级主管
- **重要区分：** 用户说"直属主管/直属领导/汇报线主管"才用此规则；用户说"主管审批/leader审批"（模糊）时默认用 `target_management`（部门主管）

### 3. 发起人自己（target_originator）

发起人自行审批。

```json
{
  "type": "target_originator",
  "isEmpty": false
}
```

最简单的规则，只有 `type` 和 `isEmpty`。

### 4. 部门主管（target_management）

从发起人所在部门层级找主管。

```json
{
  "type": "target_management",
  "level": 1,
  "autoUp": true,
  "isEmpty": false
}
```

- `level: 1`：直接部门主管
- `autoUp: true`：找不到时向上级部门搜索
- **这是"主管审批/leader审批"模糊场景的默认选择**

### 5. 表单部门主管（target_formula / managerOfDept）

根据表单中部门控件选择的主管。

```json
{
  "type": "target_formula",
  "subType": "managerOfDept",
  "formula": "ManagerOfDept(corpId,$('DepartmentField_XXX'),1)",
  "isEmpty": false
}
```

- `formula` 中引用表单中的 `DepartmentField` 控件 ID

### 6. 发起人自选（target_select）

发起人在提单时自行选择审批人。

```json
{
  "type": "target_select",
  "select": ["allStaff"],
  "range": {},
  "key": "manual_nodeId_xxxx_yyyy",
  "multi": 1,
  "isEmpty": false
}
```

- `select: ["allStaff"]`：可选全组织人员
- `multi: 1`：单选
- `key`：格式 `manual_{nodeId}_{hex}_{hex}`
- 在 `create-instance` 中对应 `targetSelectActioners` 的 `actionerKey`

### 7. 角色标签主管（target_managers_labels）

按角色标签找多级主管。

```json
{
  "type": "target_managers_labels",
  "labelNames": ["项目经理"],
  "labels": ["labelId123"],
  "levels": [1],
  "isEmpty": false
}
```

- `labels` 中的 ID 必须通过 `dws contact label get --names "<角色名>" --format json` 获取；已知角色名时直接查询，否则先 `dws contact label list --format json` 获取全部角色列表后匹配

### 8. 表单联系人（target_formcomponent_approval）

从表单中的联系人控件读取审批人。

```json
{
  "type": "target_formcomponent_approval",
  "paramKey": "InnerContactField_XXX",
  "label": "项目负责人",
  "isEmpty": false
}
```

- `paramKey` 指向表单中的 `InnerContactField` 控件 ID
- 该控件中填写的人即为审批人

### 9. 角色标签（target_label）

按角色标签找人（如"财务"、"HR"）。

```json
{
  "type": "target_label",
  "labelNames": "财务",
  "labels": "459272424",
  "isEmpty": false
}
```

- `labels`：角色标签 ID（字符串），必须通过 `dws contact label get --names "<角色名>" --format json` 获取；未知角色名时先 `dws contact label list --format json`
- `labelNames`：角色显示名称
- **严禁编造 label ID**

### 10. 审批矩阵（target_matrix_approval）

按审批矩阵规则确定审批人。

```json
{
  "type": "target_matrix_approval",
  "matrixId": "xxx",
  "roleColumnId": "yyy",
  "expression": {
    "subFilters": [...],
    "operator": "AND"
  }
}
```

- 仅适用于审批人节点
- 目前尚在完善中

---

## 条件分支详解

### 条件类型

| `type` | 依据 | 关键字段 |
|--------|------|---------|
| `dingtalk_actioner_dept_condition` | 发起人部门/人员/角色 | `paramKey: "dingtalk_origin_dept"`, `conds[]` |
| `dingtalk_actioner_dept_component_condition` | 表单部门控件 | `paramKey: 控件ID`, `conds[]` |
| `dingtalk_actioner_range_condition` | 数值/金额/时长范围 | `lowerBound`(>=) / `lowerBoundNotEqual`(>) / `upperBoundEqual`(<=) / `upperBound`(<) / `boundEqual`(=) |
| `dingtalk_actioner_value_condition` | 单选匹配 | `paramKey: 控件ID`, `paramValues[]`（选项 key） |
| `dingtalk_multi_value_condition` | 多选匹配 | `paramKey: 控件ID`, `paramValues[]`, `matchType`(1=精确/2=全选/3=任一) |
| `dingtalk_actioner_cascade_component_condition` | 级联控件 | `paramValues[]`, `displayValues[]` |
| `dingtalk_actioner_boolean_condition` | 布尔值 | `boundEqual: true/false` |
| `dingtalk_rule_template` | 节假日判断 | `template`, `outVars` |
| `dingtalk_formula` | 公式 | `formula`, `formulaDisplay` |
| `dingtalk_biz_var_condition` | 业务变量 | `dsKey`, `conds[]` |
| `dingtalk_table_condition` | 明细内字段 | `parentFieldId`, `componentName`, `paramValue` |

### 范围条件操作符

| 字段 | 含义 |
|------|------|
| `lowerBound` | >= (大于等于) |
| `lowerBoundNotEqual` | > (大于) |
| `upperBoundEqual` | <= (小于等于) |
| `upperBound` | < (小于) |
| `boundEqual` | = (等于) |

### 默认分支

- `isdefault: true`
- `conditions: [[]]`（一个空的条件组）
- **必须放在 `conditionNodes[]` 的最后**

---

## create-instance 中的节点参数映射

### directAppointedApprovers（指定审批人覆盖模板流程）

当需要**不使用模板默认流程、直接指定审批人**时使用。

```json
{
  "directAppointedApprovers": [
    {
      "staffIds": ["userId1", "userId2"],
      "taskActionType": "NONE",
      "staffId": ""
    }
  ]
}
```

| 字段 | 说明 |
|------|------|
| `staffIds` | 审批人 userId 列表（通过 `dws aisearch person --keyword "<姓名>" --dimension name --format json` 获取；多结果须消歧） |
| `taskActionType` | `NONE`（单人）/ `AND`（会签）/ `OR`（或签） |
| `staffId` | 留空字符串 |

### targetSelectActioners（自选审批人）

当模板流程中存在**自选审批节点**（`target_select` 类型）时必填。

```json
{
  "targetSelectActioners": [
    {
      "actionerKey": "manual_nodeId_xxxx_yyyy",
      "actionerStaffIds": ["userId1"]
    }
  ]
}
```

| 字段 | 说明 |
|------|------|
| `actionerKey` | 自选节点的规则 key，从审批流程节点信息接口获取 `actorKey` |
| `actionerStaffIds` | 操作人 userId 列表 |

---

## 组装优先级

1. 先用 `forecast-process` 获取模板的流程节点结构（`workflowActivityRuleVOs`）
2. 根据节点中的 `activityType` 和 `targetSelect` 判断是否需要传入 `directAppointedApprovers` 或 `targetSelectActioners`
3. 如果预测返回 `targetSelect: true` 的自选节点，`targetSelectActioners` 必填
4. 如果用户要求覆盖默认流程，使用 `directAppointedApprovers`
5. **所有 userId 必须通过 `dws aisearch person --keyword "<姓名>" --dimension name --format json` 获取，严禁填姓名；多结果须消歧**

> **交互优化：** 若用户在 `forecast-process` 前已指定审批人/抄送人姓名，`forecast-process` 返回自选节点后应自动映射，仅对未覆盖的自选节点追问，不要重复询问。详见 [oa.md](../oa.md) 交互优化原则。
