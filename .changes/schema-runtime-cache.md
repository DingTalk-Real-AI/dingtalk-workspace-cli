---
category: Changed
---

- Introduce release-identity-verified Schema caching with live declaration fallback in the existing single `dws` executable.
- Reduce command startup work by mounting only the selected product when the runtime command surface is static.
- Stop waiting for telemetry delivery after command completion; the final queued event is best effort and may be lost when the process exits.
- Reduce temporary allocations during Schema validation and command initialization.
