---
category: Fixed
---

- **Chat message decrypt fallback** — skips crypto policy lookups and decrypt failure ledger fields on chat read paths when the DWS binary does not include the SafeChat backend, while preserving policy-driven decryption after `chat message list --page-all` aggregates its pages.
