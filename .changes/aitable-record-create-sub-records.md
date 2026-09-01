---
category: Added
---

- **AI Table sub-record creation via `record create`** — `dws aitable record create` now accepts `--parent-record-id` to create hierarchy sub-records under a parent record (dispatching to the `create_sub_records` MCP tool), with optional `--view-id` (view carrying the hierarchy config); sub-record-only flags are rejected without `--parent-record-id`, the 100-record limit is enforced client-side, and plain creation remains a pinned `create_records` projection with unchanged behavior.
