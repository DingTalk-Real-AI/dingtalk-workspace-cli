---
name: dingtalk-aitable
description: 钉钉 AI 表格（多维表）。Use when 用户说 AI表格/多维表/数据表/base/table/建表/查记录/写数据/字段/记录增删改查/筛选/排序/公式/模板搜索/批量导入CSV或JSON/导出/仪表盘/图表/上传附件到表格/按字段类型建表/数据源/创建数据源/更新数据源配置/触发数据源同步/按任务 ID 查询同步状态/获取数据源配置/列出数据源可用来源/获取数据源可同步字段/审批数据同步。不做电子表格单元格读写（走 dingtalk-misc）、文档编辑（走 dingtalk-doc）；听记待办入表先用 dingtalk-minutes 提取，再由本 skill 写入。命令前缀：dws aitable。
metadata:
  cli_version: ">=0.2.14"
  category: product
  requires:
    bins:
      - dws
---

# 钉钉 AI 表格 Skill

<!-- DWS_RUNTIME_CONTRACT_START -->
## 最小 DWS 执行契约

- 钉钉业务操作只通过 `dws` CLI；本 Skill 明确发布的脚本可编排 `dws` 并完成预签名文件上传。结构化读取使用 `--format json`，按真实返回判断结果。
- 已知 leaf 直接执行。只有参数或安全语义不确定时，最多读取一次 `dws schema --cli-path "aitable <leaf>" --compact --format json`；仅当该 compact leaf Schema 与 Cobra 实际不一致时，才读取同一 leaf 的 `dws aitable <leaf> --help`。禁止通过父级 Help、产品 Help 或完整 Catalog 探索命令。
- 不猜命令、flag、字段、ID、账号或时间。后续 ID 必须来自真实返回；零命中、多候选或类型不明时停止并消歧。
- 解析目标、读取上下文和最终执行必须使用同一 profile；不得跨组织复用 userId、openDingTalkId 或 openConversationId。多账号组织只使用明确的 `isOrgCurrent=true` 默认账号；没有默认账号时要求用户指定，禁止选择第一项、最近登录或最近使用账号。
- 不输出或记录 token、refresh token、appSecret、webhook token 等凭据；宿主已注入认证时不要索要凭据。
- 写操作必须符合用户明确意图。是否需要确认以最终 Runtime gate 和 Schema 为准；本轮用户已明确要求执行、目标与影响无歧义的非破坏性写操作时，该明确指令就是本次确认，首次调用直接携带 Runtime 所需的 `--yes`，不先制造 `confirmation_required`。删除、停用自动化等破坏性或高风险动作仍须先说明对象、动作与影响并取得独立确认。
- 写后按任务结果契约验证；不能仅凭退出码宣称成功。部分结果、未知投递状态和失败项必须如实保留。
- 时间戳面向用户展示时转换为带时区的可读时间；默认使用当前会话时区，必要时同时保留原值。
- 遇到认证、权限、profile、confirmation 或未知错误时，只加载 `dingtalk-shared` 中对应 reference；不要连续猜测替代命令。
<!-- DWS_RUNTIME_CONTRACT_END -->

<!-- VISIBLE_SHORTCUTS_START -->
## Shortcut 发现（按需）

`aitable` 当前有 100 条公开 shortcut，完整清单保留在 Runtime Catalog 与 Schema，不在高频产品根 Skill 中重复展开。已知 leaf 直接执行。只有参数不确定时，最多读取一次 `dws schema --cli-path "aitable <leaf>" --compact --format json`；仅当该 compact leaf Schema 与 Cobra 实际不一致时，才读取同一 leaf 的 `dws aitable <leaf> --help`。禁止用父级 Help、产品 Help 或完整 Catalog 探索命令；一个 Case 一旦读取 Reference，就不再读取 Help 或第二个 Reference。

仅当根路由、精确 task reference 和 `references/aitable.md` 的低频原子索引都无法定位能力时，才执行 `dws shortcut list --service aitable --format json` 做最终回退；不要为已知意图加载完整 Shortcut Catalog 或产品级 Schema。
<!-- VISIBLE_SHORTCUTS_END -->

## Golden Route（高频复合任务）

已有 ID 直接使用；完整 URL 先解析；名称先唯一解析为稳定 ID。零命中或多候选时停止，不默认选第一项。

