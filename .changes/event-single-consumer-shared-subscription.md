---
category: Fixed
---

- **Personal event subscription cleanup** — single-event consumers sharing an idempotently reused subscription on the same host and configuration directory now retain it until the final participating consumer exits. Cleanup failures remain visible in stderr. Multi-event consumers and older binaries do not participate in this coordination.
