---
category: Fixed
---

- **AI Table composite verification** — accepts the service's real `newRecordIds`, view-filter, and workflow-detail response shapes, and retries only idempotent table-copy read-backs so delayed visibility no longer reports a false partial success.
- **Workflow deployment status reporting** — replaces `resolved.enable` with `resolved.enableRequested`; `verification.running` now reports the workflow's observed remote state instead of mirroring whether `--enable` was requested.
