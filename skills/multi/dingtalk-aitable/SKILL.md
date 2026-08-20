---
name: dingtalk-aitable
description: 钉钉 AI 表格（多维表）。Use when 用户说 AI表格/多维表/数据表/base/table/建表/查记录/写数据/字段/记录增删改查/筛选/排序/公式/模板搜索/批量导入CSV或JSON/导出/仪表盘/图表/上传附件到表格/按字段类型建表。不做电子表格单元格读写（走 dingtalk-misc）、文档编辑（走 dingtalk-doc）；听记待办入表先用 dingtalk-minutes 提取，再由本 skill 写入。命令前缀：dws aitable。
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
- 已知命令直接执行。优先读取精确 leaf Schema 补参数；只有 Schema 与当前运行时不一致时才读取该 leaf Help，不要用父级 Help 或产品级 Catalog 探路。
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

`aitable` 当前有 94 条公开 shortcut，完整清单保留在 Runtime Catalog 与 Schema，不在高频产品根 Skill 中重复展开。已知意图直接使用下方的优先路由、意图表或任务 reference；命令已选中时直接执行，只在参数/安全语义不确定时读取 leaf Schema，在当前 Cobra flags 不确定时读取 leaf Help。

仅当现有路由和 reference 都无法定位低频能力时，才执行 `dws shortcut list --service aitable --format json` 做最后回退；不要为已知高频意图加载完整 Shortcut Catalog 或产品级 Schema。
<!-- VISIBLE_SHORTCUTS_END -->

## Golden Route

已有 ID 直接使用；完整 URL 先解析；名称先唯一解析为稳定 ID。零命中或多候选时停止，不默认选第一项。

| 用户意图 | 唯一推荐入口 | 关键边界 |
|---|---|---|
| 从 URL 解析稳定 ID | `dws aitable +url-resolve --url <URL>` | 只解析 URL 中已有的 baseId/tableId/viewId/recordId，不做远端名称搜索 |
| 按名称唯一定位并操作 Base/Table | `dws aitable +resolve-base --name <名称>` → `dws aitable +resolve-table --base <ID> --name <表名>` | 默认精确匹配；只有用户明确接受模糊匹配时才加 `--fuzzy` |
| 搜索 Base 候选或检查是否存在 | `dws aitable +base-search --query <关键词>` | 用户说“搜索/找一下/候选/如果没有就创建”时直接走本入口，不先调用 `+resolve-base`；AITable 上下文中的 Base 名称不得路由到 `dws aisearch person` |
| 浏览 Base 下的数据表 | `dws aitable +list-tables --base <ID>` | 只返回 tableId/tableName，不加载字段 |
| 新建 Base 与整套表字段 | `dws aitable +base-bootstrap --name <名称> --tables '[{"name":"<表名>","fields":[{"fieldName":"<字段名>","type":"text"}]}]'` | 表对象键必须是 `name`，不是 `tableName`；字段使用 `fieldName/type/config`；参数已足够时直接执行，不读 Reference 或 Help |
| 复制 Base 到文档目录 | `dws aitable +base-copy --base-id <B> --target-folder-id <DOC_FOLDER_ID> [--only-struct]`；只有 nodeId/rootFolderId/URL 时改传 `--target-folder-node <NODE>` | `target-folder-id` 必须是 `dws doc info` 返回的 `folderId`，不是 rootFolderId、fileId/nodeId、dentryId、spaceId 或 workspaceId；两种目标参数二选一 |
| 已有 Base 新建一张表与字段 | `dws aitable +table-bootstrap --base-id <ID> --name <表名> --fields '<JSON数组>'` | 字段使用 `fieldName/type/config`；自动按 15 个字段分片并读回验证 |
| 读取字段目录或完整配置 | `dws aitable field list --base-id <B> --table-id <T>` / `dws aitable +field-get --base-id <B> --table-id <T>` | 只需 fieldId/name/type 用 `field list`；需要 config 用 `+field-get`；不存在 `+field-list` 或 `+list-fields` |
| 查询、筛选、排序或字段投影 | `dws aitable +record-query --base-id <ID> --table-id <ID> [--record-ids <IDs>] [--field-ids <IDs>] [--filters <JSON>] [--sort <JSON>] [--query <关键词>]` | 用户要求“只返回/仅查看”指定字段时必须传对应 `--field-ids`，不能只在最终文本删列；明确要求全量时改用原子 `record query --all --page-limit <N>` |
| 新增单条或批量记录 | `dws aitable record create --base-id <ID> --table-id <ID> --records <JSON>` | 当前无 `+record-create`；写前取字段定义，写后按新 ID 回读 |
| 更新已知 recordId | `dws aitable +record-update --base-id <ID> --table-id <ID> --records <JSON>` | 自动分片并读回；只传需修改字段 |
| 按业务键同步或按条件批改 | 唯一键用 `dws aitable +record-upsert-by-key ...`；有界批改用 `dws aitable +record-bulk-patch ... --max-matches <N>` | upsert 仅允许 0 条创建、1 条更新；批改必须有 query/filters/record-ids 边界，准确结构只读 [record-ops](references/aitable-record-ops.md) |
| 创建视图 | `dws aitable view create --base-id <B> --table-id <T> --view-type <Grid|FormDesigner|Gantt|Calendar|Kanban|Gallery> [--name <名称>]` | 准确参数是 `--view-type`；复制用 `+view-duplicate --base-id <B> --table-id <T> --view-id <V> [--new-name <名称>]`，复杂配置才读 [view-config](references/aitable/aitable-view-config.md)，不先查 Help |
| 创建并验证 Dashboard/Chart | `dws aitable +dashboard-bootstrap --base-id <B> --name <名称> [--chart-config <JSON> --chart-layout <JSON>]` | 只提交一次 Dashboard 创建；内部读回并对账后才创建 Chart，失败时不要重建或更换 dashboardId |
| Base 内创建 Section 并移动节点 | `dws aitable +section-create --base-id <B> --name <名称>` → `dws aitable +section-move-node --base-id <B> --node-id <N> --new-parent-section-id <S>` → `dws aitable +section-list-nodes --base-id <B>` | Table、Dashboard、Section 都是 AITable 的 nsheet 节点；禁止改走 Wiki/Drive 文件夹或移动命令 |
| 将本地 CSV/XLSX/XLS 导入新表 | `python scripts/aitable_import_via_task.py <BASE_ID> <FILE_PATH>` | 首选本 Skill 自带脚本，一次完成申请凭证、空 Content-Type PUT 和 `import data`；不要猜 `+import-csv` 或给 `import upload` 传 `--file` |

