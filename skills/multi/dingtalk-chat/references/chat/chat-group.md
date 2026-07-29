# chat-group：群聊、成员、设置与群身份

> 返回入口：[chat.md](../chat.md)

## 适用场景

用于群搜索、建群、成员增删、机器人进群、群设置、群主转让、群邀请分享、群公告、入群审批、群身份和群禁言。

## 必读约束

- 群聊目标统一使用 `openConversationId`。只有数字群号时，先用 `chat group get-by-group-id` 转换。
- `chat search` 在 `chat` 根下，不在 `chat group` 下。
- 群成员操作中，`--users` 常为逗号分隔列表；具体要求以命令 `--help` 为准。
- 解散群、踢人、转让群主、禁言、管理员设置都是高影响操作，执行前必须确认目标群和用户。
- 发布或修改群公告会触达群成员；公告正文是 Markdown，定时公告 `--run-at` 建议带时区。

## 命令明细

### 搜索与基础信息

| 命令 | 用途 | 示例与要点 |
|------|------|------------|
| `chat search` | 按关键词搜索群聊 | `dws chat search --query "项目冲刺" --limit 20 --cursor 0`；可选 `--exclude-muted` |
| `chat search-common` | 搜索共同群 | `dws chat search-common --nicks "风雷,山乔" --match-mode AND --limit 20 --cursor 0` |
| `chat group get-by-group-id` | 数字群号转 openConversationId | `dws chat group get-by-group-id --group-id 12345678` |
| `chat group bots` | 查看群内所有机器人 | `dws chat group bots --group <openConversationId>` |

`search-common` 中 `--match-mode AND` 表示所有人都在群里，`OR` 表示任一人在群里。

### 群创建与基础操作

#### `dws chat group create`

```bash
dws chat group create --name "Q1 项目冲刺群" --users userId1,userId2,userId3
dws chat group create --name "外部合作群" --users userId1,userId2 --type EXTERNAL
dws chat group create --name "话题圈" --users userId1,userId2 --thread
```

关键 flags：

| Flag | 说明 |
|------|------|
| `--name` | 群名称，必填 |
| `--users` | 成员 userId 或 openDingTalkId，逗号分隔，必填 |
| `--type` | `INTERNAL` / `EXTERNAL` / `NORMAL`，默认 `INTERNAL` |
| `--thread` | 开启话题模式，创建话题圈 |

创建成功后提取 `openConversationId`，用于发消息、成员管理、群设置。

| 命令 | 用途 | 必填参数 |
|------|------|----------|
| `group rename` | 更新群名称 | `--id` `--name` |
| `group quit` | 当前用户退出群聊 | `--group` |
| `group dismiss` | 解散群聊，不可逆 | `--group` |

### 成员与机器人

| 命令 | 用途 | 示例 |
|------|------|------|
| `group members` | 分页查看群成员 | `dws chat group members --id <openConversationId> --cursor 0` |
| `group members add` | 添加群成员 | `dws chat group members add --id <openConversationId> --users userId1,userId2` |
| `group members remove` | 移除群成员 | `dws chat group members remove --id <openConversationId> --users userId1,userId2` |
| `group members list-by-ids` | 按 openDingTalkId 批量查成员详情 | `dws chat group members list-by-ids --id <openConversationId> --users openDingTalkId1,openDingTalkId2` |
| `group members add-bot` | 将自定义机器人加入群 | `dws chat group members add-bot --id <openConversationId> --robot-code <robot-code>` |
| `group members remove-bot` | 从群内移除机器人 | `dws chat group members remove-bot --id <openConversationId> --bot-id <openBotId>` |

机器人发群消息如果报“机器人不存在”，先 `group members add-bot` 再重发。

### 群设置与权限

