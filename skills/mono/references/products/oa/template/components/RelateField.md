# 关联审批单 — `RelateField`

> 分类：增强控件

关联其他已提交的审批单。

## JSON Schema

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `componentName` | string | 是 | 固定值 `"RelateField"` |
| `props.id` | string | 是 | 唯一标识，格式 `RelateField_{随机字符串}` |
| `props.label` | string | 是 | 控件标题 |
| `props.placeholder` | string | 否 | 占位提示文字 |
| `props.required` | boolean | 否 | 是否必填，默认 false |
| `props.notPrint` | string | 否 | 是否不打印，`"1"` 不打印 |

## 示例

```json
{
  "componentName": "RelateField",
  "props": {
    "id": "RelateField_SD87LE56RVK0",
    "label": "关联审批单",
    "placeholder": "请选择",
    "required": false,
    "notPrint": "1"
  }
}
```
