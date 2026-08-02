# report Lite Recipe

本文件从单 Skill `lite-recipes.md` 拆分而来，仅保留与本产品相关的轻量流程。

## #5 工作汇报

### query-report-list

1. 收到的日志：先把用户时间词转成起止时间，再执行 `report inbox list --start "<YYYY-MM-DDT00:00:00+08:00>" --end "<YYYY-MM-DDT23:59:59+08:00>" --cursor 0 --size 20 --format json`。用户只说“最近/近期/最近收到”时默认最近 7 天。
2. 我发过/我创建的日志：首条查询必须用 `report outbox list --cursor 0 --size 20 --format json`；如用户指定时间，补 `--start "<YYYY-MM-DDT00:00:00+08:00>" --end "<YYYY-MM-DDT23:59:59+08:00>"`。
3. 按发件人过滤收件箱：先 `aisearch person --keyword "<姓名>" --dimension name --format json` 取 `userId/staffId`，再加 `--sender-user-ids <id>`；空结果必须说明未找到该发件人的日志，不得改选其他人。
4. 面向用户时必须基于 `result[]` 拼 Markdown 表，表头固定为 `日期 | 标题 | 发送人 | 状态 | 钉钉链接`；每条 `result[]` 都会带这五个中文字段，不要把 `reportId` / `日志ID` 作为主列。
5. 用户要正文、详情、统计、汇总或总结多篇日志时，必须用内部保留的 `reportId` 逐篇执行 `report entry get --report-id <reportId> --format json` 或 `report entry stats --report-id <reportId> --format json`；选前 5 篇时调用次数应等于实际选中篇数。

时间 flag 硬约束：只允许 `--start` / `--end`；禁止 `--start-date` / `--end-date` / `--date`。不要只传 `2026-05-04`，必须展开成 `2026-05-04T00:00:00+08:00` 这种完整 ISO；禁止 UTC `Z` / `date -u`。

硬约束：`report inbox list` 是收到的日志（别人发给我），`report outbox list` 是我创建/发出的日志（我发给别人）。不要混淆方向；不要回答"API 不支持收到的日志"。

> 旧命令兼容：`report list` / `report inbox` / `report sent` / `report created` / `report detail` / `report stats` 仍可执行，但已 deprecated，stderr 会打废弃提醒，新计划一律使用 `inbox list` / `outbox list` / `entry get` / `entry stats`。

禁止：不要先查 help，不要为了格式化列表创建脚本；不要传 `--size 50/100`。`report inbox` 可作为兼容入口使用，但新计划优先写规范命令 `report inbox list --start "<YYYY-MM-DDT00:00:00+08:00>" --end "<YYYY-MM-DDT23:59:59+08:00>" --cursor 0 --size 20 --format json`。

### check-report-read-status

`report entry stats --report-id <reportId>` → 已读/未读
