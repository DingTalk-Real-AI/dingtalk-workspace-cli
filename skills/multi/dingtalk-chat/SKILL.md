---
name: dingtalk-chat
description: 钉钉群聊与消息。Use when 用户提到 发消息/编辑或撤回消息/单聊/群聊/建群/普通群升级外部群/群昵称/会话分组/群成员管理/@消息/搜索聊天记录/话题回复/收藏消息/机器人群发/Webhook通知/发送或下载消息图片与文件。不做紧急 DING/短信/电话（走 dingtalk-misc）、邮件（走 dingtalk-mail）、班级群（走 dingtalk-misc）。命令前缀：dws chat。
metadata:
  cli_version: ">=0.2.14"
  category: product
  requires:
    bins:
      - dws
---

# 钉钉群聊 / 消息 Skill

<!-- DWS_RUNTIME_CONTRACT_START -->
## 最小 DWS 执行契约

- 只通过 `dws` CLI 操作钉钉；结构化读取使用 `--format json`，按真实返回判断结果。
- 已知命令直接执行。只有 leaf 参数或安全语义不确定时读取精确 Schema，只有 Cobra flag 不确定时读取精确 leaf Help；不要加载产品级 Catalog 代替选路。
- 不猜命令、flag、字段、ID、账号或时间。后续 ID 必须来自真实返回；零命中、多候选或类型不明时停止并消歧。
- 解析目标、读取上下文和最终执行必须使用同一 profile；不得跨组织复用 userId、openDingTalkId 或 openConversationId。多账号组织只使用明确的 `isOrgCurrent=true` 默认账号；没有默认账号时要求用户指定，禁止选择第一项、最近登录或最近使用账号。
- 不输出或记录 token、refresh token、appSecret、webhook token 等凭据；宿主已注入认证时不要索要凭据。
- 写操作必须符合用户明确意图。是否需要确认以最终 Runtime gate 和 Schema 为准；需要确认时先说明对象、动作与影响，再追加 `--yes`。
- 写后按任务结果契约验证；不能仅凭退出码宣称成功。部分结果、未知投递状态和失败项必须如实保留。
- 时间戳面向用户展示时转换为带时区的可读时间；默认使用当前会话时区，必要时同时保留原值。
- 遇到认证、权限、profile、confirmation 或未知错误时，只加载 `dingtalk-shared` 中对应 reference；不要连续猜测替代命令。
<!-- DWS_RUNTIME_CONTRACT_END -->

<!-- VISIBLE_SHORTCUTS_START -->
## Shortcut 发现（按需）

`chat` 当前有 98 条公开 shortcut，完整清单保留在 Runtime Catalog 与 Schema，不在高频产品根 Skill 中重复展开。已知意图直接使用下方的优先路由、意图表或任务 reference；命令已选中时直接执行，只在参数/安全语义不确定时读取 leaf Schema，在当前 Cobra flags 不确定时读取 leaf Help。

仅当现有路由和 reference 都无法定位低频能力时，才执行 `dws shortcut list --service chat --format json` 做最后回退；不要为已知高频意图加载完整 Shortcut Catalog 或产品级 Schema。
<!-- VISIBLE_SHORTCUTS_END -->

## Golden Route

按用户任务选择最小充分入口。公开层按意图分流；Resolver、发送执行、消息投影和错误契约在 Runtime 内复用，不把所有能力塞进一个万能命令。

