---
category: Fixed
---

- **Drive list pattern filtering** (#942) — `dws drive list --pattern` on the
  single-layer pan route now filters the returned page by name pattern; the
  flag was previously accepted but silently ignored.

- **Drive list `--type folder --latest` composition** (#942) — `--latest` now
  ranks the filtered entries (folders included when `--type folder` is set)
  instead of unconditionally dropping folders, so the documented combination
  returns the most recently modified folders rather than an empty list.
