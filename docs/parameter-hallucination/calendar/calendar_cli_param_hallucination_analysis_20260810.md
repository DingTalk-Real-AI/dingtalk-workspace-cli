# DWS 日程 CLI 参数幻觉与参数契约分析

分析日期：2026-08-10
分析基线：`codex/fix-im-reliability@b243b38d650a93143ec39bba37441ffc4af802ed`
分析对象：DWS `calendar` 产品的正式原生命令、公开内置快捷命令、运行时 Schema、Cobra Help、日程 Skill、命令实现和当前中央参数别名表。

## 1. 结论摘要

日程产品的基础参数契约是一致的，但业务实体较多、同一实体在原生命令与快捷命令中换名明显，因此仍然容易诱发参数幻觉。本轮从固定提交重新构建临时二进制，盘点了 44 个公开可执行命令，其中包括 24 个原生命令和 20 个公开内置快捷命令。运行时 Schema 发布 41 个工具；`calendar acl add`、`calendar acl delete` 和 `calendar book update` 是有明确理由的 reviewed exclusions，仍属于公开可执行能力，已一并纳入分析。

对 41 个 Schema 工具逐条比较完整运行时 Schema 与可见 Cobra Help 后，参数名集合差异为 0。当前没有发现“Schema 发布了 CLI 不接受的可见参数”这种基础漂移。41 个 Schema 工具共有 134 次参数出现、40 个不同参数名；包含 reviewed exclusions 后，44 个公开命令中有 37 个带业务参数。

本轮识别 10 类聚合问题，在 Excel 中展开为 92 条命令级表现。风险主要集中在五组：

- 标识符角色：同一个 eventId 在 10 个命令中叫 `--event`、在 5 个命令中叫 `--id`；calendarId 多数叫 `--calendar-id`，日历本 get/update 又叫 `--id`。同一命令同时出现 eventId 与 calendarId 时，裸 `--id` 不能安全猜测。
- 人员值域：`--attendees/--users` 接收 userId 列表，`--open-dingtalk-ids` 是另一种标识符，`+book/+invite/+suggest-time --with` 和 `+free --who` 则接收姓名并进行解析。姓名和稳定 ID 不能仅改参数名互换。
- 会议室角色：`--rooms` 是 roomId 列表，`--room-name` 是搜索词，`--group-id` 是会议室分组 ID，`--location` 只是日程地点文本；四者不能混用。
- 时间与分页单位：多数 `start/end` 是 ISO-8601 时间点，`+free-slots --from/--to` 却是整数小时；`cursor`、零基 `pageIndex` 与相对日期 `in-days` 也不能通过改名互换。
- 结构化输入：`--files` 需要 `<fileId>:<name>`，循环日程需要成组 recurrence 参数，提醒是“开始前多少分钟”的 CSV；别名表不能自动补字段、组装 JSON 或做时间计算。

当前正式中央表只对 `calendar event list` 生成 1 条日程命令规则，形成 10 对 alias 和 3 条保护。大量原生命令另有隐藏 Cobra 兼容参数，但快捷命令与危险值域边界没有被统一治理。

本轮候选表新增 11 个 calendar concept，扩展 6 个已有 concept，并增加 26 条精确命令 override。候选生成后覆盖 28 个日程命令，形成 181 对 alias、210 条保护和 1 条 ambiguous；所有非日程生成规则保持逐字节一致。

候选已通过 JSON/生成器、`internal/cli`、`internal/pipeline`、generated drift、Schema assembly determinism 和 Schema policy，并完成两组最终 dry-run payload 等价验证和三组 dispatch 前保护验证。完整 `internal/app` 按仓库政策仍要求为 20 个新增活跃命令补齐 complete-command E2E 模板，涉及 39 个活跃 alias fixture。因此候选是“语义审核与生成链路已通过、可作为下一轮编码输入，但不能直接替换正式表”的草稿。

## 2. 分析基线与覆盖范围

本报告没有使用历史 badcase、`dws-eval`、评测 JSON 或历史实验工作簿作为产品事实来源。事实来自同一提交的当前产品面：

| 分析项 | 数量或结果 |
|---|---:|
| 公开可执行 calendar 命令 | 44 |
| 原生命令 | 24 |
| 公开内置 `+` shortcut | 20 |
| 运行时 Schema 工具 | 41 |
| reviewed Schema exclusions | 3 |
| Schema 参数出现次数 | 134 |
| 不同公开 Schema 参数名 | 40 |
| 带业务参数的公开命令 | 37 |
| Help/Schema 可见参数集合差异 | 0 |
| 正式表当前 calendar alias 命令 | 1 |

