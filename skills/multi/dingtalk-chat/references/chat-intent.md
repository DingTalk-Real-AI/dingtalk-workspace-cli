# 意图判断

## 意图判断

用户说"我特别关注的人最近发了什么消息/关注的人最近聊了啥/星标联系人最近的动态" → `chat message list-focused`（零参数一行命令）
用户说"某人发给我的消息/指定发送者的消息/某人最近的消息" → `chat message list-by-sender --sender-user-id <userId>` 或 `--sender-open-dingtalk-id <openDingTalkId>`（跨单聊+群聊）
用户说"和某人的单聊聊天记录/拉某人单聊历史" → `chat message list --user <userId>` 或 `--open-dingtalk-id <openDingTalkId>`
用户说"某个群的聊天记录" → `chat message list --group <openConversationId>`
用户说"我最近所有消息/我今天的消息" → `chat message list-all --start <ISO> --end <ISO>`
用户说"@我的消息/提及我的" → `chat message list-mentions --start <ISO> --end <ISO>`
用户说"搜索消息里的关键词/包含XX的消息" → `chat message search-advanced --query "<关键词>"`（首选，严格超集）
用户说"我和某人的共同群" → `chat search-common --nicks "<昵称1>,<昵称2>"`
用户说"未读会话列表" → `chat message list-unread-conversations`
用户说"群里某条话题的回复" → `chat message list-topic-replies --group <id> --topic-id <id>`
用户说"回复话题/往话题里发消息" → 先用 `chat message list` 拉取获得 openConvThreadId，再 `chat message send --group <openConvThreadId> --text "..."`
用户说"置顶会话/查看置顶" → `chat list-top-conversations` 列会话 → 再 `chat message list --group <id>` 拉消息（两步）
用户说"置顶某条消息/把这条消息置顶/消息置顶" → `chat message set-top-msg`
用户说"取消置顶消息/取消消息置顶" → `chat message unset-top-msg`

