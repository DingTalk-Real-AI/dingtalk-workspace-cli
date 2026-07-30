# 会话与群聊 (chat) 命令参考

> **渐进式文档**：本文件为路由层（索引 + 意图判断 + 全局约束），详细参数、示例和踩坑说明在 [chat/](./chat/) 目录下按场景加载。

> 命令别名：`dws im` 等价于 `dws chat`。

## Shortcut 优先路由

精确脚本 / recipe 未覆盖时，常见 Agent 意图优先使用公开 `+` Shortcut；原子命令保留给需要特定原始返回结构、兼容参数或 Shortcut 未覆盖字段的场景。先尝试 `dws shortcut list --service chat --compact --format json` 动态发现；当前 CLI 报 `unknown flag: --compact` 时，立即去掉 `--compact` 重试。选中后读取 `dws schema --cli-path "<cli_path>" --format json`；若 leaf 暂未进入 Schema，则以完整 Catalog 中同一 `cli_path` 的契约为准，最后用 `dws <cli_path> --help` 核对真实 flags。

| 意图 | 首选 |
|---|---|
| 以 current-user / bot / webhook 身份发消息 | `dws chat +messages-send --as <identity> ...` |
| 拉取单个群聊或单聊的消息 | `dws chat +chat-messages ...` |
| 按关键词、发送者、@对象、会话、类型或时间组合搜索 | `dws chat +search-msg ...` |
| 查询 @ 我的消息 | `dws chat +at-me ...` |
| 根据消息 ID 批量取详情与 reaction | `dws chat +messages-mget ...` |
| 读取已知 thread/topic 的全部回复 | `dws chat +thread-replies ...` |
| 下载单个 mediaId/fileId | `dws chat +messages-resource-download ...` |
| 创建并按需立即更新流式卡片 | `dws chat +messages-send-card ...` |

- `+messages-send` 只暴露下层真实支持的身份能力：user 支持文本/Markdown、已有 mediaId 图片、本地文件和幂等键；bot 支持群聊或批量单聊文本/Markdown；webhook 的目标由 token 所在群决定。
- `+messages-send` 自动规范化并补齐对应身份的 @ 占位符。声明 `--at-*` / `--at-all` 即可，不要为统一 Shortcut 手工拼 `@10`。
- `+search-msg --page-all` 连续翻页并默认按消息 ID 批量富化；续页或富化失败会保留已取得结果并返回逐项失败 ledger。
- `+at-me`、`+chat-messages`、`+messages-mget`、`+search-msg`、`+thread-replies` 可用 `--download-resources` 下载资源。引用、回复、合并转发中的资源使用结果 `resourceRefs` 自带的子消息 `messageId`；仅当子消息缺会话 ID 时继承父消息 `openConversationId`。
- 上述五个查询 Shortcut 与 `+messages-resource-download` 都是 `read/not_required`，不要添加 `--yes`。下载只允许工作目录内相对路径、默认不覆盖并原子落盘；需要覆盖时必须由用户显式传 `--overwrite`。
- 下载器只接受经审查的钉钉与公网 OSS HTTPS 地址并逐跳校验重定向；跨主机时不会转发下层提供的请求头。

## 适用范围与安全硬约束

`chat` 覆盖钉钉会话、群聊、群成员、会话消息、机器人消息、Webhook、会话状态和群身份管理。

**发消息前参数审查（必须执行）**：

- 发消息类命令包括 `chat +messages-send`、`+messages-send-card`，以及原子 `message send`、`send-by-bot`、`send-by-webhook`、`send-card`、`reply`、`forward`。
- 执行前必须逐项核对接收对象、群/人、消息内容、@ 对象、消息类型、附件路径，确认全部来自用户原始需求。
- 用户没说发给谁、群名/人名可能匹配多个对象、消息文本由 Agent 组织、是否 @ 某人不明确时，必须先确认，严禁自行补全。
- 重试发送时，Shortcut 复用同一个 `--idempotency-key`，原子命令复用同一个 `--uuid`，避免重复投递。

**文件、图片、音频、视频发送硬规则**：

- 新场景发送本地文件、音频、视频优先用 `dws chat +messages-send --as user --msg-type file|audio|video --file <本地路径>`；原子回退用 `message send --file-path`。两条链路都由 CLI 完成上传和发送，`audio` / `video` 底层按 `file` 处理。
- 不要先走 `dt_media_upload`、`extract_media_id.py`、`drive upload`；`--msg-type image --media-id` 只保留给已有 mediaId 的旧链路。

**Markdown 换行硬规则**：

- `--text` 必须使用真实换行符 `U+000A`，不能传字面量 `\n`。
- Markdown 单个换行通常不换行，需要空行 `\n\n`、行尾两个空格 + 换行，或 `<br>`。
- 机器人 `message send-by-bot` 的 Markdown 文本优先用空行分隔段落；如果在模型内部用转义形式组织文本，必须写 `\n\n`，不要只写 `\n`。

## 查询命令帮助

参数名、必填项或命令是否存在不确定时，先查 `--help`，不要猜。

```bash
dws chat --help
dws chat message --help
dws chat message send --help
dws chat group --help
dws chat bot --help
```

## Reference 索引

| Reference | 何时读取 | 主要命令 |
|-----------|----------|----------|
| [chat-message](./chat/chat-message.md) | 发消息、查消息、撤回、搜索、已读、话题、Pin/Top/Favorite、表情、卡片、文件、音频、视频、位置、名片、文本翻译 | `message *`、`file upload`、`download-media`、`text translate` |
| [chat-group](./chat/chat-group.md) | 建群、搜群、成员、机器人进群、群设置、群主、入群审批、群身份、群公告、群邀请分享 | `group *`、`group-role *`、`search`、`search-common` |
| [chat-bot](./chat/chat-bot.md) | 查机器人、机器人发消息/撤回、Webhook、机器人单聊、机器人进群 | `bot *`、`message send-by-bot`、`recall-by-bot`、`send-by-webhook` |
| [chat-conversation](./chat/chat-conversation.md) | 会话列表、置顶、免打扰、隐藏、红点、已读未读、清空、会话分组、智能分组 | `conversation-info`、`category *`、`mute`、`set-top`、`mark-read` |
| [chat-workflows](./chat/chat-workflows.md) | 高频复合流程、上下文传递表、openDingTalkId 获取方式、自动化脚本 | 工作流与字段传递 |

