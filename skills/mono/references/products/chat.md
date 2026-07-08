# 会话与群聊 (chat) 命令参考

## 命令总览

### group (群组管理)

#### 创建群 — 当前登录用户自动成为群主
```
Usage:
  dws chat group create [flags]
Example:
  dws chat group create --name "Q1 项目冲刺群" --users userId1,userId2,userId3
Flags:
      --users string    成员 userId 列表，用户本身会自动加入，无需包含，逗号分隔，不超过20个 (必填)
      --name string     群名称 (必填)
```

#### 查看群成员列表 — 分页查询指定群聊的成员
```
Usage:
  dws chat group members [flags]
Example:
  dws chat group members --id <openconversation_id>
Flags:
      --cursor string   分页游标，首次从 0 开始
      --id string       群 ID / openconversation_id (必填)
```

#### 添加群成员 — 向指定群聊添加成员，需传入群 ID 与用户 ID 列表
```
Usage:
  dws chat group members add [flags]
Example:
  dws chat group members add --id <openconversation_id> --users userId1,userId2
Flags:
      --id string      群 ID / openconversation_id (必填)
      --users string   要添加的用户 userId 列表，逗号分隔 (必填)
```

#### 移除群成员 — 从指定群聊中移除成员，需传入群 ID 与待移除的用户 ID 列表
```
Usage:
  dws chat group members remove [flags]
Example:
  dws chat group members remove --id <openconversation_id> --users userId1,userId2
Flags:
      --id string      群 ID / openconversation_id (必填)
      --users string   要移除的用户 userId 列表，逗号分隔 (必填)
```
> 移除对象是群主时 CLI 会拦截并报错，提示先用 `chat group transfer-owner` 转让群主（防止产生无主群）；查不到群主时放行。

#### 将机器人添加到群中 — 将自定义机器人添加到当前用户有管理权限的群聊中，如果没有权限则会报错
```
Usage:
  dws chat group members add-bot [flags]
Example:
  dws chat group members add-bot --robot-code <robot-code> --id <openconversation_id>
Flags:
      --id string           群聊 openConversationId (必填)
      --robot-code string   机器人 Code (必填)
```

#### 从群内移除机器人 — 将指定机器人从群聊中移除，需要群管理员或群主权限
```
Usage:
  dws chat group members remove-bot [flags]
Example:
  dws chat group members remove-bot --id <openConversationId> --bot-id <openBotId>
  # 查询群 ID: dws chat search --query "群名"
  # 查询群内机器人: dws chat group bots --group <openConversationId>
Flags:
      --id string       群聊 openConversationId (必填)
      --bot-id string   机器人 openBotId (必填)
```

#### 根据成员 ID 批量查询群成员详情 — 传入成员 openDingTalkId 列表批量查询
```
Usage:
  dws chat group members list-by-ids [flags]
Example:
  dws chat group members list-by-ids --id <openConversationId> --users openDingTalkId1,openDingTalkId2
  # 查询群 ID: dws chat search --query "群名"
  # 查询 openDingTalkId: dws contact user search --query "姓名"
Flags:
      --id string      群 ID / openConversationId (必填)
      --users string   成员 openDingTalkId 列表，逗号分隔 (必填)
```

#### 更新群名称
```
Usage:
  dws chat group rename [flags]
Example:
  dws chat group rename --id <openconversation_id> --name "新群名"
Flags:
      --id string     群 ID / openconversation_id (必填)
      --name string   修改后的群名称 (必填)
```

#### 根据群号获取群聊信息 — 当用户只提供了数字群号而非 openConversationId 时，用此命令转换
```
Usage:
  dws chat group get-by-group-id [flags]
Example:
  dws chat group get-by-group-id --group-id 12345678
  # 群号为数字类型的群ID
Flags:
      --group-id int   群号 (必填，数字类型)
```

#### 转让群主 — 将群主身份转让给群内其他成员
```
Usage:
  dws chat group transfer-owner [flags]
Example:
  dws chat group transfer-owner --group <openConversationId> --new-owner <openDingTalkId>
  dws chat group transfer-owner --group <openConversationId> --user <userId>
  # 查询群 ID: dws chat search --query "群名"
  # 查询人员: dws aisearch person --keyword "姓名" --dimension name
Flags:
      --group string       群聊 openConversationId (必填)
      --new-owner string   新群主 openDingTalkId
      --user string        新群主 userId
```

#### 获取群邀请链接 — 获取指定群聊的邀请加入链接

可选 --expires-seconds 指定链接有效期（秒），0 表示永久有效，不传则使用服务端默认值。
```
Usage:
  dws chat group invite-url [flags]
Example:
  dws chat group invite-url --group <openConversationId>
  dws chat group invite-url --group <openConversationId> --expires-seconds 86400
  dws chat group invite-url --group <openConversationId> --expires-seconds 0
  # 查询群 ID: dws chat search --query "群名"
Flags:
      --group string            群聊 openConversationId (必填)
      --expires-seconds int64   链接有效期（秒），0 表示永久有效，不传使用服务端默认值
```

#### 退出群聊 — 当前用户退出指定群聊
```
Usage:
  dws chat group quit [flags]
Example:
  dws chat group quit --group <openConversationId>
  # 查询群 ID: dws chat search --query "群名"
Flags:
      --group string   群聊 openConversationId (必填)
```
> ⚠️ 唯一群主 quit 会直接成功，产生无主群，之后对该群做 dismiss / members 等管理操作会报 listRoles null(11056) 无法管理。与 `chat group members remove` 移除群主有本地拦截不同，quit 无此防护。作为唯一群主想退群，应先 `chat group transfer-owner` 转让群主，或 `chat group dismiss` 解散群。

#### 更新群头像 — 更新指定群聊的群头像
```
Usage:
  dws chat group update-icon [flags]
Example:
  dws chat group update-icon --group <openConversationId> --icon-media-id <mediaId>
  # 查询群 ID: dws chat search --query "群名"
Flags:
      --group string          群聊 openConversationId (必填)
      --icon-media-id string  群头像 mediaId (必填)
```
> `--icon-media-id` 有本地格式校验：必须是 `@` 开头的媒体 ID（如 `dt_media_upload` 的返回值），非法格式会在本地直接报错。
> ⚠️ 本地格式校验只查前缀。格式合法但不真实存在的 mediaId 服务端仍会静默返回成功，头像并不会真正更新。务必用 `dt_media_upload` / `chat media upload` 上传真实图片拿到的 mediaId。

#### 更新群设置 — 更新指定群聊的设置项

--setting-key 指定设置项，--status 指定值（0=关闭，1=开启）。

支持的 settingKey:
  authority、joinValidation、onlyAdminCanAtAll、searchable、addFriendForbidden、
  toolbarStatus、pluginCustomizeVerify、onlyAdminCanDING、allMembersCanCreateMcsConf、
  onlyAdminCanSetMsgTop、onlyAdminCanPinMsg、onlyAdminCanSendFile、
  allMembersCanCreateCalendar、groupEmailDisabled、groupRedEnvelopeSwitch、
  groupLiveAuthority、groupBillAuthority
```
Usage:
  dws chat group update-settings [flags]
Example:
  dws chat group update-settings --group <openConversationId> --setting-key searchable --status 1
  dws chat group update-settings --group <openConversationId> --setting-key onlyAdminCanAtAll --status 0
  # 查询群 ID: dws chat search --query "群名"
Flags:
      --group string        群聊 openConversationId (必填)
      --setting-key string  群设置项 key (必填)
      --status int          设置值: 0=关闭, 1=开启 (必填)
```

#### 设置群备注 — 给群设置只有自己可见的备注标题
```
Usage:
  dws chat group update-alias [flags]
Example:
  dws chat group update-alias --group <openConversationId> --alias-title "项目A群"
  # 查询群 ID: dws chat search --query "群名"
Flags:
      --group string         群聊 openConversationId (必填)
      --alias-title string   群备注标题 (必填)
```

#### 设置我在群内的群昵称 — 设置当前用户在指定群里显示的昵称
```
Usage:
  dws chat group update-nick [flags]
Example:
  dws chat group update-nick --group <openConversationId> --nick "我的群昵称"
  # 查询群 ID: dws chat search --query "群名"
Flags:
      --group string   群聊 openConversationId (必填)
      --nick string    个人群昵称 (必填)
```

#### 查看群内所有机器人 — 获取指定群聊中的所有机器人列表
```
Usage:
  dws chat group bots [flags]
Example:
  dws chat group bots --group <openConversationId>
  # 查询群 ID: dws chat search --query "群名"
Flags:
      --group string   群聊 openConversationId (必填)
```

#### 解散群聊 — 解散指定群聊，操作不可逆，需要群主权限
```
Usage:
  dws chat group dismiss [flags]
Example:
  dws chat group dismiss --group <openConversationId>
  # 查询群 ID: dws chat search --query "群名"
Flags:
      --group string   群聊 openConversationId (必填)
```

#### 设置新成员入群可查看历史消息选项 — 控制新加入成员可见的历史消息范围
```
Usage:
  dws chat group set-history [flags]
Example:
  dws chat group set-history --group <openConversationId> --option RECENT_100
  dws chat group set-history --group <openConversationId> --option FORBIDDEN
  # 查询群 ID: dws chat search --query "群名"
Flags:
      --group string    群聊 openConversationId (必填)
      --option string   可见范围: FORBIDDEN | RECENT_100 | ALL (必填)

注意:
  - FORBIDDEN：禁止查看历史消息（默认安全策略）
  - RECENT_100：可查看最近 100 条消息（最常用）
  - ALL：可查看全部历史消息（开放性最高）
```

#### 拉取我创建/管理的群 — 查询当前用户作为群主或管理员的群列表

可通过 --role 过滤角色：OWNER 仅群主、ADMIN 仅管理员，不传则返回全部。可通过 --limit 限制返回数量，不传则返回所有符合条件的群。
```
Usage:
  dws chat group list-my-groups [flags]
Example:
  dws chat group list-my-groups
  dws chat group list-my-groups --role OWNER
  dws chat group list-my-groups --role ADMIN --limit 10
Flags:
      --role string    角色过滤: OWNER(仅群主) / ADMIN(仅管理员)，不传返回全部
      --limit int      最多返回群数量，不传返回全部

注意:
  - 底层先拉取最近 1000 条会话，剔除单聊和话题圈后筛选出群主/管理员的群
  - 内部群会校验 orgId 归属
  - 不传 --role 时返回群主 + 管理员的所有群
```

### group-role (群身份管理)

#### 查看群身份列表 — 拉取指定群聊的自定义群身份列表
```
Usage:
  dws chat group-role list [flags]
Example:
  dws chat group-role list --group <openConversationId>
Flags:
      --group string   群聊 openConversationId (必填)
```

#### 添加群身份 — 在指定群中创建一个新的自定义群身份
```
Usage:
  dws chat group-role add [flags]
Example:
  dws chat group-role add --group <openConversationId> --name "管理员"
Flags:
      --group string   群聊 openConversationId (必填)
      --name string    群身份名称 (必填)
```

