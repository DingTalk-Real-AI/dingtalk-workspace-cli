---
name: dws-group-chat
description: "钉钉群聊MCP支持创建内部群、搜索群会话、管理群成员、修改群名称及查询话题回复等群聊管理能力。"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws chat --help"
---

# group-chat

> **PREREQUISITE:** Read `../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

- Display name: 钉钉群聊
- Description: 钉钉群聊MCP支持创建内部群、搜索群会话、管理群成员、修改群名称及查询话题回复等群聊管理能力。
- Endpoint: `https://mcp-gw.dingtalk.com/server/0a1609437385696b77fc4771c3ddaf5656b487f809966c0cc8d4755e7b1d3b74`
- Protocol: `2025-03-26`
- Degraded: `false`

```bash
dws chat <group> <command> --json '{...}'
```

## Helper Commands

| Command | Tool | Description |
|---------|------|-------------|
| [`dws-group-chat-group-members-add`](./group/members/add.md) | `add_group_member` | 添加群成员 |
| [`dws-group-chat-group-create`](./group/create.md) | `create_internal_group` | 创建一个组织内部的群聊（仅限本企业成员加入），支持指定群名称、初始成员列表等参数。操作成功后返回是否创建成功的结果。群聊创建受组织安全策略限制（如成员必须属于同一企业）。适用于项目协作、临时沟通等需要快速建群的场景。 |
| [`dws-group-chat-create-internal-org-group`](./create-internal-org-group.md) | `create_internal_org_group` | 创建一个组织内部的群聊（仅限本企业成员加入），支持指定群名称、初始成员列表等参数。操作成功后返回是否创建成功的结果。群聊创建受组织安全策略限制（如成员必须属于同一企业）。适用于项目协作、临时沟通等需要快速建群的场景。 |
| [`dws-group-chat-group-members-list`](./group/members/list.md) | `get_group_members` | 查群成员列表 |
| [`dws-group-chat-list-conversation-message`](./list-conversation-message.md) | `list_conversation_message` | 已废弃！！！！拉取指定单聊或群聊的会话消息内容 |
| [`dws-group-chat-list-conversation-message-v2`](./list-conversation-message-v2.md) | `list_conversation_message_v2` | 拉取指定群聊的会话消息内容 |
| [`dws-group-chat-list-individual-chat-message`](./list-individual-chat-message.md) | `list_individual_chat_message` | 拉取指定用户的单聊会话消息内容 |
| [`dws-group-chat-message-list-topic-replies`](./message/list-topic-replies.md) | `list_topic_replies` | 针对话题群中的单个话题，分页拉取话题的回复消息 |
| [`dws-group-chat-group-members-remove`](./group/members/remove.md) | `remove_group_member` | 移除群成员 |
| [`dws-group-chat-search`](./search.md) | `search_groups_by_keyword` | 根据群名称关键词，搜索符合条件的群，返回群的openconversion_id、群名称等信息 |
| [`dws-group-chat-send-direct-message-as-user`](./send-direct-message-as-user.md) | `send_direct_message_as_user` | 以当前用户的身份给某用户发送单聊消息。 |
| [`dws-group-chat-send-message-as-user`](./send-message-as-user.md) | `send_message_as_user` | 以当前用户的身份发送群消息 |
| [`dws-group-chat-group-rename`](./group/rename.md) | `update_group_name` | 更新群名称 |

## API Tools

### `add_group_member`

- Canonical path: `group-chat.add_group_member`
- CLI route: `dws chat group members add`
- Description: 添加群成员
- Required fields: `openconversation_id`, `userId`
- Sensitive: `false`

### `create_internal_group`

- Canonical path: `group-chat.create_internal_group`
- CLI route: `dws chat group create`
- Description: 创建一个组织内部的群聊（仅限本企业成员加入），支持指定群名称、初始成员列表等参数。操作成功后返回是否创建成功的结果。群聊创建受组织安全策略限制（如成员必须属于同一企业）。适用于项目协作、临时沟通等需要快速建群的场景。
- Required fields: `groupMembers`, `groupName`
- Sensitive: `false`

### `create_internal_org_group`

- Canonical path: `group-chat.create_internal_org_group`
- CLI route: `dws chat create_internal_org_group`
- Description: 创建一个组织内部的群聊（仅限本企业成员加入），支持指定群名称、初始成员列表等参数。操作成功后返回是否创建成功的结果。群聊创建受组织安全策略限制（如成员必须属于同一企业）。适用于项目协作、临时沟通等需要快速建群的场景。
- Required fields: `groupMembers`, `groupName`
- Sensitive: `false`

### `get_group_members`

- Canonical path: `group-chat.get_group_members`
- CLI route: `dws chat group members list`
- Description: 查群成员列表
- Required fields: `openconversation_id`
- Sensitive: `false`

### `list_conversation_message`

- Canonical path: `group-chat.list_conversation_message`
- CLI route: `dws chat list_conversation_message`
- Description: 已废弃！！！！拉取指定单聊或群聊的会话消息内容
- Required fields: `openconversation_id`
- Sensitive: `false`

### `list_conversation_message_v2`

- Canonical path: `group-chat.list_conversation_message_v2`
- CLI route: `dws chat list_conversation_message_v2`
- Description: 拉取指定群聊的会话消息内容
- Required fields: `forward`, `openconversation_id`, `time`
- Sensitive: `false`

### `list_individual_chat_message`

- Canonical path: `group-chat.list_individual_chat_message`
- CLI route: `dws chat list_individual_chat_message`
- Description: 拉取指定用户的单聊会话消息内容
- Required fields: `forward`, `time`, `userId`
- Sensitive: `false`

### `list_topic_replies`

- Canonical path: `group-chat.list_topic_replies`
- CLI route: `dws chat message list-topic-replies`
- Description: 针对话题群中的单个话题，分页拉取话题的回复消息
- Required fields: `openconversationId`, `topicId`
- Sensitive: `false`

### `remove_group_member`

- Canonical path: `group-chat.remove_group_member`
- CLI route: `dws chat group members remove`
- Description: 移除群成员
- Required fields: `openconversationId`, `userIdList`
- Sensitive: `true`

### `search_groups_by_keyword`

- Canonical path: `group-chat.search_groups_by_keyword`
- CLI route: `dws chat search`
- Description: 根据群名称关键词，搜索符合条件的群，返回群的openconversion_id、群名称等信息
- Required fields: `OpenSearchRequest.query`
- Sensitive: `false`

### `send_direct_message_as_user`

- Canonical path: `group-chat.send_direct_message_as_user`
- CLI route: `dws chat send_direct_message_as_user`
- Description: 以当前用户的身份给某用户发送单聊消息。
- Required fields: `receiverUserId`, `text`, `title`
- Sensitive: `false`

### `send_message_as_user`

- Canonical path: `group-chat.send_message_as_user`
- CLI route: `dws chat send_message_as_user`
- Description: 以当前用户的身份发送群消息
- Required fields: `openConversation_id`, `text`, `title`
- Sensitive: `false`

### `update_group_name`

- Canonical path: `group-chat.update_group_name`
- CLI route: `dws chat group rename`
- Description: 更新群名称
- Required fields: `group_name`, `openconversation_id`
- Sensitive: `false`

## Discovering Commands

```bash
dws schema                       # list available products (JSON)
dws schema group-chat                     # inspect product tools (JSON)
dws schema group-chat.<tool>              # inspect tool schema (JSON)
```
