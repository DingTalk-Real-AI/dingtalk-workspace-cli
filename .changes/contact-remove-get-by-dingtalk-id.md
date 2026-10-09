---
category: Removed
---

- **Retire `contact user get-by-dingtalk-id`** — the `dws contact user get-by-dingtalk-id` command (and its `search-dingtalk` alias) backed by the `get_user_id_by_dingtalk_id` MCP tool is now hidden and fails closed with `command_retired`; it no longer appears in help or the Agent Schema. To resolve a userId, use `dws aisearch person` (name/keyword lookup), `dws contact user search-mobile --mobile "<mobile>"` (exact mobile lookup), or `dws contact user get-by-open-dingtalk-id --id <openDingTalkId>` when starting from a friend event payload's openDingTalkId. Contact skill documents no longer route through the retired command.
