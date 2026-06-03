# 字段类型 config 规范（field create / table create / field update）

> 适用命令：`dws aitable field create`、`dws aitable table create --fields`、`dws aitable field update --config`
>
> 本文件是 DWS AI 表格字段 config 的 **source of truth**。创建/更新字段时，必须严格按此规范构造 JSON。

## 1. 顶层规则

- `table create --fields` 和 `field create --fields` 中每个字段对象：`{"fieldName":"xxx", "type":"xxx", "config":{...}}`
- `field create --name --type --config` 中 config 单独传 JSON 字符串
- `field update --config` 只传 config 部分
- 不需要 config 的类型（如 text、checkbox、attachment）可省略 config 字段
- 成员/负责人字段类型使用 `user`，不要使用 `member`；字段类型不要写 `Text`/`Number`，统一写规范值 `text`/`number`

## 2. 字段类型速查

| type | 需要 config | config 核心字段 | 说明 |
|------|-------------|----------------|------|
| `text` | ❌ | — | 纯文本 |
| `number` | 可选 | `formatter` | 数字格式 |
| `singleSelect` | ✅ | `options` | 单选 |
| `multipleSelect` | ✅ | `options` | 多选 |
| `date` | 可选 | `formatter` | 日期格式 |
| `currency` | 可选 | `currencyType`, `formatter` | 货币 |
| `progress` | 可选 | `formatter`, `min`, `max`, `customizeRange` | 进度条 |
| `rating` | 可选 | `min`, `max`, `icon` | 评分 |
| `checkbox` | ❌ | — | 勾选框 |
| `user` | 可选 | `multiple` | 人员 |
| `department` | 可选 | `multiple` | 部门 |
| `group` | 可选 | `multiple` | 群组 |
| `url` | ❌ | — | 链接 |
| `richText` | ❌ | — | 富文本 |
| `telephone` | ❌ | — | 电话 |
| `email` | ❌ | — | 邮箱 |
| `attachment` | ❌ | — | 附件 |
| `geolocation` | ❌ | — | 地理位置 |
| `formula` | ✅ | `formula` | 公式（只读字段） |
| `unidirectionalLink` | ✅ | `linkedTableId`, `multiple` | 单向关联 |
| `bidirectionalLink` | ✅ | `linkedTableId`, `multiple` | 双向关联 |
| `creator` | ❌ | — | 系统字段：创建人（只读） |
| `lastModifier` | ❌ | — | 系统字段：最后编辑人（只读） |
| `createdTime` | ❌ | — | 系统字段：创建时间（只读） |
| `lastModifiedTime` | ❌ | — | 系统字段：最后编辑时间（只读） |

## 3. 各类型 config 详解

### 3.1 number（数字）

config 字段：`formatter`

可选值：
- `INT` — 整数
- `FLOAT_1` — 1 位小数
- `FLOAT_2` — 2 位小数（默认）
- `FLOAT_3` — 3 位小数
- `FLOAT_4` — 4 位小数
- `THOUSAND` — 千分位整数
- `THOUSAND_FLOAT` — 千分位 + 小数
- `PERCENT` — 百分比（整数）
- `PERCENT_FLOAT` — 百分比（小数）

```json
{"fieldName": "工时", "type": "number", "config": {"formatter": "FLOAT_2"}}
```

```json
{"fieldName": "完成率", "type": "number", "config": {"formatter": "PERCENT"}}
```

### 3.2 singleSelect / multipleSelect（单选 / 多选）

config 字段：`options`（必填）

options 结构：
- `options` 是数组，每项至少包含 `name`
- 创建时只传 `name`，`id` 由系统生成
- **更新时**：已有选项必须回传原 `id`（从 `field get` 获取），新增选项不传 id

```json
{
  "fieldName": "优先级",
  "type": "singleSelect",
  "config": {
    "options": [
      {"name": "紧急"},
      {"name": "高"},
      {"name": "中"},
      {"name": "低"}
    ]
  }
}
```

更新已有字段时（保留原选项 + 新增）：
```json
{
  "options": [
    {"id": "opt_existing_1", "name": "紧急"},
    {"id": "opt_existing_2", "name": "高"},
    {"id": "opt_existing_3", "name": "中"},
    {"name": "极低"}
  ]
}
```

> 更新 options 是**全量覆盖**，不是追加！不传的旧选项会被删除，关联的单元格数据丢失。

### 3.3 date（日期）

config 字段：`formatter`

可选值：
- `YYYY-MM-DD`（默认）
- `YYYY-MM-DD HH:mm`
- `YYYY-MM-DD HH:mm:ss`
- `YYYY/MM/DD`
- `YYYY/MM/DD HH:mm`