## ID 获取速查

| 需要的 ID | 首选获取方式 | 说明 |
|-----------|--------------|------|
| 群聊 `openConversationId` | `dws chat search --query "群名" --format json` | 群聊发消息、拉消息、成员管理、会话状态都用它 |
| 数字群号对应的 `openConversationId` | `dws chat group get-by-group-id --group-id <数字群号>` | 用户只给数字群号时先转换 |
| 单聊 `openConversationId` | `dws chat conversation-info --user <userId>` 或 `--open-dingtalk-id <openDingTalkId>` | 单聊转发、会话置顶、清红点等状态命令需要 |
| `userId` | `dws aisearch person --keyword "姓名" --dimension name --format json` | 内部人员、机器人单聊 users、按发送者 userId 搜索 |
| `openDingTalkId` | `dws aisearch person --keyword "姓名" --dimension name --format json` | 三方/跨组织、openDingTalkId 单聊、@ openDingTalkId |
| `robotCode` | `dws chat bot search --name "机器人名" --format json` | 我创建的机器人，发机器人消息和撤回使用 |
| 机器人 `openDingTalkId` | `dws chat bot find --query "机器人名" --format json` | 给机器人发单聊必须用 find，search 不返回该字段 |
| 群成员昵称 | `dws chat group members --id <openConversationId> --format json` | 返回中包含成员在群里的昵称/展示名；不要只查 contact 个人名 |
| 消息 `openMessageId` / `openMsgId` | `dws chat message list ... --format json` | 撤回、已读状态、回复、转发、表情回应 |
| 话题 `openConvThreadId` | `dws chat message list ... --format json` | 拉话题回复或往话题内回复；禁止自行拼接 |
| 卡片 `bizId` | `dws chat +messages-send-card ... --format json`（不传 `--content`）或原子 `message send-card` | `message update-card --biz-id` |

## 易混淆路由

| 易混淆说法 | 正确路由 | 不要这样做 |
|------------|----------|------------|
| “置顶会话” | `chat set-top` 或 `chat list-top-conversations` | 不要用 `message set-top-msg` |
| “置顶某条消息” | `chat message set-top-msg` | 不要用 `chat set-top` |
| “机器人发消息” | 优先 `chat +messages-send --as bot`；原子回退 `chat message send-by-bot` | 不要用当前用户身份代发 |
| “给机器人发单聊” | `chat bot find` 取 openDingTalkId → `message send --open-dingtalk-id` | 不要用 `bot search`，它没有 openDingTalkId |
| “发图片/文件/音频/视频” | 优先 `+messages-send --as user --msg-type file|audio|video --file`；原子回退用 `message send --file-path`；已有图片 mediaId 才用 `image` | 不要先 `drive upload` 或 `dt_media_upload` |
| “特别关注的人最近发了什么” | `message list-focused` | 不要先查 `contact relation list-my-followings` |
| “某人发给我的消息” | `message list-by-sender` | 不要只查单聊，结果应覆盖群聊+单聊 |
| “搜索消息” | 优先 `chat +search-msg`；需要原始返回或未覆盖字段时回退 `message search-advanced` | 不要在可组合条件下退回简单 `message search` |
| “消息转待办” | 先用 `chat +chat-messages` / `+search-msg` 取消息内容，再调用 `dws todo task create` | 不要在 chat 内寻找“转待办”命令 |
| “消息转日程” | 先用 `chat +chat-messages` / `+search-msg` 取消息内容，再调用 `dws calendar event create` | 不要在 chat 内寻找“转日程”命令 |
| “共同群” | `chat search-common --nicks` | 不要分别搜索群再手工求交 |
| “拉取群文件 / 群文件列表” | `chat conversation-info --group <openConversationId>` 取 `spaceId` → `dws drive list --space-id <spaceId>` | 不要用 `chat file upload`，它只负责上传 |
| “清空会话聊天记录” | `chat clear-messages` | 不要理解为删除群消息；它只影响当前用户视角 |

## 发送前检查清单

执行任何发送类命令前，至少检查：

1. 接收对象明确：群聊必须有 `openConversationId`，单聊必须有 `userId` 或 `openDingTalkId`。
2. 身份明确：统一 Shortcut 显式传 `--as user/bot/webhook`；原子回退分别使用 `message send`、`send-by-bot`、`send-by-webhook`。
3. 内容明确：消息正文、标题、附件、卡片内容都能从用户原始需求中找到依据。
4. @ 对象明确：`--at-all`、`--at-user-ids`、`--at-open-dingtalk-ids` 不可自行追加。
5. 文件路径明确：统一 Shortcut 用 `--file`，原子回退用 `--file-path`；并确认路径是用户提供或当前任务生成的目标文件。
6. 重试策略明确：Shortcut 复用同一个 `--idempotency-key`，原子命令复用同一个 `--uuid`，避免重复投递。

## 高频一跳命令

这些命令覆盖多数日常问题；复杂参数再跳到对应子文档。

