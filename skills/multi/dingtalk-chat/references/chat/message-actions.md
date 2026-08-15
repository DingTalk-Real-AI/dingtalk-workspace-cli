# message-actions：消息编辑、撤回、回复与对象操作

> 返回入口：[DingTalk Chat Skill](../../SKILL.md)

用于对真实消息执行编辑、撤回、引用回复、转发、Pin、Top、Favorite 和表情回应写操作，
并包含必要的紧邻验证。需要跨多个阶段传递真实结果的组合流程由
[01-messaging.md](../01-messaging.md) 说明；本文件不重复完整工作流。

## 入口选择

| 用户终点 | 推荐入口 |
|---|---|
| 撤回当前用户消息 | `dws chat +messages-recall --msg-id <openMessageId>` |
| <!-- dws-intent: chat.reply.quote -->引用回复 | `dws chat +messages-reply` |
| 编辑已发送消息 | `dws chat message edit` |
| 单条/合并/话题转发 | `+messages-forward` / `+messages-combine-forward` / `+messages-forward-topic` |
| Pin / Unpin | `+messages-set-pin` / `+messages-unset-pin` |
| 消息 Top / 取消 Top | `+messages-set-top` / `+messages-unset-top` |
| Favorite / 取消 Favorite | `+flag-create` / `+flag-cancel` |
| 默认 emoji 回应 | `+messages-add-emoji` / `+messages-remove-emoji` |

所有写操作以最终 Runtime gate 和精确 leaf Schema 为准。确认对象、消息和影响后再执行；
不要因为文档示例自行制造或省略 confirmation。

## 稳定 ID 规则

- `openTaskId` 是发送任务 ID，不是消息 ID。
- 撤回、编辑、回复、转发、Pin、Top 和 reaction 使用真实查询结果中的 `messageId`。
- 同时保留消息的 `conversationId`、thread、发送者和引用上下文。
- 子消息使用自己的 `messageId`；只在缺会话 ID 时继承父消息 `conversationId`。
- Bot 撤回使用 `processQueryKey`，不使用本文件的 `openMessageId` 路线。

刚由用户身份发送的消息如果只得到 `openTaskId`，先查询发送状态：

```text
+messages-send 或 message send
→ openTaskId
→ message query-send-status
→ openMessageId + openConversationId
→ 编辑或撤回
```

## 撤回与编辑

`+messages-recall` 可只传 `--msg-id`；省略会话 ID 时 CLI 会通过只读消息详情补齐。
兼容单值 `--message-ids`，但不要把 `processQueryKey` 当消息 ID。

```bash
dws chat +messages-recall --msg-id <openMessageId> --format json
dws chat +messages-recall --conversation-id <openConversationId> --msg-id <openMessageId> --format json
```

编辑使用 `message edit --conversation-id <cid> --msg-id <id>`，并在 `--text` 与 `--content`
中二选一。`--text` 由 CLI 生成 Markdown content；`--content` 必须是完整 content JSON。

```bash
dws chat message edit --conversation-id <openConversationId> --msg-id <openMessageId> --text "更新后的内容"
dws chat message edit --conversation-id <openConversationId> --msg-id <openMessageId> --content '{"title":"标题","text":"更新后的内容"}'
```

群聊 @所有人使用 `--at-all`；指定人员使用 `--at-open-dingtalk-ids`。正文中的占位符以
Runtime 规范化结果为准，不把裸展示名当稳定身份。

## 引用回复与转发

引用回复默认使用 `+messages-reply`。`--conversation-id` 和消息 ID 来自真实查询；
`--ref-sender` 可省略时让 CLI 只读补齐，不手工猜发送者身份。

```bash
dws chat +messages-reply --conversation-id <openConversationId> \
  --message-id <openMessageId> --text "收到" --format json
```

| 动作 | 入口 | 关键上下文 |
|---|---|---|
| 单条转发 | `+messages-forward` | 源消息 ID、源会话、目标会话 |
| 合并转发 | `+messages-combine-forward` | 多个真实消息 ID、源/目标会话 |
| 话题转发 | `+messages-forward-topic` | 源消息、源会话、源 thread、目标会话 |

只有 Shortcut 尚未发布真实必需字段时，才评估原子 `message reply`、`forward`、
`combine-forward` 或 `forward-topic`，并先读取精确 leaf Schema。不要复制正文伪装原生转发。

## Pin、Top 与 Favorite

| 对象 | 写入入口 | 说明 |
|---|---|---|
| 消息 Pin | `+messages-set-pin` / `+messages-unset-pin` | 作用于一条消息 |
| 消息 Top | `+messages-set-top` / `+messages-unset-top` | 作用于会话内一条消息 |
| Favorite | `+flag-create` / `+flag-cancel` | 当前用户收藏 |
| 会话 Top | `+conversation-set-top` | 作用于整个会话，不属于本文件 |

用户要求确认 Pin 已生效时，使用 `+messages-list-pin` 检查真实结果中的 `messageId`；取消 Pin
后仅在用户要求确认取消结果时再次查询。典型短链为：`+messages-set-pin` →
`+messages-list-pin` → `+messages-unset-pin`。

需要原子 fallback 时，消息 Pin 对应 `message set-pin-msg/unset-pin-msg`，消息 Top 对应
`message set-top-msg/unset-top-msg`，Favorite 对应 `message add-favorite/remove-favorite`。
四种对象不能互换，即使用户都使用“收藏、钉住、置顶”等自然语言。

## 表情回应

优先在 [chat-emoji-list.md](../chat-emoji-list.md) 按表情名称查默认 emoji，不必全文理解表格。

| 场景 | 入口 |
|---|---|
| 添加/移除默认 emoji | `+messages-add-emoji` / `+messages-remove-emoji` |
| 默认表情无合适项时创建文字表情 | `+messages-create-text-emotion` |
| 添加/移除文字表情 | `+messages-add-text-emotion` / `+messages-remove-text-emotion` |
| 替换文字表情 | `message update-text-emotion` |

reaction 查询属于 [message-query.md](message-query.md)，不要为了查看回应执行写命令。

## 流式卡片与文本工具

流式卡片使用根 Skill 直接链接的 [card/create.md](../card/create.md)、
[card/update.md](../card/update.md) 和 [card/schema.md](../card/schema.md)；本文件不复制卡片参数。

纯文本翻译使用：

```bash
dws chat text translate --query "你好世界" --to en_US
```

用户只要求翻译文本时不要误走消息发送。

## 完成与错误

- 写操作检查任务级结果、投递状态和失败项，不只看退出码。
- 投递状态 unknown 时保留幂等键，不自动换目标重发。
- `unknown flag` 时读取精确 leaf Help，最多修正一次。
- 目标消息不存在、会话不匹配或发送者上下文缺失时停止，不猜 ID。
- 已知稳定消息 ID 的单一动作及其紧邻验证均在本文件完成。
