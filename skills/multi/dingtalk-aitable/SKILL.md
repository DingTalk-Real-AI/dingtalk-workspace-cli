---
name: dingtalk-aitable
description: 钉钉 AI 表格（多维表）。Use when 用户说 AI表格/多维表/Base/数据表/字段/记录增删改查/筛选排序/公式与跨表引用/视图表单/仪表盘图表/高级权限/自动化工作流/模板/CSV或JSON批量导入/Excel导入导出/记录附件。不做电子表格单元格读写与工作表公式（走 dingtalk-sheet）、普通文档编辑（走 dingtalk-doc）或钉盘文件管理（走 dingtalk-drive）；听记待办入表先用 dingtalk-minutes 提取，再由本 skill 写入。命令前缀：dws aitable。
metadata:
  cli_version: ">=0.2.14"
  category: product
  requires:
    bins:
      - dws
---

# 钉钉 AI 表格 Skill

## 前置条件 — 执行操作前必读

> **CRITICAL — 执行任何 `dws` 操作前，MUST 先用 Read 工具完整读取 [`dws-shared`](../dws-shared/SKILL.md)。**该轻量文件包含全局执行契约、安全底线及 shared references 的按需加载导航；不要预加载其全部 references。

## 加载与路由顺序

1. 命中下方高频意图时直接使用精确骨架，不先查 Help 或产品级 Schema。
2. 路由优先级固定为：精确 recipe / 可运行脚本 > 匹配的公开 Shortcut > 原子命令。命令已确定且参数清楚时直接执行。
3. 参数、约束或安全语义不确定时只读 leaf Schema：`dws schema --cli-path "aitable <leaf>" --format json`；只有当前 Cobra flag 不确定时才读对应 `--help`。
4. 复杂字段、筛选、导入导出、视图、权限或工作流任务，按“低频能力与 Reference”只加载相关文件，不预读整个 `references/aitable/`。
5. 现有骨架和 reference 都无法定位能力时，才用 Runtime Shortcut Catalog 做最后发现；不得猜 `cli_path` 或 flag。
6. Schema、Help、reference 与实际返回冲突时采用更安全的解释并报告契约漂移；`confirmation=user_required` 时先确认，再添加 `--yes`。

<!-- VISIBLE_SHORTCUTS_START -->
## Shortcut 发现（按需）

`aitable` 当前有 29 条公开 shortcut。完整清单保留在 Runtime Shortcut Catalog；已完成 Schema curation 的子集可通过 leaf Schema 查询。高频产品根 Skill 不重复展开完整清单。已知意图直接使用下方的优先路由、意图表或任务 reference；命令已选中时直接执行，只在参数/安全语义不确定时读取 leaf Schema，在当前 Cobra flags 不确定时读取 leaf Help。

仅当现有路由和 reference 都无法定位低频能力时，才执行 `dws shortcut list --service aitable --compact --format json` 做最后回退；不要为已知高频意图加载完整 Shortcut Catalog 或产品级 Schema。
<!-- VISIBLE_SHORTCUTS_END -->

## 核心对象与 ID

| 对象 | 标识与执行边界 |
|---|---|
| Base | `baseId` 标识一个 AI 表格文件；名称只用于搜索或消歧，不能当 ID |
| Table | `tableId` 标识 Base 内的数据表；必须来自 `+resolve-table` / `+table-get` / 创建返回 |
| Field | `fieldId` 标识列；写入、筛选、排序和字段变更优先使用真实 `fieldId` |
| Record | `recordId` 标识行；更新、删除和分享前必须先查询得到真实 ID |
| View / Dashboard / Chart | `viewId` / `dashboardId` / `chartId` 各自绑定当前 Base/Table，不跨对象复用 |
| 异步任务 | `taskId` / `importId` 只用于对应导出或导入任务，不能替代业务对象 ID |

所有下游 ID 都从当前链路的结构化返回中提取；同名多候选必须让用户消歧，不默认取第一项，也不复用未经本轮校验的旧 ID。

## 核心意图与执行骨架

