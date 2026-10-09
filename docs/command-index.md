# dws Command Index

Every visible command currently exposed by the `dws` CLI.

- **Top-level commands**: 45
- **Visible command nodes**: 1750
- **Runnable/leaf commands**: 1750
- **Generated from**: Cobra command tree via `internal/app.NewRootCommand()`.

> Auto-generated. Hidden compatibility aliases are intentionally excluded.

## Global flags

Every command inherits these flags (documented here once, not repeated per command):

| Flag | Purpose |
|---|---|
| `--client-id` | 覆盖 OAuth 客户端 ID (钉钉 AppKey) |
| `--client-secret` | 覆盖 OAuth 客户端密钥 (钉钉 AppSecret) |
| `--debug` | 显示调试日志 |
| `--dry-run` | 预览操作内容，不实际执行 |
| `--fields` | 筛选输出字段 (逗号分隔, 如: name,id,status) |
| `-f, --format` | 输出格式: json\|table\|raw\|pretty\|ndjson\|csv (default `json`) |
| `--jq` | jq 表达式过滤输出 (如: '.items[] \| .name') |
| `--mock` | 使用 Mock 数据 (开发调试用) |
| `--profile` | 一次性指定组织或账号；支持 corpId/corpName 与 userId/userName 组合，推荐使用 profile list 返回的 corpId:userId；多个按 CSV 逗号分隔 |
| `--timeout` | HTTP 请求超时时间 (秒) (default `30`) |
| `-v, --verbose` | 显示详细日志 |
| `-y, --yes` | 跳过确认提示 (AI Agent 模式) |

## Contents

