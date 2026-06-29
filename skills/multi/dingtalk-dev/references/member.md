# 成员管理

> 成员=谁能改这个应用（DEVELOPER 等角色）；见 SKILL.md 概念地图。

`dws dev app member list/add/remove` 管理应用成员。参数查 `dws schema dev.app.member.<method>`（add/remove 需 `--user-ids` 列表 + `--member-type`，如 DEVELOPER；remove 也必须传 memberType，因为同一用户可能有多个成员身份）。

## 发现命令

调用任何方法前先查清楚再敲：

```
# 浏览命令组下的子命令与 flag
dws dev app member --help

# 查某方法的必填参数、类型、默认值
dws schema dev.app.member.<method>
```

按 `dws schema` 输出构造 `--flag`（flag 名 = schema 参数名）。
