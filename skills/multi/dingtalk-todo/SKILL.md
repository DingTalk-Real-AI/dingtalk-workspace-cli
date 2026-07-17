---
name: dingtalk-todo
description: 钉钉待办 / TODO。Use when 用户说 创建待办/TODO/任务提醒/指派任务/标记完成/查待办/紧急待办/循环待办/批量建待办/逾期待办。不做日报周报（走 dingtalk-report）、审批（走 dingtalk-oa）、日程（走 dingtalk-calendar）。命令前缀：dws todo。
cli_version: ">=0.2.14"
metadata:
  category: product
  stability: experimental
  requires:
    bins:
      - dws
---

# 钉钉待办 Skill

> 🧪 **EXPERIMENTAL · 试验版 / Preview** — multi 模式当前未达 stable 标准。全部 dingtalk-* skill 已通过 dispatch verifier，但接口、命名、跨 skill 引用后续可能调整；生产 / 共享环境请优先使用 mono 模式（`dws skill setup --mode mono`）。问题请提 issue 反馈。

## 前置条件 — 执行操作前必读

> **`use_skill(dws-shared)`** — 认证、全局参数（`--format json` / `--yes`）、错误码、URL 模板、跨产品消歧、安全规则与 capability 边界。**执行任何 `dws` 命令前先读；** 单产品的清晰命令可直接用本 skill。

<!-- SAFETY_PREAMBLE_INJECT -->

> 命令参考：[todo.md](references/todo.md)；剧本：[02-task.md](references/02-task.md)。

<!-- VISIBLE_SHORTCUTS_START -->
## Shortcuts（无专用脚本/recipe 时优先）

以下 shortcut 来自独立于 Runtime Schema 的公开 catalog。先按本 skill 的意图表、脚本和 recipe 路由：存在精确覆盖该场景的专用脚本/recipe 时按其执行；否则用户意图命中时，shortcut 优先于手写原子命令。用 `dws shortcut list --service todo --format json` 读取参数、约束、风险和示例，并以 `dws todo <shortcut> --help` 核对当前 Cobra flags；不要对 `+` 路径调用 `dws schema`。

| Shortcut | 风险 | 适用场景 |
|---|---|---|
| `dws todo +assign` | write | 按姓名给某人创建并指派一条待办（自动解析 userId） |
| `dws todo +assign-multi` | write | 把一条待办按姓名一次性指派给多个人（自动把每个姓名解析成 userId） |
| `dws todo +created-todos` | read | 列出我创建的待办（我作为创建人 creator 发起的待办，而非分配给我执行的） |
| `dws todo +get` | read | 查询待办详情 |
| `dws todo +get-my-tasks` | read | 查询当前组织下我的待办列表 |
| `dws todo +list-attachment` | read | 查询待办任务的附件列表 |
| `dws todo +list-comment` | read | 查询待办评论列表 |
| `dws todo +list-sub` | read | 查询子待办列表 |
| `dws todo +overdue` | read | 列出我已过期未完成的待办 |
| `dws todo +remind` | write | 给自己创建一条带截止/提醒时间的待办 |
| `dws todo +todo-done` | write | 按标题关键词把我的某条待办标记完成（自动定位 taskId） |
<!-- VISIBLE_SHORTCUTS_END -->

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

## 标准 SOP（必遵流程）

> 命中以下意图**必须**按对应 SOP 顺序执行；**禁止**跳步、替换命令、编造 taskId。每条命令必须带 `--format json`。创建、完成或删除后按对应 SOP 回读验证。

### SOP-1 建待办（create-todo）

**触发**：建待办/任务提醒/指派任务/TODO。

