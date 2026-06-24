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

> 🧪 **EXPERIMENTAL · 试验版 / Preview** — multi 模式当前未达 stable 标准。20 个 dingtalk-* skill 全部通过 dispatch verifier，但接口、命名、跨 skill 引用后续可能调整；生产 / 共享环境请优先使用 mono 模式（`dws skill setup --mode mono`）。问题请提 issue 反馈。

> **PREREQUISITE:** Read the root `dws` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> ⚠️ **命令可用性可能因企业服务发现配置而异**。本文档列出的命令基于 dws envelope schema 与本仓库 v1.0.30 实测，但部分命令的 cobra 子命令暴露与否还取决于你的企业 MCP gateway 是否注册了对应 tool。如果跑某条命令报 `unknown command` 或 fall back 到父级 help，说明当前账号企业未开通该能力。实际调用前可用 `dws <cmd> --help` 或 `--dry-run` 验证。


> 命令参考：[aitable.md](references/aitable.md)；复杂命令按需加载 `references/aitable/*.md`；剧本：[06-data-analytics.md](references/06-data-analytics.md)。

## 开放平台文档 RAG / 错误码排查

- 任何产品执行中，只要用户问开放平台 API、接口参数、字段含义、权限点、回调、SDK、配额、错误码，或命令返回上游 OpenAPI/SDK 错误，必须先用 `dws devdoc article search --query "<关键词>" --format json` 做官方文档 RAG。
- 查询词优先保留原始 API 名、能力名、权限点、完整错误码和 message；首轮形如 `errcode <code> <message>`，无结果再换 `<产品/场景> <错误码>`、`<接口名> 参数`。
- 本地 CLI 错误（如 `unknown command` / `unknown flag` / 认证 / recovery）仍按 root `dws` / `dws-shared` 的错误处理执行；`devdoc` 用于开放平台业务错误码和接口语义排查。
- `devdoc` 只查钉钉开放平台开发者文档，不查业务数据；排查结论必须基于命中条目的标题、摘要或链接，不能编造错误原因或不存在的命令。

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