用户说"建群/创建群聊" → `chat group create`
用户说"建外部群/创建外部群" → `chat group create --type EXTERNAL`
用户说"建普通群" → `chat group create --type NORMAL`
用户说"创建话题圈/建话题群" → `chat group create --thread`
用户说"建群并指定群主/让某某当群主" → 先 `chat group create`，从返回取 `openConversationId`，再 `chat group transfer-owner --group <openConversationId> --new-owner <openDingTalkId>`
用户说"搜索群/找群" → `chat search`
用户说"我创建的群/我管理的群/我是群主的群/我当管理员的群" → `chat group list-my-groups`
用户说"群成员/看群里有谁" → `chat group members`
用户说"拉人进群/加群成员" → `chat group members add`
用户说"踢人/移除群成员" → `chat group members remove`
用户说"加机器人到群" → `chat group members add-bot`
用户说"批量查群成员详情/按ID查群成员/查看指定成员信息" → `chat group members list-by-ids`
用户说"改群名" → `chat group rename`
用户说"聊天记录/会话消息/拉取会话" → `chat message list`
用户说"某人发给我的消息/指定发送者/某人的消息" → `chat message list-by-sender`（用户未明确说"单聊"时优先使用，跨单聊/群聊）
用户说"拉取和某人的单聊记录/单聊消息" → `chat message list --user`（用户明确说"单聊"时使用）
用户说"@我的消息/at我的/提及我的" → `chat message list-mentions`
用户说"未读消息会话/未读会话列表/我的未读会话" → `chat message list-unread-conversations`
用户说"发群消息(以个人身份)" → `chat message send --group`
用户说"发单聊消息(以个人身份)" → `chat message send --user`（有 userId 时）或 `chat message send --open-dingtalk-id`（有 openDingTalkId 时）
用户说"机器人发消息/机器人群发" → `chat message send-by-bot`
用户说"撤回我发的消息/撤回消息" → `chat message recall`（通过 IM 接口撤回当前用户自己发出的消息，需要 openConversationId + openMessageId）
用户说"撤回机器人发的消息/机器人撤回消息" → `chat message recall-by-bot`（通过机器人接口撤回机器人发出的消息，需要 robot-code + processQueryKey）
用户说"Webhook 发消息/告警消息" → `chat message send-by-webhook`
用户说"话题回复/群话题消息回复/拉取话题回复" → `chat message list-topic-replies`
用户说"所有消息/全部会话消息/拉取全部消息/时间范围内消息/我的消息/我今天的消息/查我的钉钉消息/最近的消息" → `chat message list-all`
用户说"特别关注人的消息/关注的人的消息/星标联系人的消息" → `chat message list-focused`
用户说"消息已读未读/谁看了消息/查读状态/消息读取状态" → `chat message read-status`
用户说"查看我的机器人" → `chat bot search`
用户说"搜索消息/查找关键词/搜一下消息里的XX" → 优先使用 `chat message search-advanced`（推荐首选，严格超集）；仅在简单关键词搜索且无其他维度需求时可用 `chat message search`
用户说"多维度搜索/按发送者搜索/按人搜消息/指定多个群搜索/@我的消息搜索" → `chat message search-advanced`（推荐首选，支持多维度组合搜索）
用户说"查询消息发送状态/消息发没发成功/消息状态" → `chat message query-send-status`
用户说"我和XX的共同群/我们都在哪些群/查共同群" → `chat search-common`
用户说"置顶会话/我的置顶/查看置顶会话" → `chat list-top-conversations`
用户说"置顶某条消息/把这条消息置顶" → `chat message set-top-msg`
用户说"取消置顶消息/取消消息置顶" → `chat message unset-top-msg`
用户说"查看会话分组/自定义分组" → `chat category list`
用户说"某个分组下的会话/分组会话列表" → `chat category list-conversations`
用户说"创建会话分组/新建分组/添加分组" → `chat category create`
用户说"删除会话分组/移除分组" → `chat category delete`
用户说"重命名分组/修改分组名称/更新分组名" → `chat category rename`
用户说"把会话加到分组/将群移入分组/添加会话到分组" → `chat category add-conv`
用户说"把会话从分组移出/将群从分组移除/从分组中删除会话" → `chat category remove-conv`
用户说"根据群号查群信息/群号查群/群号转openConversationId" → `chat group get-by-group-id`（当用户发消息时只提供了群号，用此工具将群号转为 openConversationId，再调用发消息接口）
用户说"查看群身份/群的自定义身份列表" → `chat group-role list`
用户说"创建/添加群身份" → `chat group-role add`
用户说"修改/更新群身份名称" → `chat group-role update`
用户说"删除群身份" → `chat group-role remove`
用户说"给某人设置群身份/设定用户的群身份" → `chat group-role set-user`
用户说"移除某人的群身份/撤销群身份" → `chat group-role remove-user`
用户说"查询某人的群身份/某人在群里有什么身份" → `chat group-role query-user`
用户说"转让群主/换群主/群主转让" → `chat group transfer-owner`
用户说"群邀请链接/入群链接/加群链接" → `chat group invite-url`
用户说"分享群链接/把群分享给某人/群链接发到某群" → `chat group share-invite`
用户说"创建智能分组/新建智能会话分组" → `chat category create-smart`
用户说"批量查消息/按ID查消息/根据消息ID查" → `chat message list-by-ids`
用户说"emoji回应/表情回应/给消息加表情" → `chat message add-emoji`
用户说"取消emoji回应/移除表情回应" → `chat message remove-emoji`
用户说"文字表情回应/添加文字表情" → `chat message add-text-emotion`
用户说"取消文字表情回应/移除文字表情" → `chat message remove-text-emotion`
用户说"创建文字表情/新建文字表情" → `chat message create-text-emotion`
用户说"免打扰/消息免打扰/静音/开启免打扰/关闭免打扰" → `chat mute`
用户说"引用回复/回复消息/引用消息回复" → `chat message reply`
用户说"转发消息/转发一条消息/把消息转发到另一个群" → `chat message forward`
用户说"合并转发/批量转发/合并转发多条消息" → `chat message combine-forward`
用户说"转发话题/转发话题消息/话题转发到另一个群" → `chat message forward-topic`
用户说"群机器人列表/群里有哪些机器人/查看群机器人" → `chat group bots`
用户说"从群里移除机器人/踢出机器人" → `chat group members remove-bot`
用户说"搜索机器人/找机器人/查机器人/帮我找XXX机器人" → `chat bot find`（全部可用机器人，额外返回 openDingTalkId 可发单聊）
用户说"给机器人发单聊/给机器人发消息/跟机器人聊天" → 必须先 `chat bot find`（拿 openDingTalkId）→ 再 `chat message send --open-dingtalk-id`（search 没有 openDingTalkId，无法发单聊）
用户说"我创建的机器人/我的机器人/我自己的机器人/查看我的机器人" → `chat bot search`（仅我创建的机器人，无 openDingTalkId）
用户说"解散群/解散群聊" → `chat group dismiss`
用户说"设置历史消息/新成员看历史/新成员可见消息" → `chat group set-history`
用户说"置顶会话/取消置顶/会话置顶" → `chat set-top`（设置/取消置顶），`chat list-top-conversations`（查看置顶列表）
用户说"全员禁言/群禁言/解除禁言" → `chat group-mute`
用户说"禁言某人/指定成员禁言/解除某人禁言" → `chat group-mute-member`
用户说"设管理员/取消管理员/设置群管理员" → `chat group set-admin`
用户说"改群昵称/设置群昵称/我在群里的名字" → `chat group update-nick`
用户说"群备注/给群加备注/修改群备注" → `chat group update-alias`
用户说"隐藏会话/隐藏群聊/隐藏对话" → `chat hide`
用户说"关闭@所有人通知/屏蔽@所有人/不接收@all" → `chat mute-at-all`
用户说"开启@所有人通知/恢复@所有人提醒" → `chat mute-at-all --off`
用户说"关闭红包通知/屏蔽红包/不接收红包提醒" → `chat mute-red-envelope`
用户说"开启红包通知/恢复红包提醒" → `chat mute-red-envelope --off`

