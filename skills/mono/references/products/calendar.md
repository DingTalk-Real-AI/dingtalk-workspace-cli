# 日历 (calendar) 命令参考

## CLI 命令树与黄金路径

- **二级子命令（必选其一）**：`event`（日程）、`attendee`（参会人）、`room`（会议室）、`busy`（闲忙）、`attachment`（日程附件）、`book`（我能看哪些日历本）、`acl`（我的日历共享给了谁）。`dws calendar` 后**必须**紧跟上述之一；**禁止**只执行 `dws calendar`（无子命令）。
- **个人日程 / 给自己留时间块 / 专注时段**：统一走 **`dws calendar event create`**。当前**没有**单独的 `personal schedule create` / `calendar create` 命令。
- **查日程列表**：`dws calendar event list --start "<ISO-8601>" --end "<ISO-8601>" --format json`，或优先使用脚本 `python scripts/calendar_today_agenda.py [today|tomorrow|week]`（见文末「自动化脚本」）。
- **查用户日历本列表**：`dws calendar book list`（返回主日历 `id == "primary"` 等）。**重要**可以查询他人共享给自己的日历本，根据日历本id可以进一步查询对方的日程信息。
- **CLI 不存在**独立的 `dws calendar list`；若误跑无子命令的 `dws calendar`，会打印整段 Usage，**切勿**将该段 help 当作工具结果再次塞进对话（会急剧增加 token 与首字延迟）。
- **必须**遵循指令说明进行调用。**绝对禁止**使用虚构指令，使用虚构参数。

## 反模式（禁止）
1. **禁止**执行 `dws calendar` 且不带二级子命令（会刷出大量帮助文本）。合法二级子命令：`event` / `attendee` / `room` / `busy` / `attachment` / `book` / `acl`。
2. **禁止**使用不存在的子命令试探（如臆造 `dws calendar list`）；需要日程列表时一律使用 **`dws calendar event list`**（带 `--start` / `--end`，见下文「查询日程列表」示例）；需要日历本列表时使用 **`dws calendar book list`**。
3. **禁止**将完整 `--help`/Usage 输出作为「观察」重复提交给模型；若误触，应直接改用本节黄金路径中的合法命令并重试。
4. **禁止**继续使用已弃用的 `--max-results` 参数 —— 它们仍可被解析但**会被丢弃**，不再透传到 MCP，模型生成命令时也不要再带。
5. **禁止**为已有日程重新创建日程来预订会议室。若日程已存在（同一会话中刚创建、或用户明确指向某日程），必须使用 `room add --event <已有EVENT_ID> --rooms <ROOM_ID>` 追加会议室，**绝不能**再调一次 `event create --rooms`（会创建重复日程）。
6. **禁止**用 `--location` 替代会议室预订。`--location` 是纯文本地点备注字段，填入会议室名称**不会**完成任何预订或占用。预订会议室必须通过 `room add --rooms <roomId>` 或 `event create --rooms <roomId>`，roomId 来自 `room search` 返回；`--location` 与 `--rooms` 是两个独立字段，用途完全不同。
7. **禁止**只传 `--recurrence-*` 部分 flag **并不是彼此独立的参数**：只传其中一项（比如只改 `--recurrence-count`、只设 `--recurrence-type`）会让服务端收到不完整的 recurrence 结构，CLI 现已前置校验并直接拒绝这类调用。**修改已有周期日程的任何一个循环字段时，都必须重新提供完整的 pattern+range 字段集合**——必要时先 `event get` 读取现有 `recurrence`，再在命令中整体重传。
8. **禁止**用一条 指令 实现串行调用。比如当用户要求一次性安排多场不同的日程（例如「上午 10 点开项目评审、下午 2 点开复盘会、晚上 7 点聚餐」）时，必须**拆解成 N 条独立的 `event create`，依次串行执行**；每条命令自己写完整的 `--title` / `--start` / `--end`，绝不能把多个标题或多段时间塞进同一行。


## 核心概念
日历（calendar）：日程的容器。每个用户有一个主日历（我的日历，id: primary），还可以订阅公共/团队日历，以及他人共享的日历。
日程（event）：日历中的单个日程，包含起止时间、地点、标题、参会人等属性。支持单次日程和重复日程(有recurrence rule的日程，又称SeriesMaster)，遵循RFC5545 iCalendar国际标准。
日程实例（event instance）：日程的具体时间实例，可以通过event list指令查询时间段内的所有实例。1个普通日程和对应1个Instance，而1个重复性日程(SeriesMaster)对应N个Instance（同属一个日程序列）。
  - 同一个日程序列具有相同的iCalUid，并且重复性日程，其eventId和iCalUid的值相同。因此可以通过重复性日程实例的iCalUid得到重复性日程(SeriesMaster)的eventId
