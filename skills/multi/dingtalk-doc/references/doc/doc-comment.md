# doc comment（文档评论：list / create / reply / update / delete / create-inline）

> **前置条件（MUST READ）：** 执行本命令前，必须先用 Read 工具读取以下文件：
> 1. [`../doc.md`](../doc.md) — 命令路由 + 场景索引 + 意图判断 + 工作流
>
> **同任务常配合**：`dws contact user search`（查 `--mention` 用 userId）/ [`doc-block.md`](./doc-block.md)（划词评论必须先取 blockId 与 paragraph 文本）

---

## doc comment list（查询文档评论列表）

```
Usage:
  dws doc comment list [flags]
Example:
  dws doc comment list --node <DOC_ID>
  dws doc comment list --node <DOC_ID> --type inline --resolve-status unresolved
  dws doc comment list --node <DOC_ID> --limit 20 --cursor <TOKEN>
Flags:
      --node string            目标文档的标识，支持传入 URL 或 ID (必填)
      --limit int          每页返回的评论数量，默认 50，最大 50
      --cursor string      分页游标，从上一次请求的返回结果中获取 (首次请求不传)
      --type string            按评论类型过滤: global (全文评论) / inline (划词评论)
      --resolve-status string  按解决状态过滤: resolved (已解决) / unresolved (未解决)
```

---

## doc comment create（创建全文评论）

```
Usage:
  dws doc comment create [flags]
Example:
  dws doc comment create --node <DOC_ID> --content "这里需要修改"
  dws doc comment create --node <DOC_ID> --content "请review" --mention uid1,uid2
Flags:
      --node string      目标文档的标识，支持传入 URL 或 ID (必填)
      --content string   评论的文字内容，纯文本 (必填)
      --mention string   被 @ 的用户 uid 列表，逗号分隔
```

---

## doc comment reply（回复评论）

```
Usage:
  dws doc comment reply [flags]
Example:
  dws doc comment reply --node <DOC_ID> --comment-key <COMMENT_KEY> --content "同意"
  dws doc comment reply --node <DOC_ID> --comment-key <COMMENT_KEY> --content "比心" --emoji
  dws doc comment reply --node <DOC_ID> --comment-key <COMMENT_KEY> --content "请确认" --mention uid1,uid2
Flags:
      --node string         目标文档的标识，支持传入 URL 或 ID (必填)
      --content string      回复的文字内容，表情回复时填写表情名称 (必填)
      --comment-key string  被回复评论的 commentKey，格式: {13位毫秒时间戳}{32位UUID}，可从 list/create 结果获取 (必填)
      --emoji               设为 true 时作为表情贴图回复 (默认 false)
      --mention string      被 @ 的用户 uid 列表，逗号分隔
```

---

## doc comment update（更新评论）

```text
Usage:
  dws doc comment update [flags]
Example:
  dws doc comment update --node <DOC_ID> --comment-key <COMMENT_KEY> --content "已按最新数据修正"
  dws doc comment update --node <DOC_ID> --comment-key <COMMENT_KEY> --content "请确认" --mention uid1,uid2
Flags:
      --node string         目标文档 ID 或 URL (必填)
      --comment-key string  待更新评论的 commentKey (必填)
      --content string      更新后的评论内容 (必填)
      --mention string      被 @ 的用户 uid 列表，逗号分隔
```

---

## doc comment delete（删除评论）

```text
Usage:
  dws doc comment delete [flags]
Example:
  dws doc comment delete --node <DOC_ID> --comment-key <COMMENT_KEY> --yes
Flags:
      --node string         目标文档 ID 或 URL (必填)
      --comment-key string  待删除评论的 commentKey (必填)
```

删除不可恢复，必须先获得用户明确确认，再传全局 `--yes`。

---

## doc comment create-inline（创建划词评论）

