---
category: Added
---

- **Doc-business delegation auth** — the `drive`, `doc`, `sheet`, `wiki`, and `markdown` command groups now accept a persistent `--principal-user-id` flag. When set, each doc-business tool call is preceded by a `check_capability` verification on behalf of the principal; granting the capability is an out-of-band action the principal completes on the server side, and the CLI never calls `grant_capability`. A denied check surfaces the server's denial message and blocks the original call.
