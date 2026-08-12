# 读取文档：`+fetch` Golden Route

## 唯一推荐入口

```bash
dws doc +fetch --node <DOC_ID_OR_URL> --format json
dws doc +fetch --query "项目周报" --scope keyword --keyword "结论" --format json
```

- 已知 ID 或 URL：传 `--node`。
- 只知道标题：传 `--query`；跨页解析必须唯一命中，否则停止并要求用户选择。
- `--node` 与 `--query` 必须且只能提供一个。
- 默认 `--detail simple --scope full`，适合普通阅读，避免加载不必要的 JSONML。

## 局部读取

```bash
dws doc +fetch --node <DOC_ID> --scope outline --detail with-ids --format json
dws doc +fetch --node <DOC_ID> --scope section --start-block-id <BLOCK_ID> --detail full --format json
dws doc +fetch --node <DOC_ID> --scope range --start-block-id <A> --end-block-id <B> --detail full --format json
dws doc +fetch --node <DOC_ID> --scope tags --tags table,img --detail full --format json
dws doc +fetch --node <DOC_ID> --scope keyword --keyword "风险|结论" --context-before 120 --context-after 240 --format json
```

只有需要块 ID、revision 或 JSONML 保真结构时才提高 `--detail`；先读取最小必要范围，避免把整篇大文档放入上下文。

## 后续路由

- 读后修改：把稳定的 `nodeId` 交给 [`doc-update.md`](doc-update.md) 的 `+update` 或 `+checkpoint-update`。
- 附件/图片：先用 [`doc-media.md`](doc-media.md) 的 `+media-list` 取得稳定 `resourceId`，再下载；不要复用正文中的临时签名 URL。
- 富结构专家编辑：确实需要原始 JSONML 或 shortcut 未公开的参数时，先读取精确 leaf Schema，再使用原子 `doc read`/`doc block`。

禁止把原子 `doc read` 当作默认入口，也不要在读取后无条件整篇回写。筛选结果只用于读取，不能把虚拟 fragment 容器整体写回文档。