| 用户意图 | 唯一推荐入口 | 关键边界 |
|---|---|---|
| 从 URL 解析稳定 ID | `dws aitable +url-resolve --url <URL>` | 只解析 URL 中已有的 baseId/tableId/viewId/recordId，不做远端名称搜索 |
| 按名称唯一定位并操作 Base/Table | `dws aitable +resolve-base --name <名称>` → `dws aitable +resolve-table --base <ID> --name <表名>` | 默认精确匹配；只有用户明确接受模糊匹配时才加 `--fuzzy` |
| 搜索 Base 候选或检查是否存在 | `dws aitable +base-search --query <关键词>` | 用户说“搜索/找一下/候选/如果没有就创建”时直接走本入口，不先调用 `+resolve-base`；返回 `hasMore/nextCursor`，仅 `hasMore=true` 时续页；AITable Base 名称不得路由到 `dws aisearch person` |
| 浏览 Base 下的数据表 | `dws aitable +list-tables --base <ID>` | 只返回 tableId/tableName，不加载字段 |
| 新建 Base 与整套表字段 | `dws aitable +base-bootstrap --name <名称> --tables '[{"name":"<表名>","fields":[{"fieldName":"<字段名>","type":"text"}]}]'` | 表对象键必须是 `name`，不是 `tableName`；字段使用 `fieldName/type/config`；参数已足够时直接执行 |
| 复制 Base 到文档目录 | `dws aitable +base-copy --base-id <B> --target-folder-id <DOC_FOLDER_ID> [--only-struct] [--new-name <名称>]`；只有 nodeId/rootFolderId/URL 时改传 `--target-folder-node <NODE>` | `target-folder-id` 必须是 `dws doc info` 返回的 `folderId`；若 Runtime 返回 `target_not_supported/retryable=false`，立即报告，不查 Help、不换 ID、不建测试文件夹，也不手工降级复制 |
| 已有 Base 新建一张表与字段 | `dws aitable +table-bootstrap --base-id <ID> --name <表名> --fields '<JSON数组>'` | 字段使用 `fieldName/type/config`；自动按 15 个字段分片并读回验证 |
| 读取字段目录或完整配置 | `dws aitable field list --base-id <B> --table-id <T>` / `dws aitable +field-get --base-id <B> --table-id <T>` | 只需 fieldId/name/type 用 `field list`；需要 config 用 `+field-get`；不存在 `+field-list` 或 `+list-fields` |
| 查询记录、记录筛选/排序或字段投影 | `dws aitable +record-query --base-id <ID> --table-id <ID> [--record-ids <IDs>] [--field-ids <IDs>] [--filters <JSON>] [--sort <JSON>] [--query <关键词>]` | 用户要求“只返回/仅查看”指定字段时必须传对应 `--field-ids`，不能只在最终文本删列；明确要求全量时改用原子 `record query --all --page-limit <N>` |
| 新增单条或批量记录 | `dws aitable record create --base-id <ID> --table-id <ID> --records <JSON>` | 当前无 `+record-create`；写前取字段定义，写后按新 ID 回读 |
| 更新已知 recordId | `dws aitable +record-update --base-id <ID> --table-id <ID> --records <JSON>` | 自动分片并读回；只传需修改字段 |
| 查询一条记录的变更历史 | `dws aitable +record-history-list --base-id <ID> --table-id <ID> --record-id <ID>` | 已知 recordId 时直接执行，不探测 Help、Catalog 或全量 Schema |
| 按业务键同步或按条件批改 | 唯一键用 `dws aitable +record-upsert-by-key ...`；有界批改用 `dws aitable +record-bulk-patch ... --max-matches <N>` | upsert 仅允许 0 条创建、1 条更新；批改必须有 query/filters/record-ids 边界。普通 update/upsert 直接执行；只有历史、分享、删除恢复、空行或特殊字段值才读 [record-ops](references/aitable-record-ops.md)；明确 AND/OR、日期或比较操作符只读 [filter-sort](references/aitable/aitable-filter-sort.md) |
| 生成记录分享链接并发送给联系人 | `dws aitable +record-share-links --base <B> --table <T> --record-ids <IDs>` → `dws chat +dm --to <姓名> --text <完整链接文本>` | AITable 只生成链接；用户要求“发送”时还必须完成真实发送，不能停在联系人解析 |
| 创建或复制视图 | 创建用 `dws aitable view create --base-id <B> --table-id <T> --view-type <Grid|FormDesigner|Gantt|Calendar|Kanban|Gallery> [--name <名称>]`；复制用 `dws aitable +view-duplicate --base-id <B> --table-id <T> --view-id <V> [--new-name <名称>]` | 创建和复制直接执行；需要配置时按下方“按需加载”选择一个 View Reference |
| 创建并验证 Dashboard，按需创建 Chart | `dws aitable dashboard create --base-id <B> --name <名称>` → `dws aitable +dashboard-get --base-id <B> --dashboard-id <D>`；需要 Chart 时按下方“按需加载”处理 | 只使用创建返回的真实 dashboardId；失败时不要猜同义命令或更换 dashboardId |
| Base 内创建 Section 并移动节点 | `dws aitable +section-create --base-id <B> --name <名称>` → `dws aitable +section-move-node --base-id <B> --node-id <N> --new-parent-section-id <S>` → `dws aitable +section-list-nodes --base-id <B>` | Table、Dashboard、Section 都是 AITable 的 nsheet 节点；禁止改走 Wiki/Drive 文件夹或移动命令 |
| 将本地 CSV/XLSX/XLS 导入新表 | `python scripts/aitable_import_via_task.py <BASE_ID> <FILE_PATH>` | 首选本 Skill 自带脚本，一次完成申请凭证、空 Content-Type PUT 和 `import data`；不要猜 `+import-csv` 或给 `import upload` 传 `--file` |
| 接入外部数据源（审批等） | `dws aitable +datasource-list-sources --base-id <ID> --datasource-type OA` → 解析 result 构造 sourceConfig → `dws aitable +datasource-create --base-id <ID> --datasource-type OA --source-config '<JSON>'` | 当前仅支持 OA 审批；processCode/name/iconUrl/url 从 list-sources 原样透传，创建后用 `+datasource-sync-status` 查同步结果 |

