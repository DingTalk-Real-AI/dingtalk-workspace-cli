# 智能合同 (contract) 命令参考

智能合同：**台账**（列表、详情、按查询维度统计各状态数量、创建）、**钉盘批量导入**、**审批模板与台账分类**、**听记 + 模版起草**、**合同审查**（权益、任务、解析、结果）、**项目管理**（增删改查、状态变更、导入导出）、**相对方管理**（增删改查、风险检测、工商信息、智能填充、排序、导入导出）。实现为 Cobra 子命令，经 DWS 调用 MCP 服务 `contract` 上的工具。

| 项目 | 说明 |
|------|------|
| **推荐入口** | `dws dingtalk contract` |
| **实现源码** | [`contract.go`](../../../wukong/extensions/vendors/dingtalk/contract.go) |

> 全局输出形态与其它 `dws` 子命令一致：常用 `--format json` 获取结构化结果；若根命令支持 dry-run 类行为，可仅打印将调用的工具与参数。

## MCP 工具对照

| CLI | MCP 工具名 |
|-----|------------|
| `record list` | `queryContracts` |
| `record get` / `record detail` | `queryContractDetails` |
| `record quantity-by-type` | `queryContractQuantityByType` |
| `record create` | `createContract` |
| `import batch` | `batchImportContractAsync` |
| `import batch-result` | `getBatchImportContractResult` |
| `process-templates` | `queryContractProcessContent` |
| `file-directories` / `directories` | `getAllFileDirectory` |
| `draft` | `draft_contract_by_minutes` |
| `review benefit` | `queryContractReviewBenefit` |
| `review create` | `createContractReviewTask` |
| `review analysis` | `contractAnalysis` |
| `review result` | `queryContractReviewResult` |
| `project add` | `addProject` |
| `project delete` | `deleteProject` |
| `project update` | `updateProject` |
| `project set-status` | `setProjectStatus` |
| `project list` | `queryProjects` |
| `project digests` | `queryProjectDigests` |
| `project detail` | `queryProjectDetail` |
| `project export` | `exportProject` |
| `project import-template` | `getImportProjectTemplate` |
| `project import` | `importProject` |
| `project import-result` | `getImportProjectResult` |
| `subject add` | `addSubject` |
| `subject list` | `querySubjects` |
| `subject detail` | `querySubjectDetail` |
| `subject update` | `updateSubject` |
| `subject delete` | `deleteSubject` |
| `subject batch-delete` | `batchDeleteSubject` |
| `subject sort` | `sortSubjects` |
| `subject detect-risk` | `detectSubjectRisk` |
| `subject base-info` | `querySubjectBaseInfo` |
| `subject auto-fill` | `autoFillSubjectInfo` |
| `subject export` | `exportSubject` |
| `subject import-template` | `getImportSubjectTemplate` |
| `subject import` | `importSubject` |
| `subject import-result` | `getImportSubjectResult` |

## 命令总览

### record（合同记录 / 台账）

#### 查询合同列表
```
Usage:
  dws dingtalk contract record list [flags]
Example:
  dws dingtalk contract record list --format json
  dws dingtalk contract record list --start "2026-03-10T00:00:00+08:00" --end "2026-03-11T23:59:59+08:00" --status approving,signing --format json
  dws dingtalk contract record list --type participation --format json
Flags:
      --start string          合同创建时间范围起点（ISO-8601）
      --end string            合同创建时间范围终点（ISO-8601，须晚于 --start）
      --status string         合同状态，英文枚举，逗号分隔（可多选）
      --type string           台账查询维度，默认 all（与 MCP queryContracts 的 type 一致）；见下文
```

按合同**创建时间**、**状态**、以及 **查询维度 `--type`** 筛选；入参与 `queryContracts` 一致。

**`--type` 查询维度**（英文小写，CLI 大小写不敏感）：`self`（我的）、`participation`（我参与的）、`department`（我部门的）、`all`（全部，默认）、`unassigned`（待分配的）。非法取值会在 CLI 侧拒绝。

