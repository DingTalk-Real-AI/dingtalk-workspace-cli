# OA 审批 (oa) 命令参考

## 命令总览

### 查询待我处理的审批
```
Usage:
  dws oa approval list-pending [flags]
Example:
  dws oa approval list-pending --start "2026-03-10T00:00:00+08:00" --end "2026-03-10T23:59:59+08:00"
  dws oa approval list-pending --start "2026-03-10T00:00:00+08:00" --end "2026-03-10T23:59:59+08:00" --query 关键词
Flags:
      --end string   结束时间 ISO-8601 (如 2026-03-10T23:59:59+08:00) (必填)
      --page string  分页页码 (可选)
      --limit string 每页大小 (可选)
      --start string 开始时间 ISO-8601 (如 2026-03-10T00:00:00+08:00) (必填)
      --query string  关键字搜索 (可选)
```

### 获取审批实例详情
```
Usage:
  dws oa approval detail [flags]
Example:
  dws oa approval detail --instance-id <processInstanceId>
Flags:
      --instance-id string   审批实例 ID (必填)
```

### 同意审批

> **CAUTION:** 审批决策不可撤回 — 执行前必须向用户确认。

```
Usage:
  dws oa approval approve [flags]
Example:
  dws oa approval approve --instance-id <id> --task-id <taskId>
  dws oa approval approve --instance-id <id> --task-id <taskId> --remark "同意"
Flags:
      --instance-id string   审批实例 ID (必填)
      --remark string        审批意见 (可选)
      --task-id string       审批任务 ID (必填)
```

### 拒绝审批

> **CAUTION:** 审批决策不可撤回 — 执行前必须向用户确认。

```
Usage:
  dws oa approval reject [flags]
Example:
  dws oa approval reject --instance-id <id> --task-id <taskId> --remark "不同意"
Flags:
      --instance-id string   审批实例 ID (必填)
      --remark string        审批意见 (可选)
      --task-id string       审批任务 ID (必填)
```

### 撤销已发起的审批
```
Usage:
  dws oa approval revoke [flags]
Example:
  dws oa approval revoke --instance-id <id> --yes
  dws oa approval revoke --instance-id <id> --remark "误发起" --yes
Flags:
      --instance-id string   审批实例 ID (必填)
      --remark string        撤销说明 (可选)
```

### 获取审批操作记录
```
Usage:
  dws oa approval records [flags]
Example:
  dws oa approval records --instance-id <processInstanceId>
Flags:
      --instance-id string   审批实例 ID (必填)
```

### 查询我已发起的审批实例记录
```
Usage:
  dws oa approval list-initiated [flags]
Example:
  dws oa approval list-initiated --process-code <code> --start "2026-03-10T00:00:00+08:00" --end "2026-03-10T23:59:59+08:00" --cursor 0 --limit 20
Flags:
      --end string            结束时间 ISO-8601 (如 2026-03-10T23:59:59+08:00) (必填)
      --limit string          每页大小，最大 20 (必填)
      --cursor string         分页游标，首次传 0 (必填)
      --process-code string   表单 processCode (必填)
      --start string          开始时间 ISO-8601 (如 2026-03-10T00:00:00+08:00) (必填)
```

### 获取当前用户可见的审批表单列表
```
Usage:
  dws oa approval list-forms [flags]
Example:
  dws oa approval list-forms --cursor 0 --limit 100
Flags:
      --cursor string  分页游标，首次传 0 (默认 "0")
      --limit string   每页大小，最大 100 (默认 "100")
```
MCP 工具: `list_user_visible_process`；参数: cursor, pageSize（对应 --cursor/--limit）。返回结果含 processCode，可用于 list-initiated 的 --process-code。

### 按关键字模糊搜索审批表单
```
Usage:
  dws oa approval search-forms [flags]
Example:
  dws oa approval search-forms --query AI
  dws oa approval search-forms --query 报销
Flags:
      --query string  关键字，匹配 processCode 或表单名称 (必填)
```

### 按模板 processCode 查询表单 Schema 信息

> **说明：** 根据已知的 processCode 精确查询表单的完整 Schema，包括表单名称、状态、创建者、创建/修改时间以及表单组件 JSON（content 字段）。

```
Usage:
  dws oa approval form-schema [flags]
Example:
  dws oa approval form-schema --process-code PROC-594AE140-6AA5-4BA4-AF0C-9E6F66DB1E0B
Flags:
      --process-code string  表单模板 processCode (必填)
```

返回值字段：
- `result.processName` — 表单名称
- `result.processCode` — 表单 processCode
- `result.processStatus` — 表单状态（如 `PUBLISHED`）
- `result.creator` — 创建者 userId
- `result.gmtCreate` / `result.gmtModified` — 创建/修改时间（毫秒时间戳）
- `result.processIconUrl` — 表单图标 URL
- `result.processDescription` — 表单描述
- `result.content` — 表单组件 JSON 字符串，包含表单项（items）和标题等配置

### 流程预测

```
Usage:
  dws oa approval forecast-process [flags]
Example:
  # 简单预测
  dws oa approval forecast-process --process-code PROC-xxx --dept-id -1 --form-values '{"单行输入框":"测试内容"}'
  # 指定部门预测
  dws oa approval forecast-process --process-code PROC-xxx --dept-id 12345 --form-values '{"金额":"5000"}'
  # 高级用法：传入完整 JSON
  dws oa approval forecast-process --request '{"processCode":"PROC-xxx","deptId":-1,"formComponentValues":[[{"name":"单行输入框","value":"测试"}]]}'
Flags:
      --process-code string   审批模板 processCode（简单模式必填）
      --form-values string    表单值 JSON，格式 '{"控件名称":"值"}'（简单模式必填）
      --dept-id string        发起人所在部门 ID，根部门填 -1（简单模式必填）
      --request string        完整请求体 JSON（高级模式，与简单模式互斥）
```

> **注意：** forecast 接口的 `formComponentValues` 比 create-instance 多一层数组包裹（`[[{...}]]`），CLI 简单模式已自动处理，高级模式需自行包裹。`processCode`、`deptId`、`formComponentValues` 三个字段均为必填，`userId` 由系统从登录态自动填充。

