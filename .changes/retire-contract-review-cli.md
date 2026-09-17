---
category: Removed
---

- **Retire legacy contract review CLI leaves** — hide `dws contract review` and `benefit|create|analysis|result` from public help, mark the leaf Agent Schema availability `unavailable`, and fail closed with `command_retired` without calling legacy review MCP tools. Historical argv remains parseable. Ledger `schema_availability_hardening` entries are consumed.
