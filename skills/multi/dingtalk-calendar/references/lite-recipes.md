# calendar Lite Recipe

本文件从单 Skill `lite-recipes.md` 拆分而来，仅保留与本产品相关的轻量流程。

## #3 会议日程

### list-today-meetings

**优先**：`dws calendar +today|+tomorrow|+week --format json`
任意时段：`dws calendar +agenda --start "<起始ISO>" --end "<结束ISO>" --format json`

### check-users-busy

查询多人在某时段内的闲忙（**busy**，不是用 `event list` 扫日程）：

1. 确认时段：用户须给出或可收敛为明确的 `--start` / `--end`（ISO-8601）；若未给出，先追问起止时间。
2. 按姓名查一人：`dws calendar +free --who "<姓名>" --start "<ISO>" --end "<ISO>" --format json`。
3. 多人共同时间：`dws calendar +suggest-time --with "张三,李四" --start "<ISO>" --end "<ISO>" --format json`。
4. 已有 userId/roomId：`dws calendar +freebusy --users <userIds> --rooms <roomIds> --start "<ISO>" --end "<ISO>" --format json`；users/rooms 至少一类。

详见 [calendar.md](./calendar.md) 中「查询用户闲忙状态」。

### start-conference

> 当前 CLI 不提供视频会议（conference）发起/入会/会中控制能力。触发「发起会议」「开个会」「创建会议」且**没有给出具体时间**时，不要构造 `conference` 命令；直接告知用户请在钉钉客户端操作。
