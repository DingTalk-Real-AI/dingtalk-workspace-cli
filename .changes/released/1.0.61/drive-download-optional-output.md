---
category: Changed
---

- **Drive download optional output** — `dws drive download` and `dws drive download-version` no longer require `--output`: when omitted, files are saved to the current directory with the filename inferred from the response `fileName` (falling back to the download URL); explicit `--output` behavior (file path or directory) is unchanged.
