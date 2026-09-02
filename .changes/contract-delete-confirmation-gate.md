---
category: Fixed
---

- **Contract command safety and Skill routing** — Contract destructive operations (`archive`, `subject delete`, `subject batch-delete`, `project delete`, and `account delete`) now require explicit user confirmation (`--yes`) before executing, with Schema Safety `confirmation=user_required`. Batch project/subject deletion rejects empty parsed ID lists, subject deletion enforces the 1000-ID service limit, and required project/subject pagination rejects non-positive values before calling MCP. Account-list execution-time filters are documented consistently as ISO-8601 CLI inputs converted to MCP milliseconds. Legal smart-contract guidance is delivered through `dingtalk-misc` instead of a standalone first-level Skill, and the retired `edu-contact` endpoint is no longer registered as a supplement server.
