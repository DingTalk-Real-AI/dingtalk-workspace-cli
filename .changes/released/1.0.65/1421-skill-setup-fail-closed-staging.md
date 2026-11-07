---
category: Fixed
---

- **Skill setup staging fail-closed** (#1421) — path walks no longer skip read-only sibling directories behind a link entry, and staging identity read failures no longer delete by name or silently degrade to compat install, preventing leftover staging objects and false success reports during `dws skill setup`.