### 简单 leaf

意图明确时直接使用；参数不确定才读 leaf Schema：

| 用户意图 | 入口 |
|---|---|
| 查看 / 改名 / 删除 Base | `+base-get` / `+base-update` / `+base-delete` |
| 搜索模板 | `+template-search` |
| 查看 / 跨 Base 复制 / 改名 / 删除 Table | `+table-get` / `+table-copy` / `+table-update` / `+table-delete` |
| 创建 / 更新 / 删除普通字段 | `field create` / `field update` / `field delete` |
| 查看 / 删除 View | `+view-get` / `+view-delete` |
| 查看 / 改名 / 删除 Dashboard | `+dashboard-get` / `+dashboard-update` / `+dashboard-delete` |

命令接在 `dws aitable` 后；资源 ID 使用 `--base-id/--table-id/--field-id/--view-id/--dashboard-id`，改名使用 `--name`。`+table-copy` 参数不规则，执行前只读其 leaf Schema。不读操作 Reference、Help 或产品 Catalog。

数据源查看来源用 `+datasource-list-sources`，获取字段用 `+datasource-get-fields`，创建、更新、同步、查状态和查配置用 `+datasource-create` / `+datasource-update` / `+datasource-sync` / `+datasource-sync-status` / `+datasource-get-config`。

## 执行约束

- 记录 filter/sort 缺 fieldId 时才读取字段目录。
- record filter/sort 与 view filter/sort/group 的协议和 Reference 互斥。普通 record query/create/update/upsert 直达；只有历史、分享、删除恢复、空行或特殊字段值读 `record-ops`。
- 普通字段 type 使用 `text/number/date/singleSelect/currency`；`singleSelect` 的 config 为 `{"options":[{"name":"<选项>"}]}`，人民币 `currency` 为 `{"currencyType":"CNY","formatter":"FLOAT_2"}`。
- 仅任务包含 4 个及以上独立业务步骤或用户明确要求时使用 TodoWrite；不按单条 CLI 拆步，只在阶段切换时更新。
- 多个资源名要求同一时间戳时，只取一次并复用。
- 复用 JSON 已返回字段，不以 `--verbose/raw/pretty` 重复请求。
- 数据源创建前必须先 `+datasource-list-sources` 获取 processCode 等透传字段，不凭记忆构造 sourceConfig。

## 记录稳定约束