#### 流程预测的作用

在 `create-instance` 之前调用 `forecast-process`，可以根据已填写的表单值预测审批流程走向，核心价值有两个：

1. **展示流程路径** — 告诉用户这个审批会经过哪些节点（审批人、抄送人、条件分支），让用户在提交前就知道流程走向。
2. **识别自选审批人节点** — 返回中 `targetSelect: true` 的节点需要用户手动选择审批人/抄送人，Agent 应提示用户选人，并将结果传入 `create-instance` 的 `targetSelectActioners`。

#### 返回值关键字段

| 字段 | 含义 |
|------|------|
| `result.forecastSuccess` | 预测是否成功 |
| `result.staticWorkflow` | 是否为静态流程（无条件分支） |
| `result.workflowForecastNodes` | 流程节点路径，每个节点包含 `activityId` 和 `outIds`（下一跳） |
| `result.workflowActivityRuleVOs` | **重点**：每个节点的详细规则，包含节点类型、审批人、是否自选等 |

#### `workflowActivityRuleVOs` 节点字段解读

| 字段 | 含义 |
|------|------|
| `activityId` | 节点 ID |
| `workflowActor.actorKey` | 自选节点的规则 key，即 `targetSelectActioners` 中 `actionerKey` 的值 |
| `activityName` | 节点名称（如"审批人"、"抄送人"） |
| `activityType` | 节点类型：`target_approval`（已指定审批人）、`target_select`（需自选）、`target_notifier`（抄送） |
| `targetSelect` | **`true` 表示需要用户自选审批人/抄送人** |
| `activityActioners` | 已确定的处理人列表（含 `emplId`、`name`） |
| `workflowActor.actorType` | 角色类型：`approver`（审批人）、`notifier`（抄送人） |
| `workflowActor.approvalMethod` | 多人审批方式：`ONE_BY_ONE`（依次审批） |
| `workflowActor.actorSelectionType` | 选人范围：`allStaff`（全员可选）等 |
| `prevActivityId` | 上一节点 ID |

#### Agent 处理流程

```
1. 调用 forecast-process，传入 processCode + form-values
2. 遍历 workflowActivityRuleVOs：
   a. 向用户展示每个节点的名称、类型、已指定处理人
   b. 若 targetSelect == true：
      - 提示用户"节点「{activityName}」需要您自选{actorType}人"
      - 使用 dws aisearch person --keyword "<姓名>" --dimension name --format json 帮用户查找并选人
      - 记录 activityId 和用户选择的 userIds
3. 将自选结果组装为 targetSelectActioners，传入 create-instance 高级模式 --request
```

#### 自选节点 → `targetSelectActioners` 组装示例

假设 forecast 返回两个自选节点：

```json
{
  "targetSelectActioners": [
    {
      "actionerKey": "manual_33ff_89cb_da91_e3aa",
      "actionerStaffIds": ["userId_选人A"]
    },
    {
      "actionerKey": "manual_a29e_9633_f8b7_7291",
      "actionerStaffIds": ["userId_选人B"]
    }
  ]
}
```

此字段通过 `create-instance --request` 的高级模式传入。`actionerKey` 来自 forecast 返回的 `workflowActor.actorKey`。

### 发起审批实例

#### 执行摘要

- **如果用户未明确给出 `processCode`，必须固定走 `search-forms` → `form-schema` → 收集表单值 → `forecast-process` → 自选节点选人 → `create-instance`**，不要跳过 `form-schema` 直接拼请求。
- **如果用户明确给出 `processCode`，固定走 `form-schema` → 收集表单值 → `forecast-process` → 自选节点选人 → `create-instance`**，不要跳过 `form-schema` 直接拼请求。
- **`form-schema` 返回的 `content` 不是创建 payload 的原样模板。** 它主要用于识别控件 `label`（即 name）、`id`、控件类型（componentName）和选项值范围；真正的 `formComponentValues` 中 `value` 结构以本文的控件值格式表为准。
- **`forecast-process` 返回的自选节点必须在发起前让用户选人。** 若 `workflowActivityRuleVOs` 中有 `targetSelect: true` 的节点，必须提示用户选择处理人，并将结果通过 `targetSelectActioners` 传入 `create-instance`。
- **所有人员类参数使用 userId。** 若用户给的是姓名，先用 `dws aisearch person --keyword "<姓名>" --dimension name --format json` 解析成 userId。**严禁把姓名直接写进** `approvers`、`ccList`、`directAppointedApprovers`、`targetSelectActioners` 或表单人员控件。
- **创建实例前一次性汇总确认。** `create-instance` 是写操作，执行前一次性展示模板、表单值、流程预测结果和审批人/抄送人供用户确认。

#### 严禁行为

- **严禁跳过 `form-schema`。** 未拿到表单 Schema 前，不得调用 `create-instance`。
- **严禁复用旧的 Schema 结果。** 每次发起实例前都必须重新调用 `form-schema`，模板可能已被修改。
- **严禁在存在不支持必填控件时强行发起。** 若 `form-schema` 返回的必填控件中有不支持类型（如附件等），直接告知用户不支持通过 CLI 发起。
- **严禁把 `form-schema` 返回的 `content` 当成可直接提交的 payload 模板。**
- **严禁把姓名直接写进 `approvers`、`ccList`、`directAppointedApprovers`、`targetSelectActioners` 或表单人员控件。** 必须先通过 `dws aisearch person --keyword "<姓名>" --dimension name --format json` 转成 userId。
- **严禁在未得到用户确认前直接执行真实提单。**
- **严禁猜测控件名称或选项值。** 必须从 `form-schema` 返回中提取。
- **严禁跳过 `forecast-process` 中的自选节点选人。** 若预测返回 `targetSelect: true` 的节点，必须让用户选人后再发起。

#### 最小判断表