**`--status` 英文枚举**（可多选）：`approving`, `signing`, `canceled`, `withdraw`, `refused`, `not-archive`, `archive-confirming`, `archived`。

#### 查询合同详情
```
Usage:
  dws dingtalk contract record get [flags]
  dws dingtalk contract record detail [flags]
Example:
  dws dingtalk contract record get --contract-id "c_xxx" --format json
  dws dingtalk contract record detail --contract-id "c_xxx" --format json
Flags:
      --contract-id string   合同 ID（必填，对应 MCP queryContractDetails 的 contractId）
```

`get` 与 `detail` 为别名；与台账列表/详情页中的合同主键一致。

#### 按查询维度统计各状态合同数量
```
Usage:
  dws dingtalk contract record quantity-by-type [flags]
Example:
  dws dingtalk contract record quantity-by-type --format json
  dws dingtalk contract record quantity-by-type --type department --format json
Flags:
      --type string   台账查询维度，默认 all（与 MCP queryContractQuantityByType 的 type 一致；取值同 record list）
```

入参与 `queryContractQuantityByType` 一致；`--type` 含义与上文「查询合同列表」一节相同。

#### 创建合同台账
```
Usage:
  dws dingtalk contract record create [flags]
Example:
  dws dingtalk contract record create --file ./contract.json --format json
  cat contract.json | dws dingtalk contract record create --file - --format json
Flags:
      --file string   ImportContractInfoRequest JSON 文件路径，"-" 表示 stdin（必填）
```

将合同文件与关键信息写入台账（`createContract`）。JSON 须符合 **`ImportContractInfoRequest`**；代码注释中的**必填**字段：`contentFiles`, `name`, `effectiveStatus`, `signStatus`, `ownerDeptNo`。

**枚举参考**（与实现注释一致）：

- **effectiveStatus（履约状态）**：`not-effective`(未生效), `pre-effective`(待生效), `effective`(生效中), `expired`(已到期), `ineffective`(已完结), `canceled`(已作废)
- **signStatus（签署状态）**：`signing`(签订中), `not-archive`(待归档), `archived`(已归档)
- **amountType（金额类型）**：`payment_party_other`(收入), `payment_party_our`(支出), `none`(无金额)
- **signType（签署方式）**：`entity_seal`(纸质签署), `electronic_seal`(电子签署)
- **termType（期限类型）**：`accurate_end_date`(固定期限), `perform_finished`(无固定期限)
- **sealTypes（印章类型）**：`contract_seal`(合同章), `common_seal`(公章), `legal_seal`(法人章)

完整字段以服务端定义为准。

### import（批量导入）

#### 从钉盘模版文件创建批量导入任务
```
Usage:
  dws dingtalk contract import batch [flags]
Example:
  dws dingtalk contract import batch --file-id "123456" --space-id "7890" --format json
  dws dingtalk contract import batch --file-id "123456" -s "7890" --format json
Flags:
      --file-id string    钉盘批量导入模版文件的 fileId（必填；勿用 -f，与全局 --format 冲突）
  -s, --space-id string   模版文件所在钉盘空间的 spaceId（必填）
```

异步任务（`batchImportContractAsync`）；仅需钉盘上模版文件的 `fileId` 与 `spaceId`。

#### 获取批量合同导入任务结果
```
Usage:
  dws dingtalk contract import batch-result [flags]
Example:
  dws dingtalk contract import batch-result --task-id "task_xxx" --format json
Flags:
      --task-id string   批量导入任务 ID（必填，对应 getBatchImportContractResult 的 taskId）
```

### process-templates（审批模板）

#### 查询当前用户可见审批模板
```
Usage:
  dws dingtalk contract process-templates [flags]
Example:
  dws dingtalk contract process-templates --format json
Flags:
      （无业务必填参数）
```

对应 `queryContractProcessContent`。

### file-directories（台账分类）

#### 查询所有合同台账分类
```
Usage:
  dws dingtalk contract file-directories [flags]
  dws dingtalk contract directories [flags]
Example:
  dws dingtalk contract file-directories --format json
  dws dingtalk contract directories --format json
Flags:
      （无业务必填参数）
```

