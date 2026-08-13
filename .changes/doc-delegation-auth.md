---
category: Added
---

- **Doc-business delegation auth** — the `drive`, `doc`, `sheet`, `wiki`, and `markdown` command groups now accept a persistent `--principal-user-id` flag. When set, each doc-business tool call is preceded by a one-time `grant_capability` + `check_capability` handshake on behalf of the principal; a denied check surfaces the server's denial message and blocks the original call.
