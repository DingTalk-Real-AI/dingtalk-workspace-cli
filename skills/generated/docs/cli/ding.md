# Canonical Product: ding

Generated from shared Tool IR. Do not edit by hand.

- Display name: DING消息
- Description: DING消息
- Server key: `9d39ee2c7636f32c`
- Endpoint: `https://mcp-gw.dingtalk.com/server/404106cbb828de22de78bd390e7af4b2b24ec0cdc5088440ce10a41614fa328d`
- Protocol: `2025-03-26`
- Degraded: `false`

## Tools

- `message recall`
  - Path: `ding.recall_ding_message`
  - CLI route: `dws ding message recall`
  - Description: 撤回已发送的DING消息
  - Flags: `--id`, `--robot-code`
  - Schema: `skills/generated/docs/schema/ding/recall_ding_message.json`
- `search_my_robots`
  - Path: `ding.search_my_robots`
  - CLI route: `dws ding search_my_robots`
  - Description: 搜索我创建的机器人
  - Flags: `--currentPage`, `--pageSize`, `--robotName`
  - Schema: `skills/generated/docs/schema/ding/search_my_robots.json`
- `message send`
  - Path: `ding.send_ding_message`
  - CLI route: `dws ding message send`
  - Description: 使用企业内机器人发送DING消息，可发送应用内DING、短信DING、电话DING。
  - Flags: `--content`, `--users`, `--type`, `--robot-code`
  - Schema: `skills/generated/docs/schema/ding/send_ding_message.json`