#### 更新群身份名称 — 修改指定群身份的名称
```
Usage:
  dws chat group-role update [flags]
Example:
  dws chat group-role update --group <openConversationId> --role-id <openRoleId> --name "新名称"
Flags:
      --group string     群聊 openConversationId (必填)
      --role-id string   群身份 openRoleId，由 group-role list 返回 (必填)
      --name string      群身份新名称 (必填)
```

#### 删除群身份 — 删除指定群聊中的某个自定义群身份
```
Usage:
  dws chat group-role remove [flags]
Example:
  dws chat group-role remove --group <openConversationId> --role-id <openRoleId>
Flags:
      --group string     群聊 openConversationId (必填)
      --role-id string   群身份 openRoleId，由 group-role list 返回 (必填)
```

#### 设置用户群身份 — 覆盖指定用户在群中的全部群身份（传空则清除所有身份）
```
Usage:
  dws chat group-role set-user [flags]
Example:
  dws chat group-role set-user --group <openConversationId> --user <userId> --role-ids roleId1,roleId2
  # 查询人员: dws aisearch person --keyword "姓名" --dimension name
  # 查询 role-id: dws chat group-role list --group <openConversationId>
Flags:
      --group string      群聊 openConversationId (必填)
      --user string       用户 userId（必填）
      --role-ids string   群身份 openRoleId 列表，逗号分隔 (必填)，传空字符串则清除该用户所有群身份
```

#### 移除用户的指定群身份 — 从用户身上移除指定的群身份（不影响其他群身份）

> ⚠️ 当前不可用：该命令当前调用常失败（服务端返回 1002 系统繁忙），多为服务端侧限制。需要清除某人身份时，用 `chat group-role set-user --role-ids ""`（传空 role-ids 覆盖为无身份）兜底；但 set-user 会清掉该用户的**全部**身份，无法只移除其中一个。
```
Usage:
  dws chat group-role remove-user [flags]
Example:
  dws chat group-role remove-user --group <openConversationId> --user <userId> --role-ids roleId1,roleId2
Flags:
      --group string      群聊 openConversationId (必填)
      --user string       用户 userId（必填）
      --role-ids string   要移除的群身份 openRoleId 列表，逗号分隔 (必填)
```

#### 查询群成员的群身份 — 查询指定群成员当前持有的所有群身份
```
Usage:
  dws chat group-role query-user [flags]
Example:
  dws chat group-role query-user --group <openConversationId> --user <userId>
Flags:
      --group string   群聊 openConversationId (必填)
      --user string    用户 userId（必填）
```

### search (搜索群聊)

#### 根据关键词搜索群聊 — 分页返回匹配群聊列表

hasMore=true 时用返回的 nextCursor 作为下次 --cursor 继续翻页。

**注意：**
1. query 不要拆分得太细，应使用群名称中连续的核心词作为关键词（如群名"项目冲刺群"应搜"项目冲刺"而非拆成"项目"+"冲刺"分别搜索）。
2. 当搜索结果返回多个群聊时，应列出候选群让用户确认目标群聊，不要自行假定并直接进行后续操作。

```
Usage:
  dws chat search [flags]
Example:
  dws chat search --query "项目冲刺"
  dws chat search --query "项目冲刺" --limit 20 --cursor 0
Flags:
      --query string   搜索关键词 (必填)
      --limit int        每页返回数量（默认 20）
      --cursor string    分页游标（默认 "0"，翻页传 nextCursor）
      --exclude-muted    是否排除已设置免打扰的群聊（默认 false）
```

### data-auth (数据授权)

#### 跨组织数据授权 — 授予 chat 跨组织消息拉取权限

用于跨组织消息拉取等数据访问场景，不用于发送、撤回、群管理等操作。该命令调用 `chat_permission_grant`，固定使用数据授权类别：`scope=chat.data:cross-org`、`grantCategory=data`。

```
Usage:
  dws chat data-auth cross-org [flags]

Example:
  dws chat data-auth cross-org --target-org-id 439446171 --format json
  dws chat data-auth cross-org --target-org-id 439446171 --agentCode wukong --grant-type timed --ttl 24h --format json
  dws chat data-auth cross-org --all --grant-type timed --ttl 24h --format json

Flags:
      --target-org-id string   目标组织 ID（与 --all 二选一）
      --all                    授权所有目标组织
      --agentCode string       Agent 标识，默认 wukong
      --grant-type string      授权策略: once|session|timed|permanent (default "timed")
      --ttl string             timed 授权有效期，如 1h/4h/24h/7d (default "24h")
      --session-id string      session 授权的会话标识

注意:
  - --target-org-id 和 --all 必须且只能指定一个。
  - --grant-type=session 时必须传 --session-id。
  - --grant-type=timed 时必须传 --ttl。
```

### message (会话消息管理)

#### 拉取会话消息内容 — 拉取指定群聊或单聊的会话消息内容

--group 指定群聊，--user 指定单聊用户（通过 userId），--open-dingtalk-id 指定单聊用户（通过 openDingTalkId），三者互斥。用 --direction 控制时间方向：newer=从给定时间往现在拉，older=从给定时间往以前拉。hasMore=true 时用结果中的边界 createTime 作为下次 --time 翻页。
```
Usage:
  dws chat message list [flags]
Example:
  dws chat message list --group <openconversation_id> --time "2025-03-01 00:00:00"
  dws chat message list --user <userId> --time "2025-03-01 00:00:00" --limit 50
  dws chat message list --open-dingtalk-id <openDingTalkId> --time "2025-03-01 00:00:00" --limit 50
  dws chat message list --group <openconversation_id> --time "2025-03-01 00:00:00" --direction older
Flags:
      --direction string         时间方向: newer=从给定时间往现在拉，older=从给定时间往以前拉（推荐）
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
  - 翻页：hasMore=true 时，用结果中的边界 createTime 作为下次 --time
  - 话题圈消息拉取流程：如果返回的会话消息中包含 openConvThreadId 字段，说明是话题类消息。要获取完整的话题内容，需要两步操作：(1) 先通过 dws chat message list 拉取话题主消息（即话题帖子本身）；(2) 再调用 dws chat message list-topic-replies --group <openConversationId> --topic-id <openConvThreadId> 分页拉取该话题下的所有回复消息。只有话题主消息 + 回复列表合在一起，才是一条话题的完整内容。
```

#### 以当前用户身份发送消息 — --group 群聊 / --user 或 --open-dingtalk-id 单聊

**重要：该接口会真实发送消息到目标会话，不可用于测试或试探性调用。调用前必须确认消息内容和接收对象无误。**

--group 指定群聊 openConversationId 发群消息；--user 指定用户 userId 发单聊；--open-dingtalk-id 指定用户 openDingTalkId 发单聊。三者只能选其一，不能同时指定。纯文本/Markdown 单聊传 --user 时直接走 userId 发送能力，不需要先手动查询 openDingTalkId。推荐使用 --text flag 传递消息内容（也支持位置参数）。可选 --title 作为消息标题。
若用户只提供了数字群号而非 openConversationId，需先调用 `chat group get-by-group-id` 将群号转为 openConversationId，再传入 --group。
--群聊时可选 --at-all @所有人，或 --at-open-dingtalk-ids 指定成员（仅群聊时生效）。
--富媒体消息：通过 --msg-type 指定类型（image/file/audio/video），audio/video 是 file 的语义别名，发给服务端仍按 file 发送；必须根据文件扩展名判断 msgType 后再发送。
```
Usage:
  dws chat message send [flags] [<text>]

富媒体消息 msgType 决策（必须按此规则判断，不可跳过）:
  文件扩展名                               → msgType → 发送参数
  .jpg/.jpeg/.png/.gif/.bmp/.webp          → image       → dt_media_upload 上传 → `python scripts/extract_media_id.py <URL>` 提取 mediaId → --msg-type image --media-id
  .mp3/.wav/.m4a/.aac/.flac                → audio(file) → 本地 --file-path 或 conversation-info/drive 上传 → --msg-type audio
  .mp4/.mov/.avi/.mkv/.webm                → video(file) → 本地 --file-path 或 conversation-info/drive 上传 → --msg-type video
  其他所有（.pdf/.doc/.xls/.zip 等）         → file        → 本地 --file-path 或 conversation-info/drive 上传 → --msg-type file

Example:
  dws chat message send --group <openconversation_id> --text "hello"
  dws chat message send --user <userId> --text "请查收"
  dws chat message send --open-dingtalk-id <openDingTalkId> --text "请查收"
  dws chat message send --group <openconversation_id> "hello"
  dws chat message send --group <openconversation_id> --title "周报提醒" --text "请大家本周五前提交周报"
  # 幂等发送（24h 内相同 uuid 不重复投递）
  dws chat message send --group <openconversation_id> --text "hello" --uuid "unique-id-123"
  dws chat message send --group <openconversation_id> --at-all "<@all> 请大家注意"
  dws chat message send --group <openconversation_id> --at-open-dingtalk-ids openDingTalkId1,openDingTalkId2 "<@openDingTalkId1> <@openDingTalkId2> 请查收"
  # 发送图片
  dws chat message send --group <openconversation_id> --msg-type image --media-id <mediaId>
  # 发送文件/音频/视频（audio/video 是 file 的语义别名）
  # 先 dws chat conversation-info --group <id> 获取 spaceId（取 newCSpaceIdIM）
  # 再 dws drive upload --file <文件> --space-id <spaceId> 上传
  # 再 dws drive info --file-id <fileId> --space-id <spaceId> 获取 dentryId
  dws chat message send --group <openconversation_id> --msg-type file --dentry-id <dentryId> --space-id 24557356340 --file-name "report.pdf" --file-type "pdf" --file-path "/report.pdf" --file-size 234724
  dws chat message send --group <openconversation_id> --msg-type audio --file-path ./recording.mp3
  dws chat message send --group <openconversation_id> --msg-type video --file-path ./demo.mp4
Flags:
      --text string              消息内容（推荐使用，也可用位置参数）
      --group string             群聊 openconversation_id（群聊时必填）
      --user string              单聊接收人 userId（单聊时与 --open-dingtalk-id 二选一）
      --open-dingtalk-id string  单聊接收人 openDingTalkId（单聊时与 --user 二选一）
      --title string             消息标题（可选，默认「消息」）
      --at-all                   @所有人（仅群聊时生效，可选，默认 false）
      --at-open-dingtalk-ids string  @指定成员的 openDingTalkId 列表，逗号分隔（仅群聊时生效，可选）
      --media-id string          图片 mediaId（dt_media_upload 上传后用 `python scripts/extract_media_id.py <URL>` 提取，仅 msgType=image）
      --msg-type string          消息类型: image/file/audio/video（audio/video 是 file 别名；image 用 mediaId，file/audio/video 用钉盘上传）
      --dentry-id int64          钉盘文件 dentryId（msgType=file 时必填，通过 drive info 获取）
      --space-id int64           钉盘空间 ID（msgType=file 时必填）
      --file-name string         文件名（msgType=file 时必填）
      --file-type string         文件类型/扩展名（msgType=file 时必填）
      --file-path string         文件路径（msgType=file 时必填）
      --file-size int64          文件大小，单位字节（msgType=file 时必填）
      --uuid string             幂等 UUID，相同 uuid 在 24h 内不会重复发送（可选）
      --ai-tag                   消息是否带 AI 发送角标（可选，默认 true）

注意:
  - --text 和位置参数二选一，--text 优先
  - --group、--user、--open-dingtalk-id 三者互斥，只需指定其一：群聊用 --group，单聊用 --user 或 --open-dingtalk-id
  - 纯文本/Markdown 单聊发送时 `--user` 和 `--open-dingtalk-id` 都可用；传 `--user` 时直接走 userId 发送能力
  - --group 的别名: --id, --chat, --conversation-id (均可替代 --group)
  - --at-all 和 --at-open-dingtalk-ids 仅在 --group 群聊时生效，单聊时无效；当设置--at-all时，消息内容中一定要包含对应的占位符<@all>;当设置--at-open-dingtalk-ids openDingTalkId1,openDingTalkId2时，消息内容中一定要包含对应格式的占位符<@openDingTalkId1> <@openDingTalkId2>
  - **换行符**：消息内容按 Markdown 渲染，换行有两层要求，缺一不可：
    1. 必须使用**真实换行符**（Unicode `U+000A`），而非字面量字符串 `\n`（反斜杠 + 字母 n）。程序或大模型构造参数时，须确保已正确反转义；否则全部内容会渲染在同一行
    2. Markdown 规范下**单个换行不产生换行效果**。需要换行时请使用：段落分隔（连续两个真实换行符 `\n\n`）、行尾两个空格 + 真实换行符（硬换行 `<br>`），或直接写 HTML 的 `<br>` 标签
  - 富媒体消息类型与参数对应关系：
    - image（图片）：--msg-type image --media-id
    - audio/video：file 的语义别名，发给服务端仍是 file；可直接传本地 --file-path，或先 conversation-info 获取 spaceId → drive upload --space-id 上传 → drive info 获取 dentryId → --msg-type audio 或 --msg-type video --dentry-id --space-id --file-name --file-type --file-path --file-size
    - file（文档/压缩包等其他非图片文件）：可直接传本地 --file-path，或先 conversation-info 获取 spaceId → drive upload --space-id 上传 → drive info 获取 dentryId → --msg-type file --dentry-id --space-id --file-name --file-type --file-path --file-size
  - mediaId 通过 dt_media_upload 上传获得，必须用脚本提取：`python scripts/extract_media_id.py "<URL>"`（输出如 @lQLPxxx，直接用于 --media-id）。禁止手动从 URL 中截取或拼接 mediaId，手动解析会因 URL 格式不稳定导致尺寸后缀残留
  - --uuid 用于幂等发送，传入相同 uuid 在 24h 内不会重复投递消息（可选，群聊和单聊均支持）
  - 富媒体消息的单聊优先使用 `--open-dingtalk-id`；传 `--user` 时 CLI 会尝试解析成 openDingTalkId 后发送
  - 发送文件/媒体消息时，必须先根据文件扩展名判断 msgType：图片(.jpg/.png/.gif/.bmp/.webp)→image，音频(.mp3/.wav/.m4a/.aac/.flac)→audio，视频(.mp4/.mov/.avi/.mkv/.webm)→video，其他所有→file；不可跳过此判断步骤。audio/video 是 file 别名，不代表不支持音频/视频
  - 20MB 降级：图片超过 20MB 时 dt_media_upload 会失败，必须降级走钉盘上传 + Markdown 嵌入方式发送（参见「发送图片+文字消息」章节）。音频/视频/文件走钉盘上传无 20MB 限制
  - 发送文字 + 文件混合消息时的完整流程：除了将文件以 Markdown 链接内嵌到文字消息中发送一条 md 消息外，还必须额外逐个发送独立的文件消息（--msg-type file），确保接收方可以直接下载原始文件。即：先发一条包含文字和文件链接的 md 消息，再对每个涉及的文件各发一条 --msg-type file 的文件消息
```

