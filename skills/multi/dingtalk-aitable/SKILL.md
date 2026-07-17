---
name: dingtalk-aitable
description: 钉钉 AI 表格（多维表）。Use when 用户说 AI表格/多维表/数据表/base/table/建表/查记录/写数据/字段/记录增删改查/筛选/排序/公式/模板搜索/批量导入CSV或JSON/导出/仪表盘/图表/上传附件到表格/按字段类型建表。不做电子表格单元格读写（走 dingtalk-sheet）、文档编辑（走 dingtalk-doc）；听记待办入表见配方 minutes-todo-to-aitable。命令前缀：dws aitable。
cli_version: ">=0.2.14"
metadata:
  category: product
  stability: experimental
  requires:
    bins:
      - dws
---

# 钉钉 AI 表格 Skill

> 🧪 **EXPERIMENTAL · 试验版 / Preview** — multi 模式当前未达 stable 标准。全部 dingtalk-* skill 已通过 dispatch verifier，但接口、命名、跨 skill 引用后续可能调整；生产 / 共享环境请优先使用 mono 模式（`dws skill setup --mode mono`）。问题请提 issue 反馈。

## 前置条件 — 执行操作前必读

> **`use_skill(dws-shared)`** — 认证、全局参数（`--format json` / `--yes`）、错误码、URL 模板、跨产品消歧、安全规则与 capability 边界。**执行任何 `dws` 命令前先读；** 单产品的清晰命令可直接用本 skill。

<!-- SAFETY_PREAMBLE_INJECT -->

> 命令参考：[aitable.md](references/aitable.md)；复杂命令按需加载 `references/aitable/*.md`；剧本：[06-data-analytics.md](references/06-data-analytics.md)。
> 高频模块：[Base](references/aitable/aitable-base-index.md) · [Table](references/aitable/aitable-table-index.md) · [Field](references/aitable/aitable-field-index.md) · [Record](references/aitable/aitable-record-index.md) · [View](references/aitable/aitable-view-index.md) · [Form](references/aitable/aitable-form-index.md) · [Workflow](references/aitable/aitable-workflow-index.md)。

<!-- VISIBLE_SHORTCUTS_START -->
## Shortcuts（无专用脚本/recipe 时优先）

以下 shortcut 来自独立于 Runtime Schema 的公开 catalog。先按本 skill 的意图表、脚本和 recipe 路由：存在精确覆盖该场景的专用脚本/recipe 时按其执行；否则用户意图命中时，shortcut 优先于手写原子命令。用 `dws shortcut list --service aitable --format json` 读取参数、约束、风险和示例，并以 `dws aitable <shortcut> --help` 核对当前 Cobra flags；不要对 `+` 路径调用 `dws schema`。

