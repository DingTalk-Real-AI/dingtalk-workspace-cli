---
category: Fixed
---

- **Contract command safety and Skill routing** — Contract delete commands (`subject delete`, `subject batch-delete`, `project delete`, and `account delete`) now require explicit user confirmation (`--yes`) before executing, with Schema Safety `confirmation=user_required`. Batch project/subject deletion rejects empty parsed ID lists, and subject deletion enforces the 1000-ID service limit before calling MCP. Legal smart-contract guidance is delivered through `dingtalk-misc` instead of a standalone first-level Skill, and the retired `edu-contact` endpoint is no longer registered as a supplement server.