- [`dws agoal`](#dws-agoal) · 29 command nodes
- [`dws aisearch`](#dws-aisearch) · 5 command nodes
- [`dws aitable`](#dws-aitable) · 312 command nodes
- [`dws api`](#dws-api) · 1 command nodes
- [`dws attendance`](#dws-attendance) · 80 command nodes
- [`dws audit`](#dws-audit) · 4 command nodes
- [`dws auth`](#dws-auth) · 9 command nodes
- [`dws cache`](#dws-cache) · 4 command nodes
- [`dws calendar`](#dws-calendar) · 60 command nodes
- [`dws chat`](#dws-chat) · 257 command nodes
- [`dws completion`](#dws-completion) · 1 command nodes
- [`dws config`](#dws-config) · 2 command nodes
- [`dws contact`](#dws-contact) · 72 command nodes
- [`dws contract`](#dws-contract) · 47 command nodes
- [`dws dev`](#dws-dev) · 86 command nodes
- [`dws devapp`](#dws-devapp) · 26 command nodes
- [`dws devdoc`](#dws-devdoc) · 3 command nodes
- [`dws ding`](#dws-ding) · 13 command nodes
- [`dws doc`](#dws-doc) · 99 command nodes
- [`dws doctor`](#dws-doctor) · 1 command nodes
- [`dws drive`](#dws-drive) · 91 command nodes
- [`dws event`](#dws-event) · 7 command nodes
- [`dws help`](#dws-help) · 1 command nodes
- [`dws hrbrain`](#dws-hrbrain) · 17 command nodes
- [`dws html`](#dws-html) · 5 command nodes
- [`dws live`](#dws-live) · 3 command nodes
- [`dws mail`](#dws-mail) · 97 command nodes
- [`dws markdown`](#dws-markdown) · 8 command nodes
- [`dws mcp`](#dws-mcp) · 6 command nodes
- [`dws minutes`](#dws-minutes) · 77 command nodes
- [`dws oa`](#dws-oa) · 41 command nodes
- [`dws pat`](#dws-pat) · 4 command nodes
- [`dws plugin`](#dws-plugin) · 16 command nodes
- [`dws profile`](#dws-profile) · 4 command nodes
- [`dws recovery`](#dws-recovery) · 4 command nodes
- [`dws recruit`](#dws-recruit) · 5 command nodes
- [`dws report`](#dws-report) · 23 command nodes
- [`dws schema`](#dws-schema) · 1 command nodes
- [`dws sheet`](#dws-sheet) · 110 command nodes
- [`dws skill`](#dws-skill) · 5 command nodes
- [`dws todo`](#dws-todo) · 49 command nodes
- [`dws upgrade`](#dws-upgrade) · 1 command nodes
- [`dws version`](#dws-version) · 1 command nodes
- [`dws whiteboard`](#dws-whiteboard) · 22 command nodes
- [`dws wiki`](#dws-wiki) · 41 command nodes

## `dws agoal`

| Command | Description | Type |
|---|---|---|
| `dws agoal` | Agoal 管理 | runnable |
| `dws agoal +contract-fields` | 查询经营合约字段配置 | leaf |
| `dws agoal +obj-template-list` | 分页查询 Agoal 目标模板 | leaf |
| `dws agoal +report-statistics-list` | 查询周月报规则提交统计 | leaf |
| `dws agoal +report-submit-detail` | 分页查询周月报人员提交详情 | leaf |
| `dws agoal +user-rules` | 查询用户 Agoal 规则周期 | leaf |
| `dws agoal contract` | 经营合约管理 | runnable |
| `dws agoal contract detail` | 获取经营合约详情 | leaf |
| `dws agoal contract fields` | 获取经营合约字段列表 | leaf |
| `dws agoal contract list` | 获取经营合约列表 | leaf |
| `dws agoal contract update` | 更新经营合约 | leaf |
| `dws agoal obj-template` | 目标模板管理 | runnable |
| `dws agoal obj-template create-or-update` | 新增或更新目标模板 | leaf |
| `dws agoal obj-template list` | 获取目标模板列表 | leaf |
| `dws agoal report` | 周月报管理 | runnable |
| `dws agoal report list-statistics` | 获取周月报数据跟催列表 | leaf |
| `dws agoal report submit-detail` | 获取周月报规则提交详情 | leaf |
| `dws agoal scorecard` | 计分卡管理 | runnable |
| `dws agoal scorecard detail` | 获取计分卡详情 | leaf |
| `dws agoal scorecard entity-detail` | 获取计分卡实体详情 | leaf |
| `dws agoal scorecard search-entities` | 搜索计分卡指标与关键事项 | leaf |
| `dws agoal scorecard update` | 更新计分卡 | leaf |
| `dws agoal strategy` | 战略解码管理 | runnable |
| `dws agoal strategy detail` | 获取战略解码详情 | leaf |
| `dws agoal strategy list` | 获取战略解码列表 | leaf |
| `dws agoal strategy update` | 更新战略解码 | leaf |
| `dws agoal user` | 用户目标管理 | runnable |
| `dws agoal user objectives` | 查询用户目标列表 | leaf |
| `dws agoal user rules` | 获取用户的规则周期列表 | leaf |

## `dws aisearch`

| Command | Description | Type |
|---|---|---|
| `dws aisearch` | AI 搜问 | runnable |
| `dws aisearch +search-person` | 按姓名、部门、职责或组织关系语义搜索企业人员 | leaf |
| `dws aisearch behavior` | 搜索明确的发送/创建/接收等行为记录 | leaf |
| `dws aisearch enterprise` | 搜索企业内部知识内容和相关消息 | leaf |
| `dws aisearch person` | 搜索企业人员 | leaf |

## `dws aitable`

| Command | Description | Type |
|---|---|---|
| `dws aitable` | AI 表格操作 | runnable |
| `dws aitable +advperm-disable` | 关闭指定 Base 的高级权限总开关（所有自定义角色失效） | leaf |
| `dws aitable +advperm-enable` | 开启指定 Base 的高级权限总开关 | leaf |
| `dws aitable +app-block-create` | 创建应用 Widget 并独立核对配置和 48 列布局 | leaf |
| `dws aitable +app-block-delete` | 删除应用 Widget 并核对其所属页面列表 | leaf |
| `dws aitable +app-block-get` | 读取准确应用 Widget 配置与布局 | leaf |
| `dws aitable +app-block-list` | 列出准确应用页面中的全部 Widget | leaf |
| `dws aitable +app-block-update` | 更新应用 Widget 并独立核对名称、配置或布局 | leaf |
| `dws aitable +app-get` | 获取 Base 唯一应用模式 App；不存在时自动创建，需确认 | leaf |
| `dws aitable +app-page-create` | 新建应用仪表盘页面并按返回 ID 读回名称 | leaf |
| `dws aitable +app-page-delete` | 删除应用页面并核对完整页面目录中已不存在 | leaf |
| `dws aitable +app-page-get` | 读取准确应用页面及组件摘要 | leaf |
| `dws aitable +app-page-list` | 列出 Base 应用页面；不存在 App 时会创建默认 App，需确认 | leaf |
| `dws aitable +app-page-update` | 重命名应用页面并按准确 ID 读回 | leaf |
| `dws aitable +attachment-put` | 准备凭证、实际 PUT 本地文件、写入 attachment 单元格并读回验证 | leaf |
| `dws aitable +attachment-remove` | 清空 attachment 字段或按文件名解析 resourceId 删除，并读回验证 | leaf |
| `dws aitable +attachment-upload` | 为 attachment 字段申请 OSS 直传地址（uploadUrl / fileToken） | leaf |
| `dws aitable +base-bootstrap` | 一次创建 Base、数据表和字段，逐层读回验证并在中断时报告已知副作用 | leaf |
| `dws aitable +base-copy` | 复制 AI 表格（可选目标目录，可仅复制结构） | leaf |
| `dws aitable +base-delete` | 删除指定 Base（不可逆） | leaf |
| `dws aitable +base-get` | 获取指定 Base 的目录信息（tables / dashboards summary） | leaf |
| `dws aitable +base-get-primary-doc-id` | 根据 baseId/tableId/recordId 查询主键文档是否存在及其 dentryUuid | leaf |
| `dws aitable +base-list` | 获取当前用户可访问的 AI 表格 Base 列表（最近访问，支持游标分页） | leaf |
| `dws aitable +base-schema-snapshot` | 读取 Base、全部数据表、字段和视图的可复用结构快照，并严格校验每层响应 | leaf |
| `dws aitable +base-search` | 按名称关键词搜索 AI 表格 Base | leaf |
| `dws aitable +base-update` | 更新 Base 名称（可选备注） | leaf |
| `dws aitable +chart-create` | 创建图表并独立核对配置和布局 | leaf |
| `dws aitable +chart-delete` | 删除指定 chart 及其布局项（不可逆） | leaf |
| `dws aitable +chart-get` | 获取指定 chart 的详细信息 | leaf |
| `dws aitable +chart-share-get` | 查询 chart 的分享配置 | leaf |
| `dws aitable +chart-share-update` | 开启/关闭 chart 分享并可设置分享类型 | leaf |
| `dws aitable +chart-update` | 更新指定 chart 的配置或布局（--config 必填；layout 写前强制校验 12/48 列协议） | leaf |
| `dws aitable +chart-widgets-example` | 获取所有图表类型的 widget config 示例 | leaf |
| `dws aitable +dashboard-arrange` | 对指定仪表盘做服务端智能布局重排 | leaf |
| `dws aitable +dashboard-config-example` | 获取 dashboard config 的结构示例 | leaf |
| `dws aitable +dashboard-create` | 创建仪表盘并独立核对新 ID 与名称 | leaf |
| `dws aitable +dashboard-delete` | 删除指定 dashboard（级联删除其 chart，不可逆） | leaf |
| `dws aitable +dashboard-get` | 获取指定 dashboard 的详细信息（含只读 schemaVersion 类型证据与 charts summary） | leaf |
| `dws aitable +dashboard-list` | 从完整 Base 目录读取仪表盘列表并验证每项 ID | leaf |
| `dws aitable +dashboard-share-get` | 查询 dashboard 的分享配置 | leaf |
| `dws aitable +dashboard-share-update` | 开启/关闭 dashboard 分享并可设置分享类型 | leaf |
| `dws aitable +dashboard-update` | 更新指定 dashboard 的配置 | leaf |
| `dws aitable +data-query` | 用 DWS JSON DSL 统一执行标量或分组聚合，不拉全表做本地统计 | leaf |
| `dws aitable +datasource-create` | 为指定 AI 表格创建数据源同步配置，创建一张数据源表并触发首次全量同步。返回新建数据源表 ID 和同步任务 ID。 | leaf |
| `dws aitable +datasource-get-config` | 获取指定数据源表的同步配置信息，包括源配置、是否全量同步、是否自动同步、同步状态等。仅适用于数据源表（sync=true），普通表会返回错误。仅支持 OA 审批数据源（datasourceType=OA），其他数据源类型暂不支持，待后续开放。返回的 sourceConfig 包含数据源连接信息（如审批模板 ID、源表 ID 等）。 | leaf |
| `dws aitable +datasource-get-fields` | 获取指定数据源下可供同步的字段列表，用于了解来源字段结构；当前仅支持全量同步。传入从 +datasource-list-sources 获取的 sourceConfig。仅支持 OA 审批数据源（datasourceType=OA），其他数据源类型暂不支持，待后续开放。 | leaf |
| `dws aitable +datasource-list-sources` | 列出指定 Base 下可用的数据源条目。仅支持 OA 审批数据源（datasourceType=OA）。返回的每条条目包含 result 字段（下游原始 JSON 字符串）和 sourceType 字段（OA 审批对应 2，仅供参考）。OA 审批场景下 result 为包含 approvals 数组的 JSON 字符串，每个 approval 包含 processCode、name、iconUrl、url、keepRemovedFields、splitParentTableField 等字段。须原样透传至 sourceConfig 的字段（仅以下 4 个）：processCode、name、iconUrl、url；调用方自行设置的字段（即使 result 中有值也不透传）：keepRemovedFields、splitParentTableField；enableDataSyncOaDetailList 为下游内部字段，无需传入 sourceConfig。调用方应自行解析 result，提取目标模板字段后构造 sourceConfig 传入 +datasource-create。 | leaf |
| `dws aitable +datasource-sync` | 对指定 AI 表格中的数据源表触发一次手动同步。单次最多 5 张表，每张表独立提交，部分失败不影响其他表。该工具仅触发任务即返回，不会等待同步完成。返回结果包含文档链接，用户可打开文档查看同步进度与最终数据。每张表独立提交，整体仍返回 success；调用方需遍历 tasks[] 按单条 status 判断。同步运行中的表返回 failed 状态（errorCode=SYNC_RUNNING），属幂等冲突，应视为稍后重试而非最终失败。非数据源表（sync=false）不能用此工具触发同步，会以参数错误返回。 | leaf |
| `dws aitable +datasource-sync-status` | 按任务 ID 查询指定数据源表的同步任务状态。与 +datasource-sync / +datasource-create / +datasource-update 配对使用，单次最多 5 个 taskId。仅 RUNNING 需要继续轮询；FINISHED、FAILED、NOT_FOUND 为终态；UNKNOWN 表示状态不可断言，也不应继续轮询。批量查询整体 success 时仍需逐项检查 tasks[].status 和 errorCode。 | leaf |
| `dws aitable +datasource-update` | 更新已有数据源表的完整源配置或自动同步设置，并触发一次全量同步。仅适用于数据源表，当前不支持选择同步字段。 | leaf |
| `dws aitable +export-data` | 导出 AI 表格数据（创建导出任务或按 taskId 续等） | leaf |
| `dws aitable +field-create` | 批量新增字段，15 个分片并核对新字段 ID、类型与配置 | leaf |
| `dws aitable +field-delete` | 删除指定字段（不可逆） | leaf |
| `dws aitable +field-get` | 批量获取字段详情（含类型相关完整配置） | leaf |
| `dws aitable +field-run-ai` | 提交 AI 字段运行请求；缺少可靠受理证据时非零退出并保留回执 | leaf |
| `dws aitable +field-update` | 更新字段名称 / 说明 / 配置 / AI 配置（类型不可改） | leaf |
| `dws aitable +find-record` | 在指定多维表里按关键词查记录（只读） | leaf |
| `dws aitable +form-create` | 创建表单并按新 viewId 核对名称 | leaf |
| `dws aitable +form-delete` | 删除指定表单视图（不可逆） | leaf |
| `dws aitable +form-field-hide` | 切换表单字段的隐藏/显示状态 | leaf |
| `dws aitable +form-field-list` | 列出表单视图当前可见的字段及其配置 | leaf |
| `dws aitable +form-field-update` | 更新表单字段的必填状态或描述 | leaf |
| `dws aitable +form-get` | 按准确 viewId 获取表单及可见题目 | leaf |
| `dws aitable +form-list` | 列出指定数据表下的所有表单视图 | leaf |
| `dws aitable +form-questions-remove` | 只隐藏表单题目并核对表字段仍存在，不删除整列 | leaf |
| `dws aitable +form-share-get` | 读取表单分享配置及服务端真实 UUID、状态和封面 | leaf |
| `dws aitable +form-share-update` | 部分更新分享表单配置，并返回经服务端回读和 CP 投影校验的真实终态 | leaf |
| `dws aitable +form-submit` | 提交已分享表单并按返回 rowId 核对写入值 | leaf |
| `dws aitable +form-update` | 更新表单标题 / 描述 | leaf |
| `dws aitable +import-data` | 将已上传文件导入 AI 表格（新建表或追加到已有表） | leaf |
| `dws aitable +import-file` | 申请上传、PUT 本地 CSV/XLS/XLSX，并用同一 importId 触发导入；也可只续等已有任务 | leaf |
| `dws aitable +import-upload` | 为导入任务申请 OSS 直传地址（uploadUrl / importId） | leaf |
| `dws aitable +list-tables` | 列出某个多维表(base)里的所有数据表（只读，投影 tableId/tableName） | leaf |
| `dws aitable +record-batch-create` | 批量新增记录，100 条分片并按返回 ID 独立验证；已有 recordId 明确拒绝 | leaf |
| `dws aitable +record-bulk-patch` | 完整查询目标记录后批量合并同一组 cells，自动分片并逐条读回验证 | leaf |
| `dws aitable +record-delete` | 批量删除记录（不可逆），自动按 100 条分片并逐批确认记录已不存在 | leaf |
| `dws aitable +record-download-attachment` | 按记录单元格中的准确 resourceId 下载附件，校验字节数并返回 SHA256 | leaf |
| `dws aitable +record-history-list` | 按 recordId 查询单条记录的变更历史 | leaf |
| `dws aitable +record-primary-doc-create` | 为记录创建主键文档（幂等），fieldId 须为 primaryDoc 类型 | leaf |
| `dws aitable +record-primary-doc-get` | 查询记录关联的主键文档 nodeId | leaf |
| `dws aitable +record-query` | 查询单表记录，支持准确 ID、视图、条件、全量分页与 NDJSON 文件 | leaf |
| `dws aitable +record-query-empty` | 扫描并过滤出完全没填用户字段的空行 | leaf |
| `dws aitable +record-share-links` | 批量（可 >20 条）获取多维表记录分享链接：去重+分片+合并 | leaf |
| `dws aitable +record-share-url` | 按 recordId 批量获取记录分享链接，单次最多 20 条 | leaf |
| `dws aitable +record-update` | 批量更新记录，自动按 100 条分片并逐批读回验证 | leaf |
| `dws aitable +record-upsert` | 按 recordId 自动拆分 create/update，按 100 条分片并读回验证 | leaf |
| `dws aitable +record-upsert-by-key` | 按唯一字段值有则更新、无则创建记录，并读回验证 | leaf |
| `dws aitable +record-write-result` | 按原 clientToken 只读核对已落库记录；未知结果不能作为重新创建依据 | leaf |
| `dws aitable +resolve-base` | 按名称搜索多维表 Base 并解析出唯一 baseId（只读） | leaf |
| `dws aitable +resolve-field` | 按精确名称唯一定位字段 | leaf |
| `dws aitable +resolve-table` | 在某个多维表 Base 内按名称解析出唯一的数据表 tableId（只读） | leaf |
| `dws aitable +resolve-view` | 按精确名称唯一定位视图 | leaf |
| `dws aitable +role-create` | 在指定 Base 下创建自定义角色 | leaf |
| `dws aitable +role-delete` | 删除 Base 下指定的自定义角色（不可逆） | leaf |
| `dws aitable +role-get` | 获取单个角色的完整配置 | leaf |
| `dws aitable +role-list` | 列出指定 Base 下的全部角色 | leaf |
| `dws aitable +role-update` | 按 PATCH 语义增量更新自定义角色 | leaf |
| `dws aitable +section-create` | 在指定 Base 下创建文件夹（组织 table / dashboard） | leaf |
| `dws aitable +section-delete` | 删除指定文件夹（不可逆） | leaf |
| `dws aitable +section-list-empty` | 列出指定 Base 下所有没有子节点的空文件夹 | leaf |
| `dws aitable +section-list-nodes` | 列出指定 Base 当前版本下的全部 nsheet 节点 | leaf |
| `dws aitable +section-move-node` | 把任意 nsheet 节点移动到目标文件夹下（可选调整位置） | leaf |
| `dws aitable +section-rename` | 重命名指定文件夹 | leaf |
| `dws aitable +section-reorder` | 在当前父文件夹下调整文件夹的展示顺序 | leaf |
| `dws aitable +table-bootstrap` | 在已有 Base 中一次创建数据表和字段，自动分片并读回验证 | leaf |
| `dws aitable +table-copy` | 跨 Base 同步复制一张表的可创建字段结构，并可同步复制全部记录 | leaf |
| `dws aitable +table-delete` | 删除指定数据表（不可逆） | leaf |
| `dws aitable +table-get` | 批量获取指定数据表的表级信息、字段目录与视图目录 | leaf |
| `dws aitable +table-update` | 更新数据表名称 / 备注 / 行命名规则 | leaf |
| `dws aitable +template-search` | 按名称关键词搜索 AI 表格模板 | leaf |
| `dws aitable +url-resolve` | 解析 AI 表格 URL 中的 baseId/tableId/viewId/recordId | leaf |
| `dws aitable +view-delete` | 删除指定视图（不可逆） | leaf |
| `dws aitable +view-duplicate` | 复制视图，生成配置相同的新视图 | leaf |
| `dws aitable +view-get` | 获取视图完整信息（列顺序、筛选、排序、分组等） | leaf |
| `dws aitable +view-get-frozen-cols` | 获取视图当前冻结的左侧列数 | leaf |
| `dws aitable +view-get-lock` | 获取视图锁定状态 | leaf |
| `dws aitable +view-get-row-height` | 获取视图单元格行高（像素） | leaf |
| `dws aitable +view-lock` | 锁定视图（默认）或解锁（--off） | leaf |
| `dws aitable +view-preset-apply` | 按视图精确名称幂等创建或更新预设；Gantt 可用独立 timebar 完成专用两步写入 | leaf |
| `dws aitable +view-set-fill-color-rule` | 全量覆盖 Grid 视图的条件填色规则（传 '[]' 清空） | leaf |
| `dws aitable +view-set-frozen-cols` | 设置视图冻结列数（0 表示取消冻结） | leaf |
| `dws aitable +view-set-row-height` | 设置视图单元格行高（像素，合法档位 32/56/88/128） | leaf |
| `dws aitable +view-update` | 更新视图名称 / 描述 / 配置（含类型校验后的筛选） | leaf |
| `dws aitable +workflow-deploy` | 创建或更新完整 workflow-dsl/v1，检查 valid/flowId；新建默认验证 STOP，--enable 验证 RUNNING | leaf |
| `dws aitable +workflow-disable` | 禁用指定 Base 中的自动化工作流（影响业务自动化） | leaf |
| `dws aitable +workflow-enable` | 启用指定 Base 中的自动化工作流 | leaf |
| `dws aitable +workflow-get` | 获取单个自动化工作流的详细信息 | leaf |
| `dws aitable +workflow-list` | 列出指定 Base 中的自动化工作流（分页） | leaf |
| `dws aitable advperm` | 高级权限管理（开关 / 角色查看与删除） | runnable |
| `dws aitable advperm disable` | 关闭高级权限总开关（高危） | leaf |
| `dws aitable advperm enable` | 开启高级权限总开关 | leaf |
| `dws aitable advperm role-create` | 创建自定义角色 | leaf |
| `dws aitable advperm role-delete` | 删除自定义角色（不可逆） | leaf |
| `dws aitable advperm role-get` | 获取单个角色完整配置 | leaf |
| `dws aitable advperm role-list` | 列出 Base 下所有角色 | leaf |
| `dws aitable advperm role-update` | 增量更新自定义角色配置（patch 语义） | leaf |
| `dws aitable app` | 应用模式管理 | runnable |
| `dws aitable app get` | 获取应用信息 | leaf |
| `dws aitable app page` | 应用页面管理 | runnable |
| `dws aitable app page create` | 创建应用页面 | leaf |
| `dws aitable app page delete` | 删除应用页面 | leaf |
| `dws aitable app page get` | 获取应用页面详情 | leaf |
| `dws aitable app page list` | 列出应用页面 | leaf |
| `dws aitable app page move` | 调整应用页面顺序 | leaf |
| `dws aitable app page update` | 更新应用页面 | leaf |
| `dws aitable app update` | 更新应用外观配置 | leaf |
| `dws aitable app widget` | 应用页面组件管理 | runnable |
| `dws aitable app widget create` | 创建应用页面 Widget | leaf |
| `dws aitable app widget delete` | 删除应用页面 Widget | leaf |
| `dws aitable app widget get` | 获取应用页面 Widget | leaf |
| `dws aitable app widget list` | 列出应用页面 Widget | leaf |
| `dws aitable app widget update` | 更新应用页面 Widget | leaf |
| `dws aitable attachment` | 附件管理 | runnable |
| `dws aitable attachment remove` | 删除记录附件 | leaf |
| `dws aitable attachment upload` | 准备附件上传 | leaf |
| `dws aitable base` | Base 管理 | runnable |
| `dws aitable base copy` | 复制 AI 表格 | leaf |
| `dws aitable base create` | 创建 AI 表格 | leaf |
| `dws aitable base delete` | 删除 AI 表格 | leaf |
| `dws aitable base get` | 获取 AI 表格信息 | leaf |
| `dws aitable base get-primary-doc-id` | 获取主键文档ID | leaf |
| `dws aitable base list` | 获取 AI 表格列表 | leaf |
| `dws aitable base search` | 搜索 AI 表格 | leaf |
| `dws aitable base update` | 更新 AI 表格 | leaf |
| `dws aitable chart` | 图表管理 | runnable |
| `dws aitable chart create` | 创建图表 | leaf |
| `dws aitable chart delete` | 删除图表 | leaf |
| `dws aitable chart get` | 获取图表信息 | leaf |
| `dws aitable chart share` | 图表分享管理 | runnable |
| `dws aitable chart share get` | 获取图表分享配置 | leaf |
| `dws aitable chart share update` | 更新图表分享配置 | leaf |
| `dws aitable chart update` | 更新图表 | leaf |
| `dws aitable chart widgets-example` | 获取图表配置示例 | leaf |
| `dws aitable comment` | 记录评论管理 | runnable |
| `dws aitable comment create` | 在记录上创建评论 | leaf |
| `dws aitable comment delete` | 删除记录评论 | leaf |
| `dws aitable comment list` | 分页查询记录评论 | leaf |
| `dws aitable comment reply` | 回复记录评论 | leaf |
| `dws aitable comment update` | 完整替换记录评论正文 | leaf |
| `dws aitable create` | 创建 AI 表格（dws aitable base create 的别名） | leaf |
| `dws aitable dashboard` | 仪表盘管理 | runnable |
| `dws aitable dashboard arrange` | 自动重排仪表盘图表布局 | leaf |
| `dws aitable dashboard config-example` | 获取仪表盘配置示例 | leaf |
| `dws aitable dashboard create` | 创建仪表盘 | leaf |
| `dws aitable dashboard delete` | 删除仪表盘 | leaf |
| `dws aitable dashboard get` | 获取仪表盘信息 | leaf |
| `dws aitable dashboard share` | 仪表盘分享管理 | runnable |
| `dws aitable dashboard share get` | 获取仪表盘分享配置 | leaf |
| `dws aitable dashboard share update` | 更新仪表盘分享配置 | leaf |
| `dws aitable dashboard update` | 更新仪表盘 | leaf |
| `dws aitable datasource` | 数据源同步管理 | runnable |
| `dws aitable datasource create` | 创建数据源表并触发首次同步 | leaf |
| `dws aitable datasource get-config` | 获取数据源表同步配置 | leaf |
| `dws aitable datasource get-fields` | 获取数据源可同步字段列表 | leaf |
| `dws aitable datasource list-sources` | 列出可用数据源来源 | leaf |
| `dws aitable datasource sync` | 触发数据源表手动同步 | leaf |
| `dws aitable datasource sync-status` | 按任务 ID 查询数据源同步任务状态 | leaf |
| `dws aitable datasource update` | 更新数据源表同步配置并触发同步 | leaf |
| `dws aitable entity` | 搜索 AI 表格人员、部门和群组实体 | runnable |
| `dws aitable entity search` | 搜索实体候选，不自动选择重名或模糊结果 | leaf |
| `dws aitable export` | 数据导出 | runnable |
| `dws aitable export data` | 导出数据 | leaf |
| `dws aitable field` | 字段管理 | runnable |
| `dws aitable field create` | 创建字段 | leaf |
| `dws aitable field delete` | 删除字段 | leaf |
| `dws aitable field get` | 获取字段详情 | leaf |
| `dws aitable field list` | 获取字段信息（dws aitable field get 的别名） | leaf |
| `dws aitable field run-ai` | 触发 AI 字段运行 | leaf |
| `dws aitable field search-options` | 搜索单选/多选字段的选项 | leaf |
| `dws aitable field update` | 更新字段 | leaf |
| `dws aitable form` | 表单管理 | runnable |
| `dws aitable form create` | 创建表单视图 | leaf |
| `dws aitable form delete` | 删除表单 | leaf |
| `dws aitable form field` | 表单字段管理 | runnable |
| `dws aitable form field hide` | 切换表单字段隐藏 | leaf |
| `dws aitable form field list` | 列出表单字段 | leaf |
| `dws aitable form field update` | 更新表单字段 | leaf |
| `dws aitable form get` | 获取单个表单视图详情 | leaf |
| `dws aitable form list` | 列出表单视图 | leaf |
| `dws aitable form questions` | 表单题目管理（等价于 field create / delete） | runnable |
| `dws aitable form questions create` | 向表单添加题目（等价于 field create） | leaf |
| `dws aitable form questions delete` | 从表单删除题目（等价于 field delete，不可逆） | leaf |
| `dws aitable form share` | 表单分享管理 | runnable |
| `dws aitable form share get` | 获取表单分享配置 | leaf |
| `dws aitable form share notify` | 通知表单填写人 | leaf |
| `dws aitable form share update` | 更新分享表单配置 | leaf |
| `dws aitable form submit` | 提交已分享表单 | leaf |
| `dws aitable form update` | 更新表单配置 | leaf |
| `dws aitable import` | 数据导入 | runnable |
| `dws aitable import data` | 导入数据 | leaf |
| `dws aitable import upload` | 准备导入文件上传 | leaf |
| `dws aitable info` | 获取 AI 表格信息（dws aitable base get 的别名） | leaf |
| `dws aitable list` | 获取 AI 表格列表（dws aitable base list 的别名） | leaf |
| `dws aitable psql` | 以 PostgreSQL 逻辑表方式查询 AI 表格 | leaf |
| `dws aitable record` | 记录管理 | runnable |
| `dws aitable record batch-update` | 批量更新记录（同一 cells 应用到多条 recordId） | leaf |
| `dws aitable record create` | 新增记录 | leaf |
| `dws aitable record create-sub` | 在父记录下创建子记录 | leaf |
| `dws aitable record delete` | 删除行记录 | leaf |
| `dws aitable record get` | 按 ID 获取记录（record query --record-ids 的便捷别名，单次最多 100 条） | leaf |
| `dws aitable record group-stats` | 分组、去重及高级聚合统计 | leaf |
| `dws aitable record history-list` | 查询行记录变更历史 | leaf |
| `dws aitable record ids` | 分页获取记录 ID | leaf |
| `dws aitable record list` | 获取行记录（dws aitable record query 的别名） | leaf |
| `dws aitable record primary-doc-create` | 为记录创建主键文档 | leaf |
| `dws aitable record primary-doc-get` | 查询记录的主键文档 | leaf |
| `dws aitable record query` | 获取行记录 | leaf |
| `dws aitable record query-empty` | 查询完全没填用户字段的空行 | leaf |
| `dws aitable record share-url` | 批量获取记录分享链接 | leaf |
| `dws aitable record stats` | 整表或过滤后的字段聚合统计 | leaf |
| `dws aitable record update` | 更新记录 | leaf |
| `dws aitable record upsert` | 批量创建或更新记录（Upsert） | leaf |
| `dws aitable search` | AI 表格搜索（dws aitable base search 的别名） | leaf |
| `dws aitable section` | 文件夹与节点管理 | runnable |
| `dws aitable section create` | 创建文件夹 | leaf |
| `dws aitable section delete` | 删除文件夹 | leaf |
| `dws aitable section list-empty` | 列出空文件夹 | leaf |
| `dws aitable section list-nodes` | 列出全部节点 | leaf |
| `dws aitable section move-node` | 移动节点 | leaf |
| `dws aitable section rename` | 重命名文件夹 | leaf |
| `dws aitable section reorder` | 调整文件夹顺序 | leaf |
| `dws aitable table` | 数据表管理 | runnable |
| `dws aitable table create` | 创建数据表 | leaf |
| `dws aitable table delete` | 删除数据表 | leaf |
| `dws aitable table get` | 获取数据表 | leaf |
| `dws aitable table list` | 获取数据表信息（dws aitable table get 的别名） | leaf |
| `dws aitable table update` | 更新数据表 | leaf |
| `dws aitable template` | 模板搜索 | runnable |
| `dws aitable template search` | 搜索模板 | leaf |
| `dws aitable view` | 视图管理 | runnable |
| `dws aitable view create` | 创建视图 | leaf |
| `dws aitable view delete` | 删除视图 | leaf |
| `dws aitable view duplicate` | 复制视图 | leaf |
| `dws aitable view get` | 获取视图详情 | runnable |
| `dws aitable view get aggregate` | 获取视图 aggregate 配置 | leaf |
| `dws aitable view get card` | 获取视图 card 配置 | leaf |
| `dws aitable view get field-widths` | 获取视图 field-widths 配置 | leaf |
| `dws aitable view get fill-color-rule` | 获取视图 fill-color-rule 配置 | leaf |
| `dws aitable view get filter` | 获取视图 filter 配置 | leaf |
| `dws aitable view get frozen-cols` | 获取视图冻结列数 | leaf |
| `dws aitable view get group` | 获取视图 group 配置 | leaf |
| `dws aitable view get hidden-fields` | 获取视图隐藏字段 | leaf |
| `dws aitable view get lock` | 获取视图锁定状态 | leaf |
| `dws aitable view get row-height` | 获取视图行高（单元格高度） | leaf |
| `dws aitable view get sort` | 获取视图 sort 配置 | leaf |
| `dws aitable view get timebar` | 获取视图 timebar 配置 | leaf |
| `dws aitable view get visible-fields` | 获取视图 visible-fields 配置 | leaf |
| `dws aitable view list` | 获取视图信息（dws aitable view get 的别名） | leaf |
| `dws aitable view lock` | 锁定/解锁视图 | leaf |
| `dws aitable view update` | 更新视图 | runnable |
| `dws aitable view update aggregate` | 更新视图字段聚合统计（仅 Grid） | leaf |
| `dws aitable view update card` | 更新视图 card 配置（Kanban / Gallery） | leaf |
| `dws aitable view update field-widths` | 更新视图字段列宽（仅 Grid） | leaf |
| `dws aitable view update fill-color-rule` | 更新视图数据高亮规则 | leaf |
| `dws aitable view update filter` | 更新视图 filter 配置 | leaf |
| `dws aitable view update frozen-cols` | 更新视图冻结列数 | leaf |
| `dws aitable view update group` | 更新视图 group 配置 | leaf |
| `dws aitable view update name` | 重命名视图（= view update --name 的便捷子命令） | leaf |
| `dws aitable view update row-height` | 更新视图行高（单元格高度） | leaf |
| `dws aitable view update sort` | 更新视图 sort 配置 | leaf |
| `dws aitable view update timebar` | 更新视图 timebar 配置（仅 Gantt） | leaf |
| `dws aitable view update visible-fields` | 更新视图可见字段列表 | leaf |
| `dws aitable workflow` | 自动化工作流管理（创建 / 更新 / 启停 / 执行 / 历史 / 查询） | runnable |
| `dws aitable workflow create` | 创建并发布自动化工作流 | leaf |
| `dws aitable workflow disable` | 禁用指定工作流（高危） | leaf |
| `dws aitable workflow edit-example` | 获取工作流编辑文档与示例 | leaf |
| `dws aitable workflow enable` | 启用指定工作流 | leaf |
| `dws aitable workflow get` | 获取单个工作流详情 | leaf |
| `dws aitable workflow history` | 查询工作流执行历史 | leaf |
| `dws aitable workflow list` | 列出 Base 下的工作流 | leaf |
| `dws aitable workflow run` | 执行指定自动化工作流 | leaf |
| `dws aitable workflow update` | 更新并发布已有自动化工作流 | leaf |

## `dws api`

| Command | Description | Type |
|---|---|---|
| `dws api` | 调用钉钉 OpenAPI (Raw HTTP) | leaf |

## `dws attendance`

| Command | Description | Type |
|---|---|---|
| `dws attendance` | 考勤打卡 / 排班 / 统计 | runnable |
| `dws attendance +calculate-approve-duration` | 计算加班/请假/外出等考勤审批时长 | leaf |
| `dws attendance +check-companion-schedules` | 校验出差/外出同行人的班次是否允许一起提交 | leaf |
| `dws attendance +check-record` | 查询用户打卡流水（打卡时间/地点/定位方式） | leaf |
| `dws attendance +check-result` | 查询用户打卡结果（迟到/早退/缺卡等） | leaf |
| `dws attendance +get-adjustment-rule` | 根据补卡规则主键 ID 查询补卡规则详情 | leaf |
| `dws attendance +get-approve-template` | 查询补卡/请假/加班/外出/出差审批提交链接 | leaf |
| `dws attendance +get-checkin-record` | 查询指定员工一段时间内的签到记录 | leaf |
| `dws attendance +get-complex-overtime-setting` | 查询员工在指定日期适用的复杂加班规则 | leaf |
| `dws attendance +get-leave-records` | 查询指定员工的假期余额变更记录 | leaf |
| `dws attendance +get-overtime-rule` | 根据加班规则主键 ID 查询加班规则详情 | leaf |
| `dws attendance +get-schedule` | 获取指定用户一段时间内的排班记录 | leaf |
| `dws attendance +get-self-setting` | 查询个人规则设置（打卡提醒/极速打卡/缺卡提醒等） | leaf |
| `dws attendance +get-summary` | 查询某个人的考勤统计摘要（周/月） | leaf |
| `dws attendance +list-approve` | 查询用户考勤审批单（补卡/加班/请假/出差外出） | leaf |
| `dws attendance +list-leave-types` | 查询当前用户可用的假期规则列表 | leaf |
| `dws attendance +my-attendance` | 查我今天的考勤打卡记录（打卡流水，自动解析当前用户） | leaf |
| `dws attendance +query-report-data` | 根据字段查询考勤报表数据（仅管理员） | leaf |
| `dws attendance +search-adjustment-rule` | 查询当前用户可管理的补卡规则列表 | leaf |
| `dws attendance +search-class` | 查询当前用户可管理的班次详情列表 | leaf |
| `dws attendance +search-group` | 查询当前用户可管理的考勤组列表 | leaf |
| `dws attendance +search-overtime-rule` | 查询当前用户可管理的加班规则列表 | leaf |
| `dws attendance +this-month` | 查我本月的考勤打卡记录（打卡流水，自动解析当前用户） | leaf |
| `dws attendance adjustment` | 补卡规则 | runnable |
| `dws attendance adjustment get` | 根据补卡规则主键 ID 查询补卡规则详情 | leaf |
| `dws attendance adjustment search` | 查询当前用户可管理的补卡规则列表 | leaf |
| `dws attendance approve` | 审批单查询 | runnable |
| `dws attendance approve leave-check` | 提交前校验请假资格（时间冲突 / 可撤销单 / 额度） | leaf |
| `dws attendance approve leave-duration` | 计算请假时长（服务端口径，与客户端发起页一致） | leaf |
| `dws attendance approve leave-types` | 查询可用假期类型及余额 | leaf |
| `dws attendance approve list` | 查询用户审批单（补卡/加班/请假/出差外出） | leaf |
| `dws attendance approve supply-check` | 提交前校验补卡资格（期限 / 次数 / 状态） | leaf |
| `dws attendance approve supply-plans` | 匹配补卡目标异常班次（服务端口径，与客户端发起页一致） | leaf |
| `dws attendance approve templates` | 查询补卡/请假/加班/外出/出差审批提交链接 | leaf |
| `dws attendance boss-check` | BOSS 改签打卡记录 | leaf |
| `dws attendance check` | 打卡查询 | runnable |
| `dws attendance check record` | 查询打卡流水 | leaf |
| `dws attendance check result` | 查询打卡结果 | leaf |
| `dws attendance checkin` | 签到管理 | runnable |
| `dws attendance checkin records` | 查询指定员工的签到记录 | leaf |
| `dws attendance class` | 班次规则 | runnable |
| `dws attendance class create` | 创建班次 | leaf |
| `dws attendance class get` | 根据班次 ID 查询班次详情 | leaf |
| `dws attendance class search` | 查询当前用户可管理的所有班次详情 | leaf |
| `dws attendance class update` | 更新班次 | leaf |
| `dws attendance globalsetting` | 全局规则设置（仅管理员） | runnable |
| `dws attendance globalsetting get` | 查询全局规则设置（仅管理员） | leaf |
| `dws attendance globalsetting save` | 更新保存全局规则设置（仅管理员） | leaf |
| `dws attendance group` | 考勤组 | runnable |
| `dws attendance group create` | 创建考勤组 | leaf |
| `dws attendance group filtered-get` | 根据考勤组 ID 按需查询成员/打卡地址/蓝牙/Wifi 信息 | leaf |
| `dws attendance group get` | 根据考勤组 ID 查询考勤组全量信息 | leaf |
| `dws attendance group search` | 查询当前用户可管理的考勤组列表 | leaf |
| `dws attendance group update` | 更新考勤组配置（仅修改需要变更的字段） | leaf |
| `dws attendance group update-members` | 更新考勤组成员（添加/删除考勤人员、部门、无需考勤人员） | leaf |
| `dws attendance overtime` | 加班规则 | runnable |
| `dws attendance overtime get` | 根据加班规则主键 ID 查询加班规则详情 | leaf |
| `dws attendance overtime search` | 查询当前用户可管理的加班规则列表 | leaf |
| `dws attendance record` | 考勤记录 | runnable |
| `dws attendance record get` | 查询个人考勤详情 | leaf |
| `dws attendance report` | 查询考勤报表和结果 | runnable |
| `dws attendance report columns` | 获取企业考勤字段列表 | leaf |
| `dws attendance report query-data` | 根据字段查询考勤数据 | leaf |
| `dws attendance report query-leave` | 查询用户假期数据 | leaf |
| `dws attendance rules` | 查询考勤组与考勤规则 | leaf |
| `dws attendance schedule` | 排班管理 | runnable |
| `dws attendance schedule get` | 获取指定用户的排班记录 | leaf |
| `dws attendance schedule import` | 导入排班记录到排班制考勤组 | leaf |
| `dws attendance selfsetting` | 个人规则设置 | runnable |
| `dws attendance selfsetting get` | 查询个人规则设置 | leaf |
| `dws attendance selfsetting save` | 更新保存个人规则设置 | leaf |
| `dws attendance shift` | 班次查询 | runnable |
| `dws attendance shift list` | 批量查询员工班次信息 | leaf |
| `dws attendance summary` | 查询某个人的考勤统计摘要 | leaf |
| `dws attendance vacation` | 假期管理 | runnable |
| `dws attendance vacation balance` | 查询指定员工假期余额 | leaf |
| `dws attendance vacation records` | 查询指定员工假期余额变更记录 | leaf |
| `dws attendance vacation save-balance` | 设置员工假期余额 | leaf |
| `dws attendance vacation types` | 查询当前用户假期规则列表 | leaf |
| `dws attendance vacation update-type` | 更新假期规则 | leaf |

## `dws audit`

| Command | Description | Type |
|---|---|---|
| `dws audit` | 操作审计日志管理 | runnable |
| `dws audit export` | 导出审计日志 | leaf |
| `dws audit tail` | 查看最近的审计记录 | leaf |
| `dws audit verify` | 校验审计日志哈希链完整性 | leaf |

## `dws auth`

| Command | Description | Type |
|---|---|---|
| `dws auth` | 认证管理 | runnable |
| `dws auth exchange` | 使用外部授权码登录并保存完整身份 | leaf |
| `dws auth export` | 导出可迁移认证包 | leaf |
| `dws auth import` | 导入可迁移认证包 | leaf |
| `dws auth login` | 登录钉钉（自动刷新 token，必要时扫码） | leaf |
| `dws auth logout` | 清除认证信息（默认退出所有组织） | leaf |
| `dws auth migrate-keychain` | 将 macOS 系统 Keychain 登录态安全迁移到 file-DEK | leaf |
| `dws auth reset` | 重置认证信息（清除本地 Token，触发重新授权） | leaf |
| `dws auth status` | 查看认证状态 | leaf |

## `dws cache`

| Command | Description | Type |
|---|---|---|
| `dws cache` | 不再支持：服务发现缓存兼容入口 | runnable |
| `dws cache clean` | 不再支持：静态端点模式无需服务发现缓存 | leaf |
| `dws cache refresh` | 不再支持：静态端点模式无需服务发现缓存 | leaf |
| `dws cache status` | 不再支持：静态端点模式无需服务发现缓存 | leaf |

## `dws calendar`

| Command | Description | Type |
|---|---|---|
| `dws calendar` | 日历日程 / 会议室 / 闲忙 | runnable |
| `dws calendar +agenda` | 查询日程列表（不传时间默认查询今天） | leaf |
| `dws calendar +attendee-list` | 查看日程参会人 | leaf |
| `dws calendar +book` | 创建日程，并可按姓名邀请参会人（自动解析 userId，失败自动回滚删除日程） | leaf |
| `dws calendar +book-list` | 查询用户的日历本列表 | leaf |
| `dws calendar +book-search` | 按名称模糊搜索日历本 | leaf |
| `dws calendar +cancel-event` | 取消（删除）一个已有日程（删除前先确认它真实存在） | leaf |
| `dws calendar +conflicts` | 检测我某天日程的时间冲突（重叠/双重预订，默认今天） | leaf |
| `dws calendar +create` | 创建日程并读回验证 | leaf |
| `dws calendar +free` | 按姓名查询某人在指定时间段内的忙闲状态（自动解析 userId） | leaf |
| `dws calendar +free-slots` | 找我某天工作时段内的空闲时间段（默认今天 09:00-18:00） | leaf |
| `dws calendar +freebusy` | 查询用户 / 会议室闲忙状态（--users 与 --rooms 至少其一） | leaf |
| `dws calendar +get` | 严格获取日程详情 | leaf |
| `dws calendar +invite` | 按姓名把参会人加入已有日程（自动解析 userId 后批量添加） | leaf |
| `dws calendar +my-free` | 查我自己在某时间段的忙闲（默认今天，无需输入姓名） | leaf |
| `dws calendar +next-event` | 查看接下来最近的一个日程（默认扫描未来 7 天） | leaf |
| `dws calendar +reschedule` | 改一个已有日程的时间（只动开始/结束时间，其他字段不变） | leaf |
| `dws calendar +room-find` | 按时间段搜索可用会议室（不传时间默认当前起 1 小时） | leaf |
| `dws calendar +room-groups` | 会议室分组列表 | leaf |
| `dws calendar +room-search` | 按名称模糊搜索会议室（不检查可用性） | leaf |
| `dws calendar +rsvp` | 响应日程邀请并读取终态 | leaf |
| `dws calendar +search-event` | 在一个日程页内严格搜索事件 | leaf |
| `dws calendar +suggest-time` | 按姓名解析多位参与者，推荐大家都有空的可开会时间段（自动解析 userId） | leaf |
| `dws calendar +suggestion` | 按 userId 严格推荐共同空闲时间 | leaf |
| `dws calendar +today` | 列出我今天的日程（自动计算今天的起止时间，无需手动填时间范围） | leaf |
| `dws calendar +tomorrow` | 列出我明天的日程（自动计算明天的起止时间，无需手动填时间范围） | leaf |
| `dws calendar +update` | 分阶段更新日程并读回验证 | leaf |
| `dws calendar +week` | 列出我本周的日程（自动按周一为周首计算本周起止时间，无需手动填时间范围） | leaf |
| `dws calendar acl` | 管理我的日历访问权限（共享给他人） | runnable |
| `dws calendar acl add` | 把我的日历共享给某人 | leaf |
| `dws calendar acl delete` | 删除日历访问权限 | leaf |
| `dws calendar acl list` | 查询我的日历共享给了谁 | leaf |
| `dws calendar attachment` | 日程附件管理 | runnable |
| `dws calendar attachment add` | 添加日程附件 | leaf |
| `dws calendar attendee` | 日程参会人管理 | runnable |
| `dws calendar attendee add` | 添加参会人 | leaf |
| `dws calendar attendee delete` | 移除参会人 | leaf |
| `dws calendar attendee list` | 查看参会人 | leaf |
| `dws calendar book` | 日历本管理（我能看哪些日历） | runnable |
| `dws calendar book get` | 查询指定日历本 | leaf |
| `dws calendar book list` | 查询用户的日历列表 | leaf |
| `dws calendar book search` | 搜索日历本 | leaf |
| `dws calendar book update` | 更新指定日历本 | leaf |
| `dws calendar busy` | 闲忙查询 | runnable |
| `dws calendar busy search` | 查询用户 / 会议室闲忙状态 | leaf |
| `dws calendar event` | 日程管理 | runnable |
| `dws calendar event create` | 创建日程 | leaf |
| `dws calendar event delete` | 删除日程 | leaf |
| `dws calendar event get` | 获取日程详情 | leaf |
| `dws calendar event instances` | 查询循环日程的实例列表 | leaf |
| `dws calendar event list` | 查询日程列表 | leaf |
| `dws calendar event respond` | 响应日程（接受/拒绝/暂定） | leaf |
| `dws calendar event share-info` | 获取日程的分享信息 | leaf |
| `dws calendar event suggest` | 建议日程时间 | leaf |
| `dws calendar event update` | 修改日程 | leaf |
| `dws calendar room` | 会议室管理 | runnable |
| `dws calendar room add` | 预定会议室 | leaf |
| `dws calendar room delete` | 移除会议室 | leaf |
| `dws calendar room list-groups` | 会议室分组列表 | leaf |
| `dws calendar room search` | 搜索会议室 (按名称搜索或按时间段查可用会议室) | leaf |

## `dws chat`

| Command | Description | Type |
|---|---|---|
| `dws chat` | 群聊 / 消息 / 机器人 | runnable |
| `dws chat +at-me` | 查最近 @我 的消息（自动算时间窗，投影发送人/时间/内容/会话） | leaf |
| `dws chat +bot-find` | 搜索全部可用机器人（含他人/官方，返回 openDingTalkId 可发单聊） | leaf |
| `dws chat +bot-search` | 搜索当前用户自己创建的机器人 | leaf |
| `dws chat +broadcast` | 按姓名逐一给多个人群发同一条单聊消息（自动解析 userId、逐个发送） | leaf |
| `dws chat +category-add-conversation` | 将会话移动到指定的自定义分组中 | leaf |
| `dws chat +category-create` | 创建用户自定义会话分组 | leaf |
| `dws chat +category-delete` | 删除用户自定义会话分组 | leaf |
| `dws chat +category-list` | 获取用户自定义会话分组 | leaf |
| `dws chat +category-list-conversations` | 按稳定 categoryId 列出会话分组中的会话 | leaf |
| `dws chat +category-remove-conversation` | 将会话从指定的自定义分组中移出 | leaf |
| `dws chat +category-rename` | 更新用户自定义会话分组的名称 | leaf |
| `dws chat +chat-add-bot` | 将机器人添加到群中 | leaf |
| `dws chat +chat-audit-join` | 审批入群验证（通过/拒绝/删除/忽略/拉黑） | leaf |
| `dws chat +chat-bots` | 查看群内所有机器人 | leaf |
| `dws chat +chat-create` | 按成员和可选群主全量预检后创建一个钉钉群聊 | leaf |
| `dws chat +chat-dismiss` | 解散群聊（不可逆，需群主权限） | leaf |
| `dws chat +chat-get-by-id` | 根据群号获取群聊信息 | leaf |
| `dws chat +chat-invite-url` | 获取群邀请链接 | leaf |
| `dws chat +chat-list` | 分页列出当前用户加入的会话（默认群聊；可选包含单聊） | leaf |
| `dws chat +chat-list-all` | 分页拉取我加入的所有群列表 | leaf |
| `dws chat +chat-list-join-requests` | 分页拉取入群验证记录 | leaf |
| `dws chat +chat-list-mine` | 拉取我创建/管理的群 | leaf |
| `dws chat +chat-members-get` | 根据成员 openDingTalkId 批量查询群成员详情 | leaf |
| `dws chat +chat-members-list` | 列出群成员并把用户与机器人分桶（支持群名语义解析） | leaf |
| `dws chat +chat-messages` | 读取指定群聊或单聊的消息记录，支持有界全量分页与原子 JSON 导出 | leaf |
| `dws chat +chat-mute` | 全员禁言 / 取消全员禁言 | leaf |
| `dws chat +chat-mute-member` | 指定群成员禁言 / 取消禁言 | leaf |
| `dws chat +chat-quit` | 退出群聊 | leaf |
| `dws chat +chat-remove-bot` | 从群内移除机器人 | leaf |
| `dws chat +chat-role-add` | 添加群身份 | leaf |
| `dws chat +chat-role-list` | 拉取会话的群身份列表 | leaf |
| `dws chat +chat-role-query-user` | 查询群成员的群身份 | leaf |
| `dws chat +chat-role-remove` | 删除群身份 | leaf |
| `dws chat +chat-role-remove-user` | 移除用户的指定群身份 | leaf |
| `dws chat +chat-role-set-user` | 设置用户的群身份（覆盖该用户的全部群身份） | leaf |
| `dws chat +chat-role-update` | 更新群身份名称 | leaf |
| `dws chat +chat-search` | 按关键词分页搜索群聊，支持有界自动翻页和完整性检查 | leaf |
| `dws chat +chat-set-admin` | 设置 / 取消群管理员 | leaf |
| `dws chat +chat-set-history` | 设置新成员入群可查看历史消息范围 | leaf |
| `dws chat +chat-transfer-owner` | 转让群主 | leaf |
| `dws chat +chat-update` | 更新群名称（仅名称，不支持 description） | leaf |
| `dws chat +chat-update-alias` | 设置群备注（仅自己可见） | leaf |
| `dws chat +chat-update-icon` | 更新群头像 | leaf |
| `dws chat +chat-update-nick` | 设置当前用户在群内的群昵称 | leaf |
| `dws chat +chat-update-settings` | 更新群设置（settingKey + status） | leaf |
| `dws chat +conversation-clear-all-red-point` | 清除所有会话红点（全部已读） | leaf |
| `dws chat +conversation-clear-messages` | 清空当前用户指定会话的聊天记录（仅本人视角，不可逆） | leaf |
| `dws chat +conversation-clear-red-point` | 清除会话红点 | leaf |
| `dws chat +conversation-hide` | 会话列表中隐藏会话（收到新消息会重新出现） | leaf |
| `dws chat +conversation-info` | 获取会话信息（群聊传 --group，单聊传 --open-dingtalk-id） | leaf |
| `dws chat +conversation-list` | 分页或一键全量获取当前用户的会话列表（单聊+群聊） | leaf |
| `dws chat +conversation-list-top` | 拉取置顶会话列表，可只看群聊或单聊 | leaf |
| `dws chat +conversation-mark-read` | 标记消息已读（该消息及之前的消息都标记为已读） | leaf |
| `dws chat +conversation-mark-unread` | 标记会话为未读 | leaf |
| `dws chat +conversation-mute` | 会话消息免打扰（支持单聊/群聊） | leaf |
| `dws chat +conversation-set-top` | 批量会话置顶 / 取消置顶（最多 10 个） | leaf |
| `dws chat +dm` | 按姓名直接给某人发单聊消息（自动解析唯一 openDingTalkId） | leaf |
| `dws chat +feed-group-query-item` | 在会话分组结果中按会话 ID 精确查询多项 | leaf |
| `dws chat +feed-shortcut-create` | 置顶指定会话（固定设置置顶；不支持 head/tail 顺序） | leaf |
| `dws chat +feed-shortcut-remove` | 取消指定会话置顶（固定取消，不需要 --off） | leaf |
| `dws chat +flag-cancel` | 取消收藏一条或多条消息（最多 10 条） | leaf |
| `dws chat +flag-create` | 收藏一条或多条消息（最多 10 条） | leaf |
| `dws chat +flag-list` | 分页查询当前用户收藏的消息，支持有界自动翻页 | leaf |
| `dws chat +group-members` | 按群名唯一解析后全量列出用户成员并公开分页完整性 | leaf |
| `dws chat +messages-add-emoji` | 对消息添加 emoji 表情回应 | leaf |
| `dws chat +messages-add-text-emotion` | 对消息添加文字表情回应 | leaf |
| `dws chat +messages-batch-recall-by-bot` | 机器人撤回单聊消息 | leaf |
| `dws chat +messages-batch-send-by-bot` | 机器人批量向用户发送单聊 Markdown 消息 | leaf |
| `dws chat +messages-combine-forward` | 合并转发多条消息 | leaf |
| `dws chat +messages-create-text-emotion` | 创建文字表情（获取 emotionId） | leaf |
| `dws chat +messages-edit` | 编辑当前用户自己的 Markdown 消息（不支持 Bot 普通消息编辑） | leaf |
| `dws chat +messages-forward` | 转发单条消息 | leaf |
| `dws chat +messages-forward-topic` | 转发话题消息到目标会话 | leaf |
| `dws chat +messages-list` | 拉取群聊会话消息 | leaf |
| `dws chat +messages-list-direct` | 拉取单聊会话消息 | leaf |
| `dws chat +messages-list-pin` | 拉取会话中钉住的消息列表 | leaf |
| `dws chat +messages-list-unread-conversations` | 获取有未读消息的会话列表 | leaf |
| `dws chat +messages-mget` | 根据消息 ID 批量查询消息（最多 50 条） | leaf |
| `dws chat +messages-query-send-status` | 查询消息投递状态并衔接后续消息操作 | leaf |
| `dws chat +messages-read-status` | 查询消息的已读/未读状态 | leaf |
| `dws chat +messages-recall` | 撤回当前用户发送的消息 | leaf |
| `dws chat +messages-recall-by-bot` | 机器人撤回群消息 | leaf |
| `dws chat +messages-remove-emoji` | 移除消息的 emoji 表情回应 | leaf |
| `dws chat +messages-remove-text-emotion` | 移除消息的文字表情回应 | leaf |
| `dws chat +messages-reply` | 统一回复已有消息：个人群/单聊引用、个人 Thread 追加、Bot 群引用 | leaf |
| `dws chat +messages-resource-download` | 安全下载消息资源（图片/视频/语音/文件）到本地 | leaf |
| `dws chat +messages-resource-url` | 获取消息资源（图片/视频/语音）下载链接 | leaf |
| `dws chat +messages-send` | 按身份和目标统一发送消息，Bot 多群返回逐目标 ledger | leaf |
| `dws chat +messages-send-by-bot` | 机器人向群聊发送 Markdown 消息 | leaf |
| `dws chat +messages-send-by-webhook` | 兼容旧入口的自定义机器人 Webhook 群消息发送 | leaf |
| `dws chat +messages-send-card` | 创建流式卡片，可在同一次调用中写入内容并结束；群聊创建时可 @成员或 @所有人 | leaf |
| `dws chat +messages-set-pin` | 钉住消息（Pin） | leaf |
| `dws chat +messages-set-top` | 置顶消息 | leaf |
| `dws chat +messages-unset-pin` | 取消钉住消息（Unpin） | leaf |
| `dws chat +messages-unset-top` | 取消置顶消息 | leaf |
| `dws chat +messages-update-card` | 流式更新卡片内容（最后一次 --flow-status 应为 3） | leaf |
| `dws chat +my-groups` | 列出我加入的群，可按类型过滤并投影关键字段 | leaf |
| `dws chat +recent-conversations` | 列出指定时间以来有新消息的单聊和群聊 | leaf |
| `dws chat +search-msg` | 按稳定 ID、内容、时间等条件搜索消息，可校验会话范围、全量翻页并批量富化 | leaf |
| `dws chat +send-to-group` | 按群名或 openConversationId 直接给群发消息 | leaf |
| `dws chat +thread-replies` | 按主消息 ID 或 thread/topic ID 分页读取话题回复，支持完整排序与有界自动翻页 | leaf |
| `dws chat +unread-chats` | 列出我有未读消息的会话（投影会话名/未读数/会话ID） | leaf |
| `dws chat bot` | 机器人管理 | runnable |
| `dws chat bot find` | 搜索【全部可用】机器人（含他人/官方，额外返回 openDingTalkId 可发单聊） | leaf |
| `dws chat bot search` | 搜索【我自己创建】的机器人（仅本人创建的，不含他人/官方机器人） | leaf |
| `dws chat category` | 会话分组管理 | runnable |
| `dws chat category add-conv` | 将会话移动到指定的自定义分组中 | leaf |
| `dws chat category batch-info` | 批量拉取用户自定义会话分组信息 | leaf |
| `dws chat category create` | 创建用户自定义会话分组 | leaf |
| `dws chat category create-smart` | 创建智能会话分组 | leaf |
| `dws chat category delete` | 删除用户自定义会话分组 | leaf |
| `dws chat category list` | 获取用户自定义会话分组 | leaf |
| `dws chat category list-by-conv` | 拉取指定会话所属的用户自定义会话分组 | leaf |
| `dws chat category list-conversations` | 拉取指定自定义会话分组下的会话 | leaf |
| `dws chat category remove-conv` | 将会话从指定的自定义分组中移出 | leaf |
| `dws chat category rename` | 更新用户自定义会话分组的名称 | leaf |
| `dws chat chmod` | 授予 chat 高风险操作权限 | leaf |
| `dws chat clear-all-red-point` | 清除所有会话红点（全部已读） | leaf |
| `dws chat clear-messages` | 清空当前用户指定会话的聊天记录 | leaf |
| `dws chat clear-red-point` | 清除会话红点 | leaf |
| `dws chat conversation-file` | 会话文件空间管理 | runnable |
| `dws chat conversation-file upload` | 上传本地文件到会话文件空间，不发送消息 | leaf |
| `dws chat conversation-info` | 获取会话基础信息 | leaf |
| `dws chat crypto` | 消息三方解密 | runnable |
| `dws chat crypto decrypt` | 解密一条三方密文消息 | leaf |
| `dws chat data-auth` | 授予 chat 数据读取权限 | runnable |
| `dws chat data-auth cross-org` | 授予跨组织 chat 数据访问权限 | leaf |
| `dws chat emotion` | 个人收藏表情 | runnable |
| `dws chat emotion favorite` | 新增个人收藏表情 | leaf |
| `dws chat emotion list` | 列出个人收藏表情 | leaf |
| `dws chat emotion send` | 发送个人收藏表情 | leaf |
| `dws chat group` | 群组管理 | runnable |
| `dws chat group audit-join-validation` | 审批入群验证（通过、拒绝、删除） | leaf |
| `dws chat group bots` | 查看群内所有机器人 | leaf |
| `dws chat group create` | 创建群（支持内部群/外部群/普通群） | leaf |
| `dws chat group dismiss` | 解散群聊 | leaf |
| `dws chat group get-by-group-id` | 根据群号获取群聊信息 | leaf |
| `dws chat group get-mute-config` | 查询群用户禁言配置 | leaf |
| `dws chat group invite-url` | 获取群邀请链接 | leaf |
| `dws chat group list-all` | 分页拉取我所有群列表 | leaf |
| `dws chat group list-join-validations` | 分页拉取入群验证记录 | leaf |
| `dws chat group list-my-groups` | 拉取我创建/管理的群 | leaf |
| `dws chat group members` | 群成员管理 | runnable |
| `dws chat group members add` | 添加群成员 | leaf |
| `dws chat group members add-bot` | 将机器人添加到群中 | leaf |
| `dws chat group members list-by-ids` | 根据成员 ID 批量查询群成员详情 | leaf |
| `dws chat group members remove` | 移除群成员 | leaf |
| `dws chat group members remove-bot` | 从群内移除机器人 | leaf |
| `dws chat group notice` | 群公告管理 | runnable |
| `dws chat group notice create` | 发布群公告 | leaf |
| `dws chat group notice edit` | 修改群公告 | leaf |
| `dws chat group notice get` | 查看群公告详情 | leaf |
| `dws chat group notice list` | 查看群公告列表 | leaf |
| `dws chat group quit` | 退出群聊 | leaf |
| `dws chat group rename` | 更新群名称 | leaf |
| `dws chat group set-admin` | 设置 / 取消群管理员 | leaf |
| `dws chat group set-history` | 设置新成员入群可查看历史消息选项 | leaf |
| `dws chat group share-invite` | 分享群聊链接到会话 | leaf |
| `dws chat group transfer-owner` | 转让群主 | leaf |
| `dws chat group update-alias` | 设置群备注 | leaf |
| `dws chat group update-icon` | 更新群头像 | leaf |
| `dws chat group update-nick` | 设置或清除用户在群内的群昵称 | leaf |
| `dws chat group update-settings` | 更新群设置 | leaf |
| `dws chat group upgrade-to-external` | [危险] 将普通群升级为外部群 | leaf |
| `dws chat group user-settings` | 批量查询或更新当前用户的群会话设置 | runnable |
| `dws chat group user-settings query` | 批量查询当前用户的群会话设置 | leaf |
| `dws chat group user-settings set` | 批量更新当前用户的群会话设置 | leaf |
| `dws chat group-mute` | 全员禁言 / 取消全员禁言 | leaf |
| `dws chat group-mute-member` | 指定群成员禁言 / 取消禁言 | leaf |
| `dws chat group-role` | 群身份管理 | runnable |
| `dws chat group-role add` | 添加群身份 | leaf |
| `dws chat group-role list` | 拉取会话的群身份列表 | leaf |
| `dws chat group-role query-user` | 查询群成员的群身份 | leaf |
| `dws chat group-role remove` | 删除群身份 | leaf |
| `dws chat group-role remove-user` | 移除用户的指定群身份 | leaf |
| `dws chat group-role set-user` | 设置用户的群身份（覆盖该用户的全部群身份） | leaf |
| `dws chat group-role update` | 更新群身份名称 | leaf |
| `dws chat hide` | 会话列表中隐藏会话 | leaf |
| `dws chat list-all-conversations` | 分页获取当前用户的全部会话列表 | leaf |
| `dws chat list-top-conversations` | 拉取置顶会话列表 | leaf |
| `dws chat mark-read` | 标记消息已读 | leaf |
| `dws chat mark-unread` | 标记会话为未读 | leaf |
| `dws chat media` | 已下线：媒体文件上传兼容入口 | runnable |
| `dws chat media upload` | 已下线：请通过 chat message send 直接发送本地文件 | leaf |
| `dws chat message` | 会话消息管理 | runnable |
| `dws chat message add-emoji` | 对消息添加 emoji 表情回应 | leaf |
| `dws chat message add-favorite` | 收藏指定消息 | leaf |
| `dws chat message add-text-emotion` | 对消息添加文字表情回应 | leaf |
| `dws chat message combine-forward` | 合并转发多条消息 | leaf |
| `dws chat message create-text-emotion` | 创建文字表情（获取 emotionId） | leaf |
| `dws chat message download-media` | 下载消息中的资源（图片/视频/语音等）到本地 | leaf |
| `dws chat message edit` | 编辑消息 | leaf |
| `dws chat message forward` | 转发单条消息（源/目标会话均支持单聊/群聊） | leaf |
| `dws chat message list` | 拉取会话消息内容 | leaf |
| `dws chat message list-all` | 拉取指定时间范围内当前用户的所有会话消息 | leaf |
| `dws chat message list-by-ids` | 根据消息 ID 批量查询消息 | leaf |
| `dws chat message list-by-sender` | 拉取指定发送者的消息（包含单聊和群聊） | leaf |
| `dws chat message list-emotion-replies` | 批量拉取消息的表情回复和文字回复 | leaf |
| `dws chat message list-favorites` | 查询收藏的消息列表 | leaf |
| `dws chat message list-focused` | 拉取特别关注人的消息 | leaf |
| `dws chat message list-mentions` | 拉取 @我 的消息 | leaf |
| `dws chat message list-pin-msg` | 拉取会话中钉住的消息列表 | leaf |
| `dws chat message list-unread-conversations` | 获取未读会话列表 | leaf |
| `dws chat message query-send-status` | 查询消息发送状态 | leaf |
| `dws chat message read-status` | 查询消息的已读/未读状态 | leaf |
| `dws chat message recall` | 撤回用户发送的消息 | leaf |
| `dws chat message recall-by-bot` | 机器人撤回消息（--conversation-id 群聊 / 不传为单聊） | leaf |
| `dws chat message remove-emoji` | 移除消息的 emoji 表情回应 | leaf |
| `dws chat message remove-favorite` | 取消收藏指定消息 | leaf |
| `dws chat message remove-text-emotion` | 移除消息的文字表情回应 | leaf |
| `dws chat message reply` | 引用回复消息（支持单聊/群聊） | leaf |
| `dws chat message search` | 按关键词搜索消息 | leaf |
| `dws chat message search-advanced` | 多维度搜索消息 | leaf |
| `dws chat message send` | 以当前用户身份发送消息（--conversation-id 群聊 / --user 或 --open-dingtalk-id 单聊） | leaf |
| `dws chat message send-a2ui-card` | 创建并推送 A2UI 卡片 | leaf |
| `dws chat message send-by-bot` | 机器人发送消息（--conversation-id 群聊 / --users 单聊） | leaf |
| `dws chat message send-by-webhook` | 自定义机器人 Webhook 发送群消息 | leaf |
| `dws chat message send-card` | 创建并推送流式卡片 | leaf |
| `dws chat message set-pin-msg` | 钉住消息（Pin） | leaf |
| `dws chat message set-top-msg` | 置顶消息 | leaf |
| `dws chat message unset-pin-msg` | 取消钉住消息（Unpin） | leaf |
| `dws chat message unset-top-msg` | 取消置顶消息 | leaf |
| `dws chat message update-a2ui-card` | 更新 A2UI 卡片内容和状态 | leaf |
| `dws chat message update-card` | 流式更新卡片内容 | leaf |
| `dws chat message update-text-emotion` | 更新消息的文字表情回应 | leaf |
| `dws chat mute` | 会话消息免打扰 | leaf |
| `dws chat mute-at-all` | 关闭/开启 @所有人消息提醒 | leaf |
| `dws chat mute-red-envelope` | 关闭/开启红包消息提醒 | leaf |
| `dws chat search` | 根据关键词搜索群聊 | leaf |
| `dws chat search-common` | 搜索共同群（查询指定人共同所在的群聊） | leaf |
| `dws chat set-top` | 会话置顶 / 取消置顶（支持单聊/群聊） | leaf |
| `dws chat text` | 文本内容处理 | runnable |
| `dws chat text translate` | 翻译文本内容 | leaf |
| `dws chat thread` | 群聊话题（Thread）管理 | runnable |
| `dws chat thread add-emoji` | 给 Thread 消息添加 emoji | leaf |
| `dws chat thread add-text-emotion` | 给 Thread 消息添加文字表情或状态 | leaf |
| `dws chat thread create-group` | 创建话题圈 | leaf |
| `dws chat thread forward` | 转发 Thread 到目标会话 | leaf |
| `dws chat thread list` | 分页列出群聊中的话题主消息 | leaf |
| `dws chat thread list-emotion-replies` | 查询 Thread 消息的 emoji 与文字状态 | leaf |
| `dws chat thread list-replies` | 分页读取指定 Thread 的回复 | leaf |
| `dws chat thread promote` | 将普通群中的已有消息升级为 Thread | leaf |
| `dws chat thread recall-message` | 撤回 Thread 中的一条消息 | leaf |
| `dws chat thread remove-emoji` | 移除 Thread 消息上的 emoji | leaf |
| `dws chat thread remove-text-emotion` | 移除 Thread 消息上的文字表情或状态 | leaf |
| `dws chat thread reply` | 向指定 openConvThreadId 直接追加回复（非引用回复） | leaf |
| `dws chat thread send` | 发布一条新话题 | leaf |
| `dws chat thread update-text-emotion` | 更新 Thread 消息上的文字表情或状态 | leaf |
| `dws chat toolbar` | 快捷栏管理 | runnable |
| `dws chat toolbar add` | 将入口添加到快捷栏可见区 | leaf |
| `dws chat toolbar create-custom` | 创建自定义快捷栏入口 | leaf |
| `dws chat toolbar hide` | 将入口从快捷栏可见区隐藏 | leaf |
| `dws chat toolbar list` | 查询会话快捷栏入口列表 | leaf |
| `dws chat toolbar remove-custom` | 删除自定义快捷栏入口 | leaf |
| `dws chat toolbar sort` | 排序快捷栏入口 | leaf |
| `dws chat toolbar update-custom` | 更新自定义快捷栏入口 | leaf |

## `dws completion`

| Command | Description | Type |
|---|---|---|
| `dws completion` | 生成 Shell 自动补全脚本 | leaf |

## `dws config`

| Command | Description | Type |
|---|---|---|
| `dws config` | 配置管理 | runnable |
| `dws config list` | 列出所有可用配置项 | leaf |

## `dws contact`

| Command | Description | Type |
|---|---|---|
| `dws contact` | 通讯录 / 用户 / 部门 / 角色 / 人员关系 | runnable |
| `dws contact +by-mobile` | 按手机号查询某人的完整资料（自动解析 userId 后取详情） | leaf |
| `dws contact +dept-members` | 按部门名列出部门成员（自动解析 deptId） | leaf |
| `dws contact +list-dept-members` | 查看部门成员（仅本部门，不含下级） | leaf |
| `dws contact +list-followings` | 获取当前用户的特别关注列表 | leaf |
| `dws contact +list-role-members` | 查询角色下的成员列表 | leaf |
| `dws contact +list-roles` | 获取企业所有角色（标签）列表 | leaf |
| `dws contact +list-sub-depts` | 查看指定部门的子部门 | leaf |
| `dws contact +lookup` | 按姓名查询某人的完整资料（自动解析 userId 后取详情） | leaf |
| `dws contact +me` | 查看我自己的通讯录资料（姓名/userId/手机/部门/组织，干净投影） | leaf |
| `dws contact +org` | 按姓名查某人所在部门的详情（自动解析 userId 与 deptId） | leaf |
| `dws contact +resolve-dept` | 按名称搜索部门并解析出唯一 deptId（只读） | leaf |
| `dws contact +search-mobile` | 按手机号搜索通讯录用户 | leaf |
| `dws contact +search-user` | 按关键词搜索通讯录用户 | leaf |
| `dws contact +team` | 按姓名列出某人所在部门的成员（自动解析 userId 与 deptId） | leaf |
| `dws contact account` | 企业账号管理 | runnable |
| `dws contact account create` | 创建企业专属账号 | leaf |
| `dws contact account update` | 更新企业账号用户信息 | leaf |
| `dws contact dept` | 部门管理 | runnable |
| `dws contact dept create` | 创建部门 | leaf |
| `dws contact dept get-info` | 获取部门详情（部门ID、名称、人数） | leaf |
| `dws contact dept invite-audit` | 设置加入部门申请免审核（部门级） | leaf |
| `dws contact dept list-children` | 查看子部门 | leaf |
| `dws contact dept list-members` | 查看部门成员（仅本部门，不含下级） | leaf |
| `dws contact dept search` | 搜索部门 | leaf |
| `dws contact dept update` | 更新部门信息 | leaf |
| `dws contact exclusive-account` | 企业账号状态管理 | runnable |
| `dws contact exclusive-account disable` | 停用企业账号 | leaf |
| `dws contact exclusive-account enable` | 启用企业账号 | leaf |
| `dws contact ext-field` | 自定义成员字段管理 | runnable |
| `dws contact ext-field create` | 创建自定义字段 | leaf |
| `dws contact ext-field delete` | 删除自定义字段 | leaf |
| `dws contact ext-field list` | 列出企业自定义成员字段 | leaf |
| `dws contact ext-field update` | 更新自定义字段设置 | leaf |
| `dws contact label` | 角色查询与管理 | runnable |
| `dws contact label add-members` | 给成员添加角色 | leaf |
| `dws contact label create` | 创建角色或角色组 | leaf |
| `dws contact label delete` | 删除角色或角色组 | leaf |
| `dws contact label get` | 根据角色名称查询角色 | leaf |
| `dws contact label list` | 获取企业所有角色列表 | leaf |
| `dws contact label list-members` | 查询角色下的成员 | leaf |
| `dws contact label remove-members` | 移除成员角色 | leaf |
| `dws contact label update` | 修改角色名称 | leaf |
| `dws contact label update-member-scope` | 修改角色管理范围 | leaf |
| `dws contact org` | 企业管理 | runnable |
| `dws contact org apply-approve` | 同意加入企业申请 | leaf |
| `dws contact org apply-block` | 屏蔽加入企业申请 | leaf |
| `dws contact org apply-list` | 查询加入企业申请列表 | leaf |
| `dws contact org apply-reject` | 拒绝加入企业申请 | leaf |
| `dws contact org apply-remove` | 删除加入企业申请 | leaf |
| `dws contact org create` | 创建企业 | leaf |
| `dws contact org invite-audit` | 设置加入企业申请免审核（企业级） | leaf |
| `dws contact org invite-info` | 获取企业邀请信息 | leaf |
| `dws contact org invite-list` | 查询企业邀请记录列表 | leaf |
| `dws contact org invite-switch` | 设置加入企业申请开关 | leaf |
| `dws contact relation` | 人员关系查询 | runnable |
| `dws contact relation list-my-followings` | 获取当前用户的特别关注列表 | leaf |
| `dws contact user` | 人员管理 | runnable |
| `dws contact user dismission` | 离职员工查询 | runnable |
| `dws contact user dismission search` | 分页获取离职员工列表 | leaf |
| `dws contact user get` | 批量获取用户详情（组织管理信息） | leaf |
| `dws contact user get-by-dingtalk-id` | 按钉钉号获取用户ID | leaf |
| `dws contact user get-self` | 获取当前用户信息（我是谁 / 本人） | leaf |
| `dws contact user invite` | 邀请员工加入企业 | leaf |
| `dws contact user profile` | 用户档案（花名册） | runnable |
| `dws contact user profile fields` | 查询花名册有权限的字段列表 | leaf |
| `dws contact user profile get` | 查询员工花名册字段信息（个人档案） | leaf |
| `dws contact user search` | 按关键词搜索用户 | leaf |
| `dws contact user search-mobile` | 按手机号搜索用户 | leaf |
| `dws contact user update` | 修改员工信息 | leaf |
| `dws contact user update-ownness` | 更新用户个人状态 | leaf |
| `dws contact user update-self` | 更新当前用户自己的 profile 信息 | leaf |

## `dws contract`

| Command | Description | Type |
|---|---|---|
| `dws contract` | 智能合同管理 | runnable |
| `dws contract account` | 合同账款管理 | runnable |
| `dws contract account create` | 创建账款信息 | leaf |
| `dws contract account delete` | 删除账款信息 | leaf |
| `dws contract account get` | 获取账款信息 | leaf |
| `dws contract account list` | 列举账款信息 | leaf |
| `dws contract account update` | 更新账款信息 | leaf |
| `dws contract archive` | 合同文档归档 | leaf |
| `dws contract draft` | 根据听记和模版起草合同 | leaf |
| `dws contract file-directories` | 查询所有合同台账分类 | leaf |
| `dws contract import` | 批量导入合同 | runnable |
| `dws contract import batch` | 从钉盘模版文件创建批量导入任务 | leaf |
| `dws contract import batch-result` | 获取批量合同导入任务结果 | leaf |
| `dws contract process-templates` | 查询当前用户可见审批模板 | leaf |
| `dws contract project` | 合同项目管理 | runnable |
| `dws contract project add` | 新增项目 | leaf |
| `dws contract project delete` | 删除项目（支持批量） | leaf |
| `dws contract project detail` | 查询项目详情 | leaf |
| `dws contract project digests` | 分页查询项目摘要列表 | leaf |
| `dws contract project export` | 项目导出到 Excel | leaf |
| `dws contract project import` | 批量导入项目 | leaf |
| `dws contract project import-result` | 获取项目批量导入结果 | leaf |
| `dws contract project import-template` | 获取批量导入项目模板 | leaf |
| `dws contract project list` | 分页查询项目列表 | leaf |
| `dws contract project set-status` | 更新项目状态 | leaf |
| `dws contract project update` | 更新项目信息 | leaf |
| `dws contract record` | 合同记录 | runnable |
| `dws contract record create` | 创建合同台账 | leaf |
| `dws contract record get` | 查询合同详情 | leaf |
| `dws contract record list` | 查询合同列表 | leaf |
| `dws contract record quantity-by-type` | 按查询维度统计各状态合同数量 | leaf |
| `dws contract review` | 合同审查（已下线） | leaf |
| `dws contract subject` | 合同相对方管理 | runnable |
| `dws contract subject add` | 添加相对方 | leaf |
| `dws contract subject auto-fill` | 相对方信息智能填充 | leaf |
| `dws contract subject base-info` | 查询相对方工商基本信息 | leaf |
| `dws contract subject batch-delete` | 批量删除相对方 | leaf |
| `dws contract subject delete` | 删除相对方（单个） | leaf |
| `dws contract subject detail` | 查询相对方详情 | leaf |
| `dws contract subject detect-risk` | 检测相对方风险 | leaf |
| `dws contract subject export` | 导出相对方到 Excel | leaf |
| `dws contract subject import` | 批量导入相对方 | leaf |
| `dws contract subject import-result` | 查询相对方批量导入结果 | leaf |
| `dws contract subject import-template` | 获取相对方批量导入模板 | leaf |
| `dws contract subject list` | 查询相对方列表 | leaf |
| `dws contract subject sort` | 己方主体排序 | leaf |
| `dws contract subject update` | 修改相对方 | leaf |

## `dws dev`

| Command | Description | Type |
|---|---|---|
| `dws dev` | 开放平台开发者能力 | runnable |
| `dws dev app` | 开放平台应用 | runnable |
| `dws dev app create` | 创建开放平台企业内部应用 | leaf |
| `dws dev app credentials` | 开放平台应用凭证 | runnable |
| `dws dev app credentials get` | 读取开放平台应用凭证 | leaf |
| `dws dev app delete` | 删除开放平台企业内部应用（不可逆，需 --confirm-name 二次确认） | leaf |
| `dws dev app disable` | 停用开放平台企业内部应用 | leaf |
| `dws dev app enable` | 启用开放平台企业内部应用 | leaf |
| `dws dev app event` | 开放平台应用事件订阅 | runnable |
| `dws dev app event list` | 查询应用已订阅的事件列表 | leaf |
| `dws dev app event subscribe` | 订阅应用事件回调 | leaf |
| `dws dev app event unsubscribe` | 取消订阅应用事件 | leaf |
| `dws dev app get` | 查询开放平台企业内部应用详情 | leaf |
| `dws dev app list` | 查询开放平台企业内部应用列表 | leaf |
| `dws dev app member` | 开放平台应用成员管理 | runnable |
| `dws dev app member add` | 添加开放平台应用成员 | leaf |
| `dws dev app member list` | 查询开放平台应用成员 | leaf |
| `dws dev app member remove` | 移除开放平台应用成员 | leaf |
| `dws dev app permission` | 开放平台应用权限 | runnable |
| `dws dev app permission add` | 申请开放平台应用权限点 | leaf |
| `dws dev app permission list` | 查询开放平台应用权限列表 | leaf |
| `dws dev app permission remove` | 取消开放平台应用权限点 | leaf |
| `dws dev app robot` | 开放平台应用机器人能力 | runnable |
| `dws dev app robot config` | 创建或更新现有应用的机器人配置（upsert） | leaf |
| `dws dev app robot disable` | 停用现有应用的机器人能力 | leaf |
| `dws dev app robot enable` | 启用现有应用机器人能力（纯启用，无需配置字段） | leaf |
| `dws dev app robot get` | 查询现有应用的机器人配置 | leaf |
| `dws dev app robot result` | 查询机器人异步创建任务结果 | leaf |
| `dws dev app robot submit` | 异步提交钉钉智能体机器人创建任务（支持失败重试） | leaf |
| `dws dev app security` | 开放平台应用安全设置 | runnable |
| `dws dev app security config` | 更新开放平台应用安全配置 | leaf |
| `dws dev app update` | 修改开放平台企业内部应用基础信息 | leaf |
| `dws dev app version` | 开放平台应用版本发布 | runnable |
| `dws dev app version check-approval` | 预检版本发布是否需要审批（不实际发布） | leaf |
| `dws dev app version create` | 基于当前配置创建应用新版本 | leaf |
| `dws dev app version get` | 查询指定版本详情 | leaf |
| `dws dev app version list` | 分页查询应用版本列表 | leaf |
| `dws dev app version publish` | 发布指定版本（含高敏权限需 --confirmed-sensitive） | leaf |
| `dws dev app version status` | 查询版本发布/审批状态 | leaf |
| `dws dev app webapp` | 开放平台网页应用配置 | runnable |
| `dws dev app webapp config` | 配置网页应用能力 | leaf |
| `dws dev app webapp get` | 查询网页应用配置 | leaf |
| `dws dev connect` | 建联：把现成机器人接到当前本地 agent（起 Stream，不建号） | runnable |
| `dws dev connect list` | 列出本机所有连接器及健康状态（healthy/degraded/down）；--json 供脚本消费 | leaf |
| `dws dev connect restart` | 重启连接器守护进程（通过持久化的 unifiedAppId 重新拉取密钥，无需本地存密钥） | leaf |
| `dws dev connect status` | 查看连接器健康状态（healthy/degraded/down，pid、收发活动、日志路径；--json 供外部托管消费） | leaf |
| `dws dev connect stop` | 优雅停止后台连接器守护进程（释放单实例锁与 Stream 连接） | leaf |
| `dws dev doc` | 开放平台文档搜索 | runnable |
| `dws dev doc search` | 搜索开放平台文档 | leaf |
| `dws dev mcp` | MCP 服务与工具管理 | runnable |
| `dws dev mcp auth` | MCP 下游鉴权配置 | runnable |
| `dws dev mcp auth get` | 查询 MCP 下游鉴权配置 | leaf |
| `dws dev mcp auth save` | 保存 MCP 下游鉴权配置 | leaf |
| `dws dev mcp credential` | MCP 凭证账号管理 | runnable |
| `dws dev mcp credential bind` | 绑定 MCP 凭证账号 | leaf |
| `dws dev mcp credential debug` | 调试 MCP 凭证账号（会真实调用下游接口，TOKEN 型含现场换 token） | leaf |
| `dws dev mcp credential delete` | 删除 MCP 凭证账号（不可恢复） | leaf |
| `dws dev mcp credential get` | 查询 MCP 凭证账号详情 | leaf |
| `dws dev mcp credential list` | 查询 MCP 凭证账号列表 | leaf |
| `dws dev mcp credential save` | 新增或修改 MCP 凭证账号（TOKEN 型会现场调换 token 接口验密钥，密钥无效则保存失败） | leaf |
| `dws dev mcp credential unbind` | 解绑发布实例的生效凭证（bind 的逆操作；credential delete 报 credential_in_use 时先走本命令） | leaf |
| `dws dev mcp hsf` | HSF 方法发现（建 hsf 型工具前查方法与 DTO 字段名） | runnable |
| `dws dev mcp hsf method-list` | 查询 HSF 接口的方法清单（建 hsf 工具前的方法发现，含每方法出入参 schema） | leaf |
| `dws dev mcp member` | MCP 开发协作者管理 | runnable |
| `dws dev mcp member add` | 新增 MCP 开发协作者 | leaf |
| `dws dev mcp member list` | 查询 MCP 开发协作者列表 | leaf |
| `dws dev mcp member remove` | 移除 MCP 开发协作者 | leaf |
| `dws dev mcp service` | MCP 服务管理 | runnable |
| `dws dev mcp service create` | 新建 MCP 服务 | leaf |
| `dws dev mcp service delete` | 删除 MCP 服务（不可恢复） | leaf |
| `dws dev mcp service get` | 查询 MCP 服务详情 | leaf |
| `dws dev mcp service list` | 查询有开发权限的 MCP 服务列表（含 serverName） | leaf |
| `dws dev mcp service update` | 修改 MCP 服务信息 | leaf |
| `dws dev mcp tool` | MCP 工具管理 | runnable |
| `dws dev mcp tool create` | 新建 MCP 工具草稿 | leaf |
| `dws dev mcp tool create-hsf` | 新建 HSF 型 MCP 工具草稿（apiInputs/apiOutputs 由服务端按方法 schema 自动生成） | leaf |
| `dws dev mcp tool debug` | 调试 MCP 工具 | leaf |
| `dws dev mcp tool delete` | 删除 MCP 工具（不可恢复） | leaf |
| `dws dev mcp tool get` | 读取 MCP 工具定义 | leaf |
| `dws dev mcp tool list` | 查询 MCP 服务下的工具列表 | leaf |
| `dws dev mcp tool publish` | 发布 MCP 工具草稿 | leaf |
| `dws dev mcp tool update` | 编辑 MCP 工具并保存为草稿 | leaf |
| `dws dev mcp tool update-hsf` | 编辑 HSF 型工具（⚠️部分更新语义：只传要改的字段，未传保持原值——与 http 版全量提交完全相反） | leaf |
| `dws dev mcp tool versions` | 查询 MCP 工具版本历史 | leaf |
| `dws dev mcp url` | MCP 接入地址 | runnable |
| `dws dev mcp url get` | 获取 MCP 实例接入地址（按调用者个人身份生成，含个人 key 勿外发） | leaf |

## `dws devapp`

| Command | Description | Type |
|---|---|---|
| `dws devapp` | devapp shortcuts | runnable |
| `dws devapp +create` | 创建开放平台企业内部应用 | leaf |
| `dws devapp +credentials-get` | 读取开放平台应用凭证 | leaf |
| `dws devapp +delete` | 删除开放平台企业内部应用（不可逆） | leaf |
| `dws devapp +disable` | 停用开放平台企业内部应用 | leaf |
| `dws devapp +enable` | 启用开放平台企业内部应用 | leaf |
| `dws devapp +event-list` | 查询应用可用事件目录与订阅状态 | leaf |
| `dws devapp +event-subscribe` | 订阅应用事件回调 | leaf |
| `dws devapp +get` | 查询开放平台企业内部应用详情 | leaf |
| `dws devapp +list` | 查询开放平台企业内部应用列表 | leaf |
| `dws devapp +member-add` | 添加开放平台应用成员 | leaf |
| `dws devapp +member-list` | 查询开放平台应用成员 | leaf |
| `dws devapp +member-remove` | 移除开放平台应用成员 | leaf |
| `dws devapp +permission-list` | 查询开放平台应用权限列表 | leaf |
| `dws devapp +robot-config` | 创建或更新现有应用的机器人配置（upsert） | leaf |
| `dws devapp +robot-disable` | 停用现有应用的机器人能力 | leaf |
| `dws devapp +robot-enable` | 启用现有应用机器人能力（纯启用，无需配置字段） | leaf |
| `dws devapp +robot-get` | 查询现有应用的机器人配置 | leaf |
| `dws devapp +update` | 修改开放平台企业内部应用基础信息 | leaf |
| `dws devapp +version-check-approval` | 预检版本发布是否需要审批（不实际发布） | leaf |
| `dws devapp +version-create` | 基于当前配置创建应用新版本 | leaf |
| `dws devapp +version-get` | 查询指定版本详情 | leaf |
| `dws devapp +version-list` | 分页查询应用版本列表 | leaf |
| `dws devapp +version-status` | 查询版本发布/审批状态 | leaf |
| `dws devapp +webapp-config` | 配置网页应用能力 | leaf |
| `dws devapp +webapp-get` | 查询网页应用配置 | leaf |

## `dws devdoc`

| Command | Description | Type |
|---|---|---|
| `dws devdoc` | 开放平台文档搜索 | runnable |
| `dws devdoc article` | 文档文章 | runnable |
| `dws devdoc article search` | 搜索开放平台文档 | leaf |

## `dws ding`

| Command | Description | Type |
|---|---|---|
| `dws ding` | DING 消息 / 发送 / 撤回 | runnable |
| `dws ding +list` | 查询 DING 消息列表 | leaf |
| `dws ding +recall-personal` | 撤回本人发起的 DING | leaf |
| `dws ding +receiver-status` | 查询 DING 消息接收人已读状态 | leaf |
| `dws ding +send-personal` | 以本人身份发送 DING 给指定人 | leaf |
| `dws ding message` | DING 消息管理 | runnable |
| `dws ding message list` | 查询 DING 消息历史 | leaf |
| `dws ding message recall` | 撤回 DING 消息 | leaf |
| `dws ding message recall-personal` | 以用户身份撤回 DING | leaf |
| `dws ding message receiver-status` | 查看 DING 接收状态 | leaf |
| `dws ding message send` | 发送 DING 消息 | leaf |
| `dws ding message send-by-message` | 消息转 DING（将聊天消息转为 DING 通知） | leaf |
| `dws ding message send-personal` | 以用户身份发送 DING | leaf |

## `dws doc`

| Command | Description | Type |
|---|---|---|
| `dws doc` | 钉钉文档管理 | runnable |
| `dws doc +access-change` | 预检已有协作者后变更文档角色 | leaf |
| `dws doc +access-grant` | 按姓名解析后批量授予文档权限 | leaf |
| `dws doc +access-revoke` | 预检并移除指定协作者的文档权限 | leaf |
| `dws doc +background-delete` | 清除文档背景色 | leaf |
| `dws doc +background-update` | 设置文档 #RRGGBB 背景纯色 | leaf |
| `dws doc +checkpoint-update` | 先保存可回滚版本，再更新并读回验证 | leaf |
| `dws doc +comment-create` | 创建全文评论，或按 selection 创建划词评论 | leaf |
| `dws doc +comment-delete` | 永久删除指定文档评论 | leaf |
| `dws doc +comment-list` | 查询文档评论列表 | leaf |
| `dws doc +comment-reply` | 回复文档中的一条评论 | leaf |
| `dws doc +comment-update` | 更新指定文档评论正文和 mention | leaf |
| `dws doc +copy` | 复制文档/文件到指定文件夹或知识库 | leaf |
| `dws doc +create` | 从 Markdown 或 JSONML 创建在线文字文档 | leaf |
| `dws doc +create-from-template` | 使用已选定的 templateId 创建文档 | leaf |
| `dws doc +create-with-media` | 确认后创建文档并按顺序上传本地图片和附件 | leaf |
| `dws doc +doc-append` | 在文档末尾追加一段文本（安全追加，不改动原有内容） | leaf |
| `dws doc +download-overwrite` | 确认后下载正文媒体或封面，允许原子替换已有普通文件 | leaf |
| `dws doc +export` | 提交、轮询并安全下载在线文档导出文件 | leaf |
| `dws doc +export-get` | 根据 jobId 查询文档导出任务结果 | leaf |
| `dws doc +export-submit` | 提交在线文档导出任务 (docx/markdown/pdf)，返回 jobId | leaf |
| `dws doc +fetch` | 读取完整或局部文档内容，并按 detail 控制保真度 | leaf |
| `dws doc +find-doc` | 按关键词搜索云文档并投影关键字段（只读） | leaf |
| `dws doc +grant-and-share` | 确保目标角色后按姓名逐人发送文档链接 | leaf |
| `dws doc +history-list` | 兼容入口：分页列出文档历史版本 | leaf |
| `dws doc +history-revert` | 兼容入口：预检并回滚文档到指定历史版本 | leaf |
| `dws doc +history-save` | 兼容入口：手动保存当前文档版本快照 | leaf |
| `dws doc +import` | 上传本地文件并等待转换成在线文档对象；白名单外格式自动改走文件上传原样入库 | leaf |
| `dws doc +inspect` | 聚合文档元信息，并按需附带样式、权限、历史、媒体和评论 | leaf |
| `dws doc +list` | 列出文件夹或知识库下的直接子节点 | leaf |
| `dws doc +media-download` | 安全下载文档正文附件到工作目录 | leaf |
| `dws doc +media-insert` | 上传本地图片或文件并插入文档正文 | leaf |
| `dws doc +media-list` | 列出文档正文中的图片和附件资源 | leaf |
| `dws doc +media-preview` | 下载正文媒体到受控临时目录并返回预览路径 | leaf |
| `dws doc +media-upload` | 上传同文档可复用媒体并下载校验字节，不插入正文 | leaf |
| `dws doc +move` | 移动文档/文件到指定文件夹或知识库 | leaf |
| `dws doc +resource-delete` | 幂等清除文档封面 | leaf |
| `dws doc +resource-download` | 读取并安全下载当前文档封面 | leaf |
| `dws doc +resource-update` | 从本地图片或 HTTPS URL 设置文档封面 | leaf |
| `dws doc +review` | 聚合未解决评论、引用原文和块上下文 | leaf |
| `dws doc +script` | 初始化本地文档草稿，或检查Markdown/JSONML结构和字数 | leaf |
| `dws doc +search` | 按关键词或过滤条件搜索有权限的文档；默认只读取一页 | leaf |
| `dws doc +share` | 按姓名发送文档链接，不改变文档权限 | leaf |
| `dws doc +share-doc` | 按姓名把文档链接私信发给某人（自动解析 userId） | leaf |
| `dws doc +template-list` | 浏览当前用户可用的 MY/PUBLIC 文档模板；默认只读取一页 | leaf |
| `dws doc +template-search` | 按名称或关键词检索文档模板 | leaf |
| `dws doc +update` | 追加、覆盖或按 block 精确更新文档内容 | leaf |
| `dws doc +version-list` | 分页列出文档历史版本 | leaf |
| `dws doc +version-revert` | 预检并回滚文档到指定历史版本 | leaf |
| `dws doc +version-save` | 手动保存当前文档版本快照 | leaf |
| `dws doc block` | 块级编辑 | runnable |
| `dws doc block delete` | 删除块元素 | leaf |
| `dws doc block insert` | 插入块元素 | leaf |
| `dws doc block list` | 查询块元素 | leaf |
| `dws doc block update` | 更新块元素 | leaf |
| `dws doc comment` | 文档评论 / 评论管理 | runnable |
| `dws doc comment batch-query` | 按 topicId + commentKey 批量查询评论详情 | leaf |
| `dws doc comment create` | 创建文档评论 | leaf |
| `dws doc comment create-inline` | 创建划词评论 | leaf |
| `dws doc comment delete` | 删除文档评论 | leaf |
| `dws doc comment list` | 查询文档评论列表 | leaf |
| `dws doc comment list-replies` | 分页查询评论的直接子回复 | leaf |
| `dws doc comment react-reply` | 创建一条表情回复 | leaf |
| `dws doc comment reply` | 回复评论 | leaf |
| `dws doc comment resolve` | 将评论标记为已解决 | leaf |
| `dws doc comment restore` | 将已解决评论恢复为未解决 | leaf |
| `dws doc comment update` | 更新文档评论 | leaf |
| `dws doc create` | 创建文档 | leaf |
| `dws doc export` | 导出在线文档 (支持 docx / markdown / pdf) | runnable |
| `dws doc export get` | 查询导出任务结果（手动兜底） | leaf |
| `dws doc file` | 文件管理 | runnable |
| `dws doc file create` | 创建文件 | leaf |
| `dws doc import` | 导入本地文件为在线文档 (支持 docx / xlsx / md 等) | runnable |
| `dws doc import get` | 查询导入任务结果（手动兜底） | leaf |
| `dws doc info` | 获取文档元信息 | leaf |
| `dws doc media` | 文档媒体 / 附件管理 | runnable |
| `dws doc media download` | 下载文档附件 | leaf |
| `dws doc media insert` | 上传附件并插入文档 | leaf |
| `dws doc media upload` | 上传可复用的文档媒体资源 | leaf |
| `dws doc read` | 读取文档内容 (Markdown) | leaf |
| `dws doc style` | 文档样式配置 (封面/背景) | runnable |
| `dws doc style background` | 文档背景设置/清除 | runnable |
| `dws doc style background clear` | 清除文档背景 | leaf |
| `dws doc style background set` | 设置文档背景纯色 | leaf |
| `dws doc style cover` | 文档封面设置/移除 | runnable |
| `dws doc style cover clear` | 移除文档封面 | leaf |
| `dws doc style cover set` | 设置文档封面 | leaf |
| `dws doc style get` | 读取文档封面/背景 (只读) | leaf |
| `dws doc template` | 文档模板管理 | runnable |
| `dws doc template apply` | 应用文档模板 | leaf |
| `dws doc template list` | 获取文档模板列表 | leaf |
| `dws doc template search` | 搜索文档模板 | leaf |
| `dws doc update` | 更新文档内容 | leaf |
| `dws doc version` | 文档历史版本管理 | runnable |
| `dws doc version list` | 查看文档历史版本列表 | leaf |
| `dws doc version revert` | 回滚文档到指定版本 | leaf |
| `dws doc version save` | 手动保存文档版本快照 | leaf |
| `dws doc whiteboard` | 白板卡片管理 | runnable |
| `dws doc whiteboard insert` | 插入白板卡片 | leaf |

## `dws doctor`

| Command | Description | Type |
|---|---|---|
| `dws doctor` | 环境健康检查 | leaf |

## `dws drive`

| Command | Description | Type |
|---|---|---|
| `dws drive` | 钉盘文件管理 | runnable |
| `dws drive +copy` | 复制在线文档到指定位置并读回验证 | leaf |
| `dws drive +cover` | 读取节点封面或缩略图地址 | leaf |
| `dws drive +create-folder` | 创建钉盘文件夹并读回验证 | leaf |
| `dws drive +create-shortcut` | 为已有节点创建快捷方式并验证新节点 | leaf |
| `dws drive +delete` | 将已确认节点移入回收站 | leaf |
| `dws drive +download` | 安全下载钉盘文件到工作目录 | leaf |
| `dws drive +find-file` | 按名称关键词搜索钉盘文件并投影关键字段（只读） | leaf |
| `dws drive +info` | 获取钉盘文件/文件夹元数据 | leaf |
| `dws drive +inspect` | 聚合检查节点元数据及可选统计、公开状态和封面 | leaf |
| `dws drive +list` | 严格分页列出钉盘文件和文件夹 | leaf |
| `dws drive +move` | 移动文件/文档到指定位置 | leaf |
| `dws drive +publish-get` | 查询文件互联网公开状态 | leaf |
| `dws drive +publish-unset` | 关闭文件互联网公开发布 | leaf |
| `dws drive +recent` | 获取最近访问/编辑的文档列表 | leaf |
| `dws drive +recycle-list` | 严格分页列出钉盘回收站 | leaf |
| `dws drive +recycle-restore` | 恢复已确认的回收站条目并读回节点 | leaf |
| `dws drive +rename` | 重命名文件或文件夹并读回验证 | leaf |
| `dws drive +search` | 搜索钉盘文件 | leaf |
| `dws drive +search-docs` | 搜索文档空间文档 | leaf |
| `dws drive +star-add` | 收藏指定节点 | leaf |
| `dws drive +star-list` | 严格分页列出当前用户收藏 | leaf |
| `dws drive +star-remove` | 取消收藏指定节点 | leaf |
| `dws drive +stats` | 读取节点访问和协作统计 | leaf |
| `dws drive +upload` | 从工作目录上传普通文件到钉盘或文档空间并读回验证 | leaf |
| `dws drive +version-download` | 安全下载普通文件指定历史版本 | leaf |
| `dws drive +version-get` | 按版本号精确读取普通文件版本元数据 | leaf |
| `dws drive +version-history` | 严格分页列出普通文件历史版本 | leaf |
| `dws drive +version-revert` | 预检并回滚普通文件到指定历史版本 | leaf |
| `dws drive comment` | 普通文件评论管理 | runnable |
| `dws drive comment batch-query` | 按 commentKey 批量查询文件全局评论详情 | leaf |
| `dws drive comment create` | 创建普通文件全文评论 | leaf |
| `dws drive comment create-v2` | 创建本地文件全局评论 | leaf |
| `dws drive comment delete` | 永久删除本地文件评论 | leaf |
| `dws drive comment list` | 查询普通文件评论列表 | leaf |
| `dws drive comment list-replies` | 分页查询评论的直接子回复 | leaf |
| `dws drive comment list-v2` | 查询本地文件全局评论列表 | leaf |
| `dws drive comment react-reply` | 创建一条表情回复 | leaf |
| `dws drive comment reply` | 回复本地文件评论 | leaf |
| `dws drive comment resolve` | 将评论标记为已解决 | leaf |
| `dws drive comment restore` | 将已解决评论恢复为未解决 | leaf |
| `dws drive comment update` | 更新本地文件评论 | leaf |
| `dws drive commit` | 提交文件上传 | leaf |
| `dws drive copy` | 复制文件/文档到指定位置 | leaf |
| `dws drive cover` | 获取节点封面地址 | leaf |
| `dws drive delete` | 删除文件/文件夹到回收站 | leaf |
| `dws drive download` | 下载钉盘文件到本地 | leaf |
| `dws drive download-version` | 下载文件历史版本到本地 | leaf |
| `dws drive export` | 导出通用文档 (docx / xlsx / markdown / pdf / pptx) | runnable |
| `dws drive export get` | 查询导出任务状态 | leaf |
| `dws drive info` | 获取文件元数据信息 | leaf |
| `dws drive list` | 获取文件/文件夹列表（统一入口） | leaf |
| `dws drive list-spaces` | 获取钉盘空间列表 (deprecated → dws wiki space list --type orgSpace/mySpace) | leaf |
| `dws drive mkdir` | 创建文件夹 | leaf |
| `dws drive move` | 移动文件/文档到指定位置 | leaf |
| `dws drive permission` | 文档节点权限管理 | runnable |
| `dws drive permission add` | 添加协作者 | leaf |
| `dws drive permission apply` | 发起权限申请 | leaf |
| `dws drive permission apply-info` | 查询节点可申请的角色与审批人 | leaf |
| `dws drive permission get-setting` | 查询节点权限设置 | leaf |
| `dws drive permission list` | 查询协作者列表 | leaf |
| `dws drive permission remove` | 移除协作者权限 | leaf |
| `dws drive permission transfer-owner` | [危险] 转交所有者 | leaf |
| `dws drive permission update` | 更新协作者权限 | leaf |
| `dws drive publish` | 文件互联网公开发布管理 | runnable |
| `dws drive publish get` | 查询文件公开发布状态 | leaf |
| `dws drive publish set` | [危险] 设置文件为互联网公开 | leaf |
| `dws drive publish unset` | [危险] 关闭文件互联网公开 | leaf |
| `dws drive pull` | 把钉盘文件夹单向镜像到本地（Drive → 本地） | leaf |
| `dws drive push` | 把本地文件夹单向镜像到钉盘（本地 → Drive） | leaf |
| `dws drive quota` | 查询企业存储容量 | runnable |
| `dws drive quota apps` | 查询应用级存储用量列表 | leaf |
| `dws drive recent` | 获取最近访问/编辑的文档列表 | leaf |
| `dws drive recycle` | 钉盘回收站管理 | runnable |
| `dws drive recycle list` | 查看回收站文件列表 | leaf |
| `dws drive recycle restore` | 还原回收站中的文件 | leaf |
| `dws drive rename` | 重命名文件/文档 | leaf |
| `dws drive revert` | [危险] 回滚文件到指定历史版本 | leaf |
| `dws drive search` | 搜索文件（聚合钉盘+文档空间） | leaf |
| `dws drive shortcut` | 为节点创建快捷方式 | leaf |
| `dws drive star` | 文档收藏管理 | runnable |
| `dws drive star add` | 收藏文档 | leaf |
| `dws drive star list` | 获取收藏列表 | leaf |
| `dws drive star remove` | 取消收藏文档 | leaf |
| `dws drive stats` | 获取节点统计信息 | leaf |
| `dws drive status` | 比较本地文件夹与钉盘文件夹的差异 | leaf |
| `dws drive sync` | 本地文件夹与钉盘文件夹双向同步（本地 ⇄ Drive） | leaf |
| `dws drive task` | 异步任务状态查询（统一入口） | runnable |
| `dws drive task get` | 查询单个异步任务状态 | leaf |
| `dws drive upload` | 上传本地文件到钉盘或文档空间 | leaf |
| `dws drive upload-info` | 获取文件上传信息 | leaf |

## `dws event`

| Command | Description | Type |
|---|---|---|
| `dws event` | 事件订阅 (DingTalk Stream 长连接) | runnable |
| `dws event +listen-im` | 按 IM 意图解析目标并监听一个或多个个人消息事件 | leaf |
| `dws event consume` | 订阅事件流并输出到 stdout | leaf |
| `dws event list` | 列出个人事件目录 | leaf |
| `dws event schema` | 显示事件 schema | leaf |
| `dws event status` | 显示个人事件订阅和本地消费状态 | leaf |
| `dws event stop` | 取消个人事件订阅并停止本地消费 | leaf |

## `dws help`

| Command | Description | Type |
|---|---|---|
| `dws help [command]` | Help about any command | leaf |

## `dws hrbrain`

| Command | Description | Type |
|---|---|---|
| `dws hrbrain` | 组织大脑：人才池、员工档案与人才搜索 | runnable |
| `dws hrbrain profile` | 员工档案管理 | runnable |
| `dws hrbrain profile career` | 查询员工公司内职业历程 | leaf |
| `dws hrbrain profile labels` | 获取员工标签 | leaf |
| `dws hrbrain profile metadata` | 查询员工档案元数据结构 | leaf |
| `dws hrbrain profile performance` | 查询员工绩效记录 | leaf |
| `dws hrbrain profile query` | 按模块批量查询员工档案数据 | leaf |
| `dws hrbrain search` | 人才搜索 | runnable |
| `dws hrbrain search employees` | 人才搜索 | leaf |
| `dws hrbrain search employees-structured` | 使用高级条件搜索人员 | leaf |
| `dws hrbrain search fields` | 获取高级搜索字段列表 | leaf |
| `dws hrbrain talent-pool` | 人才池管理 | runnable |
| `dws hrbrain talent-pool detail` | 获取人才池详情 | leaf |
| `dws hrbrain talent-pool employees` | 人才池人员列表 | leaf |
| `dws hrbrain talent-pool list` | 人才池列表 | leaf |
| `dws hrbrain talent-pool move-members` | 人才池人员出入池 | leaf |
| `dws hrbrain talent-pool save` | 创建或更新人才池 | leaf |

## `dws html`

| Command | Description | Type |
|---|---|---|
| `dws html` | HTML 文件处理 | runnable |
| `dws html create` | 创建原生 .html 文件 | leaf |
| `dws html fetch` | 获取 HTML 文件内容 | leaf |
| `dws html overwrite` | 覆盖已有 HTML 文件 | leaf |
| `dws html patch` | 局部替换 HTML 文本 | leaf |

## `dws live`

| Command | Description | Type |
|---|---|---|
| `dws live` | 直播列表 / 信息 | runnable |
| `dws live stream` | 直播流管理 | runnable |
| `dws live stream list` | 查看我的直播列表 | leaf |

## `dws mail`

| Command | Description | Type |
|---|---|---|
| `dws mail` | 邮箱 / 邮件收发 | runnable |
| `dws mail +contact-list` | 列出指定邮箱的所有邮件联系人 | leaf |
| `dws mail +find-mail-user` | 按关键词搜索邮箱联系人并投影列表（姓名/昵称/邮箱/工号等） | leaf |
| `dws mail +folder-list` | 列出顶层文件夹或指定父文件夹下的子文件夹 | leaf |
| `dws mail +message` | 读取一封邮件的完整正文与附件元数据 | leaf |
| `dws mail +messages` | 按请求顺序读取多封邮件并逐封验证身份 | leaf |
| `dws mail +recent-mail` | 列出收件箱近期邮件会话并投影列表（主题/发件人/时间/threadId） | leaf |
| `dws mail +search-mail` | 按 KQL 关键词搜索邮件并投影列表（主题/发件人/时间/messageId） | leaf |
| `dws mail +tag-list` | 列出指定邮箱下的所有邮件标签 | leaf |
| `dws mail +template-list` | 列出指定邮箱的所有邮件模板 | leaf |
| `dws mail +thread` | 读取完整邮件会话并精确验证 conversationId | leaf |
| `dws mail +thread-list` | 列出指定邮箱文件夹下的邮件会话（thread） | leaf |
| `dws mail +triage` | 列出或筛选邮件摘要，自动解析邮箱与收件箱 | leaf |
| `dws mail +unread-mail` | 列出未读邮件并投影列表（主题/发件人/时间/messageId） | leaf |
| `dws mail +user-search` | 按关键词或工号搜索邮箱用户（仅企业邮箱） | leaf |
| `dws mail allow-list` | 个人收信白名单管理 | runnable |
| `dws mail allow-list add` | 添加个人收信白名单 | leaf |
| `dws mail allow-list list` | 列出个人收信白名单 | leaf |
| `dws mail allow-list remove` | 移除个人收信白名单 | leaf |
| `dws mail attachment` | 邮件附件管理 | runnable |
| `dws mail attachment download` | 下载邮件附件到本地 | leaf |
| `dws mail attachment list` | 列举邮件附件 | leaf |
| `dws mail auto-reply` | 邮件自动回复管理 | runnable |
| `dws mail auto-reply get` | 获取用户的自动回复配置 | leaf |
| `dws mail auto-reply update` | 更新/设置用户的自动回复配置 | leaf |
| `dws mail block-list` | 个人收信黑名单管理 | runnable |
| `dws mail block-list add` | 添加个人收信黑名单 | leaf |
| `dws mail block-list list` | 列出个人收信黑名单 | leaf |
| `dws mail block-list remove` | 移除个人收信黑名单 | leaf |
| `dws mail calendar` | 邮箱日历管理 | runnable |
| `dws mail calendar list` | 列出用户可访问的日历列表 | leaf |
| `dws mail calendar-event` | 邮箱日历日程管理 | runnable |
| `dws mail calendar-event list` | 查询指定日历时间范围内的日程 | leaf |
| `dws mail contact` | 邮件联系人管理 | runnable |
| `dws mail contact batch-delete` | 批量删除邮件联系人 | leaf |
| `dws mail contact create` | 创建邮件联系人 | leaf |
| `dws mail contact list` | 列举邮件联系人 | leaf |
| `dws mail contact update` | 更新邮件联系人 | leaf |
| `dws mail draft` | 草稿管理 | runnable |
| `dws mail draft create` | 创建草稿 | leaf |
| `dws mail draft send` | 发送草稿 | leaf |
| `dws mail draft update` | 更新草稿 | leaf |
| `dws mail folder` | 邮件文件夹管理 | runnable |
| `dws mail folder create` | 创建邮件文件夹 | leaf |
| `dws mail folder delete` | 删除邮件文件夹 | leaf |
| `dws mail folder list` | 列举邮件文件夹 | leaf |
| `dws mail folder update` | 更新邮件文件夹 | leaf |
| `dws mail mailbox` | 邮箱地址管理 | runnable |
| `dws mail mailbox list` | 查询可用邮箱地址 | leaf |
| `dws mail mailbox profile` | 获取用户邮箱信息 | leaf |
| `dws mail mailbox shared-with-me` | 查询共享给我的邮箱 | leaf |
| `dws mail message` | 邮件管理 | runnable |
| `dws mail message batch-delete` | 批量删除邮件 | leaf |
| `dws mail message batch-get` | 批量获取邮件详情 | leaf |
| `dws mail message batch-move` | 批量移动邮件到指定文件夹 | leaf |
| `dws mail message batch-update` | 批量修改邮件状态（标记已读/未读/添加标签/移除标签） | leaf |
| `dws mail message export` | 导出/备份邮件（EML格式） | leaf |
| `dws mail message forward` | 转发邮件 | leaf |
| `dws mail message get` | 查看邮件完整内容 | leaf |
| `dws mail message list` | 列出文件夹中的邮件 | leaf |
| `dws mail message reply` | 回复邮件 | leaf |
| `dws mail message reply-all` | 回复所有人 | leaf |
| `dws mail message search` | 搜索邮件 (KQL 语法) | leaf |
| `dws mail message send` | 发送邮件 | leaf |
| `dws mail message share-to-chat` | [危险] 分享邮件至IM聊天 | leaf |
| `dws mail message verify` | 查询邮件发送状态 | leaf |
| `dws mail rule` | 收信规则管理 | runnable |
| `dws mail rule adjust` | 调整收信规则排序 | leaf |
| `dws mail rule create` | 创建个人收信规则 | leaf |
| `dws mail rule delete` | 删除个人收信规则 | leaf |
| `dws mail rule list` | 列出个人收信规则 | leaf |
| `dws mail rule update` | 更新个人收信规则 | leaf |
| `dws mail sent-message` | 已发送邮件管理 | runnable |
| `dws mail sent-message recall` | [危险] 撤回已发送的邮件 | leaf |
| `dws mail sent-message recall-detail` | 查询邮件撤回进度 | leaf |
| `dws mail tag` | 邮件标签管理 | runnable |
| `dws mail tag create` | 创建邮件标签 | leaf |
| `dws mail tag delete` | 删除邮件标签 | leaf |
| `dws mail tag list` | 列举邮件标签 | leaf |
| `dws mail tag update` | 更新邮件标签 | leaf |
| `dws mail template` | 邮件模板管理 | runnable |
| `dws mail template create` | 创建邮件模板 | leaf |
| `dws mail template delete` | 删除邮件模板 | leaf |
| `dws mail template get` | 获取邮件模板详情 | leaf |
| `dws mail template list` | 列举邮件模板 | leaf |
| `dws mail template update` | 更新邮件模板 | leaf |
| `dws mail thread` | 邮件会话管理 | runnable |
| `dws mail thread batch-trash` | [危险] 批量删除邮件会话 | leaf |
| `dws mail thread batch-update` | 批量修改邮件会话状态 | leaf |
| `dws mail thread get` | 获取会话详情 | leaf |
| `dws mail thread list` | 列出邮件会话 | leaf |
| `dws mail thread trash` | [危险] 删除邮件会话 | leaf |
| `dws mail thread update` | 修改邮件会话状态 | leaf |
| `dws mail user` | 邮箱用户管理 | runnable |
| `dws mail user batch-get` | 根据多个完整企业邮箱批量查询员工 | leaf |
| `dws mail user get` | 根据完整企业邮箱精确查询员工 | leaf |
| `dws mail user search` | 搜索邮箱用户 | leaf |

## `dws markdown`

| Command | Description | Type |
|---|---|---|
| `dws markdown` | Markdown 文件处理 | runnable |
| `dws markdown comment` | Markdown 评论 | runnable |
| `dws markdown comment list` | 查询 Markdown 评论列表 | leaf |
| `dws markdown create` | 创建原生 .md 文件 | leaf |
| `dws markdown diff` | 比较 Markdown 内容差异 | leaf |
| `dws markdown fetch` | 获取 Markdown 文件内容 | leaf |
| `dws markdown overwrite` | 覆盖已有 Markdown 文件 | leaf |
| `dws markdown patch` | 局部替换 Markdown 文本 | leaf |

## `dws mcp`

| Command | Description | Type |
|---|---|---|
| `dws mcp` | 管理和调用已发布 MCP 服务 | runnable |
| `dws mcp published` | 查看和调用已发布的 MCP 工具 | runnable |
| `dws mcp published invoke` | 调用当前身份可用的已发布 MCP 工具 | leaf |
| `dws mcp published tools` | 列出当前身份可用的已发布 MCP 工具 | leaf |
| `dws mcp url` | 管理 MCP 服务连接地址 | runnable |
| `dws mcp url get` | 按 mcpId 获取 MCP 的 Streamable HTTP 服务地址 | leaf |

## `dws minutes`

| Command | Description | Type |
|---|---|---|
| `dws minutes` | AI 听记 / 会议纪要 | runnable |
| `dws minutes +action-items` | 读取指定或我最新一条听记中已抽取的行动项 | leaf |
| `dws minutes +apply-permission` | 为当前用户申请语义化的听记访问权限 | leaf |
| `dws minutes +detail` | 批量聚合听记基础信息、摘要、关键词、完整逐字稿和行动项，支持安全文件输出 | leaf |
| `dws minutes +download` | 批量取得听记音视频地址并安全下载到本地 | leaf |
| `dws minutes +export-pack` | 把听记文本产物清理签名凭据后写入受控目录并生成清理台账 | leaf |
| `dws minutes +latest` | 取我最新的一条妙记（听记）详情 | leaf |
| `dws minutes +list-all` | 预览或完整查询我有权限访问的听记列表 | leaf |
| `dws minutes +list-mine` | 查询我创建的听记列表 | leaf |
| `dws minutes +list-shared` | 查询他人共享给我的听记列表 | leaf |
| `dws minutes +mindmap` | 创建听记思维导图并轮询到明确成功、失败或超时 | leaf |
| `dws minutes +prepare-asr` | 读取个人热词、只新增缺失项并读回验证 | leaf |
| `dws minutes +record-pause` | 暂停听记录音 | leaf |
| `dws minutes +record-resume` | 恢复听记录音 | leaf |
| `dws minutes +record-start` | 发起听记（开始录音） | leaf |
| `dws minutes +record-stop` | 结束听记录音 | leaf |
| `dws minutes +record-wrap-up` | 停止实时录音并有界等待听记产物，失败时保留恢复句柄 | leaf |
| `dws minutes +replace-batch` | 预检并批量执行多组听记文字替换，逐项验证且失败必定非零 | leaf |
| `dws minutes +search` | 按范围、标题关键词和时间搜索听记，支持安全全量翻页 | leaf |
| `dws minutes +share` | 按成员逐项授予一个或多个听记权限，输出可审计的部分写入 ledger | leaf |
| `dws minutes +speaker-insights` | 创建发言人段落总结并轮询结果，保留异步任务恢复句柄 | leaf |
| `dws minutes +speaker-replace` | 预检逐字稿中的发言人昵称，替换后重新读回验证 | leaf |
| `dws minutes +summary` | 读取当前纪要、校验图片引用、全量覆盖并读回验证 | leaf |
| `dws minutes +sync-asr` | 把个人热词精确同步为目标集合，删除多余项后读回验证 | leaf |
| `dws minutes +transcript` | 读取指定或我最新一条听记的完整逐字稿，并交付分页完整性证据 | leaf |
| `dws minutes +unshare` | 按成员逐项移除一个或多个听记权限，输出可审计的部分写入 ledger | leaf |
| `dws minutes +update` | 读取现状、预览差异、更新听记标题并读回验证 | leaf |
| `dws minutes +upload` | 把本地音视频完整上传并创建听记，不发送额外消息 | leaf |
| `dws minutes +upload-and-analyze` | 本地音视频直传听记并等待分析产物，可选思维导图和发言人洞察 | leaf |
| `dws minutes +upload-and-notify` | 上传本地音视频创建听记，并在生成后推送闪记卡片 | leaf |
| `dws minutes audio-memo` | 语音备忘查询 | runnable |
| `dws minutes audio-memo list` | 查询语音备忘列表 | leaf |
| `dws minutes get` | 获取听记内容 | runnable |
| `dws minutes get audio` | 获取听记音频/视频地址 | leaf |
| `dws minutes get batch` | 批量查询听记详情 | leaf |
| `dws minutes get info` | 获取听记基础信息 | leaf |
| `dws minutes get keywords` | 获取听记关键字列表 | leaf |
| `dws minutes get summary` | 获取听记 AI 摘要 | leaf |
| `dws minutes get todos` | 获取听记中提取的待办事项 | leaf |
| `dws minutes get transcription` | 获取听记语音转写原文 | leaf |
| `dws minutes hot-word` | 个人热词管理 | runnable |
| `dws minutes hot-word add` | 添加个人热词 | leaf |
| `dws minutes hot-word delete` | 批量删除个人热词 | leaf |
| `dws minutes hot-word list` | 查询我的热词列表 | leaf |
| `dws minutes list` | 听记列表 | runnable |
| `dws minutes list all` | 查询我有权限访问的所有听记列表 | leaf |
| `dws minutes list mine` | 查询我创建的听记列表 | leaf |
| `dws minutes list shared` | 查询他人共享给我的听记列表 | leaf |
| `dws minutes mind-graph` | 思维导图管理 | runnable |
| `dws minutes mind-graph create` | 创建思维导图 | leaf |
| `dws minutes mind-graph status` | 查询思维导图状态 | leaf |
| `dws minutes permission` | 听记成员权限管理 | runnable |
| `dws minutes permission add` | 批量添加听记成员并设置权限 | leaf |
| `dws minutes permission apply` | 为当前用户申请听记权限 | leaf |
| `dws minutes permission remove` | 批量移除听记成员权限 | leaf |
| `dws minutes record` | 控制听记录音 | runnable |
| `dws minutes record pause` | 暂停听记录音 | leaf |
| `dws minutes record resume` | 恢复听记录音 | leaf |
| `dws minutes record start` | 发起听记（开始录音） | leaf |
| `dws minutes record stop` | 结束听记录音 | leaf |
| `dws minutes replace-text` | 查找替换段落和纪要中匹配的文字 | leaf |
| `dws minutes speaker` | 发言人管理 | runnable |
| `dws minutes speaker replace` | 替换发言人 | leaf |
| `dws minutes speaker summary` | 发言人段落总结 | runnable |
| `dws minutes speaker summary create` | 触发创建发言人段落总结任务 | leaf |
| `dws minutes speaker summary get` | 查询发言人段落总结结果 | leaf |
| `dws minutes tag` | 听记标签/分组管理 | runnable |
| `dws minutes tag list` | 查询我的听记标签/分组列表 | leaf |
| `dws minutes tag query` | 根据标签ID查询听记列表 | leaf |
| `dws minutes update` | 更新听记信息 | runnable |
| `dws minutes update summary` | 更新纪要内容 | leaf |
| `dws minutes update title` | 修改听记标题 | leaf |
| `dws minutes upload` | 文件上传管理 | runnable |
| `dws minutes upload cancel` | 取消文件上传会话 | leaf |
| `dws minutes upload complete` | 完成文件上传并创建听记 | leaf |
| `dws minutes upload create` | 创建文件上传会话 | leaf |
| `dws minutes upload create-and-notify` | 创建上传会话并在生成后推送闪记卡片 | leaf |

## `dws oa`

| Command | Description | Type |
|---|---|---|
| `dws oa` | OA 审批 / 同意 / 拒绝 / 撤销 | runnable |
| `dws oa +list-cc` | 获取抄送当前用户的审批单列表 | leaf |
| `dws oa +list-executed` | 获取当前用户已经处理过的审批单列表 | leaf |
| `dws oa +list-forms` | 获取当前用户可见的审批表单列表 | leaf |
| `dws oa +list-pending` | 查询当前登录用户待处理的审批任务列表 | leaf |
| `dws oa +list-submitted` | 获取当前用户已发起的审批单列表 | leaf |
| `dws oa +my-initiated` | 列出我发起（提交）的审批单据 | leaf |
| `dws oa +search-forms` | 按关键字模糊搜索当前用户可见的审批表单 | leaf |
| `dws oa approval` | 审批管理 | runnable |
| `dws oa approval append-task` | 对审批任务进行加签 | leaf |
| `dws oa approval approve` | 同意审批 | leaf |
| `dws oa approval attachment` | 审批附件授权、上传、下载与链接管理 | runnable |
| `dws oa approval attachment authorize-download` | 授权当前用户下载审批钉盘文件 | leaf |
| `dws oa approval attachment authorize-preview` | 授权当前用户预览审批附件 | leaf |
| `dws oa approval attachment download-url` | 获取审批附件下载链接 | leaf |
| `dws oa approval attachment upload` | 上传本地文件为审批附件（初始化+PUT+提交，一步完成） | leaf |
| `dws oa approval create-instance` | 发起审批实例（需要 --yes 确认） | leaf |
| `dws oa approval detail` | 获取审批实例详情 | leaf |
| `dws oa approval ding-info` | 获取审批任务的被催办人 userId（需与 ding message send 串联使用） | leaf |
| `dws oa approval forecast-process` | 根据表单值预测审批流程与自选节点 | leaf |
| `dws oa approval form-schema` | 查询审批模板的表单 Schema | leaf |
| `dws oa approval list-by-admin` | 以管理员身份查询审批模板的实例列表 | leaf |
| `dws oa approval list-cc` | 获取抄送当前用户的审批单列表 | leaf |
| `dws oa approval list-executed` | 获取当前用户已经处理过的审批单列表 | leaf |
| `dws oa approval list-forms` | 获取当前用户可见的审批表单列表 | leaf |
| `dws oa approval list-initiated` | 查询当前用户在指定审批模板下发起的审批实例列表 | leaf |
| `dws oa approval list-pending` | 查询待我处理的审批 | leaf |
| `dws oa approval list-submitted` | 获取当前用户已发起的审批单列表 | leaf |
| `dws oa approval oa-cc-noticer` | 对审批实例进行抄送 | leaf |
| `dws oa approval oa-comments` | 对审批实例添加评论 | leaf |
| `dws oa approval records` | 获取审批操作记录 | leaf |
| `dws oa approval redirect-task` | 转交审批任务给其他人 | leaf |
| `dws oa approval reject` | 拒绝审批 | leaf |
| `dws oa approval revert-activities` | 获取审批任务可回退的节点信息（退回前必须调用，获取可回退节点列表） | leaf |
| `dws oa approval revert-task` | 退回审批任务到指定节点（审批人或发起人） | leaf |
| `dws oa approval revoke` | 撤销已发起的审批 | leaf |
| `dws oa approval search-forms` | 按关键字模糊搜索当前用户可见的审批表单 | leaf |
| `dws oa approval tasks` | 查询待我审批的任务 ID | leaf |
| `dws oa approval template` | 审批模板管理 | runnable |
| `dws oa approval template detail` | 获取审批模板详情，返回表单 Schema 和流程配置 | leaf |
| `dws oa approval template list` | 查询用户在当前组织可管理的审批模板 | leaf |

## `dws pat`

| Command | Description | Type |
|---|---|---|
| `dws pat` | 行为授权管理 | runnable |
| `dws pat +browser-policy` | 安全配置 PAT 授权时是否允许打开本地浏览器 | leaf |
| `dws pat browser-policy` | 配置 PAT 授权时是否打开浏览器 | leaf |
| `dws pat chmod` | 授予指定权限 | leaf |

## `dws plugin`

| Command | Description | Type |
|---|---|---|
| `dws plugin` | 插件管理 | runnable |
| `dws plugin build` | 将插件 stdio server 编译为原生二进制 | leaf |
| `dws plugin config` | 管理插件配置 | runnable |
| `dws plugin config get` | 读取插件配置项 | leaf |
| `dws plugin config list` | 列出插件所有配置项 | leaf |
| `dws plugin config set` | 设置插件配置项 | leaf |
| `dws plugin config unset` | 删除插件配置项 | leaf |
| `dws plugin create` | 脚手架生成新插件目录 | leaf |
| `dws plugin dev` | 将本地目录注册为开发态插件 | leaf |
| `dws plugin disable` | 禁用插件 | leaf |
| `dws plugin enable` | 启用插件 | leaf |
| `dws plugin info` | 查看插件详情 | leaf |
| `dws plugin install` | 安装插件 | leaf |
| `dws plugin list` | 列出已安装的插件 | leaf |
| `dws plugin remove` | 卸载已安装的插件 | leaf |
| `dws plugin validate` | 校验 plugin.json | leaf |

## `dws profile`

| Command | Description | Type |
|---|---|---|
| `dws profile` | 组织 profile 管理 | runnable |
| `dws profile list` | 列出全部已登录账号 profile | leaf |
| `dws profile switch` | 切换当前账号 profile | leaf |
| `dws profile use` | 切换当前账号 profile（兼容 profile switch） | leaf |

## `dws recovery`

| Command | Description | Type |
|---|---|---|
| `dws recovery` | 不再支持：错误恢复辅助命令（兼容入口） | runnable |
| `dws recovery execute` | 不再支持：生成面向 Agent 的恢复分析包 | leaf |
| `dws recovery finalize` | 不再支持：回写恢复闭环结果 | leaf |
| `dws recovery plan` | 不再支持：基于失败快照生成恢复计划 | leaf |

## `dws recruit`

| Command | Description | Type |
|---|---|---|
| `dws recruit` | 钉钉招聘 | runnable |
| `dws recruit job` | 招聘职位管理 | runnable |
| `dws recruit job create` | 创建招聘职位 | leaf |
| `dws recruit job get` | 查询招聘职位详情 | leaf |
| `dws recruit job list` | 查询招聘职位列表 | leaf |

## `dws report`

| Command | Description | Type |
|---|---|---|
| `dws report` | 钉钉日志（OA 周报应用 / 日志模版填报） | runnable |
| `dws report +inbox-list` | 列出我收到的日志 | leaf |
| `dws report +outbox-list` | 列出我发出的日志 | leaf |
| `dws report +report-latest` | 读取我最近提交的一篇日志详情 | leaf |
| `dws report +template-search` | 按名称搜索可用日志模板 | leaf |
| `dws report create` | [deprecated] 已废弃，请改用 `dws report entry submit` | leaf |
| `dws report created` | [deprecated] 已废弃，请改用 `dws report outbox list` | leaf |
| `dws report detail` | [deprecated] 已废弃，请改用 `dws report entry get` | leaf |
| `dws report entry` | 日志条目（单条日报操作 — get / stats / submit） | runnable |
| `dws report entry get` | 读取单份日报正文（含字段明细 + 钉钉跳转链接） | leaf |
| `dws report entry stats` | 读取单份日报的已读统计 | leaf |
| `dws report entry submit` | 提交一份新日报（按模版） | leaf |
| `dws report inbox` | 收件箱（我收到的日报） | runnable |
| `dws report inbox list` | 列出我收到的日报 | leaf |
| `dws report list` | [deprecated] 已废弃，请改用 `dws report inbox list` | leaf |
| `dws report outbox` | 发件箱（我发出的日报） | runnable |
| `dws report outbox list` | 列出我发出的日报 | leaf |
| `dws report sent` | [deprecated] 已废弃，请改用 `dws report outbox list` | leaf |
| `dws report stats` | [deprecated] 已废弃，请改用 `dws report entry stats` | leaf |
| `dws report template` | 日志模版 | runnable |
| `dws report template detail` | [deprecated] 已废弃，请改用 `dws report template get` | leaf |
| `dws report template get` | 读取单个日志模版的字段定义 | leaf |
| `dws report template list` | 获取当前用户可用的日志模版列表 | leaf |

## `dws schema`

| Command | Description | Type |
|---|---|---|
| `dws schema` | 渐进查看命令 Schema (产品 / 分组 / 工具参数) | leaf |

## `dws sheet`

| Command | Description | Type |
|---|---|---|
| `dws sheet` | 钉钉表格管理 | runnable |
| `dws sheet +list-sheets` | 严格列出在线电子表格的工作表，并可按完整标题精确筛选 | leaf |
| `dws sheet +read` | 完整读取并严格校验在线电子表格范围；截断结果失败关闭 | leaf |
| `dws sheet add-dimension` | 在末尾追加空行或空列 | leaf |
| `dws sheet append` | 在工作表末尾追加数据 | leaf |
| `dws sheet batch-update` | 批量执行多个写操作（原子事务） | leaf |
| `dws sheet changeset-get` | 获取表格工作簿 revision 区间内的 changeset | leaf |
| `dws sheet chart` | 浮动图表管理 | runnable |
| `dws sheet chart create` | 创建浮动图表 | leaf |
| `dws sheet chart delete` | 删除浮动图表 | leaf |
| `dws sheet chart list` | 获取浮动图表 | leaf |
| `dws sheet chart update` | 更新浮动图表 | leaf |
| `dws sheet comment` | 表格评论 / 单元格评论管理 | runnable |
| `dws sheet comment batch-query` | 按 topicId + commentKey 批量查询评论详情 | leaf |
| `dws sheet comment create` | 创建单元格评论 | leaf |
| `dws sheet comment delete` | 删除单元格评论 | leaf |
| `dws sheet comment list` | 查询表格评论列表 | leaf |
| `dws sheet comment list-replies` | 分页查询评论的直接子回复 | leaf |
| `dws sheet comment react-reply` | 创建一条表情回复 | leaf |
| `dws sheet comment reply` | 回复单元格评论 | leaf |
| `dws sheet comment resolve` | 将评论标记为已解决 | leaf |
| `dws sheet comment restore` | 将已解决评论恢复为未解决 | leaf |
| `dws sheet comment update` | 更新单元格评论 | leaf |
| `dws sheet cond-format` | 条件格式管理 | runnable |
| `dws sheet cond-format create` | 创建条件格式规则 | leaf |
| `dws sheet cond-format delete` | 删除条件格式规则 | leaf |
| `dws sheet cond-format list` | 获取条件格式规则 | leaf |
| `dws sheet cond-format update` | 更新条件格式规则 | leaf |
| `dws sheet copy` | 复制工作表 | leaf |
| `dws sheet create` | 创建钉钉表格文档 | leaf |
| `dws sheet create-float-image` | 创建浮动图片 | leaf |
| `dws sheet create-with-data` | 创建钉钉表格文档并写入初始数据（可选样式） | leaf |
| `dws sheet csv-get` | 以 CSV 格式读取工作表数据 | leaf |
| `dws sheet csv-put` | 将 CSV 数据写入表格指定位置（支持公式，自动扩容） | leaf |
| `dws sheet delete-dimension` | 删除指定位置的行或列 | leaf |
| `dws sheet delete-dropdown` | 删除下拉列表 | leaf |
| `dws sheet delete-float-image` | 删除浮动图片 | leaf |
| `dws sheet delete-sheet` | 删除工作表 | leaf |
| `dws sheet export` | 导出表格为 xlsx（异步任务一站式） | leaf |
| `dws sheet export-csv` | 导出单个工作表为纯 CSV（同步） | leaf |
| `dws sheet filter` | 全局筛选管理 | runnable |
| `dws sheet filter clear-criteria` | 清除单列筛选条件 | leaf |
| `dws sheet filter create` | 创建全局筛选 | leaf |
| `dws sheet filter delete` | 删除全局筛选 | leaf |
| `dws sheet filter get` | 获取全局筛选信息 | leaf |
| `dws sheet filter sort` | 筛选排序 | leaf |
| `dws sheet filter update` | 批量更新筛选条件 | leaf |
| `dws sheet filter-view` | 筛选视图管理 | runnable |
| `dws sheet filter-view create` | 创建筛选视图 | leaf |
| `dws sheet filter-view delete` | 删除筛选视图 | leaf |
| `dws sheet filter-view delete-criteria` | 删除筛选视图列条件 | leaf |
| `dws sheet filter-view get-criteria` | 获取单列筛选条件 | leaf |
| `dws sheet filter-view info` | 获取单个筛选视图详情 | leaf |
| `dws sheet filter-view list` | 获取所有筛选视图 | leaf |
| `dws sheet filter-view list-criteria` | 列出筛选视图所有列条件 | leaf |
| `dws sheet filter-view update` | 更新筛选视图属性 | leaf |
| `dws sheet filter-view update-criteria` | 更新筛选视图列条件 | leaf |
| `dws sheet find` | 在工作表中搜索单元格内容 | leaf |
| `dws sheet formula-verify` | 校验表格公式错误 | leaf |
| `dws sheet get-dropdown` | 获取下拉列表配置 | leaf |
| `dws sheet get-float-image` | 获取浮动图片详情 | leaf |
| `dws sheet group-dimension` | 对指定连续行/列创建分组 | leaf |
| `dws sheet hide-gridline` | 隐藏工作表网格线 | leaf |
| `dws sheet import` | 导入本地表格文件为在线电子表格 (xlsx / xls) | runnable |
| `dws sheet import create` | 导入本地表格文件为在线电子表格 (xlsx / xls) | leaf |
| `dws sheet import get` | 查询表格导入任务结果（手动兜底） | leaf |
| `dws sheet info` | 获取指定工作表详情 | leaf |
| `dws sheet insert-dimension` | 在指定位置插入行或列 | leaf |
| `dws sheet list` | 获取全部工作表列表 | leaf |
| `dws sheet list-float-images` | 列出工作表所有浮动图片 | leaf |
| `dws sheet media-upload` | 上传附件到表格 | leaf |
| `dws sheet merge-cells` | 合并单元格 | leaf |
| `dws sheet move-dimension` | 移动行或列到指定位置/调整顺序 | leaf |
| `dws sheet new` | 新建工作表 | leaf |
| `dws sheet pivot-table` | 透视表管理 | runnable |
| `dws sheet pivot-table create` | 创建透视表 | leaf |
| `dws sheet pivot-table delete` | [危险] 删除透视表 | leaf |
| `dws sheet pivot-table list` | 获取透视表列表或详情 | leaf |
| `dws sheet pivot-table update` | 更新透视表配置 | leaf |
| `dws sheet range` | 数据区域操作 | runnable |
| `dws sheet range batch-clear` | 批量清除多个区域（原子事务） | leaf |
| `dws sheet range batch-set-style` | 批量设置样式（服务端原子事务） | leaf |
| `dws sheet range clear` | 清除工作表指定区域 | leaf |
| `dws sheet range copy-to` | 复制工作表指定区域到目标位置 | leaf |
| `dws sheet range fill` | 自动填充工作表指定区域 | leaf |
| `dws sheet range move-to` | 移动工作表指定区域到目标位置 | leaf |
| `dws sheet range read` | 读取工作表数据（别名: get） | leaf |
| `dws sheet range set-style` | 设置指定单元格区域的样式 | leaf |
| `dws sheet range sort` | 对工作表指定区域排序 | leaf |
| `dws sheet range update` | 更新工作表指定区域内容 | leaf |
| `dws sheet replace` | 查找替换/批量替换/精确匹配替换/正则替换文本 | leaf |
| `dws sheet revision-get` | 获取表格工作簿当前 revision | leaf |
| `dws sheet set-dropdown` | 设置下拉列表 | leaf |
| `dws sheet show-gridline` | 显示工作表网格线 | leaf |
| `dws sheet table-get` | 读取结构化 table 数据 | leaf |
| `dws sheet table-put` | 写入结构化 table 数据 | leaf |
| `dws sheet template` | 表格模板管理 | runnable |
| `dws sheet template apply` | 应用表格模板 | leaf |
| `dws sheet template list` | 获取表格模板列表 | leaf |
| `dws sheet template search` | 搜索表格模板 | leaf |
| `dws sheet ungroup-dimension` | 取消指定连续行/列分组 | leaf |
| `dws sheet unmerge-cells` | 取消合并单元格 | leaf |
| `dws sheet update` | 更新工作表属性 | leaf |
| `dws sheet update-dimension` | 更新指定范围行/列属性（显隐、行高/列宽） | leaf |
| `dws sheet update-float-image` | 更新浮动图片属性 | leaf |
| `dws sheet version` | 表格历史版本管理 | runnable |
| `dws sheet version list` | 查看表格历史版本列表 | leaf |
| `dws sheet version revert` | [危险] 回滚表格到指定历史版本或 revision | leaf |
| `dws sheet version save` | 手动保存表格版本快照 | leaf |
| `dws sheet write-image` | 上传图片并写入表格单元格 | leaf |

## `dws skill`

| Command | Description | Type |
|---|---|---|
| `dws skill` | 技能管理 | runnable |
| `dws skill get` | 获取技能压缩文件 | leaf |
| `dws skill install` | 下载并安装技能到指定目录 | leaf |
| `dws skill search` | 从钉钉技能市场搜索技能 | leaf |
| `dws skill setup` | 安装 dws 自身 skill 到 Agent 目录 | leaf |

## `dws todo`

| Command | Description | Type |
|---|---|---|
| `dws todo` | 待办任务管理 | runnable |
| `dws todo +assign` | 按姓名给某人创建并指派一条待办（自动解析 userId） | leaf |
| `dws todo +assign-multi` | 把一条待办按姓名一次性指派给多个人（自动把每个姓名解析成 userId） | leaf |
| `dws todo +comment` | 添加待办评论并读回验证 | leaf |
| `dws todo +complete` | 完成待办并读回验证 | leaf |
| `dws todo +create` | 创建待办并读回验证 | leaf |
| `dws todo +created-todos` | 列出我创建的待办（我作为创建人 creator 发起的待办，而非分配给我执行的） | leaf |
| `dws todo +due-today` | 列出我今天到期的待办 | leaf |
| `dws todo +get` | 查询待办详情 | leaf |
| `dws todo +get-my-tasks` | 查询当前组织下我的待办列表 | leaf |
| `dws todo +get-related-tasks` | 一次性列出与我相关的全部待办（我作为创建人/执行人/参与人三种角色的并集，按 taskId 去重） | leaf |
| `dws todo +list-attachment` | 查询待办任务的附件列表 | leaf |
| `dws todo +list-comment` | 查询待办评论列表 | leaf |
| `dws todo +list-sub` | 查询子待办列表 | leaf |
| `dws todo +overdue` | 列出我已过期未完成的待办 | leaf |
| `dws todo +remind` | 给自己创建一条带可选截止时间的待办 | leaf |
| `dws todo +reminder` | 设置或清除待办提醒（仅终端回执） | leaf |
| `dws todo +reopen` | 重新打开待办并读回验证 | leaf |
| `dws todo +search` | 搜索与我相关的全部待办 | leaf |
| `dws todo +todo-done` | 按标题关键词把我的某条待办标记完成（自动定位 taskId） | leaf |
| `dws todo +update` | 更新待办并读回验证 | leaf |
| `dws todo comment` | 待办评论：新增 / 列表 / 删除 | runnable |
| `dws todo comment add` | 新增待办评论 | leaf |
| `dws todo comment delete` | 删除待办评论 | leaf |
| `dws todo comment list` | 查询待办评论列表 | leaf |
| `dws todo tag` | 待办标签：打标 / 列表 / 创建 / 更新 / 删除 | runnable |
| `dws todo tag add` | 给待办打标 | leaf |
| `dws todo tag create` | 创建待办标签 | leaf |
| `dws todo tag delete` | 删除待办标签 | leaf |
| `dws todo tag list` | 查询待办标签列表 | leaf |
| `dws todo tag update` | 更新待办标签 | leaf |
| `dws todo task` | 创建 / 查询 / 更新 / 删除待办 | runnable |
| `dws todo task add-attachment` | 上传待办附件 | leaf |
| `dws todo task add-executor` | 添加待办执行人 | leaf |
| `dws todo task add-participant` | 添加待办参与人 | leaf |
| `dws todo task add-reminder` | 添加待办提醒 | leaf |
| `dws todo task create` | 创建待办 | leaf |
| `dws todo task create-sub` | 创建子待办 | leaf |
| `dws todo task delete` | 删除待办 | leaf |
| `dws todo task done` | 修改执行者的待办完成状态 | leaf |
| `dws todo task get` | 待办详情 | leaf |
| `dws todo task list` | 查询待办列表 | leaf |
| `dws todo task list-attachment` | 查询待办任务的附件列表 | leaf |
| `dws todo task list-sub` | 查询子待办列表 | leaf |
| `dws todo task remove-attachment` | 删除待办任务的附件 | leaf |
| `dws todo task remove-executor` | 移除待办执行人 | leaf |
| `dws todo task remove-participant` | 移除待办参与人 | leaf |
| `dws todo task reset-reminder` | 重置待办提醒 | leaf |
| `dws todo task update` | 修改待办任务 | leaf |

## `dws upgrade`

| Command | Description | Type |
|---|---|---|
| `dws upgrade` | 升级 DWS CLI 到最新版本 | leaf |

## `dws version`

| Command | Description | Type |
|---|---|---|
| `dws version` | 显示版本信息 | leaf |

## `dws whiteboard`

| Command | Description | Type |
|---|---|---|
| `dws whiteboard` | 钉钉白板管理 | runnable |
| `dws whiteboard +diff` | 读取当前白板并预览 proposed OpenNodes 更新的节点、媒体与风险变化 | leaf |
| `dws whiteboard +query` | 严格读取文档内嵌或独立白板的 OpenNodes 快照 | leaf |
| `dws whiteboard +update` | 确认后更新文档内嵌或独立白板并精确读回 | leaf |
| `dws whiteboard create-with-content` | 使用 OpenNodes 创建独立白板 | leaf |
| `dws whiteboard export` | 导出独立白板到本地 | leaf |
| `dws whiteboard export-get` | 查询白板导出任务并下载 | leaf |
| `dws whiteboard query` | 读取白板内容 | leaf |
| `dws whiteboard render` | 把 OpenNodes 预渲染为本地 SVG | leaf |
| `dws whiteboard template` | 独立白板模板管理 | runnable |
| `dws whiteboard template personal` | 个人独立白板模板 | runnable |
| `dws whiteboard template personal create` | 从个人模板创建独立白板 | leaf |
| `dws whiteboard template personal list` | 查询个人独立白板模板 | leaf |
| `dws whiteboard template personal save` | 保存独立白板为个人模板 | leaf |
| `dws whiteboard template public` | 公共独立白板模板 | runnable |
| `dws whiteboard template public create` | 从公共模板创建独立白板 | leaf |
| `dws whiteboard template public list` | 查询公共独立白板模板 | leaf |
| `dws whiteboard template team` | 团队独立白板模板 | runnable |
| `dws whiteboard template team create` | 从团队模板创建独立白板 | leaf |
| `dws whiteboard template team list` | 查询团队独立白板模板 | leaf |
| `dws whiteboard template team save` | 保存独立白板为团队模板 | leaf |
| `dws whiteboard update` | 追加或整页重建白板内容 | leaf |

## `dws wiki`

| Command | Description | Type |
|---|---|---|
| `dws wiki` | 知识库 / 空间管理 / 节点管理 / 成员管理 / 动态查询 | runnable |
| `dws wiki +delete-space` | 删除知识库 | leaf |
| `dws wiki +feed-list` | 严格分页列出知识库动态 | leaf |
| `dws wiki +member-add` | 添加知识库成员 | leaf |
| `dws wiki +member-list` | 严格列出知识库成员 | leaf |
| `dws wiki +member-remove` | 移除知识库成员 | leaf |
| `dws wiki +member-update` | 更新知识库成员角色 | leaf |
| `dws wiki +move` | 移动节点到知识库并读回验证 | leaf |
| `dws wiki +move-to-drive` | 移动 Wiki 节点到我的文档并验证目标域 | leaf |
| `dws wiki +node-copy` | 复制知识库节点并验证独立副本 | leaf |
| `dws wiki +node-create` | 创建知识库节点并严格读回验证 | leaf |
| `dws wiki +node-delete` | 删除知识库节点 | leaf |
| `dws wiki +node-get` | 获取知识库节点详情 | leaf |
| `dws wiki +node-list` | 严格分页列出知识库节点 | leaf |
| `dws wiki +node-search` | 严格分页搜索知识库节点 | leaf |
| `dws wiki +resolve-space` | 按名称搜索知识空间并解析单次结果中的唯一 spaceId（只读） | leaf |
| `dws wiki +space-create` | 创建知识库并读回验证空间类型 | leaf |
| `dws wiki +space-get` | 获取知识库详情 | leaf |
| `dws wiki +space-list` | 严格分页列出知识库并保留类型范围证据 | leaf |
| `dws wiki +space-search` | 严格搜索知识库 | leaf |
| `dws wiki +wiki-new-doc` | 在指定名称的知识库下新建一个文档节点（按单次名称搜索解析 workspaceId） | leaf |
| `dws wiki feed` | 知识库动态查询 | runnable |
| `dws wiki feed list` | 查询知识库动态列表 | leaf |
| `dws wiki member` | 知识库成员管理 | runnable |
| `dws wiki member add` | 添加知识库成员 | leaf |
| `dws wiki member list` | 查询知识库成员列表 | leaf |
| `dws wiki member remove` | 移除知识库成员 | leaf |
| `dws wiki member update` | 更新知识库成员权限 | leaf |
| `dws wiki node` | 知识库节点管理 | runnable |
| `dws wiki node copy` | 复制知识库节点 | leaf |
| `dws wiki node create` | 在知识库中创建节点 | leaf |
| `dws wiki node delete` | 删除知识库节点 | leaf |
| `dws wiki node list` | 列出知识库节点 | leaf |
| `dws wiki node move` | 移动知识库节点 | leaf |
| `dws wiki node search` | 在知识库中搜索节点 | leaf |
| `dws wiki space` | 知识库管理 | runnable |
| `dws wiki space create` | 创建知识库 | leaf |
| `dws wiki space delete` | 删除知识库 | leaf |
| `dws wiki space get` | 查看知识库详情 | leaf |
| `dws wiki space list` | 列出空间（知识库 / 钉盘空间） | leaf |
| `dws wiki space search` | 搜索知识库 | leaf |
