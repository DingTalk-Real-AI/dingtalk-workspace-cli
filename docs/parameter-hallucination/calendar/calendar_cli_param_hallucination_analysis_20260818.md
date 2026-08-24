# DWS 日程 CLI 参数幻觉与参数契约分析（最新版重审）

分析日期：2026-08-18
分析基线：`origin/main@7186a69b7821f1db0760f6d1bf606571939d95e1`
分析对象：DWS `calendar` 产品的原生命令、内置 Shortcut、运行时组装 Schema、Cobra Help、日程 Skill、命令实现与正式中央参数别名表。

## 1. 结论摘要

最新版日程产品的 Help 与运行时 Schema 基础契约是对齐的，但参数体系仍有明显的“同一实体多种命名、同名不同角色、相似名称不同值域”问题。尤其是 2026-08-17 新增的一组日程 Shortcut，使旧版分析不再完整：公开可执行命令从 44 个增至 52 个，内置 Shortcut 从 20 个增至 27 个；新增 `+create`、`+get`、`+room-find`、`+rsvp`、`+search-event`、`+suggestion`、`+update` 七个命令。

本轮从最新 `main` 重新构建独立二进制，盘点 52 个公开可执行命令，其中 25 个原生命令、27 个 Shortcut。运行时 Schema 发布 49 个工具；`calendar acl add`、`calendar acl delete`、`calendar book update` 是 3 个有明确理由的 reviewed exclusions，仍作为真实公开命令纳入分析。52 个命令共有 188 次业务参数出现、49 个不同公开参数名，45 个命令带业务参数。逐命令对比 Help 与完整 Schema leaf，公开参数名集合差异为 0。

问题不在 Schema 生成错误，而在跨命令的业务命名和角色差异：eventId 在 `--event/--id` 间切换，calendarId 多数叫 `--calendar-id`、日历本自身却用 `--id`；人员输入同时存在姓名、userId、openDingTalkId 和增加/移除两个方向；会议室同时存在 roomId、roomName、groupId 和纯文本 location；时间参数又混有事件时间、查询范围、整数小时、相对天数和分钟偏移。Agent 很容易把在一个命令学到的参数名迁移到另一个命令，或把看起来相似但值域不同的值原样传入。

当前正式中央表对日程生成的规则仍只有 `calendar event list` 一条命令，生成 10 对 alias 和 3 条保护；原生命令虽有较多隐藏兼容 flag，但新 Shortcut 基本没有中央语义兜底。日程 Skill 的可见 Shortcut 表只列出 21/27 个命令，漏掉 `+create`、`+get`、`+rsvp`、`+search-event`、`+suggestion`、`+update` 六个；会议室参考还在声明 `room search --query` 必然 unknown，与当前公开的 `--room-name` 以及 Shortcut `+room-find` 能力不一致。

本轮候选表基于最新正式表重新生成，而不是复用旧候选文件。独立审核后将仅服务一两个命令的单姓名、相对天数、提醒分钟和响应状态从 concept 下沉到精确 command override，最终只新增 7 个可稳定复用的日程 concept，扩展 7 个已有 concept，新增或修改 34 条日程 command override。候选覆盖 35 个日程命令，生成 291 对 alias、334 条 block 和 7 条 ambiguous。会议室复审确认 6 个真正消费 roomId 的命令全部接收列表，单个 roomId 是一元素列表，因此增加 6 条精确的 `room-id → rooms`，而 4 个原生命令已有的隐藏 `--room-ids/--roomIds` 仍由命令自身接管。`+book-search` 的 `--name` 在该精确命令中明确表达日历名称搜索词，因此由命令级 override 映射到 `--query`，不扩大到其他搜索命令。数量较大主要来自经过命令范围限制的时间、分页、标识符 concept 展开以及其 `excludes` 保护，并非 625 条手写独立判断。生成前后非日程规则逐字节一致。