| 场景 | 一跳命令 |
|------|----------|
| 找群 | `dws chat search --query "群名" --format json` |
| 建群 | `dws chat group create --name "群名" --users userId1,userId2 --format json` |
| 看群成员 | `dws chat group members --id <openConversationId> --format json` |
| 看群内昵称 | `dws chat group members --id <openConversationId> --format json` |
| 发群文字 | `dws chat +messages-send --as user --chat-id <openConversationId> --text "内容" --idempotency-key <key> --format json` |
| 发单聊文字 | `dws chat +messages-send --as user --user <userId> --text "内容" --idempotency-key <key> --format json` |
| 发本地文件 | `dws chat +messages-send --as user --chat-id <openConversationId> --msg-type file --file ./file.pdf --format json` |
| 发音频/视频 | `dws chat +messages-send --as user --chat-id <openConversationId> --msg-type audio --file ./voice.mp3 --format json` / `--msg-type video --file ./demo.mp4` |
| 发位置/名片 | `dws chat message send --group <openConversationId> --msg-type location ...` / `--msg-type profile --contact-id <openDingTalkId>` |
| 拉群消息 | `dws chat +chat-messages --group <openConversationId> --time "2026-03-10 00:00:00" --direction older --format json` |
| 搜消息 | `dws chat +search-msg --query "关键词" --start <ISO> --end <ISO> --format json` |
| 查 @ 我的消息 | `dws chat +at-me --days 7 --format json` |
| 查未读会话 | `dws chat message list-unread-conversations --count 20 --format json` |
| 撤回自己消息 | `dws chat message recall --conversation-id <openConversationId> --msg-id <openMessageId> --format json` |
| 查我的机器人 | `dws chat bot search --name "机器人名" --format json` |
| 机器人发群消息 | `dws chat +messages-send --as bot --robot-code <robot-code> --chat-id <openConversationId> --title "标题" --markdown "## 标题\n\n正文" --format json` |
| Webhook 告警 | `dws chat +messages-send --as webhook --webhook-token <token> --title "告警" --text "内容" --format json` |
| 查单聊会话 ID | `dws chat conversation-info --user <userId> --format json` |
| 查群钉盘空间 | `dws chat conversation-info --group <openConversationId> --format json` |
| 发群公告 | `dws chat group notice create --group <openConversationId> --content "公告内容" --format json` |
| 分享群邀请 | `dws chat group share-invite --source <sourceOpenConversationId> --target <targetOpenConversationId> --format json` |
| 置顶会话 | `dws chat set-top --conversation-id <openConversationId> --format json` |
| 清红点 | `dws chat clear-red-point --conversation-id <openConversationId> --format json` |
| 会话分组 | `dws chat category list --format json` |
| 智能会话分组 | `dws chat category create-smart --name "重点群" --keywords "项目,重点" --format json` |
| 文本翻译 | `dws chat text translate --query "你好世界" --to en_US --format json` |

## 命令索引表

### 消息与文件

