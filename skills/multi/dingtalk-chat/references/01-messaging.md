# 消息任务级流程

只在单个 Golden Route 不能完成任务、需要跨步骤传递真实结果时读取本文件。简单姓名/群名文本发送、单会话读取和跨会话搜索直接按根 Skill 执行。

## 选择路线

1. 先选择任务语义最窄的 Shortcut。
2. 只有 Shortcut 暂不接受自然目标或目标类型时，才用一个只读 leaf/Shortcut 解析 ID。
3. 解析全部完成并消歧后再写入；不要边解析边产生部分副作用。
4. 后续步骤只使用真实返回字段，不从名称、URL 或上下文猜 ID。

## 群聊消息

<!-- dws-intent: chat.read.conversation -->读取或导出指定群聊/单聊的消息记录时使用 `dws chat +chat-messages`。可附带非必填的 `--sender-query <姓名>`：没有稳定 ID 时保留全部消息，唯一解析成功后按 userId/openDingTalkId 筛选同一次读取结果并返回 `resolvedFilters`，不要补跑 `+search-msg`。

用户以发送者、关键词、@对象或消息类型为主要条件直接检索时优先使用 `+search-msg`；范围可以是单个、多个或全部会话。

指定会话按时间读取时，`+chat-messages` 使用公开可选的 `--start/--end/--order`，范围固定为
`[start,end)`；仅开始时间表示到本次执行当前时间，仅结束时间只支持 `desc`，升序必须有开始时间。
兼容别名为 `--start-time/--end-time/--sort`。旧 `--time/--direction` 只用于单边界兼容模式，不能混用。

已知群 ID 时直接读取：

```bash
dws chat +chat-messages --group <openConversationId> --format json
```

要求全量或导出时直接使用 Runtime 能力：

```bash
dws chat +chat-messages --group <openConversationId> \
  --page-all --page-limit 50 \
  --output ./exports/messages.json \
  --format json
```

```bash
dws chat +chat-messages --group <openConversationId> \
  --start "2026-08-01T00:00:00+08:00" --end "2026-08-02T00:00:00+08:00" \
  --order asc --page-all --format json
```

必须检查 `complete/hasMore/nextPage/stopReason/failures`；达到页数或结果上限不是来源完整。

只有群名时，读取历史直接用 `+chat-messages --group <群名>`，普通文本发送直接用 `+send-to-group`。其它尚不接受群名的高级动作才先用 `+chat-search --query <群名>`；只有唯一候选才把 `openConversationId` 传给下一步。查询结果需要资源时在读取命令上加 `--download-resources`，不要让 Agent 手工遍历资源引用。按姓名读取单聊时先解析唯一用户 ID，再传给 `+chat-messages --user`。

## 发送消息

- <!-- dws-intent: chat.send.dm -->姓名 + 简单文本：`dws chat +dm`。
- <!-- dws-intent: chat.send.group -->群名 + 简单文本：`dws chat +send-to-group`。
- <!-- dws-intent: chat.send.advanced -->已知 ID、文件、Bot、Webhook、复杂 @ 或幂等：`dws chat +messages-send`。
- 姓名 + 文件/高级控制：`+messages-send --as user --user-query <姓名> --file <相对路径>`。
- 群名 + 文件/高级控制：`+messages-send --as user --chat-query <群名> --file <相对路径>`。
- Bot 多群文本/Markdown：`+messages-send --as bot --robot-code <code> --groups <cid1,cid2>`；
  Runtime 去重并返回 `im.batch-write.v1` 逐目标 ledger，最多 100 个稳定群 ID。

`--user-query` 和 `--chat-query` 会在 CLI 内运行真实只读解析；零命中或多候选时在上传或发送前停止。Bot/Webhook 不接受这两个自然目标参数。

文件直接交给 `+messages-send --file`。不要恢复“独立上传 → 提取 mediaId → 发送”的旧默认链路。

## 创建群聊

<!-- dws-intent: chat.create.group -->基础建群默认使用 `dws chat +chat-create`；它同时接受 `--users` 稳定 ID 和 `--member-query` 姓名/花名。群主默认当前用户，也可用 `--owner-open-dingtalk-id` 或 `--owner-query` 明确指定。自然身份解析、候选消歧、稳定 ID 去重和创建前预检都由 CLI 完成：

```text
传入全部姓名
→ 对零命中和多候选统一消歧
→ 按稳定 ID 去重
→ 全部成功后执行一次 +chat-create
```

任一成员或群主未唯一解析时不会创建群；显式群主会加入初始成员且不再读取当前用户，省略群主时才以当前用户兜底。`--dry-run` 也走同一解析链。不要用群名预搜索伪装幂等，因为业务上允许同名群。

## 机器人消息

已知 `robotCode` 时使用 `+messages-send --as bot`；单群用 `--group`，多群用 `--groups` 或工作目录内安全的 `--groups-file`。未知机器人、机器人入群或撤回读取 [chat-bot.md](chat/chat-bot.md)。Bot 不继承 user 的文件/图片能力；只使用 leaf Schema 明确发布的文本/Markdown 能力。

## 引用与转发

- <!-- dws-intent: chat.reply.quote -->引用回复：`dws chat +messages-reply`；优先继续使用结果中的 `messageId`、`conversationId`、`deliveryStatus` 和 `referencedMessage`，未知投递状态不得写成成功送达。
- 单条转发：`+messages-forward`。
- 合并转发：`+messages-combine-forward`。
- 话题转发：`+messages-forward-topic`。

先用 `+chat-messages`、`+search-msg` 或 `+messages-mget` 取得真实 `messageId` 和 conversation/thread 上下文。引用或合并消息中的子消息优先使用自己的 `messageId`，不要拿父消息 ID 代替。

## 上下文传递表

| 上一步 | 真实返回 | 下一步用途 |
|---|---|---|
| `+chat-search` | `openConversationId` | 高级发送、读取、群管理 |
| `dingtalk-contact` 唯一用户解析 | `userId` / `openDingTalkId` | 单聊、建群、@、按发送者搜索 |
| `+messages-send` | `openTaskId` / 投递结果 | 查询投递状态；不是回复/撤回消息 ID |
| `+chat-messages` / `+search-msg` / `+messages-mget` | `messageId`、conversation/thread、`resourceRefs` | 回复、转发、撤回、资源下载 |
| `+chat-create` | `openConversationId` | 新群后续消息与群管理 |
| 分页查询 | `hasMore` / `nextCursor` / `complete` | 继续翻页和完整性判断 |

## 完成判断

- 写操作检查任务级结果或可查询状态，不只看退出码。
- 读取检查 `complete`、`hasMore` 和 `failures`。
- 下载检查每项 ledger；单项失败不抹掉已取得消息。
- 投递状态未知时报告 unknown 并保留幂等键，不自动换目标重发。
