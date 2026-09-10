# 智能合同命令参考

命令前缀统一为 `dws contract`。所有调用追加 `--format json`。命令树已发布到 Agent Schema；参数、约束或确认语义不确定时，优先读取精确 compact leaf Schema，仅在 Schema 不可用或与 Cobra 实际行为发生漂移时读取同一 leaf 的 `--help`。不要使用旧的 dingtalk 二级入口，不要直接调用 MCP、HTTP API 或自行猜测工具名。

## 能力范围与边界

Use when：用户要查询或创建合同台账、批量导入合同、按听记起草合同、发起合同审查、归档合同，或管理合同项目、相对方与收付款账款。

Avoid when：

- 查询、同意、拒绝、转交或撤销已有合同 OA 审批实例走 `dws oa`，不要与合同审批模板 `process-templates` 混淆。
- 经营合约、目标、计分卡和 OKR 跟进走 `dws agoal`。
- 合同文件的搜索、上传、下载及钉盘元数据走 `dws drive`；合同台账、审查和归档仍走 `dws contract`。
- AI 听记内容查询走 `dws minutes`；取得真实 `taskUuid` 后再调用 `dws contract draft`。
- 花名册中的劳动合同等员工基础字段走 `dws contact`。

## 意图路由

| 用户意图 | 命令族 |
|---|---|
| 查合同列表、详情、状态数量、创建台账 | `dws contract record ...` |
| 从钉盘模板批量导入合同、查导入结果 | `dws contract import ...` |
| 查审批模板或台账分类 | `dws contract process-templates` / `dws contract file-directories` |
| 根据 AI 听记和模板起草合同 | `dws contract draft` |
| 解析合同、创建审查任务、查审查结果或权益 | `dws contract review ...` |
| 管理合同项目、导入导出项目 | `dws contract project ...` |
| 管理相对方、工商信息、风险、导入导出 | `dws contract subject ...` |
| 管理合同收付款账款 | `dws contract account ...` |
| 将合同文件归档 | `dws contract archive` |

## 执行规则

1. 先定位真实对象。更新、删除、导出、归档前，先通过对应 `list` / `get` / `detail` 获取真实 ID 和当前状态；禁止根据名称猜 ID。
2. 复杂请求只通过 `--file <json>` 或 `--file -` 传递。提交前检查 JSON 是对象，字段名、必填项和枚举符合本文及当前 leaf Schema；不要把整个请求 JSON 拼成未声明的 flags。
3. 时间单位不得混用：台账、项目及账款列表的筛选日期使用 ISO-8601；账款 JSON 的 `executionDate` 与归档 JSON 的 `archiveTime` 使用 Unix 毫秒时间戳。
4. 异步操作保存创建响应中的真实 `taskId`，再调用配套结果命令。结果未完成时只报告处理中；仅在返回明确可重试状态时轮询，并遵守服务端重试间隔。
5. 删除项目、相对方、账款以及归档等不可逆或高影响操作，在执行前说明对象和影响并获得用户确认；确认后才在 Runtime gate 要求时添加 `--yes`。
6. 创建或更新后优先使用详情查询回读。没有对应详情接口时，保留原始回执并明确说明未能独立回读验证。
7. 列表存在分页参数时按返回继续翻页，直到满足用户范围或服务端表明结束；不要把第一页当成完整结果。

## 标准流程

### 查询合同

1. 用 `record list` 按时间、状态或查询维度缩小范围。
2. 零结果直接说明；多候选先列出名称、状态和合同 ID，让用户消歧。
3. 需要完整字段时，用真实 `contractId` 调用 `record get`。

### 创建台账或归档

1. 查 `file-directories`、`process-templates` 或钉盘文件信息，取得真实目录、模板和文件 ID。
2. 按本文构造 JSON 文件，向用户确认提交对象与关键字段。
3. 创建使用 `record create`；归档使用 `archive`。执行后根据返回的真实 ID 回读或报告状态。

### 合同审查

1. 可先执行 `review benefit` 确认权益。
2. 不确定审查类型或推荐模型时，先用 `review analysis` 解析文件。
3. 用 `review create` 创建任务，保存真实 `taskId` 和 `reviewType`。
4. 用 `review result` 查询结果；未完成不得宣称审查已完成。

### 批量导入

1. 从模板命令或已有钉盘文件取得真实 `fileId` / `spaceId`。
2. 创建导入任务并保存真实 `taskId`。
3. 使用同一命令族的 `*-result` 查询结果，逐项报告成功、失败和处理中状态。

## 命令目录