重复规则（recurrence rule）：定义重复性日程的重复规则。
参会人（attendee）：日程的参与者。常用通讯录工具查询userId，dws contact user search --keyword "姓名"。
响应状态（response）：参会人对日程的回应，包括：未响应、接受、待定、拒绝。
忙闲时间（busy）：查询用户在指定时间段的忙闲状态，查询会议室在指定时间段的预定状态，用于会议时间协调。
会议室（room）：room是 会议室 ，room可视为日程的资源类参会人，需要加入日程完成预订。注意和location区分，location只是地点，和room不同。
访问控制 (acl)：用户可通过设置acl将自己的日历访问权限授予给他人（即：共享日历），授予后，他人就可以查看日历下的日程信息。通过 `dws calendar acl list` 可查看当前要用户已经授权出去的权限。若 privilege >= reader，可查询此日历下的日程数据。若 privilege >= writer ，可操作（创建/修改/删除/响应等）此日历下的日程数据。
共享日历：将自己日历的访问权限授予给他人，用于协作。通过 `dws calendar book list` 查询当前用户日历列表，其中type == `shared` 即他人共享给当前用户的日历。
订阅日历：创建一个公共日历供他人订阅，他人订阅后即可查询日历下的日程数据。通过 `dws calendar book list` 查询当前用户日历列表，其中type == `subscribed` 即当前用户已订阅的日历。注意：订阅日历下的日程无参会人概念，无法执行 attendee 相关指令，也无法添加会议室。

## 命令概览

### event 相关三级子命令
```
# 针对单个日程： 创建 ｜ 修改 ｜ 单查询 ｜ 删除 ｜ 响应日程(接受、暂定、拒绝)
dws calendar event [create|update|get|delete|respond] [flags]
# 按时间范围批量查询
dws calendar event list [flags]
# 对于非明确时间或一段时间范围的约会场景，可基于所有参会人的忙闲状态，推荐多个可用的时间块方案
dws calendar event suggest [flags]
```

### attendee 相关三级子命令
```
# 日程中参会人操作：添加 ｜ 删除 ｜ 查询
dws calendar attendee [add|delete|list] [flags]
```

### room 相关三级子命令
```
# 查询分组
dws calendar room list-groups [flags]
# 会议室搜索
dws calendar room search [flags]
# 预定会议室
dws calendar room add [flags]
# 释放会议室
dws calendar room delete [flags]
```
> room是会议室，用于线下开会场景。

### busy 相关三级子命令
```
# 按用户 / 会议室 + 时间窗查闲忙状态（--users 与 --rooms 至少其一），会议室的忙闲等同于预定记录
dws calendar busy search [flags]
```

### attachment 相关三级子命令
```
# 把已上传到钉盘的文件挂到日程上（不负责上传，只负责挂载）
dws calendar attachment add [flags]
```

### book 相关三级子命令（查"我能看哪些日历本"）
```
# 查询我拥有和可访问的所有日历本（含他人共享给我的）
dws calendar book list [flags]
# 查询指定日历本信息
dws calendar book get [flags]
# 按名称模糊搜索日历本
dws calendar book search [flags]
# 更新日历本信息（需 owner 权限）
dws calendar book update [flags]
```

### acl 相关三级子命令（查"我的日历共享给了谁"）
```
# 查询我的日历共享给了哪些人、各自什么权限
dws calendar acl list [flags]
# 把我的日历共享给某人
dws calendar acl add [flags]
# 取消我的日历对某人的共享
dws calendar acl delete [flags]
```
> **说明**: 可以通过 --help 进一步查看指令明细，也可以继续查看下一节 命令总览

## 命令总览

### 查询日程列表
```
Usage:
  dws calendar event list [flags]
Example:
  dws calendar event list --start "2026-03-10T14:00:00+08:00" --end "2026-03-10T18:00:00+08:00"
  dws calendar event list --calendar-id primary
Flags:
      --calendar-id string   日历 ID (默认 primary 主日历，仅在查询其他日历本时填写；通过 `book list` 获取)
      --end string           结束时间 ISO-8601 (例如 2026-03-10T18:00:00+08:00)
      --start string         开始时间 ISO-8601 (例如 2026-03-10T14:00:00+08:00)
```

> `--max-results` 已弃用：MCP 不再支持，CLI 仍接受但参数会被丢弃，结果固定最多返回 100 条。

**默认行为**：不传 `--start` / `--end` 时，默认返回今天的日程（00:00:00 ~ 23:59:59）。
**权限**：查询共享日历下的日程时，至少要有reader权限。

### 获取日程详情
```
Usage:
  dws calendar event get [flags]
Example:
  dws calendar event get --id <EVENT_ID>
  dws calendar event get --id <EVENT_ID> --calendar-id primary
Flags:
      --id string            日程 ID (必填)
      --calendar-id string   日历 ID (默认 primary 主日历)
```

