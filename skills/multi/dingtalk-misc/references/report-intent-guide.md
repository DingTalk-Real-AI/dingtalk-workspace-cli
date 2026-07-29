# report 局部意图消歧

本文件从单 Skill `intent-guide.md` 拆分而来，仅保留与本产品相关的跨产品消歧规则。

| 用户说... | 真实意图 | 应该用 | 不要用 | 理由 |
|---|---|---|---|---|
| "帮我看看收到的日报" | 收到的日志 | `report` | `doc` | 钉钉日志系统（日报/周报），不是文档 |
| "帮我创建一个待办提醒" | 个人待办 | `todo` | `report` | 个人任务提醒，不是日志汇报 |
| "把最近几次关于XX的会议汇总成报告" | 按主题汇总多次听记 | #5 generate-topic-report | #7 meeting-followup | #7 是单次会议听记跟进；多次会议按主题汇总属于工作汇报 |
| "整理一下XX项目的所有讨论" | 跨源主题归档 | #5 generate-topic-report | #4 write-doc | #4 侧重单篇文档创作；按主题跨听记/群消息汇总属于工作汇报 |
| "我发给某人的消息/邮件/文档/今天我干了什么" | 搜行为记录 | `aisearch behavior` | `chat` / `mail` / `doc` / `report` | 关注“谁对什么做过什么”，走 behavior；具体 `behavior-type/direction/chat-scope` 抽槽见 `aisearch.md` |
