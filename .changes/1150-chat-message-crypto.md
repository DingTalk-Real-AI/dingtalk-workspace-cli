---
category: Added
---

- **Chat third-party message decrypt** (#1150) — adds policy-driven Ding + SafeChat message decryption for core chat read paths, explicit `dws chat crypto decrypt` diagnostics, and IM MCP wiring for policy lookup plus Ding batch decrypt. Outbound send encryption and `dws chat crypto encrypt` are intentionally not enabled in this PR.