### 创建日程
```
Usage:
  dws calendar event create [flags]
Example:
  dws calendar event create --title "Q1 复盘会" \
    --start "2026-03-10T14:00:00+08:00" --end "2026-03-10T15:00:00+08:00"
  dws calendar event create --title "周会" \
    --start "2026-03-10T14:00:00+08:00" --end "2026-03-10T15:00:00+08:00" \
    --attendees userId1,userId2
  dws calendar event create --title "项目评审" \
    --start "2026-03-10T14:00:00+08:00" --end "2026-03-10T15:00:00+08:00" \
    --rooms roomId1,roomId2     # 创建时直接预定会议室
  dws calendar event create --title "每日站会" \
    --start "2026-03-10T09:00:00+08:00" --end "2026-03-10T09:30:00+08:00" \
    --recurrence-type daily --recurrence-interval 1 --recurrence-range-type numbered --recurrence-count 10
Flags:
      --title string                    日程标题 (必填，最大2048字符)
      --start string                    开始时间 ISO-8601 (必填，例如 2026-03-10T14:00:00+08:00)
      --end string                      结束时间 ISO-8601 (必填，例如 2026-03-10T15:00:00+08:00)
      --timezone string                 时区 IANA 格式 (例如 Asia/Shanghai，默认 Asia/Shanghai)
      --desc string                     日程描述 (最大5000字符)
      --attendees string                参会人 userId 列表，逗号分隔 (最多500人) 日程组织人自动放入参会人列表，无需传入userId
      --open-dingtalk-ids string        openDingTalkId 列表，逗号分隔 (与 --attendees 至少传一个)
      --rooms string                    会议室 roomId 列表，逗号分隔 (创建时直接预定，roomId 必须来自 `room search` 返回，若是循环会议，必须设置recurrence-end-date，避免长期预订)
      # 以下 --recurrence-* 一旦使用任一 flag，必须同时提供完整的 pattern+range 字段（至少 --recurrence-type、--recurrence-interval(>0) 与 --recurrence-range-type）
      # 否则 CLI 会报 "recurrence 结构不完整" 并拒绝执行
      --recurrence-type string          循环类型: daily|weekly|absoluteMonthly|relativeMonthly|absoluteYearly
      --recurrence-interval int         循环间隔 (如 daily 时表示每N天)
      --recurrence-days-of-week string  周几: sunday,monday,...,saturday (weekly/relativeMonthly 时必填)
      --recurrence-day-of-month int     每月第几天 (absoluteMonthly/absoluteYearly 时必填)
      --recurrence-index string         每月第几周: first|second|third|fourth|last (relativeMonthly 时必填)
      --recurrence-first-day-of-week string  一周起始日，默认 sunday
      --recurrence-range-type string    循环范围: noEnd|endDate|numbered (与 --recurrence-type 必须成对出现)
      --recurrence-end-date string      循环结束时间 ISO-8601 (range-type=endDate 时必填)
      --recurrence-count int            循环次数 (range-type=numbered 时必填)
      --rich-text-desc string           html格式的富文本类型日程描述，用于复杂内容的展示
      --location string                 地点信息（纯文本备注，如‘3号楼A区’；**不等于**预订会议室）
      --free-busy string                此日程的忙碌状态，默认值为busy。busy - 在忙闲视图中，此日程时间段为忙碌; free - 此日程不占用忙闲
```

> **说明**：个人日程也走 `event create`。如果只是给自己安排时间，不传 `--attendees` / `--open-dingtalk-ids` 即可。

### 修改日程
```
Usage:
  dws calendar event update [flags]
Example:
  dws calendar event update --id <EVENT_ID> --title "新标题"
  dws calendar event update --id <EVENT_ID> --desc "新描述" --timezone Asia/Tokyo
  dws calendar event update --id <EVENT_ID> --recurrence-type daily --recurrence-interval 1 \
    --recurrence-range-type numbered --recurrence-count 5
Flags:
      --id string                       日程 ID (必填)
      --title string                    新标题
      --start string                    新开始时间 ISO-8601
      --end string                      新结束时间 ISO-8601
      --desc string                     新描述 (最大5000字符)
      --timezone string                 时区 IANA 格式 (例如 Asia/Shanghai)
      # 以下 --recurrence-* 在 修改周期日程的循环规则时必须**整体**传入：MCP 不合并部分字段，只改其中一项（例如只传 --recurrence-count）会把规则覆盖成不完整状态
      # 若只想微调已有规则，请先 `event get --id <ID>` 读取现有 recurrence，再在本命令重传完整的 pattern+range
      --recurrence-type string          循环类型: daily|weekly|absoluteMonthly|relativeMonthly|absoluteYearly
      --recurrence-interval int         循环间隔 (如 daily 时表示每N天)
      --recurrence-days-of-week string  周几: sunday,monday,...,saturday (weekly/relativeMonthly 时必填)
      --recurrence-day-of-month int     每月第几天 (absoluteMonthly/absoluteYearly 时必填)
      --recurrence-index string         每月第几周: first|second|third|fourth|last (relativeMonthly 时必填)
      --recurrence-first-day-of-week string  一周起始日，默认 sunday
      --recurrence-range-type string    循环范围: noEnd|endDate|numbered (与 --recurrence-type 必须成对出现)
      --recurrence-end-date string      循环结束时间 ISO-8601 (range-type=endDate 时必填)
      --recurrence-count int            循环次数 (range-type=numbered 时必填)
      --rich-text-desc string           html格式的富文本类型日程描述，用于复杂内容的展示
      --location string                 地点信息（纯文本备注，如‘3号楼A区’；**不等于**预订会议室）
      --free-busy string                修改此日程的忙碌状态，无需修改则不传。busy - 在忙闲视图中，此日程时间段为忙碌; free - 此日程不占用忙闲
```
> 支持修改标题、描述、时间、地点、忙碌状态等。如需修改会议室，请使用 dws calendar room [add|delete]；如需修改参会人，请使用 dws calendar attendee [add|delete]

