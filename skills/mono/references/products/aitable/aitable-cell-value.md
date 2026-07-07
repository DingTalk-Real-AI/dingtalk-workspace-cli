# cells 写入/读取格式规范（cellValue 数据结构）

> 适用命令：`dws aitable record create --records`、`dws aitable record update --records`、`dws aitable record query` 返回
>
> 本文件是 DWS AI 表格 cellValue 的 **source of truth**。写入记录时，必须严格按此格式构造 cells 对象。

## 顶层规则

- cells 的 key **必须是 fieldId**（如 `fldXXX`），不是字段名称
- fieldId 必须从 `table get` 返回中获取
- 不同字段类型的 value 格式不同，混用会报错
- 系统只读字段（creator/lastModifier/createdTime/lastModifiedTime/formula）不可写入

## 各字段类型详解

### text（文本）

**写入**：字符串
```json
{"fldTextId": "这是一段文本"}
```

**读取**：字符串
```json
{"fldTextId": "这是一段文本"}
```

---

### number（数字）

**写入**：数字或数字字符串
```json
{"fldNumId": 123.45}
{"fldNumId": "123.45"}
```

**读取**：字符串形式的数字
```json
{"fldNumId": "123.45"}
```

---

### singleSelect（单选）

**写入**：选项名称字符串（推荐），或对象形式 `{id, name}`
```json
{"fldSelectId": "进行中"}
{"fldSelectId": {"id": "opt_xxx", "name": "进行中"}}
```

> 写入不存在的选项名称时，系统会自动创建该选项。
> 对象写入时 id 为准，服务端会校验 id 是否存在。

**读取**：对象 `{id, name}`
```json
{"fldSelectId": {"id": "opt_abc123", "name": "进行中"}}
```

---

### multipleSelect（多选）

**写入**：选项名称数组（推荐），或对象数组
```json
{"fldMultiId": ["标签A", "标签B"]}
{"fldMultiId": [{"id": "opt_a", "name": "标签A"}, {"id": "opt_b", "name": "标签B"}]}
```

> 写入时每项需带 id（对象模式）或直接传 name 字符串。不存在的 name 会自动补入选项配置。

**读取**：对象数组
```json
{"fldMultiId": [{"id": "opt_a", "name": "标签A"}, {"id": "opt_b", "name": "标签B"}]}
```

---

### date（日期）

**写入**：日期字符串、RFC3339 字符串、或毫秒时间戳
```json
{"fldDateId": "2026-03-15"}
{"fldDateId": "2026-03-15 09:00"}
{"fldDateId": "2026-03-15T09:00+08:00"}
```

**读取**：RFC3339 字符串
```json
{"fldDateId": "2026-03-15T09:00:00+08:00"}
```

---

### currency（货币）

**写入**：数字（与 number 相同）
```json
{"fldCurrencyId": 99.5}
```

**读取**：字符串形式的数字（小数位数取决于 formatter 配置）
```json
{"fldCurrencyId": "99.5"}
```

---

### progress（进度）

**写入**：0~1 之间的浮点数（0 表示 0%，1 表示 100%）
```json
{"fldProgressId": 0.75}
```

> ⚠️ **常见错误**：写入 75 不会报错，但会被存储为 7500%（因为系统将其理解为 75 倍）。
> 正确做法：75% 应写入 0.75。API 不会拒绝超出 [0,1] 的值，但显示会异常。
> 如果字段配置了 `customizeRange`，则按自定义范围传值。

**读取**：字符串形式的数字
```json
{"fldProgressId": "0.75"}
```

---

### rating（评分）

**写入**：整数，必须在字段配置的 min~max 范围内
```json
{"fldRatingId": 4}
```

> ⚠️ 超出 max 范围的值（如 max=5 时写入 6）会被服务端拒绝并返回错误。

**读取**：数字（字符串形式）
```json
{"fldRatingId": "4"}
```

---

### checkbox（勾选）

**写入**：布尔值
```json
{"fldCheckId": true}
{"fldCheckId": false}
```

**读取**：布尔值
```json
{"fldCheckId": true}
```

---

### user（人员）

**写入**：对象数组，每项必须含 `userId` 和 `corpId`
```json
{"fldUserId": [{"userId": "staff_001", "corpId": "dingxxxxxxxx"}]}
```

> 单选字段（`multiple=false`）也必须传数组，只是数组长度为 1。
> 如果目标用户不在当前请求组织内，回退为 `[{"userRef": "ur_0AaZ19"}]`。

**读取**：对象数组
```json
{"fldUserId": [{"userId": "staff_001", "corpId": "dingxxxxxxxx"}]}
```

---

### department（部门）

**写入**：对象数组，每项含 `deptId`
```json
{"fldDeptId": [{"deptId": "52528700"}]}
```