`directories` 为别名；对应 `getAllFileDirectory`。

### draft（听记 + 模版起草）

#### 根据听记和模版起草合同
```
Usage:
  dws dingtalk contract draft [flags]
Example:
  dws dingtalk contract draft --task-uuids uuid1,uuid2 --template-url "https://..." --format json
  dws dingtalk contract draft --task-uuids uuid1 --template-content "$(cat 模版.txt)" --format json
Flags:
      --task-uuids string        听记任务 id 列表，逗号分隔（必填）
      --template-url string      合同模版 URL（与 --template-content 至少一项）
      --template-content string  合同模版全文（与 --template-url 至少一项）
```

对应 `draft_contract_by_minutes`。听记 id 取自 `bizType` 为 `flashMinutes` 的 `fileUri` 或 `id`；支持多个听记合并。**模版二选一至少填一项**（`templateUrl` / `templateContent`）。

### review（合同审查）

#### 查询合同审查权益
```
Usage:
  dws dingtalk contract review benefit [flags]
Example:
  dws dingtalk contract review benefit --format json
Flags:
      （无业务必填参数）
```

对应 `queryContractReviewBenefit`。

#### 创建合同审查任务
```
Usage:
  dws dingtalk contract review create [flags]
Example:
  dws dingtalk contract review create --file ./review_request.json --format json
  cat review_request.json | dws dingtalk contract review create --file - --format json
Flags:
      --file string   IntelligentContractReviewClientRequest JSON 路径，"-" 表示 stdin（必填）
```

对应 `createContractReviewTask`；请求体须符合 **`IntelligentContractReviewClientRequest`**。

**字段摘要**（与实现注释一致）：`source`（可选）；`fileInfo`（`fileId`, `spaceId`, `fileName` 须带扩展名, `fileSize`, `fileType`）；`reviewType`；`companyList`（可含 `reviewPosition`）；`reviewPosition`；`reviewResultType`；`customReviewRules`。

示例 JSON：

```json
{
  "source": "OPEN_CLAW",
  "fileInfo": {
    "fileId": "xxx",
    "spaceId": "yyy",
    "fileName": "采购合同.pdf",
    "fileSize": "102400",
    "fileType": "pdf"
  },
  "reviewType": "AI_REVIEW",
  "reviewPosition": "甲方",
  "reviewResultType": "standard",
  "companyList": [{"reviewPosition": "乙方"}]
}
```

#### 解析合同文件
```
Usage:
  dws dingtalk contract review analysis [flags]
Example:
  dws dingtalk contract review analysis --file ./analysis_request.json --format json
  cat analysis_request.json | dws dingtalk contract review analysis --file - --format json
Flags:
      --file string   contractAnalysis 请求 JSON 路径，"-" 表示 stdin（必填）
```

对应 `contractAnalysis`；服务端包装为 **`AnalysisContractApiRequest`**。

示例 JSON：

```json
{
  "fileInfo": {
    "fileId": "xxx",
    "spaceId": "yyy",
    "fileName": "采购合同.pdf",
    "fileSize": "102400",
    "fileType": "pdf"
  }
}
```

#### 查询合同审查结果
```
Usage:
  dws dingtalk contract review result [flags]
Example:
  dws dingtalk contract review result --task-id "MjIzODAwMkFJX1JFVklFVw==" --review-type AI_REVIEW --format json
Flags:
      --task-id string      审查任务 ID（必填，通常由 review create 返回）
      --review-type string  审查类型，如 AI_REVIEW（必填）
```

对应 `queryContractReviewResult`；参数包在 `IntelligentLegalContractReviewClientRequest` 下。

### project（合同项目管理）

