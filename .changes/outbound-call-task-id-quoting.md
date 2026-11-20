---
category: Fixed
---

- **Outbound-call follow-up commands**: Fix task ID quoting in `meta.operation.next_command` so safe IDs work across shells. On Windows, values that cannot be safely inlined use a `TASK_ID` placeholder while the original ID remains available in `meta.operation.id` and the business receipt.
