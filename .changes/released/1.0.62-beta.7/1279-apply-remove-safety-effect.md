---
category: Fixed
---

- **Contact apply-remove safety semantics** — `dws contact org apply-remove`
  deletes organization join application records irreversibly; its schema safety
  effect is now declared as `destructive` (risk `high`, confirmation
  `user_required`) and the selection guidance discloses that the deletion is
  not recoverable so Agents no longer treat it as an ordinary write.
