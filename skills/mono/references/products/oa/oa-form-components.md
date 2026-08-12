# OA 审批表单控件参考

本文档详细描述钉钉 OA 审批中每种表单控件（componentName）在**发起审批实例**时 `formComponentValues` 的 `value` 格式、约束和注意事项。

> **核心原则：** `formComponentValues[].name` 必须与审批模板中控件的 `props.label` **完全一致**，`value` 为字符串类型（最大 65535 字符）。

---

## 通用约束

| 约束 | 说明 |
|------|------|
| 单表单最大控件数 | 200 |
| label / placeholder 最大长度 | 50 字符 |
| value 最大长度 | 65535 字符 |
| ID / bizAlias 唯一性 | 同一表单内不可重复 |
| TextNote | 不收集数据，不出现在 formComponentValues 中 |

---

## 基础控件

### TextField（单行输入框）

| 属性 | 说明 |
|------|------|
| `componentName` | `TextField` |
| value 格式 | 纯文本字符串 |
| 示例 | `"测试内容"` |
| 约束 | 无特殊约束 |

```json
{ "name": "单行输入框", "value": "测试内容" }
```

### TextareaField（多行输入框）

| 属性 | 说明 |
|------|------|
| `componentName` | `TextareaField` |
| value 格式 | 纯文本字符串，支持换行 |
| 示例 | `"第一行\n第二行"` |
| 约束 | 无 `ratio` 属性 |

```json
{ "name": "多行输入框", "value": "第一行\n第二行\n第三行" }
```

### NumberField（数字输入框）

| 属性 | 说明 |
|------|------|
| `componentName` | `NumberField` |
| value 格式 | 数字字符串 |
| 示例 | `"100"` |
| 约束 | 适合数量、天数等纯数字场景 |

```json
{ "name": "加班天数", "value": "3" }
```

### DDSelectField（单选框）

| 属性 | 说明 |
|------|------|
| `componentName` | `DDSelectField` |
| value 格式 | 选项文本字符串 |
| 示例 | `"同意"` |
| 约束 | **必须与模板 `options[].value` 完全匹配**，不可自行编造选项 |

模板中的选项结构（从 `form-schema` 获取）：
```json
"options": [
  { "key": "option_0", "value": "同意" },
  { "key": "option_1", "value": "不同意" }
]
```

提交时传选项的 `value` 文本：
```json
{ "name": "审批意见", "value": "同意" }
```

### DDMultiSelectField（多选框）

| 属性 | 说明 |
|------|------|
| `componentName` | `DDMultiSelectField` |
| value 格式 | JSON 数组字符串，每个元素为选项文本 |
| 示例 | `'["选项A","选项B"]'` |
| 约束 | 每个选项须与模板 `options[].value` 匹配；|

```json
{ "name": "兴趣爱好", "value": "[\"阅读\",\"运动\"]" }
```

### DDDateField（日期控件）

| 属性 | 说明 |
|------|------|
| `componentName` | `DDDateField` |
| value 格式 | `yyyy-MM-dd` 格式字符串 |
| 示例 | `"2026-07-27"` |
| 约束 | 格式固定，不可传其他日期格式 |

```json
{ "name": "请假日期", "value": "2026-07-27" }
```

### DDDateRangeField（时间区间控件）

| 属性 | 说明 |
|------|------|
| `componentName` | `DDDateRangeField` |
| value 格式 | JSON 数组字符串 `[开始日期, 结束日期]` |
| 示例 | `'["2026-07-27","2026-07-30"]'` |
| 约束 | `props.label` 为数组 `["开始时间","结束时间"]`；提交时 `name` 使用**开始时间的 label** |

模板中的 label 结构（从 `form-schema` 获取）：
```json
"props": { "label": ["开始时间", "结束时间"] }
```

提交时用**开始时间 label** 作为 name：
```json
{ "name": "开始时间", "value": "[\"2026-07-27\",\"2026-07-30\"]" }
```

### PhoneField（电话控件）

| 属性 | 说明 |
|------|------|
| `componentName` | `PhoneField` |
| value 格式 | 手机号字符串 |
| 示例 | `"13800138000"` |
| 约束 | `mode: "phone"` 为手机号 |

```json
{ "name": "联系电话", "value": "13800138000" }
```

### IdCardField（身份证控件）