### 删除日程

> **CAUTION:** 不可逆操作 — 所有参会人同步取消，必须先向用户确认。

```
Usage:
  dws calendar event delete [flags]
Example:
  dws calendar event delete --id <EVENT_ID> --yes
Flags:
      --id string   日程 ID (必填)
```

### 查看参会人
```
Usage:
  dws calendar attendee list [flags]
Example:
  dws calendar attendee list --event <EVENT_ID>
Flags:
      --event string   日程 ID (必填)
```

### 添加参会人
```
Usage:
  dws calendar attendee add [flags]
Example:
  dws calendar attendee add --event <EVENT_ID> --attendees <USER_ID_1>,<USER_ID_2>
  dws calendar attendee add --event <EVENT_ID> --attendees <USER_ID> --optional
Flags:
      --event string       日程 ID (必填)
      --attendees string   参会人 userId 列表，逗号分隔 (必填，最多500人)
      --optional           参会人可选 (默认必选参会人)
```

### 移除参会人

> **CAUTION:** 写操作 — 执行前须用户确认。

```
Usage:
  dws calendar attendee delete [flags]
Example:
  dws calendar attendee delete --event <EVENT_ID> --attendees <USER_ID> --yes
Flags:
      --event string       日程 ID (必填)
      --attendees string   参会人 userId 列表，逗号分隔 (必填)
```

### 搜索会议室
> 此指令可用于搜索当前用户可用的会议室。**注意**，大部分会议室**仅在工作时间**可用，如果检索时间不在工作时间可能查不到任何结果。
> 此指令搜索到的会议室结果中，有两个值需要注意：
 		- customApprovalProcess: true - 表示该会议室设置了自定义审批流程，只能通过客户端完成预订。
 		- supportRecurring: true - 表示该会议室支持循环预定；false - 表示不支持循环预定，直接加入到循环日程会失败。

```
Usage:
  dws calendar room search [flags]
Example:
  dws calendar room search --start "2026-03-10T14:00:00+08:00" --end "2026-03-10T15:00:00+08:00"
  dws calendar room search --start "2026-03-10T14:00:00+08:00" --end "2026-03-10T15:00:00+08:00" --group-id <GROUP_ID>
  dws calendar room search --room-name 永澄亭   # 注意：用户即使说「永澄亭会议室」，也应仅传「永澄亭」
  dws calendar room search   # 不传 --start/--end 时默认当前时间起 1 小时
Flags:
      --start string        开始时间 ISO-8601 (可选，不传则默认当前时间+1分钟缓冲）
      --end string          结束时间 ISO-8601 (可选，不传则默认当前时间+1 小时)
      --group-id string     会议室分组ID（可选，留空查根目录；超100条时需按分组查询）
      --room-name string    按会议室名称过滤（可选，服务端模糊匹配；传入前必须由调用方精简，只保留核心专名）
```

> **时间约束（API 限制）**：`start` 必须是未来的时间（服务端校验：start can not less current time）。
> - 若传入的 `--start` 早于当前时间，CLI 会自动修正为 `now + 1min`，调用方无需额外处理。
> - 若传入的 `--end` 早于当前时间，CLI 直接报错——无法检索已过去的时间段。
> - **最佳实践**：调用方在组装时间参数时应确保 start/end 都是未来时间；若不确定，可省略 `--start`/`--end` 让 CLI 使用默认值（当前时间起 1 小时）。

**名称过滤使用规范**：`--room-name` 适用于用户说「预定永澄亭」「约西湖厅」这类按名找会议室的场景。
- **服务端是模糊匹配，但匹配词越精简命中率越高**，关键疗法：**调用方必须在调用 CLI 前自行精简名称，CLI 不会再做任何删减**。
- 常见需要剔除的用户口语后缀（仅示例，实际场景由模型自行判断）：「会议室」「大会议室」「小会议室」「厅」「房」等。
- 示例对映：
  - 用户：「帮我订永澄亭会议室」 → `--room-name 永澄亭`
  - 用户：「西湖厅有空吗」 → `--room-name 西湖厅`（本身就是专名，不删即可）
  - 用户：「预定贡嘎山大会议室」 → `--room-name 贡嘎山`