候选已通过 JSON/生成器、`internal/cli`、`internal/pipeline`、全量 fixture、全量 guard、generated drift、Schema assembly determinism 和 Schema policy；新增七个 Shortcut 各抽取一个代表 alias，alias 与 canonical 到达相同最终 transport payload，7/7 通过。收敛后的四类命令级映射以及原生 `room-ids` 兼容又完成 7 组成对行为比较。会议室复审额外验证 6 个 `room-id → rooms` 和 4 个原生 `room-ids`，10/10 到达与 canonical 完全相同的最终 transport payload；`+book-search --name` 与 `--query` 的最终查询参数一致。完整 `internal/app` 唯一未通过的候选相关门禁是 complete-command E2E 覆盖：30 个新增活跃日程命令的 76 条 active fixture 尚未加入正式测试模板。因此候选结论是“业务语义与生成链路已审核，可作为编码输入；正式替换前必须补齐 payload 模板与代表性最终 payload 测试”，不是可以直接合并的正式表。

## 2. 分析基线与覆盖范围

| 分析项 | 数量或结果 |
|---|---:|
| 最新 `main` commit | `7186a69b7821f1db0760f6d1bf606571939d95e1` |
| 公开可执行 calendar 命令 | 52 |
| 原生命令 | 25 |
| 内置 Shortcut | 27 |
| 运行时 Schema 工具 | 49 |
| reviewed Schema exclusions | 3 |
| Help/Schema 参数出现次数 | 188 / 181 |
| 不同公开业务参数名 | 49 |
| 带业务参数的公开命令 | 45 |
| Help/Schema 可见参数集合差异 | 0 |
| Skill 已列 / 实际 Shortcut | 21 / 27 |
| 正式表当前生成 calendar 命令 | 1 |

运行时 Schema `source` 为 `runtime-assembled`，Catalog hash 为 `sha256:b79227d926ebff2882da91e114a3fa20380f488ea6dc86aaf6384cfdc5d14d23`，surface hash 为 `sha256:51b201c1115cf4829d276759267328aeaaf5b46d90579d9413dc244a1f8b15fc`；全仓库共组装 1149 个工具。

本报告没有把历史 badcase、`dws-eval`、历史工作簿、旧固定 Catalog、用户自定义 Shortcut 或插件作为产品事实来源。旧报告仅用于指出需要复查的方向，所有结论都回到本次同一提交的 Help、运行时 Schema、Skill 和实现重新确认。

## 3. 主要参数问题

### 3.1 eventId、calendarId 与宽泛 `id` 的命名和角色混杂

19 个命令用 `--event` 或 `--id` 表达 eventId；20 个命令公开 `--calendar-id`，而 `calendar book get/update --id` 的 `id` 表达 calendarId。`acl delete --acl-id` 又是第三种 ID。新 `+get/+rsvp/+update` 同时包含 `--event` 和 `--calendar-id`，此时裸 `--id` 有两个合理目标，不能静默选择。

候选用 `calendar_event_id` 与 `calendar_book_id` 分开治理，只在目标角色唯一时把 `event-id/calendar-event-id → event`、`calendar/calendar-book-id → calendar-id`；`+attendee-list/+get/+rsvp/+update --id` 标记为 ambiguous。`+cancel-event/+invite/+reschedule` 只有 eventId 一个标识符角色，允许精确的 `id → event`。原生命令已经接受的隐藏 `event-id/eventId/calendarId/calendar` 保持原生，不重复建立中央 alias。

### 3.2 姓名、userId、openDingTalkId、单值/列表及增删方向不能合并

`+book/+invite/+suggest-time --with` 和 `+free --who` 接收姓名并由 Shortcut 解析；`+create/attendee add/delete --attendees`、`+freebusy/+suggestion/busy search --users` 接收 userId 列表；`event create --open-dingtalk-ids` 是另一种标识符；`acl add --user` 是单个用户。新 `+update` 还把同一 userId 列表拆成 `--add-attendees` 与 `--remove-attendees` 两个相反角色。

