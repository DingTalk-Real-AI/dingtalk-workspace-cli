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

- 业务只通过 `dws` CLI；Skill 脚本可编排 `dws` 和预签名上传。结构化读取用 `--format json`。
- 已知命令直接执行；参数或返回契约不明时只查一次精确 leaf compact Schema，只有 Schema 与 Runtime 不一致才查同一 leaf Help。禁止用父级 Help、产品 Schema 或完整 Catalog 探路。
- 不猜命令、flag、字段、ID、账号或时间。ID 只取真实返回；零/多候选或类型不明时停止。解析、读取、执行保持同一 profile，不跨组织复用 ID。
- 多账号组织只用 `isOrgCurrent=true` 默认账号；无默认账号时要求指定。不得输出或索要 token、appSecret 等凭据。
- confirmation 只由 Runtime/Schema 的 Safety 契约决定：`not_required` 直接执行；`user_required` 由宿主取得并传递独立确认。Skill 不添加绕过参数，不把一般任务描述视为高风险确认。
- 写后按任务结果契约验证；不能仅凭退出码宣称成功。部分结果、未知投递状态和失败项必须如实保留。
- 遇到认证、权限、profile、confirmation 或未知错误时，只加载 `dingtalk-shared` 中对应 reference；不要连续猜测替代命令。
- 预计不超过 3 个执行步骤的简单任务直接完成，不创建 Todo；只有 4 步及以上、需要跨产品协作或包含多个可独立验收阶段时才使用 Todo。
<!-- DWS_RUNTIME_CONTRACT_END -->

<!-- VISIBLE_SHORTCUTS_START -->
## Shortcut 发现（按需）

`aitable` 当前有 93 条公开 shortcut，完整清单保留在 Runtime Catalog 与 Schema，不在高频产品根 Skill 中重复展开。已知意图直接使用下方的优先路由、意图表或任务 reference；命令已选中时直接执行，只在参数/安全语义不确定时读取 leaf Schema，在当前 Cobra flags 不确定时读取 leaf Help。

仅当现有路由和 reference 都无法定位低频能力时，才执行 `dws shortcut list --service aitable --format json` 做最后回退；不要为已知高频意图加载完整 Shortcut Catalog 或产品级 Schema。
<!-- VISIBLE_SHORTCUTS_END -->

## Golden Route

已有 ID 直接用；URL 先解析；名称唯一解析。零/多候选时停止。下表入口均在 `dws aitable` 下，可省略公共前缀。

| 用户意图 | 唯一推荐入口 | 关键边界 |
|---|---|---|
| 从 URL 解析稳定 ID | `dws aitable +url-resolve --url <URL>` | 只解析 URL 中已有的 baseId/tableId/viewId/recordId，不做远端名称搜索 |
| 按名称唯一定位并操作 Base/Table | `dws aitable +resolve-base --name <名称>` → `dws aitable +resolve-table --base <ID> --name <表名>` | 默认精确匹配；只有用户明确接受模糊匹配时才加 `--fuzzy` |
| 搜索 Base 候选或检查是否存在 | `dws aitable +base-search --query <关键词>` | 用户说“搜索/找一下/候选/如果没有就创建”时直接走本入口，不先调用 `+resolve-base`；AITable 上下文中的 Base 名称不得路由到 `dws aisearch person` |
| 浏览 Base 下的数据表 | `dws aitable +list-tables --base <ID>` | 只返回 tableId/tableName，不加载字段 |
| 查看、改名或删除 Base | `+base-get --base-id <B>` / `+base-update --base-id <B> --name <名称>` / `+base-delete --base-id <B>` | 删除不可逆，先核对真实 baseId 并按 Runtime confirmation 执行 |
| 只新建 Base | `dws aitable base create --name <名称>` → `dws aitable +base-get --base-id <返回的baseId>` | 不用空 `tables` 或临时表调用 bootstrap；直接复用创建回执的 baseId，禁止立即按名称搜索 |
| 新建 Base 与整套表字段 | `dws aitable +base-bootstrap --name <名称> --tables '[{"name":"<表名>","fields":[{"fieldName":"<字段名>","type":"text"}]}]'` | 成功回执已含 tableId 及 `fieldId/fieldName/type`，后续直接复用，不再调用 `field list` |
| 从模板创建 Base | `+template-search --query <关键词>` → `base create --name <名称> --template-id <TEMPLATE_ID>` | 只使用唯一选定的真实 templateId，不先探测 Help |
| 复制 Base 到文档目录 | `dws aitable +base-copy --base-id <B> --target-folder-id <FOLDER_DENTRY_UUID> [--only-struct]` | 只接受 Drive 可精确读回且类型为 folder 的 dentryUuid；“我的文件根目录”的 rootFolderId/nodeId 不能证明该契约，当前明确不支持；收到 `retryable=false` 立即停止，禁止轮换 ID |
| 已有 Base 新建一张表与字段 | `dws aitable +table-bootstrap --base-id <ID> --name <表名> --fields '<JSON数组>'` | 字段使用 `fieldName/type/config`；自动按 15 个字段分片并读回验证 |
| 查看、改名或删除 Table | `+table-get --base-id <B> [--table-ids <T>]` / `+table-update --base-id <B> --table-id <T> --name <名称>` / `+table-delete --base-id <B> --table-id <T>` | 删除整表不可逆；导入后改名也直接复用 `+table-update` |
| 读取字段目录或完整配置 | `dws aitable field list --base-id <B> --table-id <T>` / `dws aitable +field-get --base-id <B> --table-id <T>` | 只需 fieldId/name/type 用 `field list`；需要 config 用 `+field-get`；不存在 `+field-list` 或 `+list-fields` |
| 记录 CRUD、历史、同步或批改 | `+record-query` / `record create` / `+record-update` / `+record-delete` / `+record-history-list` / `+record-upsert-by-key` / `+record-bulk-patch` | 当前无 `+record-create`；upsert 仅允许 0/1 条匹配，批改必须有选择边界；字段值、分页和回读只读 [record-ops](references/aitable-record-ops.md) |
| 查看、创建或复制视图 | `view list --base-id <B> --table-id <T>` / `view create ... --view-type <TYPE>` / `+view-duplicate ... --view-id <V>` | 准确 flag 是 `--view-type`；类型与配置读 [view-config](references/aitable/aitable-view-config.md) |
| 查看或创建 Dashboard/Chart | `+section-list-nodes --base-id <B>` → `+dashboard-get ... --dashboard-id <D>`；创建 Chart 时仅在缺 config 时调用一次 `+chart-widgets-example --chart-type <TYPE>`，再执行 `chart create --config '<CONFIG>' --layout '{"x":0,"y":0,"w":12,"h":6}'` → `chart get` | 这是唯一 Chart Golden Route；示例直接返回标准 JSON 的单一 config 与准确 layout，不再抓全量 JSONC |
| 创建或移动 Base 内 Section/节点 | `+section-create --base-id <B> --name <名称>` → `+section-move-node --base-id <B> --node-id <N> --new-parent-section-id <S>` → `+section-list-nodes --base-id <B>` | 这些是 AITable nsheet 节点，不走 Wiki/Drive；根目录移动细节按需读 [section](references/aitable-section.md) |
| 将本地 CSV/XLSX/XLS 导入新表 | `python3 scripts/aitable_import_via_task.py <BASE_ID> <FILE_PATH>` | 回执取 ID、改名与恢复细节按需读 [export-import](references/aitable/aitable-export-import.md) |

