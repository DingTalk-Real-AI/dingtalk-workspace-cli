# 钉钉 OA 审批流程节点描述文件（JSON Schema + 示例版）

> **适用场景**：本文档用于构造 `dws oa approval template create/update` 命令的 `--process-config` 参数（对应服务端 `processConfig` 字段）。`--process-config` 的值是一个 JSON 对象，以发起人节点为根、通过 `childNode` 逐层嵌套后续节点的树结构。
>
> **传参方式**：`--process-config` 支持直接 JSON 对象、`@文件路径` 或 `-`（stdin），CLI 会自动转为 JSON 字符串发给服务端。
>
> **节点 ID**：每个节点必须有唯一 `nodeId`，发起节点固定为 `sid-startevent`，其余可用 `python3 scripts/generate_node_id.py` 生成。
>
> **更新模板**：更新前必须先 `dws oa approval template detail --process-code <code>` 读取现有 `processConfig`，在其基础上修改并保留已有节点 `nodeId`。
>
> 命令用法详见 [oa.md「创建和更新审批模板」章节](../../oa.md)。
>
> 每个节点的详细 JSON Schema 和示例请参阅 `nodes/` 目录下对应文件。

---

## 概览：什么是审批流程与流程节点

在钉钉 OA 审批中，**审批流程**定义了审批单从发起到结束的完整流转路径。流程采用**嵌套树结构**，由根节点（发起人）通过 `childNode` 逐层链接后续节点。当前支持的节点分为四大类：发起节点（1 种）、人工节点（3 种）、分支节点（2 种）、套件节点（1 种）。

### 节点总览

| 序号 | 节点名称 | 类型标识 (`type`) | 分类 | 说明 | 详情 |
|------|---------|-------------------|------|------|------|
| 1 | 发起人 | `start` | 发起节点 | 流程起点，不可删除 | [查看](nodes/originator.md) |
| 2 | 审批人 | `approver` | 人工节点 | 核心决策节点 | [查看](nodes/approver.md) |
| 3 | 办理人 | `handler` | 人工节点 | 执行具体工作后返回流程 | [查看](nodes/handler.md) |
| 4 | 抄送人 | `notifier` | 人工节点 | 仅接收通知 | [查看](nodes/cc.md) |
| 5 | 条件分支 | `route` + `condition` | 分支节点 | `route` 承载 `conditionNodes[]`，按条件分叉流程 | [查看](nodes/condition.md) |
| 6 | 并行分支 | `parallel` | 分支节点 | 并行执行多条分支 | [查看](nodes/parallel.md) |
| 7 | 付款人 | `payer` | 套件节点 | 财务套件节点 | [查看](nodes/payer.md) |

---

## 流程 JSON 整体结构

流程采用嵌套树结构，根节点为发起人（`type: "start"`），后续节点通过 `childNode` 逐层嵌套：

```json
{
  "name": "发起人",
  "type": "start",
  "nodeId": "sid-startevent",
  "properties": {},
  "childNode": {
    "isDefaultName": false,
    "name": "部门主管审批",
    "prevId": "sid-startevent",
    "type": "approver",
    "nodeId": "1a2b_3c4d",
    "properties": {
      "actionerRules": [
        {
          "level": 1,
          "autoUp": true,
          "isEmpty": false,
          "actType": "",
          "type": "target_management"
        }
      ],
      "noneActionerAction": "admin",
      "activateType": "ONE_BY_ONE",
      "approvalType": "MANUAL",
      "agreeAll": false
    }
  }
}
```

### 节点通用字段

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `name` | string | 是 | 节点显示名称 |
| `type` | string | 是 | 节点类型标识 |
| `nodeId` | string | 是 | 节点唯一标识。发起节点固定为 `sid-startevent`，其他节点通过 [`generate_node_id.py`](../../../../scripts/generate_node_id.py) 脚本生成，格式为 `{4位hex}_{4位hex}` |
| `prevId` | string | 是 | 上一个节点的 `nodeId`（发起节点无此字段） |
| `isDefaultName` | boolean | 否 | 是否为系统默认名称 |
| `properties` | object | 是 | 节点配置属性（各类型不同） |
| `childNode` | object | 否 | 下一个节点。**无后续节点时直接省略该字段**，禁止输出 `"childNode": null` |

> ⚠️ **`childNode` 硬性规则**：节点有后续时 `childNode` 为对象；**没有后续时必须省略 `childNode` 字段**，不得写成 `null`。同样适用于条件分支中的 `conditionNodes[].childNode` 与 `route.childNode`（无子节点时省略）。

---

## 选人规则（`actionerRules`）

审批人（`approver`）、办理人（`handler`）和抄送人（`notifier`）节点均通过 `properties.actionerRules` 数组配置“选人规则”。每条规则通过 `type` 字段区分类型。

每种规则的详细说明和示例请参阅 `rules/` 目录下对应文件。

