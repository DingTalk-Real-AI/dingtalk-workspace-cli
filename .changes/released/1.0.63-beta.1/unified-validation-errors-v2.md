---
category: Fixed
---

- **Command validation errors** — report framework-owned parameter failures consistently as validation errors with exit code 3, while preserving API errors, explicit exit codes, cancellation, and deadlines.
- **Parent and proxy flags** — route parent traversal and wiki proxy parse failures through the same validation boundary and preserve target command hints.

- **Required flag wording** — framework-prepared commands now report `missing required flag(s): --name` instead of Cobra’s `required flag(s) "name" not set`. The original Cobra error remains available as the cause.

- **Generated commands** — normalize native validation failures in lazily created help/completion commands, and remove duplicate required/group checks while retaining business hook order.
