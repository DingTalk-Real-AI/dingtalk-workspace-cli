---
name: dingtalk-calendar
description: 钉钉日历与会议室。Use when 用户说 约会议/查日程/订会议室/查闲忙/加参会人/改期/取消会议/今天的日程/本周日程/共同空闲。不做视频会议发起/邀请入会/会中控制（走 dingtalk-misc）、AI 听记（走 dingtalk-minutes）、待办任务（走 dingtalk-todo）。命令前缀：dws calendar。
metadata:
  cli_version: ">=0.2.14"
  category: product
  requires:
    bins:
      - dws
---

# 钉钉日历 Skill

## 前置条件 — 执行操作前必读

> **CRITICAL — 执行任何 `dws` 操作前，MUST 先用 Read 工具完整读取 [`dingtalk-shared`](../dingtalk-shared/SKILL.md)。**该轻量文件包含全局执行契约、安全底线及 shared references 的按需加载导航；不要预加载其全部 references。

> 常用路径直接按本文件执行，不要预读大而全的 [calendar.md](references/calendar.md)。仅在循环日程、响应邀请、共享/ACL、附件、日历本或低频参数无法由本文件定位时按需读取对应小节；订会议室的范围与失败收束才读取 [03-meeting.md](references/03-meeting.md)。

<!-- VISIBLE_SHORTCUTS_START -->
## Shortcuts（无专用脚本/recipe 时优先）

以下 shortcut 同时进入公开 catalog 与 Runtime Schema。先按本 skill 的意图表、脚本和 recipe 路由：存在精确覆盖该场景的专用脚本/recipe 时按其执行；否则用户意图命中时，shortcut 优先于手写原子命令。命令已选中时直接执行；只在参数或安全语义不确定时读取 Agent leaf Schema（例如 `dws schema --cli-path "calendar +<shortcut>" --compact --format json`），在当前 Cobra flags 不确定时读取 `dws calendar <shortcut> --help`。只有参数映射、接口绑定或 provenance 审计才省略 `--compact`。仅当现有路由和 reference 都无法定位低频能力时，才用 `dws shortcut list --service calendar --format json` 批量发现。

| Shortcut | 风险 | 适用场景 |
|---|---|---|
| `dws calendar +agenda` | read | 查询日程列表（不传时间默认查询今天） |
| `dws calendar +attendee-list` | read | 查看日程参会人 |
| `dws calendar +book` | write | 创建日程，并可按姓名邀请参会人（自动解析 userId，失败自动回滚删除日程） |
| `dws calendar +book-list` | read | 查询用户的日历本列表 |
| `dws calendar +book-search` | read | 按名称模糊搜索日历本 |
| `dws calendar +cancel-event` | high-risk-write | 取消（删除）一个已有日程（删除前先确认它真实存在） |
| `dws calendar +conflicts` | read | 检测我某天日程的时间冲突（重叠/双重预订，默认今天） |
| `dws calendar +free` | read | 按姓名查询某人在指定时间段内的忙闲状态（自动解析 userId） |
| `dws calendar +free-slots` | read | 找我某天工作时段内的空闲时间段（默认今天 09:00-18:00） |
| `dws calendar +freebusy` | read | 查询用户 / 会议室闲忙状态（--users 与 --rooms 至少其一） |
| `dws calendar +invite` | write | 按姓名把参会人加入已有日程（自动解析 userId 后批量添加） |
| `dws calendar +my-free` | read | 查我自己在某时间段的忙闲（默认今天，无需输入姓名） |
| `dws calendar +next-event` | read | 查看接下来最近的一个日程（默认扫描未来 7 天） |
| `dws calendar +reschedule` | write | 改一个已有日程的时间（只动开始/结束时间，其他字段不变） |
| `dws calendar +room-find` | read | 按时间段搜索可用会议室（不传时间默认当前起 1 小时） |
| `dws calendar +room-groups` | read | 会议室分组列表 |
| `dws calendar +room-search` | read | 按名称模糊搜索会议室（不检查可用性） |
| `dws calendar +suggest-time` | read | 按姓名解析多位参与者，推荐大家都有空的可开会时间段（自动解析 userId） |
| `dws calendar +today` | read | 列出我今天的日程（自动计算今天的起止时间，无需手动填时间范围） |
| `dws calendar +tomorrow` | read | 列出我明天的日程（自动计算明天的起止时间，无需手动填时间范围） |
| `dws calendar +week` | read | 列出我本周的日程（自动按周一为周首计算本周起止时间，无需手动填时间范围） |
<!-- VISIBLE_SHORTCUTS_END -->