| 序号 | 规则名称 | 类型标识 (`type`) | 适用节点 | 说明 | 详情 |
|------|---------|-------------------|---------|------|------|
| 1 | 指定成员 | `target_approval` | 全部 | 固定指定具体人员 | [查看](rules/target_approval.md) |
| 2 | 直属主管 | `target_formula` (reportLineManager) | 全部 | 从汇报线获取主管 | [查看](rules/target_formula_reportLineManager.md) |
| 3 | 发起人自己 | `target_originator` | 审批/办理 | 发起人本人 | [查看](rules/target_originator.md) |
| 4 | 部门主管 | `target_management` | 全部 | 发起人所在部门的主管 | [查看](rules/target_management.md) |
| 5 | 表单部门主管 | `target_formula` (managerOfDept) | 审批/办理 | 表单中选择的部门的主管 | [查看](rules/target_formula_managerOfDept.md) |
| 6 | 发起人自选 | `target_select` | 全部 | 提交时自行选择人员 | [查看](rules/target_select.md) |
| 7 | 角色标签主管 | `target_managers_labels` | 审批/办理 | 拥有指定角色标签的主管 | [查看](rules/target_managers_labels.md) |
| 8 | 表单内联系人 | `target_formcomponent_approval` | 审批/办理 | 从表单联系人控件获取 | [查看](rules/target_formcomponent_approval.md) |
| 9 | 角色标签 | `target_label` | 全部 | 拥有指定角色标签的人员 | [查看](rules/target_label.md) |
| 10 | 审批矩阵 | `target_matrix_approval` | 审批 | 从审批矩阵获取 | [查看](rules/target_matrix_approval.md) |
| 11 | 从连接器获取 | `target_connect_approval` | 审批/办理 | 通过连接器动态获取 | [查看](rules/target_connect_approval.md) |
---

## Agent 生成流程

1. 解析用户自然语言需求 → 提取流程结构（节点类型、审批人来源、审批方式、分支条件等）
2. 为每个节点生成唯一 `nodeId`：发起节点固定 `sid-startevent`，其余运行 `python3 scripts/generate_node_id.py`（如 `a44f_71c6`）
3. 匹配审批人来源（「我的主管 / 主管审批」默认 → `target_management`（部门主管），「直属主管 / 直属领导」 → `target_formula` + `reportLineManager`，「财务审批」 → `target_label`）
4. 匹配审批方式（「依次」→ `ONE_BY_ONE` + `agreeAll: true`，「会签」→ `ALL` + `agreeAll: true`，「或签」→ `ALL` + `agreeAll: false`）
5. 处理条件/并行分支（构建 `route`（条件分支）或 `parallel` 节点及其子节点）
6. 组装嵌套树结构（发起节点为根，审批节点至少一个，抄送置于末尾）
7. 输出完整 JSON

### 约束速查

| 约束项 | 限制值 |
|--------|--------|
| 每个流程至少一个审批节点 | 是 |
| 发起节点不可删除 | 是 |
| 单节点审批人最多 | 100 个 |
| 条件分支控件锁定 | 已作为分支条件的控件不可修改或删除 |
| 节点 ID 唯一性 | 同一流程内不可重复 |
| 企业模板上限 | 200 |

---

## 完整示例：报销审批（含多级审批 + 抄送）

```json
{
  "name": "发起人",
  "type": "start",
  "nodeId": "sid-startevent",
  "properties": {},
  "childNode": {
    "isDefaultName": false,
    "name": "部门主管审批",
    "prevId": "sid-startevent",
    "type": "approver",
    "nodeId": "a1b2_c3d4",
    "properties": {
      "actionerRules": [
        {
          "level": 1,
          "autoUp": true,
          "isEmpty": false,
          "actType": "",
          "type": "target_management"
        }
      ],
      "noneActionerAction": "admin",
      "activateType": "ONE_BY_ONE",
      "approvalType": "MANUAL",
      "agreeAll": false
    },
    "childNode": {
      "isDefaultName": false,
      "name": "财务审批",
      "prevId": "a1b2_c3d4",
      "type": "approver",
      "nodeId": "e5f6_7890",
      "properties": {
        "actionerRules": [
          {
            "labelNames": "财务",
            "isEmpty": false,
            "actType": "and",
            "type": "target_label",
            "labels": "TODO_LABEL_ID_3"
          }
        ],
        "noneActionerAction": "admin",
        "activateType": "ALL",
        "approvalType": "MANUAL",
        "agreeAll": true
      },
      "childNode": {
        "isDefaultName": false,
        "name": "抄送部门负责人",
        "prevId": "e5f6_7890",
        "type": "notifier",
        "nodeId": "ab12_cd34",
        "properties": {
          "actionerRules": [
            {
              "type": "target_management",
              "level": 1
            }
          ]
        }
      }
    }
  }
}
```