关键区分:
- `chat search` — 搜**群/会话名**返回 `openConversationId`，**不**搜消息内容；要搜消息内容请用 `chat message search-advanced`（首选）/ `chat message search` / `list-by-sender` / `list-all`，**勿混淆**
- `chat message list` — 拉取指定会话的消息（需指定 --group 或 --user），按时间点 + 方向翻页
- `chat message list --user` — list 的单聊模式，拉取与指定用户的单聊记录（用户明确说"单聊""私聊"时使用）
- `chat message list-by-sender` — 搜索指定发送者发给我的消息，跨所有会话（单聊+群聊均包含，用户只说"某人发的消息"时优先使用）
- `chat message list-mentions` — 拉取 @我 的消息（跨单聊/群聊，可选指定群）
- `chat message list-unread-conversations` — 拉取当前用户存在未读消息的会话列表（可选 `--count`）
- `chat message read-status` — 查询指定消息的已读/未读状态（仅消息发送者可查询自己发的消息，需指定 --group 和 --message-id，可选 --target-open-dingtalk-ids 查特定人）
- `chat message list-all` — 拉取当前用户所有会话的消息，按时间范围 + cursor 分页。只要用户没有指定某个具体的会话（如某个群名、某个人名），即使提到"单聊消息""群聊消息"等笼统范围，也应路由到此命令
- `chat message list-topic-replies` — 拉取群话题的回复消息列表
- `chat message list-focused` — 拉取特别关注人的消息，cursor 分页
- `chat list-top-conversations` — 拉取置顶会话列表（用户询问"置顶会话"时路由到此），cursor 分页
- `chat message send` — 以当前用户身份发消息（群聊或单聊），text 为位置参数；发图片/本地文件统一用 `--msg-type file --file-path <本地路径>`（任意扩展名一条命令直发）；`--msg-type image --media-id` 仅作旧链路兼容（上游已持有 mediaId 时使用）
- `chat message search` — 按关键词搜索消息内容（跨所有会话，可选指定群）
- `chat search-common` — 搜索共同群，查询指定人共同所在的群聊（AND=所有人都在，OR=任一人在）
- `chat message send-by-bot` — 以**机器人**身份发消息（群聊或单聊），text 为 --text flag
- `chat message send-by-webhook` — 通过**自定义机器人 Webhook** 发群消息
- `chat message recall-by-bot` — 通过**机器人接口**撤回机器人发出的消息，需要 `--robot-code` + `--keys`（发送时返回的 processQueryKey）；传 `--group` 为群聊撤回，不传为单聊撤回
- `chat message recall` — 通过 **IM 接口**撤回当前用户自己发出的消息，需要 `--conversation-id`（openConversationId）+ `--msg-id`（openMessageId，可通过 `chat message list` 获取）；群聊单聊均通过 `--conversation-id` 区分
- `chat message query-send-status` — 查询个人发送的消息的发送状态（需 send 返回的 openTaskId）
- `chat message search-advanced` — 多维度搜索消息（支持关键词、发送者、@我、@指定人、多个会话等维度组合，与 `search` 的区别：`search` 仅支持关键词且必填，`search-advanced` 所有参数均可选）
- `chat message list-by-ids` — 根据消息 ID 批量查询消息（最多 50 条）
- `chat message add-emoji` / `remove-emoji` — 对消息添加/移除 emoji 表情回应
- `chat message add-text-emotion` / `remove-text-emotion` — 对消息添加/移除文字表情回应
- `chat message create-text-emotion` — 创建文字表情模板，返回 emotionId 供 add-text-emotion 使用
- `chat category list` — 获取用户自定义会话分组列表
- `chat category list-conversations` — 拉取指定分组下的会话列表
- `chat category create` — 创建用户自定义会话分组
- `chat category delete` — 删除用户自定义会话分组
- `chat category rename` — 更新用户自定义会话分组的名称
- `chat category add-conv` — 将会话移动到指定的自定义分组中
- `chat category remove-conv` — 将会话从指定的自定义分组中移出
- `chat mute` — 开启/关闭会话消息免打扰（默认开启，--off 关闭）
- `chat group transfer-owner` — 转让群主
- `chat group invite-url` — 获取群邀请链接
- `chat group share-invite` — 分享群聊链接到会话（--target 指定目标会话，--receiver 指定单聊用户，二选一）
- `chat category create-smart` — 创建智能会话分组（可指定群名称关键词和成员作为匹配规则）
- `chat message reply` — 引用回复消息（在群聊中引用某条消息并回复文字）
- `chat message forward` — 转发单条消息（将一条消息从源会话转发到目标会话）
- `chat set-top` — 设置/取消会话置顶（默认置顶，--off 取消）
- `chat message set-top-msg` — 置顶会话内某条消息（与 chat set-top 不同：后者是会话列表置顶，前者是消息置顶）
- `chat message unset-top-msg` — 取消会话内某条消息的置顶状态
- `chat group-mute` — 全员禁言/取消全员禁言（默认禁言，--off 取消）
- `chat group-mute-member` — 指定群成员禁言/取消禁言（需指定 --users 和 --mute-time）
- `chat group set-admin` — 设置/取消群管理员（默认设为管理员，--off 取消）
- `chat group update-nick` — 设置用户在群内的群昵称
- `chat group update-alias` — 设置群备注（仅自己可见）
- `chat hide` — 在会话列表中隐藏会话（支持单聊/群聊，收到新消息时重新出现）
- `chat mute-at-all` — 关闭/开启 @所有人消息提醒（默认关闭，--off 恢复）
- `chat mute-red-envelope` — 关闭/开启红包消息提醒（默认关闭，--off 恢复）

## openDingTalkId 获取方式

多个命令参数需要 openDingTalkId（如 --open-dingtalk-id、--at-open-dingtalk-ids、--sender-open-dingtalk-id），统一获取方式如下：

1. 若知道姓名：`dws contact user search --query "姓名"` → 直接从结果中获取 openDingTalkId
2. 若只有 userId：先 `dws contact user get --ids <userId>` 获取姓名 → 再 `dws contact user search --query "姓名"` 获取 openDingTalkId

openDingTalkId 为当前用户视角下的目标用户唯一标识，不可跨用户共享。
