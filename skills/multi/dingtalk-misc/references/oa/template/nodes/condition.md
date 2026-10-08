# 条件分支 — `route` + `condition`

> 分类：分支节点

根据条件把流程分叉到不同分支。条件分支由**路由节点**（`type: "route"`）承载，其 `conditionNodes` 数组包含若干**条件节点**（`type: "condition"`）；每个条件节点定义一组判断条件，并通过自身的 `childNode` 串接该分支内的后续节点。

> ⚠️ 条件分支的容器是 `route` 节点，**不是** `condition`；`condition` 是 `conditionNodes` 数组内的分支节点。

## 整体结构

发起人（`start`）的 `childNode` 指向路由节点（`route`），路由节点的 `conditionNodes` 列出各分支：

```json
{
  "name": "发起人",
  "type": "start",
  "nodeId": "sid-startevent",
  "properties": {},
  "childNode": {
    "type": "route",
    "prevId": "sid-startevent",
    "nodeId": "8ed3_3d58",
    "conditionNodes": [ "...条件节点..." ],
    "properties": {}
  }
}
```

## 路由节点（`route`）

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `type` | string | 是 | 固定值 `"route"` |
| `nodeId` | string | 是 | 路由节点唯一标识，格式 `{4位hex}_{4位hex}` |
| `prevId` | string | 是 | 上一个节点的 `nodeId` |
| `conditionNodes` | array | 是 | 条件分支列表，**按优先级从高到低排列**（运行时按数组顺序依次匹配，命中即停）；**默认分支（`isdefault: true`）优先级最低，必须放在数组最后** |
| `properties` | object | 是 | 路由节点属性，固定为空对象 `{}` |
| `childNode` | object | 否 | 所有分支汇合后的下一个节点；无后续时省略（禁止 `null`） |

## 条件节点（`conditionNodes[]` 元素，`type: "condition"`）

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `name` | string | 是 | 分支名称（如“条件1”“默认条件”） |
| `type` | string | 是 | 固定值 `"condition"` |
| `nodeId` | string | 是 | 条件节点唯一标识，格式 `{4位hex}_{4位hex}` |
| `prevId` | string | 是 | 路由节点的 `nodeId` |
| `properties.conditions` | array | 是 | 条件组（二维数组），见下文；默认分支为 `[[]]` |
| `isdefault` | boolean | 否 | 默认分支为 `true`；普通分支不设置 |
| `childNode` | object | 否 | 该分支内的第一个节点；分支为空时省略（禁止 `null`） |

## `conditions` 条件结构

`conditions` 是**二维数组**：

- 外层数组：多个**条件组**，组与组之间为**或（OR）**关系。
- 内层数组：组内多个**条件对象**，条件之间为**且（AND）**关系。

条件对象字段：

| 字段 | 类型 | 说明 |
|------|------|------|
| `paramKey` | string | 参数键：发起人条件为固定值 `dingtalk_origin_dept`；表单控件条件为该控件的 `props.id` |
| `type` | string | 条件类型，决定判断依据与取值字段结构，见下文《条件类型与取值规则》 |
| `paramLabel` | string | 参数显示名（如 `发起人` 或控件 label） |
| `isEmpty` | boolean | 是否尚未配置值（未配置为 `true`） |
| （取值字段） | — | **随 `type` 不同而不同**：选人类用 `conds[]`，数值区间类用 `lowerBound`/`upperBound` 等，单/多选类用 `paramValues`，详见下文 |

> 选人类条件的 `conds[].value`（部门 / 人员 / 角色 ID）为环境数据，需通过钉钉通讯录 MCP 查询实际 ID，**不要编造**。

### 条件类型与取值规则

条件对象的 `type` 决定判断依据，并对应不同的取值字段。以下类型均来自条件设置器源码（`sw-flow-condition-setter`）。

#### 类型总览

