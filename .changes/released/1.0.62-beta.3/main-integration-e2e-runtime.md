---
category: Fixed
---

- **Main integration reliability** — keeps main and release multi-profile E2E
  validation focused on the isolated profile chain while existing CI shards own
  the complete Go regressions, avoiding the long-lived runner shutdown window
  without adding CI jobs.
