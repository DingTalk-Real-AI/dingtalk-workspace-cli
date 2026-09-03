---
category: Changed
---

- **Faster Schema Catalog assembly** — projects typed values into payload JSON
  without re-running a validation scan over documents `json.Marshal` has just
  produced, cutting roughly a third of the projection work across the full tool
  set. Untrusted JSON input keeps its existing validation.
