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

## 执行契约

- 明确的 Calendar 请求直接按本 Skill 执行；仅在跨产品、profile、确认或错误恢复需要时读取 [`dingtalk-shared`](../dingtalk-shared/SKILL.md) 的对应章节。
- 已知意图直接走下方 Golden Route。只有 leaf 参数或安全语义不确定时读取单个 compact Schema，只有 Cobra flag 不确定时读取精确 leaf Help。
- 所有目标解析、读取、写入和验证使用同一 profile。`eventId`、`roomId`、`calendarId`、`userId` 只取真实返回；零命中或多候选时停止消歧。
- 写操作按 Runtime confirmation gate 执行：先解析并展示目标；需要确认时获得确认后才加 `--yes`。退出码为 0 不等于业务完成，必须检查结构化 outcome 和读回证据。
- 用户在原请求中明确要求创建、修改、邀请、取消、删除或恢复，即已授权对应步骤：第一次调用就带 `--yes --format json`，不要先故意触发 `confirmation_required`、不要插入 dry-run，也不要在“看过以后/办完后/最后删除”处再次停下来询问。只有目标或影响范围尚不明确时才追问。
- 默认只加载一个操作 Reference：通用原子命令读 [calendar.md](references/calendar.md)；涉及会议室预订读 [03-meeting.md](references/03-meeting.md)；遇到写后读回不一致、长分页或多步清理时改读更短的 [执行收束](references/execution-bounds.md)，不要同时加载完整命令参考。

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

## Golden Routes

| 用户意图 | 首选入口 | 身份与边界 |
|---|---|---|
| 今天 / 明天 / 本周日程 | `dws calendar +today|+tomorrow|+week` | 当前 profile 的主日历；无需手算时间窗 |
| 任意时段日程 | `dws calendar +agenda --start "<ISO>" --end "<ISO>"` | 保留 `eventId` 和分页证据；`hasMore` 时继续翻页 |
| 按标题、描述或地点找日程 | `dws calendar +search-event --query "<关键词>"` | 单页零命中且 `hasMore=true` 不是全局零命中 |
| 创建个人日程或按姓名约人 | `dws calendar +book --title "<主题>" --start "<ISO>" --end "<ISO>" [--with "张三,李四"]` | 姓名必须唯一解析；写后读回；不含会议室预订 |
| 查看最近一场 | `dws calendar +next-event` | 默认未来 7 天；不要用它代替完整列表 |
| 查某人 / 会议室闲忙 | 姓名用 `+free`；ID 用 `+freebusy` | 必须有明确时段；至少指定 users/rooms 一类 |
| 推荐多人共同时间 | `dws calendar +suggest-time --with "张三,李四" --start "<ISO>" --end "<ISO>"` | 只推荐，不创建日程 |
| 找可用会议室 | `dws calendar +room-find --start "<ISO>" --end "<ISO>"` | 名称定位但不查可用性时才用 `+room-search` |
| 邀请参会人 | `dws calendar +invite --event <EVENT_ID> --with "张三,李四"` | 只修改指定已有日程 |
| 改期 | `dws calendar +reschedule --event <EVENT_ID> --start "<ISO>" --end "<ISO>"` | 只改起止时间；其他字段保持不变 |
| 取消日程 | `dws calendar +cancel-event --event <EVENT_ID>` | 高风险删除；先读目标，确认后执行并验证不存在 |

当用户需要 shortcut 未公开的字段、共享日历、循环规则、附件、ACL 或会议室绑定时，才降级到 [calendar.md](references/calendar.md) 的单个原子 leaf。

## 最短完成路径

- 按用户验收项维护最小资源台账；后续更新、恢复、读回和清理只用同一创建回执的 `eventId/roomId/aclId`，禁止从宽泛列表换用相似旧资源。
- 已知路由直接执行；仅在 `unknown flag/command` 后查一次精确 leaf Help并最多修正一次。`+book` 只覆盖标题、时间和姓名；需地点、忙闲、循环、提醒或创建时订房时直接用 `event create`。
- 原请求已明确“办完后删除/取消/恢复/撤销”即已授权收尾，到达该步通过 Runtime gate 加 `--yes`，不重复询问；任务中途失败或预算将尽也先清理本轮临时资源。
- 用户要求“把详情/名单/规则/分享信息给我”时，清理前先读回并保存这些验收字段；随后完成清理，最终答复同时呈现保存的业务信息和清理终态。不要因为资源已删除而只回答“已删除”。
- 只保留下一步所需的 ID、游标、时间、状态和验收字段，不把完整列表或原始 JSON 反复送回上下文。

## 有限搜索与 Token 预算