| 用户意图 | 唯一推荐入口 | 关键边界 |
|---|---|---|
| <!-- dws-intent: chat.send.dm -->按姓名发简单文本或 Markdown | `dws chat +dm --to <姓名> --text <内容>` | CLI 解析唯一用户；多候选时停止，不先手工查 ID |
| <!-- dws-intent: chat.send.group -->按群名或 ID 发简单文本或 Markdown | `dws chat +send-to-group --group <群名或ID> --text <内容>` | 稳定 ID 直接使用；群名多候选时停止 |
| <!-- dws-intent: chat.send.advanced -->文件、Bot、Webhook、复杂 @ 或高级发送 | `dws chat +messages-send` | Bot 多群用 `--groups/--groups-file` 并检查逐项 ledger |
| <!-- dws-intent: chat.read.conversation -->读取指定会话、返回较多消息 | `dws chat +chat-messages` | 粗粒度读取；目标条件明确时优先 `+search-msg` |
| <!-- dws-intent: chat.search.filtered -->多维度条件搜索（发送者/关键词/@/类型，单/跨会话） | `dws chat +search-msg` | 目标条件明确时使用 |
| 查看指定群成员（用户/机器人） | `dws chat +chat-members-list --group <群名或ID>` | 唯一解析并全量读取 |
| 获取群邀请链接 | `dws chat +chat-invite-url --group <群名或ID>` | 多候选时停止 |
| 查看群机器人 | `dws chat +chat-bots --group <群名或ID>` | 返回稳定 `bots[]` |
| 修改群名称 | `dws chat group rename --id <openConversationId> --name <新名称>` | 只知群名时先用 `+chat-search --query <群名>` 唯一解析 ID；不猜 `+chat-rename` |
| 查看指定群内 @我的消息 | `dws chat +at-me --group <群名> --page-all` | 检查 `complete`；空结果仍返回数组 |
| 查看全部会话 | `dws chat +conversation-list --page-all` | 检查 `complete` / `failures` |
| 读取并下载消息资源 | 查询命令加 `--download-resources` | 不另起手工下载循环；下载失败项保留在结果中 |
| <!-- dws-intent: chat.conversation.list-top -->查看置顶会话 | `dws chat +conversation-list-top` | 会话 Top 与消息 Pin、消息 Top、Favorite 不同 |
| 监听未来 IM 事件 | [`dingtalk-event`](../dingtalk-event/SKILL.md) | 常规监听走 `+listen-im`；生命周期/高级控制走 `consume` |

以下次级入口在意图明确时直接使用，不需要先加载完整 Catalog：

| 用户意图 | 入口 |
|---|---|
| 已知消息 ID 批量读取详情 | `dws chat +messages-mget` |
| 已知资源引用单独下载 | `dws chat +messages-resource-download` |
| 按关键词搜索群 | `dws chat +chat-search` |
| 查看消息收藏 | `dws chat +flag-list` |
| <!-- dws-intent: chat.reply.quote -->引用回复 | `dws chat +messages-reply`；成功结果保留新消息/会话/投递与原消息来源上下文 |
| 撤回当前用户消息 | `dws chat +messages-recall --msg-id <openMessageId>`；可省略会话 ID，由 CLI 只读补齐；兼容单值 `--message-ids` |
| 已知话题主消息 ID 或 thread/topic ID 读取回复 | `dws chat +thread-replies` |
| <!-- dws-intent: chat.create.group -->按成员 ID 或姓名创建群聊 | `dws chat +chat-create`；成员/群主均可自然解析，任一歧义都会在创建前整体停止 |
| 跨全部会话查看 @我的消息 | `dws chat +at-me --page-all` |

### 发送入口边界

- `+dm`：姓名目标的简单文本/Markdown，参数空间最小。
- `+send-to-group`：群名或稳定 ID 目标的简单文本/Markdown，避免暴露无关身份矩阵。
- Markdown 中的公网图片必须写成 `![图片标题](https://example.com/image.png)` 才会内联展示；
  省略开头的 `!` 时只会显示为链接。
