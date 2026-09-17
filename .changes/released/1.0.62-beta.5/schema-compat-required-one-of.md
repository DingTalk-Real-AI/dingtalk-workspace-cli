---
category: Fixed
---

- **Schema compatibility checks** — accept a new `require_one_of` group when a historical unconditional required parameter without a default already guarantees a supplied member. Other incompatible parameter changes remain rejected; CLI runtime behavior is unchanged.
