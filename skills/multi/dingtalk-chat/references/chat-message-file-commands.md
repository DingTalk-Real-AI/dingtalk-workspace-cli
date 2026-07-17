# 消息与会话文件命令

### message (会话消息管理)

#### 拉取会话消息内容 — 拉取指定群聊或单聊的会话消息内容

--group 指定群聊，--user 指定单聊用户（通过 userId），--open-dingtalk-id 指定单聊用户（通过 openDingTalkId），三者互斥。推荐使用 --direction 控制时间方向：newer 表示从给定时间往现在拉，older 表示从给定时间往以前拉。--forward 为旧兼容参数：true 等价 newer，false 等价 older。hasMore=true 时用结果中的边界 createTime 作为下次 --time 翻页。引用回复消息会带 `quotedMessage`；被引用的原消息是合并转发或图片时，类型与内容也会保留在引用上下文中。
```
Usage:
  dws chat message list [flags]
Example:
  dws chat message list --group <openconversation_id> --time "2025-03-01 00:00:00"
  dws chat message list --user <userId> --time "2025-03-01 00:00:00" --limit 50
  dws chat message list --open-dingtalk-id <openDingTalkId> --time "2025-03-01 00:00:00" --limit 50
  dws chat message list --group <openconversation_id> --time "2025-03-01 00:00:00" --direction newer
  dws chat message list --group <openconversation_id> --time "2025-03-01 00:00:00" --direction older
Flags:
      --direction string         时间方向: newer=从给定时间往现在拉，older=从给定时间往以前拉（推荐）
      --forward                  旧兼容参数: true 等价 --direction newer，false 等价 --direction older (default true)
      --group string             群聊 openconversation_id（群聊时必填）
      --limit int                返回数量，不传则不限制
      --time string              开始时间，格式: yyyy-MM-dd HH:mm:ss (必填)
      --user string              单聊用户 userId（单聊时与 --open-dingtalk-id 二选一）
      --open-dingtalk-id string  单聊用户 openDingTalkId（单聊时与 --user 二选一，适用于三方应用等无法获取 userId 的场景）

注意:
  - --group、--user、--open-dingtalk-id 三者互斥，只需指定其一：群聊用 --group，单聊用 --user 或 --open-dingtalk-id
  - --user 和 --open-dingtalk-id 都是发起单聊消息拉取，区别在于用不同格式的用户标识：
    - --user 传 userId（企业内部应用常用）
    - --open-dingtalk-id 传 openDingTalkId（三方应用或跨组织场景常用，无法获取 userId 时使用）
  - --group 的别名: --id, --chat, --conversation-id (均可替代 --group)
  - 时间方向：优先使用 --direction newer/older；仅为兼容旧调用保留 --forward。两者同时传入且语义冲突时会报错
  - 翻页：hasMore=true 时，用结果中的边界 createTime 作为下次 --time
  - 处理引用回复时读取 `quotedMessage`，不要只看回复正文；合并转发与图片引用的原消息内容也在该上下文中
  - 话题圈消息拉取流程：如果返回的会话消息中包含 openConvThreadId 字段，说明是话题类消息。要获取完整的话题内容，需要两步操作：(1) 先通过 dws chat message list 拉取话题主消息（即话题帖子本身）；(2) 再调用 dws chat message list-topic-replies --group <openConversationId> --topic-id <openConvThreadId> 分页拉取该话题下的所有回复消息。只有话题主消息 + 回复列表合在一起，才是一条话题的完整内容。
```

#### 以当前用户身份发送消息 — --group 群聊 / --user 或 --open-dingtalk-id 单聊

**重要：该接口会真实发送消息到目标会话，不可用于测试或试探性调用。调用前必须确认消息内容和接收对象无误。**

--group 指定群聊 openConversationId 发群消息；--user 指定用户 userId 发单聊；--open-dingtalk-id 指定用户 openDingTalkId 发单聊。三者只能选其一，不能同时指定。纯文本/Markdown 单聊传 --user 时直接走 userId 发送能力，不需要先手动查询 openDingTalkId。推荐使用 --text flag 传递消息内容（也支持位置参数）。可选 --title 作为消息标题。
若用户只提供了数字群号而非 openConversationId，需先调用 `chat group get-by-group-id` 将群号转为 openConversationId，再传入 --group。
--群聊时可选 --at-all @所有人，或 --at-open-dingtalk-ids 指定成员（仅群聊时生效）。
**发送图片或本地文件 — 一条命令直发**

收到本地路径就直接传给 `--file-path`，CLI 自动完成上传与发送，**无需任何前置工具调用**（不要再走 `dt_media_upload` / `extract_media_id.py` / `drive upload` 任何前置链路）。`.png/.jpg/.pdf/.mp4/.zip` 等任意类型一律：

```bash
# 群聊
dws chat message send --group <openConversationId> --msg-type file --file-path /local/path/to/file.png --format json
# 单聊（推荐 --open-dingtalk-id，userId 也可）
dws chat message send --open-dingtalk-id <openDingTalkId> --msg-type file --file-path /local/path/to/file.png --format json
```

> 单条命令完成上传 + 发送，没有"先 dt_media_upload 再 send"两步流程。带文字说明再额外发一条 `--text "..."` 即可。

