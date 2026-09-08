# Wiki 节点操作

## 创建与内容交接

```bash
dws wiki +node-create --workspace <workspaceId> --name "新文档" --type adoc --format json
```

`--type` 可用 `adoc|axls|able|appt|adraw|amind|folder`，父目录加 `--folder <nodeId>`。创建成功从结果取新 nodeId，并按类型交给 Doc/Sheet/AITable；不要靠同名搜索重新定位。

文件夹回执的 `nodeId` 是目录本体，`folderId` 是它所在的父目录。把子节点放入新目录时传新目录的 `nodeId`；不得把回执的父 `folderId` 当成新目录。

只有空间名称时，先明确范围：组织用 `+space-list --type orgWikiSpace --limit 50 --page-all` 取完后唯一精确匹配；个人用 `dws wiki space list --type myWikiSpace --format json`，核对成功回执恰好一条且精确同名，不要求自动分页字段。取得真实 workspaceId 后再 `+node-create`；0 条、多条、名称不符或响应不完整时停止，不用 `+wiki-new-doc` 的单页名称搜索替代。正文仍切 Doc。

## 本地文件与内容交接

- 知识库根目录的独立普通附件：`dws drive upload --file <本地文件> --workspace <workspaceId> --format json`，不加 `--convert`；子目录加 `--folder <目录nodeId>`。这里用原生 `drive upload`，不能把其 workspace 能力套到 `drive +upload`，也不混传 `--space-id`。
- 已有文档正文里的附件：交给 Doc 媒体插入；其结果是正文 block，不能替代知识库目录中的普通文件。已有本地文件要转换为在线文档：交给 Doc import；用户明确先建空节点再写内容时仍按该步骤执行。
- 按 Runtime 确认后执行；用上传回执的新 nodeId 核对实体类型及 workspace/folder，已有字段足够时直接复用，不足时按该 ID 读取节点详情；不能只凭文件名或 `success=true` 宣称放对位置。转换失败、类型不符或效果未知时保留回执，不自动改成另一种对象或重复上传。

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

- `+node-copy/+move` 的“本库首页”固定为当前 `--workspace` 并省略 `--folder`；只有用户明确指定其他库才换目标。“首页”不能默认解释成个人空间；移出方向用 `+move-to-drive`，不传 `--workspace`，省略 `--folder` 时目标是个人我的文档根。
- copy 产生新 nodeId，必须读回副本；源 ID 不能当新 ID。两份 ID 不同还不够，要核对副本 workspace/folder 是否符合目标；已有回执足够时不重复 list。
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
- 读回与请求不一致时返回结构化失败并保留真实目标信息。已创建的副本仍进入资源账本；副本若在其他容器，删除源知识库不能代替副本清理。未知效果不自动复制重试或删除。
