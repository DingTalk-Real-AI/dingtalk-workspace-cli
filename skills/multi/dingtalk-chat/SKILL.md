---
name: dingtalk-chat
description: 钉钉群聊与消息。Use when 收发/搜索消息、建群、群治理、Bot/Webhook、文件，或仅限 IM 的消息谓词筛选。跨源主题/行为轨迹走 dingtalk-aisearch；DING/班级群走 dingtalk-misc；邮件走 dingtalk-mail。
metadata:
  cli_version: ">=0.2.14"
  category: product
  requires:
    bins:
      - dws
---

# 群聊/消息

<!-- DWS_RUNTIME_CONTRACT_START -->
## 最小 DWS 执行契约

- 只用 `dws`；结构化读取加 `--format json`，按真实返回判断。
- 已知命令直调；参数/约束/安全不明查 leaf 窄 Schema。Schema 不可用才读已知 leaf Help 一次；`unknown flag` 用同 leaf Help 修正一次。`unknown command` 不查 Help：优先错误中的明确 suggestion，其次已加载 Skill/reference 中的明确兼容入口；均无则报漂移并停，禁全 Catalog。低频 reference 不默认 Help，禁 root/parent/product Help。发现后必须执行或说明阻塞。
- 不猜命令/flag/字段/ID/账号/业务事实；ID 来自真实返回。目标零命中/多候选/类型不明先消歧；仅可选时间/展示范围用契约默认，缺必需信息即停。
- 解析/读/写同一 profile，ID 不跨组织。多账号只用唯一 `isOrgCurrent=true`；否则用户指定，禁止选择第一项、最近登录或最近使用账号。
- 不输出/记录 token、refresh token、appSecret、webhook token；已注入认证时不索要。
- 写须符合明确意图；确认以最终 Runtime gate/Schema 为准，确认后才加 `--yes`。
- 写后验证结果，不凭退出码宣称成功。退出须最终答复，区分完成、部分、阻塞、待确认、失败；保留已有数据及 `complete/hasMore/stopReason/failures`。
- 时间戳按会话时区展示，必要时保留原值。
- 认证/权限/profile/confirmation/未知错误只读 `dingtalk-shared` 对应 reference，禁连续猜替代命令。
<!-- DWS_RUNTIME_CONTRACT_END -->

## 对象与路由边界

- `openConversationId` 是会话，`messageId/openMessageId` 是消息，`openTaskId` 是发送任务，不可混用。
- 会话分组/分类走 `category`，群聊/聊天群走 `chat group`；会话列表无正文，总结/统计读消息。Pin/Top/Favorite 不是分类。

<!-- VISIBLE_SHORTCUTS_START -->
## Shortcut 发现（Shortcut-first）

按 Golden Route/reference 选 Shortcut；仅缺底层字段用 atomic，低频走 reference/Catalog。

参数查 `dws schema --cli-path "chat <leaf>" --compact --jq '{cli_path,parameters,constraints,confirmation}' -f json`；仅需且已发布 `result` 时查 outcomes/pagination，字段级再查 `data_schema`；缺失不以 Help/样例推断。Schema 不可用才读一次已知 leaf Help；`unknown flag` 用同 leaf Help 修正一次。`unknown command` 禁 Help：错误 suggestion → 已加载 Skill/reference 明确入口；均无则报漂移。禁全 Catalog/root/parent/product Help；低频 reference 不默认 Help。
<!-- VISIBLE_SHORTCUTS_END -->

## Golden Route

