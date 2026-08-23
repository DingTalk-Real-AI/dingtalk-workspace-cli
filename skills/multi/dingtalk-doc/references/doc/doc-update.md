# 更新在线文字文档

## 唯一推荐入口

普通追加、覆盖和 block 编辑统一使用 `+update`：

`--command` 只接受下列枚举值，不接受 JSON、自然语言或拼接子命令；动作参数必须分别传给 `--content/--old/--new/--block-id/--after-block-id`。

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

## 最小改写决策

| 已知条件 | 推荐路径 | 成本与成功率理由 |
|---|---|---|
| 用户明确要求在末尾追加 | 直接 `append` | 不为找末尾先拉全文；需要语气衔接时只读末节 |
| 已知唯一旧文本与新文本 | 直接 `str_replace --old --new` | 省掉 block 解析；旧文本不唯一时 Runtime 必须失败，不放宽匹配 |
| 指定章节但没有 block ID | `+fetch outline` → `+fetch section --detail with-ids` → block 动作 | 两个小读取换取稳定锚点，避免全文 token 和误改相邻章节 |
| 已知真实 block ID | 直接 `block_replace/delete/insert_after` | 最小副作用；不改无关 block |
| 多处富结构保真修改 | `+fetch --detail full` 后定点 JSONML 更新 | 保留图片、附件、引用、表格和样式；不要从 Markdown 有损重建 |
| 整篇重要覆盖 | `+checkpoint-update --mode overwrite` | 自动保存恢复点、执行并回读；普通 overwrite 只用于明确不需恢复点的场景 |

同一篇文档的正文由一个主上下文串行维护：Plan（确定最小变更）→ Execute（一次写）→ Observe（先消费回执）→ Iterate（只修未达标部分）。不要按章节并行写同一文档，也不要每次迭代都重新读取全文或 Schema。

## Block ID 生命周期与保真

- `block_replace` 成功后 Runtime 使用同一 `blockId` 回读验证，该 ID 可继续作为锚点；验证失败时先局部 `+fetch` 核对现状。`block_delete` 成功后旧 ID 失效，不得继续复用。
- `block_insert_after` / `block_copy_insert_after` 后，原锚点通常仍可识别，但新 block 的 ID 必须来自真实返回或局部回读，禁止按顺序猜测。
- `str_replace` 的简单行内替换通常不要求重新取 ID；若后续依赖块结构，仍以局部回读为准。
- 从 Markdown 读取后覆盖整篇可能丢失图片、附件、@人/@文档、评论锚点、表格样式和嵌套块。只改局部时使用 block 手术；确需整篇保真改写时使用 `full` JSONML，并以 `--expected-revision` 防止覆盖并发修改。

## 确认与验证

- `+update` 与 `+checkpoint-update` 当前都要求用户确认。目标、动作、内容范围或参数变化后必须重新确认；只有发现本地文档与 live leaf 漂移时才查一次精确 Schema，禁止每次写入都重复发现。
- `+update` 已负责回读验证。除非结果为 `unknown` 或任务需要额外结构验收，不要再做一次全篇读取。
- `doc_write_verification_failed` 表示写入已经发生，必须先读取现状；禁止直接重复写入。
- `partial_success` 只恢复未完成步骤，不能重放成功步骤。

## 高级通道

只有 shortcut 未公开底层参数或必须保留原始响应时，才读取精确 leaf Schema 后使用 `dws doc update` / `doc block`。富结构保真按需读取 [doc-update-workflow.md](style/doc-update-workflow.md)，但执行入口仍优先使用 `+update/+checkpoint-update`。
