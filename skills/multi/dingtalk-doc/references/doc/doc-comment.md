# doc comment（文档评论：list / create / reply / update / delete / create-inline）

> **前置条件（MUST READ）：** 执行本命令前，必须先用 Read 工具读取以下文件：
> 1. [`../doc.md`](../doc.md) — 命令路由 + 场景索引 + 意图判断 + 工作流
>
> **同任务常配合**：`dws aisearch person`（查 `--mention` 用 userId）/ `dws chat search`（查群用 openConversationId）/ [`doc-block.md`](./doc-block.md)（划词评论必须先取 blockId 与 paragraph 文本）

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
  dws doc comment create --node <DOC_ID> --content "请群内关注" --mentioned-open-conversation-id <OPEN_CID>
Flags:
      --node string      目标文档的标识，支持传入 URL 或 ID (必填)
      --content string   评论的文字内容，纯文本 (必填)
      --mention string   被 @ 的用户 uid 列表，逗号分隔
      --mentioned-open-conversation-id strings  被 @ 的群 openConversationId，可重复指定或逗号分隔
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
  dws doc comment reply --node <DOC_ID> --comment-key <COMMENT_KEY> --content "请群内确认" --mentioned-open-conversation-id <OPEN_CID>
Flags:
      --node string         目标文档的标识，支持传入 URL 或 ID (必填)
      --content string      回复的文字内容，表情回复时填写表情名称 (必填)
      --comment-key string  被回复评论的 commentKey，格式: {13位毫秒时间戳}{32位UUID}，可从 list/create 结果获取 (必填)
      --emoji               设为 true 时作为表情贴图回复 (默认 false)
      --mention string      被 @ 的用户 uid 列表，逗号分隔
      --mentioned-open-conversation-id strings  被 @ 的群 openConversationId，可重复指定或逗号分隔；不得与 --emoji 同时使用
```

---

## doc comment update（更新评论）

```
Usage:
  dws doc comment update [flags]
Example:
  dws doc comment update --node <DOC_ID> --comment-key <COMMENT_KEY> --content "已按最新数据修正"
  dws doc comment update --node <DOC_ID> --comment-key <COMMENT_KEY> --content "更新后的评论内容"
  dws doc comment update --node <DOC_ID> --comment-key <COMMENT_KEY> --content "请群内关注" --mentioned-open-conversation-id <OPEN_CID>
Flags:
      --node string         目标文档的标识，支持传入 URL 或 ID (必填)
      --comment-key string  待更新评论的 commentKey，格式: {13位毫秒时间戳}{32位UUID}，可从 list/create/create-inline 结果获取 (必填)
      --content string      更新后的评论文字内容，纯文本 (必填)
      --mention string      被 @ 的用户 uid 列表，逗号分隔
      --mentioned-open-conversation-id strings  被 @ 的群 openConversationId，可重复指定或逗号分隔
```

---

## doc comment delete（删除评论）

> **CAUTION:** 不可逆操作。必须先向用户确认。用户明确同意后，命令加 `--yes` 执行；不加 `--yes` 时 CLI 会在终端中提示输入 `yes/no`。

```
Usage:
  dws doc comment delete [flags]
Example:
  dws doc comment delete --node <DOC_ID> --comment-key <COMMENT_KEY> --yes
Flags:
      --node string         目标文档的标识，支持传入 URL 或 ID (必填)
      --comment-key string  待删除评论的 commentKey，格式: {13位毫秒时间戳}{32位UUID}，可从 list/create/create-inline 结果获取 (必填)