| 你手上有什么 | 下一步 |
|---|---|
| 只有口语需求，比如"帮我发起请假审批" | 先 `search-forms --query 请假` |
| 已拿到 `processCode` | 直接 `form-schema --process-code <code>` |
| 已拿到 Schema | 向用户展示控件列表，收集表单值 |
| 已收集表单值 | `forecast-process` 预测流程走向 |
| 预测返回有 `targetSelect: true` 节点 | 让用户为自选节点选人（`dws aisearch person --keyword "<姓名>" --dimension name --format json` 解析姓名） |
| 预测完成，自选节点已选人 | 汇总确认后 `create-instance --yes` |
| 用户明确说"不走模板流程，直接指定审批人" | 使用 `directAppointedApprovers`（高级模式） |

#### 工作流

```
1. search-forms --query <关键词>     → 拿到 processCode（若已有则跳过）
2. form-schema  --process-code <code> → 拿到控件列表、类型、选项值
3. 检查 Schema 中是否有不支持的必填控件  → 若有则直接告知用户不支持发起
4. 收集表单值                       → 向用户展示控件列表，收集用户填写的表单值
5. forecast-process                  → 根据表单值预测流程走向，识别自选节点
6. 自选节点选人                       → 若预测返回 targetSelect=true 的节点，让用户选人（用 dws aisearch person --keyword "<姓名>" --dimension name --format json 解析姓名）
7. 汇总确认后 create-instance --yes  → 展示完整信息（表单值 + 流程路径 + 审批人），用户确认后执行发起
```

> **IMPORTANT：每次发起实例前都必须重新调用 `form-schema` 查询模板。** 即使用户之前查询过同一个 processCode，模板可能已被修改（控件增减、选项变更、必填属性调整等），不得复用旧的 Schema 结果。

#### 交互优化原则

> **核心目标：流程清晰，步骤有序，避免重复询问。**

1. **先查 Schema 再收集表单值（步骤 2→4）：** `form-schema` 后向用户展示需要填写的控件列表，然后一次性收集全部表单值。不要在未拿到 Schema 前就问用户填什么。

2. **流程预测后再选自选审批人（步骤 5→6）：** `forecast-process` 返回流程路径和自选节点后：
   - 先向用户展示完整的流程路径（经过哪些节点、各节点处理人）
   - 对 `targetSelect: true` 的节点，提示用户"节点「{activityName}」需要您自选{actorType}人"
   - 用 `dws aisearch person --keyword "<姓名>" --dimension name --format json` 帮用户查找并选人
   - 若有多个自选节点，一次性收集所有自选节点的选人结果

3. **单次汇总确认（步骤 7）：** 发起前一次性展示完整信息供用户确认：
   - 审批模板名称
   - 表单各控件值
   - 流程预测结果（审批路径）
   - 各节点审批人/抄送人（含自选节点选人结果）

**反例（禁止）：**
-  未查 Schema 就直接问用户填什么表单值
-  流程预测后逐个节点分别询问选人，而非一次性收集
-  用户确认前直接执行发起

```
Usage:
  dws oa approval create-instance [flags]
Example:
  # 简单发起（Agent 在汇总确认后需加 --yes）
  dws oa approval create-instance --process-code PROC-xxx --form-values '{"单行输入框":"测试内容"}' --yes
  # 指定审批人（OR=或签，AND=会签，NONE=单人）
  dws oa approval create-instance --process-code PROC-xxx --form-values '{"单行输入框":"测试"}' --approvers "userId1,userId2" --approvers-action-type OR --yes
  # 指定抄送人
  dws oa approval create-instance --process-code PROC-xxx --form-values '{"单行输入框":"测试"}' --cc-list "userId1" --cc-position START --yes
  # 高级用法：传入完整 JSON（支持 directAppointedApprovers、targetSelectActioners 等全部字段）
  dws oa approval create-instance --request '{"processCode":"PROC-xxx","deptId":-1,"formComponentValues":[{"name":"单行输入框","value":"测试"}]}' --yes
Flags:
      --process-code string              审批模板 processCode（简单模式必填）
      --form-values string               表单值 JSON，格式 '{"控件名称":"值"}'（简单模式必填）
      --dept-id string                   发起人所在部门 ID，根部门填 -1（可选，默认 -1）
      --originator-user-id string        审批发起人 userId（可选，MCP 工具可从登录态获取）
      --approvers string                 审批人 userId 列表，多个用逗号分隔（可选）
      --approvers-action-type string     审批类型：AND（会签）、OR（或签）、NONE（单人）（可选，默认 OR）
      --cc-list string                   抄送人 userId 列表，多个用逗号分隔（可选）
      --cc-position string               抄送时间点：START/FINISH/START_FINISH（可选，默认 START）
      --request string                   完整请求体 JSON（高级模式，与简单模式互斥）
      --yes                              显式确认并发起审批；未提供时命令直接拒绝，不进入交互确认（Agent 必须先汇总并获得用户确认）
```

#### 两种模式

- **简单模式：** 通过 `--process-code` + `--form-values` + 可选 flags 发起，适合大多数场景
- **高级模式：** 通过 `--request` 传入完整 JSON 请求体，支持 `directAppointedApprovers`、`targetSelectActioners` 等复杂字段

#### 组装 form-values

`form-values` 是简单模式下的核心入参；传入时必须是一个 JSON 对象字符串，key 为控件 label，value 为该控件的提交值。组装原则：

- 先用 `form-schema` 识别有哪些控件、每个控件的 `label`（name）、`componentName`（type）、选项值范围以及明细子控件结构。
- **`form-schema` 返回的 `content` 不是可直接提交的原样模板。** 它提供控件定义，`value` 结构须按下方控件值格式表组装。
- 提交时必须保证每个控件的 `name`（即 label）与 Schema 中的 `props.label` **完全一致**。
- 如果用户提供的是人员信息，先用 `dws aisearch person --keyword "<姓名>" --dimension name --format json` 转成 userId 后再写入对应控件。
- 单选/多选控件提交的是选项文本（option value），该值从 `form-schema` 返回的选项定义中取得。
- `InnerContactField`、`DepartmentField`、`TableField`、`DDDateRangeField`、`DDAttachment` 等控件的 `value` 结构各不相同，必须按下方格式表单独组装，不要套用文本控件的写法。
- `TextNote`（文字说明）不收集数据，**不要**出现在 `formComponentValues` 中。

#### 表单控件值格式速查