**优先路径**：当用户给了会议室中文名时，**优先用 `room search --room-name <核心专名> --start <开始时间> --end <结束时间>`**代替「先走 `list-groups`再遍历」的旧流程。`--room-name` 可与 `--group-id` 同时使用，表示「在指定分组内按名称过滤」。若返回空列表，再降级使用 `list-groups` 定位分组再查。

**`roomId` 与用户说的话不是一回事**：用户说的「C6-4-06-N 贡嘎山」等是**展示名/编号文案**，**绝不能**直接填进 `room add --rooms`。`--rooms` 只接受上一步 `room search`（或同类接口）返回 JSON 里的 **`rooms[].roomId`**。形态上多为**小写十六进制串**（长度以接口为准，例如 `e6b7b65b8b30fb707afcf6c3b699f028003e6834fdd7fee7`）。含**中文、空格、连字符拼接的楼层编号**、或凭空调 UUID/纯数字「试格式」——一律视为非法，必须先搜房再取返回字段。

> 如果知道roomId，想查该会议室的预订记录，直接用dws calendar busy search 指令

--- 

### 预定会议室
```
Usage:
  dws calendar room add [flags]
Example:
  dws calendar room add --event <EVENT_ID> --rooms <ROOM_ID>
Flags:
      --event string   日程 ID (必填)
      --rooms string   会议室 ID 列表 (必填)
```
> room是会议室，用于线下开会场景。将room加入到日程完成预订
> 重复性日程，预订会议室时，必须设置 循环结束时间（recurrence-end-date），noEnd 或者 指定循环次数 都无法完成预定。


### 移除会议室

> **CAUTION:** 写操作 — 执行前须用户确认。

```
Usage:
  dws calendar room delete [flags]
Example:
  dws calendar room delete --event <EVENT_ID> --rooms <ROOM_ID> --yes
Flags:
      --event string   日程 ID (必填)
      --rooms string   会议室 ID 列表 (必填)
```

### 会议室分组列表
```
Usage:
  dws calendar room list-groups [flags]
Example:
  dws calendar room list-groups
  dws calendar room list-groups --limit 20 --page 0
Flags:
      --limit string       页大小 (可选，不填默认 100，超过 100 按 100 处理)
      --page string        分页起始位置 (可选，不填默认 0)
```

### 添加日程附件
```
Usage:
  dws calendar attachment add [flags]
Example:
  dws calendar attachment add --event <EVENT_ID> --files <FILE_ID>:report.pdf,<FILE_ID2>:slides.pptx
Flags:
      --event string   日程 ID (必填)
      --files string   附件列表，格式 <fileId>:<name>，多项逗号分隔 (必填)
```

> 上传文件得到 `fileId` 需配合钉盘相关流程；本命令只负责把已上传的文件挂载到日程上。

### 查询用户日历列表
```
Usage:
  dws calendar book list [flags]
Example:
  dws calendar book list
```
> 通过此接口可查询当前用户的日历列表，包含 主日历本、他人共享的日历、订阅的公共/团队日历。查的是"我能看哪些日历本"；注意区分：`acl list` 是查"我的日历共享**给了谁**"，方向相反。
> 共享日历本中有来自 xxx 的，且权限大于reader，那么通过 `event list --calendar-id <xxx的日历本id> `可查到xxx完整的日程安排
> 主日历 `id` 固定为 `primary`，绝大多数日程操作都默认走主日历，只有当用户明确要求查/写其他日历本时才需要带 `--calendar-id`。

### 查询指定日历本
```
Usage:
  dws calendar book get [flags]
Example:
  dws calendar book get --id primary
  dws calendar book get --id CALENDAR_ID
Flags:
      --id string   日历 ID (必填，主日历固定为 primary)
```

> **说明**：根据日历 id 查询指定日历的详细信息。用户主日历本 id 固定为 `primary`。

### 搜索日历本
```
Usage:
  dws calendar book search [flags]
Example:
  dws calendar book search --query "项目"
  dws calendar book search --query "团队周报"
Flags:
      --query string   按日历本名称模糊检索 (必填)
```

> **说明**：搜索当前用户拥有的日历本，支持按日历本名模糊搜索。获取全部日历请使用 `book list`。

### 更新日历本
```
Usage:
  dws calendar book update [flags]
Example:
  dws calendar book update --id CALENDAR_ID --summary "新日历名"
  dws calendar book update --id CALENDAR_ID --desc "日历描述"
Flags:
      --id string        日历 ID (必填)
      --summary string   日历标题
      --desc string      日历描述
```

> **说明**：更新日历信息，最低权限要求：privilege == "owner"。注意：用户主日历本 以及 他人共享的日历本 **不支持更新**。

### 查询我的日历共享给了谁
```
Usage:
  dws calendar acl list [flags]
Example:
  dws calendar acl list
```

> **说明**：查询"我的日历共享给了哪些人、各自什么权限"（即主日历的访问控制列表）。注意区分：`book list` 是查"我能看哪些日历本"，方向相反。

