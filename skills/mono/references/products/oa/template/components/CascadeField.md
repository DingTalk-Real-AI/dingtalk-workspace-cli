# 级联/分类 — `CascadeField`

> 分类：增强控件

多层级联动选择，适用于产品分类、地区分类等。

## JSON Schema

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `componentName` | string | 是 | 固定值 `"CascadeField"` |
| `props.id` | string | 是 | 唯一标识，格式 `CascadeField_{随机字符串}` |
| `props.label` | string | 是 | 控件标题 |
| `props.placeholder` | string | 否 | 占位提示文字 |
| `props.require` | boolean | 否 | 是否必填 |
| `props.bizAlias` | string | 否 | 业务别名 |
| `props.dataSource` | object | 是 | 数据源配置 |
| `props.dataSource.type` | string | 是 | 数据源类型，如 `"db_table"` |
| `props.dataSource.target` | object | 是 | 数据源目标 |

## 示例

```json
{
  "componentName": "CascadeField",
  "props": {
    "id": "CascadeField_1ZOPQ6ZRCPEO0",
    "label": "产品分类",
    "placeholder": "请输入",
    "require": true,
    "bizAlias": "",
    "dataSource": {
      "type": "db_table",
      "target": { "sourceForm": "oa", "appId": "-4", "scene": "CascadeField_2829052" }
    }
  }
}
```