#### 新增项目
```
Usage:
  dws dingtalk contract project add [flags]
Example:
  dws dingtalk contract project add --name "2024采购项目" --format json
  dws dingtalk contract project add --name "Q1项目" --code "PRJ-001" --owners "staff1,staff2" --start-date "2026-01-01T00:00:00+08:00" --end-date "2026-12-31T23:59:59+08:00" --format json
Flags:
      --name string           项目名称（必填）
      --code string           项目编码
      --owners string         负责人 staffId 列表，逗号分隔
      --start-date string     开始日期（ISO-8601，如 2026-03-10T14:00:00+08:00）
      --end-date string       结束日期（ISO-8601，须晚于 --start-date）
      --remark string         备注
      --contract-ids string   关联合同 ID 列表，逗号分隔
      --source string         来源
```

对应 `addProject`；仅 `--name` 为必填，其余均可选。未指定 `--owners` 时服务端默认以当前操作人为负责人。

#### 删除项目（支持批量）
```
Usage:
  dws dingtalk contract project delete [flags]
Example:
  dws dingtalk contract project delete --project-ids "1001,1002" --format json
Flags:
      --project-ids string   项目 ID 列表，逗号分隔（必填）
```

对应 `deleteProject`；支持一次传多个 ID 批量删除。

#### 更新项目信息
```
Usage:
  dws dingtalk contract project update [flags]
Example:
  dws dingtalk contract project update --project-id 1001 --name "更新后的名称" --format json
Flags:
      --project-id int        项目 ID（必填）
      --name string           项目名称（必填）
      --code string           项目编码
      --owners string         负责人 staffId 列表，逗号分隔
      --start-date string     开始日期（ISO-8601，如 2026-03-10T14:00:00+08:00）
      --end-date string       结束日期（ISO-8601，须晚于 --start-date）
      --remark string         备注
      --contract-ids string   关联合同 ID 列表，逗号分隔
```

对应 `updateProject`；`--project-id` 与 `--name` 为必填。

#### 更新项目状态
```
Usage:
  dws dingtalk contract project set-status [flags]
Example:
  dws dingtalk contract project set-status --project-id 1001 --status "active" --format json
Flags:
      --project-id int     项目 ID（必填）
      --status string      项目状态（必填）
```

对应 `setProjectStatus`。

#### 分页查询项目列表
```
Usage:
  dws dingtalk contract project list [flags]
Example:
  dws dingtalk contract project list --current-page 1 --page-size 20 --scope all --format json
  dws dingtalk contract project list --current-page 1 --page-size 10 --scope self --name "采购" --status active --format json
Flags:
      --current-page int          当前页码（必填）
      --page-size int             每页条数（必填）
      --scope string              查询范围：self(我负责的)/all(所有项目)（必填）
      --name string               项目名称（模糊搜索）
      --code string               项目编码
      --owners string             负责人 staffId 列表，逗号分隔
      --status string             项目状态
      --start-date-left string    开始日期左区间（ISO-8601，如 2026-01-01T00:00:00+08:00）
      --start-date-right string   开始日期右区间（ISO-8601）
      --end-date-left string      结束日期左区间（ISO-8601）
      --end-date-right string     结束日期右区间（ISO-8601）
```

对应 `queryProjects`；`--current-page`、`--page-size`、`--scope` 为必填，其余可选筛选。

#### 分页查询项目摘要列表
```
Usage:
  dws dingtalk contract project digests [flags]
Example:
  dws dingtalk contract project digests --current-page 1 --page-size 20 --scope all --format json
Flags:
      （参数同 project list）
```

对应 `queryProjectDigests`；入参与 `project list` 完全一致，返回精简的摘要字段。

#### 查询项目详情
```
Usage:
  dws dingtalk contract project detail [flags]
Example:
  dws dingtalk contract project detail --project-id 1001 --format json
Flags:
      --project-id int   项目 ID（必填）
```

对应 `queryProjectDetail`。

#### 项目导出到 Excel
```
Usage:
  dws dingtalk contract project export [flags]
Example:
  dws dingtalk contract project export --project-ids "1001,1002" --format json
Flags:
      --project-ids string    项目 ID 列表，逗号分隔（必填）
      --process-code string   审批模板 code（可选）
```

