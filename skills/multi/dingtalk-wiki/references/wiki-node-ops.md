# Wiki 节点操作

## 创建与内容交接

```bash
dws wiki +node-create --workspace <workspaceId> --name "新文档" --type adoc --format json
```

`--type` 可用 `adoc|axls|able|appt|adraw|amind|folder`，父目录加 `--folder <nodeId>`。创建成功只有在读回的 ID、workspace、名称、类型和显式父文件夹都一致时成立；从结果取新 nodeId，并按类型交给 Doc/Sheet/AITable，不要靠同名搜索重新定位。

只有空间名称时，先明确组织/个人范围，用 `+space-list --type <orgWikiSpace|myWikiSpace> --limit 50 --page-all` 取完并按完整名称唯一匹配；再将真实 workspaceId 传给 `+node-create`。当前不要用 `+wiki-new-doc` 直接写入，因为其内部名称搜索不暴露分页完成证据。正文仍切 Doc。

## 读取、搜索与列表

- 已知 nodeId/URL：`+node-get --node <值>`。
- 浏览目录：`+node-list --workspace <ID> [--folder <ID>]`；全量加 `--page-all`。
- 关键词检索：`+node-search --workspace <ID> --query <词>`；需要完整命中集时加 `--page-all` 并检查 `autoPageComplete`，不拿 list 代替 search。
- 节点结果中的 `extension/type/hasChildren/parentFolderId` 来自服务端或规范化别名；缺少字段时停止推断，不能仅凭名称宣称文件、文件夹或父子关系。

## 复制与移动

```bash
dws wiki +node-copy --workspace <目标库ID> --node <源nodeId> [--folder <目标folderId>]
dws wiki +move --workspace <目标库ID> --node <nodeId> [--folder <目标folderId>]
dws wiki +move-to-drive --node <nodeId> [--workspace <来源知识库ID>] [--folder <我的文档folderId>]
```

- copy 先读回源节点，再产生新 nodeId 并读回副本；新 ID 必须不同于源 ID，副本 workspace/folder 必须等于请求目标，结果使用 `source/copy` 直接报告两份位置。
- move 保持 nodeId，但必须验证目标 workspace/folder；结果使用 `source/target` 报告移动前后位置。
- move-to-drive 可用 `--workspace` 断言来源知识库，预检归属不一致会在写前停止；移动后除验证 workspace 已改变外，还必须确认目标 workspace 出现在 `myWikiSpace` 范围列表。结果中的 `targetDomain=my_documents`、`sourceWorkspaceId/targetWorkspaceId` 和 `targetSpace` 是位置证据。只要源是 Wiki workspace 中的在线节点且目标是“我的文档”，固定使用此入口；不要查 `mySpace/rootFolderId` 后改用 `drive +move`。workspaceId 不能当普通 folderId。
- 这些命令按 Runtime confirmation 执行；dry-run 与正式执行使用同一目标。

## 删除

```bash
dws wiki +node-delete --workspace <workspaceId> --node <nodeId>
```

删除前读取节点并核对其 workspace；不匹配立即停止。确认后只接受 `success=true`，不因列表暂时未刷新而重复删除。

## 恢复原则

- create/copy 缺少新 ID：提交效果未知，先按目标库和精确名称定向查询。
- move 响应异常：按原 nodeId 读回 workspace/folder 决定终态。
- delete 响应异常：检查节点详情或回收状态；不能盲重放。
- 读回与请求不一致时返回结构化失败并保留真实目标信息。