| `type` | 判断依据 | 触发来源 | `paramKey` | 取值字段 |
|--------|---------|---------|-----------|---------|
| `dingtalk_actioner_dept_condition` | 发起人所属部门 / 人员 / 角色 | 发起人 | 固定 `dingtalk_origin_dept` | `conds[]`：`{ type: "dept"\|"user"\|"label", value, attrs }` |
| `dingtalk_actioner_dept_component_condition` | 表单部门控件的值 | `DepartmentField`（单选） | 控件 `props.id` | `conds[]`：`{ type: "dept", value, attrs }` |
| `dingtalk_actioner_range_condition` | 数值 / 时长区间 | `NumberField`、`MoneyField`、`CalculateField`、`DDDateRangeField`(时长)、明细求和、假期/加班等套件时长 | 控件 `props.id`（时长字段可能带 `__duration` 后缀） | 见下文「数值区间取值」 |
| `dingtalk_actioner_value_condition` | 单选值匹配 | `DDSelectField`、假期/出差类型 | 控件 `props.id`（假期带 `__options`） | `paramValues[]`（选项 key，旧数据用 `paramValue` 单值）、`oriValue`（全部选项） |
| `dingtalk_multi_value_condition` | 多选值匹配 | `DDMultiSelectField` | 控件 `props.id` | `paramValues[]` + `matchType`（`1`=完全等于、`2`=同时选中、`3`=选中任一）、`oriValue` |
| `dingtalk_actioner_cascade_component_condition` | 级联控件值 | `CascadeField` | 控件 `props.id` | `paramValues[]` + `displayValues[]`（+ `dataSource`） |
| `dingtalk_actioner_boolean_condition` | 布尔值 | 连接器布尔字段 | paramKey | `boundEqual`（`true`/`false`） |
| `dingtalk_rule_template` | 日期是否含法定节假日 | `DDDateField` / 时间区间 | paramKey | `template` + `outVars{ startTimeFieldId, endTimeFieldId, operator: ">"\|"="\|"<" }` |
| `dingtalk_formula` | 公式表达式 | 公式条件 | — | `formula` / `formulaDisplay` |
| `dingtalk_biz_var_condition` | 业务变量 | 连接器 / 办事节点关联数据源 | paramKey | `dsKey` + `conds[]`：`{ dataKey, opType, opValue, fieldType, fieldName }` |
| `dingtalk_table_condition` | 明细表内字段 | `TableField` 内 数字/单选/多选 字段 | 字段 id | `parentFieldId` + `componentName` + `paramValue{ opType, opValue[] }` |

> 选人类 ID（部门 / 人员 / 角色）与业务数据 ID 均需通过钉钉通讯录 MCP 或真实环境获取，**不要编造**。

#### 数值区间取值（`dingtalk_actioner_range_condition`）

数值条件**不使用运算符字符串**，而是通过上下界字段组合表达（可组合成区间）：

| 字段 | 含义 |
|------|------|
| `lowerBound` | ≥（大于等于） |
| `lowerBoundNotEqual` | ＞（大于） |
| `upperBoundEqual` | ≤（小于等于） |
| `upperBound` | ＜（小于） |
| `boundEqual` | ＝（等于） |
| `unit` | 单位（可选，如“天”“元”） |

示例：“金额 ≥ 5000” → `{ "type": "dingtalk_actioner_range_condition", "paramKey": "MoneyField_XXX", "lowerBound": 5000 }`；“5000 ≤ 金额 ＜ 10000” → 同时设置 `lowerBound: 5000` 与 `upperBound: 10000`。

#### 运算符（`opType`，用于业务变量 / 明细表条件）

`EQ`(等于) / `CONTAIN`(包含) / `IN`(属于) / `LT`(小于) / `LE`(小于等于) / `GT`(大于) / `GE`(大于等于) / `BETWEEN`(区间) / `LISTCONTAIN`(包含任一) 等；明细数字字段另有 `LISTCONTAINGE`/`GT`/`LE`/`LT`。