对应 `exportProject`。

#### 获取批量导入项目模板
```
Usage:
  dws dingtalk contract project import-template [flags]
Example:
  dws dingtalk contract project import-template --format json
Flags:
      （无业务必填参数）
```

对应 `getImportProjectTemplate`；返回 Excel 模板下载链接。

#### 批量导入项目
```
Usage:
  dws dingtalk contract project import [flags]
Example:
  dws dingtalk contract project import --file-id "abc123" --space-id 7890 --format json
Flags:
      --file-id string     钉盘文件 ID（必填）
      --space-id int       钉盘空间 ID
      --file-name string   文件名称
      --file-type string   文件类型
      --file-size int      文件大小（字节）
```

对应 `importProject`；从钉盘文件批量导入。

#### 获取项目批量导入结果
```
Usage:
  dws dingtalk contract project import-result [flags]
Example:
  dws dingtalk contract project import-result --task-id "task_xxx" --format json
Flags:
      --task-id string   导入任务 ID（必填）
```

对应 `getImportProjectResult`。

### subject（合同相对方管理）

#### 添加相对方
```
Usage:
  dws dingtalk contract subject add [flags]
Example:
  dws dingtalk contract subject add --file ./subject.json --format json
  cat subject.json | dws dingtalk contract subject add --file - --format json
Flags:
      --file string   AddSubjectOpenRequest JSON 文件路径，"-" 表示 stdin（必填）
```

对应 `addSubject`；字段较多，推荐通过 `--file` 传入 JSON。

**枚举值说明**：
- **partyType**：`other`（对方）、`our`（己方）
- **bankAccountType**：`BUSINESS_ACCOUNT`（对公）、`PERSONAL_ACCOUNT`（个人）
- **source**：`contract`（智能合同）、`oa`（OA 审批）

示例 JSON：

```json
{
  "partyType": "other",
  "name": "北京示例科技有限公司",
  "ucsi": "91110108MA01XXXXX",
  "legalPerson": "张三",
  "tags": ["供应商", "战略合作"]
}
```

#### 查询相对方列表
```
Usage:
  dws dingtalk contract subject list [flags]
Example:
  dws dingtalk contract subject list --current-page 1 --page-size 20 --format json
  dws dingtalk contract subject list --current-page 1 --page-size 10 --party-type other --name "科技" --format json
Flags:
      --current-page int    当前页码（必填）
      --page-size int       每页条数（必填）
      --party-type string   相对方类型：other(对方)/our(己方)
      --name string         相对方名称（模糊匹配）
      --code string         主体编号
      --source string       来源：contract/oa
```

对应 `querySubjects`；`--current-page`、`--page-size` 为必填，其余可选筛选。

#### 查询相对方详情
```
Usage:
  dws dingtalk contract subject detail [flags]
Example:
  dws dingtalk contract subject detail --subject-id 2001 --format json
Flags:
      --subject-id int   相对方 ID（必填）
```

对应 `querySubjectDetail`。

#### 修改相对方
```
Usage:
  dws dingtalk contract subject update [flags]
Example:
  dws dingtalk contract subject update --file ./subject_update.json --format json
Flags:
      --file string   UpdateSubjectOpenRequest JSON 文件路径，"-" 表示 stdin（必填）
```

对应 `updateSubject`；JSON 中须包含 `subjectId`、`partyType`、`name` 等必填字段。

示例 JSON：

```json
{
  "subjectId": 2001,
  "partyType": "other",
  "name": "北京示例科技有限公司（更新）",
  "remark": "信息变更"
}
```

#### 删除相对方（单个）
```
Usage:
  dws dingtalk contract subject delete [flags]
Example:
  dws dingtalk contract subject delete --subject-id 2001 --format json
Flags:
      --subject-id int   相对方 ID（必填）
```

对应 `deleteSubject`。

#### 批量删除相对方
```
Usage:
  dws dingtalk contract subject batch-delete [flags]
Example:
  dws dingtalk contract subject batch-delete --subject-ids "2001,2002,2003" --format json
Flags:
      --subject-ids string   相对方 ID 列表，逗号分隔（必填，最多 1000 个）
```