```json
{"fieldName": "截止日期", "type": "date", "config": {"formatter": "YYYY-MM-DD"}}
```

```json
{"fieldName": "创建时间", "type": "date", "config": {"formatter": "YYYY-MM-DD HH:mm"}}
```

### 3.4 currency（货币）

config 字段：`currencyType`（必填）、`formatter`（可选）

currencyType 可选值：
`CNY` | `HKD` | `USD` | `EUR` | `GBP` | `MOP` | `VND` | `JPY` | `KRW` | `AED` | `AUD` | `BRL` | `CAD` | `CHF` | `INR` | `IDR` | `MXN` | `MYR` | `PHP` | `PLN` | `RUB` | `SGD` | `THB` | `TRY` | `TWD`

formatter 可选值（控制小数位）：`INT` | `FLOAT_1` | `FLOAT_2`（默认）| `FLOAT_3` | `FLOAT_4`

```json
{"fieldName": "预算", "type": "currency", "config": {"currencyType": "CNY", "formatter": "FLOAT_2"}}
```

### 3.5 progress（进度）

config 字段：`formatter`（固定为 `PERCENT`）、`customizeRange`、`min`、`max`

- 默认范围：0~1（即 0%~100%）
- 自定义范围时 `customizeRange` 必须为 `true`

```json
{"fieldName": "完成度", "type": "progress", "config": {"formatter": "PERCENT"}}
```

自定义范围：
```json
{"fieldName": "进度", "type": "progress", "config": {"formatter": "PERCENT", "customizeRange": true, "min": 0, "max": 1}}
```

### 3.6 rating（评分）

config 字段：`min`、`max`、`icon`

- `min`：固定为 `1`
- `max`：1~10，默认 `5`
- `icon`：默认 `star`

```json
{"fieldName": "满意度", "type": "rating", "config": {"min": 1, "max": 5, "icon": "star"}}
```

### 3.7 user / department / group（人员 / 部门 / 群组）

config 字段：`multiple`

- `multiple`：`true`（多选，默认）| `false`（单选）

```json
{"fieldName": "负责人", "type": "user", "config": {"multiple": false}}
```

```json
{"fieldName": "协作部门", "type": "department", "config": {"multiple": true}}
```

### 3.8 formula（公式）

config 字段：`formula`（必填）

- 公式中引用字段使用**方括号 + 字段名**：`[字段名]`
- 支持的函数：参考钉钉 AI 表格公式文档

```json
{"fieldName": "合计", "type": "formula", "config": {"formula": "[单价] * [数量]"}}
```

```json
{"fieldName": "是否逾期", "type": "formula", "config": {"formula": "IF([截止日期] < NOW(), \"是\", \"否\")"}}
```

> ⚠️ formula 字段创建后为**只读**，不能通过 record create/update 写入值。

### 3.9 unidirectionalLink（单向关联）

config 字段：`linkedTableId`（必填）、`multiple`

- `linkedTableId`：目标表的 tableId
- `multiple`：`true`（多选，默认）| `false`（单选）

```json
{"fieldName": "关联项目", "type": "unidirectionalLink", "config": {"linkedTableId": "tblXXXXXX", "multiple": true}}
```

### 3.10 bidirectionalLink（双向关联）

config 字段：`linkedTableId`（必填）、`multiple`

- 与单向关联参数相同
- 创建后系统会**自动**在被关联表创建反向字段

```json
{"fieldName": "关联任务", "type": "bidirectionalLink", "config": {"linkedTableId": "tblYYYYYY", "multiple": true}}
```

## 4. AI 字段（ai-config）

AI 字段不使用 config，而使用独立的 `--ai-config` 参数。详见 [aitable-field.md](./aitable-field.md) 中的 AI 字段创建示例。

核心规则：
- `outputType` 必须与 `--type` 对应：text→text, select→singleSelect, multiSelect→multipleSelect, number→number, currency→currency, image/video→attachment
- `prompt` 中必须至少包含一个 `fieldRef` 引用
- 纯文本 prompt 会被后端拒绝

## 5. 常见错误

| 错误 | 说明 |
|------|------|
| options 更新时不传已有选项的 id | 会被视为新选项，旧选项被删除，关联数据丢失 |
| options 更新时只传新增项 | 全量覆盖，旧选项全部丢失 |
| formula 字段尝试写入值 | 只读字段，record create/update 会报错 |
| linkedTableId 传表名而非 ID | 必须传 tableId（如 `tblXXX`），不接受表名 |
| 成员字段使用 `member` | 不支持；人员/负责人/成员字段统一使用 `user` |
| progress 值写入 50 表示 50% | 实际应写入 0.5（range 0~1） |
| rating 值超出 max | 写入会报错 |
