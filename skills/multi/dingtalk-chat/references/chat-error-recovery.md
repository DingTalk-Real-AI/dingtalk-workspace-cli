# chat 高频错误与自纠指南

> 本文件是 `dingtalk-chat` 的局部错误契约，与 `dws-shared` 的全局错误规则配合使用。
> 原则：可自纠的错误给出一次明确修正；不可自纠的错误立即停止并报告用户。

## 错误读取顺序

chat 命令由 Agent 执行时统一加 `--format json`。失败时优先读取结构化错误中的 `code`、`message`、`hint`、`actions` 和 `available_flags`；如果当前版本只返回纯文本，则保留原始错误并按同样顺序判断。

## 可自行修复的错误

| 错误表现 | 根因 | Agent 自纠动作 |
|---|---|---|
| `--group` 与单聊收件人参数同时传入 | 单聊/群聊参数互斥 | 删除错误目标；群聊保留 `--group`，单聊保留 `--user` 或 `--open-dingtalk-id` |
| `openConversationId is required` / 群不存在 | 群 ID 错误或缺失 | 执行 `chat search --query "<群名>" --format json`，从返回中取真实 `openConversationId` |
| `userId is required` / 收件人无效 | 未先把人名解析为 ID | 执行 `aisearch person` 或 `contact user search`，取 `userId` / `openDingTalkId` |
| 群成员命令提示 flag 不存在 | 把消息参数用于成员管理 | 改用 `group members --id <openConversationId>`；不要臆造 `members list` |
| 消息搜索结果不符合关键词条件 | 错用了按时间拉取 | 从 `message list` 改为 `message search`；组合过滤才用 `search-advanced` |
| 消息 ID / 卡片 bizId 无效 | 使用了占位符或产品对象 ID | 从源会话消息或 `send-card` 的真实返回中重新提取，不自行构造 |
| Webhook Token 无效 | token 错误或失效 | 停止并报告用户，要求确认 Webhook Token |
| 添加/移除群成员失败 | ID 错误或无权限 | 先确认成员 ID；当前用户无管理权限则停止并报告 |
| 机器人无法添加到群 | 当前用户非群管理员或机器人不可用 | 停止并报告用户，不改用普通用户身份代发 |
| 撤回消息失败 | message key 错误或已超时 | 停止并报告用户 |

## 需用户介入的错误

遇到以下情况不得自行重试或猜测：

- 权限不足（`PermissionDenied`）。
- 资源不存在且重新搜索也无结果。
- 配额超限（例如群成员达到上限）。
- 搜索返回多个同名用户或群，目标存在歧义。
- 不可逆操作尚未获得用户明确确认。

报告时包含原始错误、已验证的目标与仍缺少的信息。

## 跨步骤数据传递

1. `chat search --format json` 返回的 `openConversationId` 直接传给下一步群聊参数。
2. `contact user search` / `aisearch person` 返回的 `openDingTalkId`、`userId` 分别传给对应 flag。
3. `chat message list` / `search` 返回的 `openMessageId` 必须与所属 `openConversationId` 同源，再用于回复、转发、收藏、表情或置顶。
4. `chat message send-card` 返回的 `bizId` 只用于后续 `update-card`。

不要自行构造 ID，也不要假设不同命令返回的字段可以互换。
