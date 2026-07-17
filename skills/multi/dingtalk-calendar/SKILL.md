---
name: dingtalk-calendar
description: 钉钉日历与会议室。Use when 用户说 约会议/查日程/订会议室/查闲忙/加参会人/改期/取消会议/今天的日程/本周日程/共同空闲。视频会议发起/邀请入会/会中控制当前 CLI 不支持，应提示在钉钉客户端操作；AI 听记走 dingtalk-minutes，待办任务走 dingtalk-todo。命令前缀：dws calendar。
cli_version: ">=0.2.14"
metadata:
  category: product
  stability: experimental
  requires:
    bins:
      - dws
---

# 钉钉日历 Skill

> 🧪 **EXPERIMENTAL · 试验版 / Preview** — multi 模式当前未达 stable 标准。全部 dingtalk-* skill 已通过 dispatch verifier，但接口、命名、跨 skill 引用后续可能调整；生产 / 共享环境请优先使用 mono 模式（`dws skill setup --mode mono`）。问题请提 issue 反馈。

## 前置条件 — 执行操作前必读

> **`use_skill(dws-shared)`** — 认证、全局参数（`--format json` / `--yes`）、错误码、URL 模板、跨产品消歧、安全规则与 capability 边界。**执行任何 `dws` 命令前先读；** 单产品的清晰命令可直接用本 skill。

<!-- SAFETY_PREAMBLE_INJECT -->

> 命令参考：[calendar.md](references/calendar.md)；剧本：[03-meeting.md](references/03-meeting.md)。

<!-- VISIBLE_SHORTCUTS_START -->
## Shortcuts（无专用脚本/recipe 时优先）

以下 shortcut 来自独立于 Runtime Schema 的公开 catalog。先按本 skill 的意图表、脚本和 recipe 路由：存在精确覆盖该场景的专用脚本/recipe 时按其执行；否则用户意图命中时，shortcut 优先于手写原子命令。用 `dws shortcut list --service calendar --format json` 读取参数、约束、风险和示例，并以 `dws calendar <shortcut> --help` 核对当前 Cobra flags；不要对 `+` 路径调用 `dws schema`。

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
| "约会议（含参会人 + 会议室）" | `python scripts/calendar_schedule_meeting.py --title "<主题>" --start "<起>" --end "<止>" [--users <ids>] [--book-room]` |
| "多人共同空闲" | `python scripts/calendar_free_slot_finder.py --users <ids> --date <yyyy-MM-dd>` |
| "查闲忙" | `dws calendar event list --start "<ISO>" --end "<ISO>"` |
| "加参会人" / "订房" / "取消" | `dws calendar attendee add` / `room add` / `event delete` |

## 标准 SOP（必遵流程）

> 命中以下意图**必须**按对应 SOP 顺序执行；**禁止**跳步、替换命令、编造 userId/eventId。每条命令必须带 `--format json`，时间参数**必须**是 ISO-8601（如 `2026-07-03T14:00:00+08:00`）。

### SOP-1 查日程（list-events）

**触发**：今天/明天/本周日程/我有什么会/某时段日程。

1. **首选脚本（必须）**：`python scripts/calendar_today_agenda.py today|tomorrow|week`（聚合今日议程）。
2. **降级 CLI（必须）**：脚本不可用时 `dws calendar event list --start "<起始ISO>" --end "<结束ISO>" --format json`；不传 `--start/--end` 默认查今天（00:00:00~23:59:59）。`hasMore=true` 用 `--limit`/翻页。
3. **解析（必须）**：取真实 `eventId`、`attendees[]`、`start/end`；按需抽取，**禁止**把整段 JSON 原样贴出。

**禁止**：用 `event list` 替代闲忙查询（查闲忙走 SOP-3）、编造时间窗口、用非 ISO 时间格式。

### SOP-2 建日程（create-event）

**触发**：建日程/约会议/加日程。

1. **解析与会人（必须）**：对每个姓名 `dws aisearch person --keyword "<姓名>" --dimension name --format json` 取 `userId`，多人逗号拼接。
2. **执行（必须）**：`dws calendar event create --title "<主题>" --start "<ISO>" --end "<ISO>" --attendees <userId1,userId2> --format json`（按需加 `--location`/`--desc`/`--rooms`）。
3. **验证（必须）**：从返回取 `eventId`，可 `dws calendar event list --start "<ISO>" --end "<ISO>" --format json` 复核。

**禁止**：跳过与会人 userId 解析直接传姓名、编造会议室 roomId。

### SOP-3 查闲忙（check-busy）

**触发**：某人/会议室是否有空/找空闲时段/避免冲突。

1. **解析对象（必须）**：姓名 → `dws aisearch person --keyword "<姓名>" --dimension name --format json` 取 `userId`；会议室用 `roomId`。
2. **收敛时段（必须）**：`--start`/`--end` **必须**由用户给出或明确收敛；时段不明确**必须先追问**，**禁止**默认全天窗口。
3. **执行（必须）**：`dws calendar busy search --users <userId1,userId2> --start "<ISO>" --end "<ISO>" --format json`（查会议室换 `--rooms <roomId...>`，可同时传）。**禁止**用 `event list` 扫日程替代闲忙查询。
4. **空闲时段（必须）**：找共同空闲用 `python scripts/calendar_free_slot_finder.py`。

**禁止**：用 `event list` 冒充 `busy search`、未确认时段就默认全天查询。

## 执行硬约束

- 多轮日程任务必须保留 `eventId`，后续加人、移人、订房、换房、改描述、删除都基于同一个 `eventId` 执行；不要重新创建重复日程。
- 用户明确授权“任意空闲会议室”时，`room search` 返回候选后选择第一个可预订且不需要自定义审批的 `roomId` 执行 `room add`；用户给了地点、容量或设备条件时按条件过滤。
- 已有日程订房：`dws calendar room search --start ... --end ... --format json` → `dws calendar room add --event <EVENT_ID> --rooms <ROOM_ID> --format json` → `event get` 或 `room/busy` 验证。
- 换会议室：先 `room delete --event <EVENT_ID> --rooms <OLD_ROOM_ID>`，再 `room add --event <EVENT_ID> --rooms <NEW_ROOM_ID>`，最后回查；不要只更新 `--location`。
- 参会人变化用 `attendee add/delete`，日程描述变化用 `event update --desc`，删除日程用 `event delete --id`。用户当前消息已明确要求删除/取消时可直接执行；否则先确认。
- 脚本失败或参数不完整时，查明缺失参数后改用明确的 `dws calendar event/attendee/room` 命令完成同一流程。
- 所有 dws 命令带 `--format json`；查询时间必须显式 `--start` / `--end`。

## 跨产品协作

- 视频会议发起 / 入会链接 / 邀请入会 / 会中控制 → 当前 CLI 不支持，请在钉钉客户端操作；预约日程仍走 `calendar`
- 会后摘要 / 待办 → 切到 `dingtalk-minutes`
- 参会人按人名 → 先用 `dingtalk-aisearch` 解析

## 注意

`schedule-meeting` 必须读 [03-meeting.md](references/03-meeting.md) 中的「两准则」「搜房失败硬门禁」，禁止假设 `roomId`。
## 局部意图与 Recipe

- [局部意图消歧](references/intent-guide.md)；[Lite Recipe](references/lite-recipes.md)。
