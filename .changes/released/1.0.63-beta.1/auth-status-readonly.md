---
category: Added
---

- **Auth status read-only snapshot** — `dws auth status --readonly` observes the local credential snapshot without the authentication lock, network validation, refresh, migration, or credential writes, keeping the same output fields as normal status and reporting inconclusive local state through explicit `reason` codes instead of a confirmed logout.
