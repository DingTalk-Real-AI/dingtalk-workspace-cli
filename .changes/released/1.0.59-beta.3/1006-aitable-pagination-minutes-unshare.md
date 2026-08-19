---
category: Fixed
---

- **Aitable pagination and Minutes unshare verification** (#1006) — keeps
  record queries on the service's 20-record page boundary so multi-page reads
  and mutation readbacks no longer report false retryable failures, preserves
  `totalCount` when supplied, validates `--dry-run` plans before transport,
  follows active deletion readback continuations before proving absence, and
  rejects Minutes unshare success until the listening note exists and the
  service acknowledges the exact task and member targets.