运行时 Schema 来源为 `runtime-assembled`，Catalog hash 为 `sha256:5d953fea6f9417039454c5d37b9c1e9f49189e9b56faa7627865e2403b0c8ebc`，surface hash 为 `sha256:936e55fe818f238b853752c7ed890eef22c4fd62545b04c9d9b0fdb280de9caf`。分析没有把提交态 Catalog、用户自定义 shortcut 或插件作为官方参数事实。

## 3. 主要参数问题

### 3.1 eventId 在 `event/id` 之间换名，裸 `id` 可能同时指向 calendarId

`+attendee-list`、`+cancel-event`、`+invite`、`+reschedule`、attachment/attendee/room 子资源命令使用 `--event`；`event get/delete/instances/respond/update` 使用 `--id`。这些值都属于 eventId，但参数名由命令形态决定。

原生命令已广泛注册隐藏 `event-id/eventId` 兼容参数，中央表不需要重复接管。候选新增 `calendar_event_id`，主要覆盖尚未具备同类兼容能力的快捷命令，并在事件角色唯一的 `+cancel-event/+invite/+reschedule` 上允许 `id → event`。

`+attendee-list` 同时有必填 `--event` 和可选 `--calendar-id`。此时裸 `--id` 有两个同等合理目标，候选将它标为 ambiguous，不静默选择。

### 3.2 calendarId 多数叫 `calendar-id`，日历本自身却使用 `id`

15 个 Schema 工具公开 `--calendar-id`，用于指定日历容器；`calendar book get` 和 reviewed exclusion `calendar book update` 则使用 `--id` 表示 calendarId。该 `id` 与 eventId、aclId、roomId 都不是一个值域。

原生日历命令已有隐藏 `calendar/calendarId` 兼容参数。候选新增 `calendar_book_id`，只在 `+agenda/+attendee-list` 等缺口上提供 `calendar/calendar-book-id → calendar-id`，并对可能混入 eventId、aclId、roomId 的拼写加保护。`+agenda --id` 被 block；`+attendee-list --id` 因双角色被 ambiguous。

### 3.3 人员姓名、userId 与 openDingTalkId 是三种不同输入域

`attendee add/delete/event create --attendees` 和 `busy search/event suggest/+freebusy --users` 接收稳定 userId 列表；`event create --open-dingtalk-ids` 接收另一类开放标识符。快捷命令 `+book/+invite/+suggest-time --with` 与 `+free --who` 接收姓名，并在命令内部进行人员解析。

候选分别建立姓名列表、单姓名和 ID 列表 concept，只在相同值域与基数内改名。`attendee-names → with` 可以成立，`user-ids → with` 不成立；`room/person name → users` 也不成立。后两类通过 block 引导模型选择 resolver 快捷命令或先查到稳定 ID。

### 3.4 roomId、roomName、room groupId 与 location 不能合并

`event create/room add/room delete/+freebusy/busy search --rooms` 接收 roomId 列表；`room search/+room-search --room-name` 是展示名搜索词；`room search --group-id` 是会议室分组 ID；`event create/update --location` 只是地点备注文本，不会预订会议室。

候选只允许 `room-ids → rooms`，以及在精确 `room search` 上允许 `room-group-id/group → group-id`、在 `+room-search` 上允许 `query/name → room-name`。roomName、roomId、groupId 和 location 之间全部保持保护边界。

### 3.5 `start/end` 是 ISO 时间，`+free-slots from/to` 是整数小时

14 个命令的 `--start/--end` 表达 ISO-8601 时间点，适合复用已有 `time_start/time_end` concept。`+free-slots --from/--to` 表达本地工作窗口小时，例如 `9` 和 `18`，同名 `from/to` 的值类型和单位已经不同。

候选把 ISO 时间同义名限制在经过审核的精确命令；`to → end` 使用精确 override，不加入全局 `time_end`，因为 `to` 在 `+free-slots` 上是整数。`+free-slots` 只允许 `start-hour/end-hour → from/to`，并阻止 ISO 时间拼写。

### 3.6 cursor、pageIndex、limit 不是一套可随意互换的分页参数

`event list/instances/+agenda` 使用 `cursor + limit`；`room search/list-groups/+room-groups` 使用零基 `pageIndex`（CLI 为 `page`）和 `limit`。`page` 不能改成 cursor，`cursor` 也不能转换为 pageIndex。

候选扩展已有 `pagination_size/page_cursor` 到经过审核的快捷命令，允许 `max-results/page-size/next-cursor` 等同值映射；`+room-groups` 只精确允许 `page-index → page`，并阻止 cursor/page-token。

### 3.7 标题、描述和搜索词存在 `title/summary/desc/query` 换名

事件创建/更新和 `+book` 使用 `--title`，calendar book update 使用 `--summary`；事件描述使用 `--desc`，book update 同样公开 `--desc` 但兼容 `description`；日历本搜索使用 `--query`，快捷命令同样表达名称关键词。

