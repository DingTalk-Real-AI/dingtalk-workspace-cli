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

## 执行入口

执行任何 `dws` 操作前，完整读取 [`dws-shared`](../dws-shared/SKILL.md)，但不要预加载其 references。高频意图直接使用本文件骨架；仅特殊参数、复杂数据形态或边界不明时读取一个 branch reference。

## 加载与路由顺序

1. 命中下方高频意图时直接使用精确骨架，不先查 Help 或产品级 Schema。
2. 路由优先级固定为：精确骨架 / recipe > 匹配的公开 Shortcut > 原子命令。脚本只用于 Runtime 尚未覆盖的批量、文件传输或异步编排，不与普通原子命令竞争默认入口。
3. 参数、约束或安全语义不确定时只读 leaf Schema：`dws schema --cli-path "aitable <leaf>" --format json`；只有当前 Cobra flag 不确定时才读对应 `--help`。
4. 复杂字段、筛选、导入导出、视图、权限或工作流任务，按“低频能力与 Reference”只加载相关文件，不预读整个 `references/aitable/`。
5. 现有骨架和 reference 都无法定位能力时，才用 Runtime Shortcut Catalog 做最后发现；不得猜 `cli_path` 或 flag。
6. Schema、Help、reference 与实际返回冲突时采用更安全的解释并报告契约漂移；`confirmation=user_required` 时先确认，再添加 `--yes`。
7. 用户已给足目标、字段和数据时，按依赖链连续执行；中间结果只用于提取真实 ID 和判断停止条件，全部完成后统一回读并答复。

<!-- VISIBLE_SHORTCUTS_START -->
## Shortcut 发现（按需）

`aitable` 当前有 29 条公开 shortcut，完整清单保留在 Runtime Shortcut Catalog，根 Skill 不重复展开。

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

所有下游 ID 都从当前链路的结构化返回中提取；同名多候选必须让用户消歧，不默认取第一项。Base → Table → Field/Record/View 的容器关系必须保持一致，不跨 Base 或 Table 复用子对象 ID。

创建、复制、导入或新建字段/记录返回 ID 后，立即绑定同一请求中的“这个”“刚才新建的”等指代；除非用户明确转向历史资源，否则不得再按名称搜索并替换为旧对象。

## 核心意图与执行骨架

| 用户意图 | 首选骨架 | 必须保留的执行边界 |
|---|---|---|
| 按名称找 Base | `dws aitable +resolve-base --name "<名称>" --format json` | 唯一命中才继续；多候选停止并消歧 |
| 浏览最近访问 | `dws aitable +base-list --format json` | 只代表最近访问，不得宣称全量 |
| 搜索模板 | `dws aitable template search --query "<关键词>" --format json` | 只返回真实候选，不擅自套用模板或创建 Base |
| 按名称找 Table | `dws aitable +resolve-table --base <baseId> --name "<表名>" --format json` | `baseId` 必须来自上一步真实返回 |
| 取表、字段与视图目录 | `dws aitable +table-get --base-id <baseId> [--table-ids <tableId>] --format json` | `tables[].fields[]` 是字段目录；完整类型/config 再用 `+field-get` |
| 取字段完整配置 | `dws aitable +field-get --base-id <baseId> --table-id <tableId> [--field-ids <ids>] --format json` | 写入前核对类型、只读性和 select options；按需展开以控制返回体 |
| 查/搜/筛记录 | `dws aitable +record-query --base-id <baseId> --table-id <tableId> [--query <词>\|--filters '<JSON>'\|--record-ids <ids>] --format json` | ID 模式忽略 filter/sort；不要猜 `--page-limit`，分页使用返回的 cursor 与 leaf Help 中的真实分页 flag |
| 新增记录 | `dws aitable record create --base-id <baseId> --table-id <tableId> --records '[{"cells":{"<fieldId>":<值>}}]' --format json` | 单次最多 100；取 `data.newRecordIds[]` 后立即按 ID 回读 |
| 更新记录 | `dws aitable record update --base-id <baseId> --table-id <tableId> --records '[{"recordId":"<id>","cells":{"<fieldId>":<值>}}]' --format json` | 先 query 拿 recordId；只传需改字段；取 `data.recordIds[]` 后回读 |
| 删除记录 | 先 `dws aitable +record-query ...` 定位，再 `dws aitable record delete --base-id <baseId> --table-id <tableId> --record-ids <ids>` | 展示目标与影响，得到明确确认后才加 `--yes` |
| 创建 Base / Table | `dws aitable base create --name "<名>"` / `dws aitable table create --base-id <id> --name "<名>" --fields '[...]'` | 使用创建返回的真实 ID；系统改名/加后缀时不得继续猜原名 |
| 复制视图 | `dws aitable view duplicate --base-id <baseId> --table-id <tableId> --view-id <viewId> --new-name "<名>"` | `viewId` 必须属于当前表；不能用复制 Table 或新建 Dashboard 替代 |
| 创建图表 | 先读 `dws aitable chart config-example --format json`，再执行 `chart create ... --config '<JSON>' --layout '<JSON>'` | `--layout` 是必填项；不能只传 config 后依据退出码声称图表已创建 |
| 批量追加 CSV / JSON 到已有表 | `python3 scripts/import_records.py <baseId> <tableId> <file> [batch_size]` | CSV 表头必须是 fieldId；脚本返回不完整 ledger 时不得宣称全成功 |
| 文件导入为新数据表 | `python3 scripts/aitable_import_via_task.py <baseId> <file>` | 与“追加已有 table”不同；走 prepare → PUT → import task |
| 批量创建字段 | `python3 scripts/bulk_add_fields.py <baseId> <tableId> fields.json` | 单次最多 15；逐项检查成功/失败结果 |
| 导出 Base / Table / View | `python3 scripts/aitable_export_via_task.py <baseId> --scope all\|table\|view [...]` | 保存路径、覆盖与异步未完成状态必须显式处理 |
| 上传记录附件 | `python3 scripts/upload_attachment.py <baseId> <file>` | 返回 `fileToken` 后仍需按字段格式写入记录并回读 |