```
Usage:
  dws chat message send [flags] [<text>]

Example:
  # 文本/Markdown
  dws chat message send --group <openconversation_id> --text "hello"
  dws chat message send --user <userId> --text "请查收"
  dws chat message send --open-dingtalk-id <openDingTalkId> --text "请查收"
  dws chat message send --group <openconversation_id> --title "周报提醒" --text "请大家本周五前提交周报"
  # 幂等发送（24h 内相同 uuid 不重复投递）
  dws chat message send --group <openconversation_id> --text "hello" --uuid "unique-id-123"
  # 话题回复（拉消息返回的 openConvThreadId 作为 --group 传入）
  dws chat message send --group <openConvThreadId> --text "回复话题内容"
  # @ 群成员
  dws chat message send --group <openconversation_id> --at-all "<@all> 请大家注意"
  dws chat message send --group <openconversation_id> --at-open-dingtalk-ids odt1,odt2 "<@odt1> <@odt2> 请查收"
  # 发送本地图片（直接传路径，CLI 自动上传并发送）
  dws chat message send --group <openconversation_id> --msg-type file --file-path ./screenshot.png
  # 发送本地文件（同一条命令，任意扩展名）
  dws chat message send --group <openconversation_id> --msg-type file --file-path ./report.pdf
  # 旧链路（仅当上游已用 dt_media_upload 拿到 mediaId 时使用）
  dws chat message send --group <openconversation_id> --msg-type image --media-id <mediaId>
Flags:
      --text string              消息内容（推荐使用，也可用位置参数）
      --group string             群聊 openconversation_id（群聊时必填）
      --user string              单聊接收人 userId（单聊时与 --open-dingtalk-id 二选一）
      --open-dingtalk-id string  单聊接收人 openDingTalkId（单聊时与 --user 二选一）
      --title string             消息标题（可选，默认「消息」）
      --at-all                   @所有人（仅群聊时生效，可选，默认 false）
      --at-open-dingtalk-ids string  @指定成员的 openDingTalkId 列表，逗号分隔（仅群聊时生效，可选）
      --media-id string          图片 mediaId（仅旧链路：`--msg-type image` 时使用；新场景请直接用 `--msg-type file --file-path`）
      --msg-type string          富媒体消息类型: image/file/audio/video（推荐统一用 `file --file-path`；image+media-id 仅作旧链路兼容）
      --dentry-id int64          旧链路兼容：文件 dentryId（与 --space-id 成对传入时跳过自动上传）
      --space-id int64           旧链路兼容：空间 ID（与 --dentry-id 成对传入时跳过自动上传）
      --file-name string         旧链路兼容：文件名
      --file-type string         旧链路兼容：文件类型/扩展名
      --file-path string         本地文件路径（msgType=file 时可直接上传发送；旧链路中作为 content.filePath）
      --file-size int64          旧链路兼容：文件大小，单位字节
      --uuid string             幂等 UUID，相同 uuid 在 24h 内不会重复发送（可选）

注意:
  - --text 和位置参数二选一，--text 优先
  - --group、--user、--open-dingtalk-id 三者互斥，只需指定其一：群聊用 --group，单聊用 --user 或 --open-dingtalk-id
  - **话题回复**：话题圈拉消息（`chat message list`）返回的 openConvThreadId 可直接作为 --group 的值传入，即可往该话题内发送回复消息。注意 openConvThreadId 仅从消息列表接口的返回值中获取，禁止自行拼接或猜测
  - 纯文本/Markdown 单聊发送时 `--user` 和 `--open-dingtalk-id` 都可用；传 `--user` 时直接走 userId 发送能力
  - --group 的别名: --id, --chat, --conversation-id (均可替代 --group)
  - --at-all 和 --at-open-dingtalk-ids 仅在 --group 群聊时生效，单聊时无效；当设置--at-all时，消息内容中一定要包含对应的占位符<@all>;当设置--at-open-dingtalk-ids openDingTalkId1,openDingTalkId2时，消息内容中一定要包含对应格式的占位符<@openDingTalkId1> <@openDingTalkId2>
  - **换行符**：消息内容按 Markdown 渲染，换行有两层要求，缺一不可：
    1. 必须使用**真实换行符**（Unicode `U+000A`），而非字面量字符串 `\n`（反斜杠 + 字母 n）。程序或大模型构造参数时，须确保已正确反转义；否则全部内容会渲染在同一行
    2. Markdown 规范下**单个换行不产生换行效果**。需要换行时请使用：段落分隔（连续两个真实换行符 `\n\n`）、行尾两个空格 + 真实换行符（硬换行 `<br>`），或直接写 HTML 的 `<br>` 标签
  - **发图片/文件统一走 `--msg-type file --file-path <本地路径>`**（任意扩展名 png/jpg/pdf/mp4/zip…），CLI 内部一条命令完成上传 + 发送，无需 `dt_media_upload` / `extract_media_id` / `drive upload` 等前置步骤
  - 旧链路兼容：仅当上游已经通过 `dt_media_upload` 拿到 `@lQL...` 形式的 mediaId 时，才使用 `--msg-type image --media-id`；新代码与新指引一律用 file-path 路径
  - 富媒体消息的单聊优先使用 `--open-dingtalk-id`；传 `--user` 时 CLI 会尝试解析成 openDingTalkId 后发送
  - --uuid 用于幂等发送，传入相同 uuid 在 24h 内不会重复投递消息（可选，群聊和单聊均支持）
  - 当前命令不支持位置消息或联系人名片；不得生成 `location` / `profile` 类型或相应的臆造参数
```

