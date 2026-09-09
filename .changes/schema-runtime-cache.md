---
category: Changed
---

- Introduce Schema delivery inside the existing Schema command of the single `dws` executable. Production does not produce or embed Schema identity at compile or release time; schema handlers assemble from live declarations. Authenticated disk cache remains available to tests that inject identity, and does not depend on sealed ldflags. Persistent cache is unused when plugins or other runtime extensions change the command surface.
- Keep one complete Cobra tree for every public invocation. Compact typed metadata and shared builders reduce complete-tree allocations; process argv does not select a product factory or a utility-only tree.
- Keep a bounded best-effort wait for telemetry delivery on process exit (SDK default FlushTimeout, about 300ms), matching main. `NoFlushWait` remains an optional SDK field for callers that accept last-event loss; the CLI default does not set it. Business cleanup, signals, and exit codes remain synchronous.
- Reduce temporary allocations during Schema validation and command initialization.