> **重要：** `formComponentValues` 中每条记录的 `name` 必须与审批模板中控件的 `label`（即 `form-schema` 返回的 `content.items[].props.label`）**完全一致**。`value` 为字符串类型，最大 65535 字符。
>
> **详细参考：** 每种控件的完整属性、约束和示例见 [oa-form-components.md](oa/oa-form-components.md)。组装前**必须先阅读该文档**。

| 控件类型 | componentName | value 格式 | 示例 | 备注                                                    |
|---------|---------------|-----------|------|-------------------------------------------------------|
| 单行输入框 | `TextField` | 纯文本 | `"测试内容"` |                                                       |
| 多行输入框 | `TextareaField` | 纯文本 | `"第一行\n第二行"` |                                                       |
| 数字输入框 | `NumberField` | 数字字符串 | `"100"` |                                                       |
| 单选框 | `DDSelectField` | 选项文本 | `"同意"` | 必须与模板 options 中的 value 完全匹配                           |
| 多选框 | `DDMultiSelectField` | JSON 数组字符串 | `'["选项A","选项B"]'` | 每个选项须与模板 options 匹配；                   |
| 日期控件 | `DDDateField` | `yyyy-MM-dd` | `"2026-07-27"` |                                                       |
| 时间区间 | `DDDateRangeField` | JSON 数组字符串 | `'["2026-07-27","2026-07-30"]'` | label 为数组 `["开始","结束"]`，用开始时间 label 作 name            |
| 金额控件 | `MoneyField` | 数字字符串 | `"1500.50"` | 自动显示大写金额                                              |
| 电话控件 | `PhoneField` | 手机号字符串 | `"13800138000"` |                                                       |
| 联系人控件 | `InnerContactField` | userId | `"user123"` | 多人时传 JSON 数组 `'["user1","user2"]'`；choice="0"单选/"1"多  |
| 部门控件 | `DepartmentField` | 部门 ID | `"12345"` | 多部门传 JSON 数组；multiple=true 时支持多选                      |
| 省市区控件 | `AddressField` | JSON 数组字符串 | `'["浙江省","杭州市","西湖区"]'` | 三级联动；needDetail=true 时末尾加详细地址                         |
| 图片控件 | `DDPhotoField` | URL 数组转义字符串 | `"[\"http://example.com/img1.jpg\"]"` | 支持 URL 直接提交；**不支持本地文件上传** |
| 附件控件 | `DDAttachment` | JSON 数组转义字符串 | `"[{\"spaceId\":\"xxx\",\"fileName\":\"a.pdf\",\"fileSize\":\"333\",\"fileType\":\"pdf\",\"fileId\":\"xxx\"}]"` | **当前不支持通过 CLI 提交**，需钉盘上传接口获取 fileId 等字段 |
| 评分控件 | `StarRatingField` | 数字字符串 | `"4"` | limit 控制最大星数（默认 5）                                    |
| 关联审批单 | `RelateField` | 审批实例 ID | `"q-xxx"` | 须为当前组织下已存在的实例                                         |
| 明细控件 | `TableField` | JSON 数组字符串 | `'[{"子控件名":"值1"},{"子控件名":"值2"}]'` | 不可嵌套 TableField；不可含 DDMultiSelectField/DDPhotoField；最大 100 行 |
| 身份证控件 | `IdCardField` | 身份证号 | `"330102199001011234"` | 内置格式校验                                                |
| 文字说明 | `TextNote` | — | — | **不收集数据**，不会出现在 formComponentValues 中                 |

#### API 不支持的控件

以下控件**不支持**通过创建实例 API 提交：

- `TextNote`（文字说明）— 纯展示，不收集数据
- `CalculateField`（计算公式）— 由系统自动计算
- `SeqNumberField`（流水号）— 由系统自动生成
- `OcrTextField` / `OcrIdCardField`（OCR 识别）— 需要客户端交互
- **`DDAttachment`（附件控件）— 当前不支持通过 CLI 提交**，value 需要 spaceId、fileName、fileSize、fileType、fileId 字段，须通过钉盘上传附件接口获取
- **套件类控件（暂不支持）** — `InvoiceField`（发票）、`RecipientAccountField`（收款账户）等业务套件控件当前暂不支持通过 CLI 发起，包含这些控件的审批模板请直接在钉钉客户端操作

> **部分支持的控件：** `DDPhotoField`（图片控件）**支持通过 URL 直接提交**（见上方速查表），仅不支持本地文件上传（CLI 未封装钉盘 CDN 上传流程）。若用户只有本地文件，需告知在钉钉客户端补充。

如果目标审批模板包含上述控件，不要硬拼 `form-values`；应告知用户这些字段无需填写或需要在钉钉客户端补充。

> **必填不支持控件判断规则：** 检查 `form-schema` 返回的控件列表，若存在上述不支持控件且其 `props.required` 为 `true`（必填项），则**直接告知用户该审批模板不支持通过 CLI 发起**，请在钉钉客户端操作。只有不支持控件为非必填时，才可跳过该控件继续发起。

#### 高级模式请求体字段（`--request` JSON 完整结构）

| 字段 | 类型 | 必填 | 说明 |
|-----|------|------|------|
| `processCode` | String | 是 | 审批模板唯一码 |
| `originatorUserId` | String | 是 | 发起人 userId（MCP 工具可从登录态自动获取） |
| `deptId` | Long | 否 | 发起人部门 ID，根部门填 -1；approvers 已传时可不填 |
| `formComponentValues` | Array | 是 | 表单控件值列表，最大 150 条 |
| `approvers` | Array | 否 | 指定审批人列表（覆盖模板流程），最大 20 条 |
| `approvers[].actionType` | String | 否 | `AND`（会签）/ `OR`（或签）/ `NONE`（单人） |
| `approvers[].userIds` | Array | 否 | 审批人 userId 列表 |
| `ccList` | Array | 否 | 抄送人 userId 列表，最大 50 |
| `ccPosition` | String | 否 | `START` / `FINISH` / `START_FINISH` |
| `directAppointedApprovers` | Array | 否 | 指定审批人组（覆盖模板流程），结构见下方 |
| `targetSelectActioners` | Array | 否 | 自选审批人（模板中有自选节点时必填），最大 20 条 |