#### 查询消息发送状态 — 查询以当前用户身份发送的消息的发送状态

查询以当前用户身份发送的消息的发送状态。需要传入发送消息时返回的 openTaskId。
```
Usage:
  dws chat message query-send-status [flags]
Example:
  dws chat message query-send-status --open-task-id <openTaskId>
  # openTaskId 由 dws chat message send 返回
Flags:
      --open-task-id string   消息发送任务 ID (必填)

注意:
  - openTaskId 由 `dws chat message send` 发送消息成功后返回
  - 用于确认消息是否已成功发送或获取发送失败的原因
```

#### 撤回消息 — 撤回自己或他人的消息

撤回消息。支持撤回自己发送的消息，也支持在群聊中以群主/管理员身份撤回他人的消息。与 `recall-by-bot` 的区别：本命令通过 IM 接口撤回用户消息，`recall-by-bot` 通过机器人接口撤回机器人发出的消息（需要 robot-code + processQueryKey）。
```
Usage:
  dws chat message recall [flags]
Example:
  dws chat message recall --conversation-id <openConversationId> --msg-id <openMessageId>
  # 群主/管理员撤回他人消息：直接传目标消息 ID 即可
  # 查询会话 ID: dws chat search --query "群名"
  # 消息 ID 可通过 dws chat message list 获取
Flags:
      --conversation-id string   会话 openConversationId (必填，支持单聊/群聊，别名: --group / --id / --chat)
      --msg-id string            消息 openMessageId (必填)

注意:
  - --conversation-id 的别名: --group, --id, --chat (均可替代 --conversation-id)
  - 消息 ID 可通过 `dws chat message list` 命令获取
  - 支持撤回自己发出的消息，也支持群主/管理员撤回群内他人的消息
  - 与 `recall-by-bot` 的区别：本命令通过 IM 接口撤回用户消息（需要 openConversationId + openMessageId），`recall-by-bot` 通过机器人接口撤回机器人发出的消息（需要 robot-code + processQueryKey）
```

#### 机器人发送消息（--group 群聊 / --users 单聊）

**重要：该接口会真实发送消息到目标会话，不可用于测试或试探性调用。调用前必须确认消息内容和接收对象无误。**

群聊：传 --group 指定群；单聊：传 --users 指定用户列表，二者只能选其一，不能同时指定。--text 支持 Markdown。群聊时可选 --at-user-ids @指定成员。

如果用户明确要求"用机器人/机器人身份/robot"发送，必须使用本命令，严禁改用 `chat message send` 以当前用户身份发送。

**重要**：机器人发群消息前，必须确认该机器人已在目标群中。若机器人不在群内会报错"机器人不存在"，需先执行 `dws chat group members add-bot --id <openConversationId> --robot-code <robot-code>` 将机器人加入群聊后再发送。
```
Usage:
  dws chat message send-by-bot [flags]
Example:
  dws chat message send-by-bot --robot-code <robot-code> --group <openconversation_id> --title "日报" --text "## 今日完成..."
  dws chat message send-by-bot --robot-code <robot-code> --users userId1,userId2 --title "提醒" --text "请提交周报"
  dws chat message send-by-bot --robot-code <robot-code> --open-dingtalk-ids openDingtalkId1,openDingtalkId2 --title "提醒" --text "请提交周报"
  dws chat message send-by-bot --robot-code <robot-code> --group <openconversation_id> --at-user-ids userId1,userId2 --title "提醒" --text "@userId1 @userId2 请查收本周报告"
  dws chat message send-by-bot --robot-code <robot-code> --group <openconversation_id> --at-open-dingtalk-ids openDingtalkId1,openDingtalkId2 --title "提醒" --text "@openDingtalkId1 @openDingtalkId2 请查收本周报告"
  dws chat message send-by-bot --robot-code <robot-code> --group <openconversation_id> --at-all --title "通知" --text "请所有人注意"
Flags:
      --group string                 群聊 openConversationId（群聊时必填）
      --robot-code string            机器人 Code (必填)
      --text string                  消息内容 Markdown (必填)
      --title string                 消息标题 (必填)
      --users string                 用户 userId 列表，逗号分隔，最多20个（单聊时必填）
      --open-dingtalk-ids string     用户 openDingtalkId 列表，逗号分隔（单聊时可替代 --users，可选）
      --at-user-ids string           @指定成员的 userId 列表，逗号分隔（仅群聊时生效，可选）
      --at-open-dingtalk-ids string  @指定成员的 openDingtalkId 列表，逗号分隔（仅群聊时生效，可选）
      --at-all                        @所有人（可选），服务端接收字符串 true/false

注意:
  - 用户明确要求机器人发送时，必须使用 `chat message send-by-bot`；严禁使用 `chat message send` 以用户身份代发
  - --group 与 --users/--open-dingtalk-ids 互斥，必须且只能指定其一
  - --group 的别名: --id, --chat, --conversation-id (均可替代 --group)
  - --at-user-ids 仅在 --group 群聊时生效，单聊时无效；设置时 --text 中需包含 @userId 对应文本
  - --at-open-dingtalk-ids 仅在 --group 群聊时生效，单聊时无效；设置时 --text 中需包含 @openDingtalkId 对应文本
  - --at-all @所有人，仅群聊时生效；只需带上 --at-all flag 即可，服务端会自动处理
  - userId 获取方式：`dws contact user search --query "姓名"` 搜人获取 userId
  - **换行符**：--text 按 Markdown 渲染，换行规则同 `chat message send`：
    1. 必须使用**真实换行符**（`U+000A`），而非字面量 `\n`，否则全部内容会渲染在同一行
    2. 单个换行不产生换行效果，需用空行（`\n\n`）做段落分隔，或行尾两空格 + 换行/`<br>` 做硬换行
```

