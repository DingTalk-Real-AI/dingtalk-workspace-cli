---
category: Changed
---

- **数字员工回复复用 chat** — 通用发送与引用回复增加 stdin 正文、投递回执和绑定上下文校验；本地数字员工 worker 使用通用 chat 回传，删除重复的 `channel reply` / `channel operator-private`，配套 DSH 要求 `chatDelivery=true`，缺失时提示升级；双方须配套升级。
- **A2UI 审批卡片兼容** — 发卡命令支持 `--summary` 自定义聊天预览，满足 DSH 数字员工的卡片能力探测；省略时继续使用原有默认摘要。

local_agent 聊天权限统一读取 DEAP 已发布可见范围（成员及部门子树）；其他场景保留本地白名单。DWS/DSH 共用绑定查询判定，固定管理 Profile，查询失败不回退放行。
