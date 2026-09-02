---
category: Fixed
---

- **Drive download concurrent-writer safety** — `dws drive download` and `dws drive download-version` no longer risk publishing corrupted mixed content when two processes download to the same target concurrently. Streamed (non-ranged) downloads now write to a uniquely created temp file in the target directory instead of the shared `<target>.dwspart`, so concurrent writers can no longer truncate each other. Ranged/resume downloads keep the fixed `.dwspart` path (required for checkpoint reuse) and take a cross-process lock (`<target>.dwspart.lock`): a second concurrent writer fails fast with holder diagnostics (pid/host/start time) instead of interleaving writes; the atomic no-replace publish still guards the final target either way.
