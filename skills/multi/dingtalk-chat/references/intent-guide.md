# Chat 低频意图消歧

只在根 Skill 的 Golden Route 无法区分相邻能力时读取。命令 flags 和安全语义以精确 leaf Schema/Runtime 为准。

## 消息选择

| 用户终点 | 选择 | 不要混用 |
|---|---|---|
| 给姓名发简单文本 | `+dm` | 先查人再原子发送 |
| 给群名发简单文本 | `+send-to-group` | 先搜群再原子发送 |
| 文件、Bot、Webhook、复杂 @、幂等 | `+messages-send` | 为不同身份各走一套原子入口 |
| 读取或导出指定会话，可附带发送者姓名 | `+chat-messages`；姓名用非必填 `--sender-query` | 解析成功后读后筛选；解析失败抑制未过滤消息并返回错误；不补跑搜索 |
| 直接按发送者、关键词、@对象或消息类型搜索 | `+search-msg` | 条件检索优先，可限定单个或跨多个会话 |
| 已知消息 IDs 取详情 | `+messages-mget` | 重新搜索关键词 |
| @我的消息 | `+at-me` | 全量消息后本地猜测 @ |
| 已知 thread/topic ID 的回复 | `+thread-replies` | 普通消息列表 |
| 引用回复一条消息 | `+messages-reply` | 普通发送 |
| 单条/合并/话题转发 | `+messages-forward` / `+messages-combine-forward` / `+messages-forward-topic` | 复制正文重新发送 |
| 流式卡片创建或更新 | `+messages-send-card` / `+messages-update-card` | 普通 text/Markdown 发送 |

## Topic 选择

话题与话题圈的创建、发布、浏览、回复、互动和整条转发统一读取 [thread.md](chat/thread.md)，不从普通消息入口选路。

## 对象层级

| 用户终点 | 对象 | Reference |
|---|---|---|
| 收藏或取消收藏 | 当前用户的 Favorite | [message-actions.md](chat/message-actions.md) |
| Pin/Unpin 一条消息 | 消息 Pin | [message-actions.md](chat/message-actions.md) |
| 置顶/取消置顶一条消息 | 消息 Top | [message-actions.md](chat/message-actions.md) |
| 置顶/取消置顶整个会话 | 会话 Top | [chat-conversation.md](chat/chat-conversation.md) |
| 查看置顶会话 | 会话列表 | `+conversation-list-top` |
| 标记消息已读 | 消息读取状态 | [message-actions.md](chat/message-actions.md) |
| 清红点、标记会话未读 | 会话状态 | [chat-conversation.md](chat/chat-conversation.md) |

Favorite、消息 Pin、消息 Top 和会话 Top 不能互换，即使用户都说“收藏/钉住/置顶”。

## 群与机器人

| 用户终点 | 选择 |
|---|---|
| 已有成员 IDs 创建群 | `+chat-create` |
| 查群、查看成员、邀请链接 | [group-discovery.md](chat/group-discovery.md) |
| 加人、踢人、管理员、群公告、群设置 | [group-admin.md](chat/group-admin.md) |
| 找可用机器人并取得单聊 ID | `chat bot find`，不是只查自己创建机器人的 `bot search` |
| 已知 robotCode 发送 | `+messages-send --as bot` |
| 机器人入群、移除、批量群发或撤回 | [chat-bot.md](chat/chat-bot.md) |

## 跨产品边界

- 紧急 DING、短信或电话：切 `dingtalk-misc`，不要当普通 Chat 消息。
- 邮件：切 `dingtalk-mail`。
- 只查询人员资料或把姓名解析成 ID：切 `dingtalk-contact`；若终点只是简单发消息，直接留在 Chat 使用 `+dm`。
- 企业知识跨文档、消息、邮件搜索：切 `dingtalk-aisearch`；明确只搜聊天消息时使用 `+search-msg`。
- 文档翻译先由文档产品读取正文；`chat text translate` 只处理纯文本。
