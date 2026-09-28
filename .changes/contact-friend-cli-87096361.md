---
category: Added
---

- **Contact friend CLI** — adds `dws contact +friend-list`, `+friend-request-list`, `+friend-request-send`, `+friend-request-accept`, `+friend-request-reject`, and `+friend-remove` shortcuts for the friend-link MCP tools published under `mcpId=2400`.
- **Friend commands now use openDingTalkId** — all friend shortcuts accept and return `openDingTalkId` (the open-platform pairwise identifier used in friend event payloads) instead of `dingtalkId`.
- **Contact user lookup by openDingTalkId** — adds `dws contact user get-by-open-dingtalk-id --id <openDingTalkId>` (alias `search-open-dingtalk`) to retrieve a user's `userId` from their openDingTalkId.
