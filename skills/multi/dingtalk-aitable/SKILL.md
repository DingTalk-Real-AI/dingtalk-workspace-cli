---
name: dingtalk-aitable
description: 钉钉 AI 表格（多维表）。Use when 用户说 AI表格/多维表/数据表/base/table/建表/查记录/写数据/字段/记录增删改查/筛选/排序/公式/模板搜索/批量导入CSV或JSON/导出/仪表盘/图表/上传附件到表格/按字段类型建表。Distinct from 主 dws skill 的 dws sheet(电子表格/单元格读写/公式)、dws doc(文档编辑)。命令前缀：dws aitable。
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

> **PREREQUISITE:** Read the root `dws` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> ⚠️ **命令可用性以当前 dws 二进制为准**。服务发现已下线，本文档随内置 skill 发布；如果 `dws <cmd> --help` 不存在，说明当前版本未暴露该命令。若命令存在但调用失败，请按错误中的 endpoint 或 tool 提示确认静态端点目录和后端工具注册。实际调用前可用 `dws <cmd> --help` 或 `--dry-run` 验证。


> 命令参考：[aitable.md](references/aitable.md)；复杂命令按需加载 `references/aitable/*.md`；剧本：[06-data-analytics.md](references/06-data-analytics.md)。

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

## 危险操作

`base delete` / `table delete` / `field delete` / `record delete` 不可逆，必须先向用户确认再加 `--yes`。

## 评测高频硬约束

- 创建/改字段/写记录是多轮连续任务时，不能在"让我执行/先获取 ID"后停下；必须实际调用对应 `dws aitable` 命令并验证结果。
- 字段重命名使用 `dws aitable field update --base-id <baseId> --table-id <tableId> --field-id <fieldId> --name "<新名称>" --format json`；先 `field get` 找真实 `fieldId`，不要猜字段名能直接更新。
- 写记录前必须 `field get` 获取 `fieldId` 与类型；`record create/update` 的 `cells` key 用 `fieldId`，不是字段中文名。长 JSON 使用 `--records-file`。
- 表或字段创建返回名称被系统自动加后缀时，后续必须使用返回的真实 `tableId`/`fieldId`，不要继续按原名称猜。
- `record update/delete` 先 `record query/list` 定位 `recordId`；删除必须确认，普通新增/更新按用户明确要求可直接执行后读回验证。
- `record query/create/update/delete`、`field create`、导入导出、图表和附件场景必须先读对应 `references/aitable/*.md`，不要凭旧单文件参数猜 flag。

## 字段类型规则

详见本 skill 的 [field-rules.md](references/field-rules.md)。

## 跨产品协作

- 单元格 / 工作表 / 公式 → 走主 `dws` skill 的 `sheet` 产品路由（`dws sheet`）
