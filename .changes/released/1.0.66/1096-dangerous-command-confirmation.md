---
category: Changed
---

- **Dangerous command confirmation** (#1096) — `calendar event delete`, `calendar attendee delete`, `calendar room delete`, `chat group members remove`, `minutes replace-text`, and `doc permission update` now require explicit confirmation and publish `high` risk in Schema. Event deletion and minutes text replacement also publish `destructive` effects. Non-interactive scripts must pass `--yes` after obtaining user approval; otherwise these commands return `confirmation_required` without calling the remote service. The mono skill's dangerous-operation table now uses the canonical `calendar attendee delete` path for Schema lookup.
