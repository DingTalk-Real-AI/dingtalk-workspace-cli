---
category: Changed
---

- Introduce Schema delivery inside the existing Schema command of the single `dws` executable. Production does not produce or embed Schema identity at compile or release time. On supported ends (darwin/linux/windows amd64/arm64) each machine generates identity from live declarations at install or first `dws schema`, writes authenticated protobuf shards under the shared or user cache directory, and later hits verify digests then read those shards. Miss or corruption repairs from live assembly. Empty local identity generates then uses the cache; it is not a permanent live-only mode. Persistent cache is unused when plugins or other runtime extensions change the command surface.
- Keep one complete Cobra tree for every public invocation. Compact typed metadata and shared builders reduce complete-tree allocations; process argv does not select a product factory or a utility-only tree.
- Telemetry exit no longer waits for delivery: the CLI default enables `NoFlushWait`, so the process returns right after the completion event is enqueued and the last event is expected to be lost. `FlushTimeout` (SDK default ~300ms) remains an SDK option for callers that accept a bounded wait; reliable non-blocking delivery needs the durable outbox follow-up. Business cleanup, signals, and exit codes remain synchronous.
- Reduce temporary allocations during Schema validation and command initialization.
- Normative notes live in `docs/rfc-schema-runtime-cache.md` only; no sibling plan/design/performance pages are kept.
