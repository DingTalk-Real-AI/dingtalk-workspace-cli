---
category: Added
---

- **Sheet SourceRange dropdowns** — supports range-backed dropdowns across direct, cell, and batch write paths, with structured readback for valid and invalid references. Batch `set-dropdown` now rejects unsupported top-level `colors` / `source-colors`; Inline colors belong in `options[].color`, while SourceRange color writes remain unsupported.
- **Sheet read completion metadata** — documents and preserves returned ranges, truncation reasons, and partial-read status for large range and CSV reads.
