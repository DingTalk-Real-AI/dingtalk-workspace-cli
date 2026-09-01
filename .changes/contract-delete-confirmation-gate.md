---
type: fixed
---

Contract delete commands (subject delete, subject batch-delete, project delete,
account delete) now require explicit user confirmation (`--yes`) before
executing. Schema Safety `confirmation` updated from `not_required` to
`user_required`. Removed retired `edu-contact` endpoint from supplement servers.
