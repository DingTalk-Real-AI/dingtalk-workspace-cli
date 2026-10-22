---
category: Fixed
---

- **Direct message completeness** (#1297, #1298) — `chat +messages-list-direct --page-all` preserves `partial=true` after message decryption failures and reports `paginationKnown=false` when a fetched page lacks usable `hasMore` evidence. Existing pagination errors, retained messages, and decryption failure details remain available.
