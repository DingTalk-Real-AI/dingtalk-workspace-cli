---
name: dingtalk-todo
description: 钉钉待办 / TODO。Use when 用户说 创建待办/TODO/任务提醒/指派任务/标记完成/查待办/紧急待办/循环待办/批量建待办/逾期待办。不做日报周报（走 dingtalk-misc）、审批（走 dingtalk-misc）、日程（走 dingtalk-calendar）。命令前缀：dws todo。
metadata:
  cli_version: ">=0.2.14"
  category: product
  requires:
    bins:
      - dws
---

# 钉钉待办 Skill

## 执行契约

- 执行第一个 `dws` 操作前完整读取 [`dingtalk-shared`](../dingtalk-shared/SKILL.md)；当前任务已加载则不重复读取。
- 先把请求拆成有序步骤，再逐步选入口。已知命令直接执行，不先查 Help、Schema 或 Shortcut Catalog；只有当前 leaf 的 flag 或安全语义确实不明时才查精确 leaf。
- 所有命令加 `--format json`，按结构化业务返回判断结果。后续 ID 只取自本次真实返回；零匹配、多匹配或类型不明时停止并消歧。
- 写操作遵循最终 Runtime gate。需要确认时先说明对象、动作和影响，用户确认后才追加 `--yes`；不要把 `--yes` 写入存储示例。
- 写后必须核验。非幂等写超时、缺少稳定 ID 或读回失败时先查询对账，禁止盲目重放。

<!-- VISIBLE_SHORTCUTS_START -->
## Shortcuts（无专用脚本/recipe 时优先）

以下 shortcut 同时进入公开 catalog 与 Runtime Schema。先按本 skill 的意图表、脚本和 recipe 路由：存在精确覆盖该场景的专用脚本/recipe 时按其执行；否则用户意图命中时，shortcut 优先于手写原子命令。命令已选中时直接执行；只在参数或安全语义不确定时读取 Agent leaf Schema（例如 `dws schema --cli-path "todo +<shortcut>" --compact --format json`），在当前 Cobra flags 不确定时读取 `dws todo <shortcut> --help`。只有参数映射、接口绑定或 provenance 审计才省略 `--compact`。仅当现有路由和 reference 都无法定位低频能力时，才用 `dws shortcut list --service todo --format json` 批量发现。

| Shortcut | 风险 | 适用场景 |
|---|---|---|
| `dws todo +assign` | write | 按姓名给某人创建并指派一条待办（自动解析 userId） |
| `dws todo +assign-multi` | write | 把一条待办按姓名一次性指派给多个人（自动把每个姓名解析成 userId） |
| `dws todo +comment` | write | 添加待办评论并读回验证 |
| `dws todo +create` | write | 创建待办并读回验证 |
| `dws todo +created-todos` | read | 列出我创建的待办（我作为创建人 creator 发起的待办，而非分配给我执行的） |
| `dws todo +due-today` | read | 列出我今天到期的待办 |
| `dws todo +get` | read | 查询待办详情 |
| `dws todo +get-my-tasks` | read | 查询当前组织下我的待办列表 |
| `dws todo +get-related-tasks` | read | 一次性列出与我相关的全部待办（我作为创建人/执行人/参与人三种角色的并集，按 taskId 去重） |
| `dws todo +list-attachment` | read | 查询待办任务的附件列表 |
| `dws todo +list-comment` | read | 查询待办评论列表 |
| `dws todo +list-sub` | read | 查询子待办列表 |
| `dws todo +overdue` | read | 列出我已过期未完成的待办 |
| `dws todo +remind` | write | 给自己创建一条带可选截止时间的待办 |
| `dws todo +reminder` | write | 设置或清除待办提醒（仅终端回执） |
| `dws todo +search` | read | 搜索与我相关的全部待办 |
| `dws todo +todo-done` | write | 按标题关键词把我的某条待办标记完成（自动定位 taskId） |
| `dws todo +update` | write | 更新待办并读回验证 |
<!-- VISIBLE_SHORTCUTS_END -->

## 路由优先级

