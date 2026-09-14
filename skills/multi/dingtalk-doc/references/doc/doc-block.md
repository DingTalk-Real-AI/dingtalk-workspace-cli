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

## 标题块例外

新增 heading 必须使用结构化插入，不能把 `# 标题` 作为 Markdown 传给 `block_replace`。例如在首块前新增一级标题：

```bash
dws doc block insert --node <DOC_ID> --heading "发布说明 v1.0" --level 1 --ref-block <FIRST_BLOCK_ID> --where before --format json
```

插入回执有稳定新 block ID 时，用 `dws doc block list --node <DOC_ID> --block-id <NEW_BLOCK_ID> --format json` 定点验证；回执只有插入 index 时，只执行一次 `dws doc block list --node <DOC_ID> --content-format jsonml --format json`，按该 index 验证 `blockType=heading`、`heading.level="heading-1"` 和标题文字。`block list` 没有 `--limit`，禁止 Help/试错；一次完整 JSONML 列表已满足结构验收时立即终止。CLI 写入仍使用数值参数 `--level 1`。修改现有标题才使用 `doc block update --block-id ... --heading ... --level ...`；不要用普通文字替换改变块类型。

## 富结构专家路径

只有需要 shortcut 未公开的 callout、分栏、复杂表格或 JSONML element 参数时：

1. 按需读取 [JSONML schema](format/doc-jsonml-schema.md) 或 [cookbook](format/doc-jsonml-cookbook.md)，不要两者都预加载；用户已明确结构时优先 cookbook 的可执行样例。
2. 读取精确 `doc block` leaf Schema，确认当前 flags。
3. 用原子 block 命令只改目标块；JSONML update 的 uuid 必须等于目标 block ID。

图片和附件不得手写临时 URL 或 OSS 请求，统一走 [`doc-media.md`](doc-media.md) 的 `+media-insert/+media-download`。删除与覆盖的确认以 Runtime gate 为准，示例不得预填 `--yes`。
