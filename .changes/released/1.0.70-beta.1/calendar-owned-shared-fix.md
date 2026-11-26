---
category: Fixed
---

- **邮箱日历默认查询** — 修复个人日程脚本将 `type=shared`、`privilege=owner` 的本人邮箱日历排除的问题。mono/multi Skill 默认通过 calendar 查询本人拥有的 primary/shared 日历，继续排除订阅、团队、公共日历及仅有读写权限的他人日历，并同步范围说明与回归用例；CLI、Schema 和脚本输出结构保持兼容。
