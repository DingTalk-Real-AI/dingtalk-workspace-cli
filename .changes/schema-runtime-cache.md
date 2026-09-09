---
category: Changed
---

- Introduce release-identity-verified Schema caching inside the existing Schema command of the single `dws` executable. Cache hits authenticate binary-pinned digests before decode; a miss, corruption, or empty identity falls back to live declaration assembly. Persistent cache is unused when plugins or other runtime extensions change the command surface.
- Keep one complete Cobra tree for every public invocation. Compact typed metadata and shared builders reduce complete-tree allocations; process argv does not select a product factory or a utility-only tree.
- Do not wait for telemetry delivery when the process exits. The tracker is configured not to flush, so a one-shot invocation may exit before the queued HTTP send completes; business cleanup, signals, and exit codes remain synchronous.
- Reduce temporary allocations during Schema validation and command initialization.
