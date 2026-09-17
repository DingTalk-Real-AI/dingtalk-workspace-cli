---
category: Fixed
---

- **Post-merge CI admission reuse** — reuses exact successful full-suite
  PR evidence for tree-identical protected-main merges and promotes the
  verified coverage artifact to the merge SHA cache, reducing duplicate runner
  work without adding jobs or weakening required contexts.