- 记录 `cells` 使用当前 fieldId，按真实字段类型写值，只读字段不得写入。
- 新增或更新只使用真实返回的 ID 回读；写入效果未知时回读，不重放成功批次。
- 全量查询检查 `hasMore`，批量写检查最终状态；分页未结束或 `partial_success` 都不得声称完整完成。

## 安全边界

- 删除不可逆，按 Runtime confirmation 核对真实目标；`base list` 只是最近访问。字段零/多候选、类型不明时停止；多批写保留已完成批次和续跑位置。
- 数据源 `+datasource-create` / `+datasource-update` 会触发真实数据同步；执行前确认目标 Base 和 sourceConfig。`+datasource-sync` 单次最多 5 张表。

## 按需加载（复杂 JSON 与恢复语义）

Golden/次级直达覆盖时不读 Reference；否则按最终专有能力读取一个精确 Reference。读取后直接执行，不再读取其他 AITable Reference。

| 触发条件 | Reference |
|---|---|
| `+record-query`、upsert、bulk patch 的记录 filters/sort/date/AND/OR/比较操作符 | [filter-sort](references/aitable/aitable-filter-sort.md) |
| 记录历史、分享、删除恢复、空行或特殊字段值 | [record-ops](references/aitable-record-ops.md) |
| 记录统计、分组聚合或去重率 | [record-stats](references/aitable/aitable-record-stats.md) |
| 查询记录的主键文档，或为记录创建主键文档 | 首次建表前读取 [primary-doc](references/aitable/aitable-primary-doc.md)；普通 Base/Table/字段/记录创建与导入不读取 |
| AI 字段、关联字段、lookup/filterUp 或其他复杂 config | [field](references/aitable/aitable-field.md) |
| formula 字段或公式语法 | [formula-guide](references/aitable/aitable-formula-guide.md) |
| 导入导出任务恢复 | [export-import](references/aitable/aitable-export-import.md) |
| 视图列顺序、移到/放到/固定在最左、filter、sort、group（不得转读记录 `filter-sort`） | [view-config](references/aitable/aitable-view-config.md) |
| Kanban/Gallery 的 card、Gantt 的 timebar、Grid 的 aggregate | [view-types](references/aitable/aitable-view-types.md) |
| 视图锁定、明确冻结前 N 列、行高或填色 | [view-extras](references/aitable/aitable-view-extras.md) |
| Base 内 Section/节点移动或清理 | [section](references/aitable-section.md) |
| Chart 创建或更新所需 config，或 Dashboard 完整 config/arrange；普通 Dashboard CRUD 走上方直达 | [dashboard-chart](references/aitable/aitable-dashboard-chart.md) |
| 表单创建、题目或分享 | [form](references/aitable/aitable-form.md) |
| 附件上传或移除 | [attachment](references/aitable/aitable-attachment.md) |
| 自动化工作流 | [workflow](references/aitable/aitable-workflow.md) |
| 普通角色或高级权限 | [advperm](references/aitable/aitable-advperm.md) |
| 数据源接入、同步管理、sourceConfig 构造或审批数据同步 | [datasource](references/aitable/aitable-datasource.md) |
| 产品边界不明确 | [intent-guide](references/intent-guide.md) |
| 只有上述 Reference 仍无法定位的低频原子能力 | [aitable.md](references/aitable.md) 的对应章节 |

不要预加载这些 Reference。

## 错误最短路径

1. 零/多候选、字段歧义或分页不完整：停止并返回证据；需要后续页时只透传真实 `nextCursor`。
2. 类型错误只复核目标字段，不删字段或丢输入；`partial_success` 从 checkpoint 续跑，未知写入先回读。
3. 错误提供 `actions` / `available_flags` 时只按其中的 `next_command` 修正一次；`retryable=false` 或目标 ID 类型不符时停止。
4. 数据源同步 `errorCode=4014` 表示同步运行中重复触发，可稍后重试；非数据源表触发同步前先用 `+base-get` 确认 `sync=true`。

## 跨产品边界

- Excel 式单元格、区域和公式操作 → `dingtalk-misc` 的 Sheet。
- Base 作为整体在普通文件夹间移动或做外层存储重命名 → Drive；Base 结构复制/删除，以及 Base 内 Table、Dashboard、Section 的创建、复制、移动、重命名、删除 → AITable。
- 记录主键文档正文 → 取得真实 nodeId 后切 `dingtalk-doc`。
