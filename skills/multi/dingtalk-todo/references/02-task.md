# Todo 组合生命周期

当一个请求包含多个资源动作并需要传递 `taskId`、`commentId`、`attachmentId` 或 `tagCode` 时使用本文件。组合链路可以逐步混用 shortcut 与原子命令，但只能传递已经规范化的稳定 ID，不能把一种入口的完整返回结构当成另一种入口的结构。

## 通用骨架

1. 先列出完整动作序列和需要传递的 ID，不要边执行边发现路线。
2. 未指定执行人时运行 `dws contact me --format json`；指定姓名时运行 `dws aisearch person --query "<姓名>" --dimension name --format json`，唯一匹配后取 `userId`。
3. 选择创建入口：后续只有搜索、详情回读和清理时可用 `+remind` / `+create`；若要列表筛选、资源写入、创建子资源或一次创建多个对象，使用原子创建：

   ```bash
   dws todo task create --title "<标题>" --executors <USER_ID> [--priority 10|20|30|40] [--due "<截止ISO>"] [--recurrence "<规则>"] --format json
   ```

4. 原子创建从成功响应的 `result.taskId` 取 ID；创建 shortcut 从其成功结果的 `taskId` 取 ID。需要详情时优先 `dws todo +get --task-id <TASK_ID> --format json`。
5. 按下表执行后续动作；每一步都复用真实 ID。表中原子命令是保底路径；若对应 `+get`、`+search`、`+complete`、`+reopen`、`+update`、`+comment`、`+reminder` 或 `+list-*` 完整覆盖当前步骤，则优先使用 shortcut 自带的校验。
6. 只清理本次创建且已记录 ID 的对象；删除类操作先走 Runtime 确认门。删除后 `task get` 不存在或列表移除才算清理完成。

## 原子路线表

| 意图 | 写命令 | 核验 / ID 来源 |
|---|---|---|
| 按状态/优先级/角色/日期查询 | `task list --status ... --priority ... --role-types ... --plan-finish-date-start ... --plan-finish-date-end ...` | 遍历 `result.todoCards[]`；`hasMore=true` 时递增 `--page` |
| 更新标题/优先级/截止时间 | `task update --task-id <TASK_ID> ...` | `task get --task-id <TASK_ID>` 逐字段核验 |
| 完成 / 重开 | `task done --task-id <TASK_ID> --status true|false` | `task get` 或对应状态的 `task list` |
| 创建子待办 | `task create-sub --parent-id <PARENT_ID> --title "<标题>" --executors <USER_ID>` | 取响应 `result.taskId`；`task list-sub --task-id <PARENT_ID>` |
| 增删执行人 | `task add-executor --task-id <TASK_ID> --executors <USER_ID>` / `task remove-executor --task-id <TASK_ID> --executors <USER_ID>` | `task get`；只移除本次明确添加的人 |
| 增删参与人 | `task add-participant --task-id <TASK_ID> --participants <USER_ID>` / `task remove-participant --task-id <TASK_ID> --participants <USER_ID>` | `task get`；执行人与参与人不可混用 |
| 添加评论 | `comment add --task-id <TASK_ID> --content "<内容>"` | 从 `comment list` 的 `result.comments[].id` 取真实评论 ID 并核对内容 |
| 删除评论 | `comment delete --task-id <TASK_ID> --comment-id <COMMENT_ID>` | 再次 `comment list` 确认目标不存在 |
| 上传附件 | `task add-attachment --task-id <TASK_ID> --file <绝对路径>` | `task list-attachment` 取真实 `attachmentId` 并核对文件名 |
| 移除附件 | `task remove-attachment --task-id <TASK_ID> --attachment-id <ATTACHMENT_ID>` | 再次 `task list-attachment` |
| 添加截止前提醒 | `task add-reminder --task-id <TASK_ID> --base-time dueTime --due-date-offset -30` | 只有终端写回执；待办必须已有截止时间 |
| 添加独立提醒 | `task add-reminder --task-id <TASK_ID> --base-time customTime --reminder-time-stamp "<提醒ISO>"` | 只有终端写回执 |
| 替换/清空提醒 | `task reset-reminder --task-id <TASK_ID> [--reminder-rules '<JSON数组>']` | 不传规则即清空；不能声称已读回规则 |
| 创建/列出标签 | `tag create --name "<名称>"` / `tag list` | 创建前后比较 `result.userTags[]`，只接受唯一新增的 `code` |
| 关联标签 | `tag add --task-id <TASK_ID> --tag-codes <TAG_CODE_1>[,<TAG_CODE_2>]` | `task get` 或写回执；最多两个标签 |
| 改名标签 | `tag update --user-tags '[{"code":"<TAG_CODE>","name":"<新名称>"}]'` | `tag list` 核对同一 `code` 的名称 |
| 删除标签定义 | `tag delete --tag-codes <TAG_CODE>` | `tag list` 确认标签定义不存在 |
| 删除待办 | `task delete --task-id <TASK_ID>` | `task get` 不存在或对应列表已移除 |

所有表中命令都加前缀 `dws todo` 和后缀 `--format json`。

## 动态 ID 账本

| ID | 只允许来自 | 禁止来源 |
|---|---|---|
| `taskId` | 原子 `task create/create-sub/get/list` 或 Todo shortcut 的成功业务返回 | 标题、URL、展示序号、其他 case |
| `commentId` | 同一 `taskId` 的 `+comment` 成功结果或 `comment list` 的 `result.comments[].id` | 评论文本或猜测 |
| `attachmentId` | 同一 `taskId` 的 `task list-attachment` 返回 `attachments[].attachmentId` | 文件名或本地路径 |
| `tagCode` | `todo tag create/list` 返回 `result.userTags[].code` | 标签名或 Git tag |
| `userId` | `contact me` 或 `aisearch person` 的唯一匹配 | 姓名、手机号片段、其他 profile |

“待办标签”只能调用 `dws todo tag ...`；禁止运行 `git tag`。本地待办附件只能调用 `dws todo task add-attachment`，不要改走 Drive。

## 失败与恢复

- `unknown command/flag`：查该 leaf 的 `--help` 后最多修正一次，不要轮询相似命令。
- 创建、评论等非幂等写超时或返回不明：保留“可能已提交”，先按标题/父 ID 查询对账，不自动重试。
- 提醒接口没有查询能力：成功时只报告服务端接受写入；失败时保留原错误。
- 任一步失败后仍要按账本清理已创建的临时对象；未取得稳定 ID 的对象不得猜 ID 清理。
