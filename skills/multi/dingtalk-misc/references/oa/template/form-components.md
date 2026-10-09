# 钉钉 OA 审批表单控件描述文件 v4（JSON Schema + 示例版）

> **适用场景**：本文档用于构造 `dws oa approval template create/update` 命令的 `--schema-content` 参数（对应服务端 `schemaContent` 字段）。`--schema-content` 的值是一个 JSON 对象，包含顶层 `title` 和 `items[]` 控件数组。
>
> **传参方式**：`--schema-content` 支持直接 JSON 对象、`@文件路径` 或 `-`（stdin），CLI 会自动转为 JSON 字符串发给服务端。
>
> **控件 ID**：每个控件的 `props.id` 必须在同一表单内唯一，可用 `python3 scripts/generate_form_component_id.py <ComponentType>` 生成。
>
> **更新模板**：更新前必须先 `dws oa approval template detail --process-code <code>` 读取现有 `schemaContent`，在其基础上修改并保留已有控件 `props.id`。
>
> 命令用法详见 [oa.md「创建和更新审批模板」章节](../../oa.md)。
>
> 每个控件的详细 JSON Schema 和示例请参阅 `components/` 目录下对应文件。

---

## 概览：什么是 OA 审批表单与表单控件

在钉钉 OA 审批中，**表单**是审批流程的数据载体，由标题、描述和一系列**表单控件**组成。每个控件负责采集一类特定数据，在 JSON 中以 `componentName`（类型标识）和 `props`（配置属性）两个字段定义。钉钉 OA 审批设计器提供 38 种控件，分为五大类：布局控件（1 种）、基础控件（10 种）、增强控件（17 种）、业务控件（6 种）、高级控件（3 种）。本文档面向需要自动化生成审批表单的 Agent，提供每种控件的 JSON Schema、可执行示例和标准化生成流程。

### 控件总览

| 分类 | 控件名称 | componentName | 详情 |
|------|---------|---------------|------|
| 布局 | 分栏 | `ColumnLayout` | [查看](components/ColumnLayout.md) |
| 基础 | 单行输入框 | `TextField` | [查看](components/TextField.md) |
| 基础 | 多行输入框 | `TextareaField` | [查看](components/TextareaField.md) |
| 基础 | 数字输入框 | `NumberField` | [查看](components/NumberField.md) |
| 基础 | 单选框 | `DDSelectField` | [查看](components/DDSelectField.md) |
| 基础 | 多选框 | `DDMultiSelectField` | [查看](components/DDMultiSelectField.md) |
| 基础 | 日期 | `DDDateField` | [查看](components/DDDateField.md) |
| 基础 | 日期区间 | `DDDateRangeField` | [查看](components/DDDateRangeField.md) |
| 基础 | 说明文字 | `TextNote` | [查看](components/TextNote.md) |
| 基础 | 身份证 | `IdCardField` | [查看](components/IdCardField.md) |
| 基础 | 电话 | `PhoneField` | [查看](components/PhoneField.md) |
| 增强 | 级联/分类 | `CascadeField` | [查看](components/CascadeField.md) |
| 增强 | AI 控件 | ⚠️ | 暂不支持自动化生成 |
| 增强 | 图片 | `DDPhotoField` | [查看](components/DDPhotoField.md) |
| 增强 | 明细/表格 | `TableField` | [查看](components/TableField.md) |
| 增强 | 金额 | `MoneyField` | [查看](components/MoneyField.md) |
| 增强 | 附件 | `DDAttachment` | [查看](components/DDAttachment.md) |
| 增强 | 手写签名 | `SignatureField` | [查看](components/SignatureField.md) |
| 增强 | 钉钉文档 | ⚠️ | 暂不支持自动化生成 |
| 增强 | 外部联系人 | `ExternalContactField` | [查看](components/ExternalContactField.md) |
| 增强 | 联系人 | `InnerContactField` | [查看](components/InnerContactField.md) |
| 增强 | 部门 | `DepartmentField` | [查看](components/DepartmentField.md) |
| 增强 | 行业通讯录部门 | ⚠️ | 暂不支持自动化生成 |
| 增强 | 地点 | `TimeAndLocationField` | [查看](components/TimeAndLocationField.md) |
| 增强 | 计算公式 | `CalculateField` | [查看](components/CalculateField.md) |
| 增强 | 关联审批单 | `RelateField` | [查看](components/RelateField.md) |
| 增强 | 省市区 | `AddressField` | [查看](components/AddressField.md) |
| 增强 | 评分 | `StarRatingField` | [查看](components/StarRatingField.md) |
| 业务 | 发票 | `InvoiceField` | [查看](components/InvoiceField.md) |
| 业务 | 客户 | ⚠️ | 暂不支持自动化生成 |
| 业务 | 收款账户 | `RecipientAccountField` | [查看](components/RecipientAccountField.md) |
| 业务 | 预算申请 | ⚠️ | 暂不支持自动化生成 |
| 业务 | 关联合同 | ⚠️ | 暂不支持自动化生成 |
| 业务 | 工程项目 | ⚠️ | 暂不支持自动化生成 |
| 高级 | 通用文字识别 | `OcrTextField` | [查看](components/OcrTextField.md) |
| 高级 | 身份证识别 | `OcrIdCardField` | [查看](components/OcrIdCardField.md) |
| 高级 | 流水号 | `SeqNumberField` | [查看](components/SeqNumberField.md) |
| 关联 | 关联表单 | `FormRelateField` | [查看](components/FormRelateField.md) |

