# calendar Lite Recipe

本文件从单 Skill `lite-recipes.md` 拆分而来，仅保留与本产品相关的轻量流程。

## #3 会议日程

### list-today-meetings

**优先**：`python scripts/calendar_today_agenda.py [today|tomorrow|week]`
备选：`dws calendar event list --start "<今日起始ISO>" --end "<今日结束ISO>"`（须加 `--format json`）

### check-users-busy

查询多人在某时段内的闲忙（**busy**，不是用 `event list` 扫日程）：

1. 解析用户：对每个姓名执行 `aisearch person --keyword "<姓名>" --dimension name` → `userId`；多人将 `userId` 用英文逗号拼接（无空格或按 [calendar.md](./calendar.md) `busy search` 要求）。
2. 确认时段：用户须给出或可收敛为明确的 `--start` / `--end`（ISO-8601）；若未给出，**先追问**起止时间，禁止用任意默认全天窗口代替用户意图。
3. 执行：`dws calendar busy search --users <userId1,userId2,...> --start "<ISO>" --end "<ISO>" --format json`

详见 [calendar.md](./calendar.md) 中「查询用户闲忙状态」。

### 视频会议能力边界

当前 CLI 不提供视频会议发起、预约、入会邀请或会中控制。用户只说“开个会/发起会议”且未给日程时段时，直接说明请在钉钉客户端操作，不要构造视频会议命令。

- 有具体时间（如“明天 3 点开会”）或明确预约日程 → 走 [03-meeting.md](./03-meeting.md) 的 `schedule-meeting`
- 只有实时视频会议诉求 → 说明当前 CLI 不支持，请在钉钉客户端发起
