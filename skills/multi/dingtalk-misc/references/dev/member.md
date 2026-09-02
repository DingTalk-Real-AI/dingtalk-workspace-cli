# 应用成员管理

应用成员表示谁能管理该应用，常用成员类型为 `DEVELOPER`。

“某开放平台应用的全部管理成员/按成员角色汇总”就是本域，不是聊天群成员：不要切到 `dws chat`、搜索同名群或用群角色替代应用角色。

```bash
dws dev app member list --unified-app-id <id> --format json
dws dev app member add --unified-app-id <id> --user-ids <staffId1,staffId2> --member-type DEVELOPER --dry-run --format json
dws dev app member remove --unified-app-id <id> --user-ids <staffId1,staffId2> --member-type DEVELOPER --dry-run --format json
```

执行规则：

1. 应用名先用一次 `dws dev app list --name <名> --format json` 定位并复用 ID。
2. 只有姓名时，用 `dingtalk-aisearch` 一次解析到唯一 staffId/userId；重名时展示候选，不猜人。
3. add/remove 的 dry-run 核对无误且用户已明确要求后，改为 `--yes --format json` 正式执行；remove 也必须带原成员类型。
4. 查询“全部”时以一次 `member list` 的 `data.members[]` 完整返回为准；按真实 `memberType` 分组计数并列出成员，字段缺失就标记“角色未返回”，不要猜。写后只回读一次，按目标人员是否存在判断结果。
5. 若后续还要删除应用，先完成并保存用户要求的成员名单与移除结果，再把删除放最后；最终说明该名单是删除前快照。

已知路径不调用 group help；仅 flag 确有疑问时查一次精确 leaf Schema，必要时再退回一次 leaf help。