| 用户终点 | 唯一推荐入口 | 关键边界 |
|---|---|---|
| <!-- dws-intent: chat.read.conversation -->读取指定群聊/单聊 | `dws chat +chat-messages --no-reactions` | 全部时加 `--page-all` |
| <!-- dws-intent: chat.search.filtered --><!-- dws-intent: chat.read.reactions -->按关键词/发送者/@/类型/reaction 过滤 | `dws chat +search-msg`（reaction 加 `--has-reactions`） | 默认 7 天；范围用 `--start/--end` |
| 跨会话读取/总结/统计 | `dws chat message list-all --start <开始> --end <结束> --page-all --no-reactions` | 不先列会话逐群循环 |
| <!-- dws-intent: chat.conversation.active-since -->时间后活跃会话 | `dws chat +recent-conversations --start <时间>` | 摘要；查 `complete`；`+active-conversations` 仅兼容 |
| 查看 @我的消息 | `dws chat +at-me [--group <群名或ID>] --page-all --no-reactions` | 未指定群则跨会话；默认 7 天 |
| 查看未读消息 | `dws chat +unread-chats` | 需正文时沿 CID 读消息 |
| 已知消息 ID 批量取详情 | `dws chat +messages-mget` | 看 leaf Schema；保留会话上下文 |
| <!-- dws-intent: chat.send.dm -->按姓名发文本/Markdown | `dws chat +dm --to <姓名> --content <内容>` | 唯一解析；多候选停止 |
| <!-- dws-intent: chat.send.group -->按群名/ID 发文本/Markdown | `dws chat +send-to-group --group <群名或ID> --content <内容>` | 多候选停止 |
| <!-- dws-intent: chat.send.advanced -->文件/Bot/Webhook/复杂 @ | `dws chat +messages-send` | Bot 多群检查逐项 ledger |
| 全部会话 | `dws chat +conversation-list --page-all` | 含群聊/单聊，非正文 |
| 查加入/管理的群 | `+my-groups --page-all` / `+chat-list-mine` | 后者无 `--page-all`；flag 不跨 leaf |
| 搜群或查看全部成员 | `+chat-search --query <词>` / `+chat-members-list --group <群名或ID>` | 多候选停止；检查 buckets/完整性 |
| 查群资料/Bot/邀请链接 | `+conversation-info` / `+chat-bots` / `+chat-invite-url` | 只读 |
| <!-- dws-intent: chat.create.group -->创建/清理临时群 | `dws chat +chat-create --name <名称> --member-query <姓名列表>` → 保存 CID → `+chat-dismiss --group <cid>` | 已知 ID 用 `--users`；清理须确认、验证 |
| 改群资料/设置/禁言/管理员 | `+chat-update` / `+chat-update-settings` / `+chat-mute` / `+chat-mute-member` / `+chat-set-admin` | 用真实群/用户 ID；写后读回 |
| 管理群身份 | 读 [group-admin](references/chat/group-admin.md) 角色 family | 角色 CRUD、成员绑定/解绑/查询；不切 atomic，写后回读 |
| <!-- dws-intent: chat.category.list-conversations -->列分类内会话 | `dws chat +category-list-conversations --category-id <ID>` | 分类≠群；先取 ID |
| <!-- dws-intent: chat.reply.quote -->引用回复 | `dws chat +messages-reply` | 用真实消息/CID；未知投递状态非成功 |
| 撤回/转发 | `+messages-recall`；`+messages-forward` / `+messages-combine-forward` / `+messages-forward-topic` | 不复制正文冒充原生转发 |
| Pin/消息 Top/Favorite | `+messages-set-pin` / `+messages-unset-pin`；`+messages-set-top` / `+messages-unset-top`；`+flag-create` / `+flag-cancel` | 对象互不替代；用对应查询验证 |
| 添加/移除 reaction | `+messages-add-emoji` / `+messages-remove-emoji` | 扩展动作读 `message-actions` |
| 会话置顶/免打扰/隐藏 | [chat-conversation](references/chat/chat-conversation.md) | 用真实 CID；会话 Top 非消息 Top |
| 已读/未读/清红点/清空 | `+conversation-mark-read` / `+conversation-mark-unread` / `+conversation-clear-red-point` / `+conversation-clear-all-red-point` / `+conversation-clear-messages` | 已读需消息 ID；清空按 Runtime 确认 |
| 下载消息资源 | 查询加 `--download-resources --output-dir <目录>`；已有引用用 `+messages-resource-download` | 不猜 ID；保留 ledger；临时 URL 不交付 |

次级：Thread `+thread-replies`；<!-- dws-intent: chat.conversation.list-top -->置顶 `dws chat +conversation-list-top`；上传 `conversation-file upload`；IM 事件走 [`dingtalk-event`](../dingtalk-event/SKILL.md)。

## 关键结果语义

- 查询保留范围、数量、完整性、停止原因、失败/下载 ledger；不完整不得称成功。消息仅 `--page-all` 翻页，达预算返回有界 partial。
- 本地图片仅已有 `mediaId` 才发内联 image，否则发 file；只上传不发送用 `conversation-file upload`。
- 子消息用自身 messageId；缺 CID 才继承父 conversationId。写操作沿真实结果传 ID。

## 写生命周期

- 多步骤写先校验目标、限制、内容、确认，再沿稳定 ID 串行执行并验证；不截断或换目标。临时资源保留 ID/阶段，失败仅做已授权清理。

## 上下文预算

- Skill/Reference/leaf Schema/Help 各读一次；Schema 已给参数禁 Help。总结/统计/检索默认 `--no-reactions`，资源下载仅显式 opt-in。
- 用 `--fields`/`--jq`/limit/compact 预览，禁 `head` 截 JSON；大正文落文件，保留 ID、正文证据、下步字段、完整性和失败项。

## 按需加载

任务≤1个

| 场景 | Reference |
|---|---|
| 消息 | [查询](references/chat/message-query.md) / [动作](references/chat/message-actions.md) / [资源](references/chat/message-media.md) |
| 群 | [读取](references/chat/group-discovery.md) / [治理](references/chat/group-admin.md) |
| 会话/Bot | [会话](references/chat/chat-conversation.md) / [Bot](references/chat/chat-bot.md) |
| 组合/话题/表情/卡片 | [组合](references/01-messaging.md) / [话题](references/chat/thread.md) / [表情](references/chat-emoji-list.md) / [卡片](references/card/create.md) |
| 结果/其他原子能力 | [contracts](references/contracts.md) / [chat](references/chat.md) |

## 错误最短路径

1. 零命中/多候选：停止写并消歧，禁默认第一项。
2. 参数/确认不明查 leaf Schema；Schema 不可用才查已知 leaf Help。`unknown flag` 用同 leaf Help 修正一次；`unknown command` 不查 Help：先用错误的明确 suggestion，再用已加载 Skill/reference 的明确兼容入口；无则报漂移并停，禁全 Catalog。
3. 仅 `retryable=true` 按原命令重试一次；API/internal/MCP、写后不一致或无重试性即停，不换 Shortcut/atomic。保留 Trace ID、已完成/待清理项并最终答复；`confirmation_required` 等用户确认，不补 `--yes`。
