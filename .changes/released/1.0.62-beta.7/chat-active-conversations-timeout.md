---
category: Fixed
---

- **Chat recent conversations pagination** — adds a shared `--total-timeout` budget (default 300 seconds, range 1–3600) across all requests, retries and page delays in one invocation. A timeout preserves validated pages in the partial-failure result and returns the failed page's input cursor. Cursor continuation remains caller-managed: reuse the same profile, time window and page size, and merge batches by conversation ID. Progress is not persisted to disk; forcibly terminated queries must be restarted.