| 命令 | 用途 | 必填参数 | 详见 |
|------|------|----------|------|
| `message send` | 当前用户发群聊/单聊文本、Markdown、图片、文件、音频、视频、位置、名片 | `--group` / `--user` / `--open-dingtalk-id` 三选一；文本用 `--text`，本地文件/音视频用 `--msg-type file|audio|video --file-path` | [chat-message](chat/chat-message.md#发送消息) |
| `message list` | 拉取群聊或单聊消息 | `--time` + `--group` / `--user` / `--open-dingtalk-id` 三选一 | [chat-message](chat/chat-message.md#拉取消息) |
| `message list-direct` | 拉取与指定用户的单聊消息 | `--time` + `--user` / `--open-dingtalk-id` 二选一 | [chat-message](chat/chat-message.md#拉取消息) |
| `message list-all` | 指定时间范围内拉取当前用户全部会话消息 | `--start` `--end` `--limit` `--cursor` | [chat-message](chat/chat-message.md#拉取消息) |
| `message list-by-sender` | 跨单聊和群聊查指定发送者消息 | `--sender-user-id` / `--sender-open-dingtalk-id` 二选一 | [chat-message](chat/chat-message.md#拉取消息) |
| `message list-mentions` | 查 @ 我的消息 | 可选 `--group`、`--start`、`--end` | [chat-message](chat/chat-message.md#拉取消息) |
| `message list-focused` | 查特别关注人消息 | 无 | [chat-message](chat/chat-message.md#拉取消息) |
| `message list-unread-conversations` | 获取未读会话列表 | 可选 `--count` | [chat-conversation](chat/chat-conversation.md#会话列表与红点) |
| `message search-advanced` | 原子多维度搜索回退；支持消息类型、会话类型、机器人消息过滤 | 至少一个搜索条件 | [chat-message](chat/chat-message.md#搜索消息) |
| `message search` | 简单关键词搜索消息 | `--query` | [chat-message](chat/chat-message.md#搜索消息) |
| `message read-status` | 查询消息已读/未读状态 | `--conversation-id` `--message-id` | [chat-message](chat/chat-message.md#消息状态与撤回) |
| `message query-send-status` | 查询发送任务状态 | `--open-task-id` | [chat-message](chat/chat-message.md#消息状态与撤回) |
| `message recall` | 撤回当前用户消息；群主/管理员可撤回群内他人消息 | `--conversation-id` `--msg-id` | [chat-message](chat/chat-message.md#消息状态与撤回) |
| `message edit` | 编辑已发送消息内容 | `--conversation-id` `--msg-id`，`--text` / `--content` 二选一 | [chat-message](chat/chat-message.md#消息状态与撤回) |
| `message reply` | 引用回复消息 | `--conversation-id` `--ref-msg-id` `--ref-sender` `--text` | [chat-message](chat/chat-message.md#回复与转发) |
| `message forward` | 转发单条消息 | `--src-conversation-id` `--msg-id` `--dest-conversation-id` | [chat-message](chat/chat-message.md#回复与转发) |
| `message combine-forward` | 合并转发多条消息 | `--src-conversation-id` `--msg-ids` `--dest-conversation-id` | [chat-message](chat/chat-message.md#回复与转发) |
| `message list-topic-replies` | 拉取话题回复 | `--group` `--topic-id` | [chat-message](chat/chat-message.md#话题与卡片) |
| `message forward-topic` | 转发话题消息 | 源消息、源会话、话题 ID、目标会话 | [chat-message](chat/chat-message.md#话题与卡片) |
| `message send-card` / `update-card` | 创建并流式更新卡片 | `send-card` 目标二选一；`update-card` 需 `--biz-id` | [chat-message](chat/chat-message.md#话题与卡片) |
| `message add-emoji` / `remove-emoji` | 添加/移除默认表情回应 | `--conversation-id` `--msg-id` `--emoji` | [chat-message](chat/chat-message.md#表情回应) |
| `message add-text-emotion` / `update-text-emotion` / `remove-text-emotion` | 添加/更新/移除文字表情 | update 需 `--old-emotion-id` `--emotion-id`，其余用 `--emotion-*` | [chat-message](chat/chat-message.md#表情回应) |
| `message create-text-emotion` | 创建文字表情模板 | `--emotion-name` `--text` | [chat-message](chat/chat-message.md#表情回应) |
| `message list-emotion-replies` | 批量拉取消息表情回复和文字回复 | `--msg-ids` | [chat-message](chat/chat-message.md#表情回应) |
| `file upload` | 上传本地或 URL 文件到会话文件空间 | 目标三选一 + `--file` / `--url` 二选一 | [chat-message](chat/chat-message.md#文件与媒体) |
| `message download-media` | 下载消息中的图片/视频/语音等资源 | `--type` `--resource-id` `--message-id` `--open-conversation-id` `--output` | [chat-message](chat/chat-message.md#文件与媒体) |
| `text translate` | 翻译文本内容 | `--query` `--to` | [chat-message](chat/chat-message.md#文本工具) |

### 群聊与群身份

| 命令 | 用途 | 必填参数 | 详见 |
|------|------|----------|------|
| `search` | 按关键词搜索群聊 | `--query` | [chat-group](chat/chat-group.md#搜索与基础信息) |
| `search-common` | 查询指定人共同群 | `--nicks` | [chat-group](chat/chat-group.md#搜索与基础信息) |
| `group create` | 创建内部群/外部群/普通群/话题圈 | `--name` `--users` | [chat-group](chat/chat-group.md#群创建与基础操作) |
| `group members` | 查看群成员 | `--id` | [chat-group](chat/chat-group.md#成员与机器人) |
| `group members add/remove` | 添加/移除群成员 | `--id` `--users` | [chat-group](chat/chat-group.md#成员与机器人) |
| `group members add-bot/remove-bot` | 添加/移除群机器人 | `--id` + 机器人标识 | [chat-group](chat/chat-group.md#成员与机器人) |
| `group members list-by-ids` | 批量查群成员详情 | `--id` `--users` | [chat-group](chat/chat-group.md#成员与机器人) |
| `group rename` | 修改群名称 | `--id` `--name` | [chat-group](chat/chat-group.md#群创建与基础操作) |
| `group get-by-group-id` | 数字群号转 openConversationId | `--group-id` | [chat-group](chat/chat-group.md#搜索与基础信息) |
| `group transfer-owner` | 转让群主 | `--group` + `--user` / `--new-owner` | [chat-group](chat/chat-group.md#群设置与权限) |
| `group upgrade-to-external` | 仅将 NORMAL_GROUP 普通群升级为外部群 | `--group` `--yes`，可选 `--extension` | [chat-group](chat/chat-group.md#群设置与权限) |
| `group invite-url` | 获取群邀请链接 | `--group` | [chat-group](chat/chat-group.md#群设置与权限) |
| `group share-invite` | 将群邀请链接分享到另一个会话或单聊用户 | `--source` + `--target` / `--receiver` 二选一 | [chat-group](chat/chat-group.md#群设置与权限) |
| `group notice *` | 群公告创建、修改、详情、列表 | `--group`，视子命令补充公告参数 | [chat-group](chat/chat-group.md#群公告) |
| `group quit` / `dismiss` | 退出/解散群聊 | `--group` | [chat-group](chat/chat-group.md#群创建与基础操作) |
| `group update-icon` | 更新群头像 | `--group` `--icon-media-id` | [chat-group](chat/chat-group.md#群设置与权限) |
| `group update-settings` | 更新管理员级别的群功能开关 | `--group` `--setting-key` `--status` | [chat-group](chat/chat-group.md#群设置与权限) |
| `group user-settings query/set` | 批量查询/更新当前用户自己的群会话设置 | `query --groups` / `set --items` | [chat-group](chat/chat-group.md#群设置与权限) |
| `group update-nick` / `update-alias` | 设置或清除群昵称 / 设置群备注 | `update-nick` 需 `--group`，`--nick` 可选；`update-alias` 需备注 | [chat-group](chat/chat-group.md#群设置与权限) |
| `group set-history` | 设置新成员可见历史消息范围 | `--group` `--option` | [chat-group](chat/chat-group.md#群设置与权限) |
| `group get-mute-config` | 查询群用户禁言配置 | `--group` | [chat-group](chat/chat-group.md#群设置与权限) |
| `group list-my-groups` / `list-all` | 拉取我管理的群 / 我加入的所有群 | 可选分页参数 | [chat-group](chat/chat-group.md#群列表与入群审批) |
| `group list-join-validations` | 拉取入群验证记录 | 可选分页参数 | [chat-group](chat/chat-group.md#群列表与入群审批) |
| `group audit-join-validation` | 审批入群验证 | `--group` `--record-id` `--applicant` `--inviter` `--status` | [chat-group](chat/chat-group.md#群列表与入群审批) |
| `group-mute` / `group-mute-member` | 全员禁言 / 指定成员禁言 | `--group`，成员禁言还需用户与时长 | [chat-group](chat/chat-group.md#群设置与权限) |
| `group set-admin` | 设置/取消群管理员 | `--group` `--user` / `--users` | [chat-group](chat/chat-group.md#群设置与权限) |
| `group-role *` | 群身份列表、增删改、设置成员身份 | `--group` + role/user 参数 | [chat-group](chat/chat-group.md#群身份) |

### 机器人与 Webhook

| 命令 | 用途 | 必填参数 | 详见 |
|------|------|----------|------|
| `bot search` | 搜索我创建的机器人 | 可选 `--name` | [chat-bot](./chat/chat-bot.md#机器人搜索) |
| `bot find` | 搜索全部可用机器人，返回 openDingTalkId | `--query` | [chat-bot](./chat/chat-bot.md#机器人搜索) |
| `message send-by-bot` | 机器人发群聊或单聊消息 | `--robot-code` `--title` `--text` + 目标二选一 | [chat-bot](./chat/chat-bot.md#机器人发送与撤回) |
| `message recall-by-bot` | 撤回机器人消息 | `--robot-code` `--keys` | [chat-bot](./chat/chat-bot.md#机器人发送与撤回) |
| `message send-by-webhook` | 自定义机器人 Webhook 发群消息 | `--token` `--title` `--text` | [chat-bot](./chat/chat-bot.md#webhook) |
| `group members add-bot/remove-bot` | 机器人加入/移出群 | `--id` + 机器人标识 | [chat-bot](./chat/chat-bot.md#机器人进群) |
| `group bots` | 查看群内所有机器人 | `--group` | [chat-bot](./chat/chat-bot.md#机器人进群) |

### 会话状态与分组

| 命令 | 用途 | 必填参数 | 详见 |
|------|------|----------|------|
| `conversation-info` | 获取群聊或单聊会话基础信息 | `--group` / `--user` / `--open-dingtalk-id` 三选一 | [chat-conversation](./chat/chat-conversation.md#会话基础信息) |
| `list-all-conversations` | 分页获取全部会话 | 可选分页参数 | [chat-conversation](./chat/chat-conversation.md#会话列表与红点) |
| `list-top-conversations` | 拉取置顶会话列表 | 可选分页参数 | [chat-conversation](./chat/chat-conversation.md#会话列表与红点) |
| `set-top` | 设置/取消会话置顶 | `--conversation-id` | [chat-conversation](./chat/chat-conversation.md#会话置顶与通知) |
| `mute` | 开启/关闭会话免打扰 | `--conversation-id` | [chat-conversation](./chat/chat-conversation.md#会话置顶与通知) |
| `hide` | 隐藏会话 | `--conversation-id` | [chat-conversation](./chat/chat-conversation.md#会话置顶与通知) |
| `mute-at-all` / `mute-red-envelope` | 关闭/恢复 @所有人或红包通知 | `--conversation-id` | [chat-conversation](./chat/chat-conversation.md#会话置顶与通知) |
| `mark-unread` / `mark-read` | 标记会话未读 / 标记消息已读 | 会话 ID；`mark-read` 还需消息 ID | [chat-conversation](./chat/chat-conversation.md#已读未读与清理) |
| `clear-red-point` / `clear-all-red-point` | 清除单会话/全部红点 | 单会话需 `--conversation-id` | [chat-conversation](./chat/chat-conversation.md#会话列表与红点) |
| `clear-messages` | 清空当前用户视角会话消息 | `--conversation-id`；确认后传 `--yes` | [chat-conversation](./chat/chat-conversation.md#已读未读与清理) |
| `category list-by-conv` | 拉取指定会话所属的用户自定义会话分组 | `--group` | [chat-conversation](./chat/chat-conversation.md#会话分组) |
| `category batch-info` | 批量拉取用户自定义会话分组信息 | `--category-ids` | [chat-conversation](./chat/chat-conversation.md#会话分组) |
| `category *` | 自定义会话分组和智能分组管理 | 视子命令而定 | [chat-conversation](./chat/chat-conversation.md#会话分组) |

## 意图判断

### 消息

| 用户说 | 路由 |
|--------|------|
| “发群消息 / 发到某群” | 优先 `chat +messages-send --as user --chat-id`，先用 `chat search` 找 `openConversationId` |
| “发单聊 / 发给某人” | 优先 `chat +messages-send --as user --user/--open-dingtalk-id`，人员信息见 `dingtalk-contact` |
| “发图片 / 发文件 / 发截图 / 发音频 / 发视频” | 优先 `chat +messages-send --as user --msg-type file|audio|video --file <本地路径>`；原子回退用 `message send --file-path` |
| “发位置 / 分享地址” | `chat message send --msg-type location`，需经用户确认经纬度和地址名 |
| “发联系人名片 / 分享联系人” | `chat message send --msg-type profile --contact-id <openDingTalkId>` |
| “机器人发消息 / 机器人群发” | 单次优先 `chat +messages-send --as bot`；多群广播优先专用脚本；原子回退 `chat message send-by-bot` |
| “撤回我发的消息” | `chat message recall` |
| “撤回机器人发的消息” | `chat message recall-by-bot` |
| “查某群聊天记录” | 单次优先 `chat +chat-messages --group`；完整导出优先专用脚本 |
| “查和某人的单聊记录” | 单次优先 `chat +chat-messages --user/--open-dingtalk-id`；完整历史优先专用脚本 |
| “某人发给我的消息 / 指定发送者消息” | `chat message list-by-sender`，跨单聊和群聊 |
| “我今天/最近所有消息” | `chat message list-all --start <ISO> --end <ISO>` |
| “@我的消息” | 优先 `chat +at-me`；组合过滤用 `chat +search-msg --at-me` |
| “特别关注的人最近发了什么” | `chat message list-focused`，不要先查关注人员列表 |
| “搜索消息里的关键词 / 多维度搜索” | 首选 `chat +search-msg`；原始接口字段未覆盖时回退 `chat message search-advanced` |
| “只搜文件消息 / 只搜群聊 / 只看机器人消息” | `chat +search-msg --message-type file --conversation-type group_chat --only-robot` 按需组合 |
| “翻译这段文字” | `chat text translate --query <文本> --to <语言代码>` |
| “把这条消息转待办 / 消息里提到的事项建待办” | 先取消息内容和相关人员，再按 `dingtalk-todo` 调 `dws todo task create`；标题、执行人、截止时间必须来自消息或用户确认 |
| “把这条消息转日程 / 消息里约的会建日程” | 先取消息内容、时间、地点、参会人，再按 `dingtalk-calendar` 调 `dws calendar event create`；时间不明确必须先确认 |
| “话题回复 / 往话题里回复” | 已知 thread/topic 时优先 `chat +thread-replies` 读取；发送回复仍用 `message send --group <openConvThreadId>` |
| “消息已读未读 / 谁看了消息” | `chat message read-status` |
| “置顶某条消息 / 取消消息置顶” | `chat message set-top-msg` / `unset-top-msg` |

### 群与成员

| 用户说 | 路由 |
|--------|------|
| “建群 / 创建群聊” | `chat group create` |
| “建外部群 / 建普通群 / 建话题圈” | `chat group create --type EXTERNAL` / `--type NORMAL` / `--thread` |
| “找群 / 搜索群” | `chat search --query` |
| “群成员 / 群里有谁” | `chat group members` |
| “群成员在群里的昵称 / 群昵称” | `chat group members --id <openConversationId>`，返回中包含群内昵称/展示名 |
| “拉人进群 / 踢人” | `chat group members add` / `remove` |
| “批量查群成员详情” | `chat group members list-by-ids` |
| “改群名 / 群头像 / 群备注 / 群昵称” | `group rename` / `update-icon` / `update-alias` / `update-nick` |
| “转让群主 / 设置管理员 / 禁言” | `group transfer-owner` / `group set-admin` / `group-mute*` |
| “分享群链接 / 群邀请发给某人或某群” | `chat group share-invite` |
| “发群公告 / 改群公告 / 查群公告” | `chat group notice create/edit/get/list` |
| “入群审批 / 入群验证记录” | `group list-join-validations` → `group audit-join-validation` |
| “群身份 / 群标签 / 给成员设置身份” | `group-role *` |
| “我和某人的共同群” | `chat search-common --nicks` |

### 机器人

| 用户说 | 路由 |
|--------|------|
| “查看我的机器人 / 我创建的机器人” | `chat bot search` |
| “找机器人 / 搜索全部可用机器人 / 给机器人发单聊” | `chat bot find`，需要 `openDingTalkId` |
| “加机器人到群 / 从群移除机器人 / 群里有哪些机器人” | `group members add-bot` / `remove-bot` / `group bots` |
| “Webhook 发告警” | 优先 `chat +messages-send --as webhook`；原子回退 `chat message send-by-webhook` |

### 会话状态

| 用户说 | 路由 |
|--------|------|
| “未读会话列表” | `chat message list-unread-conversations` |
| “置顶会话 / 查看置顶会话” | `chat list-top-conversations` |
| “把会话置顶 / 取消会话置顶” | `chat set-top` / `chat set-top --off` |
| “免打扰 / 关闭免打扰” | `chat mute` / `chat mute --off` |
| “隐藏会话” | `chat hide` |
| “清红点 / 全部已读” | `chat clear-red-point` / `chat clear-all-red-point` |
| “标记未读 / 标记已读” | `chat mark-unread` / `chat mark-read` |
| “清空聊天记录” | 优先 `chat +conversation-clear-messages`；确认用户同意后才执行并传 `--yes`；原子回退 `chat clear-messages` |
| “会话分组” | `chat category *` |
| “智能分组 / 按关键词或成员自动分组” | `chat category create-smart` |
| “群文件 / 拉取群文件列表” | `chat conversation-info --group <openConversationId>` 取 `spaceId`，再按 `dingtalk-drive` 调 `dws drive list --space-id <spaceId>` |

## 核心工作流索引

| 场景 | 入口 |
|------|------|
| 搜索群并拉消息 | [chat-workflows §群聊消息](./chat/chat-workflows.md#群聊消息) |
| 个人身份发送群聊/单聊/文件 | [chat-workflows §发送消息](./chat/chat-workflows.md#发送消息) |
| 机器人发消息后撤回 | [chat-workflows §机器人消息](./chat/chat-workflows.md#机器人消息) |
| 机器人不在群内时先邀请再发送 | [chat-workflows §机器人消息](./chat/chat-workflows.md#机器人消息) |
| 给机器人发单聊 | [chat-workflows §机器人消息](./chat/chat-workflows.md#机器人消息) |
| 发送图片 / 文件 / 音频 / 视频 | [chat-workflows §发送消息](./chat/chat-workflows.md#发送消息) |
| 流式卡片创建与更新 | [chat-message §话题与卡片](./chat/chat-message.md#话题与卡片) |
| 群公告发布与修改 | [chat-group §群公告](./chat/chat-group.md#群公告) |
| 消息转待办 | 先用 [chat-message](./chat/chat-message.md#拉取消息) 取消息内容，再读取 `dingtalk-todo` 创建待办 |
| 消息转日程 | 先用 [chat-message](./chat/chat-message.md#拉取消息) 取消息内容，再读取 `dingtalk-calendar` 创建日程 |
| 拉取群文件 | `conversation-info` 取 `spaceId`，再读取 `dingtalk-drive` 查询钉盘文件 |

## 上下文传递摘要

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `chat search` / `chat group create` / `chat group get-by-group-id` | `openConversationId` | `--group`、`--conversation-id`、群成员、消息发送/拉取 |
| `chat conversation-info` | 单聊 `openConversationId` | 单聊消息搜索、转发、会话状态操作 |
| `chat conversation-info --group` | `spaceId` | `drive list/info/download --space-id`，用于拉取群文件 |
| `chat group members` | 群成员昵称/展示名、`userId`、`openDingTalkId` | 识别群内发言人、@ 人、待办执行人、日程参会人 |
| `aisearch person` | `userId`、`openDingTalkId` | 单聊、@ 人、机器人单聊、按发送者搜索 |
| `chat bot search` | `robotCode` | `send-by-bot`、`recall-by-bot` |
| `chat bot find` | `openDingTalkId` | 给机器人发单聊 |
| `chat message send-by-bot` | `processQueryKey` | `recall-by-bot --keys` |
| `chat message send` | `openTaskId` | `query-send-status` |
| `chat message list` | `openMessageId`、`openMsgId`、`openConvThreadId` | 撤回、已读状态、表情、回复、转发、话题回复 |
| `chat message list/search-advanced` | 消息正文、发送人、时间 | 创建 `todo task` 或 `calendar event` 前的输入材料 |
| `chat message search(-advanced)` / `list-all` | `nextCursor` | 下一页 `--cursor` |
| `chat group-role list` | `openRoleId` | `group-role update/remove/set-user/remove-user` |
| `chat message send-card` | `bizId` | `message update-card --biz-id` |

完整字段传递表见 [chat-workflows](./chat/chat-workflows.md#上下文传递表)。

## 注意事项

- **发消息前参数审查（必须执行）**：
  - 发消息（`chat message send`、`send-by-bot`、`send-by-webhook`、`send-card`、`reply`、`forward`）是严肃操作，一旦发错人/发错群会导致严重问题，因此在执行发送之前，agent 必须对所有参数进行内部审查
  - 审查方式：将即将发送的**全部参数**（收件人/群、消息内容、@对象、消息类型等）与用户的**原始需求**逐一对比，确认每个参数都能从原始需求中找到明确依据
  - 如果存在任何不明确、有歧义或原始需求中未提及的参数（例如：用户没说发给谁、没说发到哪个群、消息内容与用户意图有出入、不确定是否需要 @某人等），**必须先向用户确认**，严禁自行假设或补全
  - 典型需要确认的场景：用户只说了"发个消息"但没指定群/人；用户的描述可匹配多个群或多个联系人；消息文本由 agent 组织而非用户原文提供时需确认措辞
- uuid 幂等参数（发消息最佳实践）：
  - 发消息时建议始终带上 `--uuid` 参数，传入用户自行生成的唯一标识（如 UUID v4），用于幂等控制
  - 如果发送失败需要重试，重试时 `--uuid` 必须与首次发送保持一致，服务端据此去重，避免重复发消息
  - 如果不传 `--uuid`，每次调用都视为新消息，重试可能导致消息重复发送
  - 此参数适用于 `chat message send`（群聊和单聊均支持）
- `--group` 为群聊会话 ID (openconversation_id)，可从群搜索或群聊信息中获取
- `chat message send` 推荐用 `--text` 传消息内容（含换行或特殊字符时必须使用），也支持一个位置参数；群聊用 `--group`，单聊用 `--user`（userId）或 `--open-dingtalk-id`（openDingTalkId），三者互斥；纯文本/Markdown 单聊传 `--user` 时直接走 userId 发送能力；`--at-all`、`--at-open-dingtalk-ids` 仅在 `--group` 群聊时生效；本地图片/文件/音视频统一用 `--msg-type file --file-path`，其中图片是可下载附件；`--msg-type image --media-id` 仅用于上游已经提供有效 mediaId 的内联图片
- `chat message list-all` 的四个参数（--start、--end、--limit、--cursor）每次请求都必须传递；翻页时用响应中的 nextCursor 值作为下次 --cursor
- `chat message list` 的 `--group`、`--user`、`--open-dingtalk-id` 三者互斥，必须且只能指定其一
- `chat message list-by-sender` 不需要指定单聊/群聊，返回结果自带会话类型标识；`--sender-user-id`（userId）与 `--sender-open-dingtalk-id`（openDingTalkId）二选一；时间用 `--start`/`--end`（ISO-8601），分页用 `--limit`/`--cursor`
- `chat message list-mentions` 可选 `--group` 指定群聊，不传则查全部；时间用 `--start`/`--end`（ISO-8601），分页用 `--limit`/`--cursor`
- `chat message list-unread-conversations` 获取当前用户未读会话列表，可选 `--count` 指定返回条数
- `chat message search` 按关键词搜索消息内容，`--query` 必填，可选 `--group` 限定搜索某个会话；时间用 `--start`/`--end`（ISO-8601），分页用 `--limit`（默认 100）/`--cursor`
- `chat message read-status` 查询指定消息的已读/未读状态，仅消息发送者可查询自己发出的消息；`--conversation-id`、`--message-id` 必填；目标用户 userId 用 `--user`/`--users`，openDingTalkId 用 `--target-open-dingtalk-ids`，不传则查所有接收者
- `chat search-common` 搜索共同群，`--nicks` 传人员昵称（逗号分隔），`--match-mode` AND/OR 控制匹配逻辑，分页用 `--limit`（默认 20）/`--cursor`
- `chat list-top-conversations` 只拉取置顶会话列表，分页用 `--limit`（默认 1000）/`--cursor`；置顶或取消置顶某条消息使用 `chat message set-top-msg` / `unset-top-msg`，不要混用
- `--user` 和 `--open-dingtalk-id` 本质上都是发起单聊操作，只是用户标识格式不同：userId 为企业内部应用常用标识，openDingTalkId 为三方应用或跨组织场景下的用户标识，服务端对两种 ID 的解析逻辑不同
- `--time` 格式: `yyyy-MM-dd HH:mm:ss`，为拉取消息的起始时间点；`--direction` 控制方向（newer=从给定时间往现在拉，older=从给定时间往以前拉），`--limit` 控制数量
- `chat search` 挂在 `chat` 下（非 `chat group` 下），路径为 `dws chat search`
- `send-by-bot` 群聊传 `--group`，单聊传 `--users` 或 `--open-dingtalk-ids`，与 `--group` 互斥且必选其一；群聊时可选 `--at-user-ids` @指定成员（传 userId 列表）或 `--at-open-dingtalk-ids` @指定成员（传 openDingtalkId 列表），content 中需包含对应 @标识；`--at-all` @所有人；群聊场景如果返回"机器人不存在"错误，需先通过 `chat group members add-bot --id <openConversationId> --robot-code <robot-code>` 将机器人邀请进群后再发送
- `recall-by-bot` 群聊传 `--group` + `--keys`，单聊仅传 `--keys`（不传 `--group` 即为单聊撤回）
- `send-by-webhook` 支持 `--at-all`、`--at-mobiles`、`--at-users` 进行 @ 操作，但需在 `--text` 中包含 `@userId` 或 `@手机号` 才能生效；`--at-all` @所有人时需在 `--text` 中包含 `@10`
- `chat group-role` 系列命令用于管理群的自定义身份标签：`list` 查列表，`add` 创建，`update` 改名，`remove` 删除；`set-user` 覆盖某人全部身份（传空 --role-ids 则清除），`remove-user` 仅移除指定身份，`query-user` 查询某人当前身份；用户用 `--user <userId>`
- 消息**换行符**（`send` / `send-by-bot` / `send-by-webhook` 的 `--text`）有两层要求：(1) 必须是**真实换行符** `U+000A`，不是字面量 `\n`；(2) Markdown 规范下单换行不生效，需用空行 `\n\n`（段落分隔）或行尾两空格 + 换行 / `<br>`（硬换行）
- `chat group transfer-owner` 转让群主，需传 --group（openConversationId）；新群主 userId 用 `--user`，openDingTalkId 用 `--new-owner`
- `chat group invite-url` 获取群邀请链接，需传 --group（openConversationId），可选 --expires-seconds 指定有效期（秒，0=永久）
- `chat group quit` 退出群聊，需传 --group（openConversationId）
- `chat group update-icon` 更新群头像，需传 --group（openConversationId）和由可信上游提供的有效 --icon-media-id（mediaId）；DWS CLI 不能从本地图片生成该 ID
- `chat group update-settings` 更新群设置，需传 --group（openConversationId）、--setting-key（设置项 key）、--status（0=关闭 1=开启）
- `chat +messages-send-card` 的 `--group`、`--receiver`、`--receiver-open-dingtalk-id` 三选一；传 `--content` 时创建并立即更新，不传时返回 `bizId` 供后续 `message update-card` 使用；`--flow-status` 范围 1–5，默认 3
- 原子 `chat message send-card` 创建卡片后仍用 `message update-card --biz-id` 更新内容
- `chat message update-card` 流式更新卡片内容，需传 --biz-id（创建卡片返回的业务 ID）、--content、--flow-status
- `chat message list-by-ids` 根据消息 ID 批量查询，--msg-ids 逗号分隔，最多 50 条
- `chat message add-emoji` / `remove-emoji` 需传 --group（openConversationId）、--msg-id（openMsgId）、--emoji（表情名称）
- `chat message add-text-emotion` / `remove-text-emotion` 需传 --group、--msg-id、--emotion-id、--emotion-name、--text、--background-id，六个参数全部必填
- `chat message create-text-emotion` 创建文字表情模板，返回 emotionId；--background-id 可选，不传由服务端默认分配
- `chat category list` 无需参数；`category list-conversations` 需传 --category-id（通过 category list 获取）
- `chat mute` 默认开启免打扰，传 --off 关闭；--conversation-id / --id / --chat 三个别名均可用于传入会话 ID
- `chat message reply` 引用回复消息（**单聊/群聊均可**），需传 --conversation-id（openConversationId，单聊与群聊使用同一字段）、--ref-msg-id（被引用消息 openMessageId）、--ref-sender（被引用消息发送者 openDingTalkId）、--text（回复内容）；目前回复类型仅支持 text
- `chat message forward` 转发单条消息（**源/目标会话均支持单聊/群聊**，常见组合：群→群、群→单、单→群、单→单），需传 --src-conversation-id（源会话 openConversationId）、--msg-id（源消息 openMessageId）、--dest-conversation-id（目标会话 openConversationId）
- `chat set-top` 设置/取消会话置顶（**单聊/群聊均可**），需传 --conversation-id（openConversationId，单聊与群聊使用同一字段），默认置顶，传 --off 取消
- `chat message reply` 以当前用户身份引用回复，与 `chat message send` 的用户身份发送语义一致
- **如何获取 openConversationId**（如果上层已有则直接使用，不必再查）：
  - 群聊：`dws chat search --query "群名"`
  - 单聊：`dws chat conversation-info --user <userId>` 或 `dws chat conversation-info --open-dingtalk-id <openDingTalkId>`（人员信息可通过 `dws aisearch person --keyword "姓名" --dimension name` 获取）
- `chat group-mute` 全员禁言/取消全员禁言，需传 --group（openConversationId），默认禁言，传 --off 取消
- `chat group-mute-member` 指定群成员禁言，需传 --group、--user/--users（userId，逗号分隔，CLI 自动解析为 openDingTalkId）、--mute-time（毫秒，仅禁言时必填，支持 300000/3600000/86400000/604800000/2592000000），传 --off 解除禁言；禁言群主会被服务端拒绝
- `chat group set-admin` 设置/取消群管理员，需传 --group（openConversationId）、--user/--users（userId，逗号分隔），默认设为管理员，传 --off 取消

## 自动化脚本

| 脚本 | 场景 | 用法 |
|------|------|------|
| [chat_export_messages.py](../scripts/chat_export_messages.py) | 导出群聊消息到 JSON 文件 | `python3 scripts/chat_export_messages.py --query "项目冲刺" --time "2026-03-10 00:00:00"` |
| [chat_history_with_user.py](../scripts/chat_history_with_user.py) | 查询与某人的单聊聊天记录 | `python3 scripts/chat_history_with_user.py --name "张三" --time "2026-03-10 00:00:00"` |
| [bot_broadcast.py](../scripts/bot_broadcast.py) | 使用同一机器人向多个群批量发送消息 | `python3 scripts/bot_broadcast.py --robot-code <code> --chats <id1>,<id2> --title "通知" --text "内容"` |

## 相关产品

- [contact](../../dingtalk-contact/references/contact.md) — 搜索人员，获取 `userId` / `openDingTalkId`。
- [todo](../../dingtalk-todo/references/todo.md) — 消息转待办时使用；从 chat 消息内容提取标题、执行人、截止时间后调用 `dws todo task create`。
- [calendar](../../dingtalk-calendar/references/calendar.md) — 消息转日程时使用；从 chat 消息内容提取标题、开始/结束时间、地点、参会人后调用 `dws calendar event create`。
- [drive](../../dingtalk-drive/references/drive.md) — 云盘/群文件管理；拉群文件先用 `chat conversation-info` 取 `spaceId`，再用 `dws drive list --space-id <spaceId>`；chat 纯文件/音视频发送优先用 `chat +messages-send --as user --msg-type file|audio|video --file`。
- [aisearch](../../dingtalk-aisearch/references/aisearch.md) — 可用于人员、行为、历史信息搜索，但消息发送与会话管理仍走 `chat`。
- [ding](../../dingtalk-misc/references/ding.md) — DING 通知与升级提醒，不等价于群聊消息。

文本翻译（`chat text translate`）详见 [chat-message](./chat/chat-message.md#文本工具)。
