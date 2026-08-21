# Wiki 节点操作

## 创建与内容交接

```bash
dws wiki +node-create --workspace <workspaceId> --name "新文档" --type adoc --format json
```

`--type` 可用 `adoc|axls|able|appt|adraw|amind|folder`，父目录加 `--folder <nodeId>`。创建成功从结果取新 nodeId，并按类型交给 Doc/Sheet/AITable；不要靠同名搜索重新定位。

只有空间名称时，先明确组织/个人范围，用 `+space-list --type <orgWikiSpace|myWikiSpace> --limit 50 --page-all` 取完并按完整名称唯一匹配；再将真实 workspaceId 传给 `+node-create`。当前不要用 `+wiki-new-doc` 直接写入，因为其内部名称搜索不暴露分页完成证据。正文仍切 Doc。

## 读取、搜索与列表

- 已知 nodeId/URL：`+node-get --node <值>`。
- 浏览目录：`+node-list --workspace <ID> [--folder <ID>]`；全量加 `--page-all`。
- 关键词检索：`+node-search --workspace <ID> --query <词>`；不拿 list 代替 search。

## 复制与移动

```bash
dws wiki +node-copy --workspace <目标库ID> --node <源nodeId> [--folder <目标folderId>]
dws wiki +move --workspace <目标库ID> --node <nodeId> [--folder <目标folderId>]
dws wiki +move-to-drive --node <nodeId> [--folder <我的文档folderId>]
```

- copy 产生新 nodeId，必须读回副本；源 ID 不能当新 ID。
- move 保持 nodeId，但必须验证目标 workspace/folder。
- move-to-drive 必须验证 workspace 已改变；workspaceId 不能当普通 folderId。
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
