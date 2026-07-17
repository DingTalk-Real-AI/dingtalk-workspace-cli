# 机器人、分组与会话状态命令

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

#### 搜索【全部可用】机器人 — 含他人创建/官方机器人，额外返回 openDingTalkId

范围: 当前用户可用的全部机器人（含他人创建、官方机器人）。
返回字段: 额外返回 openDingTalkId（可用于给机器人发单聊消息），search 没有此字段。
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
| 额外返回 openDingTalkId | 无 | 有（可用于给机器人发单聊消息） |
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
  # 分组ID 可通过 dws chat category list 获取
Flags:
      --category-id int   会话分组 ID (必填)
      --exclude-muted     是否排除已设置免打扰的会话（默认 false）
```

#### 创建用户自定义会话分组
```
Usage:
  dws chat category create [flags]
Example:
  dws chat category create --title "工作群"
  dws chat category create --title "项目组"
Flags:
      --title string   分组名称 (必填)
```

#### 创建智能会话分组 — 可指定群名称关键词和群内成员作为匹配规则
```
Usage:
  dws chat category create-smart [flags]
Example:
  dws chat category create-smart --name "工作群"
  dws chat category create-smart --name "项目组" --keywords "项目,开发"
  dws chat category create-smart --name "团队群" --members openDingTalkId1,openDingTalkId2
  dws chat category create-smart --name "重点群" --keywords "重点" --members openDingTalkId1
Flags:
      --name string       分组名称 (必填)
      --keywords string   群名称关键词列表，逗号分隔（可选）
      --members string    群内成员 openDingTalkId 列表，逗号分隔（可选）
```

#### 删除用户自定义会话分组
```
Usage:
  dws chat category delete [flags]
Example:
  dws chat category delete --category-id <分组ID>
  # 分组ID 可通过 dws chat category list 获取
Flags:
      --category-id int   会话分组 ID (必填)
```

#### 更新用户自定义会话分组的名称
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

#### 将会话移动到指定的自定义分组中
```
Usage:
  dws chat category add-conv [flags]
Example:
  dws chat category add-conv --group <openConversationId> --category-ids 123,456
  # 分组ID 可通过 dws chat category list 获取
  # 查询群 ID: dws chat search --query "群名"
Flags:
      --group string         会话 openConversationId (必填)
      --category-ids string  目标分组 ID 列表，逗号分隔 (必填)
```

#### 将会话从指定的自定义分组中移出
```
Usage:
  dws chat category remove-conv [flags]
Example:
  dws chat category remove-conv --group <openConversationId> --category-ids 123,456
  # 分组ID 可通过 dws chat category list 获取
  # 查询群 ID: dws chat search --query "群名"
Flags:
      --group string         会话 openConversationId (必填)
      --category-ids string  目标分组 ID 列表，逗号分隔 (必填)
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
  - 标记未读后会话列表中会显示未读状态
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
  - 等效于“全部已读”操作
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
      --limit int        每页数量（默认 1000）
      --cursor int64     分页游标（首次不传或传 0，翻页传 nextCursor）
      --exclude-muted    是否排除已免打扰会话（默认 false）

注意:
  - 返回结果包含单聊和群聊，不区分会话类型
  - 翻页: hasMore=true 时用返回的 nextCursor 作为下次 --cursor
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

#### 审批入群验证 — 通过、拒绝、删除单个审核

支持通过、拒绝、删除、忽略、拒绝并拉黑等操作。

```
Usage:
  dws chat group audit-join-validation [flags]
Example:
  dws chat group audit-join-validation --group <openConversationId> --record-id 123456 --applicant <openDingTalkId> --inviter <openDingTalkId> --status AuditApprove
  dws chat group audit-join-validation --group <openConversationId> --record-id 123456 --applicant <openDingTalkId> --inviter <openDingTalkId> --status AuditRefuse --description "不符合入群条件"
  # 查询入群验证记录: dws chat group list-join-validations
Flags:
      --group string        群 openConversationId (必填)
      --record-id int64     申请记录 ID (必填)
      --applicant string    申请人 openDingTalkId (必填)
      --inviter string      邀请人 openDingTalkId (必填)
      --status string       审批动作: AuditApprove/AuditDelete/AuditIgnore/AuditRefuse/AuditBlock (必填)
      --description string  审批说明（可选）

注意:
  - status 可选值: AuditApprove(通过), AuditDelete(删除), AuditIgnore(忽略), AuditRefuse(拒绝), AuditBlock(拒绝且拉黑)
  - record-id、applicant、inviter 可通过 dws chat group list-join-validations 查询获得
```