| Shortcut | 风险 | 适用场景 |
|---|---|---|
| `dws aitable +base-get` | read | 获取指定 Base 的目录信息（tables / dashboards summary） |
| `dws aitable +base-list` | read | 获取当前用户可访问的 AI 表格 Base 列表（最近访问，支持游标分页） |
| `dws aitable +base-search` | read | 按名称关键词搜索 AI 表格 Base |
| `dws aitable +chart-get` | read | 获取指定 chart 的详细信息 |
| `dws aitable +chart-widgets-example` | read | 获取所有图表类型的 widget config 示例 |
| `dws aitable +dashboard-config-example` | read | 获取 dashboard config 的结构示例 |
| `dws aitable +dashboard-get` | read | 获取指定 dashboard 的详细信息（含 charts summary） |
| `dws aitable +field-get` | read | 批量获取字段详情（含类型相关完整配置） |
| `dws aitable +find-record` | read | 在指定多维表里按关键词查记录（只读） |
| `dws aitable +form-field-list` | read | 列出表单视图当前可见的字段及其配置 |
| `dws aitable +form-list` | read | 列出指定数据表下的所有表单视图 |
| `dws aitable +form-share-get` | read | 读取视图当前的分享表单配置 |
| `dws aitable +list-tables` | read | 列出某个多维表(base)里的所有数据表（只读，投影 tableId/tableName） |
| `dws aitable +record-history-list` | read | 按 recordId 查询单条记录的变更历史 |
| `dws aitable +record-query` | read | 查询表格记录（按 ID 取 / 条件筛选 / 关键词 / 分页） |
| `dws aitable +record-query-empty` | read | 扫描并过滤出完全没填用户字段的空行 |
| `dws aitable +record-share-links` | read | 批量（可 >20 条）获取多维表记录分享链接：去重+分片+合并 |
| `dws aitable +record-share-url` | read | 按 recordId 批量获取记录分享链接，单次最多 20 条 |
| `dws aitable +resolve-base` | read | 按名称搜索多维表 Base 并解析出唯一 baseId（只读） |
| `dws aitable +resolve-table` | read | 在某个多维表 Base 内按名称解析出唯一的数据表 tableId（只读） |
| `dws aitable +role-list` | read | 列出指定 Base 下的全部角色 |
| `dws aitable +section-list-empty` | read | 列出指定 Base 下所有没有子节点的空文件夹 |
| `dws aitable +section-list-nodes` | read | 列出指定 Base 当前版本下的全部 nsheet 节点 |
| `dws aitable +table-get` | read | 批量获取指定数据表的表级信息、字段目录与视图目录 |
| `dws aitable +template-search` | read | 按名称关键词搜索 AI 表格模板 |
| `dws aitable +view-get` | read | 获取视图完整信息（列顺序、筛选、排序、分组等） |
| `dws aitable +view-get-frozen-cols` | read | 获取视图当前冻结的左侧列数 |
| `dws aitable +view-get-lock` | read | 获取视图锁定状态 |
| `dws aitable +view-get-row-height` | read | 获取视图单元格行高（像素） |
<!-- VISIBLE_SHORTCUTS_END -->

## 意图表

| 用户说 | 命令 |
|--------|------|
| "搜表格 / 找一个 base" | `dws aitable base search --query "<名>"` |
| "创建 AI 表格 / 多维表" | `dws aitable base create --name "<名称>" [--template-id <id>]` |
| "查数据表 / 建数据表" | `dws aitable table get --base-id <baseId>` / `dws aitable table create --base-id <baseId> --name "<表名>" --fields '[...]'` |
| "查字段 / 字段类型" | `dws aitable field get --base-id <id> --table-id <id>` |
| "查记录 / 搜索记录" | `dws aitable record query --base-id <baseId> --table-id <tableId> [--filters '...']` |
| "写记录 / 更新记录 / 删除记录" | `dws aitable record create/update/delete --base-id <baseId> --table-id <tableId> ...` |
| "筛选 / 排序 / 公式 / 跨表引用" | 先读 `references/aitable/aitable-filter-sort.md` / `aitable-formula-guide.md` |
| "批量导入 JSON / CSV" | `python scripts/import_records.py <baseId> <tableId> data.csv\|data.json` |
| "批量加字段" | `python scripts/bulk_add_fields.py --base-id <id> --table-id <id> --fields fields.json` |
| "导入 / 导出表格" | 先读 `references/aitable/aitable-export-import.md`；导出优先 `python scripts/aitable_export_via_task.py <baseId> --scope table --table-id <tableId>` |
| "仪表盘 / 图表" | 先读 `references/aitable/aitable-dashboard-chart.md` |
| "上传附件到记录" | 先读 `references/aitable/aitable-attachment.md`；可用 `python scripts/upload_attachment.py --base-id <id> --file <path>` |

## 标准 SOP（必遵流程）

