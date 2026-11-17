---
category: Added
---

- `dws sport competition` 新增 9 个只读命令，沿用 helpers 手写注册与隐藏的 misc 长尾入口：`list`（企业启用中的比赛）、`rank-top`（TopN 榜，支持 TOTAL 本期总榜）、`ranking`（全员分页排行榜）、`my-list` / `my-can-join-count`（我的比赛与可参与数量）、`my-participated` / `my-participated-count`（我参与过的期次与总数）、`activity-detail`（活动详情与我的报名状态）、`my-achievement`（我的达标进度）。同批材料中已接入的配置、期次与统计工具未重复注册。
- 分页列表只取一页：源 `hasMore` / `nextCursor` 转为 `meta.pagination.endpoint_exhausted` / `next_token`，Long 游标保持精度，空页不当作结束；个人列表与全员排行首页发送 `cursor=null`，企业列表首页为 `0`。缺少 `success`、分页字段、非法计数或声明有下一页却无有效游标的回执均按错误返回，不当成空结果。
- 企业与当前用户身份由平台注入，新命令不接受 `--user-id` / `--corp-id`；同步 Help、Agent Schema 与 mono/misc 命令参考。