对应 `batchDeleteSubject`。

#### 己方主体排序
```
Usage:
  dws dingtalk contract subject sort [flags]
Example:
  dws dingtalk contract subject sort --subject-ids "2001,2003,2002" --format json
Flags:
      --subject-ids string   己方主体 ID 列表，逗号分隔，按期望顺序排列（必填）
```

对应 `sortSubjects`；设置己方主体的展示顺序。

#### 检测相对方风险
```
Usage:
  dws dingtalk contract subject detect-risk [flags]
Example:
  dws dingtalk contract subject detect-risk --subject-name "北京示例科技有限公司" --format json
Flags:
      --subject-name string   相对方名称（必填）
      --subject-id int        相对方 ID（可选）
```

对应 `detectSubjectRisk`。

#### 查询相对方工商基本信息
```
Usage:
  dws dingtalk contract subject base-info [flags]
Example:
  dws dingtalk contract subject base-info --subject-name "北京示例科技有限公司" --format json
Flags:
      --subject-name string   相对方名称（必填）
      --subject-id int        相对方 ID（可选）
```

对应 `querySubjectBaseInfo`。

#### 相对方信息智能填充
```
Usage:
  dws dingtalk contract subject auto-fill [flags]
Example:
  dws dingtalk contract subject auto-fill --subject-name "北京示例科技有限公司" --format json
Flags:
      --subject-name string   相对方名称（必填）
      --subject-id int        相对方 ID（可选）
```

对应 `autoFillSubjectInfo`；根据相对方名称智能填充详细信息。

#### 导出相对方到 Excel
```
Usage:
  dws dingtalk contract subject export [flags]
Example:
  dws dingtalk contract subject export --subject-ids "2001,2002" --format json
Flags:
      --subject-ids string    相对方 ID 列表，逗号分隔（必填）
      --process-code string   审批模板 code（可选）
```

对应 `exportSubject`。

#### 获取相对方批量导入模板
```
Usage:
  dws dingtalk contract subject import-template [flags]
Example:
  dws dingtalk contract subject import-template --format json
  dws dingtalk contract subject import-template --type other --format json
Flags:
      --type string   相对方类型：other/our（可选）
```

对应 `getImportSubjectTemplate`；返回 Excel 模板下载链接。

#### 批量导入相对方
```
Usage:
  dws dingtalk contract subject import [flags]
Example:
  dws dingtalk contract subject import --file-id "abc123" --space-id 7890 --format json
Flags:
      --file-id string     钉盘文件 ID（必填）
      --space-id int       钉盘空间 ID
      --file-name string   文件名称
      --file-type string   文件类型
      --file-size int      文件大小（字节）
```

对应 `importSubject`；从钉盘文件批量导入。

#### 查询相对方批量导入结果
```
Usage:
  dws dingtalk contract subject import-result [flags]
Example:
  dws dingtalk contract subject import-result --task-id "task_xxx" --format json
Flags:
      --task-id string   导入任务 ID（必填）
```

对应 `getImportSubjectResult`。

## 意图判断

用户说「合同台账 / 合同列表 / 查合同」：

- 列表、按时间或状态筛选 → `record list`
- 单条详情 → `record get`（需 `contractId`）
- 按查询维度统计各状态数量 → `record quantity-by-type`（`--type` 同列表）
- 写入台账 → `record create`（JSON 文件）

用户说「批量导入合同 / 模版导入」：

- 发起任务 → `import batch`（钉盘 `fileId` + `spaceId`）
- 查结果 → `import batch-result`（`taskId`）

用户说「审批流程 / 合同审批模板」→ `process-templates`
用户说「台账分类 / 合同目录」→ `file-directories`

用户说「听记起草合同 / 会议纪要生成合同」→ `draft`（`task-uuids` + 模版 URL 或全文）；听记 id 获取可参考 `dingtalk-minutes/references/minutes.md`。

