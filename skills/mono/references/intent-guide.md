# 意图路由指南

当用户请求难以判断归属哪个产品时，参考本指南。

## 易混淆场景快速对照表

| 用户说... | 真实意图 | 应该用 | 不要用 | 理由 |
|-----------|----------|--------|--------|------|
| "搜一下 OAuth2 接入文档" | 搜索开发文档 | `devdoc` | `doc search` | 搜索开放平台技术文档，不是钉钉内部内容 |
| "帮我建一个项目跟踪表" | 创建数据表格 | `aitable` | `doc` / `sheet` | 涉及结构化数据/行列操作，不是富文本文档或电子表格 |
| "帮我写个项目周报" | 创建钉钉文档 | `doc` | `aitable` | 富文本内容创作，不是数据表 |
| "参照这个生成同样的 / 按模板生成 / 复刻 X / 同样的模板 X 月份的" + 已有 alidocs URL | 模板保形生成同形态变体 | `doc copy + doc rename + doc block update` → 见 [best_practices/04-document.md `template-based-generation`](./best_practices/04-document.md#template-based-generation) | `doc read + doc create`（重写链） | adoc → markdown 是有损投影，read+create 会丢行高/单元格背景色/字号；copy 在 adoc 层保形复制后只在副本上局部修改 |
| "创建一个电子表格" | 创建表格文档 | `sheet` | `aitable` | Excel 式表格/单元格操作，不是多维表记录 |
| "帮我读一下表格 A1:D10 的数据" | 读取单元格数据 | `sheet` | `aitable` | 按单元格区域读写，不是按记录查询 |
| "这个 alidocs 表格链接帮我看下"（粘贴原始 URL） | 先 probe 节点类型 | `dws doc info --node` → 按 `extension` 路由 | 直接调 `sheet` | `alidocs/i/nodes/{id}` 可能是文档/axls/able/xlsx 等，禁止凭 URL 猜类型 |
| "读一下这个 xlsx 的数据" / xlsx 节点链接 | 下载本地表格文件 | `dws drive download --node` | `sheet range read` | xlsx / xls / xlsm / csv 是上传的本地文件（`contentType=DOCUMENT`），sheet 命令只支持在线表格，必须下载后本地解析 |
| "把这个在线表格导出为 xlsx 文件" | 在线表格格式转换 | `dws sheet export` | `dws drive download` | `export` 是 axls → xlsx 的导出转换；`download` 只能下载已有的 xlsx 节点 |
| "帮我记一下明天要做的事" | 创建个人待办 | `todo` | `doc` / `aitable` | 个人待办提醒，非文档内容或数据表 |
| "给自己留一个明天下午的时间块/建个个人日程" | 创建个人日程 | `calendar event create` | `todo` | 个人 schedule 仍属于日历事件，不是待办 |
| "帮我把这个文件传到网盘" | 钉盘上传 | `drive upload` | — | 文件上传是存储层操作，归 drive |
| "上传文件到钉盘/我的文件" | 钉盘上传 | `drive upload` | — | 提到"钉盘/网盘/我的文件"→ drive |
| "上传文件"（未指定目标） | 默认钉盘 | `drive upload` | — | 未明确目标时默认上传到钉盘 |
| "帮我看看知识库里的文件" | 知识库节点列表 | `wiki node list --workspace` | `drive list` | 明确"知识库"上下文 → wiki node list |
| "列出钉盘团队空间" | 列出钉盘空间 | `wiki space list --type orgSpace` | `drive list-spaces` | 空间管理归 wiki，drive list-spaces 已 deprecated |
| "在知识库里搜方案" | 空间内搜索 | `wiki node search --workspace` | `drive search` | 指定了空间上下文 → wiki node search |
| "搜一下有没有叫XX的文件" | 全局搜索 | `drive search` | `wiki node search` | 未指定空间 → drive search 全局聚合搜索 |
| "在知识库里创建一个文档" | 创建空文件实体 | `wiki node create --type adoc` | `doc create` | 空间内创建节点归 wiki；doc create 是向已有文档写入内容，不是创建文件节点 |
| "帮我建一个明天下午的日程" | 日历日程 | `calendar` | — | 日历日程管理（可含参与者/会议室）；视频会议(conference)当前开源 CLI 不支持 |
| "明早 9 点提醒我提交周报" | 创建个人待办，但需先声明 reminder 边界 | `todo` | `calendar` | todo 当前只支持 dueTime 截止时间，不支持独立精确 reminder |
| "帮我建一个项目群" | 创建群聊 | `chat group create` | — | 群聊管理，不是日历日程 |
| "把张三拉进群" | 添加群成员 | `chat group members add` | — | 先查 userId，再添加 |
| "通知群里的人都来开会" | 个人身份群发 | `chat message send` | `chat message send-by-bot` | 以个人身份向群发消息 |
| "让机器人每天推送日报" | 机器人定时推送 | `chat message send-by-bot` | `chat message send` | 需要机器人身份定期发送 |
| "给张三发一条机器人单聊消息" | 机器人单聊 | `chat message send-by-bot --users` | — | 机器人批量单聊，先查 userId |
| "CPU 超过 90% 自动告警" | Webhook 告警 | `chat message send-by-webhook` | `chat message send-by-bot` | 系统告警场景，需自定义 Webhook |
| "帮我看看收到的日报" | 收到的日志 | `report` | `doc` | 钉钉日志系统（日报/周报），不是文档 |
| "帮我创建一个待办提醒" | 个人待办 | `todo` | `report` | 个人任务提醒，不是日志汇报 |
| "拉取一下上周项目群的聊天记录" | 拉取会话消息 | `chat message list` | — | 拉取指定群聊的消息列表 |
| "看看张三发给我的消息" | 按发送者查询消息 | `chat message list-by-sender` | `chat message list --user` | 用户未明确说"单聊"时优先用 list-by-sender（跨单聊/群聊） |
| "拉取和张三的单聊记录" | 拉取单聊消息 | `chat message list --user` | `chat message list-by-sender` | 用户明确说"单聊"时用 list --user |
| "谁@了我/查看提及我的消息" | 查询@我的消息 | `chat message list-mentions` | `chat message list-all` | 都是跨会话时间范围查询，但 list-mentions 只返回@我的消息 |
| "查看我今天的所有消息" | 全量会话消息 | `chat message list-all` | `chat message list` | 用户未指定具体会话时用 list-all（跨所有会话），指定了具体群或人时用 list |
| "搜一下消息里的changefree链接" | 消息搜索 | `chat message search-advanced`（首选） | `chat search` | 推荐首选 search-advanced，它是 search 的严格超集（keyword 可选、支持多群、可叠加发送者/at 维度） |
| "按发送者搜索/指定多个群搜索/多维度搜消息" | 多维度搜索消息 | `chat message search-advanced`（首选） | `chat message search` | 推荐首选，支持关键词、发送者、@我、多个会话等维度组合 |
| "消息发没发成功/查询消息发送状态" | 查询消息发送状态 | `chat message query-send-status` | — | 需要 send 返回的 openTaskId |
| "撤回我发的消息/撤回消息" | 撤回个人消息 | `chat message recall` | `chat message recall-by-bot` | recall 撤回个人消息，recall-by-bot 撤回机器人消息 |
| "未读消息会话/未读会话列表/我的未读会话" | 未读会话列表 | `chat message list-unread-conversations` | `chat message read-status` | list-unread-conversations 查哪些会话有未读；read-status 查具体消息的已读状态 |
| "谁看了这条消息/消息已读未读/查读状态" | 查询消息已读状态 | `chat message read-status` | `chat message list-unread-conversations` | read-status 查具体消息的已读人员；list-unread-conversations 查未读会话列表 |
| "我和风雷的共同群/我们都在哪些群" | 搜索共同群 | `chat search-common` | `chat search` | search-common 按人员搜共同群，chat search 按群名搜索 |
| "查看会话分组/自定义分组" | 获取会话分组 | `chat category list` | — | 获取用户自定义的会话分组列表 |
| "某个分组下有哪些会话" | 分组下会话列表 | `chat category list-conversations` | — | 需先通过 category list 获取分组 ID |
| "创建会话分组/新建分组" | 创建自定义分组 | `chat category create` | — | 需指定 --title 分组名称 |
| "删除会话分组/移除分组" | 删除自定义分组 | `chat category delete` | — | 需指定 --category-id |
| "重命名分组/修改分组名称" | 更新分组名称 | `chat category rename` | — | 需指定 --category-id 和 --title |
| "把会话加到分组/将群移入分组" | 会话加入分组 | `chat category add-conv` | `chat category remove-conv` | add-conv 加入；remove-conv 移出 |
| "把会话从分组移出/从分组中删除" | 会话移出分组 | `chat category remove-conv` | `chat category add-conv` | remove-conv 移出；add-conv 加入 |
| "根据群号查群信息/群号转openConversationId" | 群号查群聊信息 | `chat group get-by-group-id` | `chat search` | 已知数字群号时直接查；用户发消息只给了群号时，先用此工具将群号转为 openConversationId |
| "我创建的群/我管理的群/我是群主的群/我当管理员的群" | 拉取我创建/管理的群 | `chat group list-my-groups` | `chat search` | list-my-groups 按角色（群主/管理员）过滤当前用户的群；chat search 按关键词搜全部群 |
| "引用消息回复/回复那条消息" | 引用回复消息 | `chat message reply` | `chat message send` | reply 引用指定消息回复；send 普通发消息不引用 |
| "转发消息/把消息转到另一个群" | 转发单条消息 | `chat message forward` | `chat message send` | forward 转发已有消息到目标会话；send 是发新消息 |
| "置顶会话/取消置顶" | 设置/取消会话置顶 | `chat set-top` | `chat list-top-conversations` | set-top 设置或取消置顶；list-top-conversations 查看置顶列表 |
| "全员禁言/群禁言" | 全员禁言或解除 | `chat group-mute` | `chat group-mute-member` | group-mute 全员禁言或解除；group-mute-member 指定成员禁言 |
| "禁言某人/指定成员禁言" | 指定成员禁言 | `chat group-mute-member` | `chat group-mute` | group-mute-member 指定成员禁言/解禁；group-mute 全员禁言 |
| "设管理员/取消管理员" | 设置群管理员 | `chat group set-admin` | `chat group members add` | set-admin 设置或取消管理员角色；加人入群用 members add |
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
| "以我的名义发DING/个人发DING/用户身份DING" | 以用户身份发 DING | `ding message send-personal` | `ding message send` | send-personal 以用户身份发送，无需 robot-code；send 以机器人身份发送 |
| "以我的名义撤回DING/个人撤回DING" | 以用户身份撤回 DING | `ding message recall-personal` | `ding message recall` | recall-personal 以用户身份撤回；recall 以机器人身份撤回 |
| "消息转DING/把这条消息DING给某人/转发为DING" | 消息转 DING | `ding message send-by-message` | `ding message send-personal` | send-by-message 是将已有消息转为 DING，需指定原消息；send-personal 是直接发新 DING |
| "把最近几次关于XX的会议汇总成报告" | 按主题汇总多次听记 | [05-reporting.md `generate-topic-report`](./best_practices/05-reporting.md) | [07-minutes.md `meeting-followup`](./best_practices/07-minutes.md) | meeting-followup 是单次会议听记跟进；多次会议按主题汇总属于工作汇报 |
| "整理一下XX项目的所有讨论" | 跨源主题归档 | [05-reporting.md `generate-topic-report`](./best_practices/05-reporting.md) | [04-document.md `write-doc`](./best_practices/04-document.md) | write-doc 侧重单篇文档创作；按主题跨听记/群消息汇总属于工作汇报 |
| "张三在哪个部门/查一下同事工号" | 通讯录精确查询 | `contact`（行动指南 [08-directory.md](./best_practices/08-directory.md)） | 汇报 / 文档 | 需要 userId、手机号、部门 ID 等精确信息时用 contact |
| "研发部的详细信息/部门信息" | 查部门详情 | `contact dept get-info` | `contact dept list-members` | 查部门属性（ID、名称、人数）用 get-info；查成员列表用 list-members |
| "研发部有多少人" | 查部门人数 | `contact dept get-info` | `contact dept list-members` | 问人数用 get-info（返回 memberCount）；问有哪些人用 list-members |
| "找一下张三/搜同事/找人" | AI搜人(首选) | `aisearch person` | `contact user search` | 搜人首选 aisearch，支持姓名/部门/职责/上下级维度；精确查 userId/手机号用 contact |
| "五道的上级是谁/谁负责XX/XX的下属有谁" | AI语义搜人 | `aisearch person` | `contact` | 涉及上下级、职责、负责人等语义维度搜索，用 aisearch |
| "222020这个工号是谁/查工号" | 按工号搜人 | `aisearch person --dimension jobNumber` | `contact` | 工号查人走 aisearch，dimension=jobNumber |
| "13800138000是谁/查手机号" | 按手机号搜人 | `aisearch person --dimension phone` | `contact` | 手机号查人走 aisearch，dimension=phone |
| "搜一下智能化方案/最近 OKR 相关邮件/最近发版相关消息" | 搜企业知识内容 | `aisearch enterprise` | `doc search` / `mail search` / `chat message search` | 跨文档、消息、日程、听记、邮件等企业内容语义检索走 enterprise；具体 `queries/types/time-range` 抽槽见 `aisearch.md` |
| "我发给某人的消息/邮件/文档/今天我干了什么" | 搜行为记录 | `aisearch behavior` | `chat` / `mail` / `doc` / `report` | 关注"谁对什么做过什么"，走 behavior；具体 `behavior-type/direction/chat-scope` 抽槽见 `aisearch.md` |
| "查/提交 请假/加班/补卡 审批单" | 考勤业务审批单 | `attendance approve`（查询走 `attendance approve list`；提交走 `attendance approve templates --type leave\|overtime\|repair-check`） | `oa approval list-pending` / `oa approval records` | 请假/加班/补卡/出差/外出 属于考勤业务审批单，按业务类型查询；`oa approval` 是通用 OA 审批中心，覆盖范围不同 |

---

## 典型场景详解

### 1. aitable vs doc vs sheet — 数据表格 vs 文档内容 vs 电子表格

**用 `aitable` 的场景**：
- "创建一个表格记录团队成员信息" — 结构化数据，有行列
- "在表格里加一列'状态'字段" — 字段/列操作
- "查一下表格里所有优先级为高的记录" — 数据筛选和查询
- "用项目管理模板建一个表" — 模板创建
- 用户提到"多维表"、"Base"、"数据表"、"记录"

**用 `doc` 的场景**：
- "帮我写个会议纪要" — 富文本内容创作
- "看一下这个文档链接的内容" — 阅读文档
- 用户提到"文档"、"知识库"、"写文档"

**用 `sheet` 的场景**：
- "创建一个电子表格" — 创建 Excel 式在线表格
- "帮我读一下这个表格 A1 到 D10 的数据" — 按单元格区域读取
- "在 B2 写入一个 SUM 公式" — 写入公式/值到单元格
- "帮我看看这个表格有哪些工作表" — 工作表管理
- 用户提到"电子表格"、"Excel"、"工作表"、"Sheet"、"单元格"、"公式"

**三者判断关键**：
- 有字段定义/记录增删改查/数据筛选 → `aitable`
- 纯文本/Markdown/富文本编辑 → `doc`
- 单元格区域读写/公式/多工作表 → `sheet`
- 个人任务清单（无行列/字段概念）→ `todo`

**易误判场景**：
- "在知识库中新建一个表格" — 指在钉钉文档空间创建表格类型节点 → `wiki node create --type axls`（不是 `aitable`）
- "帮我建个表记录项目进度" — 指创建结构化数据表 → `aitable`

---

### 1.1 xlsx vs axls — 本地表格文件 vs 在线电子表格

alidocs 链接表面长得一样（`https://alidocs.dingtalk.com/i/nodes/{id}`），但节点类型完全不同。sheet 产品线只服务 axls（在线电子表格），xlsx / xls / xlsm / csv 等本地表格文件必须走 `dws drive download`，严禁错路由。

用 `sheet` 的场景（axls，钉钉在线电子表格）:
- `dws doc info --node <URL>` 返回 `contentType=ALIDOC` + `extension=axls`
- 用户在钉钉文档空间直接"新建电子表格"得到的节点
- 所有 sheet 子命令（`list` / `range read` / `range write` / `export` 等）仅服务这类节点

用 `dws drive download` 的场景（xlsx / xls / xlsm / csv 本地表格文件）:
- `dws doc info --node <URL>` 返回 `contentType=DOCUMENT` + `extension=xlsx` / `xls` / `xlsm` / `csv`
- 用户把本地 Excel 文件上传到文档空间得到的节点，本质是"文件 + 预览"，非在线表格
- sheet 命令直接调用会报错，必须先 `dws drive download --node <URL>` 下载到本地再解析处理

判断关键：
- 未知 alidocs URL → 必须先 `dws doc info --node <URL> --format json` 探测 `contentType` 与 `extension`
- `contentType=ALIDOC` + `extension=axls` → `sheet`
- `contentType=DOCUMENT` + `extension=xlsx` / `xls` / `xlsm` / `csv` → `dws drive download`
- 用户说"把在线表格导出为 xlsx 文件" → `dws sheet export`（axls → xlsx 的格式转换，不是读取 xlsx）

易误判场景：
- 用户粘贴一个 alidocs 链接说"读一下这个表格" — 不能直接调 `sheet range read`，必须先 probe 再按 `extension` 路由
- 用户说"读一下这个 xlsx 文件里的数据" — 走 `dws drive download` 下载后本地解析，不要走 `sheet`
- 用户说"把这个在线表格导出为 xlsx" — 走 `dws sheet export`，不要走 `dws drive download`（后者只能下载已有的 xlsx 节点，无法从 axls 生成）

详见 [url-patterns.md](./url-patterns.md) 和 [sheet.md 适用范围](./products/sheet.md)。

---

### 2. devdoc vs drive search / wiki node search — 两种搜索

**用 `devdoc` 的场景**：
- "API 调用报错 403 怎么解决" — 开发调试问题
- "搜一下 OAuth2 接入文档" — 开放平台技术文档
- "CLI 命令出错了怎么办" — CLI 使用错误
- 用户提到"开发"、"API"、"调用错误"

**用 `drive search` / `wiki node search` 的场景**：
- "在我的文档里搜一下'项目方案'" — 全局搜索用 `drive search`
- 用户明确说"某个知识库里搜" — 空间内搜索用 `wiki node search --workspace`

**判断关键**：搜开发文档→ `devdoc`；搜用户自己的文档→ `drive search`（全局）或 `wiki node search`（空间内）

---

### 3. drive vs doc vs wiki — 存储层 vs 内容层 vs 空间管理层

> **两层模型判定口诀**：如果操作换一种文件类型还能成立，就是存储层（→ drive）；操作只对特定格式有意义，就是内容层（→ doc/sheet）；操作是对空间/节点的组织管理，就是空间管理层（→ wiki）。

**用 `drive`（存储层）的场景**：
- "把这个 PDF 传到钉盘" — 上传文件（不关心格式）
- "下载那个 Excel 附件" — 下载文件（不关心格式）
- "看一下钉盘根目录有什么文件" — 浏览文件列表
- "搜一下有没有叫季度汇报的文件" — 全局搜索文件实体 (`drive search`)
- "把这个文档复制一份" — 复制文件实体
- "把文件移到另一个文件夹" — 移动文件实体
- "改一下文件名" — 重命名文件实体
- "给张三加个编辑权限" — 权限管理
- 用户提到"钉盘"、"网盘"、"上传"、"下载"、"搜文件"、"找文件"、"复制"、"移动"、"重命名"、"权限"

**用 `doc`（内容层）的场景**：
- "读一下这个文档的内容" — 读取文档 Markdown（仅 adoc 有意义）
- "帮我写入一段话到文档里" — 编辑文档内容（仅 adoc 有意义）
- "在第三段后面插入一个表格" — 块级编辑（仅 adoc 有意义）
- "给这段内容加个评论" — 文档评论（仅 adoc 有意义）
- "把这个文档导出为 docx" — 文档导出（当前仅 adoc 支持）
- 用户提到"读文档内容"、"写文档"、"编辑文档"、"块级编辑"、"文档评论"、"导出文档"

**用 `wiki`（空间管理层）的场景**：
- "列出所有知识库" — 空间列表 (`wiki space list`)
- "列出钉盘团队空间" — 钉盘空间列表 (`wiki space list --type orgSpace`)
- "在产品知识库里搜一下方案" — 空间内搜索 (`wiki node search --workspace`)
- "在知识库里创建一个空白文档" — 创建节点 (`wiki node create --type adoc`)
- "把这个文件移到另一个知识库" — 跨知识库移动 (`drive move`)
- "给知识库加个成员" — 成员管理 (`wiki member add`)
- 用户提到"知识库"、"团队空间"、"空间成员"、"空间内搜索"、"列出空间"

**判断关键（两层模型）**：
- 操作不关心文件格式 → `drive`（存储层）
- 操作仅对特定文档格式有意义 → `doc` / `sheet`（内容层）
- 操作是对空间/节点的组织管理 → `wiki`（空间管理层）

**搜索场景路由**：
- "搜文件" / "找文件"（不指定空间） → `drive search`（全局聚合搜索）
- "在某个知识库里搜" → `wiki node search --workspace <id>`（空间内搜索）

**创建场景路由**：
- "在知识库里创建一个文档" → `wiki node create --type adoc`（创建空文件实体）
- "帮我写一篇项目周报" → `doc create`（创建并写入内容）
- "创建一篇文档并写入内容" → 先 `wiki node create` 再 `doc update`（先创建实体，再写内容）

**列表场景路由**：
- "列出钉盘文件" → `drive list`
- "列出知识库里的文件" → `drive list --workspace <id>` 或 `wiki node list --workspace <id>`
- "列出所有空间" → `wiki space list`

---

### 4. 视频会议 vs 日历日程 — conference 不在开源 CLI

当前开源 CLI **不提供**视频会议（conference）命令。"预约视频会议/开个会/发起会议/会中控制（静音/共享屏幕/录制）/呼叫入会"等诉求，直接告知用户请在钉钉客户端完成，不要试探 `dws conference` 或用其他命令变通。

**用 `calendar` 的场景**（这些都不是视频会议）：
- "明天下午安排个会" — 日程管理（可含会议室）
- "给自己留两个小时写方案/建个个人日程" — 个人日历事件，仍用 `calendar event create`
- "帮我约几个人开会" — 创建日程 + 添加参会人
- "看看下午有没有空闲会议室" — 会议室管理
- "帮我查一下同事有空吗" — 闲忙查询
- 用户提到"日程"、"会议室"、"约会"

---

### 5. chat 内部 — 消息发送与撤回

**用 `chat message send` 的场景**：
- "帮我在群里发个消息提醒大家" — **个人身份**发群消息
- "发个单聊消息给某人" — 个人身份发单聊：
  - 已有 userId 时直接使用 `--user`；已有 openDingTalkId 时使用 `--open-dingtalk-id`
  - 纯文本/Markdown 单聊传 `--user` 时直接走 userId 发送能力，不需要先手动查询 openDingTalkId
  - 富媒体消息（image/file）单聊优先使用 `--open-dingtalk-id`；传 `--user` 时 CLI 会尝试解析为 openDingTalkId 后发送
- "发张图片/截图/语音/视频/文件到群里" / "发张图给某某" — **统一一条命令**：`dws chat message send ... --msg-type file --file-path <本地路径>`，CLI 内部自动上传并发送，**任意扩展名（png/jpg/pdf/mp4/zip…）都走这条**
- "发图片+文字说明" — 不要硬塞进一条命令；先发文件消息再补一条 `--text "..."` 即可

```bash
dws chat message send --group <openConversationId> --msg-type file --file-path ./screenshot.png --format json
dws chat message send --open-dingtalk-id <openDingTalkId> --msg-type file --file-path ./report.pdf --format json
```

> ❌ 反模式：调 `dt_media_upload` / `extract_media_id.py` / `drive upload` / `drive download` 等前置工具再 `--msg-type image --media-id`。这是**旧链路**，仅当上游已持有 mediaId 才用；新场景一律 `--file-path` 直发，避免长链路与"空白图"现象。
> 富媒体消息单聊优先使用 `--open-dingtalk-id`；传 `--user` 时 CLI 会尝试解析为 openDingTalkId 后发送。

**用 `chat message send-by-bot` 的场景**：
- "让机器人在群里发一条通知" — **机器人身份**发消息
- "给张三发一条机器人单聊消息" — 机器人单聊

**用 `chat message send-by-webhook` 的场景**：
- "通过 Webhook 发告警到群里" — 自定义机器人 Webhook
- 用户有 Webhook Token

**用 `chat message recall-by-bot` 的场景**：
- "撤回刚才机器人发的消息" — 需要 robot-code + processQueryKey

用 `chat message recall` 的场景：
- "撤回我刚发的消息" — 撤回以个人身份发送的消息，需要 openConversationId + openMessageId

用 `chat message query-send-status` 的场景：
- "消息发没发成功/查询消息发送状态" — 查询个人发送消息的状态，需要 send 返回的 openTaskId

用 `chat message search-advanced` 的场景（推荐首选）：
- "按发送者搜索消息/指定多个群搜索/@我的消息多维度搜" — 支持关键词、发送者、@我、@指定人、多个会话等维度组合搜索
- 替代关系：完全替代 `chat message search`（严格超集：keyword 可选 vs 必填，支持多群 vs 单群）；大部分替代 `chat message list-by-sender`（--user/--users 覆盖按 userId 搜索发送者，--sender-ids 覆盖按 openDingTalkId 搜索）和 `chat message list-mentions`（--at-me 覆盖核心功能）
- 不能替代：`chat message list-focused`（「特别关注人」是独立维度）
- 默认使用 search-advanced，仅在上述不适用场景才降级到具体命令

**不支持的场景**：
- "撤回我刚发的消息"（但不知道消息 ID） — 需先通过消息拉取或搜索接口（如 `chat message list`、`chat message search-advanced` 等）获取 openMessageId，再调用 `chat message recall`

判断关键：个人发→ `send`；机器人发→ `send-by-bot`；有 Webhook Token→ `send-by-webhook`；个人撤回→ `recall`；机器人撤回→ `recall-by-bot`；查发送状态→ `query-send-status`；消息搜索类意图优先路由到 `search-advanced`（推荐首选），仅在不适用时降级到具体命令

---

### 6. report vs doc vs todo — 日志 vs 文档 vs 待办

**用 `report` 的场景**：
- "帮我看看收到的日报" — 收件箱列表 (`report inbox list`)
- "帮我写/提交今天的日报（钉钉日志模版）" — 先 `report template list` / `template get`，再 `report entry submit`
- "有什么日志模版" — 查看模版 (`report template list`)
- "看看这个日志的已读统计" — 阅读状态 (`report entry stats`)
- "我发过的日志有哪些" — 发件箱列表 (`report outbox list`)
- 用户提到"日报"、"周报"、"日志"

**用 `doc` 的场景**：
- "帮我写个项目总结文档" — 长文本创作（钉钉在线文档，非日志模版）

**用 `todo` 的场景**：
- "记一下这周要做的事" — 个人任务管理
- "创建一个待办提醒" — 仍归 `todo`，但要先说明当前只有 dueTime 截止时间，没有独立 reminder schedule

**判断关键**：钉钉日志系统(日报/周报模版，含按模版创建汇报)→ `report`；文档/知识库长文→ `doc`；任务清单→ `todo`

---

### 6.1 attendance approve vs oa approval — 考勤业务审批 vs 通用 OA 审批

**用 `attendance approve` 的场景**（考勤业务审批单）：
- "上周谁请假了 / 某人近期的加班记录 / 外出出差单 / 补卡审批单" — `attendance approve list --types overtime,leave,trip,patch`（`trip` 同时覆盖出差与外出，两者合并为同一类，查询不再细分）
- "查看考勤审批模板 / 帮我提交考勤审批" — `attendance approve templates --type leave|overtime|repair-check`（repair-check/patch=补卡）
- 用户提到"请假单 / 加班单 / 出差单 / 补卡单 / 考勤审批"等考勤上下文

**用 `oa approval` 的场景**（通用 OA 审批中心）：
- "我的待审批 / 待办审批 / 已审批记录" — `oa approval list-pending`
- "某审批单详情 / 同意或驳回审批 / 撤销我发起的审批" — `oa approval detail/approve/reject/revoke`
- "查业务审批记录 / 审批转交 / 添加评论与抄送" — `oa approval records/transfer/comment/cc`
- 用户提到"报销 / 采购 / 用印 / 合同 等非考勤类审批"

**判断关键**：
- 审批主题明确是【请假 / 加班 / 外出 / 出差 / 补卡】 → `attendance approve`
- 不限于考勤业务、面向"我上下游的审批任务"表述 → `oa approval`

**提交审批单的边界**：
- 提交考勤审批单走 `attendance approve templates --type leave|overtime|repair-check`（当前仅支持请假/加班/补卡三类模板；出差/外出的提交模板暂不支持，查询仍可用 `approve list --types trip` 覆盖），命令会返回审批表单的 submitUrl 跳转链接，由用户点击链接跳转到钉钉客户端的提交页面完成填写与提交。**展示链接时必须用 Markdown 可点击格式 `[表单名称](submitUrl)`，不要裸露 URL**。
- 提交诉求的辅助查询：可用假期余额走 `attendance vacation balance`、历史已提交记录走 `attendance approve list`。
- 任何场景下都**不要误用 `oa approval` 代替** —— 该命令组只能查/审/撤已存在的审批单，考勤业务审批单走考勤自己的逻辑便于区分。

---

### 7. 纯通讯录查询 vs 跨产品

**仅查人/部门/成员/归属/组织关系**（没有「发消息、写文档、建待办」等第二动作）→ 属通讯录场景（行动指南 [08-directory.md](./best_practices/08-directory.md)），不要走汇报或文档场景。

**多轮对话**：用户先说「搜 userId」再说「要详细资料」→ 仍属通讯录场景；第二步 **必须** 执行 `contact user get --ids`，禁止只用第一次 `user search` 的浅表字段交差。

**与消息场景区分**：终点是「把消息发给某人」→ 消息场景（[01-messaging.md](./best_practices/01-messaging.md)）；终点是「某人 userId/部门是什么」→ 通讯录。可先通讯录解析 ID 再发消息。

**与发邮件、待办、日程混排**：先后顺序与口径见 [08-directory.md](./best_practices/08-directory.md#与其他场景消歧)。

**特别关注列表查询**：用户说"我关注了谁/我的特别关注列表/我的星标联系人/特别关注的人有哪些" → `dws contact relation list-my-followings`（无入参）。

- 与 `chat message list-focused` 区分：前者拉"人员列表"，后者拉"这些人发的消息"。
- **可执行判断口径（按顺序扫描）**：
  1. 扫描 query 是否含动词【发/说/聊/讲】或名词【消息/聊天/动态/最新内容】 → 含则**强制**路由到 `chat message list-focused`，**忽略**主语中的"关注/特别关注/星标"。
  2. 仅含"关注/特别关注/星标"+"人/列表/谁/有哪些/多少" → 路由到 `dws contact relation list-my-followings`。
- **反例 query（绝不路由到 list-my-followings）**：
  - "我特别关注的人最近发了什么消息" → `chat message list-focused`
  - "关注的人最近都说了啥" → `chat message list-focused`
  - "星标联系人发的群消息" → `chat message list-focused`
- **正例 query（路由到 list-my-followings）**：
  - "我特别关注的人有哪些"、"我关注了谁"、"我的星标联系人"

---

## 跨产品工作流路由

以下场景需要多个产品配合完成，注意上下文传递顺序。多步骤操作有现成脚本时优先使用脚本。

### 发邮件给同事（aisearch → contact → mail）

用户说"给张三发封邮件"，但只知道名字不知道邮箱地址：

```bash
# 1. 搜人获取 userId（多人同名须 contact user get 消歧，禁止默认选第一个，详见 08-directory.md「多命中」）
dws aisearch person --keyword "张三" --dimension name --format json

# 2. 用 userId 查详情获取 email
dws contact user get --ids <userId> --format json

# 3. 用搜索到的邮箱地址作为收件人发送邮件
dws mail mailbox list --format json  # 获取发件人邮箱
dws mail message send --from my@company.com --to zhangsan@company.com \
  --subject "周报" --body "内容" --format json
```

### 创建日程并邀请同事（aisearch → calendar）

用户说"约张三明天下午开会"：

> 有脚本: `python scripts/calendar_schedule_meeting.py --title "会议" --start "..." --end "..." --users userId1 --book-room`

```bash
# 手动流程（脚本不可用时）:
# 1. 搜人获取 userId（多人同名须 contact user get 消歧，禁止默认选第一个，详见 08-directory.md「多命中」）
dws aisearch person --keyword "张三" --dimension name --format json

# 2. 创建日程
dws calendar event create --title "会议" \
  --start "2026-03-15T14:00:00+08:00" --end "2026-03-15T15:00:00+08:00" --format json

# 3. 添加参会人
dws calendar attendee add --event <EVENT_ID> --attendees <USER_ID> --format json
```

### 创建待办并指派（aisearch → todo）

用户说"给张三建个待办"：

```bash
# 1. 搜人获取 userId（多人同名须 contact user get 消歧，禁止默认选第一个，详见 08-directory.md「多命中」）
dws aisearch person --keyword "张三" --dimension name --format json

# 2. 创建待办
dws todo task create --title "任务内容" --executors <USER_ID> --format json
```

---

## 图片 / 文件发送

### 发送图片 / 本地文件（统一一条命令）

用户说"发张图/把这张图发给某某/发个 PDF/发个语音/发个视频/发个文件"等任何场景，**统一一条命令**：

```bash
# 群聊
dws chat message send --group <openConversationId> --msg-type file --file-path <本地路径> --format json

# 单聊（推荐 --open-dingtalk-id；--user 也支持）
dws chat message send --open-dingtalk-id <openDingTalkId> --msg-type file --file-path <本地路径> --format json
```

支持任意扩展名（`.png/.jpg/.gif/.bmp/.webp/.pdf/.doc/.xls/.zip/.mp3/.wav/.mp4/.avi` …），CLI 自动识别并处理。**无需** `dt_media_upload` / `extract_media_id.py` / `drive upload` / `drive download` / `chat conversation-info` / `chat file upload` 等任何前置工具调用。

### 图片/文件 + 文字说明

不要把文字塞进 `--msg-type file` 命令（该命令不读 `--text`）。先发文件再补一条文本消息即可：

```bash
dws chat message send --open-dingtalk-id <openDingTalkId> --msg-type file --file-path ./screenshot.png --format json
dws chat message send --open-dingtalk-id <openDingTalkId> --text "这是本周数据汇总" --format json
```

### 旧链路（mediaId）— 仅兼容场景

仅当上游已经通过 `dt_media_upload` 拿到 `@lQL...` 形式的 mediaId 时使用：

```bash
dws chat message send --group <openConversationId> --msg-type image --media-id "@lQLPD4JNnliqBq3NBQDNA8Cw" --format json
```
