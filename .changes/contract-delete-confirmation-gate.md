---
category: Fixed
---

- **Contract command safety and Skill routing** — Contract delete commands (`subject delete`, `subject batch-delete`, `project delete`, and `account delete`) now require explicit user confirmation (`--yes`) before executing, with Schema Safety `confirmation=user_required`. Legal smart-contract guidance is delivered through `dingtalk-misc` instead of a standalone first-level Skill, and the retired `edu-contact` endpoint is no longer registered as a supplement server.
