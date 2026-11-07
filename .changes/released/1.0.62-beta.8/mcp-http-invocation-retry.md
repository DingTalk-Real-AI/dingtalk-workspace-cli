---
category: Fixed
---

- Stop implicit HTTP retries of MCP tool invocations after gateway or connection failures, preventing duplicate datasource updates and other remote writes. Discovery and explicit read/reconciliation retry policies retain their existing behavior.