> 标注 ⚠️ 的 7 种控件暂不支持自动化生成。已确认的 31 种控件均经过 JSON 配置和 GUI 界面双重验证。

---

## 表单顶层结构

```json
{
    "title": "表单名称",
    "description": "表单描述",
    "icon": "common",
    "commentHiddenForProposer": "0",
    "commentRequired": "0",
    "commentDescription": "",
    "commentHiddenModal": "",
    "items": [
      {
        "componentName": "控件类型标识",
        "props": {
          "id": "唯一标识符（格式 {componentName}_{随机字符串}）",
          "label": "控件标题",
          "placeholder": "提示文字",
          "required": false,
          "ratio": 50
        }
      }
    ]
}
```

### 顶层字段说明

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `title` | string | 是 | 表单名称，最大 50 字符 |
| `description` | string | 否 | 表单描述，用于告知填写者注意事项 |
| `icon` | string | 否 | 表单图标标识，默认 `"common"`，支持其他值如 `"schoolLog#blue"` |
| `commentHiddenForProposer` | string | 否 | 是否对提交人隐藏评论，`"0"`或者 `""` 不隐藏，`"1"` 隐藏，默认 `""` |
| `commentRequired` | string | 否 | 评论是否必填，`"0"` 非必填，`"1"` 必填，默认 `""` |
| `commentDescription` | string | 否 | 评论区提示文字 |
| `commentHiddenModal` | string | 否 | 评论隐藏弹窗配置 |
| `items` | array | 是 | 表单控件列表，按顺序排列 |

### 控件通用字段说明

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `componentName` | string | 是 | 控件类型标识 |
| `props.id` | string | 是 | 唯一标识，格式 `{componentName}_{随机字符串}`，通过 [`generate_form_component_id.py`](../../../scripts/generate_form_component_id.py) 脚本生成 |
| `props.label` | string/array | 是 | 控件标题，日期区间/地点为 `["开始","结束"]` |
| `props.placeholder` | string | 否 | 占位提示文字 |
| `props.required` | boolean | 否 | 是否必填，默认 false |
| `props.ratio` | number | 否 | 宽度比例，如 50 |
| `props.bizAlias` | string | 否 | 业务别名。**仅套件子控件**（如 `DDBizSuite` 子控件、`OcrIdCardField` 的 `children`）才添加；普通 / 独立控件不要添加。若设置需同一表单内不可重复 |

---

## Agent 生成流程

1. 解析用户自然语言需求 → 提取表单名称、控件类型及配置
2. 为每个控件生成唯一 ID：运行 `python3 scripts/generate_form_component_id.py <ComponentType>`（如 `python3 scripts/generate_form_component_id.py TextField` → `TextField_19B75TV9C7310`）
3. 按描述顺序组装 items 数组
4. TableField / OcrIdCardField 需构造 children 数组
5. 组装顶层结构（title、description、items）
6. 输出完整 JSON