候选在 `+book` 建立 `summary/subject → title`，在 `+book-search` 扩展搜索同义名。原生命令已有 `summary/title`、`description/desc` 隐藏兼容时继续由命令自身处理。`rich-text-desc` 是 HTML 内容，不能与普通 `desc` 自动合并。

### 3.8 attachment、recurrence 与 reminder 需要结构化输入

`attachment add --files` 要求逗号分隔的 `<fileId>:<name>`；裸 `file-id` 缺少文件名，别名层无法补齐。循环日程的 recurrence 参数存在整组必填与类型相关约束；提醒 `--remind-minutes` 是相对开始时间的分钟偏移 CSV，不是绝对提醒时间。

候选只允许 `reminder-minutes → remind-minutes` 这种同单位映射，并阻止 `reminder-time/remind-at`。`file-id` 被 block，循环规则不提供任何“一个参数变完整规则”的 alias。

### 3.9 `status` 是参会响应，`free-busy` 是日程忙闲状态

`event respond --status` 的值域是 attendee response enum；`event create/update --free-busy` 的值域是 `busy/free`。二者都像“状态”，但业务对象、枚举和值的去向不同。

候选新增 `calendar_response_status`，只允许 `response-status/response → status`；`state/done/free-busy/availability` 在该命令上被保护。`event update` 仅允许拼写等价的 `freebusy → free-busy`。

### 3.10 duration、timezone 与相对日期只能做同单位同格式治理

`event suggest/+suggest-time --duration` 是分钟；`event create/suggest/update --timezone` 是 IANA 时区；`+conflicts/+free-slots --in-days` 是从今天起的整数天偏移。

候选允许 `duration-minutes → duration`、`tz/time-zone → timezone`、`day-offset/days-from-today → in-days`，但不做分钟与小时、时区与 UTC offset、相对天数与 ISO 日期之间的转换。

## 4. 当前别名表可以实施的方案

候选文件基于正式表完整复制后增加 calendar 改动，正式文件未修改。主要方案为：

1. 新增 `calendar_event_id`、`calendar_book_id`、`calendar_event_title`、`calendar_person_name_list`、`calendar_person_name`、`calendar_room_ids`、`calendar_duration_minutes`、`calendar_day_offset`、`calendar_reminder_minutes`、`calendar_response_status`、`calendar_timezone`；
2. 扩展已有 `search_query`、`pagination_size`、`page_cursor`、`user_ids`、`time_start`、`time_end` 的精确 calendar 命令范围；
3. 对 `id` 双角色、姓名/ID、会议室四类角色、ISO/小时、cursor/pageIndex、结构化 files 和提醒单位使用 block/ambiguous；
4. 保留原生命令已有隐藏 Cobra 兼容参数，不把同一兼容逻辑机械复制到中央表；
5. 所有自动 alias 只改参数名，参数值保持原样。

## 5. 当前能力支持不了的事项

- 从日程标题、日历本名称或用户描述查出 eventId/calendarId；需要读接口和唯一性判断。
- 姓名与 userId/openDingTalkId 互转；需要人员解析，且可能存在同名歧义。
- 会议室展示名、楼层文案或 groupId 转成 roomId；必须先执行 room search/list-groups。
- ISO-8601、Unix 毫秒、整数小时和相对天数之间转换。
- cursor 与零基 pageIndex 之间转换，或根据服务端返回生成下一页游标。
- 把裸 fileId 自动包装成 `<fileId>:<name>`，或替用户补文件名。
- 根据自然语言自动生成完整 recurrence pattern/range 参数组。
- 把绝对提醒时间换算成相对开始时间的分钟偏移。
- 对真实 flag 的错误值域做通用拦截，例如 `+book --with <userId>` 或 `room add --rooms <会议室名>`；真实参数名不会进入 unknown alias 兜底。

这些情况不是“别名表修不好一个错误参数名”，而是需要查询、值转换、结构组装或业务校验。正确做法是保持保护边界，并由 Help/Schema/Skill、resolver 或命令实现承担下一步。

## 6. Skill 与当前实现的漂移

本轮确认两处需要单独修正文档，不能用别名表掩盖：

1. 日程产品参考多处写明 `event list --max-results` 会被解析但丢弃。当前 `calendar.go` 实现实际上会读取该隐藏参数并写入最终 `limit`，因此“会被丢弃”的描述已经过期。
2. 会议最佳实践写明 `room search` 只有 `start/end/group-id/available`，并断言 `--query` 一定 unknown。当前公开 Help 还包含 `room-name/limit/page`；隐藏 `query` 可作为 `room-name` 的命令级兼容，隐藏 `available` 虽存在，但当前接口语义已经直接查询可用会议室。该最佳实践与当前产品参考和命令实现不一致。