#### 机器人撤回消息（--group 群聊 / 不传为单聊）

群聊：传 --group 与 --keys；单聊：仅传 --keys。--keys 为发送时返回的 processQueryKey 列表，逗号分隔。
```
Usage:
  dws chat message recall-by-bot [flags]
Example:
  dws chat message recall-by-bot --robot-code <robot-code> --group <openconversation_id> --keys <process-query-key>
  dws chat message recall-by-bot --robot-code <robot-code> --keys key1,key2
Flags:
      --group string         群聊 openConversationId（群聊撤回时必填）
      --keys string         消息 processQueryKey 列表，逗号分隔 (必填)
      --robot-code string   机器人 Code (必填)
```

#### 自定义机器人 Webhook 发送群消息

@ 人时需在 --text 中包含 @userId 或 @手机号，否则 @ 不生效；@所有人时需在 --text 中包含 @10 并带上 --at-all。
```
Usage:
  dws chat message send-by-webhook [flags]
Example:
  dws chat message send-by-webhook --token <webhook-token> --title "告警" --text "CPU 超 90% @10" --at-all
  dws chat message send-by-webhook --token <webhook-token> --title "test" --text "hi @118785" --at-users 118785
Flags:
      --at-all              @ 所有人（需在 --text 中包含 @10）
      --at-mobiles string   @ 指定手机号，逗号分隔
      --at-users string     @ 指定用户，逗号分隔（需在 text 中包含 @userId）
      --text string         消息内容 (必填)
      --title string        消息标题 (必填)
      --token string        Webhook Token (必填)

注意:
  - **换行符**：--text 按 Markdown 渲染，换行规则同 `chat message send`：
    1. 必须使用**真实换行符**（`U+000A`），而非字面量 `\n`，否则全部内容会渲染在同一行
    2. 单个换行不产生换行效果，需用空行（`\n\n`）做段落分隔，或行尾两空格 + 换行/`<br>` 做硬换行
```

#### 拉取群话题回复消息列表

查询指定群聊中某条话题消息的全部回复。--group 指定群会话 ID，--topic-id 指定话题 ID（由 dws chat message list 返回）。
```
Usage:
  dws chat message list-topic-replies [flags]
Example:
  dws chat message list-topic-replies --group <openconversation_id> --topic-id <topicId>
  dws chat message list-topic-replies --group <openconversation_id> --topic-id <topicId> --time "2025-03-01 00:00:00" --limit 20
  dws chat message list-topic-replies --group <openconversation_id> --topic-id <topicId> --time "2025-03-01 00:00:00" --direction newer
Flags:
      --group string      群会话 openconversationId (必填)
      --topic-id string   话题 ID，由 dws chat message list 返回 (必填)
      --time string       开始时间，格式: yyyy-MM-dd HH:mm:ss（可选）
      --limit int         返回数量（默认 50）
      --direction string  时间方向: newer=从给定时间往现在拉，older=从给定时间往以前拉（推荐，默认 older）
      --forward           旧兼容参数: true 等价 --direction newer，false 等价 --direction older（默认 false）
```

#### 拉取指定时间范围内当前用户的所有会话消息 — 分页拉取当前登录用户在指定时间范围内的所有会话消息

--start 和 --end 限定时间范围，--limit 指定每页数量，--cursor 传分页游标（首页传 "0"，后续从响应中的 nextCursor 获取）。服务端按 cursor 分页返回，hasMore=true 时用返回的 nextCursor 值作为下次 --cursor 继续翻页。
```
Usage:
  dws chat message list-all [flags]
Example:
  dws chat message list-all --start "2025-03-01 00:00:00" --end "2025-03-31 23:59:59" --limit 50
  dws chat message list-all --start "2025-03-01 00:00:00" --end "2025-03-31 23:59:59" --limit 50 --cursor "abc123token"
Flags:
      --start string         起始时间，格式: yyyy-MM-dd HH:mm:ss (必填)
      --end string           结束时间，格式: yyyy-MM-dd HH:mm:ss (必填)
      --limit int            每页返回数量（默认 50）
      --cursor string       分页游标（首页传 "0"，后续从响应中的 nextCursor 获取）

注意:
  - 四个参数每次请求都会传递给服务端，cursor 首页传 "0"
  - 与 chat message list 的区别：list 拉取指定单个会话（群聊或单聊）的消息，list-all 拉取当前用户所有会话的消息
  - 翻页：hasMore=true 时，用响应中的 nextCursor 值作为下次 --cursor 参数继续翻页
  - 时间格式统一为 yyyy-MM-dd HH:mm:ss
  - 权限/权益错误不是空结果；应把返回的 `friendly_hint` 与 `action_url` 展示给用户，不要继续盲目翻页
```

