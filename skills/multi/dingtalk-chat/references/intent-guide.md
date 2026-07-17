# chat 局部意图消歧

本文件从单 Skill `intent-guide.md` 拆分而来，仅保留与本产品相关的跨产品消歧规则。

| 用户说... | 真实意图 | 应该用 | 不要用 | 理由 |
|---|---|---|---|---|
| "通知群里的人都来开会" | 个人身份群发 | `chat message send` | `chat message send-by-bot` | 以个人身份向群发消息 |
| "让机器人每天推送日报" | 机器人定时推送 | `chat message send-by-bot` | `chat message send` | 需要机器人身份定期发送 |
| "CPU 超过 90% 自动告警" | Webhook 告警 | `chat message send-by-webhook` | `chat message send-by-bot` | 系统告警场景，需自定义 Webhook |
| "拉取一下上周项目群的聊天记录" | 拉取会话消息 | `chat message list` | — | 拉取指定群聊的消息列表 |
| "看看张三发给我的消息" | 按发送者查询消息 | `chat message list-by-sender` | `chat message list --user` | 用户未明确说"单聊"时优先用 list-by-sender（跨单聊/群聊） |
| "拉取和张三的单聊记录" | 拉取单聊消息 | `chat message list --user` | `chat message list-by-sender` | 用户明确说"单聊"时用 list --user |
| "谁@了我/查看提及我的消息" | 查询@我的消息 | `chat message list-mentions` | `chat message list-all` | 都是跨会话时间范围查询，但 list-mentions 只返回@我的消息 |
| "查看我今天的所有消息" | 全量会话消息 | `chat message list-all` | `chat message list` | 用户未指定具体会话时用 list-all（跨所有会话），指定了具体群或人时用 list |
| "搜一下消息里的changefree链接" | 消息搜索 | `chat message search-advanced`（首选） | `chat search` | 推荐首选 search-advanced，它是 search 的严格超集（keyword 可选、支持多群、可叠加发送者/at 维度） |
| "按发送者搜索/指定多个群搜索/多维度搜消息" | 多维度搜索消息 | `chat message search-advanced`（首选） | `chat message search` | 推荐首选，支持关键词、发送者、@我、多个会话等维度组合 |
| "消息发没发成功/查询消息发送状态" | 查询消息发送状态 | `chat message query-send-status` | — | 需要 send 返回的 openTaskId |
| "撤回我发的消息/撤回消息/群主撤回消息/管理员撤回他人消息" | 撤回消息 | `chat message recall` | `chat message recall-by-bot` | recall 撤回个人消息或群主/管理员撤回他人消息，recall-by-bot 撤回机器人消息 |
| "未读消息会话/未读会话列表/我的未读会话" | 未读会话列表 | `chat message list-unread-conversations` | `chat message read-status` | list-unread-conversations 查哪些会话有未读；read-status 查具体消息的已读状态 |
| "谁看了这条消息/消息已读未读/查读状态" | 查询消息已读状态 | `chat message read-status` | `chat message list-unread-conversations` | read-status 查具体消息的已读人员；list-unread-conversations 查未读会话列表 |
| "查看消息的表情回复/拉取消息回复/消息的文字回复" | 批量拉取消息表情回复和文字回复 | `chat message list-emotion-replies` | — | 根据消息 ID 批量查询表情回复和文字回复 |
| "我和风雷的共同群/我们都在哪些群" | 搜索共同群 | `chat search-common` | `chat search` | search-common 按人员搜共同群，chat search 按群名搜索 |
| "查看会话分组/自定义分组" | 获取会话分组 | `chat category list` | — | 获取用户自定义的会话分组列表 |
| "某个分组下有哪些会话" | 分组下会话列表 | `chat category list-conversations` | — | 需先通过 category list 获取分组 ID |
| "创建会话分组/新建分组" | 创建自定义分组 | `chat category create` | — | 需指定 --title 分组名称 |
| "删除会话分组/移除分组" | 删除自定义分组 | `chat category delete` | — | 需指定 --category-id |
| "重命名分组/修改分组名称" | 更新分组名称 | `chat category rename` | — | 需指定 --category-id 和 --title |
| "把会话加到分组/将群移入分组" | 会话加入分组 | `chat category add-conv` | `chat category remove-conv` | add-conv 加入；remove-conv 移出 |
| "把会话从分组移出/从分组中删除" | 会话移出分组 | `chat category remove-conv` | `chat category add-conv` | remove-conv 移出；add-conv 加入 |
| "创建智能分组/新建智能会话分组/按关键词分组" | 创建智能会话分组 | `chat category create-smart` | `chat category create` | create-smart 按关键词/成员智能匹配；create 手动创建普通分组 |
| "分享群链接/把群分享给某人/群链接发到某群" | 分享群聊链接到会话 | `chat group share-invite` | `chat group invite-url` | share-invite 直接分享到目标会话/用户；invite-url 只获取链接不发送 |
| "根据群号查群信息/群号转openConversationId" | 群号查群聊信息 | `chat group get-by-group-id` | `chat search` | 已知数字群号时直接查；用户发消息只给了群号时，先用此工具将群号转为 openConversationId |
| "我创建的群/我管理的群/我是群主的群/我当管理员的群" | 拉取我创建/管理的群 | `chat group list-my-groups` | `chat search` | list-my-groups 按角色（群主/管理员）过滤当前用户的群；chat search 按关键词搜全部群 |
| "引用消息回复/回复那条消息" | 引用回复消息 | `chat message reply` | `chat message send` | reply 引用指定消息回复；send 普通发消息不引用 |
| "转发消息/把消息转到另一个群" | 转发单条消息 | `chat message forward` | `chat message send` | forward 转发已有消息到目标会话；send 是发新消息 |
| "置顶会话/取消置顶" | 设置/取消会话置顶 | `chat set-top` | `chat list-top-conversations` | set-top 设置或取消置顶；list-top-conversations 查看置顶列表 |
| "全员禁言/群禁言" | 全员禁言或解除 | `chat group-mute` | `chat group-mute-member` | group-mute 全员禁言或解除；group-mute-member 指定成员禁言 |
| "禁言某人/指定成员禁言" | 指定成员禁言 | `chat group-mute-member` | `chat group-mute` | group-mute-member 指定成员禁言/解禁；group-mute 全员禁言 |
| "设管理员/取消管理员" | 设置群管理员 | `chat group set-admin` | `chat group invite` | set-admin 设置或取消管理员角色；invite 是邀请入群 |
| "退群/退出群聊/离开群" | 退出群聊 | `chat group quit` | `chat group members remove` | quit 是当前用户自己退群；members remove 是管理员踢别人 |
| "改群头像/更新群头像" | 更新群头像 | `chat group update-icon` | `chat group rename` | update-icon 改群头像；rename 改群名称 |
| "改群设置/群设置开关/群权限/入群许可/禁止私聊" | 更新群设置 | `chat group update-settings` | `chat group set-admin` | update-settings 改群功能开关；set-admin 是管理员角色操作 |
| "改群昵称/设置群昵称/我在群里的名字" | 设置个人群昵称 | `chat group update-nick` | `chat group rename` | update-nick 改自己的群昵称；rename 改群名称 |
| "群备注/给群加备注/修改群备注" | 设置群备注 | `chat group update-alias` | `chat group rename` | update-alias 设置仅自己可见的备注；rename 改群名称全员可见 |
| "隐藏会话/隐藏群聊/隐藏对话" | 隐藏会话 | `chat hide` | `chat mute` | hide 隐藏会话不显示；mute 是免打扰但仍显示 |
| "关闭@所有人通知/屏蔽@all/不接收@所有人" | 关闭 @所有人提醒 | `chat mute-at-all` | `chat mute` | mute-at-all 仅屏蔽 @所有人；mute 是整个会话免打扰 |
| "关闭红包通知/屏蔽红包/不接收红包提醒" | 关闭红包提醒 | `chat mute-red-envelope` | `chat mute` | mute-red-envelope 仅屏蔽红包；mute 是整个会话免打扰 |
| "解散群/解散群聊" | 解散群聊 | `chat group dismiss` | `chat group quit` | dismiss 是群主解散整个群（不可逆）；quit 是当前用户自己退群 |
| "新成员看历史/历史消息可见范围" | 设置新成员可见历史消息 | `chat group set-history` | `chat group update-settings` | set-history 控制新成员入群后可见历史消息范围；update-settings 是其他群功能开关 |
| "群里有哪些机器人/查看群机器人/列出群机器人" | 查看群内机器人列表 | `chat group bots` | `chat group members` | bots 只列机器人；members 列普通群成员 |
| "从群里移除机器人/踢机器人" | 移除群内机器人 | `chat group members remove-bot` | `chat group members add-bot` | remove-bot 通过 openBotId 移除；add-bot 通过 robotCode 添加 |
| "批量查群成员详情/按ID查群成员/查看指定成员信息" | 批量查询群成员详情 | `chat group members list-by-ids` | `chat group members` | list-by-ids 根据 openDingTalkId 列表查询指定成员详情；members 分页查询全部成员列表 |
| "标记未读/设为未读/会话标未读" | 标记会话为未读 | `chat mark-unread` | `chat clear-red-point` | mark-unread 标记为未读；clear-red-point 清除红点（相反操作） |
| "清除红点/已读/取消未读" | 清除会话红点 | `chat clear-red-point` | `chat mark-unread` | clear-red-point 清除单个会话红点；mark-unread 标记未读（相反操作） |
| "全部已读/一键清除所有未读/红点清零" | 清除所有会话红点 | `chat clear-all-red-point` | `chat clear-red-point` | clear-all-red-point 清除所有会话红点；clear-red-point 只清单个会话 |
| "所有会话/全部会话列表/我的会话" | 分页获取全部会话 | `chat list-all-conversations` | `chat list-top-conversations` | list-all-conversations 返回所有会话；list-top-conversations 仅返回置顶会话 |
| "清空聊天记录/删除会话消息/清空消息" | 清空会话聊天记录 | `chat clear-messages` | `chat clear-red-point` | clear-messages 清空聊天记录；clear-red-point 仅清除未读红点 |
| "标记已读/消息已读/读了" | 标记消息已读 | `chat mark-read` | `chat clear-red-point` | mark-read 标记指定消息及之前的消息为已读；clear-red-point 仅清除红点不标记消息 |
| "我加入的所有群/拉取全部群列表/我的群" | 分页拉取所有群 | `chat group list-all` | `chat group list-my-groups` | list-all 返回用户加入的所有群；list-my-groups 仅返回用户作为群主/管理员的群 |
| "入群申请记录/谁申请入群/群申请列表/查看入群审批" | 拉取入群验证记录 | `chat group list-join-validations` | `chat group list-all` | list-join-validations 拉取入群验证/审批记录；list-all 拉取群列表 |
| "通过入群申请/拒绝入群/审批入群/同意加群" | 审批入群验证 | `chat group audit-join-validation` | `chat group list-join-validations` | audit-join-validation 执行审批操作；list-join-validations 仅查询记录 |
| "发群公告/在群里发公告/群公告置顶" | 发布群公告 | `chat group notice create` | `blackboard create` | notice create 是群维度公告（发到指定群聊）；blackboard 是企业公告（面向全员，不可撤回） |
| "改群公告/修改群公告/更新群公告" | 修改群公告 | `chat group notice edit` | `chat group notice create` | edit 整体替换已有公告内容（需 dataId）；create 发布新公告 |
| "查群公告/看群公告/群公告列表/群里有什么公告" | 查看群公告列表 | `chat group notice list` | `blackboard list` | notice list 查指定群的公告；blackboard list 查企业公告 |
| "群公告详情/公告已读人数/谁读了公告" | 查看群公告详情 | `chat group notice get` | `chat group notice list` | get 返回单条公告详情（含已读人数、点赞/评论数）；list 分页拉公告列表 |
| "搜索机器人/找机器人/查机器人/帮我找XXX机器人" | 搜索全部可用机器人 | `chat bot find` | `chat bot search` | find 返回全部可用机器人（含他人/官方），额外返回 openDingTalkId（可用于给机器人发单聊消息）；search 仅返回我创建的 |
| "给机器人发单聊/给机器人发消息/跟机器人聊天" | 给机器人发单聊消息 | `chat bot find` → `chat message send --open-dingtalk-id` | `chat bot search` | 必须先用 find 拿 openDingTalkId（search 没有此字段），再用 send --open-dingtalk-id 发单聊 |
| "我创建的机器人/我的机器人/我自己的机器人/查看我的机器人" | 搜索我创建的机器人 | `chat bot search` | `chat bot find` | search 仅返回当前用户自己创建的机器人（返回 robotCode + robotName，无 openDingTalkId）；find 返回全部可用机器人 |
| "合并转发/批量转发/合并转发多条消息" | 合并转发多条消息 | `chat message combine-forward` | `chat message forward` | combine-forward 合并多条为一条转发；forward 转发单条消息 |
| "转发话题/转发话题消息/话题转发到另一个群" | 转发话题消息 | `chat message forward-topic` | `chat message forward` | forward-topic 专用于转发话题消息（需要话题ID）；forward 转发普通单条消息 |
| "发卡片消息/推送流式卡片" | 创建并推送流式卡片 | `chat message send-card` | `chat message send` | send-card 发流式卡片；send 发普通文本/Markdown 消息 |
| "更新卡片/流式更新卡片" | 流式更新卡片内容 | `chat message update-card` | `chat message send-card` | update-card 更新已有卡片；send-card 创建新卡片 |
| "钉住消息/Pin消息/置顶消息到会话" | 钉住消息 | `chat message set-pin-msg` | `chat set-top` | set-pin-msg 钉住单条消息（Pin）；set-top 置顶整个会话 |
| "取消钉住/Unpin消息" | 取消钉住消息 | `chat message unset-pin-msg` | `chat message set-pin-msg` | unset-pin-msg 取消钉住；set-pin-msg 设置钉住 |
| "查看钉住的消息/Pin列表/钉住消息列表" | 拉取钉住消息列表 | `chat message list-pin-msg` | `chat message list` | list-pin-msg 只返回被钉住的消息；list 拉取全部消息 |
| "置顶某条消息/把这条消息置顶" | 置顶消息 | `chat message set-top-msg` | `chat set-top` | set-top-msg 置顶会话内某条消息；set-top 置顶整个会话在列表中 |
| "取消置顶消息/取消消息置顶" | 取消置顶消息 | `chat message unset-top-msg` | `chat message set-top-msg` | unset-top-msg 取消置顶；set-top-msg 设置置顶 |
| "DING消息/查DING/DING历史" | 查询 DING 消息列表 | `ding message list` | `chat message list` | ding 是独立顶层命令；ding message list 查 DING 消息；chat message list 查普通聊天消息 |
| "DING接收状态/谁收到了DING" | DING 接收状态 | `ding message receiver-status` | `chat message read-status` | ding 是独立顶层命令；receiver-status 查 DING 接收；chat message read-status 查普通消息已读 |
| "发DING/DING通知" | 发送 DING 消息 | `ding message send` | `chat message send` | DING 是钉钉的强提醒（应用内/短信/电话），独立顶层命令；普通群消息用 chat |
| "撤回DING" | 撤回 DING 消息 | `ding message recall` | `chat message recall` | DING 撤回独立命令；chat recall 是撤回普通聊天消息 |
| "搜一下智能化方案/最近 OKR 相关邮件/最近发版相关消息" | 搜企业知识内容 | `aisearch enterprise` | `doc search` / `mail search` / `chat message search` | 跨文档、消息、日程、听记、邮件等企业内容语义检索走 enterprise；具体 `queries/types/time-range` 抽槽见 `aisearch.md` |
| "我发给某人的消息/邮件/文档/今天我干了什么" | 搜行为记录 | `aisearch behavior` | `chat` / `mail` / `doc` / `report` | 关注“谁对什么做过什么”，走 behavior；具体 `behavior-type/direction/chat-scope` 抽槽见 `aisearch.md` |
| "把这段文字翻译成英文/translate this" | 通用文本翻译 | `chat text translate` | `doc` / `aisearch` | 纯文本翻译，不是文档编辑或语义搜索 |
| "帮我把这个文档翻译成日文" | 文档内容翻译 | 先 `doc get` 再 `chat text translate` | `chat text translate` 直接传文件 | translate 仅支持纯文本，需先提取文档内容 |