### 默认分支

默认分支优先级**最低**，**必须是 `conditionNodes` 数组的最后一个元素**（不能放在普通条件分支之前），所有前置条件都不满足时兜底：

- `isdefault: true`
- `properties.conditions` 为 `[[]]`（一个空条件组）
- 无分支节点时省略 `childNode`

## 完整示例（按发起人部门分流）

```json
{
  "name": "发起人",
  "type": "start",
  "nodeId": "sid-startevent",
  "properties": {},
  "childNode": {
    "type": "route",
    "prevId": "sid-startevent",
    "nodeId": "8ed3_3d58",
    "conditionNodes": [
      {
        "name": "条件1",
        "type": "condition",
        "prevId": "8ed3_3d58",
        "nodeId": "e27b_784a",
        "properties": {
          "conditions": [
            [
              {
                "paramKey": "dingtalk_origin_dept",
                "type": "dingtalk_actioner_dept_condition",
                "paramLabel": "发起人",
                "isEmpty": false,
                "conds": [
                  {
                    "type": "dept",
                    "value": "TODO_DEPT_ID_2",
                    "attrs": {
                      "name": "人事",
                      "memberCount": 3
                    }
                  }
                ]
              }
            ]
          ]
        },
        "childNode": {
          "isDefaultName": false,
          "name": "主管审批",
          "prevId": "e27b_784a",
          "type": "approver",
          "nodeId": "1918_5cd3",
          "properties": {
            "activateType": "ONE_BY_ONE",
            "approvalType": "MANUAL",
            "actionerRules": [
              {
                "isEmpty": false,
                "formula": "ReportLineManager(corpId,originator,1)",
                "subType": "reportLineManager",
                "type": "target_formula"
              }
            ],
            "agreeAll": false
          }
        }
      },
      {
        "name": "条件2",
        "type": "condition",
        "prevId": "8ed3_3d58",
        "nodeId": "13e5_6934",
        "properties": {
          "conditions": [
            [
              {
                "paramKey": "dingtalk_origin_dept",
                "type": "dingtalk_actioner_dept_condition",
                "paramLabel": "发起人",
                "isEmpty": false,
                "conds": [
                  {
                    "type": "dept",
                    "value": "TODO_DEPT_ID_3",
                    "attrs": {
                      "name": "财务",
                      "memberCount": 3
                    }
                  }
                ]
              }
            ]
          ]
        },
        "childNode": {
          "name": "办理人",
          "isDefaultName": true,
          "type": "audit",
          "prevId": "13e5_6934",
          "nodeId": "4be9_e43d",
          "properties": {
            "activateType": "ONE_BY_ONE",
            "agreeAll": false,
            "actionerRules": [
              {
                "type": "target_originator",
                "isEmpty": false
              }
            ]
          }
        }
      },
      {
        "name": "默认条件",
        "type": "condition",
        "prevId": "8ed3_3d58",
        "nodeId": "4539_cde5",
        "properties": {
          "conditions": [
            []
          ]
        },
        "isdefault": true
      }
    ],
    "properties": {}
  }
}
```

## 注意事项

- 条件分支容器是 `route` 节点，`condition` 是 `conditionNodes` 内的分支节点。
- `conditionNodes` 至少包含一个普通条件分支 + 一个默认分支（`isdefault: true`，`conditions: [[]]`）。
- 路由节点、各条件节点的 `nodeId` 需通过脚本生成，格式 `{4位hex}_{4位hex}`。
- `prevId` 链路：条件节点的 `prevId` 指向路由节点 `nodeId`；分支内首节点的 `prevId` 指向所属条件节点的 `nodeId`。
- `childNode` 无后续时一律省略，禁止 `null`。
- `conds[].value` 等环境 ID（部门、角色等）需通过钉钉通讯录 MCP 查询真实值，不要编造。
