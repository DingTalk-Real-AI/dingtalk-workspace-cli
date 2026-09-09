---
category: Changed
---

- **Chat recent conversations** — makes `--start` optional for `dws chat +recent-conversations`. Omission selects the 24 hours before the effective `--end`; omitting both boundaries selects the latest 24 hours ending at the current time rounded down to a whole second. Explicit `--start` retains the existing time formats, whole-second validation, and blank-input rejection. Both effective boundaries stay fixed across pagination and are returned in the result. Non-initial `--cursor` requests must explicitly reuse the previous `--start` and `--end` to preserve the original query window.
