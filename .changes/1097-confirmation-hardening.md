---
category: Changed
---

- **Dangerous command confirmation hardening** (#1097) — 6 commands that affect other users and are irreversible now require `--yes` confirmation: `calendar event delete`, `calendar attendee delete`, `calendar room delete`, `chat group members remove`, `minutes replace-text`, `doc permission update`. Scripts calling these commands without `--yes` will receive a `confirmation_required` validation error instead of executing silently.
