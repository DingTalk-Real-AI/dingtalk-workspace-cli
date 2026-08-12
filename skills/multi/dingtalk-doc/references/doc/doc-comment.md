# 文档评论 Golden Route

## Review 聚合

```bash
dws doc +review --node <DOC_ID_OR_URL> --format json
```

需要一次看到未解决评论、引用原文和 block 上下文时优先使用 `+review`。从真实返回取得 `commentKey` 和 `blockId`，禁止按数组位置或猜测 ID。

## 精确评论动作

```bash
dws doc +comment-list --node <DOC_ID> --resolve-status unresolved --limit 20 --format json
dws doc +comment-list --node <DOC_ID> --limit 20 --cursor <NEXT_TOKEN> --format json
dws doc +comment-create --node <DOC_ID> --content "这里需要补充证据"
dws doc +comment-create --node <DOC_ID> --selection "计划下周发布" --content "请确认日期"
dws doc +comment-reply --node <DOC_ID> --comment-key <COMMENT_KEY> --content "已补充"
dws doc +comment-update --node <DOC_ID> --comment-key <COMMENT_KEY> --content "修订后的意见"
dws doc +comment-delete --node <DOC_ID> --comment-key <COMMENT_KEY>
```

- 全文与划词评论统一使用公开的 `+comment-create`：优先传唯一 `--selection`；已知真实 block 时传 `--block-id --start --end`，CLI 自动回读并校验 `selectedText`。不要选用未公开的 `+comment-create-inline`。
- `--mention` 传单个 uid 或逗号分隔列表，例如 `--mention 550582,123456`；不要传 JSON 数组。
- 列表用 `--limit` 控制页大小（兼容 `--page-size`），有 `nextToken` 时原样传给 `--cursor`；不能把单页当作全部评论。
- 创建、回复、删除等写操作执行前消费 leaf Schema `confirmation`；需确认时先询问，示例不得预填 `--yes`。
- 删除不可恢复，必须核对 node 与 commentKey。部分或未知结果不得自动重试。

只有 shortcut 未公开必要字段时，才读取精确原子 leaf Schema；不要加载整份评论参考或产品 Catalog 来猜参数。
