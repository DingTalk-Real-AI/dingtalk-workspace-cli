---
category: Fixed
---

- **Contact apply-list safety semantics** — `dws contact org apply-list` marks
  unread join applications as read on the server; its schema safety effect is
  now declared as `write` (risk stays `low`, confirmation stays
  `not_required`) and the selection guidance discloses the read-marking side
  effect so Agents no longer treat it as a pure read.
