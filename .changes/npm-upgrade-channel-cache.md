---
category: Fixed
---

- **Reliable upgrade and download flow** — resolves stable and beta checks from npm dist-tags with a 10-minute cache, bypasses the cache for explicit installs, preserves npm/pnpm ownership without falling back when the owner is unavailable, verifies bundled and direct-download assets with strict streamed SHA256, and publishes retried downloads atomically.
