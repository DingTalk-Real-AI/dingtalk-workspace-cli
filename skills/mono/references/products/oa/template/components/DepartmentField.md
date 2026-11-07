# 部门 — `DepartmentField`

> 分类：增强控件

选择企业部门。

## JSON Schema

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `componentName` | string | 是 | 固定值 `"DepartmentField"` |
| `props.id` | string | 是 | 唯一标识，格式 `DepartmentField_{随机字符串}` |
| `props.label` | string | 是 | 控件标题 |
| `props.placeholder` | string | 否 | 占位提示文字 |
| `props.required` | boolean | 否 | 是否必填，默认 false |
| `props.multiple` | boolean | 否 | 是否多选，默认 false |

## 示例

```json
{
  "componentName": "DepartmentField",
  "props": {
    "id": "DepartmentField_13NBCWQ1OD8G0",
    "label": "部门",
    "placeholder": "请选择",
    "required": false,
    "multiple": false
  }
}
```