## 意图表

| 用户说 | 命令 |
|--------|------|
| "今天 / 明天 / 本周日程" | `python scripts/calendar_today_agenda.py [today\|tomorrow\|week]` |
| "约会议（含参会人 + 会议室）" | 先搜房取 `roomId`，再一次 `event create --attendees <ids> --rooms <roomId>`；复杂降级才用 `calendar_schedule_meeting.py` |
| "多人共同空闲" | `python scripts/calendar_free_slot_finder.py --users <ids> --date <yyyy-MM-dd>` |
| "查闲忙" | `dws calendar busy search --users <userIds> --start "<ISO>" --end "<ISO>"` |
| "加参会人" / "订房" / "取消" | `dws calendar attendee add` / `room add` / `event delete` |

## 常用快路径（省略 Help / Reference）

以下参数已确定时直接执行，**不要**再调用 `--help`、Schema 或读取 `calendar.md`：

| 意图 | 直接命令 |
|---|---|
| 按姓名推荐共同时间 | `dws calendar +suggest-time --with "姓名1,姓名2" --start "<ISO>" --end "<ISO>" --duration <分钟> --format json` |
| 按姓名创建会议（无会议室） | `dws calendar +book --title "<主题>" --start "<ISO>" --end "<ISO>" --with "姓名1,姓名2" --format json`；Runtime 要求确认后才精确重放并加 `--yes` |
| 检查第 N 天冲突 | `dws calendar +conflicts --in-days <N> --format json`（今天 `N=0`） |
| 查第 N 天空闲窗口 | `dws calendar +free-slots --from <开始小时> --to <结束小时> --in-days <N> --format json` |
| 搜索空闲会议室 | `dws calendar room search --start "<ISO>" --end "<ISO>" [--group-id <ID>] --available --format json` |
| 日程详情/参会人 | `dws calendar event get --id <EVENT_ID> --format json` / `dws calendar attendee list --event <EVENT_ID> --format json` |

同一状态下，相同命令和相同参数只调用一次；只有发生写操作、拿到新游标，或首次结果明确不完整时才允许重查。按标题寻找日程时，从最可能且最窄的时间窗开始；一旦得到唯一匹配立即停止，不继续扫描其他月份。

## 标准 SOP（必遵流程）

> 命中以下意图**必须**按对应 SOP 顺序执行；**禁止**跳步、替换命令、编造 userId/eventId。每条命令必须带 `--format json`，时间参数**必须**是 ISO-8601（如 `2026-07-03T14:00:00+08:00`）。

### SOP-1 查日程（list-events）

**触发**：今天/明天/本周日程/我有什么会/某时段日程。

1. **首选脚本（必须）**：`python scripts/calendar_today_agenda.py today|tomorrow|week`（聚合今日议程）。
2. **降级 CLI（必须）**：脚本不可用时 `dws calendar event list --start "<起始ISO>" --end "<结束ISO>" --format json`；不传 `--start/--end` 默认查今天（00:00:00~23:59:59）。返回 `nextCursor` 时必须继续翻页，直到游标为空；不能把“单页成功”当成完整查询。
3. **解析（必须）**：取真实 `eventId`、`attendees[]`、`start/end`；按需抽取，**禁止**把整段 JSON 原样贴出。用户要汇总或计数时，在本地完成聚合，只输出各分组计数和必要明细，避免逐条铺满答复导致后半段被截断。

**禁止**：用 `event list` 替代闲忙查询（查闲忙走 SOP-3）、编造时间窗口、用非 ISO 时间格式。

### SOP-2 建日程（create-event）

**触发**：建日程/约会议/加日程。

1. **解析与会人（必须）**：对每个姓名 `dws aisearch person --query "<姓名>" --dimension name --format json` 取 `userId`，多人逗号拼接。
2. **执行（必须）**：`dws calendar event create --title "<主题>" --start "<ISO>" --end "<ISO>" --attendees <userId1,userId2> --format json`（按需加 `--location`/`--desc`/`--rooms`）。
3. **验证（按需一次）**：从创建返回 `result.id` 取日程 ID（下游参数语义称 `eventId`）。创建返回已包含用户要求的标题、描述和时段时直接复用，不为“再看一遍”额外 list；只有返回缺字段、用户明确要求回查，或后续状态发生变化时才执行一次 `event get`/窄时间窗 `event list`。

**禁止**：跳过与会人 userId 解析直接传姓名、编造会议室 roomId。

### SOP-3 查闲忙（check-busy）

**触发**：某人/会议室是否有空/找空闲时段/避免冲突。

