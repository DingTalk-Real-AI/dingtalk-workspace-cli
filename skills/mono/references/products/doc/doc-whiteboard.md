# doc whiteboard（白板卡片）

管理钉钉文档中的白板卡片（hetu draw card）：插入空白板并获取白板资源 ID。
删除白板卡片无专用命令，与普通块一致走 `dws doc block delete`（见下文）。

> 白板卡片本体由 CLI 内部固定模板生成（无需也不能传 `--element`）：
>
> ```json
> ["card",
>  {"uuid":"<blockId>","cardType":"hetu","height":600,
>   "metadata":{"type":"application/x-alidocs-plugin-draw","id":"<whiteboardId>"}},
>  ["span",{"data-type":"text"},["span",{"data-type":"leaf"},""]]]
> ```
>
> `blockId` 与 `whiteboardId`（`metadata.id`，即白板 part id）均由 CLI 生成：
> 线上环境对缺 `metadata.id` 的 hetu card 不会自动建白板资源也不回填 id
> （预发已支持自动补齐，但客户端自带 id 可同时兼容两环境），
> 因此手写 card JSONML 插入白板时务必自行提供 `metadata.id`（推荐直接用本命令）。

---

## doc whiteboard insert（插入白板卡片）

```
Usage:
  dws doc whiteboard insert [flags]
Example:
  # 在文档末尾插入白板
  dws doc whiteboard insert --node <DOC_ID>

  # 在指定块后面插入白板
  dws doc whiteboard insert --node <DOC_ID> --ref-block <BLOCK_ID> --where after

  # 在容器（分栏/列表）内索引 2 位置插入
  dws doc whiteboard insert --node <DOC_ID> --parent-block <CONTAINER_ID> --index 2
Flags:
      --node string           文档 ID 或 URL (必填)
      --ref-block string      参考块 ID，配合 --where 在同级插入 (与 --parent-block 互斥)
      --where string          同级插入位置: before/after (需 --ref-block)
      --parent-block string   父容器块 ID，配合 --index 在容器内插入 (与 --ref-block 互斥)
      --index int             容器内插入索引 (从 0 开始，需 --parent-block)
```

**行为**：

- 生成新白板卡片（`blockId` + `whiteboardId`）并插入文档
- 插入后自动回查块 JSONML 验证 `metadata.id` 已落库
- 返回 JSON：`{"blockId":"<uuid>","whiteboardId":"<uuid>"}`

**定位方式（互斥）**：

- `--ref-block` + `--where`（`before`/`after`）：同级插入（默认 `after`）
- `--parent-block` + `--index`：容器内插入（分栏/callout/列表等）
- 两组都不传：插入到文档末尾

**回查验证**：

插入后自动轮询 3 次（0.5s / 1s / 2s），确认 `metadata.id` 已持久化：

- 成功：返回 `{"blockId":"<uuid>","whiteboardId":"<uuid>"}`
- 回查失败但块已插入：`whiteboardId` 为 `null`，并提示手动回查命令
- 回查本身报错（鉴权/MCP 错误）：命令报错，错误信息中带出已插入的 `blockId`

**后续操作**：

- 编辑白板内容：`dws whiteboard query/update --node <DOC_ID> --part-id <whiteboardId>`
- 删除白板卡片：`dws doc block delete --node <DOC_ID> --block-id <blockId>`

---

## 删除白板卡片（doc block delete）

白板卡片是文档块的一种，删除使用通用块删除命令：

```bash
dws doc block delete --node <DOC_ID> --block-id <BLOCK_ID>
```

推荐先 `doc block list --content-format jsonml --block-id <BLOCK_ID>` 确认块类型为 hetu card
（`cardType=hetu` 且 `metadata.type=application/x-alidocs-plugin-draw`），避免误删普通内容块。

---

## 关键说明

- **whiteboardId vs blockId**：`blockId` 是卡片在文档中的块 UUID；`whiteboardId` 是白板资源本身的 ID（`metadata.id` / part id），两者用途不同、不可混用。
- **--node 隐藏别名**：与其他 doc 子命令一致，支持 `--url` / `--id` / `--node-id` / `--doc-id` / `--file-id`。
- 白板内容的编辑 / 导出不在本命令范围，使用 `dws whiteboard query/update`（参见 [whiteboard.md](../whiteboard.md)）。
