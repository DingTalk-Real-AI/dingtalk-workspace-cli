---
category: Changed
---

- **Drive download overwrite guard** — `dws drive download` and `dws drive download-version` now reject downloads when the target file already exists, returning a structured `INPUT_FILE_ALREADY_EXISTS` error with recovery guidance; pass `--overwrite` to proceed. Re-running the same download used to silently overwrite the existing file. The guard is enforced both before the transfer starts and atomically at publish time (no-replace link), so a file that appears during a long download is never silently overwritten. Resume artifacts (`.dwspart`/`.dwspart.meta`) are not treated as conflicts.