1. **解析对象（必须）**：姓名 → `dws aisearch person --query "<姓名>" --dimension name --format json` 取 `userId`；会议室用 `roomId`。
2. **收敛时段（必须）**：`--start`/`--end` **必须**由用户给出或明确收敛；时段不明确**必须先追问**，**禁止**默认全天窗口。
3. **执行（必须）**：`dws calendar busy search --users <userId1,userId2> --start "<ISO>" --end "<ISO>" --format json`（查会议室换 `--rooms <roomId...>`，可同时传）。**禁止**用 `event list` 扫日程替代闲忙查询。
4. **空闲时段（必须）**：找共同空闲用 `python scripts/calendar_free_slot_finder.py`。

若指定窗口内确实没有满足时长的共同空闲，明确给出最长共同空闲窗口并询问是否缩短时长、扩大时间范围或换日期；未经用户确认不得擅自改时间，也不得声称已安排会议。

**禁止**：用 `event list` 冒充 `busy search`、未确认时段就默认全天查询。

## 执行硬约束

- **先核对诉求，再清理资源**：把用户要求看到的详情、冲突对、参会人、会议室和验证结论先从结构化返回中提取并暂存；即使随后删除临时资源，最终答复仍必须逐项呈现这些结果。工具调用成功不等于已经向用户交付结果。
- **目标验证必须点名结论**：冲突/忙闲/搜索返回很多记录或输出被截断时，必须按当前 case 的标题、`eventId`、人员或资源 ID 收敛核对；不能只说“运行了检查”。例如冲突任务需明确指出目标 A/B 的重叠起止时间，以及首尾相接是否被排除。
- **新日程 + 新会议室优先两步完成**：用户从一开始就要求创建日程并订房，且没有依赖已创建 `eventId` 的中间操作时，先 `room search` 取得真实 `roomId`，再用一次 `event create ... --rooms <ROOM_ID>` 完成创建和预订；参会人也可同时用 `--attendees` 带入。只有日程已存在、用户要求先看日程再订房，或一步创建失败时，才使用 `room add`，禁止无故走“create → room add”两次写入。
- 多轮日程任务必须保留 `eventId`，后续加人、移人、订房、换房、改描述、删除都基于同一个 `eventId` 执行；不要重新创建重复日程。
- 用户明确说"帮我订一个空闲会议室"时，`room search` 返回可用会议室后直接选择第一个可预订且不需要自定义审批的 `roomId` 执行 `room add`；不要把选择权抛回用户导致任务停住。
- 已有日程订房：`dws calendar room search --start ... --end ... --format json` → `dws calendar room add --event <EVENT_ID> --rooms <ROOM_ID> --format json` → `event get` 或 `room/busy` 验证。
- 换会议室：先 `room delete --event <EVENT_ID> --rooms <OLD_ROOM_ID>`，再 `room add --event <EVENT_ID> --rooms <NEW_ROOM_ID>`，最后回查；不要只更新 `--location`。
- 参会人变化用 `attendee add/delete`，日程描述变化用 `event update --desc`，删除日程用 `event delete --id`。用户当前消息已明确要求删除/取消时可直接执行；否则先确认。
- **删除终态按需回查一次**：用户明确要求“确认删除/没有残留”时，`event delete` 成功后用原 `eventId`、标题、日历本和原始起止时间做一次窄窗活跃列表核对；普通“办完后取消”以结构化删除成功为终态，不额外 get/list。`event get` 仍可能返回历史对象或墓碑；除非返回里真实存在 `status=cancelled`，不得声称“状态已变为 cancelled”。
- 脚本失败或参数不完整时，立即降级到明确的 `dws calendar event/attendee/room` 命令，不要停在"我要查看用法"。
- 所有 dws 命令带 `--format json`；查询时间必须显式 `--start` / `--end`。

## 跨产品协作

- 视频会议发起 / 入会链接 / 邀请入会 / 会中控制 → 当前 CLI **不支持**；请在钉钉客户端完成
- 会后摘要 / 待办 → 切到 `dingtalk-minutes`
- 参会人按人名 → 先用 `dingtalk-aisearch` 解析

## 注意

普通“给定时段 + 任意空闲会议室”直接走本文件两步快路径；只有用户限定楼宇/楼层/具体会议室、搜索为空或需要放宽范围时，才读取 [03-meeting.md](references/03-meeting.md) 的范围早停与失败门禁。任何路径都禁止假设 `roomId`。
## 局部意图与短流程

- [局部意图消歧](references/intent-guide.md)；[短流程](references/lite-recipes.md)。