1. **解析执行者（必须）**：指定姓名 → `dws aisearch person --keyword "<姓名>" --dimension name --format json` 取 `userId`；未指定 → `dws contact user get-self --format json` 取当前用户 `userId`；多人逐个搜索后英文逗号拼接。
2. **执行（必须）**：`dws todo task create --title "<标题>" --executors <userId>[,<userId2>...] --priority <10/20/30/40> --format json`；有截止时间加 `--due "<ISO>"`；循环待办加 `--due "<首次截止ISO>" --recurrence "DTSTART:<UTC>\nRRULE:FREQ=DAILY;INTERVAL=1"`。
3. **验证（必须）**：从返回取 `taskId`/`todoTaskId`，立即 `dws todo task get --task-id <taskId> --format json` 回读。

**禁止**：跳过执行者解析直接传姓名、用 `task detail` 取详情（正确是 `task get`）、创建后不回读。

### SOP-2 查询待办（query-todo）

**触发**：查待办/今天本周待办/未完成/已完成。

1. **执行（必须）**：`dws todo task list --status false|true --format json`（`false`=未完成、`true`=已完成、不传=全部）；`hasMore=true` 必须翻页。
2. **摘要脚本（必须）**：今天/本周未完成 → `python scripts/todo_daily_summary.py today|tomorrow|week`；逾期 → `python scripts/todo_overdue_check.py`。
3. **详情（必须）**：`dws todo task get --task-id <taskId> --format json`；按主题筛选先 `task list` 再按标题过滤，**禁止**编造主题查询 flag。

**禁止**：写 `--done true`（用 `--status true`）、编造主题筛选参数。

### SOP-3 完成 / 重开 / 改 / 删（mutate-todo）

**触发**：标记完成/重开/改标题截止优先级/删待办。

1. **执行（必须）**：完成/重开 `dws todo task done --task-id <taskId> --status true|false --format json`；修改 `dws todo task update --task-id <taskId> ...`；删除 `dws todo task delete --task-id <taskId>`（**必须**先与用户确认）。
2. **验证（必须）**：`task done`/`update` 后用 `task get` 或对应 `task list --status ...` 回读确认；`delete` 后用 `task get` 确认已不存在或列表已移除。

**禁止**：未确认就删除、用 `update --done`（首选 `task done --status`）、改动后不回读。

### SOP-4 批量建待办（batch-create）

**触发**：批量建待办/一次建多条。

1. **执行（必须）**：`python scripts/todo_batch_create.py todos.json`（JSON 文件，每条含 title/executors/priority/due）；执行者姓名需先批量解析成 userId 再写入 JSON。

**禁止**：在 JSON 里写姓名不写 userId、跳过脚本逐条手敲。

## 参数硬约束

- 任务详情只用 `dws todo task get --task-id <taskId>`；不要写 `task detail`。
- 完成状态首选 `dws todo task done --task-id <taskId> --status true|false`；若用 `update`，也必须是 `--task-id` + `--done true|false`。
- 查询列表完成状态用 `dws todo task list --status false|true --format json`。不要写 `--done true` 作为可见参数，虽然兼容但不作为推荐写法。
- `--id` / `--ids` 是隐藏兼容别名，文档和生成命令统一写 `--task-id`，减少模型漂移。
- 优先级映射：低=10，普通=20，较高/高/重要=30，紧急/最高/P0/马上处理=40；不要把"较高"写成 40。
- 截止时间必须是 ISO-8601。相对日期按当前日期计算；例如周五说"下周二"就是紧接下一个自然周的周二，不要再加一周。
- 创建、标记完成、重开或删除后，使用 `task get` 或对应 `task list --status ...` 验证最终状态。
- 所有 dws 命令带 `--format json`。

## 跨产品协作

- 执行人是人名 → 先用 `dingtalk-aisearch` 拿 `userId`
- 会后从听记自动建待办 → 切到 `dingtalk-minutes`
- 项目进度汇总写文档 → 切到 `dingtalk-doc`
## 局部意图与 Recipe

- [局部意图消歧](references/intent-guide.md)；[Lite Recipe](references/lite-recipes.md)。