#### 节点参数组装

> **详细参考：** 流程节点类型、审批模式、条件分支和 10 种审批人选择规则的完整说明见 [oa-process-nodes.md](oa/oa-process-nodes.md)。

**directAppointedApprovers（指定审批人覆盖模板流程）：**

当用户明确说"不走模板默认流程"或"直接指定 XX 审批"时使用。

```json
[
  {
    "staffIds": ["userId1", "userId2"],
    "taskActionType": "NONE",
    "staffId": ""
  }
]
```
- `staffIds`：审批人 userId 列表（必须通过 `dws aisearch person --keyword "<姓名>" --dimension name --format json` 获取，严禁填姓名）
- `taskActionType`：`NONE`（单人审批）/ `AND`（会签）/ `OR`（或签）

**targetSelectActioners（模板有自选审批节点时使用）：**

当 `form-schema` 返回的模板流程中存在自选审批节点（`target_select` 类型）时必填。

```json
[
  {
    "actionerKey": "manual_nodeId_xxxx_yyyy",
    "actionerStaffIds": ["userId1"]
  }
]
```
- `actionerKey`：自选节点的规则 key，可通过获取审批单流程节点信息接口获取 `actorKey`
- `actionerStaffIds`：操作人 userId 列表

**审批类型（approvers actionType）说明：**

| 值 | 含义 | 说明 |
|----|------|------|
| `AND` | 会签 | 所有审批人都必须审批通过 |
| `OR` | 或签 | 任一审批人审批即可 |
| `NONE` | 单人审批 | 只有一个审批人 |

**抄送时间点（ccPosition）说明：**

| 值 | 含义 |
|----|------|
| `START` | 审批发起时抄送 |
| `FINISH` | 审批完成时抄送 |
| `START_FINISH` | 发起和完成时都抄送 |

#### 表单控件约束

- 单个表单最多 200 个控件
- 控件 label（name）和 placeholder 最大 50 字符
- `DDSelectField` / `DDMultiSelectField` 的选项 value 必须与模板中配置的选项文本完全一致
- `TableField`（明细）内不可嵌套 `TableField`，不可包含 `DDMultiSelectField` 和 `DDPhotoField`
- `TextNote`（文字说明）不收集数据，无需在 `formComponentValues` 中传入
- `InnerContactField` 的 userId 应为当前组织下在职成员
- `DepartmentField` 应传入当前组织下存在的部门 ID
- `RelateField` 传入的审批实例 ID 应为当前组织下已存在的实例

#### 返回结果

创建成功后，返回的 `result` 字段即为新审批实例的 `processInstanceId`。建议向用户展示：

```
审批已创建成功：

- 审批模板: <processName>（来自 form-schema）
- 审批实例 ID: <processInstanceId>（来自 create-instance 返回的 result）
```

后续可用该 processInstanceId 执行 `detail`、`tasks`、`records`、`revoke` 等操作。
### 获取审批任务的被催办人 userId

> **催办必须两步串联：** ① `ding-info` 获取被催办人 `userId` → ② `ding message send` 发送催办消息。禁止跳过第一步直接猜测 userId。

```
Usage:
  dws oa approval ding-info [flags]
Example:
  dws oa approval ding-info --task-id <taskId>
Flags:
      --task-id string  审批任务 ID (必填)，来自 list-pending 或 tasks
```

返回值字段：
- `userId` — 被催办人用户 ID（必取），作为 `ding message send` 的 `--users` 入参，多个以逗号拼接

**不返回** robotCode 和 content，需由 agent 自行处理：
- `--robot-code`：优先取环境变量 `$DINGTALK_DING_ROBOT_CODE`；若无则向用户确认
- `--content`：由 agent 根据审批上下文撰写催办文案，建议格式 `"请尽快审批《{表单名}》（提交人：{发起人}，提交时间：{时间}）"`
- `--users`：取本接口返回的 `userId`

**叮消息类型（`--type`）：**
- 默认不发 `--type` → 应用内 DING 提醒（免费，推荐）
- `--type sms` → 短信提醒（有成本，需向用户确认）
- `--type call` → 电话提醒（有成本，需向用户确认）

**完整催办流程：**
```bash
# Step 1: 获取被催办人 userId
dws oa approval ding-info --task-id <taskId> --format json
# Step 2: 发送催办消息（robotCode 优先走环境变量，content 由 agent 撰写）
dws ding message send --robot-code $DINGTALK_DING_ROBOT_CODE --users <userId逗号拼接> --content "请尽快审批《XXX》" --format json
# Step 3 (可选): 如需短信/电话提醒，加 --type sms 或 --type call
dws ding message send --robot-code $DINGTALK_DING_ROBOT_CODE --users <userId逗号拼接> --content "请尽快审批《XXX》" --type sms --format json
```

### 获取任务可回退的节点信息

> **IMPORTANT:** 退回任务前**必须先调用此命令**获取可回退节点列表，从中提取 `activityId` 和 `revertAction` 作为 `revert-task` 的入参。若无返回值，明确告知用户"当前任务无可回退节点"。

```
Usage:
  dws oa approval revert-activities [flags]
Example:
  dws oa approval revert-activities --task-id <taskId>
Flags:
      --task-id string  审批任务 ID (必填)
```

返回字段说明：
- `instRevertActivities[]` — 可回退的节点列表
  - `activityId` — 节点 ID，即 `revert-task` 的 `--target-activity-id`
  - `activityName` — 节点名称（如"发起人"、"审批人"），用于向用户展示
  - `revertAction` — 退回方式，即 `revert-task` 的 `--action`
    - `REVERT_FOR_RESUBMIT` → 退回到发起人重交（此时 `activityId` 为 `sid-startevent`）
    - `REVERT_FOR_APPROVAL` → 退回到某审批节点重新审批
  - `activityActioners[]` — 该节点的审批人列表
  - `actualActioners[]` — 该节点的实际处理人列表
  - `approvalIndex` — 审批节点序号（仅审批节点有）
  - `actType` — 审批类型（如 `one_by_one` 依次审批）