上面的通用 Shortcut 优先规则只适用于**一个 Shortcut 完整覆盖当前步骤**的情况。不要因为请求里出现“创建”就默认使用 `+remind` / `+create`。

1. **先选创建入口**：按姓名指派用 `+assign` / `+assign-multi`；给自己记一条且后续只有搜索、详情或清理时用 `+remind`；已有真实 `userId` 且只需创建、回读和清理时用 `+create`。
2. **组合生命周期从原子创建开始**：创建后还要按状态/优先级/角色/日期/页码列举，或继续更新、完成/重开、提醒、评论、附件、成员、子待办、标签，或一次创建多个对象时，使用 `todo task create`。不要用创建 Shortcut 代替第一步。
3. **后续按步骤选最窄入口**：聚合、搜索和核验完整由 Shortcut 覆盖时使用 `+get-my-tasks`、`+get-related-tasks`、`+due-today`、`+overdue`、`+search`、`+get`、`+complete`、`+reopen`、`+update`、`+comment`、`+reminder` 或 `+list-*`；需要原子特有参数、动态子资源 ID、多对象或中间状态时用原子命令。
4. **确定性批量/汇总使用脚本**：批量创建、今天/明天/本周汇总、逾期扫描分别使用 bundled script。
5. **跨步骤只传稳定 ID**：Shortcut 与原子命令可以共存，但只传递规范化后的 `taskId`、`commentId`、`attachmentId`、`tagCode`、`userId`；不要假设两类入口的完整返回结构相同。

## Golden Routes

| 用户意图 | 首选入口 | 关键结果 / 边界 |
|---|---|---|
| 给自己创建，随后只搜索、看详情或清理 | `dws todo +remind --task "<标题>" [--at "<截止ISO>"] --format json` | 自动解析当前用户；`--at` 是截止时间，不是提醒时间 |
| “给张三建待办” | `dws todo +assign --to "张三" --task "<标题>" --format json` | 姓名必须唯一解析后才创建 |
| “给张三、李四建同一条待办” | `dws todo +assign-multi --to "张三,李四" --task "<标题>" --format json` | 任一姓名不唯一则零写入 |
| 已有 `userId`，只创建、回读和清理 | `dws todo +create --title "<标题>" --executors <USER_ID> [--due "<截止ISO>"] [--priority 10\|20\|30\|40] --format json` | 返回稳定 `taskId`，并读回核验标题 |
| 创建后还要筛选、变更资源或创建多个对象 | `dws todo task create --title "<标题>" --executors <USER_ID> ... --format json` | 从 `result.taskId` 进入组合生命周期；不要以 `+remind` / `+create` 起步 |
| 今天到期 / 已逾期 | `dws todo +due-today --format json` / `dws todo +overdue --format json` | 均有界拉全分页；空集合也是成功结果 |
| 当前组织下我的执行待办 | `dws todo +get-my-tasks --all --status false --format json` | `--all` 达到 40 页仍未耗尽会失败，不伪装完整 |
| 与我相关的全部待办 | `dws todo +get-related-tasks --format json` | 创建人、执行人、参与人三种角色并集，按 `taskId` 去重 |
| 按标题关键词查询 | `dws todo +search --query "<关键词>" --format json` | 搜索与 list 不混用；跨全部分页匹配 |
| 已知 `taskId` 查详情 | `dws todo +get --task-id <TASK_ID> --format json` | 详情必须回传同一个稳定 `taskId` |
| 已知 `taskId` 完成 / 重开 | `dws todo +complete --task-id <TASK_ID> --format json` / `dws todo +reopen ...` | 先读当前状态，避免重复写，再读回核验 |
| 只记得标题，标记完成 | `dws todo +todo-done --task "<关键词>" --format json` | 仅唯一命中时写；零个或多个候选均停止 |
| 修改标题、截止时间或优先级 | `dws todo +update --task-id <TASK_ID> ... --format json` | 至少指定一个待改字段；写后逐字段核验 |
| 设置独立提醒 | `dws todo +reminder --task-id <TASK_ID> --base-time customTime --at "<提醒ISO>" --format json` | 上游无提醒查询接口，只能返回终端写回执，`verified=false` |
| 基于截止时间提前提醒 | `dws todo +reminder --task-id <TASK_ID> --base-time dueTime --due-date-offset -30 --format json` | 待办必须已有截止时间；偏移单位为分钟 |
| 清除全部提醒 | `dws todo +reminder --task-id <TASK_ID> --clear --format json` | 清除写操作；不能与提醒参数混用 |
| 批量创建 | `python scripts/todo_batch_create.py <todos.json>` | 单批最多 30 条；保留逐项 `taskId`、verified/unverified/unknown 状态，不盲重试 |
| 今天/明天/本周汇总 | `python scripts/todo_daily_summary.py today\|tomorrow\|week` | 走 `+get-my-tasks --all`，只纳入范围内且有截止时间的未完成待办 |