| 命令 | 用途 | 必填参数 |
|------|------|----------|
| `group transfer-owner` | 转让群主 | `--group` + `--user`(userId) 或 `--new-owner`(openDingTalkId) |
| `group upgrade-to-external` | 将普通群升级为外部群（不可逆，需先确认） | `--group` `--yes`；可选 `--extension`（拓展字段） |
| `group invite-url` | 获取群邀请链接 | `--group`，可选 `--expires-seconds`，0 表示永久 |
| `group share-invite` | 将指定群的邀请链接分享到另一个会话或单聊用户 | `--source` + `--target` / `--receiver` 二选一 |
| `group update-icon` | 更新群头像 | `--group` `--icon-media-id` |
| `group update-settings` | 更新管理员级别的群功能开关 | `--group` `--setting-key` `--status` |
| `group user-settings query` | 批量查询当前用户自己的群会话设置 | `--groups` |
| `group user-settings set` | 批量更新当前用户自己的群会话设置 | `--items` |
| `group update-nick` | 设置或清除当前用户群昵称 | `--group`，可选 `--nick`；不传则清除群昵称 |
| `group update-alias` | 设置当前用户群备注 | `--group` `--alias-title` |
| `group set-history` | 设置新成员可查看历史消息范围 | `--group` `--option` |
| `group get-mute-config` | 查询群用户禁言配置 | `--group` |
| `group-mute` | 全员禁言/解除全员禁言 | `--group`，默认禁言，`--off` 解除 |
| `group-mute-member` | 指定成员禁言/解除禁言 | `--group` `--user`/`--users`；禁言需 `--mute-time` |
| `group set-admin` | 设置/取消管理员 | `--group` `--user`/`--users`；`--off` 取消 |

`update-settings` 是管理员级别的群功能开关操作，常用 settingKey：`authority`、`joinValidation`、`onlyAdminCanAtAll`、`searchable`、`addFriendForbidden`、`onlyAdminCanDING`、`onlyAdminCanPinMsg`、`onlyAdminCanSendFile`、`groupEmailDisabled`、`groupLiveAuthority`、`groupBillAuthority`。

`group user-settings` 是当前登录用户自己的群会话设置批量入口（置顶、免打扰、群昵称、群备注等），不是管理员级别的群功能开关；单个群昵称/群备注仍优先使用 `group update-nick` / `group update-alias`，管理员级别的群功能开关继续使用 `group update-settings`。

`group user-settings set --items` 传 JSON 数组，每个元素表示一个群会话的当前用户设置。字段含义：

| 字段 | 含义 | 值说明 |
|------|------|--------|
| `openConversationId` | 群会话 ID | 必填，来自 `chat search` / `group list-all` 等真实返回 |
| `top` | 当前用户是否置顶该群会话 | `true`=置顶，`false`=取消置顶 |
| `mute` | 当前用户是否开启该群会话免打扰 | `true`=开启免打扰，`false`=关闭免打扰 |
| `groupNick` | 当前用户在该群里的群昵称 | 字符串；空字符串表示清空昵称 |
| `groupAlias` | 当前用户给该群设置的备注 | 字符串；空字符串表示清空备注 |

只传本次要改的字段；不要补用户没要求修改的字段。批量设置多个群时，`items` 放多个对象。

`group-mute-member --mute-time` 单位毫秒，常用值：`300000`、`3600000`、`86400000`、`604800000`、`2592000000`。

`group share-invite` 的 `--source` 是被分享群的 `openConversationId`；`--target` 是接收分享消息的会话，`--receiver` 是接收分享消息的单聊用户 `openDingTalkId`，二者必须二选一。

```bash
dws chat group share-invite --source <sourceOpenConversationId> --target <targetOpenConversationId>
dws chat group share-invite --source <sourceOpenConversationId> --receiver <receiverOpenDingTalkId>
dws chat group share-invite --source <sourceOpenConversationId> --target <targetOpenConversationId> --expires-seconds 86400 --uuid <uuid>
dws chat group user-settings query --groups <openConversationId1>,<openConversationId2>
dws chat group user-settings set --items '[{"openConversationId":"cid1","top":true,"mute":false}]'
```

### 群公告

群公告正文使用 Markdown。支持标题、加粗、斜体、删除线、行内代码、链接、代码块、列表、表格、引用、分割线、图片、段落和换行；下划线、字体色、背景色、字号属于编辑器专属能力，无法通过 Markdown 表达。

| 命令 | 用途 | 必填参数 |
|------|------|----------|
| `group notice create` | 发布群公告，支持置顶、DING 和定时发布 | `--group` `--content` |
| `group notice edit` | 整体替换指定群公告内容 | `--group` `--notice-id` `--content` |
| `group notice get` | 查询单条群公告详情 | `--group` `--notice-id` |
| `group notice list` | 分页拉取群公告列表 | `--group` |