#### 收藏消息 — 为当前用户收藏指定会话中的一条消息
```
Usage:
  dws chat message add-favorite [flags]
Example:
  dws chat message add-favorite --open-message-id <openMessageId> --open-conversation-id <openConversationId>
Flags:
      --open-message-id string        消息 openMessageId (必填)
      --open-conversation-id string   消息所在会话 openConversationId (必填)
```

#### 取消收藏消息 — 移除当前用户的收藏标记，不删除原消息
```
Usage:
  dws chat message remove-favorite [flags]
Example:
  dws chat message remove-favorite --open-message-id <openMessageId> --open-conversation-id <openConversationId>
Flags:
      --open-message-id string        消息 openMessageId (必填)
      --open-conversation-id string   消息所在会话 openConversationId (必填)
```

#### 查询收藏消息 — 分页查询当前用户收藏的消息
```
Usage:
  dws chat message list-favorites [flags]
Example:
  dws chat message list-favorites
  dws chat message list-favorites --cursor 20 --size 20
Flags:
      --cursor int   数字分页游标，默认 0；翻页时传上次返回的 nextCursor
      --size int     一次拉取的收藏数量，默认 20，范围 1-100

注意:
  - 首次请求可省略分页参数，CLI 会自动使用 cursor=0、size=20
  - hasMore=true 时，将 nextCursor 作为下一次的 --cursor
```

#### 拉取指定发送者的消息 — 搜索特定人发送给我的消息（包含单聊和群聊）

> 推荐优先使用 `chat message search-advanced --user/--users`（userId）或 `--sender-ids`（openDingTalkId），它还能叠加关键词/群/at 等过滤条件。本命令保留给需要旧 list-by-sender 返回结构的场景。

搜索特定人发送给我的消息，返回结果包含单聊和群聊标识。--sender-user-id 指定发送者 userId，--sender-open-dingtalk-id 指定发送者 openDingTalkId，二者互斥。分页参数 --limit（默认 50）和 --cursor（默认 "0"）始终传递；hasMore=true 时用返回的 nextCursor 作为下次 --cursor 继续翻页。
```
Usage:
  dws chat message list-by-sender [flags]
Example:
  dws chat message list-by-sender --sender-user-id <userId> --start "2026-03-10T00:00:00+08:00" --end "2026-03-11T00:00:00+08:00" --limit 50 --cursor 0
  dws chat message list-by-sender --sender-open-dingtalk-id <openDingTalkId> --start "2026-03-10T00:00:00+08:00" --end "2026-03-11T00:00:00+08:00" --limit 50 --cursor 0
  dws chat message list-by-sender --sender-user-id <userId> --start "2026-03-10T00:00:00+08:00" --end "2026-03-10T23:59:59+08:00" --limit 20 --cursor 0
  dws chat message list-by-sender --sender-open-dingtalk-id <openDingTalkId> --start "2026-03-10T00:00:00+08:00" --end "2026-03-11T00:00:00+08:00" --limit 50 --cursor <nextCursor>
Flags:
      --sender-user-id string                发送者 userId（与 --sender-open-dingtalk-id 二选一）
      --sender-open-dingtalk-id string        发送者 openDingTalkId（与 --sender-user-id 二选一，适用于无法获取 userId 的场景）
      --start string                          开始时间，ISO-8601 格式 (必填)
      --end string                            结束时间，ISO-8601 格式 (必填)
      --limit int                             每页返回数量（默认 50）
      --cursor string                         分页游标（默认 "0"，翻页传 nextCursor）

注意:
  - --sender-user-id 和 --sender-open-dingtalk-id 二者互斥，必须且只能指定其一：
    - --sender-user-id 传 userId（企业内部应用常用）
    - --sender-open-dingtalk-id 传 openDingTalkId（三方应用或跨组织场景常用，无法获取 userId 时使用）
  - openDingTalkId 获取方式见下方「openDingTalkId 获取方式」小节
  - 不需要指定单聊/群聊，返回结果自带会话类型标识
  - 时间支持多种 ISO-8601 格式，如 "2026-03-10T00:00:00+08:00"、"2026-03-10 14:00:00"、"2026-03-10" 等
  - 翻页：hasMore=true 时，用返回的 nextCursor 作为下次 --cursor
```

#### 拉取 @我 的消息 — 搜索时间范围内 @我 的消息

> 推荐使用 `chat message search-advanced --at-me`，它还能叠加关键词/群/发送者等过滤条件。本命令适用于仅需拉取 @我 消息的简单场景。