### media (上传媒体获取 mediaId)

#### 上传图片/媒体获取 mediaId — 用于 chat message send --msg-type image 等

⚠️ 前置条件：本命令需要应用凭证。必须已通过 `dws auth login --client-id <APP_KEY> --client-secret <APP_SECRET>` 登录，或设置环境变量 `DWS_CLIENT_ID` / `DWS_CLIENT_SECRET`；否则报"缺少应用凭证"。这与其他 chat 命令走用户登录态不同。
```
Usage:
  dws chat media upload [flags]
Example:
  dws chat media upload --file ./screenshot.png
  dws chat media upload --file ./photo.jpg --type image
Flags:
      --file string   本地文件路径 (必填)
      --type string   媒体类型: image/voice/video/file（默认 image）

注意:
  - 返回的 mediaId 可直接用于 chat message send --msg-type image --media-id
  - 发图片+文字时，agent 侧一般用独立的 dt_media_upload 工具；本命令是 dws 内置的等价上传入口
```

### file (会话文件上传，已下线)

#### chat file upload 已下线

`dws chat file upload` 已隐藏下线，不再调用 `chat/upload_conversation_file_by_url`，URL 文件服务端代传当前不可用。

发送本地文件消息仍然支持，统一使用：
```
dws chat message send --group <openConversationId> --msg-type file --file-path ./report.pdf --format json
dws chat message send --open-dingtalk-id <openDingTalkId> --msg-type file --file-path ./report.pdf --format json
```

注意:
  - 不要再构造 `dws chat file upload --url ...`，该路径会直接返回下线提示
  - 常规发图/发文件/发音视频都走 `chat message send --msg-type file --file-path <本地路径>`
  - 若需要“文字 + 文件”双消息，先发送文件消息，再补一条文本或 Markdown 消息说明

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
  - 返回结果中含发送成功消息的 openMessageId，可用于后续 recall（撤回）、read-status（查已读）等命令
```

#### 撤回消息 — 撤回当前用户自己发出的消息

撤回当前用户以个人身份发送的消息。需要指定会话 ID（openConversationId）和消息 ID（openMessageId）。与 `recall-by-bot` 的区别：本命令通过 IM 接口撤回用户自己发出的消息，`recall-by-bot` 通过机器人接口撤回机器人发出的消息（需要 robot-code + processQueryKey）。
```
Usage:
  dws chat message recall [flags]
Example:
  dws chat message recall --conversation-id <openConversationId> --msg-id <openMessageId>
  # 查询会话 ID: dws chat search --query "群名"
  # 消息 ID 可通过 dws chat message list 获取
Flags:
      --conversation-id string   会话 openConversationId (必填，支持单聊/群聊，别名: --group / --id / --chat)
      --msg-id string            消息 openMessageId (必填)

注意:
  - --conversation-id 的别名: --group, --id, --chat (均可替代 --conversation-id)
  - 消息 ID 可通过 `dws chat message list` 命令获取
  - 仅支持撤回当前用户以个人身份发出的消息，不能撤回他人发送的消息，也不能撤回机器人发出的消息
  - 与 `recall-by-bot` 的区别：本命令通过 IM 接口撤回用户自己发出的消息（需要 openConversationId + openMessageId），`recall-by-bot` 通过机器人接口撤回机器人发出的消息（需要 robot-code + processQueryKey）
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
Flags:
      --group string      群会话 openconversationId (必填)
      --topic-id string   话题 ID，由 dws chat message list 返回 (必填)
      --time string       开始时间，格式: yyyy-MM-dd HH:mm:ss（可选）
      --limit int         返回数量（默认 50）
      --direction string  时间方向: newer=从给定时间往现在拉，older=从给定时间往以前拉（推荐，默认 older）
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
      --sender-open-dingtalk-id string        发送者 openDingTalkId（与 --sender-user-id 二选一）；该路径当前返回 result:{}、success:null（疑似服务端问题），优先用 --sender-user-id
      --start string                          开始时间，ISO-8601 格式 (必填)
      --end string                            结束时间，ISO-8601 格式 (必填)
      --limit int                             每页返回数量（默认 50）
      --cursor string                         分页游标（默认 "0"，翻页传 nextCursor）

注意:
  - --sender-user-id 和 --sender-open-dingtalk-id 二者互斥，必须且只能指定其一：
    - --sender-user-id 传 userId（企业内部应用常用，当前唯一可用路径）
    - --sender-open-dingtalk-id 传 openDingTalkId（当前返回 result:{}、success:null（疑似服务端问题）；即使只有 openDingTalkId，也建议先用 contact 换取 userId 后走 --sender-user-id）
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
  dws chat message list-unread-conversations --exclude-muted
Flags:
      --count int       返回未读会话条数（可选）
      --exclude-muted   是否排除已设置免打扰的会话（默认 false）
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

#### 多维度搜索消息（推荐首选） — 支持按关键词、发送者、@我、@指定人、指定会话、时间范围等多维度搜索

> 推荐：这是消息搜索的首选接口。它可以完全替代 `chat message search`（query 可选 vs 必填，支持多个会话 vs 单个），大部分替代 `chat message list-by-sender`（通过 --user/--users 按 userId 搜索发送者，或通过 --sender-ids 按 openDingTalkId 搜索）和 `chat message list-mentions`（通过 --at-me 搜索@我的消息）。仅在拉取「特别关注人」消息时需要退回 `list-focused`。

支持按关键词、发送者、@我、@指定人、指定会话、时间范围等多维度搜索消息。发送者 userId 使用 --user/--users；发送者或 @ 人的 openDingTalkId 使用 --sender-ids/--at-ids。所有参数均为可选，至少指定一个搜索条件。
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
  # 查询人员: dws aisearch person --keyword "姓名" --dimension name
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

### list-top-conversations (置顶会话)

#### 拉取置顶会话列表

拉取当前用户的置顶会话列表。分页参数 --limit 指定每页数量，--cursor 传分页游标（首次不传或传 0）。返回结果中 hasMore=true 时用 nextCursor 作为下次 --cursor 继续翻页。
```
Usage:
  dws chat list-top-conversations [flags]
Example:
  dws chat list-top-conversations --limit 1000
  dws chat list-top-conversations --limit 1000 --cursor <nextCursor>
Flags:
      --limit int        每页返回数量（默认 1000）
      --cursor int64     分页游标（首次不传或传 0，翻页传 nextCursor）
      --exclude-muted    是否排除已设置免打扰的会话（默认 false）

注意:
  - 用户询问"置顶会话"时，直接调用此命令返回置顶会话列表即可
  - 用户询问"置顶消息"时，需两步：先调用此命令拉取置顶会话列表获取各会话的 openConversationId，再用 `chat message list --group <openConversationId>` 分别拉取每个会话内的消息
  - 翻页：hasMore=true 时，用返回的 nextCursor 作为下次 --cursor
```

### download-media (下载消息资源)

#### 下载消息中的资源（图片/视频/语音等）到本地

