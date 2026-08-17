---
category: Added
---

- **Agent version and extended context passthrough** (Aone 85384225) — adds
  validated `DWS_AGENT_VER` and sensitive JSON `DWS_AGENT_EXT` metadata to
  ordinary non-plugin MCP requests without forwarding it to A2A, OAuth,
  Discovery, or third-party plugins.
