---
category: Fixed
---

- **Reviewer Router merge authority** — moves fail-closed writer-rule and auto-merge ownership validation into the trusted base-owned Router before App credentials are read, preparing metadata-only auto-merge changes to stop restarting the full CI suite without weakening protected-main admission or exact-SHA cache production.