候选分别保留姓名列表、单姓名和 userId 列表的值域。三个姓名列表 Shortcut 复用 `calendar_person_name_list`；仅 `+free` 使用的单姓名变体通过精确 override 处理，不为一个命令建立 concept。`attendee-names → with`、`name/person → who`、`user-ids → users`、`+create user-ids → attendees` 可以原样传值；姓名与 ID、单值与列表、userId 与 openDingTalkId 互转全部阻止。`+update add-user-ids/remove-user-ids` 可分别映射到方向明确的目标，但宽泛的 `attendees/users/user-ids` 被标为 ambiguous，避免猜测增加还是移除。

### 3.3 roomId、roomName、groupId 与 location 是四种不同角色

`--rooms` 是 roomId 列表，用于创建、查询忙闲或预订；`--room-name` 是会议室展示名搜索词；`--group-id` 是会议室分组；`--location` 只是日程地点文本，不会完成会议室预订。最新版 `+room-find` 同时提供时间、roomName、groupId 与分页，更容易让模型把已有 roomId 误当筛选条件。

6 个真正消费 roomId 的命令——`event create`、`room add`、`room delete`、`busy search`、`+create`、`+freebusy`——底层都接收 roomId 列表；单个 roomId 只是长度为 1 的列表。因此 `calendar_room_ids` 在这 6 个精确命令上允许 `room-id/room-ids → rooms`：其中 4 个原生命令已有真实隐藏 `--room-ids/--roomIds`，生成器对真实 flag 让路，只新增缺少的 `room-id → rooms`；两个 Shortcut 同时由中央 concept 补齐单复数拼写。在精确搜索命令上只允许 `query/name → room-name`、`room-group-id/group → group-id`。宽泛 `room` 仍被 block，因为它可能是会议室名称，不能不经搜索就作为 roomId；roomName、groupId、location 与 roomId 的值域边界继续保留。

### 3.4 事件时间、查询范围、整数小时与相对日期不能共用一套别名

多数 `start/end` 是 ISO-8601 时间，但语义分为“事件本身的开始/结束”和“查询窗口上下界”。`+free-slots --from/--to` 则是整数小时，`+conflicts/+free-slots --in-days` 是相对今天的天数。

候选只在查询范围命令使用通用 `time_start/time_end`；`+book/+create/+reschedule/+update` 等写命令只接受命令级 `from/begin/start-time → start` 与 `to/end-time → end`，不把 `since/time-min/start-date` 等范围或日期词套到事件时间上。`+free-slots` 只允许 `start-hour/end-hour → from/to`，并阻止 ISO 时间参数名。这是本次相对旧候选最重要的收敛之一。

### 3.5 cursor 与零基 pageIndex 是两种分页模型

`event list/instances/+agenda/+search-event` 使用 `cursor + limit`；`room search/list-groups/+room-find/+room-groups` 使用零基 `page + limit`。两种模型都出现“下一页”和“页码”的自然语言，但没有可逆转换关系。

候选复用 `pagination_size/page_cursor` 处理同值映射；页码命令只在精确命令上允许 `page-index → page`。cursor、page-token 与 pageIndex/offset 的交叉输入被 block，不把页码伪装成游标。

### 3.6 标题、普通描述、富文本描述和搜索词容易互相借名

事件标题在 `+book/+create/+update/event create/update` 中叫 `title`，日历本更新叫 `summary`；普通描述叫 `desc`，富文本另有 `rich-text-desc`；日历本搜索和事件搜索都叫 `query`，但事件搜索实际在当前页的标题、描述和地点中做全文匹配。

候选使用 `calendar_event_title` 处理 `summary/subject → title`，复用 `plain_description` 处理 `description → desc`，使用 `search_query` 处理 `keyword/q → query`。`+book-search` 的查询对象就是日历名称，因此在该精确命令上补充 `name → query`；这条映射不进入通用 concept，也不影响事件全文搜索。`rich-text-desc` 不映射到普通描述；`+search-event --title/--subject` 不映射到全文 `query`，因为这会把“只按标题”扩大成三字段搜索。

### 3.7 响应状态、忙闲状态和可用性不是同一种“状态”

`event respond --status` 的值域是 `needsAction/accepted/declined/tentative`；新 `+rsvp --status` 为了用户表达更自然，值域是 `needs-action/accept/decline/tentative`，命令内部再转换。`--free-busy` 是 `busy/free`，`+room-find --available` 是布尔筛选。

