# Todo 组合生命周期

当一个请求包含多个资源动作并需要传递 `taskId`、`commentId`、`attachmentId`、`tagCode` 或 `userId` 时使用本文件。先拆步骤，再逐步选择能完整覆盖当前步骤的 Shortcut 或原子命令；不要因为第一步是创建就让 `+remind` / `+create` 接管整个流程。

## 通用骨架

1. 列出完整动作序列和需要传递的 ID；同一链路使用同一个 profile。
2. 选择创建入口：
   - 按姓名创建并指派：`+assign` / `+assign-multi`。
   - 给自己简单创建，后续只有搜索、详情或清理：`+remind`。
   - 已有真实 `userId`，后续只有回读或清理：`+create`。
   - 后续需要列表筛选、更新、完成/重开、提醒、评论、附件、成员、子待办、标签或多对象：使用原子 `task create`。
3. 原子创建前解析执行人：未指定执行人用 `dws contact user get-self --format json`；指定姓名用 `dws aisearch person --query "<姓名>" --dimension name --format json`，唯一匹配后取 `userId`。
4. 原子创建：

   ```bash
   dws todo task create --title "<标题>" --executors <USER_ID> [--priority 10|20|30|40] [--due "<截止ISO>"] [--recurrence "<规则>"] --format json
   ```

5. 原子创建只从成功响应的 `result.taskId` 取 ID；创建 Shortcut 从其成功业务结果的 `taskId` 取 ID。不要跨入口猜字段层级。
6. 按下表执行后续步骤。Shortcut 完整覆盖一个步骤时可使用其核验能力；需要原子特有参数、动态子资源 ID、多对象或中间状态时使用原子命令。
7. 只清理本次创建且账本中已有精确 ID 的对象。删除后用 `task get` 不存在或对应列表移除验证。

## 步骤路由表

| 当前步骤 | 首选条件 | 入口 / 核验 |
|---|---|---|
| 当前组织我的未完成待办 | 需要有界拉全 | `+get-my-tasks --all --status false` |
| 与我相关的全部待办 | 需要按创建人/执行人/参与人合并去重 | `+get-related-tasks` |
| 今天到期 / 逾期 | 需要完整聚合 | `+due-today` / `+overdue` |
| 按标题关键词定位 | 需要跨页唯一匹配 | `+search --query "<关键词>"` |
| 按状态/优先级/角色/日期/页码列举 | 需要指定筛选条件或控制分页 | `task list --status ... --priority ... --role-types ... --plan-finish-date-start ... --plan-finish-date-end ... --page N --size N`；`hasMore=true` 继续下一页 |
| 已知 ID 查详情 | 需要稳定投影和 ID 核验 | `+get --task-id <TASK_ID>`；需要原始字段时用 `task get` |
| 单次更新并读回 | Shortcut flags 完整覆盖 | `+update --task-id <TASK_ID> ...` |
| 更新后还需中间列表或多对象对比 | 需要原子中间状态 | `task update --task-id <TASK_ID> ...`，再 `task get/list` |
| 单次完成 / 重开并读回 | 不需要按状态列表检查 | `+complete` / `+reopen` |
| 完成/重开后按状态列表确认，或处理多个/子待办 | 需要中间状态 | `task done --task-id <TASK_ID> --status true\|false`，再 `task list/get` |
| 创建子待办 | 需要父 ID、执行人和子 ID | `task create-sub --parent-id <PARENT_ID> --title "<标题>" --executors <USER_ID> ...`，取 `result.taskId` |
| 列出子待办 | 只需聚合读取 | `+list-sub --task-id <PARENT_ID>`；需要原始子 ID/状态时用 `task list-sub` |
| 增删执行人 | 成员变更 | `task add-executor/remove-executor --task-id <TASK_ID> --executors <USER_IDS>`，再 `task get` |
| 增删参与人 | 关注关系变更 | `task add-participant/remove-participant --task-id <TASK_ID> --participants <USER_IDS>`，再 `task get` |
| 添加一条评论并读回 | 不需要删除评论 | `+comment --task-id <TASK_ID> --content "<内容>"` |
| 多评论、评论 ID 或删除评论 | 需要动态评论 ID | `comment add --task-id <TASK_ID> --content "<内容>"`；`comment list --task-id <TASK_ID>`；从列表取真实 ID 后 `comment delete --task-id <TASK_ID> --comment-id <COMMENT_ID>` |
| 上传附件 | 本地文件已存在 | `task add-attachment --task-id <TASK_ID> --file <绝对路径>`，再列附件 |
| 查看或移除附件 | 只读可用 Shortcut；移除需要动态 ID | `+list-attachment --task-id <TASK_ID>`；需要 `attachmentId` 时用 `task list-attachment`，再 `task remove-attachment --task-id <TASK_ID> --attachment-id <ATTACHMENT_ID>` |
| 添加或清除一条提醒意图 | Shortcut 参数完整覆盖 | `+reminder`；只报告终端写回执 |
| 多提醒、整体替换或清空后继续重设 | 需要原子规则数组 | 截止偏移：`task add-reminder --base-time dueTime --due-date-offset -30`；自定义：`--base-time customTime --reminder-time-stamp "<提醒ISO>"`；替换/清空：`task reset-reminder [--reminder-rules '<JSON数组>']` |
| 标签生命周期 | 需要真实 `tagCode` | `tag create --name "<名称>"`；`tag list`；改名 `tag update --user-tags '[{"code":"<TAG_CODE>","name":"<新名称>"}]'`；关联 `tag add --task-id <TASK_ID> --tag-codes <TAG_CODES>`；删除 `tag delete --tag-codes <TAG_CODES>` |
| 删除待办 | 已获得本次授权和真实 ID | `task delete --task-id <TASK_ID>`，再 `task get` 验证不存在 |

所有表中命令都加前缀 `dws todo` 和后缀 `--format json`。

## 动态 ID 账本

| ID | 只允许来自 | 禁止来源 |
|---|---|---|
| `taskId` | 原子 `task create/create-sub/get/list` 或 Todo Shortcut 的成功业务返回 | 标题、URL、展示序号、其他 case |
| `commentId` | 同一 `taskId` 的 `+comment` 成功结果或 `comment list` 返回 | 评论文本或猜测 |
| `attachmentId` | 同一 `taskId` 的 `task list-attachment` 返回 | 文件名或本地路径 |
| `tagCode` | `todo tag create/list` 返回的 `result.userTags[].code` | 标签名或 Git tag |
| `userId` | `contact user get-self` 或 `aisearch person` 的唯一匹配 | 姓名、`me/self`、其他 profile |

“待办标签”只能调用 `dws todo tag ...`；禁止运行 `git tag`。本地待办附件只能调用 `dws todo task add-attachment`，不要改走 Drive。

## 失败与恢复

- `unknown command/flag`：查该精确 leaf 的 `--help` 后最多修正一次，不轮询相似命令。
- 创建、评论等非幂等写超时或返回不明：保留“可能已提交”，先按标题/父 ID 查询对账，不自动重试。
- 提醒接口没有查询能力：成功时只报告服务端接受写入；失败时保留原错误。
- 任一步失败后仍按账本清理已创建的临时对象；未取得稳定 ID 的对象不得猜 ID 清理。
