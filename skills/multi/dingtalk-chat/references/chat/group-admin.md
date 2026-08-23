# group-admin：群创建、成员写入与管理

> 返回入口：[DingTalk Chat Skill](../../SKILL.md)

用于建群、修改群资料、成员增删、邀请卡片分享、群主和管理员、禁言、公告、群设置、
入群审批、群身份、退出、解散和升级外部群。只读群发现、成员读取和邀请链接使用
[group-discovery.md](group-discovery.md)。

## 安全与目标

- 群目标统一使用当前 profile 下真实 `openConversationId`；支持自然群名的 Shortcut 由 CLI
  唯一解析，多候选时停止。
- 解散群、踢人、转让群主、禁言、管理员和外部群升级都是高影响操作；以最终 Runtime gate
  和精确 leaf Schema 为准确认对象、动作与影响。
- 所有自然成员和群主必须先完成唯一解析并按稳定 ID 去重，再开始任何写入；不得边解析边
  产生部分副作用。
- 群公告会触达成员；`notice edit` 是整体替换，必须有完整新正文。

## 建群与基础资料

<!-- dws-intent: chat.create.group -->基础建群使用 `dws chat +chat-create`。已知成员 ID 传 `--users`，
姓名/花名传 `--member-query`；群主默认当前用户，也可传 `--owner-open-dingtalk-id` 或
`--owner-query`。任一自然身份未唯一解析时，创建前整体停止。

```bash
dws chat +chat-create --name "项目冲刺群" --member-query "测试用户甲,测试用户乙" --format json
dws chat +chat-create --name "合作群" --member-query "测试用户甲" \
  --owner-query "测试用户乙" --type EXTERNAL --format json
```

修改群名称优先使用接受群名或稳定 ID 的 `+chat-update`：

```bash
dws chat +chat-update --group <群名或openConversationId> --name "新群名" --format json
```

群头像和管理员级群开关使用 `+chat-update-icon`、`+chat-update-settings`；只有 Shortcut
尚未发布真实必需字段时才评估原子 `group rename/update-icon/update-settings`。

原子 `chat group create` 只用于 `+chat-create` 未发布的真实底层字段，并先读取精确 leaf
Schema。普通内部/外部群、话题群和显式群主已经由 `+chat-create` 覆盖，不回流到手工
`aisearch → group create` 链路。

## 成员与机器人写入

| 动作 | 入口与关键参数 |
|---|---|
| 添加成员 | `group members add --id <cid> --users <userIds>` |
| 移除成员 | `group members remove --id <cid> --users <userIds>` |
| 添加已知机器人 | `+chat-add-bot` 或精确原子 `group members add-bot` |
| 查看群内机器人 | `+chat-bots --group <群名或cid>` |
| 移除群内机器人 | `+chat-remove-bot` 或精确原子 `group members remove-bot` |

普通成员增删的 `--users` 只接受组织 `userId`，必须来自真实人员解析结果；不得把
`+chat-members-list` / `+chat-members-get` 返回的 `openDingTalkId` 直接传入。添加已知机器人
使用 `robotCode`；移除机器人使用当前群 `+chat-bots` 返回的真实 `openBotId`，两者不能互换。
缺少 `openBotId` 时在同一流程中先执行 `+chat-bots`，不必额外读取群发现 reference。只有需要
搜索未知机器人、区分 `bot search` / `bot find`、机器人发送或撤回、Webhook 时，才读取
[chat-bot.md](chat-bot.md)。

## 邀请卡片、群主、管理员与禁言

邀请链接只读走 `+chat-invite-url`。实际分享邀请卡片使用 `group share-invite`：`--source`
是被分享群，接收端在 `--target` 会话和 `--receiver` 单聊用户之间二选一。

```bash
dws chat group share-invite --source <sourceCid> --target <targetCid> --format json
dws chat group share-invite --source <sourceCid> --receiver <openDingTalkId> --format json
```

