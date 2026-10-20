---
category: Fixed
---

- **升级版本比较** (#556) — `dws upgrade`、`dws upgrade --check` 和 `dws doctor` 比较版本时按 SemVer 忽略 build metadata（`+...`）。此前当前或远端版本号带 `+` 时，可能把更旧或相同的版本误报为可升级，或漏报真正的新版本。
