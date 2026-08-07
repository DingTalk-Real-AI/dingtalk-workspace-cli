# 块级编辑 Golden Route

## 普通路径

先读取最小必要范围并取得稳定 block ID：

```bash
dws doc +fetch --node <DOC_ID> --detail with-ids --scope section --start-block-id <KNOWN_BLOCK_ID> --format json
```

再按意图使用统一更新入口：

```bash
dws doc +update --node <DOC_ID> --command block_replace --block-id <BLOCK_ID> --content "新内容"
dws doc +update --node <DOC_ID> --command block_insert_after --after-block-id <BLOCK_ID> --content "补充内容"
dws doc +update --node <DOC_ID> --command block_delete --block-id <BLOCK_ID>
```

`block-id` 必须来自真实 `+fetch --detail with-ids`、`+review` 或原子 block 列表返回。确认、写入与验证统一由 `+update` 处理，正常成功不追加整篇回读。

## 富结构专家路径

只有需要 shortcut 未公开的 callout、分栏、复杂表格或 JSONML element 参数时：

1. 按需读取 [JSONML schema](format/doc-jsonml-schema.md) 或 [cookbook](format/doc-jsonml-cookbook.md)，不要两者都预加载。
2. 读取精确 `doc block` leaf Schema，确认当前 flags。
3. 用原子 block 命令只改目标块；JSONML update 的 uuid 必须等于目标 block ID。

图片和附件不得手写临时 URL 或 OSS 请求，统一走 [`doc-media.md`](doc-media.md) 的 `+media-insert/+media-download`。删除与覆盖的确认以 Runtime gate 为准，示例不得预填 `--yes`。
