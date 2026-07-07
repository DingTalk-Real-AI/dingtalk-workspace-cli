# 会议管理（日程与会议室）

> **lite 速查表**含 `list-today-meetings`、`check-users-busy`；视频会议 `start-conference` 当前 CLI 不支持。列表类操作须遵循 [calendar.md](./calendar.md) **「CLI 命令树与黄金路径」**，禁止无子命令的 `dws calendar` 或臆造 `calendar list`（见该文 **「反模式（禁止）」**）。**`schedule-meeting` 不做内联**：须读本文件 **「两准则」「搜房失败硬门禁」** 及下表 **schedule-meeting** 行全文。
> **听记、会后待办、摘要分享** 见 `dingtalk-minutes` sub-skill 的 `references/07-minutes.md`。

## 日程与会议室两准则（强制）

1. **时段**：用户已明确会议起止时间 → **禁止**自动改期、禁止用闲忙结果或「推荐时段」覆盖用户给定时段；只能在此时段内建日程、订会议室；该时段内无可用或指定资源不可用 → **立刻如实告知**，不得偷偷换时间段再试。
2. **会议室**：用户点名具体会议室 → **禁止**换其他会议室；在用户给定时段内查无该房 → **立刻告知**。**用户未给出时段时，必须先显式向用户追问具体开始/结束时间；禁止默认用「当前时刻至当日 23:59:59」之类窗口代查。** **`calendar_schedule_meeting.py`**：仅需 `--title`、`--start`、`--end`；先创建日程，再邀请参会人，最后搜房/订房。未给会议室范围时，`--book-room` 为 **单次**无 `--group-id` 的 `room search --available`，取返回的**第一个**会议室并 `room add`；无结果则告警、不删日程。**若用户明确限定楼层/楼宇/园区/分组，应先用 `room list-groups` 解析允许的 `group-id`，再把这些 `group-id` 传给脚本 `--room-group-id`（或手工 `room search --group-id ...`）；脚本只会在这些 group 内查找，若无空房则直接返回。对于同一地点（同园区/楼栋/楼层）的会议室，必须优先锁定最相关、最贴近该地点的承载 group；该 group 查无 roomId/空房，即可判定该地点当前时段无可订会议室，**不得**再去别的无关 group 继续碰运气，因为同一地点的会议室只会挂在其所属 group 下。** **在组织内、按早停规则已把应查的分组（或未限范围时的根目录一次查询）全部查完仍无可用会议室时，必须立即向用户说明「当前时段没有可预订的会议室」或「范围内未检索到可用会议室/资源」并收束，禁止继续扩区、换参重试或虚构有房。** 手工 `room search` **禁止**为试出空闲擅自改日或拉长时间窗。

### 会议室搜索早停

> 专用于 `calendar room list-groups` / `room search` / `room add`；与通用规范「无新参数不重复 search」一致。

**`room search --available`**（与传入的 `--start` / `--end` 配对）：返回的是在**该整段时段内**可被预订的空闲会议室（不是「有一段空就算」）；脚本与用户手工选房均应沿用同一时间窗，避免误以为分段凑满即等价于整段可用。

**`dws calendar room search` 合法参数**（与 [calendar.md](./calendar.md) 一致）：仅 `--start`、`--end`、`--group-id`（可选）、`--available`（可选）、`--format json` 等；**禁止使用 `--query`**，否则会报 `unknown flag: --query`。

**地点归组早停**：若用户给的是同一地点范围（如“西溪园区 C6 楼 3-5 层”或具体楼层/楼栋），先用 `room list-groups` 找到**最相关的承载 group**（通常是该楼层；若楼层下无会议室则为直接挂会议室的上一级）。在这个最相关 group 下查不到有效 `rooms[].roomId` 或空房时，**不得**再跳去别的同级/异地 group 继续搜；同一地点的会议室不会散落在别的 group 里。只有用户明确放宽到别的楼层、楼栋或园区，才能重新解析新的 group 并继续。

**用户点名具体会议室（如「C6-4-06-N / 贡嘎山」）**：**不要**尝试 `room search --query "<名称>"`；**禁止**把用户原文（含「C6-4-06-N 贡嘎山」整句）或展示名当作 `room add --rooms` 的 `roomId`。用户输入**几乎从不会是**有效 `roomId`。须先 `dws calendar room list-groups` 定位所在楼层/分组的 `group-id`，再 `dws calendar room search --start "<ISO>" --end "<ISO>" --group-id <GROUP_ID> [--available] --format json`，在返回 `rooms[]` 中对 `roomName`、`name` 等与用户表述匹配，**仅**取 JSON 里的 `roomId`（典型为小写十六进制串，长度以返回为准），最后 `dws calendar room add --event <eventId> --rooms <roomId>`。该时段无匹配或房间忙 → 如实告知；**禁止**为通过校验而编造、拼接或猜测 `roomId`。

### 搜房失败硬门禁（园区/范围搜尽仍无 roomId）

在用户限定的园区、楼宇、楼层或评测固定分组内，已按早停规则**逐组 `room search` 查完**仍得不到任何有效 `rooms[].roomId`（或无任何空闲房）→ **立即停止**，向用户**明确报错/失败结论**（例如：该时段在指定范围内未检索到可预订会议室或无法获得 roomId），**本回合订房流程结束**。

**用户汇报硬门禁**：一旦触发上面的失败条件，**下一条对外输出必须直接面向用户汇报结果**，不得继续在会话里自言自语式地延长推理。允许的后续只有两类：
1. **失败汇报**：明确说明“指定范围/指定会议室在该时段未找到可预订会议室，因此当前无法完成预订”
2. **确认放宽条件**：仅在需要继续推进时，明确问用户是否放宽地点范围、改时间或接受不订会议室

