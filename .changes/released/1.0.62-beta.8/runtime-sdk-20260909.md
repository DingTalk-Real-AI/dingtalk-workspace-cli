---
category: Changed
---

- Update the embedded runtime SDK to 20260909 and remove auxiliary ps files from builds and runtime extraction, reducing single-binary package size.
- Refresh the macOS universal runtime library while retaining the 1 MiB payload slot and using the payload digest to upgrade existing installations of the same resource version.
