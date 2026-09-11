---
category: Changed
---

- Introduce Schema delivery inside the existing Schema command of the single `dws` executable. Production does not produce or embed Schema identity at compile or release time. On supported ends (darwin/linux/windows amd64/arm64) each machine generates identity from live declarations at install or first `dws schema`, writes authenticated protobuf shards under the shared or user cache directory, and later hits verify digests then read those shards. Miss or corruption repairs from live assembly; when the shared cache exists but cannot be locked for repair (typically root-owned read-only), the repair falls back to the per-user cache and later processes reuse it. Empty local identity generates then uses the cache; it is not a permanent live-only mode. When plugins or other runtime extensions change the command surface, cache publication/repair/prewarm are skipped and command help/schema queries keep serving read-only from the cache (plugin commands never enter the Schema surface, cached or live).
- Keep one complete Cobra tree for every public invocation. Compact typed metadata and shared builders reduce complete-tree allocations; process argv does not select a product factory or a utility-only tree.
- Reduce temporary allocations during Schema validation and command initialization.
- Normative notes live in `docs/rfc-schema-runtime-cache.md` only; no sibling plan/design/performance pages are kept.
