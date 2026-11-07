# 选人规则：审批矩阵 — `target_matrix_approval`

> 适用节点：审批人（`approver`）

TODO 该规则目前还不可用，需要支持审批矩阵的 DWS 命令。
从审批矩阵获取审批人，根据表单字段值匹配矩阵行列确定审批人。

## 字段说明

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `type` | string | 是 | 固定值 `"target_matrix_approval"` |
| `matrixId` | number | 是 | 矩阵 ID |
| `roleColumnId` | number | 是 | 角色列 ID（矩阵中标识审批人的列） |
| `expression` | object | 是 | 匹配表达式 |
| `expression.operator` | string | 是 | 顶层逻辑运算符，如 `"OR"` |
| `expression.subFilters` | array | 是 | 子过滤条件列表 |
| `expression.subFilters[].operator` | string | 是 | 子条件内逻辑运算符，如 `"AND"` |
| `expression.subFilters[].conds` | array | 是 | 具体条件列表 |
| `expression.subFilters[].conds[].op` | string | 是 | 比较运算符，如 `"IN"` |
| `expression.subFilters[].conds[].columnId` | number | 是 | 矩阵列 ID |
| `expression.subFilters[].conds[].id` | string | 是 | 表单控件 ID（条件值来源） |
| `expression.subFilters[].conds[].type` | string | 是 | 条件类型，如 `"form"` |
| `actType` | string | 否 | `""` |

## 使用场景

- 用户说"从审批矩阵获取"、"按审批矩阵走"
- 需要根据多维度（如部门 + 金额）动态确定审批人

## 示例

```json
{
  "expression": {
    "subFilters": [
      {
        "conds": [
          {
            "op": "IN",
            "columnId": 167001,
            "id": "DepartmentField_EA5DNJML9JS0",
            "type": "form"
          }
        ],
        "operator": "AND"
      }
    ],
    "operator": "OR"
  },
  "roleColumnId": 167002,
  "matrixId": 114001,
  "actType": "",
  "type": "target_matrix_approval"
}
```

## 注意事项

- `matrixId` 为审批矩阵的 ID。若用户未提供真实 ID，在 JSON 中使用合法字符串占位（如 `"TODO_MATRIX_ID"`）并在待用户填写清单中说明；不要在 JSON 代码块中写 `//` 注释。
- `roleColumnId` 和 `columnId` 为矩阵中的列 ID，同样需用户提供
- `expression.subFilters[].conds[].id` 引用表单中已存在的控件 ID
- 审批矩阵功能较复杂，建议确认用户是否已在钉钉管理后台配置了矩阵
- 通常配合 `noneActionerAction: "admin"` 使用
