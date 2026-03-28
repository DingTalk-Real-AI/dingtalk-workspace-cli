---
name: dws-ding
description: "DING消息."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws ding --help"
---

# ding

> **PREREQUISITE:** Read `../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

- Display name: DING消息
- Description: DING消息
- Endpoint: `https://mcp-gw.dingtalk.com/server/404106cbb828de22de78bd390e7af4b2b24ec0cdc5088440ce10a41614fa328d`
- Protocol: `2025-03-26`
- Degraded: `false`

```bash
dws ding <group> <command> --json '{...}'
```

## Helper Commands

| Command | Tool | Description |
|---------|------|-------------|
| [`dws-ding-message-recall`](./message/recall.md) | `recall_ding_message` | 撤回已发送的DING消息 |
| [`dws-ding-search-my-robots`](./search-my-robots.md) | `search_my_robots` | 搜索我创建的机器人 |
| [`dws-ding-message-send`](./message/send.md) | `send_ding_message` | 使用企业内机器人发送DING消息，可发送应用内DING、短信DING、电话DING。 |

## API Tools

### `recall_ding_message`

- Canonical path: `ding.recall_ding_message`
- CLI route: `dws ding message recall`
- Description: 撤回已发送的DING消息
- Required fields: `openDingId`, `robotCode`
- Sensitive: `false`

### `search_my_robots`

- Canonical path: `ding.search_my_robots`
- CLI route: `dws ding search_my_robots`
- Description: 搜索我创建的机器人
- Required fields: `currentPage`
- Sensitive: `false`

### `send_ding_message`

- Canonical path: `ding.send_ding_message`
- CLI route: `dws ding message send`
- Description: 使用企业内机器人发送DING消息，可发送应用内DING、短信DING、电话DING。
- Required fields: `content`, `receiverUserIdList`, `remindType`, `robotCode`
- Sensitive: `false`

## Discovering Commands

```bash
dws schema                       # list available products (JSON)
dws schema ding                     # inspect product tools (JSON)
dws schema ding.<tool>              # inspect tool schema (JSON)
```
