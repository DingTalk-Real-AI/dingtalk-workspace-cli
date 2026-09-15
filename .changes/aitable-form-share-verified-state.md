---
category: Fixed
---

- **AI Table form sharing** now exposes server-returned `shareFormUuid`, status, and cover through shared structured result contracts for atomic commands and shortcuts; updates also publish `cpSynced`. Agent guidance requires confirming CP synchronization before reporting the sharing workflow as complete. Readback and CP projection remain the server's responsibility; DWS does not issue a second view update or construct cover URLs.
