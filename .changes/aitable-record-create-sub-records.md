---
category: Added
---

- **AI Table sub-record creation via `record create`** — `dws aitable record create` now accepts `--parent-record-id` to create hierarchy sub-records under a parent record, with optional `--view-id` (view carrying the hierarchy config; the specified view is preferred and falls back to the first configured Grid view when it has no hierarchy config). Both flags simply add the optional `parentRecordId`/`viewId` properties to the same pinned `create_records` RPC — the MCP server routes to hierarchy creation internally — so the command stays a single parameter-equivalent `create_records` projection. Sub-record-only flags are rejected without `--parent-record-id`, the 100-record limit is enforced client-side, and plain creation is unchanged.