### 把我的日历共享给某人
```
Usage:
  dws calendar acl add [flags]
Example:
  dws calendar acl add --user USER_ID --privilege reader
  dws calendar acl add --user USER_ID --privilege writer --no-notification
Flags:
      --user string        授予权限的目标用户 ID (必填)
      --privilege string   授予的日历权限 (必填): free_busy_reader(查看忙闲)|title_reader(查看标题)|reader(查看详情)|writer(创建和编辑)
      --no-notification    不向被授权用户发送提醒 (默认发送)
```

> **说明**：把我的日历共享给指定用户，授予对方相应权限。`--privilege` 可选值：`free_busy_reader`（查看忙闲）、`title_reader`（查看标题）、`reader`（查看详情）、`writer`（创建和编辑）。

### 取消我的日历对某人的共享
```
Usage:
  dws calendar acl delete [flags]
Example:
  dws calendar acl delete --acl-id ACL_ID
Flags:
      --acl-id string   已授予权限的 ID (必填，可通过 acl list 查询)
```

> **说明**：取消我的日历对某人的共享（撤回已授予的访问权限）。aclId 可通过 `acl list` 获取。

### 查询用户 / 会议室闲忙状态
```
Usage:
  dws calendar busy search [flags]
Example:
  # 查用户闲忙
  dws calendar busy search --users <USER_ID_1>,<USER_ID_2> \
    --start "2026-03-10T14:00:00+08:00" --end "2026-03-10T18:00:00+08:00"
  # 查会议室闲忙
  dws calendar busy search --rooms <ROOM_ID_1>,<ROOM_ID_2> \
    --start "2026-03-10T14:00:00+08:00" --end "2026-03-10T18:00:00+08:00"
  # 同时查用户 + 会议室
  dws calendar busy search --users <USER_ID> --rooms <ROOM_ID> \
    --start "2026-03-10T14:00:00+08:00" --end "2026-03-10T18:00:00+08:00"
Flags:
      --end string     结束时间 ISO-8601 (必填)
      --start string   开始时间 ISO-8601 (必填)
      --users string   用户 ID 列表，逗号分隔 (与 --rooms 至少其一)
      --rooms string   会议室 ID 列表，逗号分隔 (与 --users 至少其一)
```

> **说明**：
> - `--users` 与 `--rooms` 必须至少指定其一，可以同时指定；CLI 会做前置校验，两者都为空会直接报错。
> - 查询会议室闲忙前，可先用 `dws calendar room search` 或 `dws calendar room list-groups` 拿到 roomId。
> - 返回结果中的忙碌时段仅包含粗粒度的时间信息，不包含日程内容细节（如标题、参会人、地点），以保护隐私。

### 建议日程时间
```
Usage:
  dws calendar event suggest [flags]
Example:
  dws calendar event suggest --users userId1,userId2 --duration 60
  dws calendar event suggest --start "2026-03-10T09:00:00+08:00" --end "2026-03-10T18:00:00+08:00" --users userId1
  dws calendar event suggest --users userId1 --duration 30 --timezone Asia/Tokyo
Flags:
      --start string      推荐时间范围开始 ISO-8601 (默认当前时间)
      --end string        推荐时间范围结束 ISO-8601 (默认次日18点)
      --timezone string   时区 IANA 格式 (默认 Asia/Shanghai)
      --users string      参会人 userId 列表，逗号分隔
      --duration string   日程持续时间，单位分钟 (默认30)

> 对于非明确时间或一段时间范围的约会场景，可基于所有参会人的忙闲状态，推荐多个可用的时间块方案，用于解决会议时间协调问题。
```

### 响应日程
```
Usage:
  dws calendar event respond [flags]
Example:
  dws calendar event respond --id <EVENT_ID> --status accepted
  dws calendar event respond --id <EVENT_ID> --status declined
  dws calendar event respond --id <EVENT_ID> --status tentative
Flags:
      --id string       日程 ID (必填)
      --status string   响应状态: needsAction(未操作)|accepted(接受)|declined(拒绝)|tentative(暂定) (必填)
```

> **说明**：作为日程参会人，设置自己的响应状态（接受、拒绝、暂定）。`--status` 可选值：`needsAction`（未操作，默认值）、`accepted`（接受）、`declined`（拒绝）、`tentative`（暂定）。

## 意图判断

用户说"日程/会议/约会/日历":
- 查看 → `event list`
- 详情 → `event get`
- 创建/约/给自己留时间块/个人日程 → `event create`（带参会人时加 `--attendees`，循环日程加 `--recurrence-*`）
- 修改/改时间/改描述 → `event update`（支持修改标题、时间、描述、时区、循环规则）
- 取消/删除 → `event delete`
- 推荐时间/什么时候有空/协调时间 → `event suggest`
- 接受/拒绝/暂定日程 → `event respond`

用户说"参会人/与会者":
- 查看 → `attendee list`
- 邀请/添加 → `attendee add --attendees <USER_ID>`（可选参会人加 `--optional`）
- 移除 → `attendee delete --attendees <USER_ID>`