以下表述/行为视为**违例**：继续写“让我再试一次”“也许是 Mock/测试环境”“可能存在预设 roomId 映射”“我去别的 group 看看”“我换个时间验证一下”“我先看看脚本/示例还能不能推断出 roomId”。

以下行为**一律禁止**（与是否「想多试一次」无关）：编造/假设 `roomId` 格式做「预订测试」；在**没有**合法 `roomId` 时调用 `room add` 试探错误详情；拉 `event get` / 日程详情等试图**绕开** `room search` 推断 roomId；换无关园区、扩大关键词、换工具名做未经用户授权的新搜索。

**失败后强制回读**：若出现以下任一信号，下一步**必须重新读取本文件本节与 `schedule-meeting` recipe**，不得沿着当前假设继续试：
1. 连续 **2 次** `room search` 空结果/无 `roomId`
2. 任意一次 `roomId invalid`
3. 已开始尝试「换园区 / 换楼栋 / 看 event 详情 / 猜 roomId」

回读后只允许二选一：
1. **报错收束**：已搜尽允许范围/整园仍无 `roomId` 或无空房
2. **用户确认**：明确询问是否放宽范围、换时间，或接受不订会议室

| # | 规范 |
|---|------|
| 1 | **一键脚本**：`calendar_schedule_meeting.py` 做「建日程 → 加人 → 可选搜房/订房」；未限范围时可直接 `--book-room`，脚本按根目录单次 `room search --available` 订第一家。**若搜房失败，脚本应输出明确失败原因并返回非零退出码，促使上层立即向用户汇报，而不是继续试探。** |
| 2 | **要限范围/具名**：先 `list-groups` 解析允许的 `group-id`。若用户说的是同一地点（同园区/楼栋/楼层），应优先锁定**最相关的承载 group** 并只查它；该 group 无结果即可按该地点无房收束，不再试别的无关 group。仅当用户明确给出多个允许地点时，才分别对这些 group 各 **1 次** `room search --available` 再 `room add` |
| 3 | **禁止**：无新信息时反复 `--verbose`、反复切 `--available`、父组子组试探、在最相关 group 无结果后改搜别的同级/异地 group、超 100 条后仍根分组或未授权区域全量搜；**禁止**对 `room search` 使用不存在的 `--query` |
| 4 | **`roomId` 门禁**：`room add --rooms` **只能**填 `room search` 返回 JSON 中的 `rooms[].roomId`；**禁止**将用户说的会议室名、编号文案、或「假 UUID / 试数字」当作 `roomId` |
| 5 | **全量无结果即收束**：在用户允许的搜索范围内（含**整园/全 campus** 若用户或评测要求已逐组查尽）仍无任何可用会议室或有效 `roomId` → **直接报错/告知失败并结束订房**，且**下一条消息必须汇报给用户**；**不得**假设 ID、不得用 `room add` 试探、不得绕路查日程、不得继续自说自话分析 Mock/测试环境 |
| 6 | **失败触发回读**：连续 2 次空结果、任意一次 `roomId invalid`、或开始换园区/绕路时 → **必须回读本节**；回读后只允许「报错收束」或「向用户确认是否放宽条件」 |

| Recipe             | 行动指南（固定路线） |
| ------------------ | ------------------- |
| schedule-meeting   | **见上文「两准则」**、**「搜房失败硬门禁」**。**未给时段且仅说「发起/开个会」**→ 不走本 recipe；当前 CLI 不支持实时视频会议，告知用户请在钉钉客户端操作。**未给时段但有预约意图**（"安排""约""定"等词）：追问具体开始/结束时间。**已有时段后**，按固定顺序执行：1. `dws calendar event create` 建日程；2. 有参会人则 `dws calendar participant add`；3. 再处理会议室。**无明确会议室范围**：可直接 `python scripts/calendar_schedule_meeting.py --title "<主题>" --start "<起始>" --end "<结束>" [--users <userIds>] [--book-room] [--dry-run]`。**有明确范围（某楼/层）**：先 `dws calendar room list-groups`，锁定该地点**最相关的承载 group**；若只有一个地点，`--room-group-id` 应只传这个最相关 group，**不要**把同楼内多个楼层 group 打包传入碰运气。只有用户明确给出多个允许地点时，才把这些 `group-id` 一并传给 `python scripts/calendar_schedule_meeting.py ... --book-room --room-group-id "<id1,id2,...>"`。**用户点名具体会议室**：须手工 `dws calendar room search --start "<ISO>" --end "<ISO>" --group-id <GROUP_ID> [--available] --format json`（**无** `--query`），在 JSON 中匹配名称取 **`rooms[].roomId` 唯一真值** → `dws calendar room add --event <eventId> --rooms <roomId>`；**不得**把用户输入的会议室名当 `roomId`。**一旦连续 2 次空结果 / 任意一次 `roomId invalid`**：**必须回读本节并立即收束判断**；若整园/限定范围内搜尽仍无 roomId 或无空房 → **下一条消息必须直接向用户汇报失败结论**；否则只能向用户确认是否放宽范围/改时间。**禁止**假设 roomId、禁止无 ID 调用 `room add`、禁止用日程详情绕路、禁止继续猜测 Mock/测试环境。细则见「会议室搜索早停」。 |
| reschedule-meeting | 1. `calendar event list --start "<起始ISO>" --end "<结束ISO>"` → 取 `eventId` 2. `calendar event update --id <eventId> --start "<新起始ISO>" --end "<新结束ISO>"` 更新时间 3. `chat search --query "<群名>"` → 取 `openConversationId` → `chat message send --group <openConversationId> --text "<变更通知>"` 通知变更 |
