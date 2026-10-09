---
category: Added
---

- **Attendance approval helpers** — adds shortcuts to calculate attendance approval duration, validate travel/out-of-office companion schedules, and query employees' effective complex overtime settings. Proposed overtime durations (total and per-day detail entries) must be finite positive numbers matched to the requested duration unit, and numeric detail-list work dates must be 13-digit millisecond timestamps; anything else is rejected before the server call. Flag help publishes each custom constraint's decision facts so the delivered schema keeps the validation contract visible to agents, and the detail-list help matches the two-stage workflow (second-stage input; the first stage omits it to obtain the server's per-day skeleton).