- [台账](#台账)
- [批量导入](#批量导入)
- [基础资料](#基础资料)
- [合同起草](#合同起草)
- [合同审查](#合同审查)
- [项目管理](#项目管理)
- [相对方管理](#相对方管理)
- [账款管理](#账款管理)
- [合同归档](#合同归档)

## 台账

| 命令 | 必填参数 | 用途 |
|---|---|---|
| `dws contract record list` | 无 | 按创建时间、状态和查询维度筛选合同 |
| `dws contract record get` | `--contract-id` | 查询单份合同详情；别名 `record detail` |
| `dws contract record quantity-by-type` | 无 | 按查询维度统计各状态合同数量 |
| `dws contract record create` | `--file` | 从 JSON 创建合同台账 |

### 查询与统计

`record list` 支持：

- `--start` / `--end`：ISO-8601 合同创建时间；两者同时存在时结束时间不得早于开始时间。
- `--status`：逗号分隔，可选 `approving`、`signing`、`canceled`、`withdraw`、`refused`、`not-archive`、`archive-confirming`、`archived`。
- `--type`：`self`、`participation`、`department`、`all`、`unassigned`，默认 `all`。

`record quantity-by-type` 的 `--type` 与列表相同。

```bash
dws contract record list --type participation --status approving,signing --format json
dws contract record get --contract-id <CONTRACT_ID> --format json
dws contract record quantity-by-type --type department --format json
```

### 创建台账 JSON

`record create --file` 接受 `ImportContractInfoRequest` JSON 对象，必填字段为 `contentFiles`、`name`、`effectiveStatus`、`signStatus`、`ownerDeptNo`。

关键枚举：

- `effectiveStatus`：`not-effective`、`pre-effective`、`effective`、`expired`、`ineffective`、`canceled`。
- `signStatus`：`signing`、`not-archive`、`archived`。
- `amountType`：`payment_party_other`、`payment_party_our`、`none`。
- `signType`：`entity_seal`、`electronic_seal`。
- `termType`：`accurate_end_date`、`perform_finished`。
- `sealTypes`：`contract_seal`、`common_seal`、`legal_seal`。

```bash
dws contract record create --file ./contract.json --format json
```

## 批量导入

| 命令 | 必填参数 | 用途 |
|---|---|---|
| `dws contract import batch` | `--file-id`, `--space-id` | 从钉盘模板文件创建异步合同导入任务 |
| `dws contract import batch-result` | `--task-id` | 查询合同导入结果 |

`--space-id` 可缩写为 `-s`；`--file-id` 不得缩写为 `-f`，因为 `-f` 是全局输出格式。

```bash
dws contract import batch --file-id <FILE_ID> --space-id <SPACE_ID> --format json
dws contract import batch-result --task-id <TASK_ID> --format json
```

## 基础资料

| 命令 | 用途 |
|---|---|
| `dws contract process-templates` | 查询当前用户可见的合同审批模板/流程内容 |
| `dws contract file-directories` | 查询全部合同台账目录；别名 `directories` |

两条命令都没有产品级必填参数。

## 合同起草

`dws contract draft` 根据一条或多条 AI 听记和合同模板起草合同：

- `--task-uuids`：必填，听记任务 ID，多个值用英文逗号分隔。
- `--template-url` / `--template-content`：至少提供一个。

听记 ID 必须来自 `dingtalk-minutes` 的真实返回。模板正文较长时，应从可信本地文件读取后传入，不要让 shell 对内容做意外展开。

```bash
dws contract draft --task-uuids <TASK_UUIDS> --template-url <TEMPLATE_URL> --format json
```

## 合同审查

| 命令 | 必填参数 | 用途 |
|---|---|---|
| `dws contract review benefit` | 无 | 查询组织的合同审查权益 |
| `dws contract review analysis` | `--file` | 解析合同文件并返回摘要及推荐模型 |
| `dws contract review create` | `--file` | 创建合同审查任务 |
| `dws contract review result` | `--task-id`, `--review-type` | 查询审查结果 |

### analysis 请求

JSON 可包含 `fileInfo` 对象，常用字段为 `fileId`、`spaceId`、`fileName`、`fileSize`、`fileType`；可选 `source`。文件 ID 必须来自钉盘真实返回。

### create 请求

`IntelligentContractReviewClientRequest` 常用字段：

- `source`
- `fileInfo`：`fileId`、`spaceId`、`fileName`、`fileSize`、`fileType`
- `reviewType`
- `companyList[].reviewPosition`
- `reviewPosition`
- `reviewResultType`
- `customReviewRules`

```bash
dws contract review benefit --format json
dws contract review analysis --file ./analysis_request.json --format json
dws contract review create --file ./review_request.json --format json
dws contract review result --task-id <TASK_ID> --review-type <REVIEW_TYPE> --format json
```

## 项目管理

| 命令 | 必填参数 | 用途 |
|---|---|---|
| `dws contract project add` | `--name` | 创建合同项目 |
| `dws contract project update` | `--project-id`, `--name` | 更新项目 |
| `dws contract project set-status` | `--project-id`, `--status` | 更新项目状态 |
| `dws contract project list` | `--current-page`, `--page-size`, `--scope` | 分页查询项目 |
| `dws contract project digests` | `--current-page`, `--page-size`, `--scope` | 分页查询项目摘要 |
| `dws contract project detail` | `--project-id` | 查询项目详情 |
| `dws contract project delete` | `--project-ids` | 删除一个或多个项目 |
| `dws contract project export` | `--project-ids` | 导出项目到 Excel |
| `dws contract project import-template` | 无 | 获取项目导入模板链接 |
| `dws contract project import` | `--file-id` | 从钉盘文件创建项目导入任务 |
| `dws contract project import-result` | `--task-id` | 查询项目导入结果 |

### 项目字段

- 新增/更新：`--code`、`--owners`、`--start-date`、`--end-date`、`--remark`、`--contract-ids`；新增还支持 `--source`。
- 日期使用 ISO-8601；开始和结束同时提供时，结束日期不得早于开始日期。
- 列表/摘要：`--scope` 取 `self` 或 `all`；筛选支持 `--name`、`--code`、`--owners`、`--status`、四个日期区间参数。
- 导出可选 `--process-code`。
- 导入可选 `--space-id`、`--file-name`、`--file-type`、`--file-size`。

```bash
dws contract project list --current-page 1 --page-size 20 --scope all --format json
dws contract project detail --project-id <PROJECT_ID> --format json
dws contract project add --name <PROJECT_NAME> --format json
dws contract project import --file-id <FILE_ID> --space-id <SPACE_ID> --format json
dws contract project import-result --task-id <TASK_ID> --format json
```

## 相对方管理

| 命令 | 必填参数 | 用途 |
|---|---|---|
| `dws contract subject add` | `--file` | 添加相对方 |
| `dws contract subject update` | `--file` | 更新相对方 |
| `dws contract subject list` | `--current-page`, `--page-size` | 分页查询相对方 |
| `dws contract subject detail` | `--subject-id` | 查询相对方详情 |
| `dws contract subject delete` | `--subject-id` | 删除单个相对方 |
| `dws contract subject batch-delete` | `--subject-ids` | 批量删除相对方，最多 1000 个 |
| `dws contract subject sort` | `--subject-ids` | 按传入顺序排列己方主体 |
| `dws contract subject detect-risk` | `--subject-name` | 检测相对方风险；可选 `--subject-id` |
| `dws contract subject base-info` | `--subject-name` | 查询工商基本信息；可选 `--subject-id` |
| `dws contract subject auto-fill` | `--subject-name` | 智能填充相对方信息；可选 `--subject-id` |
| `dws contract subject export` | `--subject-ids` | 导出相对方到 Excel |
| `dws contract subject import-template` | 无 | 获取相对方导入模板；可选 `--type` |
| `dws contract subject import` | `--file-id` | 从钉盘文件创建相对方导入任务 |
| `dws contract subject import-result` | `--task-id` | 查询相对方导入结果 |

### 相对方 JSON

`subject add` 接受 `AddSubjectOpenRequest`，至少包含 `partyType` 和 `name`。`partyType` 取 `other`（对方）或 `our`（己方）；`source` 常用 `contract` 或 `oa`；`bankAccountType` 取 `BUSINESS_ACCOUNT` 或 `PERSONAL_ACCOUNT`。

`subject update` 接受 `UpdateSubjectOpenRequest`，至少包含真实 `subjectId`、`partyType` 和 `name`。更新前先执行 `subject detail`，在原数据基础上修改，避免覆盖未知字段。

### 查询、导入与导出

- 列表可选 `--party-type`、`--name`、`--code`、`--source`。
- 导出可选 `--process-code`。
- 导入模板 `--type` 可取 `other` 或 `our`。
- 导入可选 `--space-id`、`--file-name`、`--file-type`、`--file-size`。

```bash
dws contract subject list --current-page 1 --page-size 20 --party-type other --format json
dws contract subject detail --subject-id <SUBJECT_ID> --format json
dws contract subject add --file ./subject.json --format json
dws contract subject detect-risk --subject-name <SUBJECT_NAME> --format json
dws contract subject import --file-id <FILE_ID> --space-id <SPACE_ID> --format json
dws contract subject import-result --task-id <TASK_ID> --format json
```

删除前必须先用 `subject detail` 展示真实相对方名称与 ID；批量删除还要展示总数和全部目标 ID，获得确认后再执行。

## 账款管理

| 命令 | 必填参数 | 用途 |
|---|---|---|
| `dws contract account create` | `--file` | 创建账款信息 |
| `dws contract account update` | `--file` | 更新账款信息 |
| `dws contract account get` | `--account-id` | 查询单条账款 |
| `dws contract account list` | 无 | 按条件分页查询账款 |
| `dws contract account delete` | `--account-id` | 删除账款 |

### 账款 JSON

创建请求至少包含：

- `contractId`：真实合同 ID。
- `amount`：以元为单位的字符串金额，例如 `1234.56`。
- `transactionNo`：不可重复的单据号。
- `executionDate`：Unix 毫秒时间戳。
- `status`：`approving`、`withdraw`、`refused`、`confirming`、`canceled`、`finished` 之一。

可选字段包括 `reimbursementNo`、`currencyCode`（默认 `CNY`）、`source`、`remark`。更新请求还必须包含真实 `accountEntryId`；应先执行 `account get`，修改原数据后再提交。

### 账款列表

- `--scope`：`self`、`department`、`all`。
- `--query-status`：`all`、`pay`、`receive`。
- `--amount-type`：`payment_party_other`、`payment_party_our`、`none`。
- 其他筛选：`--status`、`--source`、`--contract-code`、`--contract-name`、`--transaction-no`。
- `--exec-start` / `--exec-end`：ISO-8601 时间字符串；CLI 转换为 MCP 所需的 Unix 毫秒时间戳。
- 分页：`--page`、`--page-size`。

```bash
dws contract account list --scope self --page 1 --page-size 20 --format json
dws contract account get --account-id <ACCOUNT_ID> --format json
dws contract account create --file ./account.json --format json
```

删除账款前必须先执行 `account get`，向用户展示账款 ID、合同 ID、金额、单据号和状态并获得确认。

## 合同归档

`dws contract archive --file` 接受 `ContractOpenArchiveRequest` JSON 对象：

- `bizId`：合同唯一标识，必填。
- `archiveTime`：Unix 毫秒时间戳，必填。
- `archiveFiles`：必填数组；每项可包含 `spaceId`、`fileId`、`fileName`、`fileType`、`fileSize`。
- `archiveCode`、`archiveComment`、`fileDirectoryId`：可选。

归档前通过 `record get` 核对合同，通过钉盘能力核对每个文件 ID，并在确认信息中列出合同、文件数量、目录和归档时间。

```bash
dws contract archive --file ./archive_request.json --format json
```

## 命令与 MCP 能力映射

仅用于诊断和维护，不应绕过 CLI 直接调用：

| CLI 命令 | MCP 工具 |
|---|---|
| `record list` / `get` / `quantity-by-type` / `create` | `queryContracts` / `queryContractDetails` / `queryContractQuantityByType` / `createContract` |
| `import batch` / `batch-result` | `batchImportContractAsync` / `getBatchImportContractResult` |
| `process-templates` / `file-directories` / `draft` | `queryContractProcessContent` / `getAllFileDirectory` / `draft_contract_by_minutes` |
| `review benefit` / `create` / `analysis` / `result` | `queryContractReviewBenefit` / `createContractReviewTask` / `contractAnalysis` / `queryContractReviewResult` |
| `project ...` | `addProject`, `deleteProject`, `updateProject`, `setProjectStatus`, `queryProjects`, `queryProjectDigests`, `queryProjectDetail`, `exportProject`, `getImportProjectTemplate`, `importProject`, `getImportProjectResult` |
| `subject ...` | `addSubject`, `querySubjects`, `querySubjectDetail`, `updateSubject`, `deleteSubject`, `batchDeleteSubject`, `sortSubjects`, `detectSubjectRisk`, `querySubjectBaseInfo`, `autoFillSubjectInfo`, `exportSubject`, `getImportSubjectTemplate`, `importSubject`, `getImportSubjectResult` |
| `account ...` / `archive` | `createAccountInfo`, `updateAccountInfo`, `getAccountEntryInfo`, `listAccountInfo`, `deleteAccountEntryInfo`, `contractOpenArchive` |
