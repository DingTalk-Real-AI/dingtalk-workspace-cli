---
name: dingtalk-todo
description: 钉钉待办 / TODO。Use when 用户说 创建待办/TODO/任务提醒/指派任务/标记完成/查待办/紧急待办/循环待办/批量建待办/逾期待办。Distinct from dingtalk-report(日报周报)、dingtalk-oa(审批)、dingtalk-calendar(日程)。命令前缀：dws todo。
cli_version: ">=0.2.14"
metadata:
  category: product
  stability: experimental
  requires:
    bins:
      - dws
---

# 钉钉待办 Skill

> 🧪 **EXPERIMENTAL · 试验版 / Preview** — multi 模式当前未达 stable 标准。20 个 dingtalk-* skill 全部通过 dispatch verifier，但接口、命名、跨 skill 引用后续可能调整；生产 / 共享环境请优先使用 mono 模式（`dws skill setup --mode mono`）。问题请提 issue 反馈。

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> ⚠️ **命令可用性可能因企业服务发现配置而异**。本文档列出的命令基于 dws envelope schema 与本仓库 v1.0.30 实测，但部分命令的 cobra 子命令暴露与否还取决于你的企业 MCP gateway 是否注册了对应 tool。如果跑某条命令报 `unknown command` 或 fall back 到父级 help，说明当前账号企业未开通该能力。实际调用前可用 `dws <cmd> --help` 或 `--dry-run` 验证。


> 命令参考：[todo.md](references/todo.md)；剧本：[02-task.md](references/02-task.md)。

## 开放平台文档 RAG / 错误码排查

- 任何产品执行中，只要用户问开放平台 API、接口参数、字段含义、权限点、回调、SDK、配额、错误码，或命令返回上游 OpenAPI/SDK 错误，必须先用 `dws devdoc article search --query "<关键词>" --format json` 做官方文档 RAG。
- 查询词优先保留原始 API 名、能力名、权限点、完整错误码和 message；首轮形如 `errcode <code> <message>`，无结果再换 `<产品/场景> <错误码>`、`<接口名> 参数`。
- 本地 CLI 错误（如 `unknown command` / `unknown flag` / 认证 / recovery）仍按 root `dws` / `dws-shared` 的错误处理执行；`devdoc` 用于开放平台业务错误码和接口语义排查。
- `devdoc` 只查钉钉开放平台开发者文档，不查业务数据；排查结论必须基于命中条目的标题、摘要或链接，不能编造错误原因或不存在的命令。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "建一条待办给张三" | `dws todo task create --title "<标题>" --executors <userId>` |
| "较高 / 高优先级待办" | `dws todo task create ... --priority 30`（10低/20普通/30较高/40紧急） |
| "紧急 / 最高优先级 / 立即处理" | `dws todo task create ... --priority 40` |
| "循环待办（每天）" | `dws todo task create ... --due "<首次截止ISO>" --recurrence "DTSTART:<UTC>\nRRULE:FREQ=DAILY;INTERVAL=1"` |
| "批量建待办（JSON 文件）" | `python scripts/todo_batch_create.py todos.json` |
| "今天 / 本周未完成待办" | `python scripts/todo_daily_summary.py [today\|tomorrow\|week]` |
| "逾期待办" | `python scripts/todo_overdue_check.py` |
| "标记完成 / 重开" | `dws todo task done --task-id <taskId> --status true\|false` |
| "修改标题/截止时间/优先级" | `dws todo task update --task-id <taskId> ...` |
| "删除待办" | `dws todo task delete --task-id <taskId>`（需用户确认） |

## 参数硬约束

- 任务详情只用 `dws todo task get --task-id <taskId>`；不要写 `task detail`。
- 完成状态首选 `dws todo task done --task-id <taskId> --status true|false`；若用 `update`，也必须是 `--task-id` + `--done true|false`。
- 查询列表完成状态用 `dws todo task list --status false|true --format json`。不要写 `--done true` 作为可见参数，虽然兼容但不作为推荐写法。
- `--id` / `--ids` 是隐藏兼容别名，文档和生成命令统一写 `--task-id`，减少模型漂移。
- 优先级映射：低=10，普通=20，较高/高/重要=30，紧急/最高/P0/马上处理=40；不要把"较高"写成 40。
- 截止时间必须是 ISO-8601。相对日期按当前日期计算；例如周五说"下周二"就是紧接下一个自然周的周二，不要再加一周。
- 创建、标记完成、重开、删除后必须 `task get` 或对应 `task list --status ...` 验证，不要只凭创建返回或口头计划结束。
- 所有 dws 命令带 `--format json`。

## 跨产品协作

- 执行人是人名 → 先用 `dingtalk-aisearch` 拿 `userId`
- 会后从听记自动建待办 → 切到 `dingtalk-minutes`
- 项目进度汇总写文档 → 切到 `dingtalk-doc`