```
Usage:
  dws doc comment create-inline [flags]
Example:
  dws doc comment create-inline --node <DOC_ID> --block-id <BLOCK_ID> --start 0 --end 10 --content "这里需要修改"
  dws doc comment create-inline --node <DOC_ID> --block-id <BLOCK_ID> --start 5 --end 20 --content "建议调整" --selected-text "被选中的原文"
  dws doc comment create-inline --node <DOC_ID> --block-id <BLOCK_ID> --start 0 --end 10 --content "请review" --mention uid1,uid2
Flags:
      --node string            目标文档的标识，支持传入 URL 或 ID (必填)
      --block-id string        评论标记所在的块 ID，可通过 dws doc block list 获取 (必填)
      --start int              评论标记在块内文本中的起始字符偏移量，从 0 开始 (必填)
      --end int                评论标记在块内文本中的结束字符偏移量，必须大于 start (必填)
      --content string         评论的文字内容，纯文本 (必填)
      --selected-text string   选中文本的内容，填写后评论列表中会展示「引用原文：xxx」
      --mention string         被 @ 的用户 uid 列表，逗号分隔
```

## 关键说明

- `--mention` 接受 `userId` 列表（逗号分隔），需要先用 `dws contact user search --query "<姓名>"` 拿到 userId。
- `--comment-key` 是评论唯一标识，从 `list` / `create` / `create-inline` 返回中提取，可用于 `reply` / `update` / `delete`。
- 划词评论的 `--start` / `--end` 是块内文本字符偏移量，从 0 开始；通过 [`./doc-block.md`](./doc-block.md) `block list` 取 `paragraph.text` 后人工或脚本计算。
- `reply` 加 `--emoji` 时 `--content` 填表情名称（如 `比心`、`赞`），不是文字内容。

## 上下文传递

| 从返回中提取 | 用于 |
|-------------|------|
| `commentList[].commentKey` | `comment reply/update/delete` 的 `--comment-key` |
| `comment create` `commentKey` | `comment reply/update/delete` 的 `--comment-key` |
| `comment create-inline` `commentKey` | `comment reply/update/delete` 的 `--comment-key` |
| [`./doc-block.md`](./doc-block.md) `block list` 的 `blocks[].element.id` | `comment create-inline` 的 `--block-id` |
| [`./doc-block.md`](./doc-block.md) `block list` 的 `blocks[].element.paragraph.text` | 计算 `create-inline` 的 `--start` / `--end` 偏移量 |
| `dws contact user search` 的 `userId` | `comment create/reply/create-inline` 的 `--mention` |

## 常用模板

```bash
# 查看文档全部评论
dws doc comment list --node <DOC_ID> --format json

# 仅看未解决的划词评论
dws doc comment list --node <DOC_ID> --type inline --resolve-status unresolved --format json

# 创建全文评论
dws doc comment create --node <DOC_ID> --content "这里需要补充数据来源" --format json

# 创建评论 + @人（先 contact user search 拿 userId）
dws contact user search --query "张三" --format json
# 提取 userId 后:
dws doc comment create --node <DOC_ID> --content "请确认这部分" --mention <uid1>,<uid2> --format json

# 文字回复
dws doc comment reply --node <DOC_ID> --comment-key <COMMENT_KEY> --content "已修改" --format json

# 表情回复（--content 填表情名称）
dws doc comment reply --node <DOC_ID> --comment-key <COMMENT_KEY> --content "比心" --emoji --format json

# 更新评论（可选 --mention）
dws doc comment update --node <DOC_ID> --comment-key <COMMENT_KEY> --content "已修正" --format json

# 删除评论（危险操作，需用户先明确确认）
dws doc comment delete --node <DOC_ID> --comment-key <COMMENT_KEY> --yes --format json

# 划词评论（先 block list 取 blockId + paragraph.text，计算 start/end）
dws doc block list --node <DOC_ID> --format json
# 计算偏移后：
dws doc comment create-inline --node <DOC_ID> --block-id <BLOCK_ID> --start 0 --end 10 --content "这里需要修改" --format json

# 划词评论 + 引用原文 + @人
dws doc comment create-inline --node <DOC_ID> --block-id <BLOCK_ID> --start 5 --end 20 --content "请确认这部分" --selected-text "被选中的原文内容" --mention <uid1>,<uid2> --format json
```

## 参考

- [`../doc.md` §意图判断](../doc.md#意图判断)（如何路由到本命令族）
- [`./doc-block.md`](./doc-block.md)（取 blockId 与块内文本以计算划词偏移）
- `dws contact user search`（取 mention 用的 userId，跨产品命令）
