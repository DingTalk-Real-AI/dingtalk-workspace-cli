# 应用权限管理

权限点以 `scopeValue` 为稳定标识。查询、申请和取消都走 `dws dev app permission`。

## 最短命令路径

```bash
# 按名称/API/权限点定位；单权限可直接用 --scope-value
dws dev app permission list --unified-app-id <id> --keyword <关键词> --page-size 50 --format json
dws dev app permission list --unified-app-id <id> --scope-value <scopeValue> --format json

dws dev app permission add --unified-app-id <id> --scope-values <scope1,scope2> --dry-run --format json
dws dev app permission remove --unified-app-id <id> --scope-values <scope1,scope2> --dry-run --format json
```

最初请求只允许执行 dry-run。展示预检返回的应用 ID、add/remove 动作、完整 `scopeValue` 列表及影响，等用户对该预览明确确认后，才把同一命令仅由 `--dry-run` 换成 `--yes`；目标或权限列表变化就重新预检并确认，确认前不得真实增删权限。写后只回读目标 `scopeValue`。

分页只有 `--cursor`：读取 `meta.pagination`，`endpoint_exhausted=false` 时把原样 `next_token` 传给下一次 `--cursor`，直到 true。CLI 不支持 `--page` 或 `--page-num`；权限记录在 `data.items[]`。

## 选择与状态

- 选择顺序：用户给的 `scopeValue` → API 名匹配 `apiPreview.name` → 权限名匹配 `scopeName/scopeDesc`。多候选时让用户选，不取第一条。
- `authed=true` 已开通，跳过重复 add；`allowedActions` 含 `apply/remove` 才能执行对应动作。
- `requiredApproval=true` 的变更会进入版本通道；权限写入成功后按 [`version.md`](./version.md) 创建并发布版本。
- remove 逐条核对 `removedScopeValues/rejectedScopeValues`，不要只看整体布尔值。
- 用户只说“去掉一个多余权限”却未给唯一标识时，不得为了猜目标拉取 150+ 全量：仅当本轮上下文已给出候选名称/scope 时做一次关键词定向查询，否则直接请求一个名称或 `scopeValue`。暂停 remove，但继续其它独立查询和可安全步骤；不要自行猜测并删除。
- 目标 add 已是 `authed=true` 时说明无需重复申请。没有任何权限写入成功时，不创建/发布版本；缺少 remove 选择也不能用发布空版本替代业务变更。
- 仅用户要求全部权限时才翻到 `meta.pagination.endpoint_exhausted=true`；普通关键词查询不要拉取 150+ 全量。最终给匹配数和相关项，避免整段 JSON。

已知路径不跑 group help；只有 flag 确有疑问时查一次精确 leaf compact Schema，Schema 不可用才查一次 leaf help。
