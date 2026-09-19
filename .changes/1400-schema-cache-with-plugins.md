---
category: Fixed
---

- **Persistent Schema cache with plugins** (#1400) — delegates Schema cache assembly and repair to an isolated child process when plugins are present, keeping persistent cache active without inheriting plugin runtime side effects.
