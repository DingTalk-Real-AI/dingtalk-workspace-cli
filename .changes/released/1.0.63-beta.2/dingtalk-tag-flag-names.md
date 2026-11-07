---
category: Changed
---

- Standardize `dingtalk-tag manage create/list/save-draft` on `--type` and `set-visibility` on `--user-ids`, keeping help and Skill guidance focused on canonical flags while preserving existing script compatibility and MCP field mappings.
- Document `set-visibility` in both mono and multi Skill references, including ALL/PARTIAL scopes, full replacement semantics, omitted-list clearing, the userId-to-staffIds mapping, and confirmation requirements.
- Send only supported fields in publish requests and keep compatibility handling silent in both preview and execution.