**读取**：对象数组
```json
{"fldDeptId": [{"deptId": "52528700"}]}
```

---

### group（群组）

**写入**：对象数组，每项含 `cid`
```json
{"fldGroupId": [{"cid": "74577067501"}]}
```

> ⚠️ key 是 **`cid`**，不是 `openConversationId`

**读取**：对象数组
```json
{"fldGroupId": [{"cid": "74577067501"}]}
```

---

### url（链接）

**写入**：对象 `{text, link}` 或纯 URL 字符串
```json
{"fldUrlId": {"text": "钉钉官网", "link": "https://dingtalk.com"}}
{"fldUrlId": "https://dingtalk.com"}
```

> 纯字符串写入时，服务端自动补齐为 `{"text":"原字符串","link":"原字符串"}`

**读取**：对象 `{text, link}`
```json
{"fldUrlId": {"text": "钉钉官网", "link": "https://dingtalk.com"}}
```

---

### richText（富文本）

**写入**：对象 `{markdown: "..."}`
```json
{"fldRichId": {"markdown": "**加粗**\n普通文字\n"}}
```

**读取**：对象 `{markdown: "..."}`（有损，颜色/@人等信息可能丢失）
```json
{"fldRichId": {"markdown": "**加粗**\n普通文字\n"}}
```

---

### attachment（附件）

**写入**：对象数组，支持 `fileToken` 或 `url` 形式

```json
{"fldAttachId": [{"fileToken": "ft_xxx"}]}
{"fldAttachId": [{"url": "https://example.com/file.pdf"}]}
```

> ⚠️ **必须先通过 [attachment upload 流程](./aitable-attachment.md) 获取 `fileToken`**。
> URL 形式是 best-effort 异步转存，不保证立即可用。
> 写入会**整体覆盖**原附件列表，不是追加。

**读取**：对象数组（含下载链接、文件名、大小）
```json
{"fldAttachId": [{"url": "https://...", "filename": "report.pdf", "size": 204800}]}
```

---

### telephone / email / barcode / idCard（电话/邮箱/条码/身份证）

**写入**：字符串
```json
{"fldPhoneId": "13800138000"}
{"fldEmailId": "test@example.com"}
{"fldBarcodeId": "978-3-16-148410-0"}
{"fldIdCardId": "520402196001067498"}
```

> idCard 必须是后端认可的合法身份证号格式

**读取**：字符串
```json
{"fldPhoneId": "13800138000"}
```

---

### geolocation（地理位置）

**写入**：对象，包含 `address`、`name`、`location`
```json
{
  "fldGeoId": {
    "address": "浙江省杭州市思凯路与爱橙街交叉口东南200米",
    "name": "阿里中心·未科D1幢",
    "location": ["120.007852", "30.271194"]
  }
}
```

> `location` 按 **[经度, 纬度]** 传**字符串数组**

**读取**：对象（含额外的 `fullAddress` 字段，由服务端自动拼接）
```json
{
  "fldGeoId": {
    "address": "浙江省杭州市",
    "fullAddress": "阿里中心-浙江省杭州市",
    "name": "阿里中心",
    "location": ["120.007852", "30.271194"]
  }
}
```

---

### unidirectionalLink / bidirectionalLink（关联字段）

**写入**：对象 `{linkedRecordIds: [...]}`
```json
{"fldLinkId": {"linkedRecordIds": ["recXXX", "recYYY"]}}
```

**读取**：对象 `{linkedRecordIds: [...]}`
```json
{"fldLinkId": {"linkedRecordIds": ["recXXX", "recYYY"]}}
```

---

### 只读字段（禁止写入）

以下字段类型由系统自动填充，`record create/update` 时**禁止传入**：

| 类型 | 说明 |
|------|------|
| `creator` | 创建人 |
| `lastModifier` | 最后编辑人 |
| `createdTime` | 创建时间 |
| `lastModifiedTime` | 最后编辑时间 |
| `formula` | 公式字段（系统计算） |
| AI 字段 | 由 AI 自动计算 |

## 常见错误速查

| 错误 | 正确做法 |
|------|----------|
| cells key 用字段名称 `"课程名称"` | 用 fieldId `"fldXXX"` |
| progress 写入 `75` | 写入 `0.75`（范围 0~1） |
| attachment 直接传文件路径 | 必须先 upload 获取 fileToken |
| user 字段传用户名字符串 | 传对象数组 `[{"userId":"...", "corpId":"..."}]` |
| group 字段用 `openConversationId` | 用 `cid` |
| singleSelect 传 option id 字符串 | 传 name 字符串或 `{"id":"...", "name":"..."}` 对象 |
| 对只读字段写入值 | 不传该字段，由系统自动填充 |
