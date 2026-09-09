---
category: Deprecated
---

- **Contract review CLI leaves** (#1330) — `dws contract review benefit|create|analysis|result` remain as historical argv compatibility shims but no longer call retired MCP tools. They fail closed immediately with a non-zero `command_retired` error (without reading legacy business input), are marked Cobra Deprecated, and publish deprecation-only Agent selection. Schema availability stays `available` in this PR while pending `schema_availability_hardening` ledger entries land; a follow-up PR will flip availability to `unavailable` and mark those migrations consumed.