其余已知能力不要探测 Help/Catalog：按下方路由只读一个精确 Reference；Base/Table 生命周期等简单 leaf 只读该 leaf Schema。运行时仍兼容常见历史 flag，但新命令始终使用 canonical flag。

## 当前最短路径

- 已有 ID 直接使用；URL 只解析一次；“唯一定位并操作”用 `+resolve-base` / `+resolve-table`，“搜索候选/存在性检查”直接用 `+base-search`，两条路径不要串行探测。filters/sort 缺 fieldId 时才读取字段目录。
- Golden Route 已给出准确命令和参数时直接执行。只有操作参数、JSON 结构或恢复语义确实缺失时，才读取下方一个精确操作 Reference。
- Shortcut 已含分片或验证时不重复拆步；已有 Base 新建完整表结构直接用 `+table-bootstrap`。
- 按宿主规则以“独立业务步骤”计数，不把单条 CLI 或工具调用单独计步：不超过 3 个独立步骤（≤3）时直接执行，不调用 TodoWrite；达到 4 个及以上独立步骤，或用户明确要求计划/待办时才创建 TodoWrite。创建后只在阶段切换时更新，不在每条 CLI 后刷新状态。
- 用户要求资源名带当前时间戳时只取一次并在 Base、Table、Dashboard 等名称中复用同一值；不要为每个资源分别取时间。
- JSON 已返回所需字段时立即复用；不得为寻找同一字段改用 `--verbose`、`raw`、`pretty` 重复请求。

## 记录输入与结果

