---
category: Added
---

- **html fetch / create / overwrite / patch** — full native `.html` / `.htm` file support in DingPan or the doc space, mirroring the markdown domain. `create` accepts a literal string, `@file`, stdin (`-`), or an existing local HTML file via `--file`. `fetch` downloads and prints the remote content (optional sanitized `--output`). `overwrite` replaces the whole file with before/after preview on command-level `--dry-run`; `patch` applies literal or RE2 replacements with zero-match never writing and an empty result aborting. Routing matches the markdown leaves: explicit `--space-id` / `--workspace`, auto domain probe, `--folder` read-only probe on create. Drive uploads submit the `text/html` MIME type. Implemented on a shared textfile engine extracted from the markdown leaves (pure refactor, behavior unchanged).