| 用户意图 | 首选骨架 | 必须保留的执行边界 |
|---|---|---|
| 按名称找 Base | `dws aitable +resolve-base --name "<名称>" --format json` | 唯一命中才继续；多候选停止并消歧 |
| 浏览最近访问 | `dws aitable +base-list --format json` | 只代表最近访问，不得宣称全量 |
| 按名称找 Table | `dws aitable +resolve-table --base <baseId> --name "<表名>" --format json` | `baseId` 必须来自上一步真实返回 |
| 取表、字段与视图目录 | `dws aitable +table-get --base-id <baseId> [--table-ids <tableId>] --format json` | `tables[].fields[]` 是字段目录；完整类型/config 再用 `+field-get` |
| 取字段完整配置 | `dws aitable +field-get --base-id <baseId> --table-id <tableId> [--field-ids <ids>] --format json` | 写入前核对类型、只读性和 select options；按需展开以控制返回体 |
| 查/搜/筛记录 | `dws aitable +record-query --base-id <baseId> --table-id <tableId> [--query <词>\|--filters '<JSON>'\|--record-ids <ids>] --format json` | ID 模式忽略 filter/sort；全量结论必须完整分页 |
| 新增记录 | `dws aitable record create --base-id <baseId> --table-id <tableId> --records '[{"cells":{"<fieldId>":<值>}}]' --format json` | 单次最多 100；取 `data.newRecordIds[]` 后立即按 ID 回读 |
| 更新记录 | `dws aitable record update --base-id <baseId> --table-id <tableId> --records '[{"recordId":"<id>","cells":{"<fieldId>":<值>}}]' --format json` | 先 query 拿 recordId；只传需改字段；取 `data.recordIds[]` 后回读 |
| 删除记录 | 先 `dws aitable +record-query ...` 定位，再 `dws aitable record delete --base-id <baseId> --table-id <tableId> --record-ids <ids>` | 展示目标与影响，得到明确确认后才加 `--yes` |
| 创建 Base / Table | `dws aitable base create --name "<名>"` / `dws aitable table create --base-id <id> --name "<名>" --fields '[...]'` | 使用创建返回的真实 ID；系统改名/加后缀时不得继续猜原名 |
| 批量追加 CSV / JSON 到已有表 | `python3 scripts/import_records.py <baseId> <tableId> <file> [batch_size]` | CSV 表头必须是 fieldId；脚本返回不完整 ledger 时不得宣称全成功 |
| 文件导入为新数据表 | `python3 scripts/aitable_import_via_task.py <baseId> <file>` | 与“追加已有 table”不同；走 prepare → PUT → import task |
| 批量创建字段 | `python3 scripts/bulk_add_fields.py <baseId> <tableId> fields.json` | 单次最多 15；逐项检查成功/失败结果 |
| 导出 Base / Table / View | `python3 scripts/aitable_export_via_task.py <baseId> --scope all\|table\|view [...]` | 保存路径、覆盖与异步未完成状态必须显式处理 |
| 上传记录附件 | `python3 scripts/upload_attachment.py <baseId> <file>` | 返回 `fileToken` 后仍需按字段格式写入记录并回读 |

## 记录读写不变量

- `record create/update` 前必须获取目标字段的 `fieldId`、`type` 与 `config`；`filterUp`、`lookup` 等只读字段不可写。完整格式只在需要时读 [aitable-cell-value.md](references/aitable/aitable-cell-value.md)。
- 筛选和排序字段使用 `fieldId`；`--filters` 最外层是 `and|or + operands`，`--sort` 使用 `direction: asc|desc`。日期和跨表字段规则按需读 [aitable-filter-sort.md](references/aitable/aitable-filter-sort.md)。
- `record query --all` 仍受 `--page-limit` 约束；分页中断或局部富化失败时保留已有结果，输出 completeness 与逐项失败 ledger，不把部分结果描述为全量。
- 创建、更新、导入、批量建字段等写操作必须检查业务 `status`、逐项结果与返回 ID；普通写入按用户明确要求执行后回读，不能只凭退出码宣称成功。
- 长 JSON 使用 `--records-file` / 任务文件；不得为绕过字段错误而静默丢列、改类型或删除失败项。

## 低频能力与 Reference

| 场景 | 按需读取 |
|---|---|
| 完整命令索引、对象 URL 与一级路由 | [aitable.md](references/aitable.md) |
| 记录 query/create/update/delete/upsert/history/share | 对应 `references/aitable/aitable-record-*.md` |
| 字段创建、字段 config、cellValue、公式与跨表引用 | [aitable-field.md](references/aitable/aitable-field.md)、[aitable-field-properties.md](references/aitable/aitable-field-properties.md)、[aitable-cell-value.md](references/aitable/aitable-cell-value.md)、[aitable-formula-guide.md](references/aitable/aitable-formula-guide.md) |
| 筛选、排序、统计、全量分析 | [aitable-filter-sort.md](references/aitable/aitable-filter-sort.md)、[aitable-data-analysis-sop.md](references/aitable/aitable-data-analysis-sop.md) |
| 导入导出、附件 | [aitable-export-import.md](references/aitable/aitable-export-import.md)、[aitable-attachment.md](references/aitable/aitable-attachment.md) |
| 视图、表单、仪表盘与图表 | [aitable-view-config.md](references/aitable/aitable-view-config.md)、[aitable-view-extras.md](references/aitable/aitable-view-extras.md)、[aitable-form.md](references/aitable/aitable-form.md)、[aitable-dashboard-chart.md](references/aitable/aitable-dashboard-chart.md) |
| 高级权限、自动化工作流、导航节点 | [aitable-advperm.md](references/aitable/aitable-advperm.md)、[aitable-workflow.md](references/aitable/aitable-workflow.md)、[aitable.md](references/aitable.md) 的 section 路由 |

## 错误恢复

- 路径或 flag 错误：按既定的 leaf Schema → leaf Help 顺序校正一次；仍失败则停止，不连续尝试猜测别名。
- 命令非零、输出非 JSON、业务 `status != success`、必需 ID 缺失、批处理部分失败均视为失败；保留成功项与 ledger，禁止吞错。
- 同名歧义、权限不足、资源不存在、字段类型漂移、分页无法推进或 Schema/Help 冲突时停止并报告。具体恢复动作按需读 [aitable-error-recovery.md](references/aitable/aitable-error-recovery.md)。
- 每次重试都从最新实际输出重新提取下游 ID；删除和其他 `confirmation=user_required` 操作不得自动重试或静默确认。

## 跨产品协作

- 电子表格工作表、单元格与公式 → `dingtalk-sheet`；结构化 Base/Table/Field/Record 才走本 skill。
- 普通文档内容 → `dingtalk-doc`；钉盘普通文件与文件夹 → `dingtalk-drive`；记录附件上传仍走本 skill。
- 用户直接提供类型不明的 alidocs URL 时，按 `dws-shared` 的 URL 预检导航确认 `extension=able` 后再执行。
- 听记内容入表：先用 `dingtalk-minutes` 提取结构化结果，再按本 skill 的字段与记录规则写入。
