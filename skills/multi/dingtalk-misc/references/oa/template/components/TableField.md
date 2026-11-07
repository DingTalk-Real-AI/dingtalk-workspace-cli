# 明细/表格 — `TableField`

> 分类：增强控件

可添加多行的子表单，通过 children 定义列。

## JSON Schema

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `componentName` | string | 是 | 固定值 `"TableField"` |
| `props.id` | string | 是 | 唯一标识，格式 `TableField_{随机字符串}` |
| `props.label` | string | 是 | 控件标题 |
| `props.actionName` | string | 否 | 添加按钮文案，如 `"添加"` |
| `props.tableViewMode` | string | 否 | 视图模式，`"table"` 表格模式 |
| `children` | array | 是 | 子控件列表，定义表格的列 |

## 限制

- children 中**不可嵌套** TableField
- children 中**不可使用** DDMultiSelectField 和 DDPhotoField

## 示例

```json
{
  "componentName": "TableField",
  "props": {
    "id": "TableField_2QMQ62QD28Q0",
    "label": "表格",
    "actionName": "添加",
    "tableViewMode": "table"
  },
  "children": [
    {
      "componentName": "TextField",
      "props": {
        "id": "TextField_1F68U92WU5Y80",
        "label": "单行输入框",
        "placeholder": "请输入",
        "required": false,
        "ratio": 50
      }
    }
  ]
}
```