下载聊天消息中的图片、视频、语音等资源到本地文件。流程：先获取下载 URL，再 HTTP GET 下载。
```
Usage:
  dws chat message download-media [flags]
Example:
  dws chat message download-media --type mediaId --resource-id <mediaId> --message-id <openMessageId> --open-conversation-id <openConversationId> --output ./photo.jpg
  dws chat message download-media --type mediaId --resource-id <mediaId> --message-id <openMessageId> --open-conversation-id <openConversationId> --output ./downloads/photo.jpg
Flags:
      --type string                  资源类型: mediaId (必填)
      --resource-id string           资源 ID，mediaId 类型时为消息中的 mediaId 值 (必填)
      --message-id string            消息 openMessageId (必填)
      --open-conversation-id string  会话 openConversationId (必填)
      --output string                本地保存路径 (必填)，建议传完整文件路径（含文件名）

注意:
  - resource-id 从 `dws chat message list` 返回的消息内容中获取 mediaId
  - message-id 从 `dws chat message list` 返回的 openMessageId
  - open-conversation-id 从 `dws chat search` 获取 openConversationId
  - --output 建议传完整文件路径（含扩展名，如 ./photo.jpg）；只传目录（如 ./downloads/）在旧版本会下载失败，请显式带上文件名
```

### search-common (搜索共同群)

#### 搜索共同群 — 查询指定人共同所在的群聊

根据昵称列表搜索共同群聊。--nicks 指定要搜索的人员昵称（逗号分隔，必填）。--match-mode 控制匹配模式：AND 表示所有人都在群里，OR 表示任一人在群里（默认 AND）。分页参数 --limit（默认 20）和 --cursor（默认 "0"）始终传递；hasMore=true 时用返回的 nextCursor 作为下次 --cursor 继续翻页。
```
Usage:
  dws chat search-common [flags]
Example:
  dws chat search-common --nicks "风雷,山乔" --limit 20 --cursor 0
  dws chat search-common --nicks "天鸡,乐函" --match-mode OR --limit 20 --cursor 0
  dws chat search-common --nicks "风雷,山乔,天鸡" --limit 10 --cursor <nextCursor>
Flags:
      --nicks string        要搜索的昵称列表，逗号分隔 (必填)
      --match-mode string   匹配模式：AND=所有人都在群里，OR=任一人在群里（默认 AND）
      --limit int           每页返回数量（默认 20）
      --cursor string       分页游标（默认 "0"，翻页传 nextCursor）
      --exclude-muted       是否排除已设置免打扰的群聊（默认 false）

注意:
  - --nicks 传人员昵称（花名），逗号分隔，如 "风雷,山乔"
  - --match-mode AND 表示群里必须包含所有指定的人；OR 表示包含任意一人即可
  - 翻页：hasMore=true 时，用返回的 nextCursor 作为下次 --cursor
```

### conversation-info (获取会话基础信息)

#### 获取会话基础信息 — 含会话关联的钉盘共享空间 ID

获取指定会话的基础信息，包含会话关联的钉盘共享空间 ID (newCSpaceIdIM)。发送文件消息前需先调用此命令获取 spaceId，再用 drive upload --space-id 上传文件到共享空间。
```
Usage:
  dws chat conversation-info [flags]
Example:
  dws chat conversation-info --group <openConversationId> --format json
  dws chat conversation-info --user <userId> --format json
  dws chat conversation-info --open-dingtalk-id <openDingTalkId> --format json
Flags:
      --group string              群聊 openConversationId（群聊时使用）
      --user string               单聊对方 userId（单聊时使用）
      --open-dingtalk-id string   单聊对方 openDingTalkId（单聊时使用）

注意:
  - --group、--user、--open-dingtalk-id 互斥，必须且只能指定其一
  - --group 的别名: --id, --chat, --conversation-id (均可替代 --group)
  - 返回值中的 newCSpaceIdIM 为会话共享空间 ID，用于 drive upload --space-id 参数
  - 上传到共享空间的文件对方才能打开，上传到个人空间的文件对方无法访问
```

#### 引用回复消息 — 引用某条消息并回复文字（单聊/群聊均可）
```
Usage:
  dws chat message reply [flags]
Example:
  dws chat message reply --conversation-id <openConversationId> --ref-msg-id <openMessageId> --ref-sender <openDingTalkId> --text "收到，马上处理"
  # 被引用消息的 openMessageId、发送者 openDingTalkId 通过 dws chat message list 获取
Flags:
      --conversation-id string   会话 openConversationId (必填，支持单聊/群聊)
      --ref-msg-id string        被引用的消息 openMessageId (必填)
      --ref-sender string        被引用消息的发送者 openDingTalkId (必填)
      --text string              回复内容 (必填)
      --ai-tag                   消息是否带 AI 发送角标（可选，默认 true）
      --uuid string              幂等键（可选）

注意:
  - 以当前用户身份引用回复，语义同 chat message send；目前回复类型仅支持 text
```

#### 转发单条消息 — 将一条消息从源会话转发到目标会话（源/目标均支持单聊/群聊）
```
Usage:
  dws chat message forward [flags]
Example:
  dws chat message forward --src-conversation-id <srcOpenCid> --msg-id <openMessageId> --dest-conversation-id <destOpenCid>
Flags:
      --src-conversation-id string    源会话 openConversationId (必填)
      --msg-id string                 源消息 openMessageId (必填)
      --dest-conversation-id string   目标会话 openConversationId (必填)
      --uuid string                   幂等键（可选）

注意:
  - 与 combine-forward 区别: forward 转单条，combine-forward 合并多条为一条转发
```

#### 转发话题消息 — 将一条话题消息转发到目标会话
```
Usage:
  dws chat message forward-topic [flags]
Example:
  dws chat message forward-topic --src-conversation-id <srcOpenCid> --src-msg-id <openMessageId> --src-thread-id <convThreadId> --dest-conversation-id <destOpenCid>
Flags:
      --src-conversation-id string    源会话 openConversationId (必填，消息所在的会话)
      --src-msg-id string             源消息 openMessageId (必填，要转发的消息)
      --src-thread-id string          话题 ID (必填，格式: convThread + 加密后的 convThreadId，即 message list 返回的 openConvThreadId)
      --dest-conversation-id string   目标会话 openConversationId (必填，转发到的会话)
```

#### 置顶消息 — 将指定消息置顶到会话顶部
```
Usage:
  dws chat message set-top-msg [flags]
Example:
  dws chat message set-top-msg --open-conversation-id <openConversationId> --msg-id <openMessageId>
Flags:
      --open-conversation-id string   会话 openConversationId (必填，支持群聊/单聊)
      --msg-id string                 消息 openMessageId (必填)
```

#### 取消置顶消息 — 取消会话顶部的置顶消息
```
Usage:
  dws chat message unset-top-msg [flags]
Example:
  dws chat message unset-top-msg --open-conversation-id <openConversationId> --msg-id <openMessageId>
Flags:
      --open-conversation-id string   会话 openConversationId (必填，支持群聊/单聊)
      --msg-id string                 消息 openMessageId (必填)
```

#### 合并转发多条消息 — 将多条消息合并后转发到目标会话（源/目标会话均支持单聊/群聊）
```
Usage:
  dws chat message combine-forward [flags]
Example:
  dws chat message combine-forward --src-conversation-id <srcOpenCid> --msg-ids <id1>,<id2>,<id3> --dest-conversation-id <destOpenCid>
  dws chat message combine-forward --src-conversation-id <srcOpenCid> --msg-ids <id1>,<id2> --dest-conversation-id <destOpenCid> --uuid <idempotencyKey>
Flags:
      --src-conversation-id string    源会话 openConversationId (必填)
      --msg-ids string                源消息 openMessageId 列表，逗号分隔 (必填)
      --dest-conversation-id string   目标会话 openConversationId (必填)
      --uuid string                   幂等键（可选）

注意:
  - 与 chat message forward 区别: forward 转单条，combine-forward 合并多条为一条转发
  - --msg-ids 多个消息 ID 用逗号分隔，无顺序要求
```

#### 钉住某条消息（Pin） — 将指定消息设置为钉住状态
```
Usage:
  dws chat message set-pin-msg [flags]
Example:
  dws chat message set-pin-msg --open-conversation-id <openConversationId> --msg-id <openMessageId>
Flags:
      --open-conversation-id string    (必填)会话 openConversationId（支持群聊/单聊）
      --msg-id string                  (必填)消息 openMessageId

注意:
  - 钉住消息后，会话成员均可在会话中看到被钉住的消息
```

#### 取消钉住某条消息（Unpin） — 取消指定消息的钉住状态
```
Usage:
  dws chat message unset-pin-msg [flags]
Example:
  dws chat message unset-pin-msg --open-conversation-id <openConversationId> --msg-id <openMessageId>
Flags:
      --open-conversation-id string    (必填)会话 openConversationId（支持群聊/单聊）
      --msg-id string                  (必填)消息 openMessageId

注意:
  - 取消钉住后消息仍保留在会话中，只是不再被标记为钉住状态
```

#### 拉取某个会话中钉住的消息列表 — 拉取指定会话中被钉住的消息列表
```
Usage:
  dws chat message list-pin-msg [flags]
Example:
  dws chat message list-pin-msg --open-conversation-id <openConversationId>
  dws chat message list-pin-msg --open-conversation-id <openConversationId> --size 50
  dws chat message list-pin-msg --open-conversation-id <openConversationId> --cursor <nextCursor> --size 20
Flags:
      --open-conversation-id string    (必填)会话 openConversationId（支持群聊/单聊）
      --cursor string   (选填)分页游标，首次不传，翻页时传上次返回的 nextCursor
      --size int        (选填)一次拉取的消息数量（默认 20，最大 100）

注意:
  - 与 `chat message list` 区别: list-pin-msg 只返回被钉住的消息；list 拉取全部消息
  - 分页: hasMore=true 时，用返回的 nextCursor 作为下次 --cursor 继续翻页
```

### bot (机器人管理)

#### 搜索【我创建的】机器人 — 仅返回当前用户自己创建的机器人

范围: 仅限当前登录用户自己创建的机器人（不含他人创建、官方机器人）。
返回字段: 没有 openDingTalkId，如果需要给机器人发单聊消息请用 find。
典型触发词: "我创建的机器人""我的机器人""我自己的机器人""我做的机器人""查看我的机器人"。

```
Usage:
  dws chat bot search [flags]
Example:
  dws chat bot search --page 1
  dws chat bot search --page 1 --size 10 --name "日报"
Flags:
      --name string   按名称搜索
      --page int      页码，从1开始 (默认 1)
      --size int      每页条数 (默认 50)，别名: --limit
```

#### 搜索【全部可用】机器人 — 含他人创建/官方机器人，额外返回机器人 openDingTalkId

范围: 当前用户可用的全部机器人（含他人创建、官方机器人）。
返回字段: 结果在 `result.bots[]` 中，每项含 `botOpenDingTalkId`（机器人的 openDingTalkId，用于给机器人发单聊消息）和 `name`。注意字段名是 `botOpenDingTalkId`，不是 `openDingTalkId`；search 没有此字段。
典型触发词: "搜索机器人""找一个机器人""帮我找 XXX 机器人""所有可用机器人""查机器人"。