## 低频原子能力

组合请求先读 [组合生命周期](references/02-task.md)。以下常用原子命令已审定，直接执行，不要先猜别名或查 Help：

| 意图 | 命令骨架 |
|---|---|
| 解析自己 / 姓名 | `dws contact user get-self --format json` / `dws aisearch person --query "<姓名>" --dimension name --format json` |
| 创建 / 子待办 | `dws todo task create ...` / `dws todo task create-sub --parent-id <PARENT_ID> --title "<标题>" --executors <USER_ID> ...` |
| 列表 / 详情 | `dws todo task list [--status true\|false] [--priority ...] [--role-types ...] [--page N --size N] ...` / `dws todo task get --task-id <TASK_ID>` |
| 更新 / 完成或重开 | `dws todo task update --task-id <TASK_ID> ...` / `dws todo task done --task-id <TASK_ID> --status true\|false` |
| 增删执行人 / 参与人 | `task add-executor` / `task remove-executor` / `task add-participant` / `task remove-participant`，均传真实 `taskId` 与 `userId` |
| 评论 | `comment add` / `comment list` / `comment delete`；删除使用列表返回的真实 `commentId` |
| 附件 | `task add-attachment --file <绝对路径>` / `task list-attachment` / `task remove-attachment` |
| 提醒 | `task add-reminder` 添加单条；`task reset-reminder` 替换全部或清空 |
| 标签 | `tag create` / `tag list` / `tag update` / `tag add` / `tag delete`；只使用真实 `tagCode` |
| 删除待办 | `dws todo task delete --task-id <TASK_ID> --format json` |

删除类操作必须由用户确认；若用户在当前请求中已明确授权“办完后删除/清理本次创建对象”，该授权只覆盖本次记录的精确 ID。附件上传会真实传输本地文件，不能用来试探权限。

## 关键约束

- 标题、URL、展示序号都不是 `taskId`。已知 ID 直接行动；未知 ID 用列表/搜索定位，零匹配或多匹配时停止。
- 待办公开命令统一使用 `--task-id`；`--id` / `--ids` 只是隐藏兼容别名，不要写入新命令或示例。
- 优先级：低=10、普通=20、较高/高/重要=30、紧急/最高/P0=40。
- `--due` / `+remind --at` 表示 deadline；独立 reminder 必须走 `+reminder`。
- 自定义时间提醒的原子 flag 是 `--reminder-time-stamp`；不要把 Shortcut 的 `--at` 套到 `task add-reminder`。
- `task list` 使用 `--status`，不要写 `--done`；详情是 `task get`，不存在 `task detail`。
- “待办标签”始终属于 Todo，使用 `dws todo tag ...`；绝不能解释成 Git tag、通讯录标签或其他产品标签。
- 创建和评论是非幂等写。超时、缺少稳定 ID 或读回失败时保留“可能已提交/未核验”状态，先查询对账，禁止盲目重放。
- 所有命令加 `--format json`；写 Shortcut 按 Runtime 安全契约确认，确认前不得自行附加 `--yes`。
- 会后行动项来自听记时先走 `dingtalk-minutes`；OA 审批走 `dingtalk-misc`；时间块和会议走 `dingtalk-calendar`。

## 按需参考

- [局部意图消歧](references/intent-guide.md)
- [轻量流程](references/lite-recipes.md)
- [组合流程](references/02-task.md)
- [完整命令参考](references/todo.md)