候选在 `event respond` 与 `+rsvp` 两个精确命令中分别处理参数名 `response-status/response → status`，不建立跨命令 concept，也不会翻译枚举值；`free-busy/state/done/availability` 在响应命令上被保护。跨命令把 `accepted` 改成 `accept` 或反向转换超出别名层能力，必须由命令自身或未来值归一模块处理。

### 3.8 duration、timezone、day offset 与 reminder 只能做同单位映射

`duration` 是分钟数，`timezone` 是 IANA 时区，`in-days` 是整数日偏移，`remind-minutes` 是相对日程开始时间的分钟列表。这些参数名常出现 `duration-minutes/tz/day-offset/reminder-minutes` 等合理变体。

候选允许同单位、同格式、值原样传递的映射；禁止分钟与小时、时区与 UTC offset、相对天数与 ISO 日期、绝对提醒时间与分钟偏移互转。

### 3.9 结构化输入和成组约束不能靠改名补全

`attachment add --files` 需要 `<fileId>:<name>`；循环日程的 `recurrence-*` 是成组约束；`+update` 的 start/end 必须一起出现，且同一 userId 不能同时增删。一个看似接近的参数名无法补齐缺失值、构造结构或满足组合约束。

候选对裸 `file-id`、不支持的 reminder/room 变更等使用 block，对 `+update` 的宽泛参会人输入使用 ambiguous；不提供“单参数生成完整 recurrence”或“自动补另一半时间”的 alias。

### 3.10 原生命令兼容 flag 与中央治理并存，必须明确边界

日程原生命令已经注册了多组隐藏兼容 flag，例如 `event get --event-id`、`event list --time-min`、`room search --query/page-index`、`book search --keyword`。这些是 Cobra 原生可接受参数，不应该被误写成中央别名能力。

候选保留原生行为，只补中央表尚未覆盖的 Shortcut 与危险边界。独立审核还移除了旧候选中重复的 `calendar event update freebusy → free-busy` 中央 override，因为当前命令自身已经发布隐藏 `--freebusy` 兼容入口。原生兼容与中央 alias 的测试、统计和文档应分开。

### 3.11 Skill 漏列六个新 Shortcut，会议室参考与当前命令漂移

当前 Skill 可见 Shortcut 表只覆盖 21/27，漏掉 `+create/+get/+rsvp/+search-event/+suggestion/+update`。此外 `references/03-meeting.md` 仍写 `room search` 合法参数“仅 start/end/group-id/available”，并断言 `--query` 一定 unknown；而当前公开 Help 已有 `room-name/limit/page`，命令自身也保留隐藏 `query → room-name` 兼容，且公开 Shortcut `+room-find` 已承担严格的可用会议室查询。

这不是别名表能够修复的问题，应修改 Skill 与会议室参考。候选表不会伪造或隐藏这类文档漂移。

## 4. 当前别名表可实施的方案

候选文件保存于同目录 `param_concepts.json`，是最新正式表的完整副本加 Calendar 改动，未修改 `internal/cli/param_concepts.json`。主要动作如下：

1. 新增 7 个稳定复用的日程 concept：`calendar_event_id`、`calendar_book_id`、`calendar_event_title`、`calendar_person_name_list`、`calendar_room_ids`、`calendar_duration_minutes`、`calendar_timezone`；单姓名、相对天数、提醒分钟和响应状态改为精确命令 override；
2. 仅向 7 个已有 concept 追加经过审核的日程命令：`search_query`、`pagination_size`、`page_cursor`、`plain_description`、`time_start`、`time_end`、`user_ids`；
3. 使用 34 条新增或修改的精确 override 处理 `id`、方向性参会人、会议室角色、事件时间、页码模型、局部同义词和结构化输入；
4. 目标唯一且值可原样传递时自动 alias；多目标用 ambiguous；不同值域、单位或角色用 block；
5. 保持命令自身已有的隐藏兼容参数为 native，不重复接管；
6. 不创建新的真实 flag，不修改值，不查询 ID，不拆分/合并列表，不绕过必填或确认。