```
Usage:
  dws chat bot find [flags]
Example:
  dws chat bot find --query "日报"
  dws chat bot find --query "日报" --limit 20
  dws chat bot find --query "日报" --limit 20 --cursor <上次返回的 nextCursor>
Flags:
      --query string   搜索关键词 (必填)
      --limit int        每页返回数量（默认 20）
      --cursor string    分页游标（首次调用不传，翻页时传上次返回的 nextCursor）

注意:
  - cursor 必须用上次返回的 nextCursor 字符串原值，不要传 "0" 或其他数字字面量
    （服务端 String 类型，但网关会把数字字符串 auto-coerce 回 Integer 导致 PARAM_ERROR）
```

search 与 find 选择指南:

| 维度 | `chat bot search` | `chat bot find` |
|------|-------------------|-----------------|
| 范围 | 仅我创建的机器人 | 全部可用机器人（含他人/官方） |
| 额外返回机器人 openDingTalkId | 无 | 有，字段名 `botOpenDingTalkId`（可用于给机器人发单聊消息） |
| 触发词 | "我创建的""我的""我自己的" | "搜索机器人""找机器人""查机器人" |

### category (会话分组管理)

#### 获取用户自定义会话分组
```
Usage:
  dws chat category list
Example:
  dws chat category list
  # 返回当前用户的所有自定义会话分组
```

#### 拉取指定分组下的会话列表
```
Usage:
  dws chat category list-conversations [flags]
Example:
  dws chat category list-conversations --category-id <分组ID>
  dws chat category list-conversations --category-id <分组ID> --exclude-muted
  # 分组ID 可通过 dws chat category list 获取
Flags:
      --category-id int   会话分组 ID (必填)
      --exclude-muted     是否排除已设置免打扰的会话（默认 false）
```

#### 创建会话分组
```
Usage:
  dws chat category create [flags]
Example:
  dws chat category create --title "工作群"
Flags:
      --title string   分组名称 (必填)
```

#### 删除会话分组
```
Usage:
  dws chat category delete [flags]
Example:
  dws chat category delete --category-id <分组ID>
  # 分组ID 可通过 dws chat category list 获取
Flags:
      --category-id int   会话分组 ID (必填)
```

#### 重命名会话分组
```
Usage:
  dws chat category rename [flags]
Example:
  dws chat category rename --category-id <分组ID> --title "新名称"
  # 分组ID 可通过 dws chat category list 获取
Flags:
      --category-id int   会话分组 ID (必填)
      --title string      新的分组名称 (必填)
```

#### 将会话加入分组
```
Usage:
  dws chat category add-conv [flags]
Example:
  dws chat category add-conv --group <openConversationId> --category-ids 123,456
  # 分组ID 可通过 dws chat category list 获取
  # 查询群 ID: dws chat search --query "群名"
Flags:
      --group string          会话 openConversationId (必填)
      --category-ids string   目标分组 ID 列表，逗号分隔 (必填)
```

#### 将会话移出分组
```
Usage:
  dws chat category remove-conv [flags]
Example:
  dws chat category remove-conv --group <openConversationId> --category-ids 123,456
  # 分组ID 可通过 dws chat category list 获取
  # 查询群 ID: dws chat search --query "群名"
Flags:
      --group string          会话 openConversationId (必填)
      --category-ids string   目标分组 ID 列表，逗号分隔 (必填)
```

### mute (会话免打扰)

#### 会话消息免打扰 — 开启或关闭会话消息免打扰（支持单聊和群聊）
```
Usage:
  dws chat mute [flags]
Example:
  dws chat mute --conversation-id <openConversationId>
  dws chat mute --conversation-id <openConversationId> --off
  # 查询群 ID: dws chat search --query "群名"
  # 查询单聊会话 ID: dws chat conversation-info --user <userId>
Flags:
      --conversation-id string   会话 openConversationId (必填，支持单聊/群聊)
      --id string                --conversation-id 的别名
      --chat string              --conversation-id 的别名
      --off                      关闭免打扰（不传则开启免打扰）

注意:
  - 默认行为是开启免打扰，传 --off 则关闭免打扰
  - 支持单聊和群聊，openConversationId 可通过 chat search（群聊）或 chat conversation-info（单聊）获取
```

### hide (隐藏会话)

#### 隐藏会话 — 在会话列表中隐藏指定会话（支持单聊/群聊），收到新消息时会重新出现
```
Usage:
  dws chat hide [flags]
Example:
  dws chat hide --conversation-id <openConversationId>
  dws chat hide --id <openConversationId>
  # 查询群 ID: dws chat search --query "群名"
  # 查询单聊会话 ID: dws chat conversation-info --user <userId>
Flags:
      --conversation-id string   会话 openConversationId (必填，支持单聊/群聊)
      --id string                --conversation-id 的别名
      --chat string              --conversation-id 的别名

注意:
  - 隐藏后会话不再显示在列表中，收到新消息时会重新出现
  - 支持单聊和群聊，openConversationId 可通过 chat search（群聊）或 chat conversation-info（单聊）获取
```

### mute-at-all (关闭@所有人通知)

#### 关闭/开启 @所有人消息提醒 — 关闭或开启会话中 @所有人的消息通知

> ⚠️ 当前不可用：该命令当前调用常失败（服务端返回 1002 系统繁忙），多为服务端侧限制。命令本身参数合法，但调用不会生效。
```
Usage:
  dws chat mute-at-all [flags]
Example:
  dws chat mute-at-all --conversation-id <openConversationId>
  dws chat mute-at-all --conversation-id <openConversationId> --off
  # 查询群 ID: dws chat search --query "群名"
Flags:
      --conversation-id string   会话 openConversationId (必填，支持单聊/群聊)
      --id string                --conversation-id 的别名
      --chat string              --conversation-id 的别名
      --off                      恢复接收 @所有人通知（不传则关闭通知）

注意:
  - 默认行为是关闭 @所有人通知，传 --off 则恢复接收通知
  - 支持单聊和群聊，openConversationId 可通过 chat search（群聊）或 chat conversation-info（单聊）获取
```

### mute-red-envelope (关闭红包通知)

#### 关闭/开启红包消息提醒 — 关闭或开启会话中的红包消息通知

> ⚠️ 当前不可用：该命令当前调用常失败（服务端返回 1002 系统繁忙），多为服务端侧限制。命令本身参数合法，但调用不会生效。
```
Usage:
  dws chat mute-red-envelope [flags]
Example:
  dws chat mute-red-envelope --conversation-id <openConversationId>
  dws chat mute-red-envelope --conversation-id <openConversationId> --off
  # 查询群 ID: dws chat search --query "群名"
Flags:
      --conversation-id string   会话 openConversationId (必填，支持单聊/群聊)
      --id string                --conversation-id 的别名
      --chat string              --conversation-id 的别名
      --off                      恢复接收红包通知（不传则关闭通知）

注意:
  - 默认行为是关闭红包通知，传 --off 则恢复接收通知
  - 支持单聊和群聊，openConversationId 可通过 chat search（群聊）或 chat conversation-info（单聊）获取
```

### mark-unread (标记会话为未读)

#### 标记会话为未读 — 将指定会话标记为未读状态
```
Usage:
  dws chat mark-unread [flags]
Example:
  dws chat mark-unread --conversation-id <openConversationId>
  dws chat mark-unread --id <openConversationId>
Flags:
      --conversation-id string   会话 openConversationId (必填，支持群聊/单聊)
      --id string                --conversation-id 的别名
      --chat string              --conversation-id 的别名

注意:
  - 支持群聊和单聊，openConversationId 可通过 chat search（群聊）或 chat conversation-info（单聊）获取
  - API 返回成功，但该未读状态在 API 侧观察不到：`list-all-conversations` 返回的 unreadPoint 不会随之变化，只有钉钉客户端 UI 上能看到未读标记
```

### clear-red-point (清除会话红点)

#### 清除会话红点 — 清除指定会话的未读红点
```
Usage:
  dws chat clear-red-point [flags]
Example:
  dws chat clear-red-point --conversation-id <openConversationId>
  dws chat clear-red-point --id <openConversationId>
Flags:
      --conversation-id string   会话 openConversationId (必填，支持群聊/单聊)
      --id string                --conversation-id 的别名
      --chat string              --conversation-id 的别名

注意:
  - 支持群聊和单聊，openConversationId 可通过 chat search（群聊）或 chat conversation-info（单聊）获取
  - 清除红点后该会话不再显示未读标记
```

### clear-all-red-point (红点清零)

#### 清除所有会话红点 — 一键全部已读
```
Usage:
  dws chat clear-all-red-point
Example:
  dws chat clear-all-red-point

注意:
  - 无需任何参数，直接清除当前用户所有会话的未读红点
  - 等效于"全部已读"操作
```

### list-all-conversations (全部会话列表)

#### 分页获取全部会话列表 — 获取当前用户的所有会话
```
Usage:
  dws chat list-all-conversations [flags]
Example:
  dws chat list-all-conversations
  dws chat list-all-conversations --limit 50
  dws chat list-all-conversations --limit 100 --cursor <nextCursor>
  dws chat list-all-conversations --exclude-muted
Flags:
      --limit int        每页数量（1-100，默认 100）；传 >100 会被明确拒绝
      --cursor int       分页游标（首次不传或传 0，翻页传 nextCursor）
      --exclude-muted    是否排除已免打扰会话（默认 false）

注意:
  - 返回结果包含单聊和群聊，不区分会话类型
  - --limit 范围 1-100，默认 100，上限 100；传入 >100 会报错拒绝，不会静默截断
  - 分页当前不可用：当前 hasMore 恒为 false、nextCursor 恒为 null，本命令最多返回 100 条会话，无法用 --cursor 继续翻页取更多
  - 与 list-top-conversations 的区别: 本命令返回全部会话（单聊+群聊），list-top-conversations 仅返回置顶会话
```

### clear-messages (清空会话聊天记录)

#### 清空会话聊天记录 — 清空当前用户指定会话的消息
```
Usage:
  dws chat clear-messages [flags]
Example:
  dws chat clear-messages --conversation-id <openConversationId>
  dws chat clear-messages --id <openConversationId>
Flags:
      --conversation-id string   会话 openConversationId (必填，支持群聊/单聊)
      --id string                --conversation-id 的别名
      --chat string              --conversation-id 的别名

注意:
  - 仅清空当前用户视角的消息，不影响其他成员
  - openConversationId 可通过 chat search（群聊）或 chat conversation-info（单聊）获取
```

### mark-read (标记消息已读)

#### 标记消息已读 — 将指定消息及之前的消息标记为已读
```
Usage:
  dws chat mark-read [flags]
Example:
  dws chat mark-read --conversation-id <openConversationId> --message-id <openMessageId>
  dws chat mark-read --id <openConversationId> --message-id <openMessageId>
Flags:
      --conversation-id string   会话 openConversationId (必填，支持群聊/单聊)
      --id string                --conversation-id 的别名
      --chat string              --conversation-id 的别名
      --message-id string        消息 openMessageId (必填)

注意:
  - 标记该消息及之前的所有消息为已读
  - openConversationId 可通过 chat search（群聊）或 chat conversation-info（单聊）获取
  - openMessageId 可通过 chat message list 获取
```

