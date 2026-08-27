---
category: Added
---

- **Drive sync batch 2** (#1086) — Five synchronized enhancements aligned with closed-source MR 28427926 / 28769810 / 28967420 / 28972632:
  - **drive quota + quota apps** (#573): `drive quota` queries enterprise storage (org/app/space levels); `drive quota apps` lists application storage usage with pagination and sorting
  - **drive task get + copy/move auto-polling** (#543, #496): unified `drive task get --type <export|import|copy|move> --id <taskId>` queries async task status via `query_task` (drive MCP); `drive copy/move` now auto-poll `query_task` when server returns `taskId` and print normalized `TaskResult` JSON on completion
  - **drive export** (#593): universal export command supporting all doc types (adoc/axls/appt) with auto-format detection, progressive-backoff polling, and optional `--async` mode; `drive export get` queries export task status
  - **publish set password/expire-days** (#584): `drive publish set` accepts `--password` (4-char alphanumeric, empty to clear) and `--expire-days` (N=days, 0=permanent); client-side validation of --permission/--password/--expire-days runs before the confirmation gate
  - **doc-whiteboard.md** (#571): added `skills/mono/references/products/doc/doc-whiteboard.md` documenting whiteboard card insertion, deletion, and post-insert verification workflow