## 5. 当前能力支持不了或不应该处理的事项

- 根据日程标题、日历本名称或自然语言定位唯一 eventId/calendarId；需要先查询并处理多候选。
- 姓名与 userId/openDingTalkId 互转；需要人员解析且可能同名。
- 根据会议室名、楼层、园区或 groupId 得到 roomId；必须执行会议室查询。
- ISO-8601、Unix 毫秒、整数小时、相对天数和分钟之间做单位或格式转换。
- cursor 与零基 pageIndex 互转，或凭空生成下一页游标。
- 把裸 fileId 包装成 `<fileId>:<name>`，自动补文件名。
- 从一个自然语言 recurrence 参数生成完整 pattern/range 参数组。
- 把绝对提醒时间换算成相对开始时间的分钟偏移。
- 在 `event respond` 与 `+rsvp` 之间翻译不同枚举拼写。
- 判断真实 canonical flag 中的值是否属于错误值域，例如 `--attendees` 实际填姓名、`--rooms` 实际填会议室展示名；unknown flag 兜底不会接管已经合法的参数名。

这些事项不阻止第一轮“参数名治理”，但必须保持保护边界，不能为了提高覆盖率强行做 alias。

## 6. 候选草稿改动与独立审核

| 项目 | 最新正式表 | 日程候选 | 变化 |
|---|---:|---:|---:|
| concept | 45 | 52 | +7 |
| command override | 208 | 242 | +34 |
| validation fixture | 425 | 531 | +106 |
| 生成 calendar 命令 | 1 | 35 | +34 |
| 生成 calendar alias | 10 | 291 | +281 |
| 生成 calendar block | 3 | 334 | +331 |
| 生成 calendar ambiguous | 0 | 7 | +7 |

独立审核结论：

- 每个 alias 目标都是同一提交该精确命令的真实 canonical flag；
- 自动映射均保持业务实体、角色、值域和单位一致，参数值原样传递；唯一的基数收敛是经过全命令盘点的单个 `room-id`，由命令原有列表解析器自然形成一元素 `roomIds`；
- `+get/+rsvp/+update` 的裸 `id`、`+update` 的宽泛参会人列表没有被强行选择；
- `+book/+create/+reschedule/+update` 没有复用包含 `since/time-min` 的范围时间 concept，只保留事件时间的命令级别名；
- `event respond` 与 `+rsvp` 的不同枚举使用各自命令级参数名映射，不建立跨命令 concept，也不进行值转换；
- 单姓名、相对天数、提醒分钟和响应状态四类局部规则已从 concept 下沉为 command override，生成的 alias/block/ambiguous 总量保持不变；
- 已移除对原生隐藏 `event update --freebusy` 的重复中央治理；`busy search/event create/room add/room delete --room-ids` 继续保持 native。四个命令进入 `calendar_room_ids` 的目的只是补 `room-id` 和统一保护边界，生成器不会为真实隐藏 flag 重复生成 alias；
- 291/334 的数量膨胀来自 35 个精确命令上 concept members/excludes 的确定性展开；抽样复核了新增七个 Shortcut 以及 alias/block 数量最高的 `+agenda/+create/+room-find/+search-event/+update`；`+book-search name → query` 只替换该命令原先对 `name` 的保护，不扩大作用域；
- 所有非 Calendar 生成规则逐字节一致，SHA-256 前后均为 `ab3dde6500fb2707b8b83e9babc8b92b595cc07ad44e4c25fee4b96fc2416503`。

审核状态：规则合理，但正式落地前仍需补测试；Skill 漏项和会议室参考漂移需另行修改；值转换与查询类场景暂不支持。

## 7. 候选验证结果

候选在独立临时副本中替换正式输入进行验证，没有覆盖当前工作区的正式 `internal/cli/param_concepts.json`。