| 属性 | 说明 |
|------|------|
| `componentName` | `IdCardField` |
| value 格式 | 身份证号字符串 |
| 示例 | `"330102199001011234"` |
| 约束 | 内置格式校验，须传合法身份证号 |

```json
{ "name": "身份证号", "value": "330102199001011234" }
```

### TextNote（文字说明）

| 属性 | 说明 |
|------|------|
| `componentName` | `TextNote` |
| value 格式 | — |
| 约束 | **不收集数据**，不出现在 formComponentValues 中 |

> 遇到 TextNote 控件时直接跳过，不要尝试为它填写值。

---

## 增强控件

### MoneyField（金额控件）

| 属性 | 说明 |
|------|------|
| `componentName` | `MoneyField` |
| value 格式 | 数字字符串 |
| 示例 | `"1500.50"` |
| 约束 | 系统自动显示大写金额（`notUpper: "0"` 时显示） |

```json
{ "name": "报销金额", "value": "1500.50" }
```

### InnerContactField（联系人控件）

| 属性 | 说明                                                 |
|------|----------------------------------------------------|
| `componentName` | `InnerContactField`                                |
| value 格式 | userId 字符串，多人时为 JSON 数组字符串                         |
| 示例（单选） | `"user123"`                                        |
| 示例（多选） | `'["userId1","userId2"]'`                              |
| 约束 | `choice: "0"` 单选 / `"1"` 多选；userId 须为**当前组织下在职成员** |

```json
{ "name": "项目负责人", "value": "[\"userId1\",\"userId2\"]" }
```

> **严禁直接写姓名。** 必须先通过 `dws aisearch person --keyword "<姓名>" --dimension name --format json` 查询获取 userId；多结果时须让用户消歧确认。

### DepartmentField（部门控件）

| 属性 | 说明 |
|------|------|
| `componentName` | `DepartmentField` |
| value 格式 | 部门 ID 字符串，多部门时为 JSON 数组字符串 |
| 示例（单选） | `"12345"` |
| 示例（多选） | `'["12345","67890"]'` |
| 约束 | `multiple: boolean` 控制单选/多选；部门 ID 须为**当前组织下存在的部门** |

```json
{ "name": "所属部门", "value": "12345" }
```

### AddressField（省市区控件）

| 属性 | 说明 |
|------|------|
| `componentName` | `AddressField` |
| value 格式 | JSON 数组字符串 `["省","市","区"]` |
| 示例 | `'["浙江省","杭州市","西湖区"]'` |
| 约束 | 三级联动选择器；`needDetail: true` 时末尾追加详细地址文本 |

```json
{ "name": "办公地点", "value": "[\"浙江省\",\"杭州市\",\"西湖区\"]" }
```

### DDPhotoField（图片控件）

> **支持通过图片 URL 提交，不支持本地文件上传。** 如果用户已有图片 URL（如公网可访问的图片链接），可直接填入 value 提交。CLI 尚未封装本地文件上传到钉盘 CDN 的流程，若用户只有本地文件而非 URL，需告知用户在钉钉客户端补充。

| 属性 | 说明 |
|------|------|
| `componentName` | `DDPhotoField` |
| value 格式 | URL 数组转义字符串，即使只有一个 URL 也需数组形式 |
| 示例 | `"[\"http://example.com/img1.jpg\",\"http://example.com/img2.jpg\"]"` |
| 约束 | 支持 URL 直接提交；**不支持本地文件上传**（CLI 未封装钉盘上传流程）； |

```json
{ "name": "图片", "value": "[\"http://example.com/photo.jpg\"]" }
```

### DDAttachment（附件控件）

> **[注意] 当前暂不支持通过 CLI 提交附件控件。** 附件控件的 value 需要包含 spaceId、fileName、fileSize、fileType 和 fileId 字段，这些字段需要通过调用钉盘的上传附件接口获取，CLI 尚未封装此流程。包含附件控件的审批模板请在钉钉客户端操作。

| 属性 | 说明 |
|------|------|
| `componentName` | `DDAttachment` |
| value 格式 | JSON 数组转义字符串，每个元素包含 spaceId、fileName、fileSize、fileType、fileId |
| 示例（参考） | `"[{\"spaceId\":\"163xxx\",\"fileName\":\"2644.JPG\",\"fileSize\":\"333\",\"fileType\":\"jpg\",\"fileId\":\"643xxx\"}]"` |
| 约束 | **当前不支持通过 CLI 提交**；各字段需通过钉盘上传附件接口获取 |

