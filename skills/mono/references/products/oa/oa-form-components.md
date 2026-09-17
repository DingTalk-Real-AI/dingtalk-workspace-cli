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

> **严禁直接写姓名。** 必须先通过 `dws aisearch person --query "<姓名>" --dimension name --format json` 查询获取 userId；多结果时须让用户消歧确认。

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

> **[支持] 已支持通过 CLI 提交附件控件。** 采用两步流程：先用 `dws oa approval attachment upload --file <path>` 上传本地文件，获取 spaceId、fileName、fileSize、fileType、fileId；再将这些字段组装为 DDAttachment value（JSON 数组转义字符串）随 `create-instance` 提交。

| 属性 | 说明 |
|------|------|
| `componentName` | `DDAttachment` |
| value 格式 | JSON 数组转义字符串，每个元素包含 spaceId、fileName、fileSize、fileType、fileId |
| 示例（参考） | `"[{\"spaceId\":\"163xxx\",\"fileName\":\"2644.JPG\",\"fileSize\":\"333\",\"fileType\":\"jpg\",\"fileId\":\"643xxx\"}]"` |
| 约束 | **支持通过 CLI 提交**；先用 `dws oa approval attachment upload --file <path>` 获取 spaceId、fileName、fileSize、fileType、fileId，再组装为 value 随 `create-instance` 提交 |

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

### DDHolidayField（请假套件）

> **支持通过 leave-duration / leave-check 命令链发起**，完整工作流见 [oa.md](../oa.md) 的「发起请假审批」章节。识别特征：`componentName == "DDHolidayField"`，`props.label` 为数组（如 `["开始时间","结束时间"]`），`props.options` 为假期类型选项列表。

模板 Schema 结构（从 `form-schema` 获取，关键字段）：
```json
{
  "componentName": "DDHolidayField",
  "props": {
    "label": ["开始时间", "结束时间"],
    "id": "DDHolidayField-J2BWEN12",
    "attendTypeLabel": "请假类型",
    "options": [
      { "unit": "hour", "name": "事假", "leaveCode": "885dc798-xxx", "displayUnit": "按小时请假" }
    ]
  }
}
```

提交条目结构（**必须走 `create-instance --request` 高级模式**，简单模式无法承载）：
```json
{
  "id": "DDHolidayField-J2BWEN12",
  "name": "[\"开始时间\",\"结束时间\"]",
  "value": "[\"2026-08-13 12:08\",\"2026-08-14 18:08\",14.87,\"hour\",\"事假\",\"请假类型\"]",
  "extValue": "{\"durationInHour\":14.87,...,\"key\":\"<leaveCode>\",\"leaveParams\":[\"<corpId>\",\"<leaveCode>\",\"<T1>\",\"<T2>\",null]}"
}
```

| 字段 | 取值规则 |
|------|---------|
| `id` | 套件 `props.id`（服务端按 id 精确匹配控件） |
| `name` | `JSON.stringify(套件 props.label 数组)`（如 `"[\"开始时间\",\"结束时间\"]"`；label 为数组时整体序列化，不取首元素；id 主通道时的回退匹配） |
| `value` | 六元数组 JSON 字符串：`[T1, T2, duration, unit, leaveName, attendTypeLabel]` |
| `extValue` | extendValue JSON 字符串（见下方组装规则，≤65535 字符） |

value 六元数组逐项规则：

| 索引 | 字段 | 取值 |
|---|---|---|
| [0] / [1] | 起止时间 | 用户输入，格式随 unit（见下表） |
| [2] | 时长（number，非字符串） | `unit ∈ {hour, halfHour, limitHour}` → `leave-duration 响应.durationInHour`，否则 → `durationInDay`，**必须服务端计算** |
| [3] | 单位 | 选定类型的 `leaveViewUnit` 原始值（vacation types 返回；hour / halfHour / limitHour / day / halfDay，小写；展示用中文映射不写入） |
| [4] | 类型名 | 选定类型的 `leaveName`（与 leaveCode 同取自 vacation types 同一条目，如「事假」） |
| [5] | 子类型标签 | 套件 `props.attendTypeLabel`，无则取 `props.push.pushTag + "类型"`，均无为 `""` |

时间格式（与 unit 硬绑定）：

| unit | T1/T2 格式 | 示例 |
|---|---|---|
| hour / halfHour / limitHour | yyyy-MM-dd HH:mm | 2026-08-13 12:08 |
| day | yyyy-MM-dd | 2026-08-13 |
| halfDay | yyyy-MM-dd 上午/下午 | 2026-08-13 上午 |

extValue 组装规则：

```
extValue = JSON.stringify({
  ...leave-duration 工具响应,   // durationInHour/Day, detailList, compressedValue, corpId, unit, pushTag... 原样合并，禁止裁剪
  key: leaveCode,               // = options[i].leaveCode
  leaveParams: [corpId, leaveCode, T1, T2, staffId]   // corpId 取工具响应回显；本人发起 staffId=null
})
```

> **IMPORTANT：**
> - 时长、detailList、compressedValue 一律取自 `dws attendance approve leave-duration` 的服务端计算结果，严禁本地构造或估算；不支持手动改时长（customDuration）。
> - 发起前必须先跑 `dws attendance approve leave-check` 提交前校验（--duration-day / --duration-hour 取自 leave-duration 输出）；success=false 时转告 errorMsg 并终止。
> - 哺乳假模板（类型条目 bizType === "breastfeeding_leave_new"；bizType 缺失时回退名称含「哺乳」）与需上传证明材料的请假类型（类型条目 leaveCertificate.enable=true 且时长双向换算小时后 ≥ 阈值，换算规则见 oa.md「发起请假审批」步骤 3）**不支持 CLI 发起**，引导用户在客户端提交。

