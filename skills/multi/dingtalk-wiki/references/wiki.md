# Wiki 空间、分页与动态参考

只在根 Skill 的 Golden Route 不足时读取相关章节。节点写操作和成员管理分别读取对应 operation reference。

## 空间定位

| 已知条件 | 入口 |
|---|---|
| 精确 workspaceId 或知识库 URL | `+space-get --workspace <值>` |
| 组织知识库名称且必须得到唯一 ID | `+space-list --type orgWikiSpace --limit 50 --page-all` 后唯一精确匹配 |
| 个人知识空间入口或名称核对 | `dws wiki space list --type myWikiSpace --format json`，核对实际单例回执 |
| 关键词且需要浏览候选 | `+space-search --query <关键词>` |
| 明确要列出组织知识库 | `+space-list --type orgWikiSpace`；全量加 `--page-all` |

名称解析前先明确组织/个人范围，未知时先消歧。组织列表须 `autoPageComplete=true` 且全量恰好一个名称精确相等的空间；个人原生入口须成功回执中恰好一条且名称精确相等，不要求 `autoPageComplete` 等自动分页字段。只有满足对应条件才把真实 workspaceId 交给后续写操作；空/畸形响应、0 条、多条、名称不符或组织分页未完成都停止，不选择第一项。

`+space-search` 用于候选发现，但当前没有可执行的续页 flag；`+resolve-space` 和 `+wiki-new-doc` 的内部名称搜索也不暴露分页完成证据。因此这三个入口的单页结果都不能证明全局唯一，当前不用于写入前的权威身份解析。空关键词不能构造 `--query ""`。

## Runtime 确认与首次执行

- 先唯一解析 workspace、node、目标父节点或成员，再以精确 leaf Schema 和 Runtime gate 判断是否需要确认；不要通过一次缺少 `--yes` 的失败调用探测确认要求。
- Runtime 要求确认且当前请求已明确授权具体空间/节点/成员、动作、目标位置或角色及影响时，首次正式远端写调用直接追加 `--yes`。只有“整理知识库”“调整成员”等宽泛意图时必须先补齐范围。
- Runtime 不要求确认时不添加 `--yes`。需要询问时，确认后必须保持同一 profile、对象、动作、范围和关键参数；移动目标、成员列表、角色或其他影响范围变化时重新确认。
- 只读定位和检查不加 `--yes`。收到 `confirmation_required` 仅表示尚未通过预执行门禁，不代表业务写入成功，也不能据此盲目重放。

## 全量分页

`+space-list`、`+node-list`、`+feed-list` 支持自动翻页：

```bash
dws wiki +space-list --page-all --page-limit 20 --format json
dws wiki +space-list --type orgWikiSpace --limit 50 --page-all --format json
dws wiki +node-list --workspace <ID> --page-all --max-items 500 --format json
dws wiki +feed-list --workspace <ID> --page-all --format json
```

- `--page-all` 才启用自动翻页；`--max-items/--page-delay` 必须配合它使用，`--page-limit` 只在自动翻页时生效。
- `autoPageComplete=true` 且 endpoint exhausted 才表示端点取完。
- 达到 `--max-items` 返回部分结果，标记 `autoPageComplete=false`、`autoPageStopReason=max_items`，不能宣称全量。
- 达到 `--page-limit` 返回 `page_limit_reached` 错误，不返回累计结果或续页游标；需要继续时提高页数上限重新查询。
- 游标缺失、停滞、循环或后续页失败均不能返回“全部”。
- `+node-search` 默认保留服务端单页 cursor；需要完整命中集时加 `--page-all`，由 Shortcut 沿真实 cursor 自动续页。游标缺失、停滞、循环或后续页失败均不能报告完整结果。

## 空间创建与删除

```bash
dws wiki +space-create --name "产品文档库" --desc "团队资料" --format json
dws wiki +delete-space --workspace <workspaceId> --format json
```

