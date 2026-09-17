# Wiki 成员管理

## 入口

```bash
dws wiki member list --workspace <workspaceId> --limit 30 --format json
dws wiki +member-add --workspace <workspaceId> --users <userId> --role READER --format json
dws wiki +member-update --workspace <workspaceId> --users <userId> --role EDITOR --format json
dws wiki +member-remove --workspace <workspaceId> --users <userId> --format json
```

- `--users` 接受 1-30 个真实 userId；姓名先由联系人/人员搜索产品解析，不能直接当 userId。
- 角色仅 `MANAGER|EDITOR|DOWNLOADER|READER`，修改前必须由用户明确角色。
- workspaceId 来自当前 profile 下真实知识库；不能跨组织复用。
- 成员接口仅用于组织知识库。`myWikiSpace` 是个人空间，不支持容器级成员管理；若用户只想分享其中某个节点，改走 Drive 节点级权限。
- `OWNER` 不在成员写入角色枚举中，不能通过 add/update/remove 创建、降级或移除所有者；所有权变更必须走对应所有者转移能力并遵守其独立确认约束。
- 调用者须满足知识库配置的最低权限角色；权限不足时如实返回，不切账号或改用节点权限绕过容器规则。
- 部门、群聊、角色组、逐成员角色或通知需求，使用原生 `wiki member` 的 `add/update/remove`，传 `--members` JSON 数组，不与 `--users` 混用。
- 每项须有 `type/id`；`USER`、`DEPT`、`TAG` 还需 `corpId`。添加/修改每项带 `roleId`，移除不带角色，每批最多 30 项。
- 添加/修改需要通知时显式传 `--notify`，仅 `USER`、`CONVERSATION` 生效；移除无此参数。

## 列表完整性

单页浏览、全量名单和指定成员核对使用原生 `dws wiki member list`，`--limit` 默认 30、最大 50。首次不传 `--next-token`；`hasMore=true` 时按返回的 `nextToken` 续页，保持 workspace、profile 和筛选不变。`+member-list` 没有续页入口；不使用 `--page-all` 或超过 50 的 limit。

只有明确 `hasMore=false` 且已获取所有前页，才能报告端点取完；若返回 `totalCount`，还要核对同一筛选范围的累计条数。缺失/重复 token、后页失败或计数不一致时保留部分结果并停止，不宣称全量。ORG 授权不在此列表中，完整名单也不能证明用户没有经组织/其他主体继承的访问权限。

```bash
dws wiki member list --workspace <workspaceId> --limit 30 --format json
dws wiki member list --workspace <workspaceId> --limit 30 --next-token <上次返回的nextToken> --format json
```

## 写入验证

当前 `+member-add/+member-update/+member-remove` 的写回执仍以 `success=true` 和 `verification.status=terminal_response_only` 表示终态；后续原生列表核对是另一条观察，不能改写回执为命令已内置读回。

核对指定人员时，用前序真实 `userId` 匹配返回的 `userId`，或明确 `type=USER` 的 `id`，有组织字段时一并核对；任意主体的 `id`、姓名或角色相同都不能替代身份匹配。add/update 命中该人且角色匹配可报告本次观察；单页未见不等于不存在，按上述游标续页。remove 核对直接 USER 授权是否消失时不筛角色，取完后才报告可见名单中不存在，不能声称所有继承权限均撤销。

写响应丢失时先作上述只读核对；无法证明、遇到 OWNER/自身目标冲突或权限错误时停止，保留未知效果，不换人演示、不自动重放成员写入。