### DDBizSuite · attendance.supply（补卡套件）

> **支持通过 supply-plans / supply-check 命令链发起**，完整工作流见 [oa.md](../oa.md) 的「发起补卡审批」章节。识别特征：`componentName == "DDBizSuite"` 且 `props.bizType == "attendance.supply"`；补卡时间子控件在 `children` 内（`DDDateField`，`bizAlias == "userCheckTime"`）。

模板 Schema 结构（从 `form-schema` 获取，需下钻 children）：
```json
{
  "componentName": "DDBizSuite",
  "props": { "bizType": "attendance.supply", "bizAlias": "supply", "id": "DDBizSuite-JYNRW2R9" },
  "children": [{
    "componentName": "DDDateField",
    "props": { "bizAlias": "userCheckTime", "format": "yyyy-MM-dd HH:mm", "id": "DDDateField-JYNRW51O", "label": "补卡时间", "required": true }
  }]
}
```

提交条目结构（**必须走 `create-instance --request` 高级模式**）——注意条目是**子控件**而非 DDBizSuite 容器：
```json
{
  "id": "DDDateField-JYNRW51O",
  "name": "补卡时间",
  "value": "2026-08-05 04:00",
  "extValue": "{\"timeStamp\":1785772800000,\"workDate\":1785772800000,\"planText\":\"2026-08-05 10:39\",\"userCheckTime\":1785873600000,\"planTip\":\"周二 ( 08.04 下班) 补卡\"}"
}
```

| 字段 | 取值规则 |
|------|---------|
| `id` | 子控件 `children[].props.id`（服务端按 id 精确匹配） |
| `name` | 子控件 `props.label`（如「补卡时间」；id 主通道时的回退匹配） |
| `value` | 按**子控件 props.format** 格式化最终补卡时刻毫秒值（默认 `yyyy-MM-dd HH:mm`，禁硬编码格式） |
| `extValue` | 班次数据 JSON 字符串（见下方组装规则） |

extValue 组装规则（素材全部来自 `dws attendance approve supply-plans` 选定班次）：

```
extValue = JSON.stringify({
  planId:      plans.planId,        // 可选（自由工时排班可为空）
  planTip:     plans.planTip,       // 带状态卡点文案，原样透传不解析
  planText:    plans.planText,
  workDate:    plans.workDate,
  timeStamp:   plans.workDate,      // 恒等于 workDate
  userCheckTime: 最终补卡时刻        // plans.supplyDate；越出 timeRange 时用夹取值，必须与 value 同一时刻
})
// clamp 规则：supplyDate < timeRange[0] → 取 timeRange[0]；
// supplyDate > timeRange[1] → 取 timeRange[1]；夹取值同时用于 supply-check --timestamp、userCheckTime 与 value
// timeZoneInfo 不本地拼接（可选字段）：服务端 supply-plans 响应不含时区数据，
// PC 真实数据不含亦可成功；仅 multiTimeZoneV2 灰度企业需要，一期不支持
```

> **IMPORTANT：**
> - `bizAlias` 不组装（MCP formComponentValues 无此字段，服务端按 id 匹配）；**不构造 `repairCheckTime` 条目**（服务端回填字段，PC 提交数据无此字段名）。
> - 班次匹配与资格判定一律以服务端为准：`supply-plans` 空 → 转告终止；多班次 → 用户按 planTip 选择；`supply-check` qualify=false → 转告 title/desc 终止。
> - 补卡理由控件（TextareaField）是否必收以 form-schema 的 required 为准（控件必填性由模板配置决定）；图片控件（DDPhotoField）一期跳过并提示客户端补充。
> - 模板流程不固定（自选审批人只是常见配置之一，管理员可自由改为主管链/条件分支等）：forecast 返回 `workflowActivityRuleVOs[].targetSelect=true` 且 `workflowActor.required=true` 的节点时**必须选人**并组装 `targetSelectActioners`（actionerKey=`workflowActor.actorKey`，漏选会创建成功但流转挂起）；无自选节点则不需要选人，一切以 forecast 返回为准。
> - `timeZoneInfo` **不本地拼接**（可选字段）：服务端 `supply-plans` 响应不含时区数据，PC 真实数据不含亦可成功。多时区 V2 灰度（multiTimeZoneV2）与补卡次数 trial（enableSupplyTimes）一期不支持。

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

> **部分支持的控件：** `DDPhotoField`（图片控件）**支持通过 URL 直接提交**，但不支持本地文件上传（CLI 未封装钉盘 CDN 上传流程）。若用户只有本地文件，需告知在钉钉客户端补充。详见本文 [DDPhotoField](#ddphotofield图片控件) 章节。

> **套件类控件（大部分暂不支持）** — `InvoiceField`（发票）、`RecipientAccountField`（收款账户）等业务套件控件当前暂不支持通过 CLI 发起，包含这些控件的审批模板请直接在钉钉客户端操作。**例外：`DDHolidayField`（请假套件）与 `DDBizSuite · attendance.supply`（补卡套件）已支持**，按上方章节与 [oa.md](../oa.md)「发起请假审批」/「发起补卡审批」工作流发起。

---

## 组装优先级

1. **每次发起前都重新调用 `form-schema`**，不得复用旧结果（模板可能已被修改）
2. 先读 `form-schema` 返回的 `content`，识别所有控件的 `label`、`componentName`、`options`、`props.required`
3. **检查是否存在不支持控件且为必填项（`props.required: true`）**，若有则直接告知用户该模板不支持通过 CLI 发起，请在钉钉客户端操作
4. 按本文档中每种控件的 value 格式组装 `formComponentValues`
5. **不要把 `form-schema` 的 `content` 当成可直接提交的模板**
6. 遇到 API 不支持的控件（非必填），跳过并告知用户