用户说"会议室/订会议室":
- 哪个空闲 → `room search`
- 按名找会议室（如「永澄亭」「永澄亭会议室」「约西湖厅」）→ 先在模型层精简名称（剔除「会议室」等通用后缀），再用 `room search --room-name <核心专名>`
- 预订
  - 给已有日程订会议室 → `room add --event <已有EVENT_ID> --rooms <ROOM_ID>`
  - 创建新日程并订会议室 → `event create --rooms`（仅当日程尚不存在时）
- 取消预定 → `room delete`
- 分组 → `room list-groups`，取 groupId 后 `room search --group-id`（可再叠加 `--room-name` 在分组内过滤）

用户说"有空吗/忙不忙/闲忙":
- 查询用户闲忙 → `busy search --users <USER_ID>`
- 查询会议室闲忙 → `busy search --rooms <ROOM_ID>`
- 用户 + 会议室一起查 → `busy search --users <USER_ID> --rooms <ROOM_ID>`

用户说"日程附件/给会议加文件/上传日程材料":
- 添加 → `attachment add`（先用钉盘上传得 fileId，再 `attachment add --files <fileId>:<name>`）

用户说"我有几个日历/查所有日历/别人共享给我的日历/他人共享给我的日历本":
- 列表 → `book list`（返回用户拥有和订阅的所有日历本，包括他人共享给自己的；主日历 id 固定为 `primary`）
- 查指定日历本 → `book get --id <CALENDAR_ID>`
- 按名称搜索日历本 → `book search --query "关键词"`
- 修改日历本名称/描述 → `book update --id <CALENDAR_ID> --summary "新名"`

用户说"我的日历共享给了谁/谁能看我日历/日历权限/取消共享/把日历分享给xxx":
- 查看我共享出去的情况（即谁有权访问我的日历） → `acl list`
- 把我的日历共享给他人 → `acl add --user <USER_ID> --privilege reader`
- 取消我的日历对某人的共享 → `acl delete --acl-id <ACL_ID>`（aclId 来自 `acl list`）

> **易混淆辨析**：`book list` 查的是"我能看哪些日历本"（包含别人共享**给我**的）；`acl list` 查的是"我的日历共享**给了谁**"（我的主日历的访问控制列表）。两者方向相反，不可混用。

用户说"查下xxx的日程安排":
- 查询是否有共享关系 -> `book list`
  - 场景1: 共享日历本中有来自 xxx 的，且权限大于reader，那么通过 `event list --calendar-id <xxx的日历本id> `可查到xxx完整的日程安排
  - 场景2: 共享日历本中没有来自 xxx 的。那么通过 `busy search -- <USER_ID>`，查询xxx的忙闲安排

## 核心工作流

### 创建会议 + 邀请参会人 + 预订会议室

`event create` 支持 `--attendees` 在创建时直接指定参会人，**自 calendar MCP v2 起**也支持 `--rooms` 在创建时一并预定会议室；旧流程的「先创建日程再 `room add`」依然有效。

**关键区分**：`event create --rooms` 仅在**日程尚不存在**时使用；若日程已存在（同一会话刚创建、或用户指向已有日程），必须走「给已有日程订会议室」流程（见下方），**禁止**重复 `event create`。

**方式一：创建时一步完成（仅当日程尚不存在时推荐）**

```bash
# Step 1: 搜索空闲会议室，记下 roomId
dws calendar room search --start "2026-03-10T14:00:00+08:00" --end "2026-03-10T15:00:00+08:00" --format json
# 若返回错误(会议室超100条)，先查分组再按分组搜索:
#   dws calendar room list-groups --format json
#   dws calendar room search --start ... --end ... --group-id <GROUP_ID> --format json

# Step 2: 创建日程时直接指定参会人 + 会议室
dws calendar event create --title "Q1 复盘会" \
  --start "2026-03-10T14:00:00+08:00" --end "2026-03-10T15:00:00+08:00" \
  --attendees userId1,userId2 \
  --rooms <ROOM_ID_FROM_STEP1> --format json
```

**方式二：先创建日程，再单独添加参会人 / 会议室**

```bash
# Step 1: 创建日程 — 提取 eventId
dws calendar event create --title "Q1 复盘会" \
  --start "2026-03-10T14:00:00+08:00" --end "2026-03-10T15:00:00+08:00" --format json

# Step 2: 添加参会人（必须用 Step 1 返回的 eventId）
dws calendar attendee add --event <EVENT_ID> --attendees userId1,userId2 --format json

# Step 3: 搜索空闲会议室
dws calendar room search --start ... --end ... --format json

# Step 4: 预定会议室
dws calendar room add --event <EVENT_ID> --rooms <ROOM_ID> --format json
```

### 给已存在的日程加附件