> 命中以下意图**必须**按对应 SOP 顺序执行；**禁止**跳步、替换命令、编造 flag/ID。每条命令必须带 `--format json`，执行后必须按"解析"步取真实字段，不得凭返回结构猜测。`baseId`/`tableId`/`fieldId`/`recordId` 一律先查后用，**禁止默认/编造**。

### SOP-1 定位 Base 与 Table（list / search → table get）

**触发**：找/打开某张 AI 表格、不知 baseId 或 tableId。

1. **选源（必须）**：有名称/关键词 → `dws aitable base search --query "<名称>"`；列最近访问 → `dws aitable base list`。`base list` 仅返回最近访问，不是全部，**禁止**当作全量清单。
2. **执行（必须）**：`dws aitable base search --query "<完整名>" --format json`（或 `dws aitable base list --format json`）。
3. **解析（必须）**：从 JSON 取真实 `baseId`；**多候选必须输出让用户选，禁止默认取第一个**。
4. **取 tableId（必须）**：`dws aitable table get --base-id <baseId> --format json` → 从 `tables[].tableId` 取目标表 ID，连同该表 `fields[]` 的 `fieldId/fieldName/type` 与 `views[]` 的 `viewId` 一并记下。
5. **失败（必须）**：`base list` 为空或不命中 → 换 `base search --query` 关键词重试一次；仍无果**必须如实告知**，禁止臆造 baseId/tableId。

**禁止**：跳过 `table get` 直接用字段名写记录、用模糊名匹配当 baseId、用旧会话里的 ID 不再校验。

### SOP-2 拿字段定义（field get，写记录/改字段前置）

**触发**：建/改/写记录、改字段名或 options、按字段类型拼写入参前。

1. **前置（必须）**：先按 SOP-1 拿到 `baseId` + `tableId`。
2. **执行（必须）**：`dws aitable field get --base-id <baseId> --table-id <tableId> --format json`（仅展开需要的字段时加 `--field-ids fld1,fld2`，单次最多 10 个）。
3. **解析（必须）**：取每个目标字段的 `fieldId`、`type`、`config`（如 singleSelect/multipleSelect 的 `options[].id|name`）；写入 cells 的 key **必须用 `fieldId`**，不是字段中文名；select 字段过滤/写入传**选项名称字面量**，不传 option ID。
4. **衔接（必须）**：拿到字段定义 → 进入 SOP-3 写记录、或 `dws aitable field update --field-id <fieldId> --name <新名>|--config <JSON> --format json` 改字段。
5. **失败（必须）**：字段不存在或类型不符 → 重新 `field get` 核对，**禁止**凭旧名称/旧类型继续写入。

**禁止**：用字段中文名当 cells key、跳过 `field get` 直接 `record create/update`、对 select 字段传 option ID 当写入值。

### SOP-3 写/批量写记录（record create）

**触发**：新增记录、批量加数据、CSV/JSON 入表。

1. **前置（必须）**：SOP-1 取 `baseId`/`tableId` + SOP-2 取 `fieldId`/类型。
2. **执行（必须）**：`dws aitable record create --base-id <baseId> --table-id <tableId> --records '[{"cells":{"<fieldId>":<值>}}]' --format json`；单次最多 100 条，超长用 `--records-file ./data.json`。
3. **写入格式（必须）**：按 `record create --help` 类型表严格传值（text→字符串、number→数值、singleSelect→"选项名"、date→RFC3339、url→`{"text","link"}`、group→`{"cid"}` 等）；`filterUp`/`lookup` 字段只读不可写。
4. **解析（必须）**：从返回取真实 `recordId`；名称被系统加后缀或字段重命名时，**后续一律用返回的新 ID**，禁止继续按原名称猜。
5. **失败（必须）**：类型/格式错误按返回报错修正后重试，**禁止**降级丢弃字段；不确定格式先 `field get` 复核 config。

**禁止**：编造 fieldId/recordId、跳过 `field get` 凭中文名写、把 URL 字符串直接塞给 url 字段。

