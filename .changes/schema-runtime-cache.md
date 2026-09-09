---
category: Changed
---

- Introduce Schema delivery inside the existing Schema command of the single `dws` executable. Production does not produce or embed Schema identity at compile or release time. On supported ends (darwin/linux amd64/arm64) each machine generates identity from live declarations at install or first `dws schema`, writes authenticated protobuf shards under the shared or user cache directory, and later hits verify digests then read those shards. Miss or corruption repairs from live assembly. Empty local identity generates then uses the cache; it is not a permanent live-only mode. Windows stays live-only. Persistent cache is unused when plugins or other runtime extensions change the command surface.
- Keep one complete Cobra tree for every public invocation. Compact typed metadata and shared builders reduce complete-tree allocations; process argv does not select a product factory or a utility-only tree.
- Keep a bounded best-effort wait for telemetry delivery on process exit (`FlushTimeout` 50ms), chosen as a latency/delivery compromise versus NoFlushWait and main's ~300ms SDK default. `NoFlushWait` remains an optional SDK field for callers that accept last-event loss; the CLI default does not set it. Business cleanup, signals, and exit codes remain synchronous.
- Reduce temporary allocations during Schema validation and command initialization.
- Normative notes live in `docs/rfc-schema-runtime-cache.md` only; sibling plan/design/performance pages are pointers to that RFC.
