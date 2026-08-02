# todo 局部意图消歧

本文件从单 Skill `intent-guide.md` 拆分而来，仅保留与本产品相关的跨产品消歧规则。

| 用户说... | 真实意图 | 应该用 | 不要用 | 理由 |
|---|---|---|---|---|
| "帮我记一下明天要做的事" | 创建个人待办 | `todo` | `doc` | 个人待办提醒，非文档内容 |
| "给自己留一个明天下午的时间块/建个个人日程" | 创建个人日程 | `calendar event create` | `todo` | 个人 schedule 仍属于日历事件，不是待办 |
| "明早 9 点提醒我提交周报" | 创建个人待办，但需先声明 reminder 边界 | `todo` | `calendar` | todo 当前只支持 dueTime 截止时间，不支持独立精确 reminder |
| "帮我创建一个待办提醒" | 个人待办 | `todo` | `report` | 个人任务提醒，不是日志汇报 |
| "把最近几次关于XX的会议汇总成报告" | 按主题汇总多次听记 | #5 generate-topic-report | #7 meeting-followup | #7 是单次会议听记跟进；多次会议按主题汇总属于工作汇报 |
