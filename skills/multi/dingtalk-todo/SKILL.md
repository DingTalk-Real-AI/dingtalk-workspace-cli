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

- 只用 `dws` 操作钉钉待办，命令统一加 `--format json`，并按结构化业务返回判断结果。
- 已知路线直接执行；仅在 leaf 参数或安全语义不确定时查精确 Schema，在 flag 不确定时查精确 Help。不要先枚举整个 Catalog，也不要连续猜命令。
- 后续 ID 必须来自本次真实返回；零匹配、多匹配或类型不明时停止并消歧。
- 写操作遵循最终 Runtime gate。需要确认时先说明对象、动作和影响，用户确认后才追加 `--yes`；不要把 `--yes` 写进存储示例。
- 写后必须核验。非幂等写超时、缺少稳定 ID 或读回失败时先对账，禁止盲目重放。

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

上面的通用 shortcut 优先规则只适用于单一、完整命中的请求。本 Skill 的实际优先级是：

1. **组合生命周期 → 原子命令闭环**：同一请求含创建后再列表、更新、完成、提醒、评论、附件、成员、子待办、标签或清理时，先读 [组合流程](references/02-task.md)，全程使用 `todo task/comment/tag ...` 原子命令。不要用 `+remind` / `+create` 代替组合流程的第一步，也不要混用两套返回结构。
2. **确定性批量/汇总 → 脚本**：批量创建、今天/明天/本周汇总、逾期扫描分别用 bundled scripts。
3. **单一意图 → Shortcut**：只在一个 shortcut 已经覆盖完整目标、校验和结果形态时直接使用。
4. **未知低频能力 → 精确 Reference/Help**：只读当前操作的小节，不要枚举后试错。

## 高频路线

| 用户意图 | 首选入口 | 边界 |
|---|---|---|
| 给自己创建一条普通待办 | `dws todo +remind --task "<标题>" [--at "<截止ISO>"] --format json` | `--at` 是截止时间，不是独立提醒 |
| 按姓名给一人/多人指派一条待办 | `+assign` / `+assign-multi` | 任一姓名不唯一就停止，不能猜 `userId` |
| 已有 `userId`，只创建并回读一条待办 | `dws todo +create --title "<标题>" --executors <USER_ID> ... --format json` | 结果必须含稳定 `taskId` 且读回一致 |
| 创建后还要做其他操作 | `dws todo task create ... --format json` | 从 `result.taskId` 进入同一原子命令闭环 |
| 按状态、优先级、角色、日期或页码枚举 | `dws todo task list ... --format json` | `--status false/true`；`hasMore=true` 继续翻页 |
| 当前组织我的待办 / 与我相关的全部待办 | `+get-my-tasks` / `+get-related-tasks` | 后者是创建人、执行人、参与人并集 |
| 按标题关键词定位 / 已知 ID 查详情 | `+search --query ...` / `+get --task-id ...` | 零个或多个候选时停止消歧 |
| 已知 ID 完成、重开、更新 | `+complete` / `+reopen` / `+update` | shortcut 会做状态检查或读回核验 |
| 今天到期 / 逾期 | `+due-today` / `+overdue` | 空集合也是成功结果 |
| 设置或清除独立提醒 | `+reminder` | 上游无提醒查询接口，只能报告写回执，不能声称读回 |

## 组合任务闭环

1. 执行前列出用户要求的全部资源动作及顺序；同一链路使用同一个 profile。
2. 原子创建必须先取得执行人：未指定执行人用 `dws contact me --format json`；指定姓名用 `dws aisearch person --query "<姓名>" --dimension name --format json` 并唯一匹配。
3. 创建后只从 `result.taskId` 提取稳定 ID。后续评论、附件和标签编号分别来自 `comment list`、`task list-attachment`、`tag create/list` 的真实返回。
4. 每次写后使用对应 read/list 核验；提醒例外，只保留终端写回执。最后仅清理本次创建且已经记录 ID 的对象。

## 关键约束

- 公开命令统一使用 `--task-id`；不要在新命令中使用隐藏兼容别名 `--id` / `--ids`。
- 优先级：低=10、普通=20、较高/高/重要=30、紧急/最高/P0=40。
- `--due` 与 `+remind --at` 表示 deadline；独立 reminder 用 `+reminder` 或原子 `task add-reminder`。
- `task list` 用 `--status`，不要写 `--done`；详情命令是 `task get`，不存在 `task detail`。
- “待办标签”始终使用 `dws todo tag ...`，绝不能解释为 Git tag、文档标签或其他产品标签。
- 本地文件作为待办附件时使用 `task add-attachment --file <绝对路径>`；先确认待办存在，不得用上传动作试探权限。
- 会后行动项先走 `dingtalk-minutes` 取真实内容；OA 审批走 `dingtalk-misc`；时间块和会议走 `dingtalk-calendar`。

## 按需参考

- [单步与短流程](references/lite-recipes.md)
- [组合生命周期与动态 ID 传递](references/02-task.md)
- [局部意图消歧](references/intent-guide.md)
- [完整原子命令参考](references/todo.md)
