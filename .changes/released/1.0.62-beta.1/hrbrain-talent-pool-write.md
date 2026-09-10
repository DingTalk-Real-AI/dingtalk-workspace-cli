---
category: Added
---

- **hrbrain talent-pool save** — creates or updates a talent pool. Omit `--pool-code` to create a new pool (only `--pool-name` is required) or pass `--pool-code` to update an existing one; optional `--pool-desc`, `--rule-json` (auto in/out rule, validated as a JSON object), and `--pool-tags` (validated as a non-empty JSON array) are forwarded to the `create_or_update_pool` MCP tool. The write is gated by a confirmation prompt (`--yes` to skip).
- **hrbrain talent-pool move-members** — batch-moves staff into or out of a talent pool via the `entering_or_leaving_pool` MCP tool. Requires `--pool-code`, `--opt-type` (`ENTERING`/`LEAVING`), and `--staff-ids` (comma-separated work numbers), with an optional `--remark`. The write is gated by a confirmation prompt (`--yes` to skip).
