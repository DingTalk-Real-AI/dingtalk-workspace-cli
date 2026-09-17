---
category: Added
---

- **Chat message list page-all** — `dws chat message list` now accepts `--page-all` to iterate the time-boundary pagination automatically and return one merged `messages` array (with `pagesFetched`, `stopReason`, `nextPage`, and per-page failure diagnostics). `--page-limit` (default 50), `--max-items`, and `--page-delay` tune the sweep; without `--page-all` the command keeps its exact single-page behavior.
