---
category: Security
---

- **Password argv sanitization** — add `--password` to the timing/perf report sensitive-flag allowlist so `SanitizeCommand` redacts it in both `--password <value>` and `--password=<value>` forms. This also fixes a pre-existing plaintext leak where `dws drive publish set --password …` wrote the raw secret into the `DWS_PERF_REPORT` argv record. The `drive permission set-share-scope --dry-run` preview now masks the password value as `***` while still emitting `requirePassword`; real calls keep sending the correct secret and no business RPC is issued during preview.
