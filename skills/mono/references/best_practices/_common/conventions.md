# 通用规范

> full recipe 执行时的共享规范。安全门控、危险操作确认、`--format json` 等已在根 [SKILL.md](../../../SKILL.md) 中定义，此处不重复。
> Recipe 元规范（YAML frontmatter、命名、三层架构）见 [recipe-conventions.md](recipe-conventions.md)。

## 批量查询规范

| # | 规范 |
|---|------|
| 1 | **并行查详情**：拿到多个 ID 后，用 `&` 合并到同一条 Shell 命令并行执行 + `wait`，**严禁逐条串行** |
| 2 | **翻页**：分页接口须拉全直至无更多 |
| 3 | **优先批量 API**：有批量接口则用批量；无则按 #1 并行 |
| 4 | **群消息**：必须先 `chat search --query` 得 `openConversationId`，再 `chat message list --group`；多群同条命令并行 |
| 5 | **列表少轮次**：带条件搜索/列表 → 一次采全详情；**禁止**无新参数时重复同一 `list` / `search` |

## 多源并行采集（公共模式）

> recipe 引用方式：`按「多源并行采集」执行（关键词=<X>，时间=<Y>至<Z>）`。

- 同条 Shell：`&` 并行 + `wait`；分页须采全。
- 只保留与主题相关的数据，无关丢弃。
- 有批量详情接口优先；否则并行拉详情（见上表 #1）。
- 具体采哪些产品列表由对应 **行动指南 recipe** 与 [SKILL 产品参考](../../../SKILL.md) 决定。

## 字段术语与 ID 传递

> list 返回 JSON 后，必须提取下表字段传给后续命令。**禁止用其他字段替代。**

| 字段 | 来源 | 传递给 |
|------|------|--------|
| `taskUuid` | `minutes list` | `minutes get summary/info/batch --id(s)` |
| `userId` | `aisearch person` / `contact user search` / `contact dept list-members` | `contact user get --ids`、`todo --executors`、`calendar --users` |
| `deptId` | `contact dept search` | `contact dept list-members --depts`；多子部门时对每个子部门分别 `dept search` 取 id |
| `nodeId` | `doc search` | `doc read/update/copy/move/rename --node` |
| `nodeId` | `doc list` 中的 folder 类型节点 / `doc folder create` | `doc list --folder`、`doc create --folder`、`doc upload --folder`、`doc copy/move --folder` |
| `eventId` | `calendar event list` | `calendar event get/update --id` |
| `processInstanceId` | `oa approval list-*` | `oa approval detail/approve --instance-id` |
| `openConversationId` | `chat search` | `chat message list/send --group` |
| `todoTaskId` | `todo task list` | `todo task update/done --task-id` |
| `reportId` | `report inbox list` / `report outbox list` | `report entry get/stats --report-id` |
| `baseId` / `tableId` | `aitable base search` | `aitable record query --base-id --table-id` |
| `dentryUuid` | `drive list` / `drive mkdir` | `drive info/download --file-id`、`drive list/mkdir/upload --parent-id` |
| `dentryId` | `drive info` 的数字字段 | 仅用于 `chat message send --dentry-id` |

**ID 边界硬约束**：`dentryId` 通常是纯数字，只表示聊天文件消息需要的钉盘条目数字 ID；它不是父目录 ID。遇到 `drive --parent-id`、`doc --folder`、`doc --node` 时，只能使用 `dentryUuid` / `nodeId` / 文档 URL。若当前上下文只有数字型 `dentryId`，必须先重新 `drive list` / `doc list` / `doc search` 获取正确 ID，不能把该数字直接代入后续命令。
