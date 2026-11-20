---
category: Fixed
---

- **CSV export argument errors** — `sheet export-csv` now classifies a missing `--node` or invalid `--value-render-option` as validation (exit code 3) instead of an internal failure (exit code 5). Invalid arguments still stop before any request or CSV output.