- 标题定位先用合理时间窗和精确关键词；普通定位限 1 个时间窗、2 个关键词、2 页，未唯一命中即收束。仅用户明确要求完整遍历时翻到 `hasMore=false`，优先最大 `--limit`；指定小页长则本地循环聚合匹配/计数/去重与最终分页证据。
- 不试探不存在的 flags，不用同义词扫描数十页，不逐间重复 busy 查询。前置资源不存在、系统日历不可修改或绑定被拒时直接报告边界；详细预算与 commit-unknown 协调见 [执行收束](references/execution-bounds.md)。

## 资源与安全约束

- 时间必须是带时区的 ISO-8601，且 `end > start`。预约意图缺少起止时间时先追问；不要自设全天窗口。`+today/+tomorrow/+week` 和自身声明默认窗口的只读 shortcut 除外。
- 多轮任务持续复用同一 `eventId`；更新、邀请、订房和取消不得通过再次创建来替代。
- 用户点名日程但未给 `eventId` 时，用标题和合理时间窗检索；零命中停止，多候选列出标题、时间和 `eventId` 让用户选择，禁止选第一条。
- 用户姓名必须唯一解析到当前 profile 的身份；零命中、多候选或跨 profile 不复用 ID。
- 多人任务建立 `输入姓名 -> userId -> displayName` 一一映射：不同姓名必须得到不同 userId，且被邀请人不得误解析为当前组织者；发现重复、别名碰撞或返回姓名不符时停止，不得继续荐时或建会。读回参会人时按 userId 核对每一位，不能只看人数或展示名。
- `roomId` 只能来自同一 profile、同一目标时段下的 `+room-find` / `room search` 返回。会议室名、楼层编号和地点文本都不是 `roomId`；`--location` 也不等于预订。
- 用户指定时间或会议室时不得擅自换时间、换房或扩大地点范围。允许范围无空房时停止并说明，若要继续必须让用户明确放宽条件。
- 分页必须跟随 `nextCursor` 或 page 语义，检测 cursor 丢失、停滞和循环；不得把第一页当全量。
- 非事务多步写入发生 partial、pending 或 commit-unknown 时如实报告已完成与未完成步骤，先读回协调，禁止盲目重试非幂等创建。
- 创建回执失败但包含 `eventId`，或报 `readback_attendee_missing` / 读回字段缺失时，视为“可能已提交”：先对该 `eventId` 做详情与参会人协调；只有回执没有 ID 时才用精确标题+起止时间唯一定位。不得再次创建，也不得借用当天列表中的其他 eventId。
- 推荐时间必须核对返回候选的实际时长等于用户要求的分钟数；无足够时长就明确无可用候选，禁止把 30 分钟候选表述成 60 分钟。
- “找时间并创建”必须把 `--duration <分钟>` 传给荐时入口，且只能选择返回区间完整容纳该时长的候选；没有合格候选就报告前置条件不满足，不得自行缩短会议。

## 写后验证

- 创建：以结构化结果中的 `eventId` 为身份，读回核对标题、起止时间和预期参会人。
- 创建命令报告读回失败但同一 `eventId` 的详情已存在时，分别记录“事件已创建”和“哪些字段尚未验证”；补做缺失字段的精确读回，不把整个创建当作可安全重放。
- 邀请 / 移除参会人：读取参会人列表核对目标；底层不提供稳定 userId 时不得伪造身份字段。
- 改期 / 更新：读回同一 `eventId`，核对实际变更字段并确认未意外覆盖其他字段。
- 订房 / 换房：读回同一日程，并用对应时段的会议室闲忙或日程详情确认绑定；仅有空响应不能证明成功。
- 取消：读回明确为不存在；权限错误、超时或缺少不存在证据时不得报告成功。
- 清理优先级高于附加汇总：任务后段出现查询失败、订房失败或 Token/时间预算将尽时，先清理本轮台账中的所有临时资源，再报告未完成项。每个删除必须使用其对应创建回执的 ID。

## 产品边界

- 视频会议发起、入会链接、会中控制：当前 Calendar CLI 不支持，不能臆造 conference 命令。
- AI 听记、会后摘要与转写：转 `dingtalk-minutes`；待办任务与独立截止提醒：转 `dingtalk-todo`。
- 按姓名解析人员由 Calendar 的 `+book/+invite/+free/+suggest-time` 优先完成；只有原子命令路径才先用 `dingtalk-aisearch`。
- 日历事件是 Calendar 资源；“给自己留时间块”不是 Todo。

涉及“创建日程并订会议室”的组合流程时读取 [03-meeting.md](references/03-meeting.md)，严格执行时段、地点、`roomId` 来源与失败收束规则。