## 记录读写不变量

- `record create/update` 前必须获取目标字段的 `fieldId`、`type` 与 `config`；`filterUp`、`lookup` 等只读字段不可写。完整格式只在需要时读 [aitable-cell-value.md](references/aitable/aitable-cell-value.md)。
- 筛选和排序字段使用 `fieldId`；`--filters` 最外层是 `and|or + operands`，`--sort` 使用 `direction: asc|desc`。日期和跨表字段规则按需读 [aitable-filter-sort.md](references/aitable/aitable-filter-sort.md)。
- 记录分页不能凭经验拼 `--page-limit`。先用当前 leaf Help 确认 page-size/cursor 的真实名称；每页读取返回 cursor，直到明确终止。分页中断或局部富化失败时保留已有结果，输出 completeness 与逐项失败 ledger，不把部分结果描述为全量。
- 创建、更新、导入、批量建字段等写操作必须检查业务 `status`、逐项结果与返回 ID；普通写入按用户明确要求执行后回读，不能只凭退出码宣称成功。
- 长 JSON 使用 `--records-file` / 任务文件；不得为绕过字段错误而静默丢列、改类型或删除失败项。
- `table create --fields` 的键固定为 `fieldName` / `type` / `config`，不能写成 `name`；单选/多选类型固定为 `singleSelect` / `multipleSelect`，不能写 `select`。number formatter 不确定时先读 leaf Help，禁止猜 `INTEGER` 等值。

## 写入计划与验证

- 用户明确列出的“先创建、再加字段、然后写记录/建视图”等阶段是可观察的验收步骤，必须逐项真实执行；不能为了得到相似终态而折叠、重排或省略。
- 删除 Base/Table/Field/Record、关闭高级权限、删除角色和其他高风险动作，先固化目标 ID、所属容器、影响数量、副作用与可恢复性；确认前写调用为零。
- 批量导入、建字段和记录写入在第一笔写入前完成全部字段类型、只读性、关联表和文件边界校验；部分失败保留输入顺序与逐项 ledger。

| 写入对象 | 成功后必须验证 |
|---|---|
| Base / Table | 使用创建返回 ID 查询对象及所属关系 |
| Field | `+field-get` 核对 fieldId、type、config 与只读性 |
| Record | 使用返回 recordId 按 ID 回读目标 cells |
| View / Dashboard / Chart | 重新读取当前 Base/Table 下的对象配置 |
| 导入 / 导出任务 | 核对 task 状态、结果对象或输出文件完整性 |

退出码 0、`status=success`、空对象或仅有 taskId 都不能单独证明业务完成。字段未生效、回读不一致、分页不完整或异步任务未完成时，报告失败、部分完成或进行中，不得声称“全部完成”。

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
- 已确认远端未写入且契约声明可重试时，才从最新实际输出重新提取 ID 后重试；写入状态未知、删除和其他 `confirmation=user_required` 操作不得自动重试或静默确认。
- 参数校验错误必须先吸收错误中的精确 hint，再最多纠正一次；不要重复猜同一 flag、formatter、folderId 或图表配置。目标能力已有原生命令时，不要以手工重建相似对象掩盖原命令失败。

## 跨产品协作

- 电子表格工作表、单元格与公式 → `dingtalk-sheet`；结构化 Base/Table/Field/Record 才走本 skill。
- 普通文档内容 → `dingtalk-doc`；钉盘普通文件与文件夹 → `dingtalk-drive`；记录附件上传仍走本 skill。
- 用户直接提供类型不明的 alidocs URL 时，按 `dws-shared` 的 URL 预检导航确认 `extension=able` 后再执行。
- 听记内容入表：先用 `dingtalk-minutes` 提取结构化结果，再按本 skill 的字段与记录规则写入。
