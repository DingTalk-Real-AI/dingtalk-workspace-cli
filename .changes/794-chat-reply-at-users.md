---
category: Added
---

- **Chat reply `--at-users`** (#794) — `dws chat message reply` accepts comma-separated userId or openDingTalkId values, matching the `--at-users` flag `chat message send-by-webhook` already exposes. User IDs are resolved through the existing contact lookup, matching `@` placeholders in the reply body are normalized to `<@openDingTalkId>`, and the resolved list is folded into `--at-open-dingtalk-ids` so both flags share one mention path.