- `cells` key 用当前 fieldId；大 JSON 用相对 `--records-file`。filters 必须是 `{"operator":"and|or","operands":[...]}`，不能把 `and/or` 直接作为 JSON key；sort 使用 `direction`。复杂条件读 [filter-sort](references/aitable/aitable-filter-sort.md)。
- 建表字段类型使用真实枚举：单选为 `singleSelect`；人民币货币字段使用 `type:"currency"` 和 `config:{"currencyType":"CNY","formatter":"FLOAT_2"}`，不要猜 `select` 或 `config.symbol`。
- 用户限定返回字段时，先复用当前字段目录中的真实 fieldId，最终 `+record-query` 必须带 `--field-ids <ID1,ID2>`；工具层投影是业务要求和 token 控制的一部分，不能用最终答复二次过滤替代。
- 按真实字段类型写值，只读字段不得写入。
- 新建从 `data.newRecordIds[]` 取 ID，再用 `+record-query --record-ids` 回读；若用户同时限定列，回读命令一并传 `--field-ids`。
- 批量结果检查 completed/failed、verification、checkpoint；`partial_success` 不是完成。全量查询使用原子 `record query --all` 并检查 `hasMore`；只有 `hasMore=false`，或按指定 ID 全命中时，才声称结果完整。
- 写入效果未知时回读，不重放成功批次。

## 安全边界

- 删除不可逆，按 Runtime confirmation 核对真实目标；`base list` 只是最近访问。字段零/多候选、类型不明时停止；多批写保留已完成批次和续跑位置。

## 按需加载

每个 Case 最多读取一个操作 Reference。Golden Route 参数足够时读取零个并直接执行；一旦读取了一个 Reference，本 Case 不再读取第二个 Reference、产品级 Catalog 或 Help。

| 触发条件 | Reference |
|---|---|
| Base/Table 查看、复制、改名、删除，或模板检索 | 直接选择 `+base-*` / `+table-*` / `+template-search`，只读最终 leaf Schema |
| 记录 CRUD、历史、分享、字段值格式 | [record-ops](references/aitable-record-ops.md) |
| 记录统计、分组聚合或去重率 | [record-stats](references/aitable/aitable-record-stats.md) |
| 记录主键文档 | [primary-doc](references/aitable/aitable-primary-doc.md) |
| filters/sort/date 操作符 | [filter-sort](references/aitable/aitable-filter-sort.md) |
| 字段创建或复杂配置 | [field](references/aitable/aitable-field.md) |
| 公式或查找引用字段 | [formula-guide](references/aitable/aitable-formula-guide.md) |
| 导入导出任务恢复 | [export-import](references/aitable/aitable-export-import.md) |
| 视图列顺序、筛选、排序、分组 | [view-config](references/aitable/aitable-view-config.md) |
| 视图锁定、冻结列、行高、填色 | [view-extras](references/aitable/aitable-view-extras.md) |
| Base 内 Section/节点移动或清理 | [section](references/aitable-section.md) |
| Dashboard/Chart 创建、读回或配置 | [dashboard-chart](references/aitable/aitable-dashboard-chart.md) |
| 表单创建、题目或分享 | [form](references/aitable/aitable-form.md) |
| 附件上传或移除 | [attachment](references/aitable/aitable-attachment.md) |
| 自动化工作流 | [workflow](references/aitable/aitable-workflow.md) |
| 普通角色或高级权限 | [advperm](references/aitable/aitable-advperm.md) |
| 产品边界不明确 | [intent-guide](references/intent-guide.md) |
| 只有上述 Reference 仍无法定位的低频原子能力 | [aitable.md](references/aitable.md) 的对应章节 |

不要预加载这些 Reference。完整 Shortcut Catalog 只在根路由、精确 Reference 和低频原子索引都无法定位时使用。

## 错误最短路径

1. 零/多候选、字段歧义或分页不完整：停止并返回证据；需要后续页时只透传真实 `nextCursor`。
2. 类型错误只复核目标字段，不删字段或丢输入；`partial_success` 从 checkpoint 续跑，未知写入先回读。
3. 错误包含 `actions` / `available_flags` 时只执行其中给出的准确修正；同一操作最多做一次有证据的参数修正。`--field-name`、视图创建的 `--type`、复制视图的 `--name` 会在各自命令内安全归一到 canonical flag，无需查 Help。`retryable=false` 或目标 ID 类型不符时停止，不把 Drive/Wiki/Space/子节点 ID 轮流代入试错。

## 跨产品边界

- Excel 式单元格、区域和公式操作 → `dingtalk-misc` 的 Sheet。
- Base 作为整体在普通文件夹间移动或做外层存储重命名 → Drive；Base 结构复制/删除，以及 Base 内 Table、Dashboard、Section 的创建、复制、移动、重命名、删除 → AITable。
- 记录主键文档正文 → 取得真实 nodeId 后切 `dingtalk-doc`。