### 约束速查

| 约束项 | 限制值 |
|--------|--------|
| 表单最大控件数 | 200 |
| 标题/提示文字最大长度 | 50 字符 |
| 明细总长度上限 | 65535 字符（≤100 行） |
| 明细嵌套限制 | 不可嵌套 TableField |
| 明细禁用控件 | DDMultiSelectField、DDPhotoField |
| 企业模板上限 | 200 |
| ID/bizAlias 唯一性 | 同一表单内不可重复 |

---

## 完整示例：报销申请

```json
{
  "title": "报销申请",
  "description": "请如实填写报销信息，财务部将在 3 个工作日内审核",
  "icon": "common",
  "commentHiddenForProposer": "0",
  "commentRequired": "0",
  "commentDescription": "",
  "commentHiddenModal": "",
  "items": [
    {
      "componentName": "TextNote",
      "props": {
        "id": "TextNote_NOTE001",
        "content": "请如实填写报销信息，单次报销金额上限 5000 元",
        "notPrint": "0"
      }
    },
    {
      "componentName": "DDDateField",
      "props": {
        "id": "DDDateField_DATE001",
        "label": "报销日期",
        "placeholder": "请选择",
        "required": true,
        "format": "yyyy-MM-dd",
        "unit": "天"
      }
    },
    {
      "componentName": "TableField",
      "props": {
        "id": "TableField_TBL001",
        "label": "费用明细",
        "actionName": "添加费用",
        "tableViewMode": "table"
      },
      "children": [
        {
          "componentName": "DDDateField",
          "props": {
            "id": "DDDateField_DTL001",
            "label": "费用日期",
            "placeholder": "请选择",
            "required": true,
            "format": "yyyy-MM-dd",
            "unit": "天"
          }
        },
        {
          "componentName": "DDSelectField",
          "props": {
            "id": "DDSelectField_DTL002",
            "label": "费用类型",
            "placeholder": "请选择",
            "required": true,
            "options": [
              { "key": "opt_1", "value": "交通费" },
              { "key": "opt_2", "value": "餐饮费" },
              { "key": "opt_3", "value": "住宿费" },
              { "key": "opt_4", "value": "办公用品" },
              { "key": "opt_5", "value": "其他" }
            ]
          }
        },
        {
          "componentName": "MoneyField",
          "props": {
            "id": "MoneyField_DTL003",
            "label": "金额（元）",
            "placeholder": "请输入金额",
            "required": true,
            "notUpper": "0"
          }
        },
        {
          "componentName": "TextField",
          "props": {
            "id": "TextField_DTL004",
            "label": "备注",
            "placeholder": "选填",
            "required": false
          }
        }
      ]
    },
    {
      "componentName": "CalculateField",
      "props": {
        "id": "CalculateField_CALC001",
        "label": "报销总额",
        "placeholder": "自动汇总费用明细",
        "notUpper": "0"
      }
    },
    {
      "componentName": "DDAttachment",
      "props": {
        "id": "DDAttachment_ATT001",
        "label": "费用凭证",
        "required": true
      }
    },
    {
      "componentName": "InvoiceField",
      "props": {
        "id": "InvoiceField_INV001",
        "label": "发票信息",
        "required": false,
        "bizAlias": "",
        "appId": "78641",
        "invoiceChecking": "true",
        "commonSetterConfig": {
          "paymentBeforeInvoice": ["NO_INVOICE", "TODO_COLLECT"]
        }
      }
    },
    {
      "componentName": "RecipientAccountField",
      "props": {
        "id": "RecipientAccountField_ACCT001",
        "label": "收款账户",
        "placeholder": "请选择收款账户",
        "required": true
      }
    },
    {
      "componentName": "TextareaField",
      "props": {
        "id": "TextareaField_REM001",
        "label": "报销说明",
        "placeholder": "如有特殊情况请在此说明",
        "required": false
      }
    }
  ]
}
```
