# 群组、群身份与群搜索命令

## 命令总览

### group (群组管理)

#### 创建群 — 支持内部群/外部群/普通群/话题圈，当前登录用户自动成为群主

创建一个群聊，支持指定群名称、初始成员列表、群类型和话题模式。默认创建内部群。

```
Usage:
  dws chat group create [flags]
Example:
  dws chat group create --name "Q1 项目冲刺群" --users userId1,userId2,userId3
  dws chat group create --name "外部合作群" --users userId1,userId2 --type EXTERNAL
  dws chat group create --name "话题圈" --users userId1,userId2 --thread
Flags:
      --name string     群名称 (必填)
      --users string    成员 userId 或 openDingTalkId（可混传），用户本身会自动加入无需包含，逗号分隔，不超过20个 (必填)
      --type string     群类型: INTERNAL(内部群,默认)/EXTERNAL(外部群)/NORMAL(普通群)
      --thread          开启话题模式，将创建话题圈
```

创建时当前登录用户自动成为群主。需要指定他人为群主时，先创建群并从返回中取 `openConversationId`，再执行 `dws chat group transfer-owner --group <openConversationId> --new-owner <openDingTalkId>`。

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

#### 批量查看群成员详情 — 根据成员 openDingTalkId 列表批量查询群成员详情信息
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
  # 查询人员: dws contact user search --query "姓名" --format json
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

#### 分享群聊链接到会话 — 将指定群的邀请链接分享到另一个会话或单聊用户

--target 和 --receiver 二选一：--target 指定目标会话，--receiver 指定单聊用户。
```
Usage:
  dws chat group share-invite [flags]
Example:
  dws chat group share-invite --source <被分享群openConversationId> --target <目标会话openConversationId>
  dws chat group share-invite --source <被分享群openConversationId> --receiver <接收者openDingTalkId>
  dws chat group share-invite --source <openConversationId> --target <openConversationId> --expires-seconds 86400
  # 查询群 ID: dws chat search --query "群名"
Flags:
      --source string            被分享群的 openConversationId (必填)
      --target string            接收分享消息的会话 openConversationId（与 --receiver 二选一）
      --receiver string          接收分享消息的单聊用户 openDingTalkId（与 --target 二选一）
      --expires-seconds int64    链接有效期（秒），0 表示永久有效，不传使用服务端默认值
      --uuid string              消息幂等键（可选）
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

#### 设置用户在群内的群昵称 — 设置当前用户在指定群聊内的个人群昵称
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

#### 设置群备注 — 设置当前用户对指定群聊的备注名称（仅自己可见）
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

> 重要限制：此命令只能拉取到用户作为「群主」或「管理员」的群，无法获取普通成员身份的群聊。如果用户要找的群不在结果中，必须改用 `dws chat search --query "关键词"` 按名称搜索。

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
      --exclude-muted  是否排除已设置免打扰的群聊（默认 false）

注意:
  - 底层先拉取最近 1000 条会话，剔除单聊和话题圈后筛选出群主/管理员的群
  - 内部群会校验 orgId 归属
  - 不传 --role 时返回群主 + 管理员的所有群
```

#### 发布群公告 — 在指定群聊中发布群公告，支持 Markdown、定时发布

正文为 Markdown 格式。支持标题、加粗、斜体、删除线、行内代码、链接、代码块、有序/无序/任务列表、表格、引用、分割线、图片、段落、换行。
```
Usage:
  dws chat group notice create [flags]
Example:
  dws chat group notice create --group <openConversationId> --content "今晚 22 点系统维护，请提前保存工作内容"
  dws chat group notice create --group <openConversationId> --content "# 重要通知\n请大家查收" --sticky --send-ding
  dws chat group notice create --group <openConversationId> --content "明早九点例会" --run-at "2026-07-03T09:00:00+08:00"
  # 查询群 ID: dws chat search --query "群名"
Flags:
      --group string       群聊 openConversationId (必填)
      --content string     公告正文，Markdown 格式 (必填)
      --sticky             是否吊顶置顶（默认 false）
      --send-ding          是否发 DING 提醒（默认 false）
      --run-at string      定时发布时间 ISO-8601（如 2026-07-03T09:00:00+08:00，传入则定时发布）

注意:
  - 下划线/字体色/背景色/字号为编辑器专属能力，Markdown 无对应语法，无法通过本命令表达
  - --run-at 建议带时区偏移，不带偏移时按北京时区处理
```

#### 修改群公告 — 整体替换指定群公告的内容
```
Usage:
  dws chat group notice edit [flags]
Example:
  dws chat group notice edit --group <openConversationId> --notice-id <dataId> --content "更新后的公告内容"
  dws chat group notice edit --group <openConversationId> --notice-id <dataId> --content "更新后的公告内容" --sticky --send-ding
  # 查询公告 ID: dws chat group notice list --group <openConversationId>
Flags:
      --group string       群聊 openConversationId (必填)
      --notice-id string   群公告 dataId (必填)
      --content string     公告新正文，Markdown 格式 (必填)
      --sticky             是否吊顶置顶（不传按 false 处理）
      --send-ding          是否发 DING 提醒（默认 false）

注意:
  - 修改会整体替换原公告正文，需传入完整的新内容
```

#### 查看群公告详情 — 查询单条群公告的详细信息
```
Usage:
  dws chat group notice get [flags]
Example:
  dws chat group notice get --group <openConversationId> --notice-id <dataId>
  # 查询公告 ID: dws chat group notice list --group <openConversationId>
Flags:
      --group string       群聊 openConversationId (必填)
      --notice-id string   群公告 dataId (必填)

注意:
  - 返回正文摘要、吊顶状态、发布者、已读人数/应收人数、点赞/评论数、是否可编辑、是否已读、是否定时公告等信息
```

#### 查看群公告列表 — 分页拉取指定群聊的群公告列表
```
Usage:
  dws chat group notice list [flags]
Example:
  dws chat group notice list --group <openConversationId>
  dws chat group notice list --group <openConversationId> --limit 20 --cursor <nextPageCursor>
  dws chat group notice list --group <openConversationId> --scheduled
  # 查询群 ID: dws chat search --query "群名"
Flags:
      --group string    群聊 openConversationId (必填)
      --limit int       每页返回数量（默认 10，最大 100）
      --cursor string   分页游标（首次不传，翻页传返回的 nextPageCursor）
      --scheduled       是否查询定时公告列表（默认 false，查询已发布公告）

注意:
  - 分页: hasMore=true 时用返回的 nextPageCursor 作为下次 --cursor
  - 默认查询已发布公告，传 --scheduled 查询尚未到发布时间的定时公告
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
  # 查询人员: dws contact user search --query "姓名" --format json
  # 查询 role-id: dws chat group-role list --group <openConversationId>
Flags:
      --group string      群聊 openConversationId (必填)
      --user string       用户 userId（必填）
      --role-ids string   群身份 openRoleId 列表，逗号分隔 (必填)，传空字符串则清除该用户所有群身份
```

#### 移除用户的指定群身份 — 从用户身上移除指定的群身份（不影响其他群身份）
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