搜索时间范围内 @我 的消息，可选指定群聊。返回结果包含单聊和群聊标识。分页参数 --limit（默认 50）和 --cursor（默认 "0"）始终传递；hasMore=true 时用返回的 nextCursor 作为下次 --cursor 继续翻页。
```
Usage:
  dws chat message list-mentions [flags]
Example:
  dws chat message list-mentions --start "2026-03-10T00:00:00+08:00" --end "2026-03-11T00:00:00+08:00" --limit 50 --cursor 0
  dws chat message list-mentions --start "2026-04-01T00:00:00+08:00" --end "2026-04-14T00:00:00+08:00" --limit 20 --cursor 0
  dws chat message list-mentions --group <openconversation_id> --start "2026-03-10T00:00:00+08:00" --end "2026-03-11T00:00:00+08:00" --limit 50 --cursor 0
  dws chat message list-mentions --start "2026-03-10T00:00:00+08:00" --end "2026-03-11T00:00:00+08:00" --limit 50 --cursor <nextCursor>
Flags:
      --group string    群聊 openconversation_id（可选，不传则查全部）
      --start string    开始时间，ISO-8601 格式 (必填)
      --end string      结束时间，ISO-8601 格式 (必填)
      --limit int       每页返回数量（默认 50）
      --cursor string   分页游标（默认 "0"，翻页传 nextCursor）

注意:
  - --group 可选，不传则查询所有会话中 @我 的消息；传入则只查指定群聊
  - --group 的别名: --id, --chat, --conversation-id (均可替代 --group)
  - 时间支持多种 ISO-8601 格式，如 "2026-03-10T00:00:00+08:00"、"2026-03-10 14:00:00"、"2026-03-10" 等
  - 翻页：hasMore=true 时，用返回的 nextCursor 作为下次 --cursor
```

#### 拉取特别关注人的消息

拉取当前用户特别关注人的消息。分页参数 --limit 指定每页数量，--cursor 传分页游标（首次不传或传 0）。返回结果中 hasMore=true 时用 nextCursor 作为下次 --cursor 继续翻页。
```
Usage:
  dws chat message list-focused [flags]
Example:
  dws chat message list-focused --limit 50
  dws chat message list-focused --limit 20 --cursor <nextCursor>
Flags:
      --limit int       每页返回数量（默认 50）
      --cursor int64    分页游标（首次不传或传 0，翻页传 nextCursor）

注意:
  - 首次调用不传 --cursor 或传 0，后续翻页传 nextCursor
```

#### 获取未读会话列表

获取当前用户有未读消息的会话信息。可选通过 `--count` 限制返回条数，不传则使用服务端默认值。
```
Usage:
  dws chat message list-unread-conversations [flags]
Example:
  dws chat message list-unread-conversations
  dws chat message list-unread-conversations --count 20
Flags:
      --count int        返回未读会话条数（可选）
      --exclude-muted    是否排除已设置免打扰的会话（默认 false）
```

#### 查询消息的已读/未读状态

查询指定会话中消息的已读/未读状态（仅消息发送者可查询自己发出的消息）。--conversation-id 指定会话 openConversationId（群聊或单聊均可），--message-id 指定消息 ID（由 dws chat message list 返回的 openMessageId，必须是当前用户发送的消息）。目标用户 userId 使用 --user/--users；目标用户 openDingTalkId 使用 --target-open-dingtalk-ids；不传目标用户则返回所有接收者的状态。
```
Usage:
  dws chat message read-status [flags]
Example:
  dws chat message read-status --conversation-id <openConversationId> --message-id <openMessageId>
  dws chat message read-status --conversation-id <openConversationId> --message-id <openMessageId> --user userId1,userId2
  dws chat message read-status --conversation-id <openConversationId> --message-id <openMessageId> --users userId1,userId2
  dws chat message read-status --conversation-id <openConversationId> --message-id <openMessageId> --target-open-dingtalk-ids openDingTalkId1,openDingTalkId2
Flags:
      --conversation-id string              会话 openConversationId (必填，群聊或单聊均可)
      --message-id string                   消息 openMessageId，由 chat message list 返回 (必填，必须是当前用户发送的消息)
      --user string                         目标用户 userId，支持逗号分隔（可选，不传则查所有接收者）
      --users string                        目标用户 userId 列表，逗号分隔（可选，不传则查所有接收者）
      --target-open-dingtalk-ids string     目标用户 openDingTalkId 列表，逗号分隔（可选，不传则查所有接收者）

注意:
  - 仅消息发送者可查询自己发出的消息的已读/未读状态，查询他人发的消息会报错
  - --conversation-id 的别名: --group, --id, --chat (均可替代 --conversation-id)
  - --message-id 从 dws chat message list 返回的消息列表中获取（字段名 openMessageId）
  - --user / --users 传目标用户 userId
  - --target-open-dingtalk-ids 不传时返回该消息所有接收者的已读状态；传入则只返回指定 openDingTalkId 用户的状态
```

#### 按关键词搜索消息 — 在当前用户的会话中按关键词搜索消息

> 推荐优先使用 `chat message search-advanced`，它是本命令的严格超集：query 可选（非必填）、支持多个会话（非单个）、还能叠加发送者/at 等维度过滤。