### StarRatingField（评分控件）

| 属性 | 说明 |
|------|------|
| `componentName` | `StarRatingField` |
| value 格式 | 数字字符串 |
| 示例 | `"4"` |
| 约束 | `limit` 控制最大星数（默认 5） |

```json
{ "name": "满意度评分", "value": "4" }
```

### RelateField（关联审批单）

| 属性 | 说明 |
|------|------|
| `componentName` | `RelateField` |
| value 格式 | 审批实例 ID 字符串 |
| 示例 | `"q-ZZ1sQaTIuYFpKI9aNC1g"` |
| 约束 | 须为**当前组织下已存在的审批实例 ID** |

```json
{ "name": "关联审批单", "value": "q-ZZ1sQaTIuYFpKI9aNC1g" }
```

### SignatureField（签名控件）

| 属性 | 说明 |
|------|------|
| `componentName` | `SignatureField` |
| value 格式 | 签名图片 mediaId |
| 约束 | 需要客户端交互签名，通常不支持 API 直接提交 |

---

## 复合控件

### TableField（明细控件）

| 属性 | 说明 |
|------|------|
| `componentName` | `TableField` |
| value 格式 | JSON 数组字符串，每个元素为一行数据的键值对 |
| 示例 | `'[{"商品名":"笔记本","数量":"2"},{"商品名":"钢笔","数量":"1"}]'` |
| 约束 | **不可嵌套 TableField**；**不可包含 DDMultiSelectField 和 DDPhotoField**；最大 100 行；总长度不超过 65535 字符 |

模板结构（从 `form-schema` 获取）：
```json
{
  "componentName": "TableField",
  "props": { "label": "采购明细" },
  "children": [
    { "componentName": "TextField", "props": { "label": "商品名", "id": "TextField_XXX" } },
    { "componentName": "NumberField", "props": { "label": "数量", "id": "NumberField_YYY" } }
  ]
}
```

提交时每行用子控件 label 作 key：
```json
{
  "name": "采购明细",
  "value": "[{\"商品名\":\"笔记本\",\"数量\":\"2\"},{\"商品名\":\"钢笔\",\"数量\":\"1\"}]"
}
```

---

## API 不支持的控件

以下控件**不支持**通过创建实例 API 提交，遇到时应告知用户需在钉钉客户端补充：

| 控件 | componentName | 原因 |
|------|---------------|------|
| 文字说明 | `TextNote` | 纯展示，不收集数据 |
| 计算公式 | `CalculateField` | 由系统自动计算，不可手动填写 |
| 流水号 | `SeqNumberField` | 由系统自动生成 |
| OCR 文本识别 | `OcrTextField` | 需要客户端 OCR 交互 |
| OCR 身份证识别 | `OcrIdCardField` | 需要客户端 OCR 交互 |
| 附件控件 | `DDAttachment` | value 需要 spaceId、fileName、fileSize、fileType、fileId，须通过钉盘上传接口获取，CLI 尚未封装 |

> **部分支持的控件：** `DDPhotoField`（图片控件）**支持通过 URL 直接提交**，但不支持本地文件上传（CLI 未封装钉盘 CDN 上传流程）。若用户只有本地文件，需告知在钉钉客户端补充。详见本文 [DDPhotoField](#ddphotofield图片控件) 章节。

> **套件类控件（暂不支持）** — `InvoiceField`（发票）、`RecipientAccountField`（收款账户）等业务套件控件当前暂不支持通过 CLI 发起，包含这些控件的审批模板请直接在钉钉客户端操作。

---

## 组装优先级

1. **每次发起前都重新调用 `form-schema`**，不得复用旧结果（模板可能已被修改）
2. 先读 `form-schema` 返回的 `content`，识别所有控件的 `label`、`componentName`、`options`、`props.required`
3. **检查是否存在不支持控件且为必填项（`props.required: true`）**，若有则直接告知用户该模板不支持通过 CLI 发起，请在钉钉客户端操作
4. 按本文档中每种控件的 value 格式组装 `formComponentValues`
5. **不要把 `form-schema` 的 `content` 当成可直接提交的模板**
6. 遇到 API 不支持的控件（非必填），跳过并告知用户