### SOP-4 查/筛/排记录（record query）

**触发**：查记录、按条件筛选、排序、取关联记录、定位待改/待删的 recordId。

1. **前置（必须）**：SOP-1 拿 `baseId`/`tableId`。
2. **执行（必须）**：`dws aitable record query --base-id <baseId> --table-id <tableId> --format json`；已知 ID 直取加 `--record-ids rec1,rec2`（忽略 filters/sort，单次≤100）。
3. **筛选/排序（必须）**：`--filters` 最外层必须 `{"operator":"and|or","operands":[...]}`，select 字段值传**选项名字面量**；日期只能用 `date_eq/before/after/not_before/not_after`，范围用 `not_before`+`not_after` 组合，**禁止** `eq`/区间/相对时间。`--sort` 用 `[{"fieldId":"..","direction":"asc|desc"}]`（**必须用 `direction`**）。公式/引用/关联字段默认不返回，需显式 `--field-ids` 指定。
4. **解析（必须）**：取真实 `recordId` 与字段值；分页用 `--cursor`，全表用 `--all --page-limit N`。
5. **衔接（必须）**：拿到 recordId → SOP-5 更新、`record delete --record-ids --yes` 删除（删前确认）。

**禁止**：用字段名做 filter/sort key、对日期用 `eq`、漏掉 `direction` 用旧 `order` 字段、用本地过滤替代服务端 filter。

### SOP-5 更新记录（record update）

**触发**：改记录字段值、批量更新状态、单字段重命名需求之外的记录改动。

1. **前置（必须）**：SOP-1 拿 `baseId`/`tableId`；SOP-2 拿字段类型；SOP-4 拿目标 `recordId`。
2. **执行（必须）**：`dws aitable record update --base-id <baseId> --table-id <tableId> --records '[{"recordId":"recXXX","cells":{"<fieldId>":<新值>}}]' --format json`（每条必含 `recordId`+`cells`，单次≤100；超长用 `--records-file`）；只传需改字段，未传保持原值。
3. **解析（必须）**：写入格式同 SOP-3；返回核对真实 recordId 与受影响字段。
4. **失败（必须）**：recordId 不存在或类型不符 → 回 SOP-4 重新定位，**禁止**编造 ID 强写。

**禁止**：省略 `recordId`、用字段中文名当 cells key、凭空猜测 recordId 直接 update。

## 危险操作

`base delete` / `table delete` / `field delete` / `record delete` 不可逆，必须先向用户确认再加 `--yes`。

## 高频硬约束

- 创建 Base、改字段或写记录时，固定按“获取真实 ID → 执行写命令 → 回读验证”顺序完成，禁止跳过任一步。
- 字段重命名使用 `dws aitable field update --base-id <baseId> --table-id <tableId> --field-id <fieldId> --name "<新名称>" --format json`；先 `field get` 找真实 `fieldId`，不要猜字段名能直接更新。
- 写记录前必须 `field get` 获取 `fieldId` 与类型；`record create/update` 的 `cells` key 用 `fieldId`，不是字段中文名。长 JSON 使用 `--records-file`。
- 表或字段创建返回名称被系统自动加后缀时，后续必须使用返回的真实 `tableId`/`fieldId`，不要继续按原名称猜。
- `record update/delete` 先 `record query/list` 定位 `recordId`；删除必须确认，普通新增/更新按用户明确要求可直接执行后读回验证。
- `record query/create/update/delete`、`field create`、导入导出、图表和附件场景必须先读对应 `references/aitable/*.md`，不要凭旧单文件参数猜 flag。

## 字段类型规则

详见本 skill 的 [field-rules.md](references/field-rules.md)。

## 跨产品协作

- 单元格 / 工作表 / 公式 → 切到 `dingtalk-sheet`（命令前缀：`dws sheet`）
## 局部意图与 Recipe

- [局部意图消歧](references/intent-guide.md)。