```bash
# Step 1: 用钉盘上传文件，得到 fileId（参见 dws drive 系列命令）
# Step 2: 把附件挂到指定日程
dws calendar attachment add --event <EVENT_ID> --files <FILE_ID>:report.pdf,<FILE_ID2>:slides.pptx --format json
```

### 查看日程列表

```bash
dws calendar event list --start "2026-03-10T14:00:00+08:00" --end "2026-03-10T15:00:00+08:00" --format json
```

## 上下文传递表

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `event create` | `eventId` | attendee/room/attachment 操作的 --event |
| `event list` | `events[].eventId` | event get/update/delete/respond 的 --id |
| `event suggest` | 推荐的时间段 | event create 的 --start/--end |
| `event respond` | 响应结果 | — |
| `room search` | `rooms[].roomId` | room add 的 --rooms 或 event create 的 --rooms |
| `room list-groups` | `groups[].groupId` | room search 的 --group-id |
| `book list` | `id`（如 `primary`） | event list/get 的 --calendar-id, book get/update 的 --id |
| `book get` | 日历详细信息 | — |
| `book search` | 匹配的日历列表 | book get/update 的 --id |
| `acl list` | `aclId` | acl delete 的 --acl-id |
| `acl add` | 新增的权限记录 | — |
| 钉盘上传 | 文件 `fileId` | attachment add 的 --files `<fileId>:<name>` |

## 注意事项

- 时间格式: `event create/update`、`event list`、`busy search` 和 `event suggest` 用 ISO-8601
- 时区: `event create/update` 和 `event suggest` 支持 `--timezone` 指定 IANA 时区（如 `Asia/Shanghai`、`America/New_York`），不传默认 `Asia/Shanghai`
- 创建日程时可通过 `--attendees` 直接指定参会人（最多500人），也可创建后用 `attendee add --attendees ...` 单独添加
- `--attendees` 和 `--open-dingtalk-ids` 至少传一个（如果需要指定参会人）
- 添加参会人时可通过 `--optional` 设为可选参会人（默认必选）
- `event suggest` 根据参会人闲忙自动推荐合适时间，适合会议时间未确定时使用
- 创建日程**支持**通过 `--rooms` 一步预定会议室（`event create --rooms roomId1,roomId2`）；若创建后再加，仍可用 `room add`
- `room search` 不带 `--group-id` 时查根目录；企业会议室超过 100 条会报错，此时需先 `room list-groups` 获取分组，再按分组逐一查询
- `room list-groups` 支持 `--limit` / `--page` 分页（schema 类型为字符串）
- **`event create --rooms` / `room add --rooms` 的唯一合法来源**：最近一次（同一会话、同一时段窗口）`room search` 返回体中的 `roomId`；禁止把用户自然语言会议室名当 `roomId` 传入（否则会 `roomId invalid` 等错误）
- **搜房无结果**：在符合早停/用户限定范围内，`room search`（含按分组逐组查）全部返回空或无空闲 → 应**直接向用户报错/说明失败**并结束订房；**禁止**假设 roomId、禁止无合法 `roomId` 时调用 `room add` / `event create --rooms` 试探、禁止用 `event get` 等绕路推断 roomId
- **评测 / 自动化断言**：凡涉及 `room add` / `event create --rooms` 的流程，`--rooms` 只能填上游 `room search`（或等价接口）返回 JSON 中的 **`rooms[].roomId`**；不得以会议室展示名、楼层文案或用户口语当作 `roomId`
- **附件**：`attachment add` 仅负责挂载，**不上传**文件；fileId 必须先通过钉盘流程取得；`--files` 多附件用 `<fileId>:<name>` 元素逗号分隔
- **日历本**：`book list` 返回的 `id` 才是合法 `calendarId`；如无明确说明，`event list` / `event get` 都不要带 `--calendar-id`，让接口默认走 primary 主日历
- **已弃用入参**：`--max-results`（event list）在新 MCP schema 中被移除；CLI 仍接受但**不会**透传到 MCP；模型生成命令时**禁止**继续使用

## 自动化脚本

| 脚本 | 场景 | 用法 |
|------|------|------|
| [calendar_today_agenda.py](../../scripts/calendar_today_agenda.py) | 查看今天/明天/本周日程安排 | `python calendar_today_agenda.py today` |
| [calendar_schedule_meeting.py](../../scripts/calendar_schedule_meeting.py) | 一键创建日程+添加参会人+预定会议室；搜房失败时输出明确原因并返回非零退出码 | `python calendar_schedule_meeting.py --title "复盘会" --start "2026-03-15T14:00" --end "2026-03-15T15:00" --users userId1 --book-room` |
| [calendar_free_slot_finder.py](../../scripts/calendar_free_slot_finder.py) | 查询多人共同空闲时段 | `python calendar_free_slot_finder.py --users userId1,userId2 --date 2026-03-15` |

## 相关产品

- conference 视频会议 — 当前 CLI 不支持发起/预约/邀请入会/会中控制；请在钉钉客户端操作
- [contact](./contact.md) — 搜索同事 userId，用于 attendee add --attendees