用户说「合同审查 / AI 审合同」：

- 权益 → `review benefit`
- 建任务 → `review create`
- 仅解析摘要 → `review analysis`
- 拉结果 → `review result`（需 `taskId` + `reviewType`）

用户说「合同项目 / 项目管理 / 查项目 / 新建项目」：

- 查列表 → `project list`（需 `--current-page`、`--page-size`、`--scope`）
- 查摘要 → `project digests`（参数同 `project list`，返回精简字段）
- 查详情 → `project detail`（需 `--project-id`）
- 新建 → `project add`（需 `--name`）
- 更新 → `project update`（需 `--project-id` + `--name`）
- 改状态 → `project set-status`（需 `--project-id` + `--status`）
- 删除 → `project delete`（需 `--project-ids`）
- 导出 → `project export`（需 `--project-ids`）
- 批量导入 → 先 `project import-template` 获取模板 → `project import`（`--file-id`）→ `project import-result`（`--task-id`）

用户说「合同相对方 / 相对方管理 / 签约方 / 对方信息 / 甲方乙方」：

- 查列表 → `subject list`（需 `--current-page`、`--page-size`；可选 `--party-type`）
- 查详情 → `subject detail`（需 `--subject-id`）
- 新增 → `subject add`（`--file` JSON；必含 `partyType` + `name`）
- 修改 → `subject update`（`--file` JSON；必含 `subjectId`）
- 删除（单个）→ `subject delete`（需 `--subject-id`）
- 删除（批量）→ `subject batch-delete`（需 `--subject-ids`）
- 排序 → `subject sort`（需 `--subject-ids`，按期望顺序）
- 风险检测 → `subject detect-risk`（需 `--subject-name`）
- 工商信息 → `subject base-info`（需 `--subject-name`）
- 智能填充 → `subject auto-fill`（需 `--subject-name`）
- 导出 → `subject export`（需 `--subject-ids`）
- 批量导入 → 先 `subject import-template` 获取模板 → `subject import`（`--file-id`）→ `subject import-result`（`--task-id`）

钉盘 `fileId` / `spaceId` 的取得可参考 `dingtalk-drive/references/drive.md`。

## 核心工作流

**台账查询**

```bash
dws dingtalk contract record list --status approving,signing --format json
dws dingtalk contract record get --contract-id "<CONTRACT_ID>" --format json
```

**批量导入（异步）**

```bash
dws dingtalk contract import batch --file-id "<FILE_ID>" --space-id "<SPACE_ID>" --format json
# 从返回中取 taskId
dws dingtalk contract import batch-result --task-id "<TASK_ID>" --format json
```

**审查（异步）**

```bash
dws dingtalk contract review create --file ./review_request.json --format json
# 从返回中取 taskId，与 reviewType 一并查询
dws dingtalk contract review result --task-id "<TASK_ID>" --review-type AI_REVIEW --format json
```

**项目管理（CRUD + 导入导出）**

```bash
# 新建项目
dws dingtalk contract project add --name "2024采购项目" --format json
# 查列表，取 projectId
dws dingtalk contract project list --current-page 1 --page-size 20 --scope all --format json
# 查详情
dws dingtalk contract project detail --project-id <PROJECT_ID> --format json
# 更新
dws dingtalk contract project update --project-id <PROJECT_ID> --name "更新后名称" --format json
# 改状态
dws dingtalk contract project set-status --project-id <PROJECT_ID> --status active --format json
# 删除
dws dingtalk contract project delete --project-ids "<PROJECT_ID>" --format json
# 批量导入
dws dingtalk contract project import-template --format json
dws dingtalk contract project import --file-id "<FILE_ID>" --format json
dws dingtalk contract project import-result --task-id "<TASK_ID>" --format json
```

**相对方管理（CRUD + 风险检测 + 导入导出）**