**无返回值处理：** 若 `instRevertActivities` 为空或不存在，必须明确告知用户"当前任务无可回退节点"，不得继续执行退回操作。

### 查询待我审批的任务 ID
```
Usage:
  dws oa approval tasks [flags]
Example:
  dws oa approval tasks --instance-id <processInstanceId>
Flags:
      --instance-id string   审批实例 ID (必填)
```
MCP 工具: `list_pending_tasks`。


### 查询我处理过的审批单
```
Usage:
  dws oa approval list-executed [flags]
Example:
  dws oa approval list-executed --limit <pageSize> --page <pageNumber> --query 关键词
Flags:
      --page string   分页页码，可选，默认是 1
      --limit string   分页大小，可选，默认是 20
      --query string   查询关键词，可选
```
### 查询我已经提交的审批单
```
Usage:
  dws oa approval list-submitted [flags]
Example:
  dws oa approval list-submitted --limit <pageSize> --page <pageNumber> --query 关键词
Flags:
      --page string   分页页码，可选，默认是 1
      --limit string   分页大小，可选，默认是 20
      --query string   查询关键词，可选
```
### 查询抄送我的审批单
```
Usage:
  dws oa approval list-cc [flags]
Example:
  dws oa approval list-cc --limit <pageSize> --page <pageNumber> --query 关键词
Flags:
      --page string   分页页码，可选，默认是 1
      --limit string   分页大小，可选，默认是 20
      --query string   查询关键词，可选
```

### 转交审批任务
```
Usage:
  dws oa approval redirect-task [flags]
Example:
  dws oa approval redirect-task --task-id <taskId> --to-actioner-id <userId>
  dws oa approval redirect-task --task-id <taskId> --to-actioner-id <userId> --remark "请帮忙处理"
Flags:
      --task-id string          审批任务 ID (必填)
      --to-actioner-id string   转交目标用户 ID (必填)
      --remark string           转交说明 (可选)
```
MCP 工具: `redirect_task`；参数: taskId, toActionerId, remark（对应 --task-id/--to-actioner-id/--remark）。taskId 可通过 `tasks` 命令获取，toActionerId 可通过 `dws contact user search` 获取。

### 对审批实例添加评论
```
Usage:
  dws oa approval oa-comments [flags]
Example:
  dws oa approval oa-comments --instance-id <processInstanceId> --content "同意，请尽快处理"
Flags:
      --instance-id string   审批实例 ID (必填)
      --content string       评论内容 (必填)
```
MCP 工具: `dingflow_comments`；参数: processInstanceId, text（对应 --instance-id/--content）。processInstanceId 可通过 `list-pending` 或 `detail` 获取。

### 对审批实例进行抄送
```
Usage:
  dws oa approval oa-cc-noticer [flags]
Example:
  dws oa approval oa-cc-noticer --instance-id <processInstanceId> --users "68674200835816"
  dws oa approval oa-cc-noticer --instance-id <processInstanceId> --users "userId1,userId2" --operator-id "123123"
Flags:
      --instance-id string   审批实例 ID (必填)
      --users string         抄送用户 ID 列表，多个用逗号分隔 (必填)
      --operator-id string   操作人 ID (可选)
```
MCP 工具: `oa_cc_noticer`；参数: processInstanceId, userList（对应 --instance-id/--users）。processInstanceId 可通过 `list-pending` 或 `detail` 获取，抄送用户 ID 可通过 `dws contact user search` 获取。

### 对审批任务进行加签

> **CAUTION:** 加签操作不可撤回 — 执行前必须向用户确认加签类型、被加签人和激活方式。
```
Usage:
  dws oa approval append-task [flags]
Example:
  dws oa approval append-task --instance-id <processInstanceId> --task-id <taskId> --type before --appender-user-ids "userId1,userId2" --activate-type ALL --agree-all true
  dws oa approval append-task --instance-id <processInstanceId> --task-id <taskId> --type after --appender-user-ids "userId1" --activate-type ONE_BY_ONE --agree-all false
Flags:
      --instance-id string        审批实例 ID (必填)
      --task-id string            审批任务 ID (必填)
      --type string               加签类型：before（前加签），after（后加签），Parallel（并加签）(必填)
      --appender-user-ids string  被加签用户 ID 列表，多个用逗号分隔 (必填)
      --activate-type string      任务激活类型：ALL（或签），ONE_BY_ONE（依次审批）(必填)
      --agree-all string          是否需要全部同意 (必填) 是 true 否 false
```

### 退回审批任务

> **CAUTION:** 退回操作不可撤回 — 执行前必须向用户确认退回方式及目标节点。
> **前置步骤：** 必须先调用 `revert-activities` 获取可回退节点列表，从中提取 `activityId` 和 `revertAction`。若无返回值，明确告知用户"当前任务无可回退节点"。

```
Usage:
  dws oa approval revert-task [flags]
Example:
  # 退回到发起人（targetActivityId 固定传 sid-startevent）
  dws oa approval revert-task --instance-id <processInstanceId> --task-id <taskId> --target-activity-id sid-startevent --action REVERT_FOR_RESUBMIT --remark "补充说明后重提"
  # 退回到某个审批节点（targetActivityId 从 revert-activities 返回中获取 activityId）
  dws oa approval revert-task --instance-id <processInstanceId> --task-id <taskId> --target-activity-id <activityId> --action REVERT_FOR_APPROVAL --remark "重新审批"
Flags:
      --instance-id string          审批实例 ID (必填)
      --task-id string              审批任务 ID (必填)
      --target-activity-id string   退回到的节点 ID；退回发起人时固定传 sid-startevent (必填)
      --action string               退回方式：REVERT_FOR_APPROVAL（退回到审批人）/ REVERT_FOR_RESUBMIT（退回到发起人）(必填)
      --remark string               退回说明 (可选)
```


## 意图判断

