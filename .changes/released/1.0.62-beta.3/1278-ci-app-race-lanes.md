---
category: Changed
---

- **Bounded CI app race fan-out** (#1278) — keeps all nine reviewed
  `internal/app` race-test partitions process-isolated while balancing them
  across three physical jobs, reducing focused and full-suite runner demand by
  six jobs without weakening partition coverage or increasing the 20-minute
  job limit.