- `+messages-send`：文件、Bot、Webhook、复杂 @ 或幂等控制。user 已知 ID 可直接传，也可用 `--user-query` / `--chat-query` 运行同一只读解析链；Bot 多群使用 `--groups/--groups-file`，返回 `im.batch-write.v1`；bot/webhook 只使用下层真实支持的文本/Markdown 能力。
- 文件直接传 `+messages-send --file <相对路径>`；不要先独立上传并提取 mediaId。
- Webhook 使用 `+messages-send --as webhook --webhook-token <token>`；不要退回原子 Webhook 命令。
- 流式卡片用 `+messages-send-card`；群聊@传 ID/`--at-all`，Runtime 把 create 返回前缀加到 `--content`；禁写占位符；仅 text。

## 关键结果语义

- `openTaskId` 是发送任务 ID，不是回复或撤回所需的消息 ID；消息 ID 必须来自真实查询结果。
- 消息查询默认保留稳定 ID、会话/thread、发送者、文本、时间、reaction、引用、转发和 `resourceRefs`；`--no-reactions` 可关闭 reaction。
- 查询结果必须检查 `complete`、`hasMore`、`failures` 和资源下载 ledger；partial result 不得表述为完整成功。
- 子消息使用自己的 `messageId`；仅缺会话 ID 时继承父消息的 `conversationId`。
- 下载只允许工作目录内安全相对路径，默认不覆盖并原子落盘；覆盖必须由用户显式传 `--overwrite`。读取和下载不需要 `--yes`。
- Favorite、消息 Pin、消息 Top、会话 Top 是不同对象层级，不能互换。

## 按需加载

只在任务命中时读取一个精确 reference：

| 场景 | Reference |
|---|---|
| 需要跨步骤传递真实结果的消息/群组合流程 | [01-messaging.md](references/01-messaging.md) |
| 消息读取与查询 | [message-query](references/chat/message-query.md) |
| 编辑、撤回、回复、转发、Pin、Top、Favorite 或 reaction 写入 | [message-actions](references/chat/message-actions.md) |
| 位置、联系人名片、底层媒体与资源下载 | [message-media](references/chat/message-media.md) |
| 群列表、群搜索、共同群、成员与群内机器人读取 | [group-discovery](references/chat/group-discovery.md) |
| 建群、成员或已知机器人增删、管理员、公告与群设置 | [group-admin](references/chat/group-admin.md) |
| 搜索未知机器人、机器人消息发送/撤回与 Webhook | [chat-bot.md](references/chat/chat-bot.md) |
| 会话置顶、分类、红点、免打扰和隐藏 | [chat-conversation.md](references/chat/chat-conversation.md) |
| 低频意图之间仍需消歧 | [intent-guide.md](references/intent-guide.md) |
| 表情名称与 ID | [chat-emoji-list.md](references/chat-emoji-list.md) |
| 稳定结果、身份矩阵与能力边界 | [contracts.md](references/contracts.md) |
| 流式卡片创建 | [card/create.md](references/card/create.md) |
| 流式卡片更新 | [card/update.md](references/card/update.md) |
| 卡片 callback 是否可用 | [card/callback.md](references/card/callback.md) |
| 卡片公开 Schema 边界 | [card/schema.md](references/card/schema.md) |
| 只有上述 reference 仍无法定位的原子能力 | [chat.md](references/chat.md) 的对应章节 |

不要预加载这些 reference。完整 Shortcut Catalog 只在根路由和精确 reference 都无法定位低频能力时使用。

## 错误最短路径

1. resolution 返回零命中或多候选：停止写操作，展示候选并让用户消歧；禁止默认第一项。
2. `unknown command` / `unknown flag`：读取精确 leaf Help，修正后最多重试一次。
3. 参数约束或 confirmation 不清楚：读取精确 leaf Schema，以 Runtime gate 为准。
4. 认证、权限、profile 或 confirmation 错误：读取 `dingtalk-shared` 的对应 reference；正常 IM 不读取完整 shared Skill。
5. `backend_dependency_unavailable`：保持原参数，对只读命令最多重试一次；不要改 flag、猜认证命令或切换同义原子命令，持续失败时保留 Trace ID。
6. 其他错误：保留真实错误和已完成/失败项；不要连续尝试同义原子命令。
