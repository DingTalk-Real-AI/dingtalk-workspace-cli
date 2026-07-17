# attendance 局部意图消歧

本文件从单 Skill `intent-guide.md` 拆分而来，仅保留与本产品相关的跨产品消歧规则。

| 用户说... | 真实意图 | 应该用 | 不要用 | 理由 |
|---|---|---|---|---|
| "查/提交 请假/加班/外出/出差/补卡 审批单" | 考勤业务审批单 | `attendance approve`（查询走 `attendance approve list`；提交走 `attendance approve templates --type leave\ | overtime\ | repair-check\ |
