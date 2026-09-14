---
category: Changed
---

- **Attendance and Mail Shortcuts** (#1045) — publishes only capabilities with
  strict response, identity, pagination, and real-data verification while
  retaining historical CLI discovery and argument compatibility for commands
  that remain unavailable to agents. Mailbox auto-resolution now accepts both
  reviewed string and object response shapes, and Attendance date ranges cover
  the complete requested end date without dropping cross-midnight punches whose
  actual check time is inside the requested range. The schedule query remains
  CLI-compatible but is withheld from the Agent catalog because its downstream
  service returns a successful process exit with a null body for both populated
  and empty ranges.