- 名称最多 32 字符，描述最多 500 字符；本地校验失败不能产生远端调用。
- 创建必须返回 workspaceId，并通过 `get_wikiSpace` 读回同一 ID。
- 删除整个知识库前先读取目标，经 Runtime 确认后只接受 `success=true`。

## 知识库动态

```bash
dws wiki +feed-list --workspace <workspaceId> --limit 20 --format json
```

动态用于回答“谁在何时创建、更新或评论了什么”。需要排除普通文件时加 `--exclude-file`；需要完整范围时加 `--page-all`。节点正文或历史版本不从 feed 推断，锁定 nodeId 后切 Doc。

需要 CLI 提供可读时间和动作标签时，用 `wiki feed list --format json`；`+feed-list` 不主动补齐 `timeFormatted/typeLabel`。缺失或未知字段不猜测，也不为同一结果默认重复查询。

只有用户明确要求核对普通文件动态是否被过滤时，才比较同一知识库、同一查询范围的未过滤和过滤结果，并检查分页完整性。未过滤结果中找到该文件的真实事件、过滤结果中没有它，才能报告本次对比中该事件被过滤；两边都未找到时报告“本轮未观测到该事件，无法验证过滤效果”。普通查看动态无需额外执行这组比较。

正文附件 block 的变更属于文档事件，不能充当独立普通文件正样本。事件身份和动作按实际返回的可解释字段说明，不自行给数字 type 编动作；仍无事件或分页未完成时，保留观察限制，不猜结论。

## 响应与错误

- 空响应、`success=false`、畸形 wrapper、缺失业务数组都属于失败，不是零条数据。
- 集合结果检查 `count` 与对应 `spaces/nodes/feeds`；分页检查 `nextCursor/hasMore` 及 meta.pagination。
- 读写始终保持同一 profile。权限失败不能通过切换账号或退回 Drive 猜测解决。
- 写入提交后连接中断时先用返回/已知 ID 读取；无法证明是否提交则报告未知效果，不自动重试非幂等创建。

## 空间分享范围

`dws wiki permission set-share-scope` 设置整个知识库空间的分享范围（可见性）。[危险] 操作，必须经用户确认后加 `--yes`。

```bash
dws wiki permission set-share-scope --workspace <workspaceId> --visibility ORGANIZATION --role READER --format json
dws wiki permission set-share-scope --workspace <workspaceId> --visibility PUBLIC --role DOWNLOADER --format json
dws wiki permission set-share-scope --workspace <workspaceId> --visibility PRIVATE --format json
```

必填 `--workspace` + `--visibility`（PRIVATE/ORGANIZATION/PUBLIC）。档位决定可用参数：
- PRIVATE — 无需其他参数。❗ 关闭后果：空间下所有未打断继承的节点一并恢复为仅成员可见，且不保留各节点关闭前的原有档位（节点档位不会自动恢复）；空间本身如需恢复公开，须重新执行本命令设置。
- ORGANIZATION — 可附带 `--role`（READER/DOWNLOADER/EDITOR）、`--partner`、`--can-search`、`--can-recommend`。
- PUBLIC — `--role` 仅接受 READER/DOWNLOADER（传 EDITOR 本地拒绝）；空间级不支持密码保护与有效期设置。

不适用组合本地拒绝（exit 3）。参数装配为部分更新语义：可选参数不传即不下发、保持原值不变。布尔三态（`--partner`/`--can-search`/`--can-recommend`）：未设不下发，显式 `--xxx=false` 也会下发。只传 `--can-search` 或 `--can-recommend` 其一时，另一项由服务端联动跟随同值。

操作者必须具备知识库的 MANAGER 或 OWNER 角色，否则返回 `forbidden.accessDenied`。当组织开启公开分享审批策略时，PUBLIC 可能进入待审批状态（`pendingApproval=true`）。

返回体已回显写入后的 `visibility`/`defaultRole`/`partnerIncluded`/`canSearch`/`canRecommend`/`spaceUrl` 等字段，无需额外查询命令。

节点级分享范围用 `dws drive permission set-share-scope`（见 dingtalk-drive）；管理知识库成员用 `wiki member`，不要用分享范围。