```bash
# 新增相对方
dws dingtalk contract subject add --file ./subject.json --format json
# 查列表，取 subjectId
dws dingtalk contract subject list --current-page 1 --page-size 20 --party-type other --format json
# 查详情
dws dingtalk contract subject detail --subject-id <SUBJECT_ID> --format json
# 修改
dws dingtalk contract subject update --file ./subject_update.json --format json
# 风险检测
dws dingtalk contract subject detect-risk --subject-name "北京示例科技有限公司" --format json
# 工商信息
dws dingtalk contract subject base-info --subject-name "北京示例科技有限公司" --format json
# 智能填充
dws dingtalk contract subject auto-fill --subject-name "北京示例科技有限公司" --format json
# 删除
dws dingtalk contract subject delete --subject-id <SUBJECT_ID> --format json
# 批量导入
dws dingtalk contract subject import-template --format json
dws dingtalk contract subject import --file-id "<FILE_ID>" --format json
dws dingtalk contract subject import-result --task-id "<TASK_ID>" --format json
```

## 上下文传递表

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `record list` | `contractId`（或返回体中等价主键字段名） | `record get --contract-id` |
| `import batch` | `taskId` | `import batch-result --task-id` |
| `review create` | `taskId` | `review result --task-id`（及 `--review-type`） |
| `file-directories` | 台账分类/目录元数据 | 了解分类树；**不**再作为 `record list` / `quantity-by-type` 的 `--type` 取值（二者为查询维度枚举） |
| `draft` | 起草结果中的合同或下载信息 | 以实际返回为准 |
| `project add` | `projectId` | `project detail --project-id`、`project update --project-id`、`project set-status --project-id`、`project delete --project-ids` |
| `project list` | `projectId` | `project detail --project-id` |
| `project import` | `taskId` | `project import-result --task-id` |
| `subject add` | `subjectId`（从 `subject list` 获取） | `subject detail --subject-id`、`subject update`、`subject delete --subject-id` |
| `subject list` | `subjectId` | `subject detail --subject-id` |
| `subject import` | `taskId` | `subject import-result --task-id` |

字段名以 MCP 实际 JSON 为准；上表为常见串联方式。

## 注意事项

- **`record list` 时间与筛选**：`--start` / `--end` 表示合同**创建时间**范围，须为 **ISO-8601** 字符串（与全局 CLI 时间规范一致）；CLI 换算为 MCP `createStartTime` / `createEndTime`（毫秒）。**禁止**将毫秒时间戳作为 CLI 入参。二者同时传入时，`--end` 须晚于 `--start`。`--status` 使用**英文**枚举，逗号分隔，解析时会 trim 空格。`--type` 仅限查询维度枚举（默认 `all`），与台账「分类名称」无关。
- **JSON 与 stdin**：`record create`、`review create`、`review analysis` 的 `--file` 可为 `-`，从标准输入读入。
- **项目管理参数**：`--start-date` / `--end-date` 及日期区间参数须为 **ISO-8601** 字符串（与全局 CLI 时间规范一致），CLI 换算为 MCP 毫秒时间戳。**禁止**将毫秒时间戳作为 CLI 入参；`--owners` / `--project-ids` / `--contract-ids` 须为**逗号分隔**格式，CLI 会自动 trim 空格。`--scope` 仅限 `self` / `all`。
- **相对方管理参数**：`subject add` / `subject update` 推荐通过 `--file` 传入 JSON（字段较多）。`--party-type` 仅限 `other`（对方）/ `our`（己方）。`--subject-ids` 批量删除最多 1000 个。`detect-risk`、`base-info`、`auto-fill` 三个命令共享相同的入参结构（`--subject-name` 必填，`--subject-id` 可选）。
- **隐藏顶层命令**：优先使用 `dws dingtalk contract`；`dws contract` 为兼容隐藏入口。
- **权限与登录**：依赖已配置的 DWS 身份；未登录、Token 过期等与全局 `dws` 行为一致。

## 相关产品

- `dingtalk-minutes` (`references/minutes.md`) — 听记 taskUuid，供 `draft --task-uuids` 使用
- `dingtalk-drive` (`references/drive.md`) — 钉盘 `fileId`、`spaceId`，供批量导入与审查文件引用