### group list-all (分页拉取所有群)

#### 分页拉取我所有群列表 — 获取当前用户加入的所有群聊
```
Usage:
  dws chat group list-all [flags]
Example:
  dws chat group list-all
  dws chat group list-all --limit 50
  dws chat group list-all --limit 100 --cursor <nextCursor>
Flags:
      --limit int       每页返回数量（默认 100，最大 200）
      --cursor string   分页游标（首次不传，翻页传返回的 nextCursor）

注意:
  - 与 `chat group list-my-groups` 区别: list-all 返回用户加入的所有群；list-my-groups 仅返回用户作为群主/管理员的群
  - 分页: hasMore=true 时用返回的 nextCursor 作为下次 --cursor
  - ⚠️ 存在同步盲区：新建群后较长时间（实测 15 分钟后全量翻页仍查不到）内不会出现在 list-all 里；而 `chat group list-my-groups` / `chat search` 能立即查到。要确认刚建的群，用 list-my-groups 或 search，别依赖 list-all
```

### group list-join-validations (分页拉取入群验证记录)

#### 分页拉取入群验证记录 — 获取当前用户的所有入群验证记录

包括自己被拒绝的记录以及作为审批者的记录。

```
Usage:
  dws chat group list-join-validations [flags]
Example:
  dws chat group list-join-validations
  dws chat group list-join-validations --limit 30
  dws chat group list-join-validations --limit 20 --cursor <nextCursor>
Flags:
      --limit int       单页数量（默认 20，最大 50）
      --cursor string   分页游标（首次不传，翻页传返回的 nextCursor）

注意:
  - 分页: hasMore=true 时用返回的 nextCursor 作为下次 --cursor
  - cursor 首次拉取不传或传 null 时从当前时间开始拉
```

### group audit-join-validation (审批入群验证)

#### 审批入群验证 — 通过、删除单个审核

当前仅 AuditApprove（通过）和 AuditDelete（删除）两个动作可用。

```
Usage:
  dws chat group audit-join-validation [flags]
Example:
  dws chat group audit-join-validation --group <openConversationId> --record-id 123456 --applicant <openDingTalkId> --inviter <openDingTalkId> --status AuditApprove
  dws chat group audit-join-validation --group <openConversationId> --record-id 123456 --applicant <openDingTalkId> --inviter <openDingTalkId> --status AuditDelete
  # 查询入群验证记录: dws chat group list-join-validations
Flags:
      --group string        群 openConversationId (必填)
      --record-id string    申请记录 ID (必填)
      --applicant string    申请人 openDingTalkId (必填)
      --inviter string      邀请人 openDingTalkId (必填)
      --status string       审批动作: AuditApprove(通过) / AuditDelete(删除) (必填)
      --description string  审批说明（可选）

注意:
  - status 仅支持 AuditApprove(通过) 和 AuditDelete(删除)；AuditIgnore(忽略)、AuditRefuse(拒绝)、AuditBlock(拒绝且拉黑) 会被服务端拒绝报 unsupported audit status，属服务端限制
  - record-id、applicant、inviter 可通过 dws chat group list-join-validations 查询获得
```

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
用户说"置顶会话/置顶消息" → `chat list-top-conversations` 列会话 → 再 `chat message list --group <id>` 拉消息（两步）

用户说"建群/创建群聊" → `chat group create`
用户说"搜索群/找群" → `chat search`
用户说"我创建的群/我管理的群/我是群主的群/我当管理员的群" → `chat group list-my-groups`
用户说"群成员/看群里有谁" → `chat group members`
用户说"拉人进群/加群成员" → `chat group members add`
用户说"踢人/移除群成员" → `chat group members remove`
用户说"加机器人到群" → `chat group members add-bot`
用户说"改群名" → `chat group rename`
用户说"设置群备注/给群加备注" → `chat group update-alias`
用户说"改我在群里的昵称/设置群昵称" → `chat group update-nick`
用户说"批量查群成员信息/按ID查群成员" → `chat group members list-by-ids`
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
用户说"置顶会话/置顶消息/我的置顶/查看置顶" → `chat list-top-conversations`
用户说"查看会话分组/自定义分组" → `chat category list`
用户说"某个分组下的会话/分组会话列表" → `chat category list-conversations`
用户说"新建会话分组/创建分组" → `chat category create`
用户说"删除会话分组" → `chat category delete`
用户说"重命名分组/改分组名" → `chat category rename`
用户说"把会话加入分组/会话归类到分组" → `chat category add-conv`
用户说"把会话移出分组/从分组移除会话" → `chat category remove-conv`
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
用户说"批量查消息/按ID查消息/根据消息ID查" → `chat message list-by-ids`
用户说"emoji回应/表情回应/给消息加表情" → `chat message add-emoji`
用户说"取消emoji回应/移除表情回应" → `chat message remove-emoji`
用户说"文字表情回应/添加文字表情" → `chat message add-text-emotion`
用户说"取消文字表情回应/移除文字表情" → `chat message remove-text-emotion`
用户说"创建文字表情/新建文字表情" → `chat message create-text-emotion`
用户说"免打扰/消息免打扰/静音/开启免打扰/关闭免打扰" → `chat mute`
用户说"隐藏会话/隐藏群聊/隐藏对话" → `chat hide`
用户说"关闭@所有人通知/屏蔽@所有人/不接收@all" → `chat mute-at-all`
用户说"开启@所有人通知/恢复@所有人提醒" → `chat mute-at-all --off`
用户说"关闭红包通知/屏蔽红包/不接收红包提醒" → `chat mute-red-envelope`
用户说"开启红包通知/恢复红包提醒" → `chat mute-red-envelope --off`
用户说"标记会话未读/标为未读" → `chat mark-unread`
用户说"标记已读/把消息标成已读" → `chat mark-read`
用户说"清除红点/去掉某个会话的未读红点" → `chat clear-red-point`
用户说"全部已读/一键清除红点/红点清零" → `chat clear-all-red-point`
用户说"我的所有会话/全部会话列表" → `chat list-all-conversations`
用户说"清空聊天记录/清空会话消息" → `chat clear-messages`
用户说"我加入的所有群/我的全部群列表" → `chat group list-all`
用户说"入群验证记录/谁申请进群" → `chat group list-join-validations`
用户说"审批入群/通过入群申请/拒绝入群申请" → `chat group audit-join-validation`
用户说"引用回复/回复消息/引用消息回复" → `chat message reply`
用户说"转发消息/转发一条消息/把消息转发到另一个群" → `chat message forward`
用户说"合并转发/批量转发/合并转发多条消息" → `chat message combine-forward`
用户说"转发话题/转发话题消息" → `chat message forward-topic`
用户说"置顶消息/把消息置顶" → `chat message set-top-msg`
用户说"取消置顶消息/撤销消息置顶" → `chat message unset-top-msg`
用户说"上传图片拿mediaId/上传媒体" → `chat media upload`
用户说"群机器人列表/群里有哪些机器人/查看群机器人" → `chat group bots`
用户说"从群里移除机器人/踢出机器人" → `chat group members remove-bot`
用户说"搜索机器人/找机器人/查机器人/帮我找XXX机器人" → `chat bot find`（全部可用机器人，额外返回 botOpenDingTalkId 可发单聊）
用户说"给机器人发单聊/给机器人发消息/跟机器人聊天" → 必须先 `chat bot find`（拿 openDingTalkId）→ 再 `chat message send --open-dingtalk-id`（search 没有 openDingTalkId，无法发单聊）
用户说"我创建的机器人/我的机器人/我自己的机器人/查看我的机器人" → `chat bot search`（仅我创建的机器人，无 openDingTalkId）
用户说"解散群/解散群聊" → `chat group dismiss`
用户说"设置历史消息/新成员看历史/新成员可见消息" → `chat group set-history`
用户说"置顶会话/取消置顶/会话置顶" → `chat set-top`（设置/取消置顶），`chat list-top-conversations`（查看置顶列表）
用户说"全员禁言/群禁言/解除禁言" → `chat group-mute`
用户说"禁言某人/指定成员禁言/解除某人禁言" → `chat group-mute-member`
用户说"设管理员/取消管理员/设置群管理员" → `chat group set-admin`

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
- `chat list-top-conversations` — 拉取置顶会话列表（用户询问"置顶会话"或"置顶消息"时路由到此），cursor 分页
- `chat message send` — 以当前用户身份发消息（群聊或单聊），text 为位置参数；支持 --msg-type 发送富媒体消息：image（图片）、file/audio/video（audio/video 是 file 别名），图片的 mediaId 通过 dt_media_upload 上传获得，其他文件可直接用 --file-path 或先获取会话共享空间再上传钉盘
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
- `chat mute` — 开启/关闭会话消息免打扰（默认开启，--off 关闭）
- `chat hide` — 在会话列表中隐藏会话（支持单聊/群聊，收到新消息时重新出现）
- `chat mute-at-all` — 关闭/开启 @所有人消息提醒（默认关闭，--off 恢复）
- `chat mute-red-envelope` — 关闭/开启红包消息提醒（默认关闭，--off 恢复）
- `chat mark-unread` / `chat mark-read` — 标记会话未读 / 标记指定消息及之前的消息已读
- `chat clear-red-point` / `chat clear-all-red-point` — 清除单个会话红点 / 一键清除所有会话红点（全部已读）
- `chat list-all-conversations` — 分页拉取当前用户全部会话列表（单聊+群聊，与 list-top-conversations 的区别是不限置顶）
- `chat clear-messages` — 清空当前用户视角下指定会话的聊天记录（不影响其他成员）
- `chat group list-all` — 分页拉取当前用户加入的所有群（list-my-groups 仅返回群主/管理员的群）
- `chat group list-join-validations` / `chat group audit-join-validation` — 拉取入群验证记录 / 审批入群验证（通过/拒绝/删除/忽略/拉黑）
- `chat file upload` — 已下线；不要调用 `chat/upload_conversation_file_by_url`，本地文件发送改用 `chat message send --msg-type file --file-path`
- `chat group transfer-owner` — 转让群主
- `chat group invite-url` — 获取群邀请链接
- `chat message reply` — 引用回复消息（在群聊中引用某条消息并回复文字）
- `chat message forward` — 转发单条消息（将一条消息从源会话转发到目标会话）
- `chat set-top` — 设置/取消会话置顶（默认置顶，--off 取消）
- `chat group-mute` — 全员禁言/取消全员禁言（默认禁言，--off 取消）
- `chat group-mute-member` — 指定群成员禁言/取消禁言（需指定 --users 和 --mute-time）
- `chat group set-admin` — 设置/取消群管理员（默认设为管理员，--off 取消）

## openDingTalkId 获取方式

多个命令参数需要 openDingTalkId（如 --open-dingtalk-id、--at-open-dingtalk-ids、--sender-open-dingtalk-id），统一获取方式如下：

1. 若知道姓名：`dws contact user search --query "姓名"` → 直接从结果中获取 openDingTalkId
2. 若只有 userId：先 `dws contact user get --ids <userId>` 获取姓名 → 再 `dws contact user search --query "姓名"` 获取 openDingTalkId