| 验证 | 结果 |
|---|---|
| `jq empty` / `go generate ./internal/cli` | 通过，生成 569 个命令条目 |
| `internal/cli` | 通过 |
| `internal/pipeline` | 通过 |
| 全量参数 fixture 经嵌入表与 PreParse | 通过 |
| 全量 reviewed guard 到运行时契约 | 通过 |
| 代表性 guard 在 dispatch 前终止 | 通过 |
| 新增七个 Shortcut alias/canonical 最终 transport payload | 7/7 通过；另验证 `+book-search name → query` |
| 收敛规则与原生 room fallback 成对最终行为 | 7/7 输出逐字节一致 |
| `room-id → rooms` 最终 transport payload | 6/6 与 canonical 一致 |
| 四个原生命令隐藏 `room-ids` 最终 transport payload | 4/4 与 canonical 一致 |
| generated drift + Schema assembly determinism | 通过 |
| Schema catalog policy | 通过：27 个产品、1149 个工具 |
| 非 Calendar 生成规则 | 0 条变化，哈希一致 |
| 完整 `internal/app` | 未全绿：complete-command E2E 模板缺口 |

七个最终 payload 代表用例为：

- `+create summary → title`；
- `+get event-id → event`；
- `+room-find query → room-name`；
- `+rsvp event-id → event`；
- `+search-event keyword → query`；
- `+suggestion user-ids → users`；
- `+update event-id → event`。

完整 App 门禁要求为 30 个日程命令补 complete-command E2E 模板，共覆盖 76 条新增 active fixture；现有 `calendar event list` 模板已存在。缺模板命令为：

```text
calendar +agenda
calendar +attendee-list
calendar +book
calendar +book-search
calendar +cancel-event
calendar +conflicts
calendar +create
calendar +free
calendar +free-slots
calendar +freebusy
calendar +get
calendar +invite
calendar +my-free
calendar +reschedule
calendar +room-find
calendar +room-groups
calendar +room-search
calendar +rsvp
calendar +search-event
calendar +suggest-time
calendar +suggestion
calendar +update
calendar busy search
calendar event create
calendar event respond
calendar event suggest
calendar event update
calendar room add
calendar room delete
calendar room search
```

本轮补齐了此前清单遗漏的三个候选完整 argv 模板，并通过注入 Caller 验证其 `room-id → rooms` 最终 payload 等价：

```text
dws calendar busy search --rooms room-1 --start "2026-08-20T10:00:00+08:00" --end "2026-08-20T11:00:00+08:00"
dws calendar room add --event event-1 --rooms room-1
dws calendar room delete --event event-1 --rooms room-1
```

由于正式 `internal/cli/param_concepts.json` 尚未启用 Calendar 候选规则，当前不能只把这三条加入正式 `paramAliasCompleteCommands`：正式全量测试会把没有活跃正式 fixture 的模板判为多余。正式替换候选表时，应把 30 个模板与 76 条 active fixture 一次性接入；写命令必须使用注入 Caller/Runner 或 dry-run，不得发起真实业务调用。

## 8. 第一轮改造建议

1. 以本候选为语义基础补齐 30 个 complete-command 模板和代表性 payload 用例；其中本轮已补齐并验证此前遗漏的 3 个会议室命令模板；
2. 再次跑 `internal/cli`、`internal/pipeline`、完整 `internal/app`、generated drift 和 Schema policy；
3. 修改日程 Skill 可见 Shortcut 表，补齐六个新命令；同步修正会议室参考的合法参数和 `query/room-name` 说明；
4. 全绿后再把候选替换到正式 `internal/cli/param_concepts.json`，重新生成 `param_aliases_generated.go`；
5. 枚举值转换、ID 查询和真实 flag 值域校验作为后续能力，不与本轮参数名治理混合。

## 9. 可复用的产品分析流程

对其他产品继续使用同一流程：冻结最新提交并重建二进制；合并运行时 Schema、官方命令树和 reviewed exclusions；逐命令对账 Help/Schema/Skill；按实体、角色、值域、单复数和单位聚合问题；只为可原样传值的同义参数建立 alias；对多目标使用 ambiguous，对不同值域使用 block；候选基于最新正式表生成并保持非目标产品不变；最后通过生成、PreParse、payload、guard、Schema 与完整 App 门禁验证。
