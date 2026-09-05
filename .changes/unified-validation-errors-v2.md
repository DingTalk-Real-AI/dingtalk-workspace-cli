---
category: Fixed
---

- **Command validation errors** — report framework-owned parameter failures consistently as validation errors with exit code 3, while preserving API errors, explicit exit codes, cancellation, and deadlines.
- **Parent and proxy flags** — route parent traversal and wiki proxy parse failures through the same validation boundary and preserve target command hints.