用户说"待审批/待处理审批" → `approval list-pending`
用户说"审批详情/看审批" → `approval detail`
用户说"同意审批/批准" → 先 `tasks` 获取 taskId，再 `approve`
用户说"拒绝审批/驳回" → 先 `tasks` 获取 taskId，再 `reject`
用户说"撤回审批/取消审批" → `approval revoke`
用户说"审批记录/操作历史" → `approval records`
用户说"我发起的审批" → `approval list-initiated`（需 --process-code，可从 list-forms / search-forms / detail 获取）
用户说"有哪些审批表单/可见表单" → `approval list-forms`
用户说"搜索审批表单/查找xx审批表单/有没有xx表单" → `approval search-forms`（需 --query）
用户说"查表单schema/查表单结构/表单模板信息/查表单组件/查表单定义/表单有哪些字段/表单的字段信息" → `approval form-schema`（需 --process-code，可从 list-forms / search-forms / detail 获取）
用户说"预测审批流程/流程预测/审批走向/这个审批走哪些人/审批流程预览" → `approval forecast-process`（需 --process-code、--dept-id、--form-values）
  - 在 `form-schema` 之后、`create-instance` 之前调用
  - 返回的 `workflowActivityRuleVOs` 中 `targetSelect: true` 的节点需要用户自选审批人
  - 自选结果组装为 `targetSelectActioners` 传入 `create-instance`
用户说"发起审批/提交审批/帮我发起XX审批/新建审批单/提一个XX审批/帮我提XX申请" → 五步流程：① `search-forms --query XX` 获取 processCode → ② `form-schema --process-code <code>` 获取表单字段定义 → ③ 阅读 [oa-form-components.md](oa/oa-form-components.md) 和 [oa-process-nodes.md](oa/oa-process-nodes.md) 后组装表单值 → ④ `forecast-process` 预测流程走向并识别自选节点 → ⑤ 若有自选节点让用户选人，确认后 `create-instance --yes` 发起
  - 如果用户已知 processCode，可跳过第①步
  - `--form-values` 的 key 必须与 `form-schema` 返回的控件 label 一致
  - `forecast-process` 返回自选节点时必须让用户选人，不得跳过
  - 执行前**必须向用户确认**表单内容、流程预测结果、审批人和抄送人
  - 示例："帮我发起一个AI审批单" → ① `search-forms --query AI` → ② `form-schema --process-code <code>` → ③ 组装表单值 → ④ `forecast-process` → ⑤ 向用户确认流程走向和自选审批人后 `create-instance --yes`
用户说"我有哪些待审的任务" → `approval tasks`
用户说"我发起的审批单" -> `approval list-submitted`
用户说"我审批/处理过的审批单" -> `approval list-executed`
用户说"抄送我的审批单" -> `approval list-cc`
用户说"转交审批/转交任务" → `approval redirect-task`（需 --task-id 和 --to-actioner-id）
用户说"评论审批/添加评论/写评论" → `approval oa-comments`（需 --instance-id 和 --content）
用户说"抄送审批/添加抄送人" → `approval oa-cc-noticer`（需 --instance-id 和 --users）

## 核心工作流

```bash
# 1. 查看待我处理的审批 — 提取 processInstanceId
dws oa approval list-pending --start "2026-03-10T00:00:00+08:00" --end "2026-03-10T23:59:59+08:00" --format json

# 2. 查看审批详情 — 了解审批内容
dws oa approval detail --instance-id <processInstanceId> --format json

# 3. 获取待审批任务 ID — 提取 taskId
dws oa approval tasks --instance-id <processInstanceId> --format json

# 4a. 同意审批
dws oa approval approve --instance-id <id> --task-id <taskId> --remark "同意" --format json

# 4b. 拒绝审批
dws oa approval reject --instance-id <id> --task-id <taskId> --remark "不符合要求" --format json

# 5. 撤销自己发起的审批
dws oa approval revoke --instance-id <id> --remark "误发起" --format json

# 6. 查看审批操作记录
dws oa approval records --instance-id <processInstanceId> --format json

# 7. 获取可见审批表单（得到 processCode）
dws oa approval list-forms --cursor 0 --limit 100 --format json

# 7b. 按关键字模糊搜索表单（快速定位 processCode）
dws oa approval search-forms --query AI --format json

# 7c. 按 processCode 查询表单 Schema（获取表单结构、组件定义）
dws oa approval form-schema --process-code <code> --format json

# 8. 查看自己发起的审批列表（--process-code 来自 list-forms / search-forms / detail）
dws oa approval list-initiated --process-code <code> \
  --start "2026-03-10T00:00:00+08:00" --end "2026-03-10T23:59:59+08:00" \
  --cursor 0 --limit 20 --format json
  
# 9. 我处理过的审批单
dws oa approval list-executed --limit <pageSize> --page <pageNumber> --query 关键词 --format json
# 10. 我发起的审批单 
dws oa approval list-submitted --limit <pageSize> --page <pageNumber> --query 关键词 --format json
# 11. 抄送我的审批单 
dws oa approval list-cc --limit <pageSize> --page <pageNumber> --query 关键词 --format json

# 12. 转交审批任务（taskId 来自 tasks，toActionerId 来自 contact user search）
dws oa approval redirect-task --task-id <taskId> --to-actioner-id <userId> --format json
dws oa approval redirect-task --task-id <taskId> --to-actioner-id <userId> --remark "请帮忙处理" --format json

# 13. 对审批实例添加评论（processInstanceId 来自 list-pending 或 detail）
dws oa approval oa-comments --instance-id <processInstanceId> --content "同意，请尽快处理" --format json

# 14. 对审批实例进行抄送（processInstanceId 来自 list-pending 或 detail）
dws oa approval oa-cc-noticer --instance-id <processInstanceId> --users "68674200835816" --format json
dws oa approval oa-cc-noticer --instance-id <processInstanceId> --users "userId1,userId2" --format json

# 18. 发起审批（完整流程：搜表单 → 查 Schema → 收集表单值 → 流程预测 → 自选节点选人 → 发起）
# 18a. 模糊搜索表单获取 processCode
dws oa approval search-forms --query AI --format json
# 18b. 查询表单 Schema 获取字段定义
dws oa approval form-schema --process-code <code> --format json
# 18c. 收集表单值（向用户展示控件列表，用户填写后组装 form-values）
# 18d. 流程预测（根据表单值预测审批走向，识别自选审批人节点；processCode/deptId/formValues 必填，userId 由登录态自动填充）
dws oa approval forecast-process --process-code <code> --dept-id -1 --form-values '{"单行输入框":"测试内容"}' --format json
# 18e. 若 forecast 返回 targetSelect=true 的节点，用 dws aisearch person --keyword "<姓名>" --dimension name --format json 帮用户选人
# 18f. 发起审批实例（form-values 的 key 须与 Schema 中控件 label 一致）
dws oa approval create-instance --process-code <code> --form-values '{"单行输入框":"测试内容"}' --yes --format json
# 18g. 发起并指定审批人和抄送人
dws oa approval create-instance --process-code <code> --form-values '{"单行输入框":"测试"}' --approvers "userId1,userId2" --approvers-action-type OR --cc-list "userId3" --cc-position START --yes --format json
# 18h. 发起并使用 forecast 自选审批人结果（高级模式）
dws oa approval create-instance --request '{"processCode":"PROC-xxx","deptId":-1,"formComponentValues":[{"name":"单行输入框","value":"测试"}],"targetSelectActioners":[{"actionerKey":"manual_33ff_89cb_da91_e3aa","actionerStaffIds":["userId_选人A"]}]}' --yes --format json
```

