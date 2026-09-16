---
category: Fixed
---

- **AI Table form sharing** now exposes server-returned `shareFormUuid`, status, and cover through shared structured result contracts for atomic commands and shortcuts; updates also publish `cpSynced`. Agent guidance requires confirming CP synchronization before reporting the sharing workflow as complete. Readback and CP projection remain the server's responsibility; DWS does not issue a second view update or construct cover URLs.
- Atomic and shortcut updates now share strict terminal validation. Missing or invalid required fields, including `cpSynced=false`, produce `partial_failure` (exit 7), preserve the remote response and mark remote execution as started without retrying the write. Result Schema documents both verified success and partial-stage evidence. Usage guidance distinguishes supplied IDs from placeholders and preserves the shortcut JSON flag.
- Terminal validation binds `baseId`, `tableId`, and `viewId` to the original request before accepting CP synchronization. A complete response for a different form is still `partial_failure`, even with `cpSynced=true`; both command entries retain the raw response and never replay or compensate the write.
- All four form-share Result Schemas accept their real dry-run request previews, including the optional shortcut `dry_run` marker. Preview success never implies remote execution or CP synchronization; assembled-leaf regression tests validate real envelopes against both full and compact JSON Schemas.
- Skill discovery preserves the requested atomic/shortcut entry for result reviews instead of substituting usage Help. Interpretation selects the actual result branch and does not invent required fields or copy Schema evidence into malformed samples.