```

---

## doc comment create-inline（创建划词评论）

> **能力边界：** `create_inline_comment` 后端当前未提供 `mentionedOpenConversationIds`，因此 `create-inline` 不支持 @群。不得只在正文写 `@群名` 冒充真实群 mention；如果用户要求划词评论 @群，应明确提示暂不支持。

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

- `--mention` 接受 `userId` 列表（逗号分隔），需要先用 `dws aisearch person --keyword "<姓名>" --dimension name` 拿到 userId。
- `--mentioned-open-conversation-id` 接受群 `openConversationId`，可重复传入或逗号分隔；CLI 稳定去重并拒绝空白值，不会转换成内部 `cid`。
- 群 mention 仅支持全文评论 `create`、`update`、`reply`；`create-inline` 后端当前未提供该字段，不支持 @群。
- 真正的用户 mention 必须传 `mentionedUserIds`，真正的群 mention 必须传 `mentionedOpenConversationIds`。只在 `content` 写 `@名称` 是普通文本，不会创建 mention 节点或通知。
- 用户直接给 openConversationId 时直接使用。用户给群名时，使用当前 DWS profile 执行 `dws chat search --query "<群名>" --format json`。只在唯一匹配时取 `openConversationId`；多个同名候选必须展示群名称、组织等区分信息让用户选择；无匹配或查询失败时不得调用评论接口。
- 不按所属组织在客户端过滤或提前拒绝 mention，最终权限以评论接口为准。服务端失败时保留或准确概括 `errorCode`、`errorMessage`、`logId`，不得删除 mention 参数静默重试；若要降级为普通文本必须先征得用户同意。
- `content` 只保留正文。仅移除已成功解析成真实 mention 的明确 `@群名` token，保持原换行；普通 `@` 文本或未解析名称不得误删。
- `--comment-key` 是 13 位毫秒时间戳 + 32 位 UUID 的拼接字符串，从 `list` / `create` / `create-inline` 返回中提取，用于 `reply` / `update` / `delete`。
- 划词评论的 `--start` / `--end` 是块内文本字符偏移量，从 0 开始；通过 [`./doc-block.md`](./doc-block.md) `block list` 取 `paragraph.text` 后人工或脚本计算。
- `reply` 加 `--emoji` 时 `--content` 填表情名称（如 `比心`、`赞`），不是文字内容。
- `reply --emoji` 与群 mention 冲突；CLI 会在调用服务端前报错，不会静默忽略。
- `delete` 是不可逆操作；AI Agent 必须先让用户确认，再追加 `--yes`，避免 CLI 进入交互等待。

## 上下文传递

| 从返回中提取 | 用于 |
|-------------|------|
| `commentList[].commentKey` | `comment reply/update/delete` 的 `--comment-key` |
| `comment create` `commentKey` | `comment reply/update/delete` 的 `--comment-key` |
| `comment create-inline` `commentKey` | `comment reply/update/delete` 的 `--comment-key` |
| [`./doc-block.md`](./doc-block.md) `block list` 的 `blocks[].element.id` | `comment create-inline` 的 `--block-id` |
| [`./doc-block.md`](./doc-block.md) `block list` 的 `blocks[].element.paragraph.text` | 计算 `create-inline` 的 `--start` / `--end` 偏移量 |
| `dws aisearch person` 的 `userId` | `comment create/reply/create-inline` 的 `--mention` |
| `dws chat search` 的 `openConversationId` | `comment create/update/reply` 的 `--mentioned-open-conversation-id` |

## 常用模板

```bash
# 查看文档全部评论
dws doc comment list --node <DOC_ID> --format json

# 仅看未解决的划词评论
dws doc comment list --node <DOC_ID> --type inline --resolve-status unresolved --format json

# 创建全文评论
dws doc comment create --node <DOC_ID> --content "这里需要补充数据来源" --format json

# 创建评论 + @人（先 aisearch person 拿 userId）
dws aisearch person --keyword "张三" --dimension name --format json
# 提取 userId 后:
dws doc comment create --node <DOC_ID> --content "请确认这部分" --mention <uid1>,<uid2> --format json

# 创建评论 + @群（搜索结果唯一后取 openConversationId）
dws chat search --query "项目群" --format json
dws doc comment create --node <DOC_ID> --content "请群内关注" --mentioned-open-conversation-id <OPEN_CID> --format json

# 文字回复
dws doc comment reply --node <DOC_ID> --comment-key <COMMENT_KEY> --content "已修改" --format json

# 回复 + @群
dws doc comment reply --node <DOC_ID> --comment-key <COMMENT_KEY> --content "请群内确认" --mentioned-open-conversation-id <OPEN_CID> --format json

# 更新评论
dws doc comment update --node <DOC_ID> --comment-key <COMMENT_KEY> --content "已按最新数据修正" --format json

# 更新 + 同时 @用户和 @群
dws doc comment update --node <DOC_ID> --comment-key <COMMENT_KEY> --content "请大家确认" --mention <UID> --mentioned-open-conversation-id <OPEN_CID> --format json

# 删除评论（不可逆；必须用户确认后再加 --yes）
dws doc comment delete --node <DOC_ID> --comment-key <COMMENT_KEY> --yes --format json

# 表情回复（--content 填表情名称）
dws doc comment reply --node <DOC_ID> --comment-key <COMMENT_KEY> --content "比心" --emoji --format json

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
- `dws aisearch person`（取 mention 用的 userId，跨产品命令）
