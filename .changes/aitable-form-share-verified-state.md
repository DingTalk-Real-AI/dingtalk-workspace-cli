---
category: Fixed
---

- **AI Table form sharing** now exposes server-returned `shareFormUuid`, status, and cover through shared structured result contracts for atomic commands and shortcuts; updates also publish `cpSynced`. Agent guidance requires confirming CP synchronization before reporting the sharing workflow as complete. Readback and CP projection remain the server's responsibility; DWS does not issue a second view update or construct cover URLs.
- Atomic and shortcut updates now share strict terminal validation. Missing or invalid required fields, including `cpSynced=false`, produce `partial_failure` (exit 7), preserve the remote response and mark remote execution as started without retrying the write. Result Schema documents both verified success and partial-stage evidence. Usage guidance distinguishes supplied IDs from placeholders and preserves the shortcut JSON flag.
