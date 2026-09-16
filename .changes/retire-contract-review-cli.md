---
category: Removed
---

- **Retire legacy contract review CLI leaves** — hide `dws contract review benefit|create|analysis|result` from help and mark their Agent Schema availability `unavailable`. The commands remain as hidden historical argv stubs and fail closed with `command_retired` without calling legacy review MCP tools. Ledger `schema_availability_hardening` entries are consumed.
