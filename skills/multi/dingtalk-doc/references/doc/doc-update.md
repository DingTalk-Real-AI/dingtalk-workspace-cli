# 更新在线文字文档

## 唯一推荐入口

普通追加、覆盖和 block 编辑统一使用 `+update`：

```bash
dws doc +update --node <DOC_ID> --command append --content "补充说明" --format json
dws doc +update --node <DOC_ID> --command append --content @append.md --format json
dws doc +update --node <DOC_ID> --command overwrite --content @full.md --format json
dws doc +update --node <DOC_ID> --command overwrite --doc-format jsonml --content @full.json --expected-revision <REVISION> --format json
dws doc +update --node <DOC_ID> --command block_replace --block-id <BLOCK_ID> --content "新内容" --format json
```

重要覆盖或明确要求恢复点时使用：

```bash
dws doc +checkpoint-update --node <DOC_ID> --mode overwrite --content @full.md --format json
```

`+checkpoint-update` 负责保存版本、写入和回读，不要手工编排 `version save → update → read`。

## 动作与输入

| `--command` | 用途 | 必要参数 |
|---|---|---|
| `append` | 末尾追加 | `--content` |
| `overwrite` | 整篇覆盖 | `--content`；JSONML 可加 `--expected-revision` 做服务端原子条件写 |
| `block_insert_after` | 在指定 block 后插入 | `--after-block-id --content` |
| `block_replace` | 替换指定 block | `--block-id --content` |
| `block_delete` | 删除指定 block | `--block-id` |
| `str_replace` | 唯一普通文本替换 | `--old --new` |
| `block_copy_insert_after` | 复制 block 后插入 | `--block-id --after-block-id` |

统一输入协议：已有或临时文件先暂存到当前工作目录后传 `@相对文件`；单次生成文本可用 `--content -` 从 stdin 读取。禁止绝对路径、`..`，也不要猜测 `--content-file`、`--content-format` 或 `replace_all`。

block ID 必须来自 `+fetch --detail with-ids` 或真实 block 列表，禁止编造。

`--expected-revision` 只允许 `--command overwrite --doc-format jsonml`。Markdown、append 和 block 接口没有服务端原子 revision 契约，禁止用写前读取模拟乐观锁。

## 确认与验证

- 执行前从精确 leaf Schema 消费 `confirmation`；需要确认时先 `--dry-run`/询问，用户确认同一参数后追加 `--yes`，禁止先失败探测门禁。
- `+update` 已负责回读验证。除非结果为 `unknown` 或任务需要额外结构验收，不要再做一次全篇读取。
- `doc_write_verification_failed` 表示写入已经发生，必须先读取现状；禁止直接重复写入。
- `partial_success` 只恢复未完成步骤，不能重放成功步骤。

## 高级通道

只有 shortcut 未公开底层参数或必须保留原始响应时，才读取精确 leaf Schema 后使用 `dws doc update` / `doc block`。富结构保真按需读取 [doc-update-workflow.md](style/doc-update-workflow.md)，但执行入口仍优先使用 `+update/+checkpoint-update`。