## 上下文传递表

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `list-pending` | `processInstanceId` | detail / tasks / records / revoke / oa-comments / oa-cc-noticer 的 --instance-id |
| `tasks` | `taskId` | approve / reject / redirect-task 的 --task-id |
| `detail` | `processCode` | list-initiated 的 --process-code |
| `list-forms` | `processCode` | list-initiated 的 --process-code |
| `search-forms` | `processCode` | list-initiated 的 --process-code |
| `form-schema` | `processCode`, `processName`, `content` | 查看表单结构定义；`content` 字段包含表单组件 JSON，可解析获取字段列表；**控件 label 作为 create-instance --form-values 的 key** |
| `search-forms` → `form-schema` | `processCode` → 表单字段定义 | forecast-process / create-instance 的 --process-code 和 --form-values 填写依据 |
| `forecast-process` | `workflowActivityRuleVOs`（`activityId`, `targetSelect`, `activityActioners`, `workflowActor`） | ① 向用户展示流程走向和各节点处理人；② `targetSelect: true` 的节点需用户自选审批人，`workflowActor.actorKey` 作为 `targetSelectActioners` 的 `actionerKey` 传入 create-instance |
| `search-forms` → `form-schema` → `forecast-process` | `processCode` → 字段定义 → 流程走向 + 自选节点 | create-instance 的完整上下文：表单值 + 流程路径 + targetSelectActioners |
| `create-instance` | `result`（processInstanceId） | detail / tasks / records / revoke 等的 --instance-id，可跟踪已发起的审批 |

## 注意事项

- `--start` / `--end` 使用 ISO-8601 格式（如 2026-03-10T00:00:00+08:00）
- `approve` / `reject` / `redirect-task` 需先通过 `tasks` 获取 `taskId`
- `redirect-task` 的 `--to-actioner-id` 可通过 `dws contact user search` 获取目标用户 userId
- `revoke` 只能撤销自己发起的审批
- `--remark` 审批意见虽为可选，但建议填写以留存审批痕迹
- `list-initiated` 的 `--process-code` 可从 `list-forms`、`search-forms` 或 `detail` 返回中提取。当 `list-forms` 返回 `processCodeList` 为空（`totalCount -1`）时，用 `search-forms --query <表单名>`（如 `--query 报销`）按名称精准拿 `processCode` 更稳
- `list-initiated` 的 `--start` / `--end` 区间有后端上限（约 120 天）。超过上限会返回误导性的 `business_error: 时间戳无效`（实为区间过长，不是时间格式问题）。跨度大时请拆成多段短区间分别查询

- `form-schema` 的 `--process-code` 可从 `list-forms`、`search-forms` 或 `detail` 返回中提取；返回的 `content` 字段为 JSON 字符串，需解析后查看表单组件（items）定义。
- `create-instance` 发起前**必须先阅读** [oa-form-components.md](oa/oa-form-components.md)（控件值格式）和 [oa-process-nodes.md](oa/oa-process-nodes.md)（流程节点规则），再调用 `form-schema` 获取表单字段定义，确保 `--form-values` 中的 key 与控件 label 完全一致。
- `create-instance` 发起前**应先调用 `forecast-process`** 预测流程走向，识别自选审批人节点（`targetSelect: true`），让用户选人后再提交。
- `create-instance` 的 `--form-values` 接受 JSON 格式 `'{"控件名称":"值"}'`，代码会自动转为 `[{"name":"控件名称","value":"值"}]`。
- `create-instance` 简单模式适合常见场景；如需 `directAppointedApprovers`（指定审批人覆盖模板流程）或 `targetSelectActioners`（自选审批节点）等高级字段，使用 `--request` 传完整 JSON。`--request` 与简单模式 flags 互斥。
- `create-instance` 会创建真实审批数据；Agent 只有在用户确认模板、表单值、流程路径和人员后才能传入 `--yes`。
- `create-instance` 返回的 processInstanceId 可用于 `detail`、`tasks`、`records`、`revoke` 等后续操作。
- `forecast-process` 的 `processCode`、`deptId`、`formComponentValues` 三个字段均为必填（`userId` 由系统自动填充）；`formComponentValues` 比 `create-instance` 多一层数组包裹（`[[{...}]]`），CLI 简单模式已自动处理。
- `forecast-process` 返回 `workflowActivityRuleVOs` 中 `targetSelect: true` 的节点，其 `workflowActor.actorKey` 必须作为 `targetSelectActioners` 的 `actionerKey` 传入 `create-instance`。

## 自动化脚本

| 脚本 | 场景 | 用法 |
|------|------|------|
| [oa_pending_review.py](../../scripts/oa_pending_review.py) | 查看待审批列表+逐条显示详情 | `python oa_pending_review.py --days 7` |
| [oa_batch_approve.py](../../scripts/oa_batch_approve.py) | 批量同意/拒绝审批项 | `python oa_batch_approve.py --action approve --days 7` |