`calendar participant ...` 是 `calendar attendee ...` 的 Cobra 路径别名，能够执行，但主 Schema 路径仍是 `attendee`；Skill 应优先展示主路径，避免 Agent 把命令路径 alias 与参数 alias 混为一层。

## 7. 候选别名表改动与审核结论

相对于正式表：

| 项目 | 正式表 | calendar 候选 | 变化 |
|---|---:|---:|---:|
| concept | 31 | 42 | +11 |
| command override | 128 | 154 | +26 |
| validation fixture | 253 | 312 | +59 |
| 生成 calendar 命令 | 1 | 28 | +27 |
| 生成 calendar alias | 10 | 181 | +171 |
| 生成 calendar block | 3 | 210 | +207 |
| 生成 calendar ambiguous | 0 | 1 | +1 |

独立审核结论：

- 所有 alias 目标都是同提交真实 Cobra flag；
- alias 不做查询、单位转换、列表拆合或 JSON 构造；
- `eventId/calendarId/aclId/roomId`、姓名/ID、roomName/roomId/groupId/location、ISO/小时和 cursor/pageIndex 均保留角色边界；
- `to` 没有加入全局 `time_end`，避免破坏 `+free-slots` 的整数小时含义；
- 生成前后所有非 calendar `ParamAliasEntry` 完全一致，SHA-256 均为 `30258faeef43a75bf4e5516993620e673b539ae40737e4b6129ed3b55b842c2b`；
- 210 条 block 只接管原本 unknown 的危险拼写，不覆盖真实 flag。

候选规模较大，正式落地前必须补齐最终 payload 模板，不能只凭生成器和 fixture 通过就替换正式表。

## 8. 候选验证结果

候选在独立临时源码副本中作为正式输入验证，未覆盖当前工作区的 `internal/cli/param_concepts.json`。

| 验证 | 结果 |
|---|---|
| `jq empty` / JSON Schema 读取 | 通过 |
| `go generate ./internal/cli` | 通过，生成 308 个命令条目 |
| `internal/cli` | 通过 |
| `internal/pipeline` | 通过 |
| generated drift + Schema assembly determinism | 通过 |
| Schema catalog policy | 通过：27 个产品、1018 个工具 |
| 非 calendar 生成规则对比 | 0 条变化，哈希一致 |
| 代表性 alias/canonical 最终 dry-run payload | 2/2 通过 |
| 代表性 block/ambiguous 在 dispatch 前终止 | 3/3 通过 |
| `internal/app` 全量 | 未全绿：20 个新增活跃命令缺 complete-command E2E 模板 |

两组最终 payload 等价验证为：

1. `event create --reminder-minutes --tz` 与 `--remind-minutes --timezone` 得到完全相同的 `summary/startDateTime/endDateTime/reminders/timeZone`；
2. `event suggest --duration-minutes --tz` 与 `--duration --timezone` 得到完全相同的 `attendeeUserIds/durationMinutes/timeZone`。

三组保护验证为：

- `+free-slots --start <ISO>`：以 `blocked_flag` 停止，避免 ISO 时间被解释成整数小时；
- `+attendee-list --id`：以 `ambiguous_flag` 停止，避免在 eventId 和 calendarId 中猜测；
- `attachment add --file-id`：以 `blocked_flag` 停止，避免把缺少文件名的值当作结构化 `files`。

需要补 complete-command 模板的 20 个命令为：

```text
calendar +agenda
calendar +attendee-list
calendar +book
calendar +book-search
calendar +cancel-event
calendar +conflicts
calendar +free
calendar +free-slots
calendar +freebusy
calendar +invite
calendar +my-free
calendar +reschedule
calendar +room-groups
calendar +room-search
calendar +suggest-time
calendar event create
calendar event respond
calendar event suggest
calendar event update
calendar room search
```

其中共有 39 个活跃 alias fixture 需要进入最终参数组装等价测试。候选当前应作为分析草稿和下一轮编码输入，不应直接替换正式表。

## 9. 第一轮实现边界

第一轮可优先落地 event/calendar 标识符、同格式时间、分页、duration/timezone 和搜索词等同值映射；同步加入 `id` 双角色、姓名/ID、会议室角色、时间单位、分页方式和结构化输入保护。补齐上述 20 个命令的最终 payload 模板后，再运行完整 app、policy 和跨产品回归。

本轮流程可直接复用于其他产品：固定提交并重建 → 合并运行时 Schema、Help、reviewed exclusions、公开 shortcut 与 Skill → 按实体/角色/值域/基数/单位审计 → 聚合问题 → 生成正式表完整副本 → 独立生成、行为等价、保护性拦截和非目标产品零影响验证。
