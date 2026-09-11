# Chat 低频原子能力索引

> 返回入口：[DingTalk Chat Skill](../SKILL.md)

本文件只用于根 Skill 和精确 task reference 都未覆盖的低频底层能力。普通发送、读取、搜索、
建群、引用回复和查看置顶会话必须回到根 Skill 的 Golden Route，不在这里重新选路。

## 使用边界

1. 先确认任务确实需要 Shortcut 未发布的底层字段、原始响应或运维控制；
2. 读取精确原子 leaf 窄 Schema，不默认读 Help，也不加载产品级 Catalog 猜参数；
3. 自然目标仍必须唯一解析，禁止选择搜索结果第一项；
4. 原子写 leaf 的 confirmation 若与对应 Golden Shortcut 不一致，停止并报告交付漂移；
5. 后续 ID 只使用当前 profile 的真实返回，不跨组织复用；
6. 完成后保留原始结果、partial failure 和可继续编排的稳定 ID。

## 高频任务返回表

| 用户终点 | 返回入口 |
|---|---|
| 姓名/群名简单发送、文件、Bot、Webhook、复杂 @ | 根 Skill Golden Route |
| 消息读取、条件搜索、@我、Favorite/reaction 查询和批量详情 | [message-query](chat/message-query.md) |
| 编辑、撤回、引用、转发、reaction/Pin/Top/Favorite 写入 | [message-actions](chat/message-actions.md) |
| 位置、名片、资源下载和特殊媒体 fallback | [message-media](chat/message-media.md) |
| 群列表、群搜索、成员读取、Bot 列表和邀请链接 | [group-discovery](chat/group-discovery.md) |
| 建群、改群、成员写入、管理员、禁言、公告和群设置 | [group-admin](chat/group-admin.md) |
| 跨步骤消息/群组合流程 | [消息任务级流程](01-messaging.md) |
| Bot 搜索、进群和撤回 | [chat-bot](chat/chat-bot.md) |
| 会话置顶、状态和分组 | [chat-conversation](chat/chat-conversation.md) |
| 话题与话题圈的创建、发布、浏览、回复、互动和整条转发 | [thread](chat/thread.md) |
| 相邻低频意图仍需消歧 | [intent-guide](intent-guide.md) |

## 消息底层能力

| 原子命令 | 仅用于 |
|---|---|
| `chat message send` | `+messages-send` 尚未发布的位置、名片等真实底层消息类型 |
| `chat message list` | 需要原始响应或显式手工 continuation；普通浏览使用 `+chat-messages` |
| `chat message list-all` | 指定时间范围的原始全会话分页接口 |
| `chat message list-by-sender` | 需要原始按发送者响应；普通组合搜索使用 `+search-msg` |
| `chat message list-mentions` | `+at-me` 未发布的字段或原始响应 |
| `chat message list-focused` | 特别关注原始列表 |
| `chat message search` / `search-advanced` | `+search-msg` 未发布的底层过滤字段或原始响应 |
| `chat message query-send-status` | `+messages-query-send-status` 未发布的字段或原始响应 |
| `chat message recall` | `+messages-recall` 未发布的字段或原始响应 |
| `chat message edit` | 编辑已知消息 |
| `chat message read-status` | `+messages-read-status` 未发布的字段或原始响应 |
| `chat message reply` | `+messages-reply` 未发布的底层引用字段，且安全门禁已对齐 |
| `chat message forward` / `combine-forward` | Shortcut 未覆盖的精确转发字段 |
| `chat message download-media` | `+messages-resource-download` 未发布的底层字段或原始响应 |

消息对象管理：

| 原子命令 | 对象 |
|---|---|
| `message set-pin-msg` / `unset-pin-msg` / `list-pin-msg` | 对应 `+messages-set-pin` / `+messages-unset-pin` / `+messages-list-pin` 未发布的字段或原始响应 |
| `message set-top-msg` / `unset-top-msg` | `+messages-set-top` / `+messages-unset-top` 的底层 fallback |
| `message add-favorite` / `remove-favorite` / `list-favorites` | `+flag-create` / `+flag-cancel` / `+flag-list` 的底层 fallback |
| `message add-emoji` / `remove-emoji` | `+messages-add-emoji` / `+messages-remove-emoji` 的底层 fallback |
| `message create-text-emotion` / `add-text-emotion` / `remove-text-emotion` | 对应 `+messages-create-text-emotion` / `+messages-add-text-emotion` / `+messages-remove-text-emotion` 的底层 fallback |
| `message update-text-emotion` | Shortcut 未覆盖的文字表情更新 |
| `message list-emotion-replies` | 批量 reaction/文字回应 |
| `emotion list` / `send` / `favorite` | 当前用户个人收藏表情列表、发送和新增 |

Favorite、消息 Pin、消息 Top 与会话 Top 是四种对象，不能互换。
个人收藏表情与消息 reaction/文字回应不同；发送收藏表情使用 `chat emotion send`，给已有消息贴表情使用 `+messages-add-emoji` 或 `+messages-add-text-emotion`。

## 群与成员底层能力

