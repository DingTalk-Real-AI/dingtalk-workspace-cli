# Canonical Product: bot

Generated from shared Tool IR. Do not edit by hand.

- Display name: 机器人消息
- Description: 钉钉机器人消息MCP服务，支持创建企业机器人、将企业机器人添加到指定的群内、企业机器人发送群消息和单聊消息、企业机器人取消发送的群或单聊消息等能力。
- Server key: `3303015f1832b28d`
- Endpoint: `https://mcp-gw.dingtalk.com/server/4717d5cbb92ecdebd89c174e4331dc17207208a97622e2004cac49c0fbedc9d1`
- Protocol: `2025-03-26`
- Degraded: `false`

## Tools

- `group members add-bot`
  - Path: `bot.add_robot_to_group`
  - CLI route: `dws bot group members add-bot`
  - Description: 将自定义机器人添加到当前用户有管理权限的群聊中。如果没有权限则会报错
  - Flags: `--id`, `--robot-code`
  - Schema: `skills/generated/docs/schema/bot/add_robot_to_group.json`
- `message recall-by-bot`
  - Path: `bot.batch_recall_robot_users_msg`
  - CLI route: `dws bot message recall-by-bot`
  - Description: 批量撤回机器人发送的单聊消息。
  - Flags: `--keys`, `--robot-code`
  - Schema: `skills/generated/docs/schema/bot/batch_recall_robot_users_msg.json`
- `batch_send_robot_msg_to_users`
  - Path: `bot.batch_send_robot_msg_to_users`
  - CLI route: `dws bot batch_send_robot_msg_to_users`
  - Description: 机器人批量发送单聊消息，在该机器人可使用范围内的员工，可接收到单聊消息。
  - Flags: `--markdown`, `--robotCode`, `--title`, `--userIds`
  - Schema: `skills/generated/docs/schema/bot/batch_send_robot_msg_to_users.json`
- `create_robot`
  - Path: `bot.create_robot`
  - CLI route: `dws bot create_robot`
  - Description: 创建企业机器人，调用本服务会在当前组织创建一个企业内部应用并自动开启stream功能的机器人，该应用被创建时自动完成发布，默认可见范围是当前用户。
  - Flags: `--desc`, `--robot-name`
  - Schema: `skills/generated/docs/schema/bot/create_robot.json`
- `message recall_robot_group_message`
  - Path: `bot.recall_robot_group_message`
  - CLI route: `dws bot message recall_robot_group_message`
  - Description: 可批量撤回企业机器人在群内发送的消息。
  - Flags: `--group`, `--keys`, `--robot-code`
  - Schema: `skills/generated/docs/schema/bot/recall_robot_group_message.json`
- `search_groups_by_keyword`
  - Path: `bot.search_groups_by_keyword`
  - CLI route: `dws bot search_groups_by_keyword`
  - Description: 根据关键词搜索我的群会话信息，包含群openconversationId、群名称等信息
  - Flags: `--cursor`, `--keyword`
  - Schema: `skills/generated/docs/schema/bot/search_groups_by_keyword.json`
- `bot search`
  - Path: `bot.search_my_robots`
  - CLI route: `dws bot bot search`
  - Description: 搜索我创建的机器人，可获取机器人robotCode等信息。
  - Flags: `--page`, `--size`, `--name`
  - Schema: `skills/generated/docs/schema/bot/search_my_robots.json`
- `message send-by-webhook`
  - Path: `bot.send_message_by_custom_robot`
  - CLI route: `dws bot message send-by-webhook`
  - Description: 使用自定义机器人发送群消息，请注意自定义机器人与企业机器人的区别。
  - Flags: `--at-mobiles`, `--at-users`, `--at-all`, `--token`, `--text`, `--title`
  - Schema: `skills/generated/docs/schema/bot/send_message_by_custom_robot.json`
- `message send-by-bot`
  - Path: `bot.send_robot_group_message`
  - CLI route: `dws bot message send-by-bot`
  - Description: 机器人发送群聊消息，该机器人必须已存在对应的群内。
  - Flags: `--text`, `--group`, `--robot-code`, `--title`
  - Schema: `skills/generated/docs/schema/bot/send_robot_group_message.json`