| 动作 | 入口与关键参数 |
|---|---|
| 转让群主 | `+chat-transfer-owner --group <cid> --new-owner <稳定ID>` |
| 设置/取消管理员 | `group set-admin --group <cid> --users <ids> [--off]` |
| 全员禁言/解除 | `group-mute --group <cid> [--off]` |
| 成员禁言/解除 | `+chat-mute-member` 或 `group-mute-member` |
| 查询禁言配置 | `group get-mute-config --group <cid>` |

原子 `group-mute-member --mute-time` 单位为毫秒。不要用展示名称代替稳定用户 ID，也不要
在未确认影响时执行转让、踢人或禁言。

## 群设置与当前用户偏好

管理员级群开关使用 `+chat-update-settings` 或原子 `group update-settings`。常见 settingKey
包括 `authority`、`joinValidation`、`onlyAdminCanAtAll`、`searchable`、
`addFriendForbidden`、`onlyAdminCanDING`、`onlyAdminCanPinMsg` 和
`onlyAdminCanSendFile`、`groupEmailDisabled`、`groupLiveAuthority`、
`groupBillAuthority`；只修改用户明确要求的字段。

新成员历史消息可见范围使用 `group set-history --group <cid> --option <值>`；`option` 只取
精确 leaf Schema 发布值，不按自然语言猜枚举。

当前登录用户自己的置顶、免打扰、群昵称和群备注使用 `group user-settings query/set`，
不是管理员群开关。单个群昵称/备注优先 `group update-nick/update-alias`。

```bash
dws chat group user-settings query --groups <cid1>,<cid2> --format json
dws chat group user-settings set \
  --items '[{"openConversationId":"cid1","top":true,"mute":false}]' --format json
```

批量设置只传本次要改的字段；空字符串清除昵称或备注，不补用户未要求的值。

## 群公告

| 动作 | 原子入口 |
|---|---|
| 发布公告 | `group notice create --group <cid> --content <完整Markdown>` |
| 修改公告 | `group notice edit --group <cid> --notice-id <id> --content <完整Markdown>` |
| 查询公告 | `group notice get/list` |

定时公告 `--run-at` 使用带时区时间；`notice list --scheduled` 查询待发布公告。分页时沿真实
`nextPageCursor` 继续。修改前必须取得完整替换正文，不把增量片段当整篇公告。

## 入群审批与群身份

先用 `group list-join-validations` 取得真实 `record-id/applicant/inviter`，再执行
`group audit-join-validation` 或 `+chat-audit-join`。审批状态只使用精确 leaf Schema 发布值。

群身份使用 `group-role` / `+chat-role-*`：

- `list/add/update/remove` 管理身份定义；
- `set-user/remove-user/query-user` 管理成员身份；
- `openRoleId` 必须来自真实身份列表。

覆盖或清除成员身份前确认用户、群和完整角色集合，不能用展示名称猜 `openRoleId`。

## 退出、解散与外部群升级

- 当前用户退出群：`+chat-quit` 或精确原子 `group quit`。
- 解散群：`group dismiss`，不可逆。
- 普通群升级外部群：`group upgrade-to-external`，不可逆。

这些动作必须以最终 Runtime gate 为准，不把示例中的确认参数当固定事实。

## 完成与错误

- 创建或更新后保留真实 `openConversationId` 和任务结果；只对查询结果真实返回的字段执行读回验证。
- 写接口成功但现有查询未返回目标设置时，报告真实写入回执和不可独立读回的边界；不用群名、
  成员数等其他字段代替验证，也不猜未发布的读回命令。
- 任一自然目标零命中或多候选时，在写入前整体停止。
- 逐项写入保留 succeeded/failed/unknown ledger，不用重试抹掉失败项。
- 分享邀请时 `--target` 与 `--receiver` 只能二选一；接收对象不明确时先确认。
- 机器人进群失败时确认机器人身份和当前用户管理权限，不连续切换同义原子命令。
