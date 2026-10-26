---
category: Added
---

- Add `dws aitable datasource plugin-create` to create an AI Minutes (`minutes`) plugin datasource table and trigger its first sync. The command is non-idempotent; unknown outcomes fail with `create_outcome_unknown` and must be verified with `table get` before any retry.
