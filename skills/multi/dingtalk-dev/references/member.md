# 成员管理

> 成员=谁能改这个应用（DEVELOPER 等角色）；见 SKILL.md 概念地图。

`dws dev app member list/add/remove` 管理应用成员。参数查 `dws schema dev.app.member.<method>`（add/remove 需 `--user-ids` 列表 + `--member-type`，如 DEVELOPER；remove 也必须传 memberType，因为同一用户可能有多个成员身份）。