openDingTalkId 为当前用户视角下的目标用户唯一标识，不可跨用户共享。

## 核心工作流

```bash
# 1. 搜索群 — 提取 openconversation_id
dws chat search --query "项目冲刺" --format json

# 2. 拉取群消息
dws chat message list --group <openconversation_id> --time "2025-03-01 00:00:00" --format json

# 2b. 拉取未读会话列表
dws chat message list-unread-conversations --count 20 --format json

# 3. 以个人身份发送群消息
dws chat message send --group <openconversation_id> --title "周报提醒" "请大家本周五前提交周报" --format json

# 4. 以个人身份单聊（通过 userId）
dws chat message send --user <userId> "你好" --format json

# 4b. 以个人身份单聊（通过 openDingTalkId，三方应用等无法获取 userId 时使用）
dws chat message send --open-dingtalk-id <openDingTalkId> "你好" --format json

# 5. 机器人发群消息（Markdown）
dws chat message send-by-bot --robot-code <robot-code> \
  --group <openconversation_id> --title "日报" --text "## 今日完成..." --format json

# 6. 机器人单聊发消息
dws chat message send-by-bot --robot-code <robot-code> \
  --users userId1,userId2 --title "提醒" --text "请提交周报" --format json

# 7. Webhook 发告警
dws chat message send-by-webhook --token <webhook-token> \
  --title "告警" --text "CPU 超 90% @10" --at-all --format json
```

## 复合工作流

### 机器人发消息后撤回（完整流程）

`recall-by-bot` 通过机器人接口撤回机器人发出的消息（需要 `--robot-code` + `--keys`）。`chat message recall` 通过 IM 接口撤回当前用户自己发出的消息（需要 `--conversation-id` + `--msg-id`）。

```bash
# Step 1: 查我的机器人 — 提取 robot-code
dws chat bot search --format json

# Step 2: 用机器人发消息 — 提取返回中的 processQueryKey
dws chat message send-by-bot --robot-code <robot-code> --group <openconversation_id> \
  --title "通知" --text "内容" --format json

# Step 3: 用同一个 robot-code + processQueryKey 撤回
dws chat message recall-by-bot --robot-code <robot-code> --group <openconversation_id> \
  --keys <processQueryKey> --format json
```

### 机器人发群消息（含机器人不在群内的处理）

机器人通过 `send-by-bot --group` 发群消息时，如果返回"机器人不存在"错误，说明该机器人尚未加入目标群，需先邀请进群再发送。

```bash
# Step 1: 查我的机器人 — 提取 robot-code
dws chat bot search --format json

# Step 2: 尝试发送，若报"机器人不存在"则执行 Step 3
dws chat message send-by-bot --robot-code <robot-code> --group <openconversation_id> \
  --title "通知" --text "内容" --format json

# Step 3: 邀请机器人进群
dws chat group members add-bot --group <openconversation_id> --robot-code <robot-code>

# Step 4: 重新发送
dws chat message send-by-bot --robot-code <robot-code> --group <openconversation_id> \
  --title "通知" --text "内容" --format json
```

### 给机器人发单聊消息（必须先用 find 拿 openDingTalkId）

给机器人发单聊消息时，必须先用 `chat bot find` 搜索机器人拿到 `openDingTalkId`，再用 `chat message send --open-dingtalk-id` 发送。不能用 `chat bot search`，因为 search 不返回 `openDingTalkId`。

```bash
# Step 1: 搜索机器人 — 提取 openDingTalkId（必须用 find，search 没有此字段）
dws chat bot find --query "玉澜" --format json

# Step 2: 用 openDingTalkId 发单聊消息
dws chat message send --open-dingtalk-id <openDingTalkId> --text "你好" --format json
```

### 机器人 @指定人发群消息

通过 `--at-user-ids` 传入 userId 列表或 `--at-open-dingtalk-ids` 传入 openDingtalkId 列表来 @指定成员，多个用逗号分隔。`--text` 中需包含 `@userId` 或 `@openDingtalkId` 文本（不要用尖括号，不要用姓名）。通过 `--at-all` @所有人。

```bash
# Step 1: 搜人获取 userId
dws contact user search --query "张三" --format json

# Step 2: 用 userId 发送并 @（注意 text 中 @userId）
dws chat message send-by-bot --robot-code <robot-code> --group <openconversation_id> \
  --at-user-ids userId1,userId2 \
  --title "提醒" --text "@userId1 @userId2 请查收本周报告" --format json

# 或者用 openDingtalkId 发送并 @
dws chat message send-by-bot --robot-code <robot-code> --group <openconversation_id> \
  --at-open-dingtalk-ids openDingtalkId1,openDingtalkId2 \
  --title "提醒" --text "@openDingtalkId1 @openDingtalkId2 请查收本周报告" --format json

# @所有人
dws chat message send-by-bot --robot-code <robot-code> --group <openconversation_id> \
  --at-all --title "通知" --text "请所有人注意" --format json
```


### 发送图片+文字 / 文件+文字消息（跨产品: drive → chat）

- **图片+文字**：图片**必须**通过 `dt_media_upload` 工具（非 dws 命令，是 agent 可调用的独立 tool）上传获取 mediaId，然后用 Markdown 嵌入方式发送。**禁止**使用钉盘上传图片。
- **文件+文字**：文件通过钉盘上传 + Markdown 嵌入方式发送。

纯发图片/文件（不带文字）的完整流程见 [intent-guide.md](../intent-guide.md) 对应章节。

```bash
# === 图片+文字 ===
# Step 1: 调用 dt_media_upload 工具上传图片（这是一个独立的 tool，不是 dws 命令）
#    dt_media_upload 会返回 mediaId（如 @lQLPxxx）
#    提取 mediaId 可使用脚本: python extract_media_id.py "<返回的URL>"

# Step 2: 用 Markdown 语法发送（mediaId 作为图片引用）
dws chat message send --group <openconversation_id> \
  --text "![截图](mediaId) 这是本周的数据汇总" --format json

# === 文件+文字 ===
# Step 1: 上传文件到钉盘
dws drive upload --file "报告.pdf" --format json

# Step 2: 获取下载链接
dws drive download --file-id <dentryUuid> --format json

# Step 3: 用 Markdown 语法发送
dws chat message send --group <openconversation_id> \
  --text "[报告.pdf](下载链接) 这是季度报告" --format json
```

### 发送图片 / 文件（统一一条命令）

**`dws chat message send --msg-type file --file-path <本地路径>`** 适用于所有发图片/文件场景，任意扩展名。CLI 内部完成上传与发送，无需任何前置工具调用。

```bash
# 群聊
dws chat message send --group <openConversationId> --msg-type file --file-path ./screenshot.png --format json
dws chat message send --group <openConversationId> --msg-type file --file-path ./report.pdf    --format json

# 单聊（推荐 --open-dingtalk-id）
dws chat message send --open-dingtalk-id <openDingTalkId> --msg-type file --file-path ./screenshot.png --format json
```

**带文字说明**：在上一步发完文件后，再补一条文本消息即可。不要尝试把文字塞进 `--msg-type file` 命令（该命令不读 `--text`）。

```bash
dws chat message send --open-dingtalk-id <openDingTalkId> --msg-type file --file-path ./screenshot.png --format json
dws chat message send --open-dingtalk-id <openDingTalkId> --text "这是本周数据汇总" --format json
```

**旧链路（仅当上游已经持有 `dt_media_upload` 返回的 mediaId 时才用）**：

```bash
dws chat message send --group <openConversationId> --msg-type image --media-id "@lQLPD4JNnliqBq3NBQDNA8Cw" --format json
```

#### 创建并推送流式卡片 — 向群聊或单聊发送流式卡片消息

群聊传 --group，单聊传 --receiver，二者互斥。

**注意：send-card 必须和 update-card 搭配使用。** 创建卡片时无需传入内容，后续通过 update-card 更新内容，最后一次更新必须将 --flow-status 设为 3（finish），否则卡片会一直处于"生成中"的加载状态。
flow-status 取值：1=处理中(PROCESSING)，2=输入中(INPUTTING)，3=完成(FINISH)，4=执行中(EXECUTING)，5=错误(ERROR)。
```
Usage:
  dws chat message send-card [flags]
Example:
  dws chat message send-card --group <openConversationId>
  dws chat message send-card --receiver <openDingTalkId>
  # 查询群 ID: dws chat search --query "群名"
  # 查询人员: dws aisearch person --keyword "姓名" --dimension name
Flags:
      --group string      群聊 openConversationId（群聊时必填，与 --receiver 互斥）
      --receiver string   单聊接收者 openDingTalkId（单聊时必填，与 --group 互斥）
```

#### 流式更新卡片内容 — 更新已发送的流式卡片内容

--biz-id 为 send-card 返回的业务 ID，--flow-status 控制流式状态。
flow-status 取值：1=处理中(PROCESSING)，2=输入中(INPUTTING)，3=完成(FINISH)，4=执行中(EXECUTING)，5=错误(ERROR)。

**最后一次更新必须将 --flow-status 设为 3（finish），否则卡片会一直处于"生成中"的加载状态。**
```
Usage:
  dws chat message update-card [flags]
Example:
  dws chat message update-card --biz-id <bizId> --content "更新的卡片内容" --flow-status 2
  dws chat message update-card --biz-id <bizId> --content "最终内容" --flow-status 3
Flags:
      --biz-id string    卡片业务 ID (必填)
      --content string   卡片消息内容 (必填)
      --flow-status int  流式状态 (必填)
```

