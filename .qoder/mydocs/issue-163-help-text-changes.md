# Issue #163: `dws chat message send --help` 修改前后对比

> 关联 Issue: https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/issues/163
> 修改日期: 2026-04-30
> 分支: fix/issue-163-msg-type

---

## 修改前

```
以当前用户身份发送群消息或单聊消息。

--group 指定群聊 openConversationId 发群消息；--user 指定 userId 发单聊；
--open-dingtalk-id 指定 openDingTalkId 发单聊 (适用于无法获取 userId 的场景)。
三者只能选其一，不能同时指定。

消息内容通过 --text 传入，也可作为位置参数；支持 Markdown。必须提供 --title 作为消息标题。

群聊场景下可用 --at-all / --at-users / --at-mobiles 进行 @ 提醒（仅 --group 时生效）。
注意 --text 中需包含对应的 <@userId> / <@all> 占位符才能在客户端渲染出 @ 效果。

Usage:
  dws chat message send [flags]

Examples:
  dws chat message send --group <openconversation_id> --text "hello"
  dws chat message send --user <userId> --text "请查收"
  dws chat message send --open-dingtalk-id <openDingTalkId> --title "提醒" --text "请确认"
  dws chat message send --group <openconversation_id> --title "拉群通知" --text "<@uid> 你被 @ 了" --at-users uid

Flags:
      --at-all                    @所有人 (仅 --group 群聊生效)
      --at-mobiles string         按手机号 @ 指定成员，逗号分隔 (仅 --group 群聊生效)
      --at-users string           按 userId @ 指定成员，逗号分隔 (仅 --group 群聊生效)
      --group string              群会话 openConversationId (群聊三选一)
  -h, --help                      help for send
      --open-dingtalk-id string   接收人 openDingTalkId (单聊三选一)
      --text string               消息内容，支持 Markdown (也可作位置参数)
      --title string              消息标题 (可选)
      --user string               接收人 userId (单聊三选一)
```

## 修改后

```
以当前用户身份发送群消息或单聊消息。

--group 指定群聊 openConversationId 发群消息；--user 指定 userId 发单聊；
--open-dingtalk-id 指定 openDingTalkId 发单聊 (适用于无法获取 userId 的场景)。
三者只能选其一，不能同时指定。

消息内容通过 --text 传入，支持 Markdown，也可作为位置参数。

消息类型:
  不传 --title 时，发送纯文本消息，仅渲染 --text 内容。
  传了 --title 时，发送卡片(actionCard)消息，title 为卡片标题，text 为正文。
  可通过 --msg-type 显式指定消息类型 (text / markdown / actionCard)。

群聊场景下可用 --at-all / --at-users / --at-mobiles 进行 @ 提醒（仅 --group 时生效）。
注意 --text 中需包含对应的 <@userId> / <@all> 占位符才能在客户端渲染出 @ 效果。

Usage:
  dws chat message send [flags]

Examples:
  dws chat message send --group <openconversation_id> --text "hello"
  dws chat message send --user <userId> --text "请查收"
  dws chat message send --open-dingtalk-id <openDingTalkId> --title "提醒" --text "请确认"
  dws chat message send --group <openconversation_id> --title "拉群通知" --text "<@uid> 你被 @ 了" --at-users uid

Flags:
      --at-all                    @所有人 (仅 --group 群聊生效)
      --at-mobiles string         按手机号 @ 指定成员，逗号分隔 (仅 --group 群聊生效)
      --at-users string           按 userId @ 指定成员，逗号分隔 (仅 --group 群聊生效)
      --group string              群会话 openConversationId (群聊三选一)
  -h, --help                      help for send
      --msg-type string           消息类型: text(纯文本) / markdown / actionCard(卡片，需配合 --title)
      --open-dingtalk-id string   接收人 openDingTalkId (单聊三选一)
      --text string               消息内容，支持 Markdown (也可作位置参数)
      --title string              消息标题 (可选)
      --user string               接收人 userId (单聊三选一)
```

## 差异摘要

| 区域 | 修改前 | 修改后 |
|------|--------|--------|
| Long 描述 | "消息内容通过 --text 传入，也可作为位置参数；支持 Markdown。必须提供 --title 作为消息标题。" | 移除"必须提供 --title"的错误说明，新增"消息类型"段落，解释 title 与 msgType 的关系 |
| --msg-type flag | 不存在 | 新增 `--msg-type string  消息类型: text(纯文本) / markdown / actionCard(卡片，需配合 --title)` |
| 行为变化（代码层） | 不传 msgType，服务端默认 actionCard → title/text 重复渲染 + 底部空白 | 传 title → 显式 msgType="actionCard"；不传 title → 不设置 msgType（保持原行为，群聊 API 不支持纯 text）；传 --msg-type → 用户值优先 |

## 新增测试用例

| 测试函数 | 场景 | 预期 |
|----------|------|------|
| `TestChatMessageSend_NoTitle_NoMsgType` | 只传 --text | params 中不含 msgType（群聊 API 不支持 text 类型） |
| `TestChatMessageSend_WithTitle_SetsActionCard` | 传 --title + --text | params["msgType"] == "actionCard" |
| `TestChatMessageSend_ExplicitMsgType_Overrides` | 传 --msg-type markdown | params["msgType"] == "markdown" |
| `TestChatMessageSend_ExplicitMsgTypeWithTitle` | 传 --msg-type markdown + --title | params["msgType"] == "markdown"，title 透传 |

## 补强现有测试

`TestChatMessageSendForwardsAtMentions` 的三个用例（group-with-at-users / group-with-at-all / group-with-at-mobiles）均在 wantParams 中新增 `"msgType": "actionCard"` 断言。