## 执行与恢复最短路径

- 参数足够时直接执行；缺参数或恢复语义时只读一个精确 Reference，仍不确定才查一次 leaf compact Schema。Shortcut 已含分片/验证时不重复拆步。
- 全程复用真实返回的 ID、cursor、字段映射；新建资源按返回 ID 核验，不立即按名称搜索。同一任务的时间戳只取一次并复用。
- 零/多候选、类型不符、分页未完成或 `retryable=false` 时停止；`partial_success` / `unknown` 只执行返回的唯一 `nextCommand` 做核验或从 checkpoint 续跑，绝不重放已知成功批次。只按错误给出的 `actions` / `available_flags` 修正一次，不轮换 ID 类型。
- 删除按 Runtime confirmation 核对真实目标；`base list` 只是最近访问，不能证明全量或唯一目标。

## 按需加载

每个 Case 最多读取一个 **AITable 操作 Reference**；Golden Route 足够时读取零个。读取后不得沿文件内链接、旧文件名或索引再进入第二个 AITable Reference，也不加载产品 Catalog 或 Help。需要复杂 filters 时一开始选 filter-sort，需要 View payload 时一开始选 view-config，需要记录 payload 时一开始选 record-ops。

| 触发条件 | Reference |
|---|---|
| Base/Table 创建、查看、复制、改名、删除，或模板检索 | [base-table-ops](references/aitable-base-table-ops.md) |
| 记录 CRUD、历史、分享、字段值格式（唯一记录 payload 入口） | [record-ops](references/aitable-record-ops.md) |
| 记录统计、分组聚合或去重率 | [record-stats](references/aitable/aitable-record-stats.md) |
| 记录主键文档 | [primary-doc](references/aitable/aitable-primary-doc.md) |
| record filters/sort/date 操作符（唯一 filter payload 入口） | [filter-sort](references/aitable/aitable-filter-sort.md) |
| 字段创建/配置，或公式/查找引用 | 分别选 [field](references/aitable/aitable-field.md) / [formula-guide](references/aitable/aitable-formula-guide.md) |
| 导入导出恢复，或附件上传移除 | 分别选 [export-import](references/aitable/aitable-export-import.md) / [attachment](references/aitable/aitable-attachment.md) |
| 视图配置与 view payload，或锁定/冻结/行高/填色 | 分别选 [view-config](references/aitable/aitable-view-config.md) / [view-extras](references/aitable/aitable-view-extras.md) |
| Base 内 Section，或 Dashboard/Chart | 分别选 [section](references/aitable-section.md) / [dashboard-chart](references/aitable/aitable-dashboard-chart.md) |
| 表单、自动化、普通角色或高级权限 | 分别选 [form](references/aitable/aitable-form.md) / [workflow](references/aitable/aitable-workflow.md) / [advperm](references/aitable/aitable-advperm.md) |
| 产品边界或低频原子能力仍不明确 | 选 [intent-guide](references/intent-guide.md)；仍无法定位才读 [aitable.md](references/aitable.md) 对应章节 |

不要预加载这些 Reference。完整 Shortcut Catalog 只在根路由、精确 Reference 和低频原子索引都无法定位时使用。

## 跨产品边界

- Excel 式单元格、区域和公式操作 → `dingtalk-misc` 的 Sheet。
- Base 作为整体在普通文件夹间移动或做外层存储重命名 → Drive；Base 结构复制/删除，以及 Base 内 Table、Dashboard、Section 的创建、复制、移动、重命名、删除 → AITable。
- 记录主键文档正文 → 取得真实 nodeId 后切 `dingtalk-doc`。