| 原子命令 | 用途 |
|---|---|
| `chat search` | `+chat-search` 未发布的字段或原始响应 |
| `chat search-common` | 查询共同群 |
| `chat group get-by-group-id` | `+chat-get-by-id` 未发布的字段或原始响应 |
| `chat group create` | `+chat-create` 尚未发布的真实底层创建字段；显式群主已由 Shortcut 覆盖 |
| `chat group members` / `members list-by-ids` | 群成员分页和精确详情 |
| `chat group members add` / `remove` | 添加/移除已知成员 ID |
| `chat group members add-bot` / `remove-bot` / `group bots` | 对应 `+chat-add-bot` / `+chat-remove-bot` / `+chat-bots` 的底层 fallback |
| `chat group rename` / `update-icon` | 对应 `+chat-update` / `+chat-update-icon` 的底层 fallback |
| `chat group transfer-owner` / `set-admin` | 对应 `+chat-transfer-owner` / `+chat-set-admin` 的底层 fallback |
| `chat group upgrade-to-external` | 普通群升级外部群；不可逆 |
| `chat group invite-url` / `share-invite` | `+chat-invite-url` 未发布的邀请链接字段或分享 |
| `chat group update-settings` / `user-settings query|set` | `+chat-update-settings` 未发布的管理员字段，或当前用户群偏好 |
| `chat group update-nick` / `update-alias` | 对应 `+chat-update-nick` / `+chat-update-alias` 的底层 fallback |
| `chat group set-history` | `+chat-set-history` 未发布的字段或原始响应 |
| `chat group-mute` / `group-mute-member` | `+chat-mute` / `+chat-mute-member` 的底层 fallback |
| `chat group notice create|edit|get|list` | 群公告 |
| `chat group list-my-groups` / `list-all` | `+my-groups` / `+chat-list-mine` 未投影的字段或原始响应 |
| `chat group list-join-validations` / `audit-join-validation` | `+chat-list-join-requests` / `+chat-audit-join` 的底层 fallback |
| `chat group-role *` | 对应 `+chat-role-*` 未发布的字段或原始响应 |

退出、解散群、踢人、转让群主、升级外部群、禁言、管理员和公告写入都属于高影响操作；
必须以最终 Runtime gate/Schema 为准确认对象与影响。

## Bot 与 Webhook 底层能力

| 原子命令 | 用途 |
|---|---|
| `chat bot search` | `+bot-search` 未发布的字段或原始响应 |
| `chat bot find` | `+bot-find` 未发布的字段或原始响应 |
| `chat message send-by-bot` | `+messages-send --as bot` 未发布的真实底层字段，包括机器人群聊引用回复的 `--reply` / `--ref-sender` |
| `chat message recall-by-bot` | `+messages-recall-by-bot` 未发布的字段或原始响应 |
| `chat message send-by-webhook` | `+messages-send --as webhook` 未发布的真实底层字段 |

新发送流程统一使用 `+messages-send`。不得因看见 bot/webhook 原子命令就绕开统一身份能力矩阵。

## 会话状态与分组

| 原子命令 | 用途 |
|---|---|
| `chat conversation-info` | `+conversation-info` 未发布的字段或原始响应 |
| `chat list-all-conversations` | `+conversation-list` 未投影的字段或原始响应 |
| `chat list-top-conversations` | 需要原始响应时的置顶会话 fallback；普通查看使用 `+conversation-list-top` |
| `chat set-top` | `+conversation-set-top` 未发布的字段或原始响应 |
| `chat mute` / `hide` / `mute-at-all` / `mute-red-envelope` | 对应会话 Shortcut 未发布的字段或原始响应 |
| `chat mark-unread` / `mark-read` | 对应会话 Shortcut 未发布的字段或原始响应 |
| `chat clear-red-point` / `clear-all-red-point` | 对应会话 Shortcut 未发布的字段或原始响应 |
| `chat clear-messages` | `+conversation-clear-messages` 未发布的字段或原始响应 |
| `chat category *` | 对应 `+category-*` 未发布的字段或原始响应；对象是会话分组，不是群聊 |

消息 Top 使用 `+messages-set-top`，整个会话 Top 使用 `+conversation-set-top`，查看置顶会话使用 `+conversation-list-top`。

## 稳定 ID 传递

| 来源 | 只可用于 |
|---|---|
| 唯一群解析 / `+chat-create` | 当前 profile 下的 `openConversationId` |
| 唯一人员解析 | 当前 profile 下的 `userId` / `openDingTalkId` |
| `+messages-send` | `openTaskId` 查询投递状态；它不是消息 ID |
| `+chat-messages` / `+search-msg` / `+messages-mget` | 回复、转发、撤回、资源操作使用的真实消息/会话/thread ID |
| `+bot-search` | `robotCode`；不能当机器人 `openDingTalkId` |
| `chat message send-by-bot` | `processQueryKey` 用于机器人撤回；群聊引用回复还需消息查询返回的 `openMessageId` 与原发送者 `openDingTalkId` |

显式稳定 ID 当前不携带可验证的 profile provenance；调用方必须保证来源，不得宣称所有
跨 profile 误用都会在本地写入前被拦截。

## 故障处理

- `unknown flag`：读取同一 leaf Help，最多修正一次；
- `unknown command`：不查 Help；先用错误返回的明确 suggestion，再用已加载 Skill/reference 的明确兼容入口，仍无则报漂移并停；不枚举全 Catalog；
- confirmation 或参数约束不清：读取精确 leaf Schema，以最终 Runtime gate 为准；
- 自然目标零命中/多候选：停止并展示候选，不选择第一项；
- 权限、认证或 profile：按 `dingtalk-shared` 对应 reference 分流；
- partial result：保留已完成项、失败 ledger、continuation 和真实错误，不换同义原子命令重试。
