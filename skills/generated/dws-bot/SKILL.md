---
name: dws-bot
description: "钉钉机器人消息MCP服务，支持创建企业机器人、将企业机器人添加到指定的群内、企业机器人发送群消息和单聊消息、企业机器人取消发送的群或单聊消息等能力。"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws bot --help"
---

# bot

> **PREREQUISITE:** Read `../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

- Display name: 机器人消息
- Description: 钉钉机器人消息MCP服务，支持创建企业机器人、将企业机器人添加到指定的群内、企业机器人发送群消息和单聊消息、企业机器人取消发送的群或单聊消息等能力。
- Endpoint: `https://mcp-gw.dingtalk.com/server/4717d5cbb92ecdebd89c174e4331dc17207208a97622e2004cac49c0fbedc9d1`
- Protocol: `2025-03-26`
- Degraded: `false`

```bash
dws bot <group> <command> --json '{...}'
```

## Helper Commands

| Command | Tool | Description |
|---------|------|-------------|
| [`dws-bot-group-members-add-bot`](./group/members/add-bot.md) | `add_robot_to_group` | 将自定义机器人添加到当前用户有管理权限的群聊中。如果没有权限则会报错 |
| [`dws-bot-message-recall-by-bot`](./message/recall-by-bot.md) | `batch_recall_robot_users_msg` | 批量撤回机器人发送的单聊消息。 |
| [`dws-bot-batch-send-robot-msg-to-users`](./batch-send-robot-msg-to-users.md) | `batch_send_robot_msg_to_users` | 机器人批量发送单聊消息，在该机器人可使用范围内的员工，可接收到单聊消息。 |
| [`dws-bot-create-robot`](./create-robot.md) | `create_robot` | 创建企业机器人，调用本服务会在当前组织创建一个企业内部应用并自动开启stream功能的机器人，该应用被创建时自动完成发布，默认可见范围是当前用户。 |
| [`dws-bot-message-recall-robot-group-message`](./message/recall-robot-group-message.md) | `recall_robot_group_message` | 可批量撤回企业机器人在群内发送的消息。 |
| [`dws-bot-search-groups-by-keyword`](./search-groups-by-keyword.md) | `search_groups_by_keyword` | 根据关键词搜索我的群会话信息，包含群openconversationId、群名称等信息 |
| [`dws-bot-bot-search`](./bot/search.md) | `search_my_robots` | 搜索我创建的机器人，可获取机器人robotCode等信息。 |
| [`dws-bot-message-send-by-webhook`](./message/send-by-webhook.md) | `send_message_by_custom_robot` | 使用自定义机器人发送群消息，请注意自定义机器人与企业机器人的区别。 |
| [`dws-bot-message-send-by-bot`](./message/send-by-bot.md) | `send_robot_group_message` | 机器人发送群聊消息，该机器人必须已存在对应的群内。 |

## API Tools

### `add_robot_to_group`

- Canonical path: `bot.add_robot_to_group`
- CLI route: `dws bot group members add-bot`
- Description: 将自定义机器人添加到当前用户有管理权限的群聊中。如果没有权限则会报错
- Required fields: `openConversationId`, `robotCode`
- Sensitive: `false`

### `batch_recall_robot_users_msg`

- Canonical path: `bot.batch_recall_robot_users_msg`
- CLI route: `dws bot message recall-by-bot`
- Description: 批量撤回机器人发送的单聊消息。
- Required fields: `processQueryKeys`, `robotCode`
- Sensitive: `false`

### `batch_send_robot_msg_to_users`

- Canonical path: `bot.batch_send_robot_msg_to_users`
- CLI route: `dws bot batch_send_robot_msg_to_users`
- Description: 机器人批量发送单聊消息，在该机器人可使用范围内的员工，可接收到单聊消息。
- Required fields: `markdown`, `robotCode`, `title`, `userIds`
- Sensitive: `false`

### `create_robot`

- Canonical path: `bot.create_robot`
- CLI route: `dws bot create_robot`
- Description: 创建企业机器人，调用本服务会在当前组织创建一个企业内部应用并自动开启stream功能的机器人，该应用被创建时自动完成发布，默认可见范围是当前用户。
- Required fields: `desc`, `robot_name`
- Sensitive: `false`

### `recall_robot_group_message`

- Canonical path: `bot.recall_robot_group_message`
- CLI route: `dws bot message recall_robot_group_message`
- Description: 可批量撤回企业机器人在群内发送的消息。
- Required fields: `openConversationId`, `processQueryKeys`, `robotCode`
- Sensitive: `false`

### `search_groups_by_keyword`

- Canonical path: `bot.search_groups_by_keyword`
- CLI route: `dws bot search_groups_by_keyword`
- Description: 根据关键词搜索我的群会话信息，包含群openconversationId、群名称等信息
- Required fields: `keyword`
- Sensitive: `false`

### `search_my_robots`

- Canonical path: `bot.search_my_robots`
- CLI route: `dws bot bot search`
- Description: 搜索我创建的机器人，可获取机器人robotCode等信息。
- Required fields: `currentPage`
- Sensitive: `false`

### `send_message_by_custom_robot`

- Canonical path: `bot.send_message_by_custom_robot`
- CLI route: `dws bot message send-by-webhook`
- Description: 使用自定义机器人发送群消息，请注意自定义机器人与企业机器人的区别。
- Required fields: `robotToken`, `text`, `title`
- Sensitive: `false`

### `send_robot_group_message`

- Canonical path: `bot.send_robot_group_message`
- CLI route: `dws bot message send-by-bot`
- Description: 机器人发送群聊消息，该机器人必须已存在对应的群内。
- Required fields: `markdown`, `openConversationId`, `robotCode`, `title`
- Sensitive: `false`

## Discovering Commands

```bash
dws schema                       # list available products (JSON)
dws schema bot                     # inspect product tools (JSON)
dws schema bot.<tool>              # inspect tool schema (JSON)
```
