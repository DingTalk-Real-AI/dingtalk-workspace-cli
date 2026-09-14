---
category: Fixed
---

- **Chat user mentions** — preserves literal `<@openDingTalkId>` tokens in current-user Markdown messages and rejects mismatches between message-body mentions and mention flags before sending.
- **Chat direct media** — uses the IM upload target field for current-user direct file, audio, and video uploads, then uses the Chat receiver field for final message delivery.