按关键词搜索消息内容。--query 指定搜索关键词（必填）。可选 --group 限定搜索某个会话，不传则搜索所有会话。时间参数 --start/--end（ISO-8601）限定搜索时间范围。分页参数 --limit（默认 100）和 --cursor（默认 "0"）始终传递；hasMore=true 时用返回的 nextCursor 作为下次 --cursor 继续翻页。
```
Usage:
  dws chat message search [flags]
Example:
  dws chat message search --query "changefree" --start "2026-04-01T00:00:00+08:00" --end "2026-04-15T00:00:00+08:00" --limit 50 --cursor 0
  dws chat message search --query "codereview" --group <openconversation_id> --start "2026-04-01T00:00:00+08:00" --end "2026-04-15T00:00:00+08:00" --limit 100 --cursor 0
  dws chat message search --query "链接" --start "2026-04-15T00:00:00+08:00" --end "2026-04-16T00:00:00+08:00" --limit 100 --cursor <nextCursor>
Flags:
      --query string   搜索关键词 (必填)
      --group string     群聊 openconversation_id（可选，不传则搜索所有会话）
      --start string     开始时间，ISO-8601 格式 (必填)
      --end string       结束时间，ISO-8601 格式 (必填)
      --limit int        每页返回数量（默认 100）
      --cursor string    分页游标（默认 "0"，翻页传 nextCursor）

注意:
  - --group 可选，不传则搜索所有会话中的消息；传入则只搜索指定会话
  - --group 的别名: --id, --chat, --conversation-id (均可替代 --group)
  - 时间支持多种 ISO-8601 格式，如 "2026-03-10T00:00:00+08:00"、"2026-03-10 14:00:00"、"2026-03-10" 等
  - 翻页：hasMore=true 时，用返回的 nextCursor 作为下次 --cursor
```

#### 多维度搜索消息（推荐首选） — 支持按关键词、发送者、@我、@指定人、指定会话和时间范围搜索

> 推荐：这是消息搜索的首选接口。它可以完全替代 `chat message search`（query 可选 vs 必填，支持多个会话 vs 单个），大部分替代 `chat message list-by-sender`（通过 --user/--users 按 userId 搜索发送者，或通过 --sender-ids 按 openDingTalkId 搜索）和 `chat message list-mentions`（通过 --at-me 搜索@我的消息）。仅在拉取「特别关注人」消息时需要退回 `list-focused`。

支持按关键词、发送者、@我、@指定人、指定会话和时间范围等维度搜索消息。发送者 userId 使用 --user/--users；发送者或 @ 人的 openDingTalkId 使用 --sender-ids/--at-ids。所有参数均为可选，至少指定一个搜索条件。
```
Usage:
  dws chat message search-advanced [flags]
Example:
  dws chat message search-advanced --query "周报" --start "2026-04-01T00:00:00+08:00" --end "2026-04-15T00:00:00+08:00"
  dws chat message search-advanced --user <userId> --start "2026-04-01T00:00:00+08:00" --end "2026-04-15T00:00:00+08:00"
  dws chat message search-advanced --users <userId1>,<userId2> --start "2026-04-01T00:00:00+08:00" --end "2026-04-15T00:00:00+08:00"
  dws chat message search-advanced --sender-ids <openDingTalkId1>,<openDingTalkId2> --start "2026-04-01T00:00:00+08:00" --end "2026-04-15T00:00:00+08:00"
  dws chat message search-advanced --at-me --start "2026-04-01T00:00:00+08:00" --end "2026-04-15T00:00:00+08:00"
  dws chat message search-advanced --at-ids <openDingTalkId1>,<openDingTalkId2> --conversation-ids <openConversationId1>,<openConversationId2> --limit 50 --cursor 0
  dws chat message search-advanced --conversation-ids <单聊openConversationId> --query "合同" --start "2026-04-01T00:00:00+08:00" --end "2026-04-15T00:00:00+08:00"
  # 查询群 ID: dws chat search --query "群名"
  # 查询单聊会话 ID: dws chat conversation-info --user <userId>
  # 查询人员: dws contact user search --query "姓名" --format json
Flags:
      --query string              搜索关键词（可选）
      --user string                 发送者 userId，支持逗号分隔（可选）
      --users string                发送者 userId 列表，逗号分隔（可选）
      --sender-ids string           发送者 openDingTalkId 列表，逗号分隔（可选）
      --at-me                       只搜索 @我 的消息（可选，默认 false）
      --at-ids string               @指定人的 openDingTalkId 列表，逗号分隔（可选）
      --conversation-ids string     会话 openConversationId 列表，逗号分隔（可选，群聊或单聊均可，不传则搜索所有会话）
      --start string                开始时间，ISO-8601 格式（可选）
      --end string                  结束时间，ISO-8601 格式（可选）
      --cursor string               分页游标（默认 "0"）
      --limit int                   每页返回数量（默认 100）
      --conversation-ids 的别名: --groups

注意:
  - 所有参数均为可选，但至少需要指定一个搜索条件
  - --user / --users 传发送者 userId
  - --sender-ids 和 --at-ids 传 openDingTalkId
  - --conversation-ids 可指定多个会话 ID（群聊或单聊均可），逗号分隔，不传则搜索所有会话
  - 群聊 openConversationId 通过 `dws chat search --query "群名"` 获取
  - 单聊 openConversationId 通过 `dws chat conversation-info --user <userId>` 或 `--open-dingtalk-id <openDingTalkId>` 获取
  - 时间支持多种 ISO-8601 格式，如 "2026-03-10T00:00:00+08:00"、"2026-03-10 14:00:00"、"2026-03-10" 等
  - 翻页：hasMore=true 时，用返回的 nextCursor 作为下次 --cursor
  - 替代关系：完全替代 search（严格超集）；大部分替代 list-by-sender（--user 覆盖按 userId 搜索发送者，--sender-ids 覆盖按 openDingTalkId 搜索）和 list-mentions（--at-me 覆盖核心功能）；不能替代 list-focused（「特别关注」是独立维度）
```

