# Wiki 成员管理

## 入口

```bash
dws wiki +member-list --workspace <workspaceId> --limit 30 --format json
dws wiki +member-add --workspace <workspaceId> --users <userId> --role READER --format json
dws wiki +member-update --workspace <workspaceId> --users <userId> --role EDITOR --format json
dws wiki +member-remove --workspace <workspaceId> --users <userId> --format json
```

- `--users` 接受 1-30 个真实 userId；姓名先由联系人/人员搜索产品解析，不能直接当 userId。
- 角色仅 `MANAGER|EDITOR|DOWNLOADER|READER`，修改前必须由用户明确角色。
- workspaceId 来自当前 profile 下真实知识库；不能跨组织复用。
- 成员接口仅用于组织知识库。`myWikiSpace` 是个人空间，不支持容器级成员管理；若用户只想分享其中某个节点，改走 Drive 节点级权限。
- `OWNER` 不在成员角色枚举中，不能通过 add/update/remove 创建、降级或移除所有者；所有权变更必须走对应所有者转移能力并遵守其独立确认约束。
- 调用者需要知识库 OWNER 或 MANAGER 权限；权限不足时如实返回，不切账号或改用节点权限绕过容器规则。

## 列表完整性

成员列表服务端单次上限为 50（`--limit` / pageSize 最大 50）。快捷命令 `+member-list` 适合单页列表，但**不支持游标翻页**；超过 50 人必须改用原生 `dws wiki member list` 的 `--next-token` 翻页：首次调用不传，后续把上一次响应中的 `nextToken` 传入 `--next-token`，直到 `hasMore` 为 false。出参 `totalCount` 为全量成员总数，可用于核对是否拉全。不要伪造 `--page-all` 或不断提高 limit 超过 50。

```bash
dws wiki member list --workspace <workspaceId> --limit 50 --format json
dws wiki member list --workspace <workspaceId> --next-token <上次返回的 nextToken> --format json
```

## 写入验证

成员 add/update/remove 当前只能以写接口 `success=true` 作为终态证据，并显式返回 `verification.status=terminal_response_only`；成员列表受上限限制，不能被包装成精确读回验证。

若 add/update 写响应丢失，先用 `dws wiki +member-list --workspace <workspaceId> --filter-role <MANAGER|EDITOR|DOWNLOADER|READER> --limit 50 --format json` 做有限核对；remove 按写前已知的原角色过滤，原角色不确定时省略 `--filter-role`。成员列表仍不能证明完整结果；无法证明时报告未知效果，不自动重放批量成员写入。