## 上下文传递表

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `chat search` | `openConversationId` | message send/list、group members 等的 --group |
| `chat group create` | `openConversationId` | 同上 |
| `chat message list-all` | `nextCursor` | 下次 list-all 的 --cursor |
| `aisearch person` | `userId` | message send 的 --user、send-by-bot 的 --users、send-by-bot 的 --at-user-ids、list-by-sender 的 --sender-user-id |
| `aisearch person` → `contact user get` | `openDingTalkId` | message send 的 --at-open-dingtalk-ids、--open-dingtalk-id、send-by-bot 的 --open-dingtalk-ids、send-by-bot 的 --at-open-dingtalk-ids、list-by-sender 的 --sender-open-dingtalk-id、message list 的 --open-dingtalk-id |
| `chat bot search` | `robotCode` | send-by-bot / recall-by-bot 的 --robot-code（仅我创建的机器人，无 openDingTalkId） |
| `chat bot find` | `botOpenDingTalkId` | 给机器人发单聊消息（send --open-dingtalk-id；字段名是 botOpenDingTalkId，非 openDingTalkId） |
| `chat message send-by-bot` | `processQueryKey` | recall-by-bot 的 --keys |
| `chat message send` | `openTaskId` | query-send-status 的 --open-task-id |
| `chat message list` | `openMessageId` | recall 的 --msg-id |
| `chat message search` | `nextCursor` | 下次 message search 的 --cursor |
| `chat message search-advanced` | `nextCursor` | 下次 message search-advanced 的 --cursor |
| `chat search-common` | `openConversationId` | message send/list 等的 --group |
| `chat conversation-info` | `newCSpaceIdIM` | drive upload 的 --space-id（发送文件消息前获取共享空间） |
| `chat file upload` | 无（已下线） | 不要调用；常规发图/发文件用 `chat message send --msg-type file --file-path` |
| `chat message list` | `openMsgId` | message read-status 的 --message-id |
| `chat group-role list` | `openRoleId` | group-role update/remove/set-user/remove-user 的 --role-id |
| `chat message create-text-emotion` | `emotionId` | add-text-emotion 的 --emotion-id |
| `chat category list` | `categoryId` | category list-conversations 的 --category-id |
| `chat group get-by-group-id` | `openConversationId` | 同 chat search，将群号转为 openConversationId |
| `chat message send-card` | `bizId` | update-card 的 --biz-id |
| `drive download` | 下载链接 | message send 的 Markdown 图片/链接语法 |
| `chat message list` | `openMessageId` | message reply 的 --ref-msg-id、message forward 的 --msg-id |
| `chat search` | `openConversationId` | set-top 的 --conversation-id、group-mute / group-mute-member 的 --group |

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
- `chat message send` 的 text 是位置参数（恰好 1 个），非 flag；群聊用 `--group`，单聊用 `--user`（userId）或 `--open-dingtalk-id`（openDingTalkId），三者互斥；纯文本/Markdown 单聊传 `--user` 时直接走 userId 发送能力；`--at-all`、`--at-open-dingtalk-ids` 仅在 `--group` 群聊时生效；富媒体消息通过 `--msg-type` 指定类型（image/file/audio/video），必须显式指定；发送文件/媒体消息时，必须先根据文件扩展名判断 msgType：图片→image，音频→audio，视频→video，其他→file，不可跳过此判断
- `chat message list-all` 的四个参数（--start、--end、--limit、--cursor）每次请求都必须传递；翻页时用响应中的 nextCursor 值作为下次 --cursor
- `chat message list` 的 `--group`、`--user`、`--open-dingtalk-id` 三者互斥，必须且只能指定其一
- `chat message list-by-sender` 不需要指定单聊/群聊，返回结果自带会话类型标识；`--sender-user-id`（userId）与 `--sender-open-dingtalk-id`（openDingTalkId）二选一；时间用 `--start`/`--end`（ISO-8601），分页用 `--limit`/`--cursor`
- `chat message list-mentions` 可选 `--group` 指定群聊，不传则查全部；时间用 `--start`/`--end`（ISO-8601），分页用 `--limit`/`--cursor`
- `chat message list-unread-conversations` 获取当前用户未读会话列表，可选 `--count` 指定返回条数
- `chat message search` 按关键词搜索消息内容，`--query` 必填，可选 `--group` 限定搜索某个会话；时间用 `--start`/`--end`（ISO-8601），分页用 `--limit`（默认 100）/`--cursor`
- `chat message read-status` 查询指定消息的已读/未读状态，仅消息发送者可查询自己发出的消息；`--group`、`--message-id` 必填；目标用户 userId 用 `--user`/`--users`，openDingTalkId 用 `--target-open-dingtalk-ids`，不传则查所有接收者
- `chat search-common` 搜索共同群，`--nicks` 传人员昵称（逗号分隔），`--match-mode` AND/OR 控制匹配逻辑，分页用 `--limit`（默认 20）/`--cursor`
- `chat list-top-conversations` 拉取置顶会话列表，分页用 `--limit`（默认 1000）/`--cursor`；用户询问"置顶会话"或"置顶消息"时均路由到此命令
- `--user` 和 `--open-dingtalk-id` 本质上都是发起单聊操作，只是用户标识格式不同：userId 为企业内部应用常用标识，openDingTalkId 为三方应用或跨组织场景下的用户标识，服务端对两种 ID 的解析逻辑不同
- `--time` 格式: `yyyy-MM-dd HH:mm:ss`，为拉取消息的起始时间点；`--direction` 控制方向（newer=从给定时间往现在拉，older=从给定时间往以前拉），`--limit` 控制数量
- `chat search` 挂在 `chat` 下（非 `chat group` 下），路径为 `dws chat search`
- `send-by-bot` 群聊传 `--group`，单聊传 `--users` 或 `--open-dingtalk-ids`，与 `--group` 互斥且必选其一；群聊时可选 `--at-user-ids` @指定成员（传 userId 列表）或 `--at-open-dingtalk-ids` @指定成员（传 openDingtalkId 列表），content 中需包含对应 @标识；`--at-all` @所有人；群聊场景如果返回"机器人不存在"错误，需先通过 `chat group members add-bot --group <openConversationId> --robot-code <robot-code>` 将机器人邀请进群后再发送
- `recall-by-bot` 群聊传 `--group` + `--keys`，单聊仅传 `--keys`（不传 `--group` 即为单聊撤回）
- `send-by-webhook` 支持 `--at-all`、`--at-mobiles`、`--at-users` 进行 @ 操作，但需在 `--text` 中包含 `@userId` 或 `@手机号` 才能生效；`--at-all` @所有人时需在 `--text` 中包含 `@10`
- `chat group-role` 系列命令用于管理群的自定义身份标签：`list` 查列表，`add` 创建，`update` 改名，`remove` 删除；`set-user` 覆盖某人全部身份（传空 --role-ids 则清除），`remove-user` 仅移除指定身份，`query-user` 查询某人当前身份；用户用 `--user <userId>`
- 消息**换行符**（`send` / `send-by-bot` / `send-by-webhook` 的 `--text`）有两层要求：(1) 必须是**真实换行符** `U+000A`，不是字面量 `\n`；(2) Markdown 规范下单换行不生效，需用空行 `\n\n`（段落分隔）或行尾两空格 + 换行 / `<br>`（硬换行）
- `chat group transfer-owner` 转让群主，需传 --group（openConversationId）；新群主 userId 用 `--user`，openDingTalkId 用 `--new-owner`
- `chat group invite-url` 获取群邀请链接，需传 --group（openConversationId），可选 --expires-seconds 指定有效期（秒，0=永久）
- `chat group quit` 退出群聊，需传 --group（openConversationId）
- `chat group update-icon` 更新群头像，需传 --group（openConversationId）和 --icon-media-id（mediaId）
- `chat group update-settings` 更新群设置，需传 --group（openConversationId）、--setting-key（设置项 key）、--status（0=关闭 1=开启）
- `chat message send-card` 创建并推送流式卡片，群聊传 --group，单聊传 --receiver，二者互斥；不传 content，后续通过 update-card 更新内容
- `chat message update-card` 流式更新卡片内容，需传 --biz-id（创建卡片返回的业务 ID）、--content、--flow-status
- `chat message list-by-ids` 根据消息 ID 批量查询，--msg-ids 逗号分隔，最多 50 条
- `chat message add-emoji` / `remove-emoji` 需传 --group（openConversationId）、--msg-id（openMsgId）、--emoji（表情名称）
- `chat message add-text-emotion` / `remove-text-emotion` 需传 --group、--msg-id、--emotion-id、--emotion-name、--text、--background-id，六个参数全部必填
- `chat message create-text-emotion` 创建文字表情模板，返回 emotionId；--background-id 可选，不传由服务端默认分配
- `chat category list` 无需参数；`category list-conversations` 需传 --category-id（通过 category list 获取）
- `chat mute` 默认开启免打扰，传 --off 关闭；--conversation-id / --id / --chat 三个别名均可用于传入会话 ID
- `chat hide` 隐藏会话，需传 --conversation-id（openConversationId，支持单聊/群聊），隐藏后不显示在列表中，收到新消息时重新出现
- `chat mute-at-all` 关闭/开启 @所有人消息提醒，需传 --conversation-id（openConversationId），默认关闭通知，传 --off 恢复接收
- `chat mute-red-envelope` 关闭/开启红包消息提醒，需传 --conversation-id（openConversationId），默认关闭通知，传 --off 恢复接收
- `chat message reply` 引用回复消息（**单聊/群聊均可**），需传 --conversation-id（openConversationId，单聊与群聊使用同一字段）、--ref-msg-id（被引用消息 openMessageId）、--ref-sender（被引用消息发送者 openDingTalkId）、--text（回复内容）；目前回复类型仅支持 text
- `chat message forward` 转发单条消息（**源/目标会话均支持单聊/群聊**，常见组合：群→群、群→单、单→群、单→单），需传 --src-conversation-id（源会话 openConversationId）、--msg-id（源消息 openMessageId）、--dest-conversation-id（目标会话 openConversationId）
- `chat set-top` 设置/取消会话置顶（**单聊/群聊均可**），需传 --conversation-id（openConversationId，单聊与群聊使用同一字段），默认置顶，传 --off 取消
- `chat message reply` 以当前用户身份引用回复，与 `chat message send` 的用户身份发送语义一致
- **如何获取 openConversationId**（如果上层已有则直接使用，不必再查）：
  - 群聊：`dws chat search --query "群名"`
  - 单聊：`dws chat conversation-info --user <userId>` 或 `dws chat conversation-info --open-dingtalk-id <openDingTalkId>`（人员信息可通过 `dws aisearch person --keyword "姓名" --dimension name` 获取）
- `chat group-mute` 全员禁言/取消全员禁言，需传 --group（openConversationId），默认禁言，传 --off 取消
- `chat group-mute-member` 指定群成员禁言，需传 --group、--user/--users（userId，逗号分隔）、--mute-time（毫秒，仅禁言时必填，支持 300000/3600000/86400000/604800000/2592000000），传 --off 解除禁言；CLI 会自动把 userId 解析成 openDingTalkId 再调用，直接传 userId 即可；禁言群主会被服务端拒绝
- `chat group set-admin` 设置/取消群管理员，需传 --group（openConversationId）、--user/--users（userId，逗号分隔），默认设为管理员，传 --off 取消

## 自动化脚本

| 脚本 | 场景 | 用法 |
|------|------|------|
| [chat_export_messages.py](../../scripts/chat_export_messages.py) | 导出群聊消息到 JSON 文件 | `python chat_export_messages.py --query "项目冲刺" --time "2026-03-10 00:00:00"` |
| [chat_history_with_user.py](../../scripts/chat_history_with_user.py) | 查询与某人的单聊聊天记录 | `python chat_history_with_user.py --name "张三" --time "2026-03-10 00:00:00"` |
| [extract_media_id.py](../../scripts/extract_media_id.py) | 从 dt_media_upload URL 提取 mediaId | `python extract_media_id.py "<URL>"`（输出如 @lQLPxxx，直接用于 --media-id） |

## 相关产品

- [contact](./contact.md) — 搜索同事/好友，获取 userId 用于 --user、send-by-bot --users、send-by-bot --at-user-ids、list-by-sender --sender-user-id；获取 openDingTalkId 用于 message send 的 --at-open-dingtalk-ids、--open-dingtalk-id、send-by-bot --open-dingtalk-ids、send-by-bot --at-open-dingtalk-ids、list-by-sender 的 --sender-open-dingtalk-id
- [drive](./drive.md) — 上传文件获取下载链接，用于 Markdown 图片/文件消息
