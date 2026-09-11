---
category: Fixed
---

- **Event group subscription validation** (#890) — reject comma-separated `--group` values before creating a subscription, including dry-run and multi-event commands, instead of accepting multiple groups and silently missing messages. Help now directs callers to start a separate consumer for each group.
