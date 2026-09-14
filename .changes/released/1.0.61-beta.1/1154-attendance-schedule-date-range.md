---
category: Fixed
---

- **Attendance schedule date ranges** (#1154) — sends `attendance schedule get`
  date ranges as upstream datetime strings, expands date-only inputs to full-day
  boundaries, and rejects reversed ranges before calling the service.
