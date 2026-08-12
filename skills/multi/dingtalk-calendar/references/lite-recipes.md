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

### start-conference

> 当前 CLI 不提供视频会议（conference）发起/入会/会中控制能力。触发「发起会议」「开个会」「创建会议」且**没有给出具体时间**时，不要构造 `conference` 命令；直接告知用户请在钉钉客户端操作。