```bash
dws chat group notice create --group <openConversationId> --content "今晚 22 点系统维护，请提前保存工作内容"
dws chat group notice create --group <openConversationId> --content "# 重要通知\n\n请大家查收" --sticky --send-ding
dws chat group notice create --group <openConversationId> --content "明早九点例会" --run-at "2026-07-03T09:00:00+08:00"
dws chat group notice list --group <openConversationId> --limit 20 --cursor <nextPageCursor>
dws chat group notice get --group <openConversationId> --notice-id <dataId>
dws chat group notice edit --group <openConversationId> --notice-id <dataId> --content "更新后的公告内容"
```

注意事项：

- `notice edit` 会整体替换原公告正文，必须传完整的新内容。
- `notice list --scheduled` 查询尚未到发布时间的定时公告；默认查询已发布公告。
- `hasMore=true` 时，用返回的 `nextPageCursor` 继续翻页。
- `notice get` 返回正文摘要、置顶状态、发布者、已读人数/应收人数、点赞/评论数、是否可编辑、是否已读、是否定时公告等信息。

### 群列表与入群审批

| 命令 | 用途 | 示例与要点 |
|------|------|------------|
| `group list-my-groups` | 拉取我创建/管理的群 | 可选 `--role OWNER/ADMIN`、`--limit`、`--exclude-muted` |
| `group list-all` | 分页拉取我加入的所有群 | `--limit` 默认 100，最大 200；翻页用 `nextCursor` |
| `group list-join-validations` | 拉取入群验证记录 | 包括自己被拒绝的记录以及作为审批者的记录 |
| `group audit-join-validation` | 审批入群验证 | `--group` `--record-id` `--applicant` `--inviter` `--status` |

审批动作 `--status`：`AuditApprove`、`AuditDelete`、`AuditIgnore`、`AuditRefuse`、`AuditBlock`。

```bash
dws chat group list-join-validations --limit 20
dws chat group audit-join-validation --group <openConversationId> --record-id 123456 --applicant <openDingTalkId> --inviter <openDingTalkId> --status AuditApprove
```

### 群身份

`chat group-role` 管理群内自定义身份标签。

| 命令 | 用途 | 必填参数 |
|------|------|----------|
| `group-role list` | 查看群身份列表 | `--group` |
| `group-role add` | 新增群身份 | `--group` `--name` |
| `group-role update` | 修改群身份名称 | `--group` `--role-id` `--name` |
| `group-role remove` | 删除群身份 | `--group` `--role-id` |
| `group-role set-user` | 覆盖用户全部群身份，空 `--role-ids` 表示清除 | `--group` `--user` `--role-ids` |
| `group-role remove-user` | 移除用户指定群身份 | `--group` `--user` `--role-ids` |
| `group-role query-user` | 查询用户当前群身份 | `--group` `--user` |

`openRoleId` 来自 `group-role list` 返回。

## 常见工作流

### 搜索群并发消息

```bash
dws chat search --query "项目冲刺" --format json
dws chat message send --group <openConversationId> --text "请大家看一下最新进展" --format json
```

### 建群并拉人

```bash
dws aisearch person --keyword "张三" --dimension name --format json
dws chat group create --name "Q1 项目冲刺群" --users userId1,userId2 --format json
dws chat group members add --id <openConversationId> --users userId3,userId4 --format json
```

### 分享群邀请并发布公告

```bash
dws chat group share-invite --source <sourceOpenConversationId> --target <targetOpenConversationId> --format json
dws chat group notice create --group <openConversationId> --content "# 项目公告\n\n请大家关注最新安排" --send-ding --format json
```

### 设置管理员并禁言成员

```bash
dws chat group set-admin --group <openConversationId> --users userId1,userId2 --format json
dws chat group-mute-member --group <openConversationId> --users userId3 --mute-time 3600000 --format json
```

## 常见错误与回退

- 只有数字群号：先 `group get-by-group-id`，不要直接当 `--group`。
- 找不到群：先扩大 `chat search --query`，不要臆测 openConversationId。
- 入群审批缺参数：从 `group list-join-validations` 提取 `record-id`、`applicant`、`inviter`。
- 机器人进群失败：确认当前用户有群管理权限。
- 分享群邀请目标不明确：`--target` 和 `--receiver` 只能二选一，先确认是发到群/会话还是发给个人。
- 修改公告前没有完整新正文：先向用户确认完整公告内容，不要只传增量片段。