#### 根据消息 ID 批量查询消息
```
Usage:
  dws chat message list-by-ids [flags]
Example:
  dws chat message list-by-ids --msg-ids msgId1,msgId2,msgId3
  # 最多传 50 条消息 ID
Flags:
      --msg-ids string   消息 ID 列表，逗号分隔，最多 50 条 (必填)
```

#### 表情回应选择策略

> 贴表情时，优先查 [chat-emoji-list.md](chat-emoji-list.md) 中的默认表情名称（共 199 个，如「赞」「鼓掌」「感谢」等）：
> - 命中 → 使用 `add-emoji --emoji <name>`（直接贴 emoji）
> - 未命中 → 先 `create-text-emotion` 创建文字表情获取 emotionId，再 `add-text-emotion` 贴文字表情

#### 对消息添加 emoji 表情回应
```
Usage:
  dws chat message add-emoji [flags]
Example:
  dws chat message add-emoji --conversation-id <openConversationId> --msg-id <openMsgId> --emoji "赞"
  dws chat message add-emoji --conversation-id <openConversationId> --msg-id <openMsgId> --emoji "鼓掌"
  # --emoji 的值必须是 chat-emoji-list.md 中的 name（中文名），如：赞、鼓掌、感谢、微笑 等
  # 查询会话 ID: dws chat search --query "群名"
Flags:
      --conversation-id string   会话 openConversationId (必填，支持单聊/群聊，别名: --group / --id / --chat)
      --msg-id string   消息 openMsgId (必填)
      --emoji string    emoji 表情名称，必须是默认表情列表中的 name 值 (必填，参见 chat-emoji-list.md)
```

#### 移除消息的 emoji 表情回应
```
Usage:
  dws chat message remove-emoji [flags]
Example:
  dws chat message remove-emoji --conversation-id <openConversationId> --msg-id <openMsgId> --emoji "赞"
  # 查询会话 ID: dws chat search --query "群名"
Flags:
      --conversation-id string   会话 openConversationId (必填，支持单聊/群聊，别名: --group / --id / --chat)
      --msg-id string   消息 openMsgId (必填)
      --emoji string    emoji 表情名称，必须是默认表情列表中的 name 值 (必填，参见 chat-emoji-list.md)
```

#### 对消息添加文字表情回应（当默认表情列表中没有所需表情时使用）
```
Usage:
  dws chat message add-text-emotion [flags]
Example:
  dws chat message add-text-emotion --conversation-id <openConversationId> --msg-id <openMsgId> --emotion-id <emotionId> --emotion-name "赞" --text "nice" --background-id im_bg_5
Flags:
      --conversation-id string   会话 openConversationId (必填，支持单聊/群聊，别名: --group / --id / --chat)
      --msg-id string          消息 openMsgId (必填)
      --emotion-id string      表情 ID (必填，通过 create-text-emotion 或已知表情获取)
      --emotion-name string    表情名称 (必填)
      --text string            文字内容 (必填)
      --background-id string   背景 ID (必填)
```

#### 移除消息的文字表情回应
```
Usage:
  dws chat message remove-text-emotion [flags]
Example:
  dws chat message remove-text-emotion --conversation-id <openConversationId> --msg-id <openMsgId> --emotion-id <emotionId> --emotion-name "赞" --text "nice" --background-id <backgroundId>
Flags:
      --conversation-id string   会话 openConversationId (必填，支持单聊/群聊，别名: --group / --id / --chat)
      --msg-id string          消息 openMsgId (必填)
      --emotion-id string      表情 ID (必填)
      --emotion-name string    表情名称 (必填)
      --text string            文字内容 (必填)
      --background-id string   背景 ID (必填)
```

#### 创建文字表情（获取 emotionId）— 当 chat-emoji-list.md 中没有所需表情时，先创建再贴
```
Usage:
  dws chat message create-text-emotion [flags]
Example:
  dws chat message create-text-emotion --emotion-name "赞" --text "nice"
  dws chat message create-text-emotion --emotion-name "感谢" --text "感谢" --background-id im_bg_5
Flags:
      --emotion-name string    表情名称 (必填)
      --text string            文字内容 (必填)
      --background-id string   背景 ID（可选，不传则由服务端默认分配）

注意:
  - 创建后返回 emotionId，可用于 add-text-emotion 命令
  - 如果已有合适的表情，无需创建新的
```

#### 批量拉取消息的表情回复和文字回复

根据消息 ID 列表批量查询消息的表情回复和文字回复信息。
```
Usage:
  dws chat message list-emotion-replies [flags]
Example:
  dws chat message list-emotion-replies --msg-ids msgId1,msgId2,msgId3
  # 消息 ID 可通过 dws chat message list 获取
Flags:
      --msg-ids string   消息 ID 列表，逗号分隔 (必填)

注意:
  - 支持批量查询多条消息的表情和文字回复
  - 消息 ID 可通过 `dws chat message list` 命令获取
```
