# 话题与话题圈

> 返回入口：[DingTalk Chat Skill](../../SKILL.md)

话题（Thread）是一条主消息及其回复线，可以位于普通群或话题圈中，使用 `openConvThreadId` 标识；承载话题的父群使用 `openConversationId`。命令沿用拆分前 `chat group` / `chat message` 的参数名称。只有需要新建话题圈时才使用 `create-group`，已有群中的话题可直接使用其余 `chat thread` 命令。

## 入口选择

| 用户终点 | 推荐入口 |
|---|---|
| 已有稳定成员 ID 创建话题圈 | `dws chat thread create-group --name <名称> --users <userId,...>` |
| 发布新话题 | `dws chat thread send --conversation-id <openConversationId>` |
| 将普通群中的已有消息升级为 Thread | `dws chat thread promote --conversation-id <openConversationId> --message-id <openMessageId>` |
| 浏览话题主消息 | `dws chat thread list --conversation-id <openConversationId>` |
| 向具体话题直接追加回复 | `dws chat thread reply --conversation-id <openConvThreadId>` |
| 读取一个话题的回复 | `dws chat +thread-replies --group <openConversationId> --thread-id <openConvThreadId>` |
| 转发整条话题 | `dws chat +messages-forward-topic --src-msg-id <openMessageId> --src-conversation-id <openConversationId> --src-thread-id <openConvThreadId> --dest-conversation-id <openConversationId>` |
| 撤回话题中的一条消息 | `dws chat thread recall-message --conversation-id <openConversationId> --message-id <openMessageId>` |
| 添加或移除 emoji | `dws chat thread add-emoji` / `remove-emoji` |
| 查询 Thread 消息的表情回复 | `dws chat thread list-emotion-replies --msg-ids <openMessageId,...>` |
| 添加、移除或更新文字表情 | `dws chat thread add-text-emotion` / `remove-text-emotion` / `update-text-emotion` |

## 发布、回复与读取

`thread send` 的 `--conversation-id` 是承载话题的会话 `openConversationId`，用于发布新的顶层话题。

`thread promote` 用于把普通群中一条已经存在的消息升级为 Thread 根消息；它同时需要消息所属普通群的 `openConversationId` 和该消息的 `openMessageId`，成功后返回新的 `openConvThreadId`。单聊消息不能升级；若要发送全新的 Thread，继续使用 `thread send`。

`thread reply` 沿用原发送命令的 `--conversation-id`，但这里传 Thread 子会话的 `openConvThreadId`。它直接追加回复，不使用消息引用回复，也不创建新的顶层 Thread。

已有父会话 `openConversationId` 和 Thread `openConvThreadId` 时使用 `+thread-replies --group ... --thread-id ...`；只有主消息 ID 时用 `--message-id` 自动解析。按需加 `--page-all`、排序或资源下载。原子 `thread list-replies` 仅用于 Shortcut 未发布的底层字段或原始响应。只浏览话题主消息时使用 `thread list`。

整条 Thread 可转发到普通群；当前不支持从话题圈向另一个话题圈转发整条 Thread。

## 消息操作

撤回、emoji 和文字表情命令沿用对应 `chat message` 命令的主参数。Runtime 会先读取消息并校验其属于 Thread，再执行操作；批量查询会逐条校验 `--msg-ids`。文字表情先用 `+messages-create-text-emotion`，添加/移除用 `+messages-add-text-emotion` / `+messages-remove-text-emotion`；其 `emotionId`、`backgroundId`、名称和文字必须来自真实返回。仅更新使用未被 Shortcut 覆盖的 `thread update-text-emotion`。

整条话题默认用 `+messages-forward-topic`；原子 `thread forward` 仅用于未公开字段或原始响应。

## 完成与错误

- 创建话题圈返回真实群会话结果，不额外制造 `openTopicId` 字段。
- 发布和回复沿用异步发送结果；`openTaskId` 是任务 ID，后续消息操作需要从发送状态或消息查询中取得真实消息 ID。
- Thread 主消息和回复均保留 `openConvThreadId`，父容器继续使用 `openConversationId`。
- 标识缺失、类型不明或消息不属于 Thread 时停止，不猜 ID。
